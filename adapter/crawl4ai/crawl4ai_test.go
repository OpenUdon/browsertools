package crawl4ai

import (
	"os"
	"testing"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
)

func fixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

// TestImportFromFixture verifies a saved crawl fixture produces a record with
// CSS candidate outputs and a warn diagnostic.
func TestImportFromFixture(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/crawl_catalogue.json")
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	rec := records[0]
	if rec.Provenance.Tool != "crawl4ai" {
		t.Errorf("tool: got %q", rec.Provenance.Tool)
	}
	if len(rec.CandidateOutputs) != 2 {
		t.Errorf("expected 2 candidate outputs, got %d", len(rec.CandidateOutputs))
	}
	for _, out := range rec.CandidateOutputs {
		if out.Source != "" {
			t.Errorf("incomplete extraction must remain unbound, got source=%q for key %q", out.Source, out.Key)
		}
		// FallbackReason should be empty — reviewer fills it in
		if out.FallbackReason != "" {
			t.Errorf("fallbackReason should be empty from adapter, got %q", out.FallbackReason)
		}
	}
}

// TestImportCSSSelectorsNotActionLocators verifies that CSS selectors are only
// in CandidateOutputs, never in CandidateLocators.
func TestImportCSSSelectorsNotActionLocators(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/crawl_catalogue.json")
	records, _ := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if len(records[0].CandidateLocators) != 0 {
		t.Error("CSS selectors must not appear as CandidateLocators")
	}
}

// TestImportWarnDiagnostic confirms a warning diagnostic is present.
func TestImportWarnDiagnostic(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/crawl_catalogue.json")
	records, _ := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	found := false
	for _, d := range records[0].Diagnostics {
		if d.Level == "warn" {
			found = true
		}
	}
	if !found {
		t.Error("expected a warn diagnostic about CSS selectors, got none")
	}
}

// TestImportMissingOriginError returns an error when Origin is not set.
func TestImportMissingOriginError(t *testing.T) {
	a := &Adapter{}
	_, err := a.Import([]byte(`{}`), adapter.Options{})
	if err == nil {
		t.Error("expected error for missing origin, got nil")
	}
}

func TestImportMissingObservedAtError(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{"url":"https://example.test/catalogue"}`)
	_, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err == nil {
		t.Fatal("expected error for missing observedAt, got nil")
	}
}

func TestImportFixtureOriginSafety(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing url", raw: `{"observedAt":"2026-01-01T00:00:00Z"}`},
		{name: "malformed url", raw: `{"url":"://bad","observedAt":"2026-01-01T00:00:00Z"}`},
		{name: "unsupported scheme", raw: `{"url":"ftp://example.test/catalogue","observedAt":"2026-01-01T00:00:00Z"}`},
		{name: "origin mismatch", raw: `{"url":"https://other.test/catalogue","observedAt":"2026-01-01T00:00:00Z"}`},
	}
	a := &Adapter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.Import([]byte(tt.raw), adapter.Options{
				Origin:          "https://example.test",
				RedactionStatus: evidence.RedactionNotRequired,
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}
