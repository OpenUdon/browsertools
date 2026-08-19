package firecrawl

import (
	"os"
	"strings"
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

// TestImportFromFixture verifies a saved Firecrawl fixture produces a
// normalized record with candidate outputs.
func TestImportFromFixture(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/scrape_article.json")
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
	if rec.Provenance.Tool != "firecrawl" {
		t.Errorf("tool: got %q", rec.Provenance.Tool)
	}
	if len(rec.CandidateOutputs) == 0 {
		t.Error("expected candidate outputs from extract field")
	}
	for _, output := range rec.CandidateOutputs {
		if output.Source != "" {
			t.Fatalf("Firecrawl hint fabricated portable source %q", output.Source)
		}
	}
}

// TestImportScrapeIDExcluded confirms scrapeId and jobId are not in the
// produced evidence record.
func TestImportScrapeIDExcluded(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/scrape_article.json")
	records, _ := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	data, _ := evidence.MarshalDeterministic(records[0])
	if strings.Contains(string(data), "fc-abc123") {
		t.Error("scrapeId fc-abc123 leaked into the evidence record")
	}
}

// TestImportDiagnosticAboutIDs confirms an info diagnostic is present when
// Firecrawl IDs are present in the fixture.
func TestImportDiagnosticAboutIDs(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/scrape_article.json")
	records, _ := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	found := false
	for _, d := range records[0].Diagnostics {
		if d.Level == "info" && strings.Contains(d.Message, "scrapeId") {
			found = true
		}
	}
	if !found {
		t.Error("expected info diagnostic about excluded Firecrawl IDs")
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

// TestImportMissingRedactionStatusError returns an error when RedactionStatus is not set.
func TestImportMissingRedactionStatusError(t *testing.T) {
	a := &Adapter{}
	_, err := a.Import([]byte(`{}`), adapter.Options{Origin: "https://example.test"})
	if err == nil {
		t.Error("expected error for missing redaction status, got nil")
	}
}

func TestImportMissingObservedAtError(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{"url":"https://example.test/article/42"}`)
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
		{name: "unsupported scheme", raw: `{"url":"ftp://example.test/article/42","observedAt":"2026-01-01T00:00:00Z"}`},
		{name: "origin mismatch", raw: `{"url":"https://other.test/article/42","observedAt":"2026-01-01T00:00:00Z"}`},
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

// TestImportTypeInference tests that extract field types are inferred correctly.
func TestImportTypeInference(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{
		"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z",
		"extract":{"name":"Alice","active":true,"items":["a","b"],"score":9.5}
	}`)
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	typeMap := map[string]string{}
	for _, o := range records[0].CandidateOutputs {
		typeMap[o.Key] = o.Type
	}
	if typeMap["name"] != "string" {
		t.Errorf("name: expected string, got %q", typeMap["name"])
	}
	if typeMap["active"] != "boolean" {
		t.Errorf("active: expected boolean, got %q", typeMap["active"])
	}
	if typeMap["items"] != "array" {
		t.Errorf("items: expected array, got %q", typeMap["items"])
	}
	if typeMap["score"] != "number" {
		t.Errorf("score: expected number, got %q", typeMap["score"])
	}
}
