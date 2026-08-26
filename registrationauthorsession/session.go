package registrationauthorsession

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

// Browser opens one live, non-persistent, no-submit authoring context.
type Browser interface {
	Open(context.Context, BrowserRequest) (Session, error)
}

// Session is the entire live-browser authority available to the protocol.
// It deliberately has no input, focus, click, submit, POST, origin-expansion,
// state-reading, capture, script, or session-export operation.
type Session interface {
	Observe(context.Context) (RawObservation, error)
	Navigate(context.Context, Navigation) error
	Close(context.Context) (NetworkSummary, error)
}

// BrowserRequest fixes all authority before a browser can be opened.
type BrowserRequest struct {
	// Protocol selects the immutable v1 or additive retained-query v2 URL rule.
	Protocol string
	// URL is the initial GET navigation. Implementations must apply the same
	// exact-origin and network guard used for later Navigation values.
	URL               string
	ApprovedOrigins   []string
	NavigationTimeout time.Duration
	TotalTimeout      time.Duration
	MaxRequests       int
	MaxResponseBytes  int64
	MaxCandidates     int
}

// Navigation is a closed GET/HEAD navigation request.
type Navigation struct {
	Method string
	URL    string
}

// ServeOptions controls local process behavior and confers no browser
// authority.
type ServeOptions struct {
	Clock    func() time.Time
	Protocol string
}

type server struct {
	ctx             context.Context
	browser         Browser
	session         Session
	output          io.Writer
	clock           func() time.Time
	protocol        string
	phase           string
	closed          bool
	profileID       string
	bounds          Bounds
	origins         []string
	originSet       map[string]struct{}
	candidates      map[string]candidateRecord
	generation      int
	observations    int
	diagnostics     []string
	diagnosticSet   map[string]struct{}
	observedAt      time.Time
	reviewedProfile *registrationReview
	activeRemaining time.Duration
}

type registrationReview struct {
	profile    registrationProfile
	bytes      []byte
	candidates []ReviewedCandidate
	flow       string
	cleanup    string
}

// These aliases keep the state machine declarations compact while retaining
// the public UWS wire type in Completion.
type registrationProfile = registrationprofile.Profile

// Serve processes one no-submit authoring session until finish, close, or a
// fail-closed terminal condition. A Completion is returned only after a
// reviewed profile and clean, policy-conforming browser teardown.
func Serve(ctx context.Context, input io.ReadCloser, output io.Writer, browser Browser, options ServeOptions) (*Completion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || output == nil || browser == nil {
		return nil, errors.New("registration author-session input, output, and browser are required")
	}
	var closeInputOnce sync.Once
	closeInput := func() { closeInputOnce.Do(func() { _ = input.Close() }) }
	defer closeInput()
	stopCancelClose := context.AfterFunc(ctx, closeInput)
	defer stopCancelClose()
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.Protocol == "" {
		options.Protocol = ProtocolV1
	}
	if options.Protocol != ProtocolV1 && options.Protocol != ProtocolV2 {
		return nil, errors.New("registration author-session protocol is unsupported")
	}
	s := &server{
		ctx: ctx, browser: browser, output: output, clock: options.Clock,
		protocol: options.Protocol,
		phase:    "awaiting_start", originSet: make(map[string]struct{}),
		candidates: make(map[string]candidateRecord), diagnosticSet: make(map[string]struct{}),
	}
	if err := s.write(ServerMessage{
		Type: "hello",
		Capabilities: []string{
			"get_head_only", "no_submit", "reduced_observation", "registration_review",
		},
	}); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), MaxProtocolLineBytes)
	for scanner.Scan() {
		message, err := decodeClientMessage(scanner.Bytes())
		if err != nil {
			return nil, s.fail("malformed_message")
		}
		completion, done, err := s.handle(message)
		if err != nil {
			if !s.closed {
				_, _ = s.closeSession()
				s.closed = true
			}
			return completion, err
		}
		if done {
			return completion, nil
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil, s.cancel()
		}
		return nil, s.fail("protocol_limit")
	}
	if ctx.Err() != nil {
		return nil, s.cancel()
	}
	if s.closed {
		return nil, nil
	}
	return nil, s.fail("unexpected_eof")
}

