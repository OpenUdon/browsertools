// Package authorsession implements the strict local protocol for one
// authenticated goal-directed browser authoring process. The browser backend
// owns all live handles; the protocol exposes reduced semantics only.
package authorsession

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/OpenUdon/browsertools/authorresult"
)

const Protocol = "browsertools.author-session.v1"

const (
	DefaultNavigationTimeout = 20 * time.Second
	DefaultTotalTimeout      = 10 * time.Minute
	DefaultMaxRequests       = 512
	DefaultMaxResponseBytes  = int64(32 << 20)
	DefaultMaxObservations   = 64
	DefaultMaxCandidates     = 128
	maxProtocolLineBytes     = 64 << 10
)

// Browser opens one live non-persistent authoring context.
type Browser interface {
	Open(context.Context, BrowserRequest) (Session, error)
}

// Session is the only live-browser surface visible to the state machine. It
// contains no API for input values, cookies, storage, or session export.
type Session interface {
	Observe(context.Context, string) (RawObservation, error)
	Focus(context.Context, string) error
	Execute(context.Context, BrowserAction) (Execution, error)
	AddOrigin(string) error
	Close() error
}

// BrowserRequest fixes the browser-owned authority before launch.
type BrowserRequest struct {
	URL               string
	ApprovedOrigins   []string
	NavigationTimeout time.Duration
	TotalTimeout      time.Duration
	MaxRequests       int
	MaxResponseBytes  int64
	MaxCandidates     int
}

// RawObservation is an in-process backend record. Serve reduces it before any
// protocol output is written.
type RawObservation struct {
	Origin      string
	Path        string
	Context     string
	Candidates  []RawCandidate
	Contexts    map[string]authorresult.Context
	Diagnostics []string
}

// RawCandidate identifies one backend-owned accessible target. BackendID is
// never serialized.
type RawCandidate struct {
	BackendID string
	Role      string
	Label     string
	InputKind string
	// TargetOrigin is backend-only preflight metadata for portable links. It is
	// never included in an observation; it can only surface as an origin gate.
	TargetOrigin string
	Matches      int
}

// BrowserAction is a closed action against a backend-owned candidate.
type BrowserAction struct {
	Kind       string
	BackendID  string
	URL        string
	Context    string
	POSTBudget int
}

// Execution contains only value-free action facts.
type Execution struct {
	POSTObserved int
	OpenedID     string
	Opened       *authorresult.Context
}

// ClientMessage is the closed NDJSON input union.
type ClientMessage struct {
	Protocol      string                      `json:"protocol"`
	Type          string                      `json:"type"`
	Title         string                      `json:"title,omitempty"`
	URL           string                      `json:"url,omitempty"`
	DashboardURL  string                      `json:"dashboardUrl,omitempty"`
	Goal          string                      `json:"goal,omitempty"`
	Origins       []string                    `json:"origins,omitempty"`
	GoalPredicate *authorresult.GoalPredicate `json:"goalPredicate,omitempty"`
	Bounds        *authorresult.Bounds        `json:"bounds,omitempty"`
	Context       string                      `json:"context,omitempty"`
	CandidateID   string                      `json:"candidateId,omitempty"`
	Action        string                      `json:"action,omitempty"`
	POSTBudget    int                         `json:"postBudget,omitempty"`
	ApprovalID    string                      `json:"approvalId,omitempty"`
	Confirmed     bool                        `json:"confirmed,omitempty"`
}

// Candidate is the reduced protocol candidate.
type Candidate struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Label   string `json:"label,omitempty"`
	Matches int    `json:"matches"`
}

// Observation is the only page-derived semantic protocol payload.
type Observation struct {
	Origin      string      `json:"origin"`
	Path        string      `json:"path"`
	Context     string      `json:"context"`
	Candidates  []Candidate `json:"candidates"`
	Diagnostics []string    `json:"diagnostics"`
}

