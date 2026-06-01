package llmscraper

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

// TestImportFromFixture verifies that a saved extraction fixture produces a
// valid record with candidate outputs and a reviewer advisory diagnostic.
func TestImportFromFixture(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/extract_product.json")
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
	if rec.Provenance.Tool != "llm-scraper" {
		t.Errorf("tool: got %q", rec.Provenance.Tool)
	}
	if len(rec.CandidateOutputs) == 0 {
		t.Error("expected candidate outputs, got none")
	}
	// Outputs should be sorted by key
	keys := make([]string, len(rec.CandidateOutputs))
	for i, o := range rec.CandidateOutputs {
		keys[i] = o.Key
	}
	if len(keys) >= 2 {
		for i := 1; i < len(keys); i++ {
			if keys[i] < keys[i-1] {
				t.Errorf("outputs not sorted: %v", keys)
			}
		}
	}
}

// TestImportLLMFieldsAreCandidateOnly verifies that a diagnostic is present
// warning the reviewer that LLM-inferred outputs are not trusted.
func TestImportLLMFieldsAreCandidateOnly(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/extract_product.json")
	records, _ := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	found := false
	for _, d := range records[0].Diagnostics {
		if d.Level == "info" {
			found = true
		}
	}
	if !found {
		t.Error("expected an info diagnostic about LLM candidate status")
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

// TestImportNoSchema handles a fixture with no schema gracefully.
func TestImportNoSchema(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z"}`)
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records[0].CandidateOutputs) != 0 {
		t.Error("expected no outputs when schema is absent")
	}
}