func (s *server) handle(message ClientMessage) (*Completion, bool, error) {
	if s.closed {
		return nil, true, errors.New("registration author session is closed")
	}
	if message.Protocol != s.protocol {
		return nil, true, s.fail("protocol_mismatch")
	}
	if _, known := clientFields[message.Type]; !known || message.Type == "unknown" {
		return nil, true, s.fail("unknown_message")
	}
	if !phaseMessages[s.phase][message.Type] {
		return nil, true, s.fail("invalid_state")
	}
	switch message.Type {
	case "start":
		return nil, false, s.start(message)
	case "observe":
		return nil, false, s.observe()
	case "navigate":
		return nil, false, s.navigate(message)
	case "review":
		return nil, false, s.review(message)
	case "finish":
		completion, err := s.finish()
		return completion, true, err
	case "close":
		return nil, true, s.closeWithoutResult()
	default:
		return nil, true, s.fail("unknown_message")
	}
}

func (s *server) start(message ClientMessage) error {
	if !identifierPattern.MatchString(message.ProfileID) {
		return s.fail("invalid_start")
	}
	initialURL, initialOrigin, err := cleanURLForProtocol(s.protocol, message.URL)
	if err != nil || initialURL != message.URL {
		return s.fail("invalid_start")
	}
	origins, err := normalizeOrigins(message.Origins)
	if err != nil {
		return s.fail("invalid_origin")
	}
	if !contains(origins, initialOrigin) {
		return s.fail("invalid_origin")
	}
	if err := validateBounds(message.Bounds); err != nil {
		return s.fail("invalid_bounds")
	}
	bounds := normalizedBounds(message.Bounds)
	s.bounds = bounds
	s.profileID = message.ProfileID
	s.origins = origins
	for _, origin := range origins {
		s.originSet[origin] = struct{}{}
	}
	s.activeRemaining = time.Duration(bounds.TotalTimeoutMS) * time.Millisecond
	var session Session
	err = s.withActiveContext(func(callCtx context.Context) error {
		var openErr error
		session, openErr = s.browser.Open(callCtx, BrowserRequest{
			Protocol: s.protocol, URL: initialURL, ApprovedOrigins: append([]string(nil), origins...),
			NavigationTimeout: time.Duration(bounds.NavigationTimeoutMS) * time.Millisecond,
			TotalTimeout:      time.Duration(bounds.TotalTimeoutMS) * time.Millisecond,
			MaxRequests:       bounds.MaxRequests,
			MaxResponseBytes:  bounds.MaxResponseBytes,
			MaxCandidates:     bounds.MaxCandidates,
		})
		return openErr
	})
	if session != nil {
		s.session = session
	}
	if err != nil || session == nil {
		return s.failBrowser()
	}
	s.phase = "observing"
	return s.write(ServerMessage{Type: "state", Phase: s.phase, Bounds: &s.bounds})
}

func (s *server) observe() error {
	s.observations++
	if s.observations > s.bounds.MaxObservations {
		return s.fail("observation_limit")
	}
	var raw RawObservation
	err := s.withActiveContext(func(callCtx context.Context) error {
		var observeErr error
		raw, observeErr = s.session.Observe(callCtx)
		return observeErr
	})
	if err != nil {
		return s.failBrowser()
	}
	observation, records, err := s.reduceObservation(raw)
	if err != nil {
		return s.fail("invalid_observation")
	}
	observedAt := s.clock().UTC().Truncate(time.Second)
	if observedAt.IsZero() || (!s.observedAt.IsZero() && observedAt.Before(s.observedAt)) {
		return s.fail("clock_unavailable")
	}
	clear(s.candidates)
	for id, record := range records {
		s.candidates[id] = record
	}
	s.reviewedProfile = nil
	s.observedAt = observedAt
	return s.write(ServerMessage{Type: "observation", Observation: &observation})
}