// ServerMessage is the closed NDJSON output union.
type ServerMessage struct {
	Protocol     string               `json:"protocol"`
	Type         string               `json:"type"`
	Capabilities []string             `json:"capabilities,omitempty"`
	Bounds       *authorresult.Bounds `json:"bounds,omitempty"`
	Phase        string               `json:"phase,omitempty"`
	Context      string               `json:"context,omitempty"`
	Observation  *Observation         `json:"observation,omitempty"`
	Approval     *Approval            `json:"approval,omitempty"`
	Checkpoint   *Checkpoint          `json:"checkpoint,omitempty"`
	Diagnostic   *Diagnostic          `json:"diagnostic,omitempty"`
	Result       *Result              `json:"result,omitempty"`
}

type Approval struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Origin      string `json:"origin,omitempty"`
	Action      string `json:"action,omitempty"`
	CandidateID string `json:"candidateId,omitempty"`
	POSTBudget  int    `json:"postBudget,omitempty"`
}

type Checkpoint struct {
	Kind        string `json:"kind"`
	CandidateID string `json:"candidateId,omitempty"`
}

type Diagnostic struct {
	Code string `json:"code"`
}

type Result struct {
	ArtifactPath string `json:"artifactPath"`
	Digest       string `json:"digest"`
}

// ServeOptions are local process controls, not protocol authority.
type ServeOptions struct {
	PrivateRoot string
	Clock       func() time.Time
}

type candidateRecord struct {
	protocol Candidate
	raw      RawCandidate
	context  string
	label    string
}

type pendingApproval struct {
	id      string
	kind    string
	message ClientMessage
	origin  string
}

type server struct {
	ctx             context.Context
	cancel          context.CancelFunc
	browser         Browser
	session         Session
	output          io.Writer
	options         ServeOptions
	started         bool
	closed          bool
	phase           string
	start           ClientMessage
	bounds          authorresult.Bounds
	origins         map[string]struct{}
	contexts        map[string]authorresult.Context
	candidates      map[string]candidateRecord
	trace           []authorresult.TraceStep
	diagnostics     []string
	observations    int
	approvalCounter int
	pending         *pendingApproval
	goalProof       *authorresult.GoalProof
	humanConfirmed  bool
	observedAt      time.Time
}

// Serve processes one author session until finish, close, or fail-closed
// termination. It never persists the NDJSON transcript.
func Serve(ctx context.Context, input io.Reader, output io.Writer, browser Browser, options ServeOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || output == nil || browser == nil {
		return fmt.Errorf("author-session input, output, and browser are required")
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if err := validatePrivateRoot(options.PrivateRoot); err != nil {
		return err
	}
	s := &server{
		ctx: ctx, browser: browser, output: output, options: options,
		origins: make(map[string]struct{}), contexts: make(map[string]authorresult.Context),
		candidates: make(map[string]candidateRecord), phase: "awaiting_start",
	}
	if err := s.write(ServerMessage{
		Type: "hello", Capabilities: []string{"chromium", "human_credentials", "human_mfa", "reduced_observation", "popup", "frame", "typed_goal"},
	}); err != nil {
		return err
	}

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxProtocolLineBytes)
	for scanner.Scan() {
		message, err := decodeClientMessage(scanner.Bytes())
		if err != nil {
			return s.fail("malformed_message", err)
		}
		done, err := s.handle(message)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return s.fail("protocol_limit", err)
	}
	if s.closed {
		return nil
	}
	return s.fail("unexpected_eof", io.ErrUnexpectedEOF)
}

func (s *server) handle(message ClientMessage) (bool, error) {
	if s.closed {
		return true, fmt.Errorf("author session is closed")
	}
	if message.Protocol != Protocol {
		return false, s.fail("protocol_mismatch", fmt.Errorf("protocol mismatch"))
	}
	if s.pending != nil && message.Type != "approve" && message.Type != "deny" && message.Type != "close" {
		return false, s.fail("approval_pending", fmt.Errorf("approval response required"))
	}
	if !s.started && message.Type != "start" && message.Type != "close" {
		return false, s.fail("invalid_state", fmt.Errorf("start is required"))
	}
	switch message.Type {
	case "start":
		return false, s.startSession(message)
	case "observe":
		return false, s.observe(message.Context)
	case "focus_human_input":
		return false, s.focus(message.CandidateID)
	case "execute":
		return false, s.requestExecute(message)
	case "approve":
		return false, s.approve(message.ApprovalID)
	case "deny":
		return false, s.deny(message.ApprovalID)
	case "human_complete":
		return false, s.complete(message.Confirmed)
	case "finish":
		return true, s.finish()
	case "close":
		return true, s.closeWithoutResult()
	default:
		return false, s.fail("unknown_message", fmt.Errorf("unknown message type"))
	}
}

