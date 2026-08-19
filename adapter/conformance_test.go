package adapter_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/adapter/crawl4ai"
	"github.com/OpenUdon/browsertools/adapter/firecrawl"
	"github.com/OpenUdon/browsertools/adapter/llmscraper"
	"github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/evidence"
)

type adapterCase struct {
	name        string
	adapter     adapter.Adapter
	valid       []byte
	maximum     int
	cardinality func(*testing.T) []byte
	wantSource  string
}

func adapterCases() []adapterCase {
	const prefix = `"url":"https://example.test/page","observedAt":"2026-01-01T00:00:00Z"`
	return []adapterCase{
		{
			name: "playwright", adapter: &playwright.Adapter{}, maximum: 8 << 20, wantSource: "jsonld",
			valid: []byte(`{` + prefix + `,"structuredData":[{"title":"Example"}]}`),
			cardinality: func(t *testing.T) []byte {
				documents := make([]json.RawMessage, 33)
				for index := range documents {
					documents[index] = json.RawMessage(`{"title":"Example"}`)
				}
				return mustJSON(t, playwright.Fixture{URL: "https://example.test/page", ObservedAt: "2026-01-01T00:00:00Z", StructuredData: documents})
			},
		},
		{
			name: "llm-scraper", adapter: &llmscraper.Adapter{}, maximum: 2 << 20,
			valid: []byte(`{` + prefix + `,"schema":{"type":"object","properties":{"title":{"type":"string"}}}}`),
			cardinality: func(t *testing.T) []byte {
				properties := make(map[string]any, 257)
				for index := 0; index < 257; index++ {
					properties[string(rune(0x1000+index))] = map[string]any{"type": "string"}
				}
				return mustJSON(t, llmscraper.Fixture{URL: "https://example.test/page", ObservedAt: "2026-01-01T00:00:00Z", Schema: map[string]any{"properties": properties}})
			},
		},
		{
			name: "crawl4ai", adapter: &crawl4ai.Adapter{}, maximum: 4 << 20,
			valid: []byte(`{` + prefix + `,"extracted":[{"key":"title","type":"string","selector":"h1"}]}`),
			cardinality: func(t *testing.T) []byte {
				items := make([]crawl4ai.ExtractedItem, 257)
				for index := range items {
					items[index] = crawl4ai.ExtractedItem{Key: string(rune(0x1000 + index)), Type: "string"}
				}
				return mustJSON(t, crawl4ai.Fixture{URL: "https://example.test/page", ObservedAt: "2026-01-01T00:00:00Z", Extracted: items})
			},
		},
		{
			name: "firecrawl", adapter: &firecrawl.Adapter{}, maximum: 4 << 20,
			valid: []byte(`{` + prefix + `,"extract":{"title":"Example"}}`),
			cardinality: func(t *testing.T) []byte {
				extract := make(map[string]any, 257)
				for index := 0; index < 257; index++ {
					extract[string(rune(0x1000+index))] = "value"
				}
				return mustJSON(t, firecrawl.Fixture{URL: "https://example.test/page", ObservedAt: "2026-01-01T00:00:00Z", Extract: extract})
			},
		},
	}
}

func TestSavedAdapterBoundaryConformance(t *testing.T) {
	options := adapter.Options{Origin: "https://example.test", RedactionStatus: evidence.RedactionNotRequired}
	for _, test := range adapterCases() {
		t.Run(test.name, func(t *testing.T) {
			unknown := append([]byte(nil), test.valid[:len(test.valid)-1]...)
			unknown = append(unknown, []byte(`,"unexpected":true}`)...)
			for name, fixture := range map[string][]byte{
				"oversized":   bytes.Repeat([]byte{' '}, test.maximum+1),
				"unknown":     unknown,
				"trailing":    append(append([]byte(nil), test.valid...), []byte(` {}`)...),
				"incomplete":  []byte(`{"url":`),
				"cardinality": test.cardinality(t),
			} {
				t.Run(name, func(t *testing.T) {
					if _, err := test.adapter.Import(fixture, options); err == nil {
						t.Fatal("expected strict fixture rejection")
					}
				})
			}
		})
	}
}

func TestSavedAdapterOutputProvenanceIsHonest(t *testing.T) {
	options := adapter.Options{Origin: "https://example.test", RedactionStatus: evidence.RedactionNotRequired}
	for _, test := range adapterCases() {
		t.Run(test.name, func(t *testing.T) {
			records, err := test.adapter.Import(test.valid, options)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].Provenance.Tool != test.adapter.Name() || len(records[0].CandidateOutputs) != 1 {
				t.Fatalf("record provenance/output mismatch: %#v", records)
			}
			if source := records[0].CandidateOutputs[0].Source; source != test.wantSource {
				t.Fatalf("output source=%q, want %q", source, test.wantSource)
			}
		})
	}
}

func TestSavedAdaptersEmitCanonicalAllowedOrigin(t *testing.T) {
	options := adapter.Options{Origin: "https://EXAMPLE.test:443/", RedactionStatus: evidence.RedactionNotRequired}
	for _, test := range adapterCases() {
		t.Run(test.name, func(t *testing.T) {
			records, err := test.adapter.Import(test.valid, options)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].Origin != "https://example.test" {
				t.Fatalf("canonical record origin = %#v", records)
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