func (s *server) navigate(message ClientMessage) error {
	if message.Method != "GET" && message.Method != "HEAD" {
		return s.fail("invalid_navigation")
	}
	canonicalURL, origin, err := cleanURLForProtocol(s.protocol, message.URL)
	if err != nil || canonicalURL != message.URL {
		return s.fail("invalid_navigation")
	}
	if _, ok := s.originSet[origin]; !ok {
		return s.fail("invalid_origin")
	}
	clear(s.candidates)
	s.reviewedProfile = nil
	err = s.withActiveContext(func(callCtx context.Context) error {
		return s.session.Navigate(callCtx, Navigation{Method: message.Method, URL: canonicalURL})
	})
	if err != nil {
		return s.failBrowser()
	}
	return s.write(ServerMessage{Type: "state", Phase: s.phase})
}

func (s *server) review(message ClientMessage) error {
	if s.observations == 0 || len(s.candidates) == 0 || len(message.Profile) == 0 {
		return s.fail("invalid_review")
	}
	profileValue, err := registrationprofile.Parse(message.Profile)
	if err != nil {
		return s.fail("invalid_profile")
	}
	if profileValue.ObservationKind != "accessibility_snapshot" {
		return s.fail("invalid_profile")
	}
	now := s.clock().UTC().Round(0)
	if now.IsZero() || registrationprofile.ValidateAt(profileValue, now) != nil {
		return s.fail("invalid_profile")
	}
	profileBytes, err := registrationprofile.MarshalJSON(profileValue)
	if err != nil {
		return s.fail("invalid_profile")
	}
	if !equalStrings(registrationprofile.Origins(profileValue), s.origins) {
		return s.fail("origin_mismatch")
	}
	if s.protocol == ProtocolV2 && registrationprofile.ValidateRetainedNavigationV2(profileValue) != nil {
		return s.fail("invalid_profile")
	}
	if !identifierPattern.MatchString(message.Flow) {
		return s.fail("invalid_flow")
	}
	if _, ok := profileValue.Flows[message.Flow]; !ok {
		return s.fail("invalid_flow")
	}
	if message.CleanupDisposition != "delete_separately" && message.CleanupDisposition != "retain_dedicated_test_identity" {
		return s.fail("invalid_cleanup")
	}
	if len(message.CandidateIDs) == 0 || len(message.CandidateIDs) > s.bounds.MaxCandidates || !sort.StringsAreSorted(message.CandidateIDs) {
		return s.fail("invalid_candidate")
	}
	reviewed := make([]ReviewedCandidate, 0, len(message.CandidateIDs))
	seen := make(map[string]struct{}, len(message.CandidateIDs))
	for _, id := range message.CandidateIDs {
		if _, duplicate := seen[id]; duplicate {
			return s.fail("invalid_candidate")
		}
		seen[id] = struct{}{}
		record, ok := s.candidates[id]
		if !ok || record.protocol.Matches != 1 || !promotableCandidate(record.protocol) {
			return s.fail("invalid_candidate")
		}
		reviewed = append(reviewed, ReviewedCandidate{
			ID: id, Generation: record.generation, Role: record.protocol.Role,
			Label: record.protocol.Label, Matches: record.protocol.Matches,
		})
	}
	if err := validateReviewedSubmit(profileValue, message.Flow, reviewed); err != nil {
		return s.fail("invalid_submit")
	}
	s.reviewedProfile = &registrationReview{
		profile: *profileValue, bytes: append([]byte(nil), profileBytes...), candidates: reviewed,
		flow: message.Flow, cleanup: message.CleanupDisposition,
	}
	s.phase = "reviewed"
	return s.write(ServerMessage{Type: "state", Phase: s.phase})
}

func validateReviewedSubmit(profileValue *registrationprofile.Profile, flowName string, reviewed []ReviewedCandidate) error {
	flow, ok := profileValue.Flows[flowName]
	if !ok {
		return errors.New("reviewed registration flow is missing")
	}
	submitCount := 0
	matchingCandidates := 0
	for _, step := range flow.Sequence {
		if step.Submit == nil {
			continue
		}
		submitCount++
		locator := step.Submit.Locator
		if locator.Role == "" || locator.Text != "" || locator.Value != "" ||
			locator.Name == "" || !safeCandidateLabel(locator.Name) ||
			locator.Name == authorsession.RedactedLabel || locator.Name == authorsession.UntrustedLabel {
			return errors.New("reviewed submit locator is not accessibility-name portable")
		}
		for _, candidate := range reviewed {
			if candidate.Role == locator.Role && candidate.Label == locator.Name && candidate.Matches == 1 {
				matchingCandidates++
			}
		}
	}
	if submitCount != 1 || matchingCandidates != 1 {
		return errors.New("reviewed submit does not bind one exact current candidate")
	}
	return nil
}