func (s *server) startSession(message ClientMessage) error {
	if s.started {
		return s.fail("invalid_state", fmt.Errorf("session already started"))
	}
	if err := validateStart(message); err != nil {
		return s.fail("invalid_start", err)
	}
	s.bounds = normalizedBounds(message.Bounds)
	s.observedAt = s.options.Clock().UTC().Round(0)
	if s.observedAt.IsZero() {
		return s.fail("clock_unavailable", fmt.Errorf("clock is unavailable"))
	}
	for _, origin := range message.Origins {
		canonical, err := exactOrigin(origin)
		if err != nil {
			return s.fail("invalid_origin", err)
		}
		s.origins[canonical] = struct{}{}
	}
	goalOrigin, err := exactOrigin(message.GoalPredicate.Origin)
	if err != nil {
		return s.fail("invalid_start", err)
	}
	message.GoalPredicate.Origin = goalOrigin
	initialOrigin, err := originForURL(message.URL)
	if err != nil {
		return s.fail("invalid_start", err)
	}
	if _, ok := s.origins[initialOrigin]; !ok {
		return s.fail("invalid_origin", fmt.Errorf("initial origin is not approved"))
	}
	if _, ok := s.origins[goalOrigin]; !ok {
		return s.fail("invalid_origin", fmt.Errorf("goal origin is not approved"))
	}
	dashboardOrigin, err := originForURL(message.DashboardURL)
	if err != nil {
		return s.fail("invalid_start", err)
	}
	if _, ok := s.origins[dashboardOrigin]; !ok {
		return s.fail("invalid_origin", fmt.Errorf("dashboard origin is not approved"))
	}
	total := time.Duration(s.bounds.TotalTimeoutMS) * time.Millisecond
	s.ctx, s.cancel = context.WithTimeout(s.ctx, total)
	session, err := s.browser.Open(s.ctx, BrowserRequest{
		URL: message.URL, ApprovedOrigins: sortedKeys(s.origins),
		NavigationTimeout: time.Duration(s.bounds.NavigationTimeoutMS) * time.Millisecond,
		TotalTimeout:      total, MaxRequests: s.bounds.MaxRequests,
		MaxResponseBytes: s.bounds.MaxResponseBytes, MaxCandidates: s.bounds.MaxCandidates,
	})
	if err != nil {
		return s.fail("browser_failure", err)
	}
	s.session, s.started, s.phase, s.start = session, true, "authentication", message
	return s.write(ServerMessage{Type: "state", Phase: s.phase, Context: "main", Bounds: &s.bounds})
}

func (s *server) observe(contextID string) error {
	if contextID == "" {
		contextID = "main"
	}
	if contextID != "main" {
		if _, ok := s.contexts[contextID]; !ok {
			return s.fail("unknown_context", fmt.Errorf("unknown context"))
		}
	}
	s.observations++
	if s.observations > s.bounds.MaxObservations {
		return s.fail("observation_limit", fmt.Errorf("observation limit exceeded"))
	}
	raw, err := s.session.Observe(s.ctx, contextID)
	if err != nil {
		return s.fail("browser_failure", err)
	}
	observation, records, err := s.reduceObservation(raw, contextID)
	if err != nil {
		return s.fail("invalid_observation", err)
	}
	for id, record := range records {
		s.candidates[id] = record
	}
	for id, context := range raw.Contexts {
		if err := s.addContext(id, context); err != nil {
			return s.fail("invalid_context", err)
		}
	}
	if dashboardMatches(s.start.DashboardURL, observation.Origin, observation.Path) {
		s.phase = "exploration"
	}
	if proof := matchGoal(*s.start.GoalPredicate, observation); proof != nil {
		s.goalProof = proof
	}
	if err := s.write(ServerMessage{Type: "observation", Observation: &observation}); err != nil {
		return err
	}
	if s.goalProof != nil {
		return s.write(ServerMessage{Type: "human_checkpoint", Checkpoint: &Checkpoint{Kind: "completion"}})
	}
	return nil
}

