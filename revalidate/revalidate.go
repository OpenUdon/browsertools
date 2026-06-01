// Package revalidate defines dry-run revalidation contracts for reviewed
// browser-profile documents.
//
// Revalidation checks a profile's consistency against a set of normalized
// evidence records without performing any browser interaction or side effects.
// The default implementation (Check) operates entirely on in-memory data;
// live browser revalidation is represented by the LiveRevalidator stub which
// always returns ErrLiveNotSupported.
//
// Checks performed:
//   - Origin: every evidence record's origin must be covered by the profile's
//     info.origin allowlist.
//   - Locators: each action that has CandidateLocators in the evidence must
//     have at least one locator present (non-empty Role).
//   - Ambiguity: locator evidence carrying AmbiguityNote must be resolved
//     before promotion.
//   - Outputs: each action output must have a valid source-specific shape.
//   - Expiry: verification.lastVerifiedAt + expiresAfter must not have elapsed.
//   - CSS fallback: outputs with source=css must carry a fallbackReason.
//   - Side-effect confirmation: actions with non-read_only sideEffects must
//     have confirmationPolicy.required=true and a post-action wait.
package revalidate

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/duration"
	"github.com/OpenUdon/browsertools/internal/profilechecks"
)

// ErrLiveNotSupported is returned by LiveRevalidator. Live browser revalidation
// is never performed in default tests.
var ErrLiveNotSupported = errors.New("live browser revalidation is not supported in this build; use a LiveRevalidator implementation that wires a real browser session")

// CheckKind classifies a revalidation check failure.
type CheckKind string

const (
	CheckOriginMismatch       CheckKind = "origin_mismatch"
	CheckMissingLocator       CheckKind = "missing_locator"
	CheckAmbiguousLocator     CheckKind = "ambiguous_locator"
	CheckExpired              CheckKind = "expired"
	CheckInvalidOutputShape   CheckKind = "invalid_output_shape"
	CheckCSSMissingFallback   CheckKind = "css_missing_fallback_reason"
	CheckSideEffectNoConfirm  CheckKind = "side_effect_no_confirmation"
	CheckSideEffectNoSafeWait CheckKind = "side_effect_no_safe_wait"
)

// Failure is a single revalidation check failure.
type Failure struct {
	Kind    CheckKind `json:"kind"`
	Field   string    `json:"field"`
	Message string    `json:"message"`
}

// Result is the outcome of a revalidation run.
type Result struct {
	// OK is true when all checks pass.
	OK bool `json:"ok"`
	// Failures lists every check that did not pass, sorted by (Kind, Field).
	Failures []Failure `json:"failures,omitempty"`
}

// Revalidator is implemented by both the fixture-based Check function (wrapped
// as a value type) and the LiveRevalidator stub.
type Revalidator interface {
	Revalidate(profile map[string]any, records []evidence.Record) (Result, error)
}

// FixtureRevalidator wraps Check as a Revalidator.
type FixtureRevalidator struct{}

// Revalidate implements Revalidator using the pure fixture-based Check function.
func (FixtureRevalidator) Revalidate(profile map[string]any, records []evidence.Record) (Result, error) {
	return Check(profile, records), nil
}

// LiveRevalidator is a stub that satisfies Revalidator but always returns
// ErrLiveNotSupported. A real implementation would wire a browser session.
type LiveRevalidator struct{}

// Revalidate implements Revalidator and always returns ErrLiveNotSupported.
func (LiveRevalidator) Revalidate(_ map[string]any, _ []evidence.Record) (Result, error) {
	return Result{}, ErrLiveNotSupported
}

// Check runs all dry-run revalidation checks against the given profile map and
// evidence records. It never contacts a browser or network service.
// The profile map is the same raw map[string]any shape produced by draft.Build
// or loaded from a YAML/JSON file.
func Check(prof map[string]any, records []evidence.Record) Result {
	var failures []Failure

	failures = append(failures, checkOrigins(prof, records)...)
	failures = append(failures, checkLocators(prof, records)...)
	failures = append(failures, checkExpiry(prof)...)
	failures = append(failures, semanticFailures(profilechecks.CheckOutputs(prof))...)
	failures = append(failures, semanticFailures(profilechecks.CheckCSSFallbacks(prof))...)
	failures = append(failures, semanticFailures(profilechecks.CheckSideEffectConfirmation(prof))...)
	failures = append(failures, semanticFailures(profilechecks.CheckSideEffectSafeWait(prof))...)

	sort.SliceStable(failures, func(i, j int) bool {
		if failures[i].Kind != failures[j].Kind {
			return failures[i].Kind < failures[j].Kind
		}
		return failures[i].Field < failures[j].Field
	})

	return Result{OK: len(failures) == 0, Failures: failures}
}

