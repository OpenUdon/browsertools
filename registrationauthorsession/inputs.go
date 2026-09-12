package registrationauthorsession

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
)

// ControlMetadata describes public form definitions, never current values.
// Required and constraints are suggestions until explicitly reviewed.
type ControlMetadata struct {
	Kind      string         `json:"kind"`
	Required  *bool          `json:"required,omitempty"`
	MinLength *int           `json:"minLength,omitempty"`
	MaxLength *int           `json:"maxLength,omitempty"`
	Minimum   *float64       `json:"minimum,omitempty"`
	Maximum   *float64       `json:"maximum,omitempty"`
	Options   []PublicOption `json:"options,omitempty"`
}

type PublicOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// PreviewRequest is an operator-reviewed public choice, not a runtime input.
// Purpose is an explicit declaration that this is not consent or verification.
type PreviewRequest struct {
	CandidateID string  `json:"candidateId"`
	Generation  int     `json:"generation"`
	Action      string  `json:"action"`
	Purpose     string  `json:"purpose"`
	Option      *string `json:"option,omitempty"`
	Checked     *bool   `json:"checked,omitempty"`
}

type PreviewRecord struct {
	Request        PreviewRequest `json:"request"`
	NextGeneration int            `json:"nextGeneration"`
}

// PreviewSession is implemented only by explicitly v3-aware backends. Legacy
// Session implementations cannot gain interaction authority through a cast.
type PreviewSession interface {
	Preview(context.Context, Candidate, PreviewRequest) error
}

func publicText(value string) bool {
	return value != "" && len(value) <= 256 && safeCandidateLabel(value) &&
		value != authorsession.RedactedLabel && value != authorsession.UntrustedLabel
}

func ValidateControlMetadata(value *ControlMetadata) error {
	bad := errors.New("invalid public registration control definition")
	if value == nil {
		return bad
	}
	switch value.Kind {
	case "text", "email", "password", "number", "checkbox", "switch", "select", "button", "link", "unsupported":
	default:
		return bad
	}
	if len(value.Options) > 32 || (value.Kind != "select" && len(value.Options) != 0) {
		return bad
	}
	seen := map[string]bool{}
	for _, option := range value.Options {
		if !publicText(option.Value) || !publicText(option.Label) || seen[option.Value] {
			return bad
		}
		seen[option.Value] = true
	}
	if value.MinLength != nil && (*value.MinLength < 0 || *value.MinLength > 4096) ||
		value.MaxLength != nil && (*value.MaxLength < 0 || *value.MaxLength > 4096) ||
		value.MinLength != nil && value.MaxLength != nil && *value.MinLength > *value.MaxLength {
		return bad
	}
	for _, number := range []*float64{value.Minimum, value.Maximum} {
		if number != nil && (math.IsNaN(*number) || math.IsInf(*number, 0) || math.Abs(*number) > 9007199254740991) {
			return bad
		}
	}
	if value.Minimum != nil && value.Maximum != nil && *value.Minimum > *value.Maximum {
		return bad
	}
	if value.Kind != "number" && (value.Minimum != nil || value.Maximum != nil) {
		return bad
	}
	if value.Kind != "text" && value.Kind != "email" && value.Kind != "password" && (value.MinLength != nil || value.MaxLength != nil) {
		return bad
	}
	return nil
}

// ValidatePreview rechecks the complete closed action against its exact current
// candidate. Labels remain untrusted; the purpose decision is operator-owned.
func ValidatePreview(candidate Candidate, request PreviewRequest) error {
	bad := errors.New("registration preview is not an approved public control")
	if request.Purpose != "public_form_preview" || request.CandidateID != candidate.ID || request.Generation <= 0 ||
		candidate.Matches != 1 || !promotableCandidate(candidate) || ValidateControlMetadata(candidate.Control) != nil {
		return bad
	}
	label := strings.ToLower(candidate.Label)
	if humanControlLabel(label) {
		return bad
	}
	for _, forbidden := range []string{"submit", "register", "sign up", "create account", "password", "email"} {
		if strings.Contains(label, forbidden) {
			return bad
		}
	}
	switch request.Action {
	case "select":
		if candidate.Control.Kind != "select" || request.Option == nil || request.Checked != nil {
			return bad
		}
		for _, option := range candidate.Control.Options {
			if option.Value == *request.Option {
				return nil
			}
		}
	case "check":
		if (candidate.Control.Kind == "checkbox" || candidate.Control.Kind == "switch") && request.Checked != nil && request.Option == nil {
			return nil
		}
	case "click":
		if (candidate.Control.Kind == "button" || candidate.Control.Kind == "link") && request.Option == nil && request.Checked == nil {
			return nil
		}
	}
	return bad
}

