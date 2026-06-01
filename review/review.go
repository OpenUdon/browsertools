// Package review produces review bundles for browser-profile draft candidates.
//
// A review bundle is a deterministic, secret-free artifact that combines:
//   - The candidate profile (as a raw map)
//   - A validation report (pass/fail + error details)
//   - An evidence summary (origin, observation kind, tool provenance)
//   - Unresolved gaps (ambiguous locators, missing CSS fallback reasons, etc.)
//   - Confidence rationale
//   - Expiry/revalidation notes
//   - Origin allowlist summary
//   - Side-effect and confirmation policy summary
//
// Bundles are intended to be stored alongside the profile and read by a human
// reviewer before the profile is promoted to "reviewed" status.
package review

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

// GapKind classifies a gap found during review.
type GapKind string

const (
	GapAmbiguousLocator      GapKind = "ambiguous_locator"
	GapMissingConfirmation   GapKind = "missing_confirmation"
	GapExpiredEvidence       GapKind = "expired_evidence"
	GapCSSFallbackReason     GapKind = "css_fallback_reason_missing"
	GapUnresolvedOutput      GapKind = "unresolved_output_validation"
	GapMissingOriginCoverage GapKind = "missing_origin_coverage"
)

// Gap is an unresolved issue that must be addressed before the profile can be
// promoted to reviewed status.
type Gap struct {
	Kind    GapKind `json:"kind"`
	Field   string  `json:"field"`
	Message string  `json:"message"`
}

// EvidenceSummary is a condensed, secret-free view of the evidence used to
// generate the profile.
type EvidenceSummary struct {
	Origins         []string `json:"origins"`
	ObservationKind string   `json:"observationKind"`
	Tools           []string `json:"tools"`
	RecordCount     int      `json:"recordCount"`
	EarliestAt      string   `json:"earliestAt,omitempty"`
	LatestAt        string   `json:"latestAt,omitempty"`
}

// OriginSummary describes the profile's browser origin allowlist.
type OriginSummary struct {
	Origins []string `json:"origins"`
}

// SideEffectSummary lists all side effects and confirmation requirements found
// in the profile's actions.
type SideEffectSummary struct {
	// HasWriteActions is true if any action has a non-read_only side effect.
	HasWriteActions bool `json:"hasWriteActions"`
	// ActionsRequiringConfirmation lists action names where confirmationPolicy.required=true.
	ActionsRequiringConfirmation []string `json:"actionsRequiringConfirmation"`
	// ActionsWithSideEffects maps action name → side effects.
	ActionsWithSideEffects map[string][]string `json:"actionsWithSideEffects"`
}

// ValidationReport records the result of running profile.Validate on the draft.
type ValidationReport struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// Bundle is the complete review artifact for a profile candidate.
type Bundle struct {
	// Profile is the raw profile map as produced by the draft builder.
	Profile map[string]any `json:"profile"`
	// Validation is the schema validation result.
	Validation ValidationReport `json:"validation"`
	// Evidence summarizes the records used to generate the profile.
	Evidence EvidenceSummary `json:"evidence"`
	// Gaps are issues that block promotion to reviewed status.
	Gaps []Gap `json:"gaps"`
	// ConfidenceRationale explains the confidence value.
	ConfidenceRationale string `json:"confidenceRationale"`
	// ExpiryNote states the expiry duration and revalidation expectation.
	ExpiryNote string `json:"expiryNote"`
	// Origins describes the allowlisted origins.
	Origins OriginSummary `json:"origins"`
	// SideEffects summarizes write actions and confirmation requirements.
	SideEffects SideEffectSummary `json:"sideEffects"`
}

// Build produces a review bundle for the given draft profile and supporting
// evidence records. The profile map is expected to come from draft.Build.
//
// Build never returns an error; all issues are recorded as Gaps in the bundle
// so the caller can inspect the full picture.
func Build(draft map[string]any, records []evidence.Record) *Bundle {
	b := &Bundle{
		Profile: draft,
	}

	// Validation report.
	if err := profile.Validate(draft); err != nil {
		b.Validation = ValidationReport{Valid: false, Errors: []string{err.Error()}}
	} else {
		b.Validation = ValidationReport{Valid: true}
	}

	// Evidence summary.
	b.Evidence = buildEvidenceSummary(records)

	// Origin allowlist.
	b.Origins = buildOriginSummary(draft)

	// Side-effect summary.
	b.SideEffects = buildSideEffectSummary(draft)

	// Confidence rationale.
	b.ConfidenceRationale = buildConfidenceRationale(draft, records)

	// Expiry note.
	b.ExpiryNote = buildExpiryNote(draft)

	// Gaps.
	b.Gaps = collectGaps(draft, records)

	return b
}