func (s *server) finish() (*Completion, error) {
	if s.reviewedProfile == nil {
		return nil, s.fail("invalid_state")
	}
	summary, err := s.closeSession()
	if err != nil {
		return nil, s.failAfterClose("teardown_failure")
	}
	if err := validateNetworkSummary(summary, s.bounds.MaxRequests); err != nil {
		return nil, s.failAfterClose("network_policy")
	}
	s.closed = true
	s.phase = "closed"
	if err := s.write(ServerMessage{Type: "state", Phase: s.phase}); err != nil {
		return nil, err
	}
	review := s.reviewedProfile
	return &Completion{
		Protocol: s.protocol, ProfileID: s.profileID, Profile: review.profile,
		ProfileBytes:       append([]byte(nil), review.bytes...),
		ReviewedCandidates: append([]ReviewedCandidate(nil), review.candidates...),
		Flow:               review.flow,
		CleanupDisposition: review.cleanup,
		Origins:            append([]string(nil), s.origins...), ObservedAt: s.observedAt,
		Bounds: s.bounds, Observations: s.observations,
		Diagnostics: append([]string(nil), s.diagnostics...), Network: summary,
	}, nil
}

func (s *server) closeWithoutResult() error {
	summary, err := s.closeSession()
	s.closed = true
	s.phase = "closed"
	if err != nil {
		return s.failAfterClose("teardown_failure")
	}
	if err := validateNetworkSummary(summary, s.bounds.MaxRequests); err != nil && s.profileID != "" {
		return s.failAfterClose("network_policy")
	}
	return s.write(ServerMessage{Type: "state", Phase: s.phase})
}

func (s *server) reduceObservation(raw RawObservation) (Observation, map[string]candidateRecord, error) {
	origin, err := exactOrigin(raw.Origin)
	if err != nil {
		return Observation{}, nil, err
	}
	if _, ok := s.originSet[origin]; !ok {
		return Observation{}, nil, errors.New("observation escaped approved origins")
	}
	if disclosurepath.Validate(raw.Path) != nil {
		return Observation{}, nil, errors.New("observation path is not disclosure-safe")
	}
	if len(raw.Candidates) > s.bounds.MaxCandidates {
		return Observation{}, nil, errors.New("candidate limit exceeded")
	}
	if len(raw.Diagnostics) > MaxUniqueDiagnostics {
		return Observation{}, nil, errors.New("diagnostic limit exceeded")
	}
	s.generation++
	sort.Slice(raw.Candidates, func(i, j int) bool {
		left, right := raw.Candidates[i], raw.Candidates[j]
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		return left.Label < right.Label
	})
	observation := Observation{
		Generation: s.generation, Origin: origin, Path: raw.Path,
		Candidates: []Candidate{}, Diagnostics: []string{},
	}
	records := make(map[string]candidateRecord, len(raw.Candidates))
	reducedLocators := make(map[string]struct{}, len(raw.Candidates))
	for index, rawCandidate := range raw.Candidates {
		if len(rawCandidate.Label) > MaxRawCandidateLabelBytes ||
			!utf8.ValidString(rawCandidate.Label) || !portableRoles[rawCandidate.Role] ||
			rawCandidate.Matches < 1 || rawCandidate.Matches > s.bounds.MaxCandidates {
			return Observation{}, nil, errors.New("backend candidate is invalid")
		}
		label := authorsession.ReduceAccessibilityLabel(rawCandidate.Label).Value
		if !safeCandidateLabel(label) {
			return Observation{}, nil, errors.New("reduced candidate label is invalid")
		}
		locatorKey := rawCandidate.Role + "\x00" + label
		if _, duplicate := reducedLocators[locatorKey]; duplicate {
			return Observation{}, nil, errors.New("reduced candidate locator is duplicated")
		}
		reducedLocators[locatorKey] = struct{}{}
		id := candidateID(s.generation, rawCandidate.Role, label, index)
		candidate := Candidate{ID: id, Role: rawCandidate.Role, Label: label, Matches: rawCandidate.Matches}
		observation.Candidates = append(observation.Candidates, candidate)
		records[id] = candidateRecord{protocol: candidate, generation: s.generation}
	}
	seenThisObservation := make(map[string]struct{}, len(raw.Diagnostics))
	for _, code := range raw.Diagnostics {
		if !ValidDiagnostic(code) {
			return Observation{}, nil, errors.New("backend diagnostic is invalid")
		}
		if _, duplicate := seenThisObservation[code]; duplicate {
			continue
		}
		seenThisObservation[code] = struct{}{}
		observation.Diagnostics = append(observation.Diagnostics, code)
		if _, exists := s.diagnosticSet[code]; !exists {
			if len(s.diagnosticSet) >= MaxUniqueDiagnostics {
				return Observation{}, nil, errors.New("diagnostic limit exceeded")
			}
			s.diagnosticSet[code] = struct{}{}
			s.diagnostics = append(s.diagnostics, code)
		}
	}
	sort.Strings(observation.Diagnostics)
	sort.Strings(s.diagnostics)
	return observation, records, nil
}