func humanControlLabel(label string) bool {
	label = strings.ToLower(label)
	for _, forbidden := range []string{"consent", "agree", "terms", "captcha", "verify", "verification", "accept", "one-time code", "otp", "security code"} {
		if strings.Contains(label, forbidden) {
			return true
		}
	}
	return false
}

func (s *server) preview(message ClientMessage) error {
	if s.protocol != ProtocolV3 || message.Preview == nil {
		return s.fail("invalid_state")
	}
	request := *message.Preview
	record, ok := s.candidates[request.CandidateID]
	backend, supported := s.session.(PreviewSession)
	if !ok || !supported || record.generation != request.Generation || ValidatePreview(record.protocol, request) != nil {
		return s.fail("invalid_candidate")
	}
	if err := s.withActiveContext(func(ctx context.Context) error { return backend.Preview(ctx, record.protocol, request) }); err != nil {
		return s.failBrowser()
	}
	clear(s.candidates)
	s.reviewedProfile = nil
	if err := s.observe(); err != nil {
		return err
	}
	s.previews = append(s.previews, PreviewRecord{Request: request, NextGeneration: s.generation})
	return nil
}

func historyCandidates(history []Observation) map[string]candidateRecord {
	result := map[string]candidateRecord{}
	for _, observation := range history {
		for _, candidate := range observation.Candidates {
			result[candidate.ID] = candidateRecord{protocol: candidate, generation: observation.Generation}
		}
	}
	return result
}

// ValidateV3Evidence independently binds each observed macro to the recorded
// page and public preview sequence. No success page is claimed observed.
func ValidateV3Evidence(profile *registrationprofile.Profile, flowName string, history []Observation, previews []PreviewRecord, steps, selected []string) error {
	bad := errors.New("registration recipe observation binding is invalid")
	if profile == nil || profile.Profile != browserregistration.ProfileNameV11 || len(history) == 0 || len(history) > 256 {
		return bad
	}
	flow, ok := profile.Flows[flowName]
	if !ok || len(steps) != len(flow.Sequence) || len(selected) == 0 || len(selected) > 512 || !sort.StringsAreSorted(selected) {
		return bad
	}
	encoded, err := json.Marshal(history)
	if err != nil || len(encoded) > MaxProtocolLineBytes {
		return bad
	}
	records := map[string]candidateRecord{}
	for index, observation := range history {
		if observation.Generation != index+1 || len(observation.Candidates) > 512 {
			return bad
		}
		if _, err := exactOrigin(observation.Origin); err != nil || !contains(registrationprofile.Origins(profile), observation.Origin) {
			return bad
		}
		if _, _, _, err := ValidateNavigationURL(ProtocolV3, observation.Origin+observation.Path); err != nil {
			return bad
		}
		for candidateIndex, candidate := range observation.Candidates {
			if candidate.ID != candidateID(observation.Generation, candidate.Role, candidate.Label, candidateIndex) || !portableRoles[candidate.Role] || !safeCandidateLabel(candidate.Label) || candidate.Matches < 1 || candidate.Matches > 512 {
				return bad
			}
			if candidate.Control != nil && ValidateControlMetadata(candidate.Control) != nil {
				return bad
			}
			if _, exists := records[candidate.ID]; exists {
				return bad
			}
			records[candidate.ID] = candidateRecord{protocol: candidate, generation: observation.Generation}
		}
	}
	lastPreview := 0
	for _, preview := range previews {
		record, ok := records[preview.Request.CandidateID]
		if !ok || record.generation != preview.Request.Generation || preview.NextGeneration != record.generation+1 || preview.NextGeneration > len(history) || preview.NextGeneration <= lastPreview || ValidatePreview(record.protocol, preview.Request) != nil {
			return bad
		}
		lastPreview = preview.NextGeneration
	}
	used := map[string]bool{}
	currentOrigin, currentPath := "", ""
	lastGeneration := 0
	submitted := false
	for index, step := range flow.Sequence {
		if step.Navigate != "" {
			parsed, err := url.Parse(step.Navigate)
			if err != nil {
				return bad
			}
			currentOrigin = parsed.Scheme + "://" + parsed.Host
			currentPath = parsed.EscapedPath()
			if currentPath == "" {
				currentPath = "/"
			}
			if steps[index] != "" {
				return bad
			}
			continue
		}
		var locator *browserregistration.Locator
		switch {
		case step.TypeCredential != nil:
			locator = &step.TypeCredential.Locator
		case step.FillInput != nil:
			locator = &step.FillInput.Locator
		case step.Click != nil:
			locator = &step.Click.Locator
		case step.Submit != nil:
			locator = &step.Submit.Locator
		case step.WaitFor != nil:
			locator = &step.WaitFor.Locator
		case step.HumanCheckpoint != nil:
			locator = step.HumanCheckpoint.Locator
		}
		if locator == nil {
			if steps[index] != "" {
				return bad
			}
			continue
		}
		if submitted && step.WaitFor != nil && steps[index] == "" && reflect.DeepEqual(*locator, profile.Flows[flowName].Success.Locator) {
			continue
		}
		record, ok := records[steps[index]]
		if !ok || record.generation < lastGeneration || record.protocol.Matches != 1 || !promotableCandidate(record.protocol) || locator.Role != record.protocol.Role || locator.Name != record.protocol.Label || locator.Text != "" || locator.Value != "" {
			return bad
		}
		observation := history[record.generation-1]
		if observation.Origin != currentOrigin || observation.Path != currentPath {
			return bad
		}
		lastGeneration = record.generation
		used[steps[index]] = true
		if step.TypeCredential != nil {
			control := record.protocol.Control
			kind := profile.CredentialSlots[step.TypeCredential.Slot].Kind
			if control == nil || (kind == "password" && control.Kind != "password") ||
				(kind == "identifier" && control.Kind != "text" && control.Kind != "email") {
				return bad
			}
		}
		if step.FillInput != nil {
			if humanControlLabel(record.protocol.Label) {
				return bad
			}
			control := record.protocol.Control
			if control == nil {
				return bad
			}
			switch step.FillInput.Control {
			case "fill":
				if control.Kind != "text" && control.Kind != "email" && control.Kind != "number" {
					return bad
				}
			case "check":
				if control.Kind != "checkbox" && control.Kind != "switch" {
					return bad
				}
			case "select":
				if control.Kind != "select" {
					return bad
				}
				for _, choice := range profile.InputSlots[step.FillInput.Slot].Enum {
					found := false
					for _, option := range control.Options {
						if choice == option.Value {
							found = true
						}
					}
					if !found {
						return bad
					}
				}
			default:
				return bad
			}
		}
		if step.Click != nil {
			found := false
			for _, preview := range previews {
				if preview.Request.CandidateID == steps[index] && preview.Request.Action == "click" {
					next := history[preview.NextGeneration-1]
					currentOrigin, currentPath = next.Origin, next.Path
					found = true
					break
				}
			}
			if !found {
				return bad
			}
		}
		if step.Submit != nil {
			submitted = true
		}
	}
	if len(used) != len(selected) {
		return bad
	}
	for _, id := range selected {
		if !used[id] {
			return bad
		}
		delete(used, id)
	}
	return nil
}

