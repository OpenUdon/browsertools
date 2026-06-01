// Package browsertools_test contains integration tests that exercise the full
// adapter → draft → review pipeline end-to-end using synthetic fixtures.
package browsertools_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

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
	result, err := draft.Build(records, draft.Options{
		Info:            draft.ProfileInfo{Title: "Example", Origin: "https://example.test"},
		ObservationKind: "accessibility_snapshot",
		Confidence:      "medium",
		ExpiresAfter:    "P30D",
	})
	if err != nil {
		t.Fatalf("draft.Build: %v", err)
	}
	if len(result.ValidationErrors) > 0 {
		t.Errorf("draft validation errors: %v", result.ValidationErrors)
	}

	// 4. Build a review bundle and assert it is promotable.
	bundle := review.Build(result.Draft, records)
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
				if err := profile.ValidateFile(path); err != nil {
					t.Errorf("profile failed validation: %v", err)
				}
			})
		}
	}
	if found == 0 {
		t.Skip("no example profiles found")
	}
}
