// Package revalidate performs deterministic, fixture-only health checks for
// reviewed browser profiles. It never launches a browser, contacts a network,
// binds a session, or executes an action.
package revalidate

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

// CheckKind classifies a revalidation failure.
type CheckKind string

const (
	CheckInvalidProfile       CheckKind = "invalid_profile"
	CheckInvalidEvidence      CheckKind = "invalid_evidence"
	CheckOriginMismatch       CheckKind = "origin_mismatch"
	CheckMissingEvidence      CheckKind = "missing_evidence"
	CheckMissingLocator       CheckKind = "missing_locator"
	CheckAmbiguousLocator     CheckKind = "ambiguous_locator"
	CheckExpired              CheckKind = "expired"
	CheckSideEffectNoSafeWait CheckKind = "side_effect_no_safe_wait"
)

// Failure is one deterministic, path-tagged failed check.
type Failure struct {
	Kind    CheckKind `json:"kind"`
	Field   string    `json:"field"`
	Message string    `json:"message"`
}

// Result is the outcome of a fixture revalidation pass.
type Result struct {
	OK       bool      `json:"ok"`
	Failures []Failure `json:"failures"`
}

// CheckAt checks profile against normalized evidence and explicit ambiguity
// decisions at now. A zero now is rejected so expiry results are reproducible.
func CheckAt(prof *profile.Profile, records []evidence.Record, decisions []evidence.LocatorDecision, now time.Time) (Result, error) {
	if prof == nil {
		return Result{}, fmt.Errorf("revalidate: profile is required")
	}
	if now.IsZero() {
		return Result{}, fmt.Errorf("revalidate: assessment time is required")
	}
	validationErr := profile.ValidateTyped(prof)
	return checkAt(prof, records, decisions, now, validationErr)
}

// CheckValidatedAt runs fixture and lifecycle checks for a profile already
// schema-validated by the enclosing top-level operation.
func CheckValidatedAt(prof *profile.Profile, records []evidence.Record, decisions []evidence.LocatorDecision, now time.Time) (Result, error) {
	if prof == nil {
		return Result{}, fmt.Errorf("revalidate: profile is required")
	}
	if now.IsZero() {
		return Result{}, fmt.Errorf("revalidate: assessment time is required")
	}
	return checkAt(prof, records, decisions, now, nil)
}

func checkAt(prof *profile.Profile, records []evidence.Record, decisions []evidence.LocatorDecision, now time.Time, validationErr error) (Result, error) {
	var failures []Failure
	if validationErr != nil {
		failures = append(failures, Failure{Kind: CheckInvalidProfile, Field: "$", Message: validationErr.Error()})
	}

	failures = append(failures, checkOrigins(prof, records)...)
	failures = append(failures, checkRecordValidity(records)...)
	failures = append(failures, checkEvidence(prof, records, decisions)...)
	failures = append(failures, checkExpiry(prof, now)...)
	failures = append(failures, checkSafeWaits(prof)...)

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Kind != failures[j].Kind {
			return failures[i].Kind < failures[j].Kind
		}
		if failures[i].Field != failures[j].Field {
			return failures[i].Field < failures[j].Field
		}
		return failures[i].Message < failures[j].Message
	})
	if failures == nil {
		failures = []Failure{}
	}
	return Result{OK: len(failures) == 0, Failures: failures}, nil
}

func checkRecordValidity(records []evidence.Record) []Failure {
	var failures []Failure
	for i, record := range records {
		raw := evidence.RawRecord{Record: record}
		normalized, err := raw.Normalize()
		if err != nil {
			failures = append(failures, Failure{
				Kind: CheckInvalidEvidence, Field: fmt.Sprintf("evidence[%d]", i), Message: err.Error(),
			})
		} else if !reflect.DeepEqual(record, normalized) {
			failures = append(failures, Failure{
				Kind: CheckInvalidEvidence, Field: fmt.Sprintf("evidence[%d]", i),
				Message: "record is not in canonical normalized form; retain the value returned by RawRecord.Normalize",
			})
		}
	}
	return failures
}

func checkOrigins(prof *profile.Profile, records []evidence.Record) []Failure {
	var failures []Failure
	allowedOrigins := map[string]struct{}{}
	for _, candidate := range prof.Info.Origin {
		canonical, err := profile.ParseOrigin(candidate)
		if err != nil {
			failures = append(failures, Failure{
				Kind: CheckOriginMismatch, Field: "info.origin",
				Message: fmt.Sprintf("profile origin %q is invalid: %v", candidate, err),
			})
			continue
		}
		allowedOrigins[canonical] = struct{}{}
	}
	seen := map[string]bool{}
	for _, rec := range records {
		if seen[rec.Origin] {
			continue
		}
		seen[rec.Origin] = true
		canonical, err := profile.ParseOrigin(rec.Origin)
		if err != nil {
			failures = append(failures, Failure{
				Kind: CheckOriginMismatch, Field: "info.origin",
				Message: fmt.Sprintf("evidence origin %q is invalid: %v", rec.Origin, err),
			})
			continue
		}
		if _, allowed := allowedOrigins[canonical]; !allowed {
			failures = append(failures, Failure{
				Kind: CheckOriginMismatch, Field: "info.origin",
				Message: fmt.Sprintf("evidence origin %q is not in the profile origin allowlist", canonical),
			})
		}
	}
	return failures
}

type declaredLocator struct {
	path string
	loc  profile.Locator
}

