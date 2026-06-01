package review

import (
	"testing"

	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
)

func baseDraft(t *testing.T) map[string]any {
	t.Helper()
	records := []evidence.Record{{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}}
	result, err := draft.Build(records, draft.Options{
		Info:            draft.ProfileInfo{Title: "Test", Origin: "https://example.test"},
		ObservationKind: "accessibility_snapshot",
		Confidence:      "medium",
		ExpiresAfter:    "P30D",
	})
	if err != nil {
		t.Fatalf("draft.Build: %v", err)
	}
	// Override verification dates to a recent timestamp so expiry checks pass.
	d := result.Draft
	d["evidence"].(map[string]any)["learnedAt"] = "2099-01-01T00:00:00Z"
	d["verification"].(map[string]any)["lastVerifiedAt"] = "2099-01-01T00:00:00Z"
	return d
}

// TestBuildValid confirms a clean draft produces a valid bundle with no gaps.
func TestBuildValid(t *testing.T) {
	records := []evidence.Record{{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}}
	d := baseDraft(t)
	b := Build(d, records)
	if !b.Validation.Valid {
		t.Errorf("expected valid, got errors: %v", b.Validation.Errors)
	}
	if len(b.Gaps) > 0 {
		t.Errorf("expected no gaps, got: %+v", b.Gaps)
	}
	if b.Evidence.RecordCount != 1 {
		t.Errorf("expected 1 record in evidence summary, got %d", b.Evidence.RecordCount)
	}
}

// TestBuildMissingConfirmationGap detects a write action without required confirmation.
func TestBuildMissingConfirmationGap(t *testing.T) {
	d := baseDraft(t)
	// Inject a write side effect with required=false
	actions := d["actions"].(map[string]any)
	actions["read_status"].(map[string]any)["sideEffects"] = []any{"state_change"}
	actions["read_status"].(map[string]any)["confirmationPolicy"] = map[string]any{"required": false}

	b := Build(d, nil)
	found := false
	for _, g := range b.Gaps {
		if g.Kind == GapMissingConfirmation {
			found = true
		}
	}
	if !found {
		t.Error("expected GapMissingConfirmation gap, got none")
	}
}

// TestBuildExpiredEvidenceGap detects a profile whose lastVerifiedAt + expiresAfter is in the past.
func TestBuildExpiredEvidenceGap(t *testing.T) {
	d := baseDraft(t)
	// Set lastVerifiedAt well in the past so P30D has definitely elapsed.
	d["verification"].(map[string]any)["lastVerifiedAt"] = "2020-01-01T00:00:00Z"
	b := Build(d, nil)
	found := false
	for _, g := range b.Gaps {
		if g.Kind == GapExpiredEvidence {
			found = true
		}
	}
	if !found {
		t.Error("expected GapExpiredEvidence gap, got none")
	}
}

// TestBuildCSSFallbackGap detects a CSS output missing fallbackReason.
func TestBuildCSSFallbackGap(t *testing.T) {
	d := baseDraft(t)
	actions := d["actions"].(map[string]any)
	action := actions["read_status"].(map[string]any)
	action["outputs"] = map[string]any{
		"items": map[string]any{
			"type":   "array",
			"source": "css",
			// selector present, fallbackReason absent — should trigger gap
			"selector":   ".item-list li",
			"validation": map[string]any{"type": "array"},
		},
	}
	b := Build(d, nil)
	found := false
	for _, g := range b.Gaps {
		if g.Kind == GapCSSFallbackReason {
			found = true
		}
	}
	if !found {
		t.Error("expected GapCSSFallbackReason gap, got none")
	}
}

// TestBuildAmbiguousLocatorGap detects ambiguous locators in evidence.
func TestBuildAmbiguousLocatorGap(t *testing.T) {
	d := baseDraft(t)
	records := []evidence.Record{{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
		CandidateLocators: []evidence.CandidateLocator{
			{Role: "button", Name: "OK", AmbiguityNote: "two buttons match"},
			{Role: "button", Name: "OK"},
		},
	}}
	b := Build(d, records)
	found := false
	for _, g := range b.Gaps {
		if g.Kind == GapAmbiguousLocator {
			found = true
		}
	}
	if !found {
		t.Error("expected GapAmbiguousLocator gap, got none")
	}
}

// TestBuildDeterministic confirms identical inputs produce identical bundles.
func TestBuildDeterministic(t *testing.T) {
	records := []evidence.Record{{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}}
	d := baseDraft(t)
	b1 := Build(d, records)
	b2 := Build(d, records)
	if b1.ConfidenceRationale != b2.ConfidenceRationale {
		t.Error("non-deterministic confidence rationale")
	}
	if len(b1.Gaps) != len(b2.Gaps) {
		t.Errorf("non-deterministic gap count: %d vs %d", len(b1.Gaps), len(b2.Gaps))
	}
}

// TestBuildSideEffectSummary checks that write actions are detected.
func TestBuildSideEffectSummary(t *testing.T) {
	d := baseDraft(t)
	// Make the action a write action with required=true
	actions := d["actions"].(map[string]any)
	actions["read_status"].(map[string]any)["sideEffects"] = []any{"creates_record"}
	actions["read_status"].(map[string]any)["confirmationPolicy"] = map[string]any{"required": true}
	b := Build(d, nil)
	if !b.SideEffects.HasWriteActions {
		t.Error("expected HasWriteActions=true")
	}
	if len(b.SideEffects.ActionsRequiringConfirmation) == 0 {
		t.Error("expected ActionsRequiringConfirmation to be non-empty")
	}
}
