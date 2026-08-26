// Package registrationauthor builds one deterministic no-submit registration
// review candidate from an explicit UWS draft and a current reduced
// observation. It never infers a locator, credential slot, flow step, success
// condition, or call-safety decision.
package registrationauthor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationdraft"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
)

const (
	ApprovalSymbol      = "registration_approval"
	DuplicatePrevention = "operator_attestation"
	OnDuplicate         = "fail"
	AmbiguousOutcome    = "stop_without_retry"
	CleanupDelete       = "delete_separately"
	CleanupRetain       = "retain_dedicated_test_identity"
)

// CallControls are explicit value-free operator decisions. The first four
// values are fixed by the M26 result contract but still have to be supplied so
// a caller cannot accidentally treat defaults as a reviewed choice.
type CallControls struct {
	ApprovalSymbol      string
	DuplicatePrevention string
	OnDuplicate         string
	AmbiguousOutcome    string
	CleanupDisposition  string
}

// BuildRequest binds one complete explicit draft to one current reduced
// observation and exact operator selections.
type BuildRequest struct {
	Protocol             string
	ProfileID            string
	Spec                 registrationdraft.Spec
	Observation          registrationauthorsession.Observation
	ApprovedOrigins      []string
	ReviewedCandidateIDs []string
	SubmitCandidateID    string
	Flow                 string
	Controls             CallControls
	AssessedAt           time.Time
}

// Candidate is an immutable-by-API pre-review value. Accessors return copies;
// the private M26 result is built only after the session independently accepts
// the ReviewMessage and closes cleanly.
type Candidate struct {
	profileID       string
	profileBytes    []byte
	observation     registrationauthorsession.Observation
	reviewedIDs     []string
	submitID        string
	flow            string
	controls        CallControls
	approvedOrigins []string
	protocol        string
}

// Build validates every explicit decision and constructs canonical UWS source.
func Build(request BuildRequest) (*Candidate, error) {
	if request.Protocol == "" {
		request.Protocol = registrationauthorsession.ProtocolV1
	}
	if request.Protocol != registrationauthorsession.ProtocolV1 && request.Protocol != registrationauthorsession.ProtocolV2 {
		return nil, errors.New("registration author protocol is unsupported")
	}
	if !identifierPattern.MatchString(request.ProfileID) || !identifierPattern.MatchString(request.Flow) {
		return nil, errors.New("registration author identity is invalid")
	}
	if request.AssessedAt.IsZero() || request.AssessedAt.Location() != time.UTC || request.AssessedAt.Nanosecond() != 0 {
		return nil, errors.New("registration author assessment time must be whole-second UTC")
	}
	if err := validateControls(request.Controls); err != nil {
		return nil, err
	}
	origins, err := validateOrigins(request.ApprovedOrigins)
	if err != nil {
		return nil, err
	}
	observation, inventory, err := validateObservation(request.Observation, origins)
	if err != nil {
		return nil, err
	}
	reviewedIDs, err := validateReviewedIDs(request.ReviewedCandidateIDs, inventory)
	if err != nil {
		return nil, err
	}
	if !contains(reviewedIDs, request.SubmitCandidateID) {
		return nil, errors.New("registration submit candidate was not explicitly reviewed")
	}
	submitCandidate := inventory[request.SubmitCandidateID]
	profileValue, err := registrationdraft.Build(request.Spec)
	if err != nil {
		return nil, errors.New("explicit registration draft is invalid")
	}
	if profileValue.ObservationKind != "accessibility_snapshot" ||
		profileValue.Evidence.LearnedAt != request.AssessedAt.Format(time.RFC3339) ||
		profileValue.Verification.LastVerifiedAt != request.AssessedAt.Format(time.RFC3339) {
		return nil, errors.New("registration draft is not bound to the explicit assessment")
	}
	if err := registrationprofile.ValidateAt(profileValue, request.AssessedAt); err != nil {
		return nil, errors.New("explicit registration draft is not current")
	}
	if request.Protocol == registrationauthorsession.ProtocolV2 && registrationprofile.ValidateRetainedNavigationV2(profileValue) != nil {
		return nil, errors.New("explicit registration draft contains an unsafe navigation URL")
	}
	if !equalStrings(registrationprofile.Origins(profileValue), origins) {
		return nil, errors.New("registration draft origins do not match approved origins")
	}
	flow, ok := profileValue.Flows[request.Flow]
	if !ok {
		return nil, errors.New("explicit registration flow is missing")
	}
	if err := bindSubmit(flow.Sequence, submitCandidate); err != nil {
		return nil, err
	}
	profileBytes, err := registrationprofile.MarshalJSON(profileValue)
	if err != nil {
		return nil, errors.New("canonical registration source is invalid")
	}
	return &Candidate{
		profileID: request.ProfileID, profileBytes: append([]byte(nil), profileBytes...),
		observation: observation,
		reviewedIDs: append([]string(nil), reviewedIDs...), submitID: request.SubmitCandidateID,
		flow: request.Flow, controls: request.Controls,
		approvedOrigins: append([]string(nil), origins...),
		protocol:        request.Protocol,
	}, nil
}

