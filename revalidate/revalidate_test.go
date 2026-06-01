package revalidate

import (
	"testing"

	"github.com/OpenUdon/browsertools/evidence"
)

// baseProfile returns a minimal valid profile map with one read-only action.
func baseProfile() map[string]any {
	return map[string]any{
		"profile": "uws.browser.1.5",
		"info": map[string]any{
			"title":  "Test",
			"origin": "https://example.test",
		},
		"observationKind": "accessibility_snapshot",
		"evidence":        map[string]any{"learnedAt": "2099-01-01T00:00:00Z"},
		"confidence":      "medium",
		"expiresAfter":    "P30D",
		"verification": map[string]any{
			"lastVerifiedAt": "2099-01-01T00:00:00Z",
			"successfulRuns": 1,
		},
		"actions": map[string]any{
			"read_status": map[string]any{
				"sequence":           []any{map[string]any{"navigate": "/"}},
				"sideEffects":        []any{"read_only"},
				"confirmationPolicy": map[string]any{"required": false},
			},
		},
	}
}

func baseRecord() evidence.Record {
	return evidence.Record{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
		CandidateLocators: []evidence.CandidateLocator{
			{Role: "status", Name: "OK"},
		},
	}
}

// TestCheckClean passes with a consistent profile and matching evidence.
func TestCheckClean(t *testing.T) {
	r := Check(baseProfile(), []evidence.Record{baseRecord()})
	if !r.OK {
		t.Errorf("expected OK=true, got failures: %+v", r.Failures)
	}
}

// TestCheckOriginMismatch detects evidence from a non-allowlisted origin.
func TestCheckOriginMismatch(t *testing.T) {
	rec := baseRecord()
	rec.Origin = "https://evil.test"
	r := Check(baseProfile(), []evidence.Record{rec})
	if r.OK {
		t.Error("expected OK=false for origin mismatch")
	}
	found := false
	for _, f := range r.Failures {
		if f.Kind == CheckOriginMismatch {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CheckOriginMismatch failure, got: %+v", r.Failures)
	}
}

// TestCheckMissingLocator detects an action with no candidate locators.
func TestCheckMissingLocator(t *testing.T) {
	rec := baseRecord()
	rec.CandidateLocators = nil
	r := Check(baseProfile(), []evidence.Record{rec})
	if r.OK {
		t.Error("expected OK=false for missing locator")
	}
	found := false
	for _, f := range r.Failures {
		if f.Kind == CheckMissingLocator {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CheckMissingLocator failure, got: %+v", r.Failures)
	}
}

// TestCheckExpired detects a profile whose lastVerifiedAt + expiresAfter has elapsed.
func TestCheckExpired(t *testing.T) {
	prof := baseProfile()
	prof["verification"].(map[string]any)["lastVerifiedAt"] = "2020-01-01T00:00:00Z"
	r := Check(prof, nil)
	if r.OK {
		t.Error("expected OK=false for expired profile")
	}
	found := false
	for _, f := range r.Failures {
		if f.Kind == CheckExpired {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CheckExpired failure, got: %+v", r.Failures)
	}
}

// TestCheckCSSMissingFallback detects a css output without fallbackReason.
func TestCheckCSSMissingFallback(t *testing.T) {
	prof := baseProfile()
	prof["actions"].(map[string]any)["read_status"].(map[string]any)["outputs"] = map[string]any{
		"items": map[string]any{
			"type":     "array",
			"source":   "css",
			"selector": ".items li",
			// fallbackReason absent
		},
	}
	r := Check(prof, nil)
	if r.OK {
		t.Error("expected OK=false for CSS missing fallback")
	}
	found := false
	for _, f := range r.Failures {
		if f.Kind == CheckCSSMissingFallback {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CheckCSSMissingFallback failure, got: %+v", r.Failures)
	}
}

// TestCheckSideEffectNoConfirmation detects a write action with required=false.
func TestCheckSideEffectNoConfirmation(t *testing.T) {
	prof := baseProfile()
	prof["actions"].(map[string]any)["read_status"].(map[string]any)["sideEffects"] = []any{"creates_record"}
	prof["actions"].(map[string]any)["read_status"].(map[string]any)["confirmationPolicy"] = map[string]any{"required": false}
	r := Check(prof, nil)
	if r.OK {
		t.Error("expected OK=false for side effect without confirmation")
	}
	found := false
	for _, f := range r.Failures {
		if f.Kind == CheckSideEffectNoConfirm {
			found = true
		}
	}
	if !found {
		t.Errorf("expected CheckSideEffectNoConfirm failure, got: %+v", r.Failures)
	}
}

// TestCheckFailuresSorted verifies the Failures slice is sorted by (Kind, Field).
func TestCheckFailuresSorted(t *testing.T) {
	prof := baseProfile()
	// Trigger both an expired and a side-effect failure.
	prof["verification"].(map[string]any)["lastVerifiedAt"] = "2020-01-01T00:00:00Z"
	prof["actions"].(map[string]any)["read_status"].(map[string]any)["sideEffects"] = []any{"creates_record"}
	prof["actions"].(map[string]any)["read_status"].(map[string]any)["confirmationPolicy"] = map[string]any{"required": false}
	r := Check(prof, nil)
	for i := 1; i < len(r.Failures); i++ {
		a, b := r.Failures[i-1], r.Failures[i]
		if string(a.Kind) > string(b.Kind) {
			t.Errorf("failures not sorted: %q > %q", a.Kind, b.Kind)
		}
	}
}

// TestLiveRevalidatorReturnsError confirms LiveRevalidator never runs.
func TestLiveRevalidatorReturnsError(t *testing.T) {
	lr := LiveRevalidator{}
	_, err := lr.Revalidate(baseProfile(), nil)
	if err == nil {
		t.Error("expected ErrLiveNotSupported, got nil")
	}
	if err != ErrLiveNotSupported {
		t.Errorf("expected ErrLiveNotSupported, got: %v", err)
	}
}

// TestFixtureRevalidatorInterface confirms FixtureRevalidator satisfies the interface.
func TestFixtureRevalidatorInterface(t *testing.T) {
	var r Revalidator = FixtureRevalidator{}
	result, err := r.Revalidate(baseProfile(), []evidence.Record{baseRecord()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Errorf("expected OK, got failures: %+v", result.Failures)
	}
}