func buildEvidenceSummary(records []evidence.Record) EvidenceSummary {
	originsSet := map[string]bool{}
	toolsSet := map[string]bool{}
	var earliest, latest string
	kinds := map[string]bool{}
	for _, rec := range records {
		originsSet[rec.Origin] = true
		if rec.Provenance.Tool != "" {
			toolsSet[rec.Provenance.Tool] = true
		}
		if rec.ObservationKind != "" {
			kinds[string(rec.ObservationKind)] = true
		}
		if rec.ObservedAt != "" {
			if earliest == "" || rec.ObservedAt < earliest {
				earliest = rec.ObservedAt
			}
			if latest == "" || rec.ObservedAt > latest {
				latest = rec.ObservedAt
			}
		}
	}
	obs := "unknown"
	kindList := sortedStringSet(kinds)
	if len(kindList) > 0 {
		obs = kindList[0]
	}
	return EvidenceSummary{
		Origins:         sortedStringSet(originsSet),
		ObservationKind: obs,
		Tools:           sortedStringSet(toolsSet),
		RecordCount:     len(records),
		EarliestAt:      earliest,
		LatestAt:        latest,
	}
}

func buildOriginSummary(draft map[string]any) OriginSummary {
	info, _ := draft["info"].(map[string]any)
	if info == nil {
		return OriginSummary{}
	}
	var origins []string
	switch v := info["origin"].(type) {
	case string:
		origins = []string{v}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				origins = append(origins, s)
			}
		}
	}
	sort.Strings(origins)
	return OriginSummary{Origins: origins}
}

func buildSideEffectSummary(draft map[string]any) SideEffectSummary {
	actions, _ := draft["actions"].(map[string]any)
	s := SideEffectSummary{
		ActionsWithSideEffects: map[string][]string{},
	}
	for name, raw := range actions {
		action, _ := raw.(map[string]any)
		if action == nil {
			continue
		}
		var effects []string
		switch v := action["sideEffects"].(type) {
		case []any:
			for _, e := range v {
				if se, ok := e.(string); ok {
					effects = append(effects, se)
					if se != "read_only" {
						s.HasWriteActions = true
					}
				}
			}
		case []string:
			for _, se := range v {
				effects = append(effects, se)
				if se != "read_only" {
					s.HasWriteActions = true
				}
			}
		}
		if len(effects) > 0 {
			sort.Strings(effects)
			s.ActionsWithSideEffects[name] = effects
		}
		policy, _ := action["confirmationPolicy"].(map[string]any)
		if policy != nil {
			if req, _ := policy["required"].(bool); req {
				s.ActionsRequiringConfirmation = append(s.ActionsRequiringConfirmation, name)
			}
		}
	}
	sort.Strings(s.ActionsRequiringConfirmation)
	return s
}

func buildConfidenceRationale(draft map[string]any, records []evidence.Record) string {
	conf, _ := draft["confidence"].(string)
	recordCount := len(records)
	hasAmbiguity := false
	for _, rec := range records {
		if len(rec.CandidateLocators) > 1 {
			hasAmbiguity = true
		}
	}
	parts := []string{fmt.Sprintf("confidence=%s", conf), fmt.Sprintf("evidence_records=%d", recordCount)}
	if hasAmbiguity {
		parts = append(parts, "has_ambiguous_locators=true")
	}
	return strings.Join(parts, "; ")
}

func buildExpiryNote(draft map[string]any) string {
	exp, _ := draft["expiresAfter"].(string)
	if exp == "" {
		return "no expiry set — revalidation schedule unknown"
	}
	return fmt.Sprintf("profile expires after %s from last verification; revalidate before use in production", exp)
}