// Profile returns an independent reconstruction of the canonical profile.
func (c *Candidate) Profile() (registrationprofile.Profile, error) {
	if c == nil {
		return registrationprofile.Profile{}, errors.New("registration author candidate is required")
	}
	value, err := registrationprofile.Parse(c.profileBytes)
	if err != nil {
		return registrationprofile.Profile{}, err
	}
	return *value, nil
}

// ProfileBytes returns the exact canonical source selected for review.
func (c *Candidate) ProfileBytes() []byte {
	if c == nil {
		return nil
	}
	return append([]byte(nil), c.profileBytes...)
}

// ProfileID returns the explicit local identity used for the session start.
func (c *Candidate) ProfileID() string {
	if c == nil {
		return ""
	}
	return c.profileID
}

// Flow returns the explicit selected registration alternative.
func (c *Candidate) Flow() string {
	if c == nil {
		return ""
	}
	return c.flow
}

// ReviewMessage returns the exact M26 review transition for this candidate.
func (c *Candidate) ReviewMessage() registrationauthorsession.ClientMessage {
	if c == nil {
		return registrationauthorsession.ClientMessage{}
	}
	return registrationauthorsession.ClientMessage{
		Protocol: c.protocol, Type: "review",
		Profile:      append(json.RawMessage(nil), c.profileBytes...),
		CandidateIDs: append([]string(nil), c.reviewedIDs...), Flow: c.flow,
		CleanupDisposition: c.controls.CleanupDisposition,
	}
}

// Observation returns a deep copy of the exact reduced generation used.
func (c *Candidate) Observation() registrationauthorsession.Observation {
	if c == nil {
		return registrationauthorsession.Observation{}
	}
	return cloneObservation(c.observation)
}

// Controls returns the exact explicit call-safety decisions.
func (c *Candidate) Controls() CallControls {
	if c == nil {
		return CallControls{}
	}
	return c.controls
}

// SubmitCandidateID returns the exact inert submit candidate selection.
func (c *Candidate) SubmitCandidateID() string {
	if c == nil {
		return ""
	}
	return c.submitID
}

// ApprovedOrigins returns the canonical complete origin inventory.
func (c *Candidate) ApprovedOrigins() []string {
	if c == nil {
		return []string{}
	}
	return append([]string(nil), c.approvedOrigins...)
}

func validateControls(value CallControls) error {
	if value.ApprovalSymbol != ApprovalSymbol || value.DuplicatePrevention != DuplicatePrevention ||
		value.OnDuplicate != OnDuplicate || value.AmbiguousOutcome != AmbiguousOutcome ||
		value.CleanupDisposition != CleanupDelete && value.CleanupDisposition != CleanupRetain {
		return errors.New("registration call controls were not explicitly fixed")
	}
	return nil
}

func validateOrigins(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 32 || !sort.StringsAreSorted(values) {
		return nil, errors.New("registration approved origins are invalid")
	}
	result := append([]string(nil), values...)
	for index, value := range result {
		if value == "" || index > 0 && result[index-1] == value {
			return nil, errors.New("registration approved origins are invalid")
		}
	}
	return result, nil
}

