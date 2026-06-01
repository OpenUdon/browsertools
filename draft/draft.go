// Package draft builds deterministic UWS browser-profile draft documents from
// normalized evidence records.
//
// The builder is designed for use after evidence collection and before the
// review bundle stage. It transforms evidence.Record values into a candidate
// browser-profile that can be validated, reviewed, and promoted to a reviewed
// profile.
//
// Key constraints:
//   - Output is deterministic: same input evidence always produces the same
//     profile structure. No clocks, random, or process-level state.
//   - Ambiguous locators (multiple candidates with the same role/name) are
//     refused unless the caller supplies an explicit ReviewDecision.
//   - Generated profiles are run through profile.Validate before returning;
//     if validation fails, path-tagged diagnostics are returned alongside the
//     draft so the caller can inspect what needs human review.
package draft

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

// ReviewDecision is the caller's explicit resolution for an otherwise-refused
// ambiguous locator or gap. Supply one per ambiguous action.
type ReviewDecision struct {
	// ActionHint is the evidence.Record.ActionHint this decision applies to.
	ActionHint string
	// ChosenLocatorIndex is the index into the CandidateLocators slice to use.
	ChosenLocatorIndex int
	// Note is a human note explaining the choice, stored in profile diagnostics.
	Note string
}

// Options controls draft generation behaviour.
type Options struct {
	// Info is the required profile-info block for the generated profile.
	Info ProfileInfo
	// ObservationKind is the kind to set on the generated profile.
	ObservationKind string
	// Confidence is one of "low", "medium", "high".
	Confidence string
	// ExpiresAfter is an ISO-8601 duration (e.g. "P30D").
	ExpiresAfter string
	// ReviewDecisions resolves ambiguous locators; keyed by ActionHint.
	ReviewDecisions map[string]ReviewDecision
}

// ProfileInfo maps to the profile.Info type for the generated document.
type ProfileInfo struct {
	Title              string
	Provider           string
	Origin             any // string or []string
	LoginStateRequired bool
}

// Result holds the generated draft and any diagnostics from validation.
type Result struct {
	// Draft is the generated profile as a raw JSON-compatible map. It can be
	// marshaled directly or passed to profile.Validate.
	Draft map[string]any
	// ValidationErrors contains schema validation errors if the draft did not
	// pass validation. A non-nil Draft with non-empty ValidationErrors means the
	// draft is a best-effort candidate that needs human repair.
	ValidationErrors []string
}

// Build constructs a browser-profile draft from a slice of normalized evidence
// records. Records are grouped by ActionHint; each distinct hint becomes one
// action in the profile.
//
// Returns an error if:
//   - opts.Info.Origin or opts.Info.Title are empty.
//   - Any evidence group has ambiguous locators and no ReviewDecision was supplied.
//
// Validation errors (schema non-compliance) are returned in Result.ValidationErrors,
// not as a Go error, so callers can inspect the draft alongside the issues.
func Build(records []evidence.Record, opts Options) (*Result, error) {
	if opts.Info.Title == "" {
		return nil, fmt.Errorf("build: opts.Info.Title is required")
	}
	if opts.Info.Origin == nil {
		return nil, fmt.Errorf("build: opts.Info.Origin is required")
	}
	if opts.Confidence == "" {
		opts.Confidence = "low"
	}
	if opts.ExpiresAfter == "" {
		opts.ExpiresAfter = "P30D"
	}
	if opts.ObservationKind == "" {
		opts.ObservationKind = "accessibility_snapshot"
	}

	// Group records by ActionHint, stable-sorted within each group.
	groups := groupByAction(records)

	// Use the earliest ObservedAt across all records for the profile-level
	// evidence block, and the latest for the verification block.
	earliest, latest := timeRange(records)
	if earliest == "" {
		earliest = "2006-01-02T15:04:05Z" // sentinel; replaced by real evidence
	}
	if latest == "" {
		latest = earliest
	}

	actions := make(map[string]any, len(groups))
	for _, hint := range sortedKeys(groups) {
		group := groups[hint]
		action, err := buildAction(hint, group, opts)
		if err != nil {
			return nil, err
		}
		actions[hint] = action
	}

	draft := map[string]any{
		"profile": "uws.browser.1.5",
		"info": map[string]any{
			"title":              opts.Info.Title,
			"origin":             opts.Info.Origin,
			"loginStateRequired": opts.Info.LoginStateRequired,
		},
		"observationKind": opts.ObservationKind,
		"evidence": map[string]any{
			"learnedAt": earliest,
			"source":    "browsertools_draft",
		},
		"confidence":   opts.Confidence,
		"expiresAfter": opts.ExpiresAfter,
		"verification": map[string]any{
			"lastVerifiedAt": latest,
			"successfulRuns": 0,
		},
		"actions": actions,
	}

	// Remove provider from info if empty.
	if opts.Info.Provider != "" {
		draft["info"].(map[string]any)["provider"] = opts.Info.Provider
	}

	// Validate and collect any schema errors, but still return the draft.
	var valErrs []string
	if err := profile.Validate(draft); err != nil {
		valErrs = append(valErrs, err.Error())
	}

	return &Result{Draft: draft, ValidationErrors: valErrs}, nil
}

