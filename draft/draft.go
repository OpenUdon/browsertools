// Package draft builds strict, deterministic browser-profile candidates from
// normalized evidence plus explicit reviewed action intent.
package draft

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/revalidate"
)

// Spec is Browsertools-only authoring input. It is not part of the portable
// browser.1.5 profile. Every action must explicitly describe its sequence and
// safety policy; evidence alone never invents those semantics.
type Spec struct {
	Info            profile.Info               `json:"info" yaml:"info"`
	ObservationKind profile.ObservationKind    `json:"observationKind" yaml:"observationKind"`
	Confidence      profile.Confidence         `json:"confidence" yaml:"confidence"`
	ExpiresAfter    profile.Duration           `json:"expiresAfter" yaml:"expiresAfter"`
	Actions         map[string]ActionSpec      `json:"actions" yaml:"actions"`
	Decisions       []evidence.LocatorDecision `json:"decisions,omitempty" yaml:"decisions,omitempty"`
}

// ActionSpec is the explicit reviewed intent for one profile action. When
// Outputs is empty, deterministic candidate outputs may be imported from the
// matching evidence records; sequences and safety fields are never inferred.
type ActionSpec struct {
	Description        string                     `json:"description,omitempty" yaml:"description,omitempty"`
	Parameters         profile.JSONSchema         `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Sequence           []profile.Step             `json:"sequence" yaml:"sequence"`
	Outputs            map[string]profile.Output  `json:"outputs" yaml:"outputs"`
	SideEffects        []profile.SideEffect       `json:"sideEffects" yaml:"sideEffects"`
	ConfirmationPolicy profile.ConfirmationPolicy `json:"confirmationPolicy" yaml:"confirmationPolicy"`
}

// Result contains the typed draft, decisions carried into review, and any
// blocking diagnostics. Build may return a non-nil Result with an error so a
// caller can render all failures at once.
type Result struct {
	Profile     *profile.Profile           `json:"profile"`
	Decisions   []evidence.LocatorDecision `json:"decisions"`
	Diagnostics []profile.Issue            `json:"diagnostics"`
}

// ReadyForReview reports whether the result has a valid profile and no
// blocking diagnostics.
func (r *Result) ReadyForReview() bool {
	return r != nil && r.Profile != nil && len(r.Diagnostics) == 0
}

// Build constructs a candidate profile from evidence and explicit action
// intent. It never inserts placeholder navigation, guesses a click, or assumes
// an action is read-only.
func Build(records []evidence.Record, spec Spec) (*Result, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("draft: at least one normalized evidence record is required")
	}
	if spec.Info.Title == "" || len(spec.Info.Origin) == 0 {
		return nil, fmt.Errorf("draft: info.title and info.origin are required")
	}
	if spec.ObservationKind == "" || spec.Confidence == "" || spec.ExpiresAfter == "" {
		return nil, fmt.Errorf("draft: observationKind, confidence, and expiresAfter are required")
	}
	if len(spec.Actions) == 0 {
		return nil, fmt.Errorf("draft: at least one explicit action specification is required")
	}

	canonicalOrigins := make(profile.Origins, len(spec.Info.Origin))
	for i, raw := range spec.Info.Origin {
		canonical, err := profile.ParseOrigin(raw)
		if err != nil {
			return nil, fmt.Errorf("draft: info.origin[%d]: %w", i, err)
		}
		canonicalOrigins[i] = canonical
	}
	spec.Info.Origin = canonicalOrigins

	earliest, latest, err := timeRange(records)
	if err != nil {
		return nil, err
	}
	actions := make(map[string]profile.Action, len(spec.Actions))
	for _, actionName := range sortedKeys(spec.Actions) {
		actionSpec := spec.Actions[actionName]
		if len(actionSpec.Sequence) == 0 {
			return nil, fmt.Errorf("draft: action %q must declare a non-empty sequence", actionName)
		}
		if len(actionSpec.SideEffects) == 0 {
			return nil, fmt.Errorf("draft: action %q must explicitly declare sideEffects", actionName)
		}
		outputs := cloneOutputs(actionSpec.Outputs)
		// A nil map means the author omitted output intent and permits the
		// historical deterministic candidate import. A present-but-empty map is
		// an explicit declaration that the action has no outputs. Guided
		// authoring relies on this distinction so an operator's "none" answer
		// cannot silently turn into inferred outputs.
		if actionSpec.Outputs == nil {
			outputs = candidateOutputs(actionName, records)
		}
		parameters, err := cloneSchema(actionSpec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("draft: clone action %q parameters: %w", actionName, err)
		}
		action := profile.Action{
			Description:        actionSpec.Description,
			Parameters:         parameters,
			Sequence:           actionSpec.Sequence,
			Outputs:            outputs,
			SideEffects:        actionSpec.SideEffects,
			ConfirmationPolicy: actionSpec.ConfirmationPolicy,
		}
		cloned, err := profile.CloneAction(action)
		if err != nil {
			return nil, fmt.Errorf("draft: clone action %q: %w", actionName, err)
		}
		actions[actionName] = cloned
	}

	prof := &profile.Profile{
		Schema:          "uws.browser.1.5",
		Info:            spec.Info,
		ObservationKind: spec.ObservationKind,
		Evidence:        profile.Evidence{LearnedAt: earliest, Source: "browsertools_draft"},
		Confidence:      spec.Confidence,
		ExpiresAfter:    spec.ExpiresAfter,
		Verification:    profile.Verification{LastVerifiedAt: latest, SuccessfulRuns: 0},
		Actions:         actions,
	}
	result := &Result{
		Profile:   prof,
		Decisions: append([]evidence.LocatorDecision(nil), spec.Decisions...),
	}

	checkedAt, err := time.Parse(time.RFC3339, latest)
	if err != nil {
		return result, fmt.Errorf("draft: latest evidence time: %w", err)
	}
	revalidation, checkErr := revalidate.CheckAt(prof, records, result.Decisions, checkedAt)
	if checkErr != nil {
		return result, checkErr
	}
	for _, failure := range revalidation.Failures {
		result.Diagnostics = append(result.Diagnostics, profile.Issue{
			Code: string(failure.Kind), Path: failure.Field, Message: failure.Message,
		})
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []profile.Issue{}
	}
	if len(result.Diagnostics) > 0 {
		return result, fmt.Errorf("draft: %d blocking diagnostic(s); first: %s: %s", len(result.Diagnostics), result.Diagnostics[0].Path, result.Diagnostics[0].Message)
	}
	return result, nil
}

// MarshalProfile serializes a typed draft as deterministic indented JSON.
func MarshalProfile(prof *profile.Profile) ([]byte, error) {
	if prof == nil {
		return nil, fmt.Errorf("draft: profile is required")
	}
	return json.MarshalIndent(prof, "", "  ")
}

func candidateOutputs(actionName string, records []evidence.Record) map[string]profile.Output {
	outputs := map[string]profile.Output{}
	for _, rec := range records {
		if rec.ActionHint != actionName {
			continue
		}
		for _, candidate := range rec.CandidateOutputs {
			if candidate.Key == "" || candidate.Source == "" {
				continue
			}
			if _, exists := outputs[candidate.Key]; exists {
				continue
			}
			out := profile.Output{
				Type: profile.OutputType(candidate.Type), Source: profile.OutputSource(candidate.Source),
				Selector: candidate.Selector, FallbackReason: profile.FallbackReason(candidate.FallbackReason),
				Property: candidate.Property,
			}
			if candidate.Locator != nil {
				out.Locator = &profile.Locator{
					Role: profile.Role(candidate.Locator.Role), Name: candidate.Locator.Name,
					Text: candidate.Locator.Text, Value: candidate.Locator.Value,
				}
			}
			if out.Source == profile.OutputCSS {
				out.Validation = profile.JSONSchema{"type": candidate.Type}
			}
			outputs[candidate.Key] = out
		}
	}
	if len(outputs) == 0 {
		return nil
	}
	return outputs
}

func timeRange(records []evidence.Record) (string, string, error) {
	var earliest, latest time.Time
	for i, rec := range records {
		observed, err := time.Parse(time.RFC3339, rec.ObservedAt)
		if err != nil {
			return "", "", fmt.Errorf("draft: evidence[%d].observedAt: %w", i, err)
		}
		observed = observed.UTC()
		if earliest.IsZero() || observed.Before(earliest) {
			earliest = observed
		}
		if latest.IsZero() || observed.After(latest) {
			latest = observed
		}
	}
	return earliest.Format(time.RFC3339), latest.Format(time.RFC3339), nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneSchema(schema profile.JSONSchema) (profile.JSONSchema, error) {
	if schema == nil {
		return nil, nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var result profile.JSONSchema
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func cloneOutputs(outputs map[string]profile.Output) map[string]profile.Output {
	if outputs == nil {
		return nil
	}
	result := make(map[string]profile.Output, len(outputs))
	for key, value := range outputs {
		result[key] = value
	}
	return result
}