func (s *server) focus(candidateID string) error {
	record, ok := s.candidates[candidateID]
	if !ok || record.raw.InputKind == "" || record.protocol.Matches != 1 {
		return s.fail("invalid_candidate", fmt.Errorf("candidate cannot receive human input"))
	}
	if record.raw.InputKind != "mfa" {
		if err := s.session.Focus(s.ctx, record.raw.BackendID); err != nil {
			return s.fail("browser_failure", err)
		}
	}
	s.trace = append(s.trace, authorresult.TraceStep{
		Kind: "focus_human_input", Phase: s.phase, CandidateID: candidateID,
		Context: record.context, Role: record.protocol.Role, Label: record.label,
		InputKind: record.raw.InputKind,
	})
	checkpoint := record.raw.InputKind
	if checkpoint == "identifier" || checkpoint == "password" {
		checkpoint = "credential"
	} else {
		checkpoint = "mfa"
	}
	return s.write(ServerMessage{Type: "human_checkpoint", Checkpoint: &Checkpoint{Kind: checkpoint, CandidateID: candidateID}})
}

func (s *server) requestExecute(message ClientMessage) error {
	switch message.Action {
	case "navigate_get":
		origin, err := originForURL(message.URL)
		if err != nil {
			return s.fail("invalid_action", err)
		}
		if _, ok := s.origins[origin]; !ok {
			return s.requireApproval("origin", message, origin)
		}
		return s.execute(message)
	case "click":
		record, ok := s.candidates[message.CandidateID]
		if !ok || record.protocol.Matches != 1 {
			return s.fail("ambiguous_target", fmt.Errorf("click candidate is missing or ambiguous"))
		}
		if message.POSTBudget < 0 || message.POSTBudget > 32 {
			return s.fail("invalid_action", fmt.Errorf("POST budget is invalid"))
		}
		if record.raw.TargetOrigin != "" {
			origin, err := exactOrigin(record.raw.TargetOrigin)
			if err != nil {
				return s.fail("invalid_action", err)
			}
			if _, ok := s.origins[origin]; !ok {
				return s.requireApproval("origin_action", message, origin)
			}
		}
		return s.requireApproval("action", message, "")
	default:
		return s.fail("invalid_action", fmt.Errorf("unsupported action"))
	}
}

func (s *server) requireApproval(kind string, message ClientMessage, origin string) error {
	s.approvalCounter++
	id := fmt.Sprintf("approval-%04d", s.approvalCounter)
	s.pending = &pendingApproval{id: id, kind: kind, message: message, origin: origin}
	return s.write(ServerMessage{Type: "approval_required", Approval: &Approval{
		ID: id, Kind: kind, Origin: origin, Action: message.Action,
		CandidateID: message.CandidateID, POSTBudget: message.POSTBudget,
	}})
}

func (s *server) approve(id string) error {
	if s.pending == nil || id == "" || id != s.pending.id {
		return s.fail("approval_mismatch", fmt.Errorf("approval ID does not match"))
	}
	pending := s.pending
	s.pending = nil
	if pending.kind == "origin" {
		if err := s.session.AddOrigin(pending.origin); err != nil {
			return s.fail("browser_failure", err)
		}
		s.origins[pending.origin] = struct{}{}
	}
	if pending.kind == "origin_action" {
		if err := s.session.AddOrigin(pending.origin); err != nil {
			return s.fail("browser_failure", err)
		}
		s.origins[pending.origin] = struct{}{}
		return s.requestExecute(pending.message)
	}
	return s.execute(pending.message)
}

func (s *server) deny(id string) error {
	if s.pending == nil || id == "" || id != s.pending.id {
		return s.fail("approval_mismatch", fmt.Errorf("approval ID does not match"))
	}
	s.pending = nil
	return s.fail("approval_denied", fmt.Errorf("required approval was denied"))
}