// FieldSuggestion is inert UI assistance. Operators must confirm the role and
// constraints. Slot names are derived from candidate IDs, never page values.
type FieldSuggestion struct {
	CandidateID    string                         `json:"candidateId"`
	Slot           string                         `json:"slot"`
	CredentialKind string                         `json:"credentialKind,omitempty"`
	Input          *browserregistration.InputSlot `json:"input,omitempty"`
	Control        string                         `json:"control,omitempty"`
	ReviewRequired bool                           `json:"reviewRequired"`
}

func SuggestFields(observation Observation) []FieldSuggestion {
	result := []FieldSuggestion{}
	for _, candidate := range observation.Candidates {
		if candidate.Matches != 1 || !promotableCandidate(candidate) || humanControlLabel(candidate.Label) || ValidateControlMetadata(candidate.Control) != nil {
			continue
		}
		control := candidate.Control
		suggestion := FieldSuggestion{CandidateID: candidate.ID, Slot: "field_" + strings.TrimPrefix(candidate.ID, "candidate-"), ReviewRequired: true}
		required := false
		if control.Required != nil {
			required = *control.Required
		}
		field := browserregistration.InputSlot{Type: "string", Label: candidate.Label, Required: &required, MinLength: control.MinLength, MaxLength: control.MaxLength}
		switch control.Kind {
		case "password":
			suggestion.CredentialKind = "password"
		case "email":
			suggestion.CredentialKind = "identifier"
		case "text":
			suggestion.Input = &field
			suggestion.Control = "fill"
		case "number":
			field.Type = "number"
			field.Minimum = control.Minimum
			field.Maximum = control.Maximum
			suggestion.Input = &field
			suggestion.Control = "fill"
		case "checkbox", "switch":
			field.Type = "boolean"
			suggestion.Input = &field
			suggestion.Control = "check"
		case "select":
			if len(control.Options) == 0 {
				continue
			}
			for _, option := range control.Options {
				field.Enum = append(field.Enum, option.Value)
			}
			suggestion.Input = &field
			suggestion.Control = "select"
		default:
			continue
		}
		result = append(result, suggestion)
	}
	return result
}