func validateObservation(value registrationauthorsession.Observation, origins []string) (registrationauthorsession.Observation, map[string]registrationauthorsession.Candidate, error) {
	if value.Generation <= 0 || value.Generation > 1<<30 || !contains(origins, value.Origin) ||
		disclosurepath.Validate(value.Path) != nil || len(value.Candidates) == 0 || len(value.Candidates) > 512 ||
		len(value.Diagnostics) > registrationauthorsession.MaxUniqueDiagnostics ||
		!sort.StringsAreSorted(value.Diagnostics) {
		return registrationauthorsession.Observation{}, nil, errors.New("registration observation is invalid")
	}
	for index, diagnostic := range value.Diagnostics {
		if !registrationauthorsession.ValidDiagnostic(diagnostic) || index > 0 && value.Diagnostics[index-1] == diagnostic {
			return registrationauthorsession.Observation{}, nil, errors.New("registration observation diagnostic is invalid")
		}
	}
	if !sort.SliceIsSorted(value.Candidates, func(i, j int) bool {
		left, right := value.Candidates[i], value.Candidates[j]
		if left.Role != right.Role {
			return left.Role < right.Role
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return left.ID < right.ID
	}) {
		return registrationauthorsession.Observation{}, nil, errors.New("registration observation candidates are not canonical")
	}
	inventory := make(map[string]registrationauthorsession.Candidate, len(value.Candidates))
	locators := make(map[string]struct{}, len(value.Candidates))
	for index, candidate := range value.Candidates {
		if !candidatePattern.MatchString(candidate.ID) || candidate.Matches < 1 || candidate.Matches > 512 ||
			!portableRoles[candidate.Role] || candidate.Label == "" || len(candidate.Label) > 256 || candidate.Label == authorsession.RedactedLabel ||
			candidate.Label == authorsession.UntrustedLabel || authorsession.ReduceAccessibilityLabel(candidate.Label).Value != candidate.Label ||
			candidate.ID != candidateID(value.Generation, candidate.Role, candidate.Label, index) {
			return registrationauthorsession.Observation{}, nil, errors.New("registration observation candidate is invalid")
		}
		if _, duplicate := inventory[candidate.ID]; duplicate {
			return registrationauthorsession.Observation{}, nil, errors.New("registration observation candidate is duplicated")
		}
		locator := candidate.Role + "\x00" + candidate.Label
		if _, duplicate := locators[locator]; duplicate {
			return registrationauthorsession.Observation{}, nil, errors.New("registration observation locator is duplicated")
		}
		inventory[candidate.ID], locators[locator] = candidate, struct{}{}
	}
	return cloneObservation(value), inventory, nil
}

func validateReviewedIDs(values []string, inventory map[string]registrationauthorsession.Candidate) ([]string, error) {
	if len(values) == 0 || len(values) > len(inventory) || !sort.StringsAreSorted(values) {
		return nil, errors.New("reviewed registration candidates are invalid")
	}
	result := append([]string(nil), values...)
	for index, id := range result {
		candidate, ok := inventory[id]
		if !ok || candidate.Matches != 1 || index > 0 && result[index-1] == id {
			return nil, errors.New("reviewed registration candidate is invalid")
		}
	}
	return result, nil
}

func bindSubmit(sequence []browserregistration.Step, candidate registrationauthorsession.Candidate) error {
	submitCount := 0
	for _, step := range sequence {
		if step.Submit == nil {
			continue
		}
		submitCount++
		locator := step.Submit.Locator
		if locator.Role == "" || locator.Name == "" || locator.Text != "" || locator.Value != "" ||
			locator.Role != candidate.Role || locator.Name != candidate.Label || candidate.Matches != 1 {
			return errors.New("explicit registration submit does not bind the reviewed candidate")
		}
	}
	if submitCount != 1 {
		return errors.New("explicit registration flow must contain one submit")
	}
	return nil
}

func cloneObservation(value registrationauthorsession.Observation) registrationauthorsession.Observation {
	value.Candidates = append([]registrationauthorsession.Candidate(nil), value.Candidates...)
	value.Diagnostics = append([]string(nil), value.Diagnostics...)
	return value
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

func candidateID(generation int, role, label string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%d", generation, role, label, index)))
	return "candidate-" + hex.EncodeToString(sum[:8])
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	candidatePattern  = regexp.MustCompile(`^candidate-[a-f0-9]{16}$`)
	portableRoles     = map[string]bool{
		"button": true, "link": true, "textbox": true, "checkbox": true, "radio": true,
		"dialog": true, "status": true, "alert": true, "heading": true, "img": true,
		"list": true, "listitem": true, "combobox": true, "option": true, "menu": true,
		"menuitem": true, "tab": true, "tabpanel": true, "table": true, "row": true,
		"cell": true, "region": true, "navigation": true, "article": true, "form": true,
		"search": true, "switch": true, "group": true,
	}
)