func (s *server) execute(message ClientMessage) error {
	action := BrowserAction{Kind: message.Action, URL: message.URL, Context: message.Context, POSTBudget: message.POSTBudget}
	var record candidateRecord
	if message.Action == "click" {
		record = s.candidates[message.CandidateID]
		action.BackendID, action.Context = record.raw.BackendID, record.context
	}
	execution, err := s.session.Execute(s.ctx, action)
	if err != nil {
		return s.fail("browser_failure", err)
	}
	if execution.POSTObserved < 0 || execution.POSTObserved > message.POSTBudget {
		return s.fail("post_budget", fmt.Errorf("POST budget exceeded"))
	}
	trace := authorresult.TraceStep{
		Kind: strings.TrimSuffix(message.Action, "_get"), Phase: s.phase,
		CandidateID: message.CandidateID, Context: action.Context, URL: cleanProtocolURL(message.URL),
		POSTBudget: message.POSTBudget, POSTObserved: execution.POSTObserved,
	}
	if message.Action == "click" {
		trace.Role, trace.Label = record.protocol.Role, record.label
	}
	if execution.Opened != nil {
		if err := s.addContext(execution.OpenedID, *execution.Opened); err != nil {
			return s.fail("invalid_context", err)
		}
		trace.OpensContext = execution.OpenedID
	}
	s.trace = append(s.trace, trace)
	return s.write(ServerMessage{Type: "state", Phase: s.phase, Context: action.Context})
}

func (s *server) complete(confirmed bool) error {
	if !confirmed || s.goalProof == nil {
		return s.fail("completion_denied", fmt.Errorf("typed goal and human confirmation are required"))
	}
	s.humanConfirmed, s.phase = true, "completed"
	return s.write(ServerMessage{Type: "state", Phase: s.phase, Context: s.goalProof.Context})
}

func (s *server) finish() error {
	if !s.humanConfirmed || s.goalProof == nil {
		return s.fail("invalid_state", fmt.Errorf("goal completion is not confirmed"))
	}
	if err := s.closeSession(); err != nil {
		return s.failAfterClose("teardown_failure", err)
	}
	envelope, err := authorresult.Build(authorresult.BuildRequest{
		ObservedAt: s.observedAt, Title: defaultTitle(s.start.Title), Goal: s.start.Goal,
		InitialURL: s.start.URL, DashboardURL: s.start.DashboardURL,
		Origins: sortedKeys(s.origins), Contexts: s.contexts, Bounds: s.bounds,
		Trace: s.trace, GoalPredicate: *s.start.GoalPredicate, GoalProof: *s.goalProof,
		HumanConfirmed: true, Diagnostics: s.diagnostics,
	})
	if err != nil {
		return s.failAfterClose("result_invalid", err)
	}
	data, err := authorresult.MarshalDeterministic(envelope)
	if err != nil {
		return s.failAfterClose("result_invalid", err)
	}
	digest := dataDigest(data)
	name := "authenticated-authoring-" + strings.TrimPrefix(digest, "sha256:")[:16] + ".json"
	path := filepath.Join(s.options.PrivateRoot, name)
	if err := writePrivateExclusive(path, data); err != nil {
		return s.failAfterClose("artifact_write", err)
	}
	s.closed = true
	return s.write(ServerMessage{Type: "result", Result: &Result{ArtifactPath: path, Digest: digest}})
}

func (s *server) closeWithoutResult() error {
	err := s.closeSession()
	s.closed = true
	if err != nil {
		return s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: "teardown_failure"}})
	}
	return s.write(ServerMessage{Type: "state", Phase: "closed"})
}

func (s *server) closeSession() error {
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.session == nil {
		return nil
	}
	err := s.session.Close()
	s.session = nil
	return err
}

func (s *server) fail(code string, cause error) error {
	closeErr := s.closeSession()
	s.closed = true
	writeErr := s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: code}})
	return errors.Join(cause, closeErr, writeErr)
}

func (s *server) failAfterClose(code string, cause error) error {
	s.closed = true
	writeErr := s.write(ServerMessage{Type: "diagnostic", Diagnostic: &Diagnostic{Code: code}})
	return errors.Join(cause, writeErr)
}

