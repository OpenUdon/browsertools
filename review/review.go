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
	"github.com/OpenUdon/browsertools/internal/duration"
	"github.com/OpenUdon/browsertools/internal/profilechecks"
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

	// Gaps — always a non-nil slice so JSON serializes as [] not null.
	gaps := collectGaps(draft, records)
	if gaps == nil {
		gaps = []Gap{}
	}
	b.Gaps = gaps

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
	if origins == nil {
		origins = []string{}
	}
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
	if s.ActionsRequiringConfirmation == nil {
		s.ActionsRequiringConfirmation = []string{}
	}
	return s
}

func buildConfidenceRationale(draft map[string]any, records []evidence.Record) string {
	conf, _ := draft["confidence"].(string)
	recordCount := len(records)
	hasAmbiguity := false
	for _, rec := range records {
		for _, loc := range rec.CandidateLocators {
			if loc.AmbiguityNote != "" {
				hasAmbiguity = true
				break
			}
		}
		if hasAmbiguity {
			break
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
			t, err := time.Parse(time.RFC3339, lastVerified)
			if err != nil {
				gaps = append(gaps, Gap{
					Kind:    GapExpiredEvidence,
					Field:   "verification.lastVerifiedAt",
					Message: fmt.Sprintf("malformed lastVerifiedAt %q: %v", lastVerified, err),
				})
			} else if dur, parseErr := duration.Parse(expiresAfter); parseErr != nil {
				gaps = append(gaps, Gap{
					Kind:    GapExpiredEvidence,
					Field:   "expiresAfter",
					Message: fmt.Sprintf("malformed expiresAfter %q: %v", expiresAfter, parseErr),
				})
			} else if time.Now().UTC().After(t.Add(dur)) {
				gaps = append(gaps, Gap{
					Kind:    GapExpiredEvidence,
					Field:   "verification.lastVerifiedAt",
					Message: fmt.Sprintf("profile last verified at %s; expiresAfter=%s has elapsed — revalidate before promoting", lastVerified, expiresAfter),
				})
			}
		}
	}

	allowedOrigins := map[string]bool{}
	for _, origin := range buildOriginSummary(draft).Origins {
		allowedOrigins[origin] = true
	}
	seenOrigins := map[string]bool{}
	for _, rec := range records {
		if rec.Origin == "" || seenOrigins[rec.Origin] {
			continue
		}
		seenOrigins[rec.Origin] = true
		if !allowedOrigins[rec.Origin] {
			gaps = append(gaps, Gap{
				Kind:    GapMissingOriginCoverage,
				Field:   "info.origin",
				Message: fmt.Sprintf("evidence origin %q is not covered by the profile origin allowlist", rec.Origin),
			})
		}
	}

	gaps = append(gaps, semanticGaps(profilechecks.CheckSideEffectConfirmation(draft))...)
	gaps = append(gaps, semanticGaps(profilechecks.CheckOutputs(draft))...)
	gaps = append(gaps, semanticGaps(profilechecks.CheckCSSFallbacks(draft))...)

	// Check for ambiguous locators in evidence.
	for _, rec := range records {
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

func semanticGaps(issues []profilechecks.Issue) []Gap {
	gaps := make([]Gap, 0, len(issues))
	for _, issue := range issues {
		gaps = append(gaps, Gap{
			Kind:    gapKind(issue.Rule),
			Field:   issue.Field,
			Message: issue.Message,
		})
	}
	return gaps
}

func gapKind(rule profilechecks.Rule) GapKind {
	switch rule {
	case profilechecks.RuleInvalidOutputShape:
		return GapUnresolvedOutput
	case profilechecks.RuleCSSMissingFallback:
		return GapCSSFallbackReason
	case profilechecks.RuleSideEffectNoConfirm:
		return GapMissingConfirmation
	default:
		return GapKind(rule)
	}
}