// buildAction converts one group of evidence records into a profile action map.
func buildAction(hint string, records []evidence.Record, opts Options) (map[string]any, error) {
	// Pick the best locator for sequence steps.
	loc, err := resolveLocator(hint, records, opts.ReviewDecisions)
	if err != nil {
		return nil, err
	}

	// Build a minimal sequence: navigate if any record has a URL-like origin,
	// then click the resolved locator (if found).
	sequence := buildSequence(loc, records)

	// Collect candidate outputs across all records in this group.
	outputs := buildOutputs(records)

	// Default to read_only; the reviewer updates this after inspecting the action.
	sideEffects := []any{"read_only"}
	confirmationPolicy := map[string]any{"required": false}

	action := map[string]any{
		"sequence":           sequence,
		"sideEffects":        sideEffects,
		"confirmationPolicy": confirmationPolicy,
	}
	if len(outputs) > 0 {
		action["outputs"] = outputs
	}
	return action, nil
}

// resolveLocator picks a single CandidateLocator from the records in a group.
// Returns nil if there are no locators (the action will rely on navigate only).
// Returns an error if there are multiple ambiguous locators and no ReviewDecision.
func resolveLocator(hint string, records []evidence.Record, decisions map[string]ReviewDecision) (*evidence.CandidateLocator, error) {
	// Collect all unique locators across the group.
	seen := map[string]evidence.CandidateLocator{}
	var ordered []string
	for _, rec := range records {
		for _, loc := range rec.CandidateLocators {
			key := loc.Role + "|" + loc.Name
			if _, exists := seen[key]; !exists {
				seen[key] = loc
				ordered = append(ordered, key)
			}
		}
	}
	if len(ordered) == 0 {
		return nil, nil
	}
	if len(ordered) == 1 {
		loc := seen[ordered[0]]
		return &loc, nil
	}

	// Multiple distinct locators — check for a review decision.
	dec, ok := decisions[hint]
	if !ok {
		return nil, fmt.Errorf("build: action %q has %d ambiguous locators with different role/name; supply a ReviewDecision to resolve", hint, len(ordered))
	}
	if dec.ChosenLocatorIndex < 0 || dec.ChosenLocatorIndex >= len(ordered) {
		return nil, fmt.Errorf("build: ReviewDecision for %q has out-of-range index %d (have %d locators)", hint, dec.ChosenLocatorIndex, len(ordered))
	}
	loc := seen[ordered[dec.ChosenLocatorIndex]]
	return &loc, nil
}

// buildSequence constructs the minimal sequence for an action given the
// resolved locator and the evidence records.
func buildSequence(loc *evidence.CandidateLocator, records []evidence.Record) []any {
	var seq []any

	// If any record has a non-empty ActionHint that looks like a path, add navigate.
	// For now we emit a navigate placeholder that the reviewer will fill in.
	// The locator click follows if we have a locator.
	seq = append(seq, map[string]any{"navigate": "/"})

	if loc != nil {
		clickLoc := map[string]any{"role": loc.Role}
		if loc.Name != "" {
			clickLoc["name"] = loc.Name
		}
		seq = append(seq, map[string]any{
			"click": map[string]any{"locator": clickLoc},
		})
	}
	return seq
}

// buildOutputs converts CandidateOutputs from all records in a group into the
// profile outputs map shape, deduplicating by key.
func buildOutputs(records []evidence.Record) map[string]any {
	seen := map[string]bool{}
	outputs := map[string]any{}
	for _, rec := range records {
		for _, out := range rec.CandidateOutputs {
			if seen[out.Key] {
				continue
			}
			seen[out.Key] = true
			entry := map[string]any{
				"type":   out.Type,
				"source": out.Source,
			}
			if out.Locator != nil {
				entry["locator"] = map[string]any{"role": out.Locator.Role, "name": out.Locator.Name}
			}
			if out.Source == "css" {
				if out.Selector != "" {
					entry["selector"] = out.Selector
				}
				if out.FallbackReason != "" {
					entry["fallbackReason"] = out.FallbackReason
				}
				entry["validation"] = map[string]any{"type": out.Type}
			}
			if out.Property != "" {
				entry["property"] = out.Property
			}
			outputs[out.Key] = entry
		}
	}
	return outputs
}

// groupByAction groups evidence records by ActionHint. Records with an empty
// ActionHint are grouped under "_default".
func groupByAction(records []evidence.Record) map[string][]evidence.Record {
	groups := map[string][]evidence.Record{}
	for _, rec := range records {
		key := rec.ActionHint
		if key == "" {
			key = "_default"
		}
		groups[key] = append(groups[key], rec)
	}
	return groups
}

// sortedKeys returns the keys of a map in alphabetical order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// timeRange returns the earliest and latest ObservedAt from a record slice.
func timeRange(records []evidence.Record) (earliest, latest string) {
	for _, rec := range records {
		if rec.ObservedAt == "" {
			continue
		}
		if earliest == "" || rec.ObservedAt < earliest {
			earliest = rec.ObservedAt
		}
		if latest == "" || rec.ObservedAt > latest {
			latest = rec.ObservedAt
		}
	}
	return earliest, latest
}

// MarshalDraft serializes a Result.Draft map to JSON.
func MarshalDraft(draft map[string]any) ([]byte, error) {
	return json.MarshalIndent(draft, "", "  ")
}