func (s *server) withActiveContext(run func(context.Context) error) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if s.activeRemaining <= 0 {
		return context.DeadlineExceeded
	}
	callCtx, cancel := context.WithTimeout(s.ctx, s.activeRemaining)
	started := time.Now()
	err := run(callCtx)
	elapsed := time.Since(started)
	cancel()
	if elapsed >= s.activeRemaining {
		s.activeRemaining = 0
		if err == nil {
			return context.DeadlineExceeded
		}
	} else {
		s.activeRemaining -= elapsed
	}
	return err
}

func (s *server) closeSession() (NetworkSummary, error) {
	if s.session == nil {
		return NetworkSummary{}, nil
	}
	session := s.session
	s.session = nil
	timeout := DefaultNavigationTimeout
	if s.bounds.NavigationTimeoutMS > 0 {
		timeout = time.Duration(s.bounds.NavigationTimeoutMS) * time.Millisecond
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return session.Close(closeCtx)
}

func (s *server) cancel() error {
	_, _ = s.closeSession()
	s.closed = true
	_ = s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: "canceled"}})
	return errors.New("registration author session failed: canceled")
}

func (s *server) fail(code string) error {
	_, _ = s.closeSession()
	s.closed = true
	writeErr := s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: code}})
	if writeErr != nil {
		return errors.Join(fmt.Errorf("registration author session failed: %s", code), writeErr)
	}
	return fmt.Errorf("registration author session failed: %s", code)
}

func (s *server) failBrowser() error {
	if s.ctx.Err() != nil {
		return s.cancel()
	}
	return s.fail("browser_failure")
}

func (s *server) failAfterClose(code string) error {
	s.closed = true
	writeErr := s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: code}})
	if writeErr != nil {
		return errors.Join(fmt.Errorf("registration author session failed: %s", code), writeErr)
	}
	return fmt.Errorf("registration author session failed: %s", code)
}

func (s *server) write(message ServerMessage) error {
	message.Protocol = s.protocol
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	written, err := s.output.Write(append(data, '\n'))
	if err != nil || written != len(data)+1 {
		return errors.New("registration author-session output failed")
	}
	return nil
}

func candidateID(generation int, role, label string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%d", generation, role, label, index)))
	return "candidate-" + hex.EncodeToString(sum[:8])
}

func promotableCandidate(candidate Candidate) bool {
	return candidate.Label != "" && candidate.Label != authorsession.RedactedLabel &&
		candidate.Label != authorsession.UntrustedLabel && safeCandidateLabel(candidate.Label)
}

func contains(values []string, target string) bool {
	index := sort.SearchStrings(values, target)
	return index < len(values) && values[index] == target
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
