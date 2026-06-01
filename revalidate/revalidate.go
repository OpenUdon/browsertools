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
//   - Expiry: verification.lastVerifiedAt + expiresAfter must not have elapsed.
//   - CSS fallback: outputs with source=css must carry a fallbackReason.
//   - Side-effect confirmation: actions with non-read_only sideEffects must
//     have confirmationPolicy.required=true.
package revalidate

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/duration"
)

// ErrLiveNotSupported is returned by LiveRevalidator. Live browser revalidation
// is never performed in default tests.
var ErrLiveNotSupported = errors.New("live browser revalidation is not supported in this build; use a LiveRevalidator implementation that wires a real browser session")

// CheckKind classifies a revalidation check failure.
type CheckKind string

const (
	CheckOriginMismatch          CheckKind = "origin_mismatch"
	CheckMissingLocator          CheckKind = "missing_locator"
	CheckExpired                 CheckKind = "expired"
	CheckCSSMissingFallback      CheckKind = "css_missing_fallback_reason"
	CheckSideEffectNoConfirm     CheckKind = "side_effect_no_confirmation"
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
	failures = append(failures, checkCSSFallbacks(prof)...)
	failures = append(failures, checkSideEffects(prof)...)

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
		for _, rec := range recs {
			if len(rec.CandidateLocators) > 0 && rec.CandidateLocators[0].Role != "" {
				hasLocator = true
				break
			}
		}
		if !hasLocator {
			failures = append(failures, Failure{
				Kind:    CheckMissingLocator,
				Field:   fmt.Sprintf("actions.%s", hint),
				Message: fmt.Sprintf("action %q has no candidate locators in evidence; add accessibility snapshot evidence before promoting", hint),
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

// checkCSSFallbacks verifies every css-source output carries a fallbackReason.
func checkCSSFallbacks(prof map[string]any) []Failure {
	actions, _ := prof["actions"].(map[string]any)
	var failures []Failure
	for _, name := range sortedMapKeys(actions) {
		action, _ := actions[name].(map[string]any)
		if action == nil {
			continue
		}
		outputs, _ := action["outputs"].(map[string]any)
		for outName, rawOut := range outputs {
			out, _ := rawOut.(map[string]any)
			if out == nil {
				continue
			}
			if src, _ := out["source"].(string); src == "css" {
				if reason, _ := out["fallbackReason"].(string); reason == "" {
					failures = append(failures, Failure{
						Kind:    CheckCSSMissingFallback,
						Field:   fmt.Sprintf("actions.%s.outputs.%s.fallbackReason", name, outName),
						Message: fmt.Sprintf("output %q uses css source but has no fallbackReason; add one before promoting", outName),
					})
				}
			}
		}
	}
	return failures
}

// checkSideEffects verifies non-read_only actions have confirmationPolicy.required=true.
func checkSideEffects(prof map[string]any) []Failure {
	actions, _ := prof["actions"].(map[string]any)
	var failures []Failure
	for _, name := range sortedMapKeys(actions) {
		action, _ := actions[name].(map[string]any)
		if action == nil {
			continue
		}
		hasWrite := false
		switch v := action["sideEffects"].(type) {
		case []any:
			for _, e := range v {
				if s, _ := e.(string); s != "" && s != "read_only" {
					hasWrite = true
				}
			}
		case []string:
			for _, s := range v {
				if s != "read_only" {
					hasWrite = true
				}
			}
		}
		if !hasWrite {
			continue
		}
		policy, _ := action["confirmationPolicy"].(map[string]any)
		if policy == nil {
			failures = append(failures, Failure{
				Kind:    CheckSideEffectNoConfirm,
				Field:   fmt.Sprintf("actions.%s.confirmationPolicy", name),
				Message: fmt.Sprintf("action %q has write side effects but no confirmationPolicy", name),
			})
		} else if req, _ := policy["required"].(bool); !req {
			failures = append(failures, Failure{
				Kind:    CheckSideEffectNoConfirm,
				Field:   fmt.Sprintf("actions.%s.confirmationPolicy.required", name),
				Message: fmt.Sprintf("action %q has write side effects but confirmationPolicy.required=false", name),
			})
		}
	}
	return failures
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