func (s *server) reduceObservation(raw RawObservation, requestedContext string) (Observation, map[string]candidateRecord, error) {
	origin, err := exactOrigin(raw.Origin)
	if err != nil {
		return Observation{}, nil, err
	}
	if _, ok := s.origins[origin]; !ok {
		return Observation{}, nil, fmt.Errorf("observation escaped approved origins")
	}
	if !cleanPath(raw.Path) {
		return Observation{}, nil, fmt.Errorf("observation path is not clean")
	}
	contextID := raw.Context
	if contextID == "" {
		contextID = requestedContext
	}
	if contextID != requestedContext {
		return Observation{}, nil, fmt.Errorf("observation context mismatch")
	}
	if len(raw.Candidates) > s.bounds.MaxCandidates {
		return Observation{}, nil, fmt.Errorf("candidate limit exceeded")
	}
	sort.Slice(raw.Candidates, func(i, j int) bool {
		left, right := raw.Candidates[i], raw.Candidates[j]
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.BackendID < right.BackendID
	})
	result := Observation{Origin: origin, Path: raw.Path, Context: contextID, Candidates: []Candidate{}, Diagnostics: []string{}}
	records := make(map[string]candidateRecord, len(raw.Candidates))
	for index, rawCandidate := range raw.Candidates {
		if rawCandidate.BackendID == "" || !portableRoles[rawCandidate.Role] || rawCandidate.Matches < 1 || rawCandidate.Matches > s.bounds.MaxCandidates {
			return Observation{}, nil, fmt.Errorf("backend candidate is invalid")
		}
		if rawCandidate.InputKind != "" && rawCandidate.InputKind != "identifier" && rawCandidate.InputKind != "password" && rawCandidate.InputKind != "otp" && rawCandidate.InputKind != "mfa" {
			return Observation{}, nil, fmt.Errorf("backend input kind is invalid")
		}
		label, _ := reduceLabel(rawCandidate.Label)
		id := candidateID(contextID, rawCandidate.Role, label, index)
		candidate := Candidate{ID: id, Role: rawCandidate.Role, Label: label, Matches: rawCandidate.Matches}
		result.Candidates = append(result.Candidates, candidate)
		records[id] = candidateRecord{protocol: candidate, raw: rawCandidate, context: contextID, label: label}
	}
	for _, code := range raw.Diagnostics {
		if !diagnosticPattern.MatchString(code) {
			return Observation{}, nil, fmt.Errorf("backend diagnostic is invalid")
		}
		result.Diagnostics = append(result.Diagnostics, code)
		s.diagnostics = append(s.diagnostics, code)
	}
	sort.Strings(result.Diagnostics)
	return result, records, nil
}

func (s *server) addContext(id string, context authorresult.Context) error {
	if !identifierPattern.MatchString(id) || id == "main" || (context.Kind != "popup" && context.Kind != "frame") {
		return fmt.Errorf("context identity is invalid")
	}
	canonicalOrigin, err := exactOrigin(context.Origin)
	if err != nil {
		return err
	}
	context.Origin = canonicalOrigin
	if _, ok := s.origins[canonicalOrigin]; !ok {
		return fmt.Errorf("context origin is not approved")
	}
	if context.Parent != "main" {
		if _, ok := s.contexts[context.Parent]; !ok {
			return fmt.Errorf("context parent is unknown")
		}
	}
	if context.Kind == "popup" && (context.Path != "" || context.Name != "") {
		return fmt.Errorf("popup context carries frame metadata")
	}
	if context.Kind == "frame" && context.Path == "" && context.Name == "" {
		return fmt.Errorf("frame context lacks exact identity")
	}
	if context.Path != "" && !cleanPath(context.Path) {
		return fmt.Errorf("context path is unsafe")
	}
	if existing, ok := s.contexts[id]; ok && existing != context {
		return fmt.Errorf("context identity changed")
	}
	depth := 1
	for parent := context.Parent; parent != "main"; depth++ {
		if depth >= 4 {
			return fmt.Errorf("context depth exceeds four")
		}
		ancestor, ok := s.contexts[parent]
		if !ok || parent == id {
			return fmt.Errorf("context graph is invalid")
		}
		parent = ancestor.Parent
	}
	s.contexts[id] = context
	return nil
}

func (s *server) write(message ServerMessage) error {
	message.Protocol = Protocol
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.output.Write(data)
	return err
}