// checkOrigins verifies every evidence record origin is in the profile allowlist.
func checkOrigins(prof map[string]any, records []evidence.Record) []Failure {
	allowed := originSet(prof)
	var failures []Failure
	seen := map[string]bool{}
	for _, rec := range records {
		if seen[rec.Origin] {
			continue
		}
		seen[rec.Origin] = true
		if !allowed[rec.Origin] {
			failures = append(failures, Failure{
				Kind:    CheckOriginMismatch,
				Field:   "info.origin",
				Message: fmt.Sprintf("evidence origin %q is not in the profile origin allowlist %v", rec.Origin, sortedMapKeys(allowed)),
			})
		}
	}
	return failures
}

// checkLocators verifies each evidence action group has at least one locator.
func checkLocators(prof map[string]any, records []evidence.Record) []Failure {
	actions, _ := prof["actions"].(map[string]any)
	byHint := map[string][]evidence.Record{}
	for _, rec := range records {
		if rec.ActionHint == "" {
			continue // no action hint — cannot match to a profile action
		}
		byHint[rec.ActionHint] = append(byHint[rec.ActionHint], rec)
	}
	var failures []Failure
	for hint, recs := range byHint {
		if _, hasAction := actions[hint]; !hasAction {
			continue
		}
		hasLocator := false
		var ambiguityNote string
		for _, rec := range recs {
			for _, loc := range rec.CandidateLocators {
				if loc.Role != "" {
					hasLocator = true
				}
				if loc.AmbiguityNote != "" && ambiguityNote == "" {
					ambiguityNote = loc.AmbiguityNote
				}
			}
		}
		if !hasLocator {
			failures = append(failures, Failure{
				Kind:    CheckMissingLocator,
				Field:   fmt.Sprintf("actions.%s", hint),
				Message: fmt.Sprintf("action %q has no candidate locators in evidence; add accessibility snapshot evidence before promoting", hint),
			})
		}
		if ambiguityNote != "" {
			failures = append(failures, Failure{
				Kind:    CheckAmbiguousLocator,
				Field:   fmt.Sprintf("actions.%s", hint),
				Message: fmt.Sprintf("action %q has unresolved ambiguous locator evidence: %s", hint, ambiguityNote),
			})
		}
	}
	return failures
}

// checkExpiry verifies lastVerifiedAt + expiresAfter has not elapsed.
// Malformed timestamps or durations produce a CheckExpired failure rather than
// being silently skipped, so broken metadata cannot mask an expired profile.
func checkExpiry(prof map[string]any) []Failure {
	verif, _ := prof["verification"].(map[string]any)
	expiresAfter, _ := prof["expiresAfter"].(string)
	if verif == nil || expiresAfter == "" {
		return nil
	}
	lastVerified, _ := verif["lastVerifiedAt"].(string)
	if lastVerified == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, lastVerified)
	if err != nil {
		return []Failure{{
			Kind:    CheckExpired,
			Field:   "verification.lastVerifiedAt",
			Message: fmt.Sprintf("malformed lastVerifiedAt %q: %v", lastVerified, err),
		}}
	}
	dur, err := duration.Parse(expiresAfter)
	if err != nil {
		return []Failure{{
			Kind:    CheckExpired,
			Field:   "expiresAfter",
			Message: fmt.Sprintf("malformed expiresAfter %q: %v", expiresAfter, err),
		}}
	}
	if time.Now().UTC().After(t.Add(dur)) {
		return []Failure{{
			Kind:    CheckExpired,
			Field:   "verification.lastVerifiedAt",
			Message: fmt.Sprintf("profile last verified at %s; expiresAfter=%s has elapsed — revalidate with a live browser before using in production", lastVerified, expiresAfter),
		}}
	}
	return nil
}

func semanticFailures(issues []profilechecks.Issue) []Failure {
	failures := make([]Failure, 0, len(issues))
	for _, issue := range issues {
		failures = append(failures, Failure{
			Kind:    checkKind(issue.Rule),
			Field:   issue.Field,
			Message: issue.Message,
		})
	}
	return failures
}

func checkKind(rule profilechecks.Rule) CheckKind {
	switch rule {
	case profilechecks.RuleInvalidOutputShape:
		return CheckInvalidOutputShape
	case profilechecks.RuleCSSMissingFallback:
		return CheckCSSMissingFallback
	case profilechecks.RuleSideEffectNoConfirm:
		return CheckSideEffectNoConfirm
	case profilechecks.RuleSideEffectNoSafeWait:
		return CheckSideEffectNoSafeWait
	default:
		return CheckKind(rule)
	}
}

// --- helpers ---

func originSet(prof map[string]any) map[string]bool {
	info, _ := prof["info"].(map[string]any)
	if info == nil {
		return map[string]bool{}
	}
	out := map[string]bool{}
	switch v := info["origin"].(type) {
	case string:
		out[v] = true
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

// sortedMapKeys returns the keys of any map[string]V in sorted order.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