func collectGaps(draft map[string]any, records []evidence.Record) []Gap {
	var gaps []Gap

	// Check for expired evidence: compare now against lastVerifiedAt + expiresAfter.
	// learnedAt is when evidence was captured; lastVerifiedAt is when the profile was
	// last confirmed working — that is the relevant clock for expiry.
	verif, _ := draft["verification"].(map[string]any)
	expiresAfter, _ := draft["expiresAfter"].(string)
	if verif != nil && expiresAfter != "" {
		if lastVerified, _ := verif["lastVerifiedAt"].(string); lastVerified != "" {
			if t, err := time.Parse(time.RFC3339, lastVerified); err == nil {
				dur, parseErr := parseISO8601Duration(expiresAfter)
				if parseErr == nil && time.Now().UTC().After(t.Add(dur)) {
					gaps = append(gaps, Gap{
						Kind:    GapExpiredEvidence,
						Field:   "verification.lastVerifiedAt",
						Message: fmt.Sprintf("profile last verified at %s; expiresAfter=%s has elapsed — revalidate before promoting", lastVerified, expiresAfter),
					})
				}
			}
		}
	}

	// Check actions.
	actions, _ := draft["actions"].(map[string]any)
	for _, name := range sortedStringSet(mapKeys(actions)) {
		raw := actions[name]
		action, _ := raw.(map[string]any)
		if action == nil {
			continue
		}

		// Non-read_only without confirmation.required=true.
		var effects []string
		switch v := action["sideEffects"].(type) {
		case []any:
			for _, e := range v {
				if s, ok := e.(string); ok {
					effects = append(effects, s)
				}
			}
		case []string:
			effects = v
		}
		hasWrite := false
		for _, e := range effects {
			if e != "read_only" {
				hasWrite = true
			}
		}
		if hasWrite {
			policy, _ := action["confirmationPolicy"].(map[string]any)
			if policy == nil {
				gaps = append(gaps, Gap{
					Kind:    GapMissingConfirmation,
					Field:   fmt.Sprintf("actions.%s.confirmationPolicy", name),
					Message: fmt.Sprintf("action %q has write side effects but no confirmationPolicy", name),
				})
			} else if req, _ := policy["required"].(bool); !req {
				gaps = append(gaps, Gap{
					Kind:    GapMissingConfirmation,
					Field:   fmt.Sprintf("actions.%s.confirmationPolicy.required", name),
					Message: fmt.Sprintf("action %q has write side effects but confirmationPolicy.required=false", name),
				})
			}
		}

		// CSS outputs missing fallbackReason.
		outputs, _ := action["outputs"].(map[string]any)
		for outName, rawOut := range outputs {
			out, _ := rawOut.(map[string]any)
			if out == nil {
				continue
			}
			if src, _ := out["source"].(string); src == "css" {
				if reason, _ := out["fallbackReason"].(string); reason == "" {
					gaps = append(gaps, Gap{
						Kind:    GapCSSFallbackReason,
						Field:   fmt.Sprintf("actions.%s.outputs.%s.fallbackReason", name, outName),
						Message: fmt.Sprintf("output %q uses css source but has no fallbackReason", outName),
					})
				}
			}
		}
	}

	// Check for ambiguous locators in evidence.
	for _, rec := range records {
		if len(rec.CandidateLocators) > 1 {
			for _, loc := range rec.CandidateLocators {
				if loc.AmbiguityNote != "" {
					gaps = append(gaps, Gap{
						Kind:    GapAmbiguousLocator,
						Field:   fmt.Sprintf("evidence[%s].candidateLocators", rec.ActionHint),
						Message: loc.AmbiguityNote,
					})
					break
				}
			}
		}
	}

	// Sort gaps for determinism.
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].Kind != gaps[j].Kind {
			return gaps[i].Kind < gaps[j].Kind
		}
		return gaps[i].Field < gaps[j].Field
	})
	return gaps
}

func sortedStringSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mapKeys(m map[string]any) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// parseISO8601Duration parses a subset of ISO-8601 durations into a time.Duration.
// Supports P(n)Y, P(n)M, P(n)W, P(n)D and PT(n)H, PT(n)M, PT(n)S forms.
// Uses approximate values: 1Y=365d, 1M=30d.
func parseISO8601Duration(s string) (time.Duration, error) {
	if len(s) == 0 || s[0] != 'P' {
		return 0, fmt.Errorf("invalid ISO-8601 duration %q", s)
	}
	var total time.Duration
	rest := s[1:]
	inTime := false
	if len(rest) > 0 && rest[0] == 'T' {
		inTime = true
		rest = rest[1:]
	}
	for len(rest) > 0 {
		if rest[0] == 'T' {
			inTime = true
			rest = rest[1:]
			continue
		}
		// Read digits
		n := 0
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			n = n*10 + int(rest[i]-'0')
			i++
		}
		if i == 0 {
			return 0, fmt.Errorf("invalid ISO-8601 duration: expected digit before unit in %q", s)
		}
		if i >= len(rest) {
			break
		}
		unit := rest[i]
		rest = rest[i+1:]
		switch {
		case !inTime && unit == 'Y':
			total += time.Duration(n) * 365 * 24 * time.Hour
		case !inTime && unit == 'M':
			total += time.Duration(n) * 30 * 24 * time.Hour
		case !inTime && unit == 'W':
			total += time.Duration(n) * 7 * 24 * time.Hour
		case !inTime && unit == 'D':
			total += time.Duration(n) * 24 * time.Hour
		case inTime && unit == 'H':
			total += time.Duration(n) * time.Hour
		case inTime && unit == 'M':
			total += time.Duration(n) * time.Minute
		case inTime && unit == 'S':
			total += time.Duration(n) * time.Second
		default:
			return 0, fmt.Errorf("unknown ISO-8601 duration unit %q in %q", unit, s)
		}
	}
	return total, nil
}