func decodeClientMessage(line []byte) (ClientMessage, error) {
	line = []byte(strings.TrimSpace(string(line)))
	if len(line) == 0 {
		return ClientMessage{}, fmt.Errorf("empty protocol message")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return ClientMessage{}, err
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &header); err != nil {
		return ClientMessage{}, err
	}
	allowed, ok := clientFields[header.Type]
	if !ok {
		allowed = clientFields["unknown"]
	}
	for field := range fields {
		if !allowed[field] {
			return ClientMessage{}, fmt.Errorf("field %q is not allowed for %q", field, header.Type)
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(line)))
	decoder.DisallowUnknownFields()
	var message ClientMessage
	if err := decoder.Decode(&message); err != nil {
		return ClientMessage{}, err
	}
	return message, nil
}

var clientFields = map[string]map[string]bool{
	"start":             fields("protocol", "type", "title", "url", "dashboardUrl", "goal", "origins", "goalPredicate", "bounds"),
	"observe":           fields("protocol", "type", "context"),
	"focus_human_input": fields("protocol", "type", "candidateId"),
	"execute":           fields("protocol", "type", "action", "candidateId", "url", "context", "postBudget"),
	"approve":           fields("protocol", "type", "approvalId"),
	"deny":              fields("protocol", "type", "approvalId"),
	"human_complete":    fields("protocol", "type", "confirmed"),
	"finish":            fields("protocol", "type"),
	"close":             fields("protocol", "type"),
	"unknown":           fields("protocol", "type"),
}

func fields(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

func validateStart(message ClientMessage) error {
	if message.GoalPredicate == nil || strings.TrimSpace(message.Title) == "" || len(message.Title) > 256 || strings.TrimSpace(message.Goal) == "" || len(message.Goal) > 1024 {
		return fmt.Errorf("title, goal, and typed predicate are required")
	}
	if len(message.Origins) == 0 || len(message.Origins) > 32 {
		return fmt.Errorf("one to 32 approved origins are required")
	}
	if _, err := cleanAbsoluteURL(message.URL); err != nil {
		return err
	}
	if message.DashboardURL == "" {
		return fmt.Errorf("dashboard URL is required")
	}
	if _, err := cleanAbsoluteURL(message.DashboardURL); err != nil {
		return err
	}
	if _, err := exactOrigin(message.GoalPredicate.Origin); err != nil {
		return err
	}
	if !cleanPath(message.GoalPredicate.Path) || !portableRoles[message.GoalPredicate.Role] || len(message.GoalPredicate.Label) > 256 {
		return fmt.Errorf("goal predicate is invalid")
	}
	if err := validateBounds(message.Bounds); err != nil {
		return err
	}
	return nil
}

func validateBounds(value *authorresult.Bounds) error {
	if value == nil {
		return nil
	}
	if value.NavigationTimeoutMS <= 0 || value.NavigationTimeoutMS > time.Minute.Milliseconds() ||
		value.TotalTimeoutMS < value.NavigationTimeoutMS || value.TotalTimeoutMS > (30*time.Minute).Milliseconds() ||
		value.MaxRequests <= 0 || value.MaxRequests > 4096 ||
		value.MaxResponseBytes <= 0 || value.MaxResponseBytes > 128<<20 ||
		value.MaxObservations <= 0 || value.MaxObservations > 256 ||
		value.MaxCandidates <= 0 || value.MaxCandidates > 512 {
		return fmt.Errorf("author-session bounds are invalid")
	}
	return nil
}

func normalizedBounds(value *authorresult.Bounds) authorresult.Bounds {
	result := authorresult.Bounds{
		NavigationTimeoutMS: DefaultNavigationTimeout.Milliseconds(), TotalTimeoutMS: DefaultTotalTimeout.Milliseconds(),
		MaxRequests: DefaultMaxRequests, MaxResponseBytes: DefaultMaxResponseBytes,
		MaxObservations: DefaultMaxObservations, MaxCandidates: DefaultMaxCandidates,
	}
	if value != nil {
		result = *value
	}
	return result
}

func matchGoal(predicate authorresult.GoalPredicate, observation Observation) *authorresult.GoalProof {
	contextID := predicate.Context
	if contextID == "" {
		contextID = "main"
	}
	if predicate.Origin != observation.Origin || predicate.Path != observation.Path || contextID != observation.Context {
		return nil
	}
	matches := []Candidate{}
	for _, candidate := range observation.Candidates {
		if candidate.Role == predicate.Role && (predicate.Label == "" || candidate.Label == predicate.Label) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 || matches[0].Matches != 1 {
		return nil
	}
	return &authorresult.GoalProof{Origin: observation.Origin, Path: observation.Path, Context: observation.Context, Role: matches[0].Role, Label: matches[0].Label, Matches: 1}
}

func reduceLabel(raw string) (string, bool) {
	raw = strings.Join(strings.FieldsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if raw == "" {
		return "", true
	}
	if len(raw) > 256 {
		return "[redacted]", false
	}
	lower := strings.ToLower(raw)
	for _, phrase := range []string{"ignore previous", "ignore all instructions", "system prompt", "developer message", "tool call", "reveal secrets"} {
		if strings.Contains(lower, phrase) {
			return "[untrusted-label]", false
		}
	}
	if emailPattern.MatchString(raw) || phonePattern.MatchString(raw) || secretPattern.MatchString(raw) {
		return "[redacted]", false
	}
	return raw, true
}

func candidateID(contextID, role, label string, ordinal int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", contextID, role, label, ordinal)))
	return "candidate-" + hex.EncodeToString(sum[:8])
}

func validatePrivateRoot(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("private root is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private root must be an existing non-symlink private directory")
	}
	return nil
}

func writePrivateExclusive(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".browsertools-author-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Link(temporaryPath, path); errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite private result")
	} else if err != nil {
		return err
	}
	return nil
}

func exactOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("origin must be exact")
	}
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", fmt.Errorf("origin must use HTTPS or loopback HTTP")
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return strings.ToLower(parsed.Scheme) + "://" + host, nil
}

func originForURL(raw string) (string, error) {
	parsed, err := cleanAbsoluteURL(raw)
	if err != nil {
		return "", err
	}
	return exactOrigin(parsed.Scheme + "://" + parsed.Host)
}

func cleanAbsoluteURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("URL must be absolute and fragment-free")
	}
	if _, err := exactOrigin(parsed.Scheme + "://" + parsed.Host); err != nil {
		return nil, err
	}
	if !cleanPath(parsed.EscapedPath()) {
		return nil, fmt.Errorf("URL path must be clean")
	}
	return parsed, nil
}

func cleanPath(raw string) bool {
	if raw == "" {
		raw = "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.ContainsAny(raw, "?#\\") {
		return false
	}
	parts := strings.Split(raw, "/")
	for i, part := range parts[1:] {
		if part == "" && i != len(parts)-2 && raw != "/" {
			return false
		}
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "." || decoded == ".." || strings.Contains(decoded, "/") {
			return false
		}
	}
	return true
}

func cleanProtocolURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String()
}

func dashboardMatches(rawURL, origin, path string) bool {
	parsed, err := cleanAbsoluteURL(rawURL)
	if err != nil {
		return false
	}
	wantOrigin, err := exactOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return false
	}
	wantPath := parsed.EscapedPath()
	if wantPath == "" {
		wantPath = "/"
	}
	return wantOrigin == origin && wantPath == path
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func defaultTitle(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Authenticated browser"
	}
	return value
}

func dataDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	diagnosticPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	emailPattern      = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern      = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
	secretPattern     = regexp.MustCompile(`(?i)(?:bearer\s+[a-z0-9._-]{12,}|(?:token|secret|password)\s*[:=]\s*\S+|sk-[a-z0-9]{12,})`)
	portableRoles     = map[string]bool{
		"button": true, "link": true, "textbox": true, "checkbox": true, "radio": true,
		"dialog": true, "status": true, "alert": true, "heading": true, "img": true,
		"list": true, "listitem": true, "combobox": true, "option": true, "menu": true,
		"menuitem": true, "tab": true, "tabpanel": true, "table": true, "row": true,
		"cell": true, "region": true, "navigation": true, "article": true, "form": true,
		"search": true, "switch": true, "group": true,
	}
)