func checkEvidence(prof *profile.Profile, records []evidence.Record, decisions []evidence.LocatorDecision) []Failure {
	byAction := map[string][]evidence.Record{}
	for _, rec := range records {
		if rec.ActionHint != "" {
			byAction[rec.ActionHint] = append(byAction[rec.ActionHint], rec)
		}
	}
	var failures []Failure
	for _, actionName := range prof.SortedActionNames() {
		actionRecords := byAction[actionName]
		if len(actionRecords) == 0 {
			failures = append(failures, Failure{
				Kind: CheckMissingEvidence, Field: "actions." + actionName,
				Message: fmt.Sprintf("action %q has no matching evidence record", actionName),
			})
			continue
		}
		for _, declared := range actionLocators(actionName, prof.Actions[actionName]) {
			matches := matchingCandidates(declared.loc, actionRecords)
			if len(matches) == 0 {
				failures = append(failures, Failure{
					Kind: CheckMissingLocator, Field: declared.path,
					Message: fmt.Sprintf("declared locator role=%q name=%q has no matching saved evidence", declared.loc.Role, declared.loc.Name),
				})
				continue
			}
			ambiguous := false
			for _, match := range matches {
				if match.AmbiguityNote != "" {
					ambiguous = true
					break
				}
			}
			if ambiguous && !hasDecision(actionName, declared.loc, decisions) {
				failures = append(failures, Failure{
					Kind: CheckAmbiguousLocator, Field: declared.path,
					Message: fmt.Sprintf("declared locator role=%q name=%q has ambiguous evidence without a reviewed rationale", declared.loc.Role, declared.loc.Name),
				})
			}
		}
	}
	return failures
}

func actionLocators(actionName string, action profile.Action) []declaredLocator {
	var out []declaredLocator
	for i, step := range action.Sequence {
		base := fmt.Sprintf("actions.%s.sequence[%d]", actionName, i)
		if loc := step.Locator(); loc != nil {
			out = append(out, declaredLocator{path: base + ".locator", loc: *loc})
		}
		if wait := step.PostWait(); wait != nil && wait.Locator != nil {
			out = append(out, declaredLocator{path: base + ".wait_for", loc: *wait.Locator})
		}
	}
	outputNames := make([]string, 0, len(action.Outputs))
	for name := range action.Outputs {
		outputNames = append(outputNames, name)
	}
	sort.Strings(outputNames)
	for _, name := range outputNames {
		outSpec := action.Outputs[name]
		if outSpec.Source == profile.OutputA11y && outSpec.Locator != nil {
			out = append(out, declaredLocator{path: fmt.Sprintf("actions.%s.outputs.%s.locator", actionName, name), loc: *outSpec.Locator})
		}
	}
	return out
}

func matchingCandidates(loc profile.Locator, records []evidence.Record) []evidence.CandidateLocator {
	var matches []evidence.CandidateLocator
	for _, rec := range records {
		for _, candidate := range rec.CandidateLocators {
			if candidate.Role == string(loc.Role) && candidate.Name == loc.Name && candidate.Text == loc.Text && candidate.Value == loc.Value {
				matches = append(matches, candidate)
			}
		}
		for _, output := range rec.CandidateOutputs {
			candidate := output.Locator
			if candidate != nil && candidate.Role == string(loc.Role) && candidate.Name == loc.Name && candidate.Text == loc.Text && candidate.Value == loc.Value {
				matches = append(matches, *candidate)
			}
		}
	}
	return matches
}

func hasDecision(actionName string, loc profile.Locator, decisions []evidence.LocatorDecision) bool {
	candidate := evidence.CandidateLocator{Role: string(loc.Role), Name: loc.Name, Text: loc.Text, Value: loc.Value}
	for _, decision := range decisions {
		if strings.TrimSpace(decision.Rationale) != "" && decision.Matches(actionName, candidate) {
			return true
		}
	}
	return false
}

func checkExpiry(prof *profile.Profile, now time.Time) []Failure {
	verifiedAt, err := time.Parse(time.RFC3339, prof.Verification.LastVerifiedAt)
	if err != nil {
		return []Failure{{Kind: CheckExpired, Field: "verification.lastVerifiedAt", Message: err.Error()}}
	}
	expiresAt, err := prof.ExpiresAfter.AddTo(verifiedAt)
	if err != nil {
		return []Failure{{Kind: CheckExpired, Field: "expiresAfter", Message: err.Error()}}
	}
	if !now.Before(expiresAt) {
		return []Failure{{
			Kind: CheckExpired, Field: "verification.lastVerifiedAt",
			Message: fmt.Sprintf("profile expired at %s", expiresAt.UTC().Format(time.RFC3339)),
		}}
	}
	return nil
}

func checkSafeWaits(prof *profile.Profile) []Failure {
	var failures []Failure
	for _, actionName := range prof.SortedActionNames() {
		action := prof.Actions[actionName]
		if !hasWriteSideEffect(action) {
			continue
		}
		last := -1
		for i, step := range action.Sequence {
			switch step.Kind {
			case profile.StepClick, profile.StepTypeText, profile.StepCheckRadio, profile.StepUncheck, profile.StepSelectOption:
				last = i
			}
		}
		safe := false
		if last >= 0 {
			safe = action.Sequence[last].PostWait() != nil
			if !safe && last+1 < len(action.Sequence) {
				safe = action.Sequence[last+1].Kind == profile.StepWaitFor
			}
		}
		if !safe {
			failures = append(failures, Failure{
				Kind: CheckSideEffectNoSafeWait, Field: "actions." + actionName + ".sequence",
				Message: fmt.Sprintf("side-effectful action %q has no completion wait after its final actionable macro", actionName),
			})
		}
	}
	return failures
}

func hasWriteSideEffect(action profile.Action) bool {
	return profile.HasWriteSideEffects(action)
}
