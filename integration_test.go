// Package browsertools_test contains integration tests that exercise the full
// adapter → draft → review pipeline end-to-end using synthetic fixtures.
package browsertools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
)

// TestEndToEndPipeline exercises the full adapter→draft→review pipeline with
// a synthetic Playwright snapshot fixture. It asserts that the resulting bundle
// passes validation and has no gaps.
func TestEndToEndPipeline(t *testing.T) {
	// 1. Build a synthetic Playwright snapshot fixture in-memory.
	// Single interactive child to avoid the ambiguous-locator check.
	fix := playwright.Fixture{
		URL:        "https://example.test/status",
		ObservedAt: "2099-01-01T00:00:00Z",
		ActionHint: "read_status",
		Snapshot: &playwright.SnapshotNode{
			Role: "region",
			Name: "Main",
			Children: []*playwright.SnapshotNode{
				{Role: "status", Name: "All systems operational"},
			},
		},
	}
	raw, err := json.Marshal(fix)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	// 2. Import via the Playwright adapter.
	a := &playwright.Adapter{}
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatalf("adapter import: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one record")
	}

	// 3. Build a draft profile.
	result, err := draft.Build(records, draft.Spec{
		Info:            profile.Info{Title: "Example", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Confidence:      profile.ConfidenceMedium,
		ExpiresAfter:    "P30D",
		Actions: map[string]draft.ActionSpec{"read_status": {
			Sequence: []profile.Step{
				{Kind: profile.StepNavigate, Navigate: "/status"},
				{Kind: profile.StepWaitFor, WaitFor: &profile.WaitForCondition{Locator: &profile.Locator{Role: "status", Name: "All systems operational"}}},
			},
			SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
			ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
		}},
	})
	if err != nil {
		t.Fatalf("draft.Build: %v", err)
	}
	if !result.ReadyForReview() {
		t.Errorf("draft diagnostics: %v", result.Diagnostics)
	}

	// 4. Build a review bundle and assert it is promotable.
	bundle, err := review.Build(result.Profile, records, result.Decisions, time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("review.Build: %v", err)
	}
	if !bundle.Validation.Valid {
		t.Errorf("bundle validation failed: %v", bundle.Validation.Errors)
	}
	// The synthetic fixture uses ObservedAt=2099-01-01 and draft.Build sets
	// verification.lastVerifiedAt to that same date, so no expiry gap is expected.
	if len(bundle.Gaps) != 0 {
		for _, g := range bundle.Gaps {
			t.Errorf("unexpected gap: %s at %s — %s", g.Kind, g.Field, g.Message)
		}
	}
	if !bundle.Promotable() {
		t.Error("expected bundle to be promotable")
	}
}

// TestOpenUdonBundleVerify proves the committed handoff bundle is bound to the
// exact profile and normalized evidence shipped with the example.
func TestOpenUdonBundleVerify(t *testing.T) {
	prof, err := profile.LoadFile("examples/wikipedia-lookup/browser-profiles/wikipedia.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evidenceData, err := os.ReadFile("examples/openudon-binding/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []evidence.Record
	if err := json.Unmarshal(evidenceData, &records); err != nil {
		t.Fatal(err)
	}
	bundleData, err := os.ReadFile("examples/openudon-binding/review-bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	var bundle review.Bundle
	if err := json.Unmarshal(bundleData, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := review.Verify(&bundle, prof, records, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("verify OpenUdon handoff bundle: %v", err)
	}
}

// TestExampleProfilesValidate asserts that all committed example browser-profile
// YAML files pass schema validation. This acts as a compatibility pin: any future
// schema change or draft builder regression that breaks existing profiles will
// fail here.
func TestExampleProfilesValidate(t *testing.T) {
	patterns := []string{
		"examples/*/browser-profiles/*.yaml",
		"examples/wrapper-service/browser-profile.yaml",
	}
	found := 0
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			t.Fatalf("glob %q: %v", pat, err)
		}
		for _, path := range matches {
			found++
			t.Run(filepath.Base(path), func(t *testing.T) {
				if _, err := profile.LoadFile(path); err != nil {
					t.Errorf("profile failed validation: %v", err)
				}
			})
		}
	}
	if found == 0 {
		t.Skip("no example profiles found")
	}
}
