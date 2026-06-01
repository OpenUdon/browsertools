// Package firecrawl imports Firecrawl scrape/crawl/map/extract/interact output
// as candidate evidence.
//
// A Firecrawl fixture is a JSON object with this shape:
//
//	{
//	  "url": "https://example.test/page",
//	  "observedAt": "2026-01-01T00:00:00Z",
//	  "actionHint": "optional_name",
//	  "scrapeId": "fc-abc123",
//	  "markdown": "## Page Content\n...",
//	  "extract": {
//	    "title": "Example Page",
//	    "price": "9.99"
//	  },
//	  "links": ["https://example.test/a", "https://example.test/b"]
//	}
//
// Firecrawl-specific identifiers (scrapeId, jobId, crawlId) stay
// adapter-private and are never included in the emitted evidence record.
// Extract fields become CandidateOutputs with source="microdata" as a
// best-effort guess; the reviewer must confirm the actual source.
package firecrawl

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
)

// Fixture is the expected shape of a saved Firecrawl API response file.
// Firecrawl-specific IDs are parsed but deliberately excluded from evidence.
type Fixture struct {
	URL        string `json:"url"`
	ObservedAt string `json:"observedAt,omitempty"`
	ActionHint string `json:"actionHint,omitempty"`
	// ScrapeID, JobID are parsed only to confirm the fixture is a Firecrawl
	// artifact; they are never written to the normalized evidence record.
	ScrapeID string         `json:"scrapeId,omitempty"`
	JobID    string         `json:"jobId,omitempty"`
	Markdown string         `json:"markdown,omitempty"`
	Extract  map[string]any `json:"extract,omitempty"`
	Links    []string       `json:"links,omitempty"`
}

// Adapter imports Firecrawl output as candidate evidence.
type Adapter struct{}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return "firecrawl" }

// Import parses a saved Firecrawl fixture and returns normalized evidence.
// Firecrawl API identifiers (scrapeId, jobId) are deliberately excluded from
// the produced record so they do not become portable profile fields.
func (a *Adapter) Import(raw []byte, opts adapter.Options) ([]evidence.Record, error) {
	if opts.Origin == "" {
		return nil, fmt.Errorf("firecrawl: opts.Origin is required")
	}
	status := opts.RedactionStatus
	if status == "" {
		return nil, fmt.Errorf("firecrawl: opts.RedactionStatus is required; set evidence.RedactionNotRequired for synthetic fixtures")
	}

	var fix Fixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		return nil, fmt.Errorf("firecrawl: parse fixture: %w", err)
	}
	if err := adapter.ValidateFixtureOrigin("firecrawl", fix.URL, opts.Origin); err != nil {
		return nil, err
	}

	observedAt := fix.ObservedAt
	if observedAt == "" {
		return nil, fmt.Errorf("firecrawl: observedAt is required")
	} else {
		t, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			return nil, fmt.Errorf("firecrawl: observedAt %q is not RFC-3339: %w", observedAt, err)
		}
		observedAt = t.UTC().Format(time.RFC3339)
	}

	actionHint := opts.ActionHint
	if actionHint == "" {
		actionHint = fix.ActionHint
	}

	var outputs []evidence.CandidateOutput
	for key, val := range fix.Extract {
		typ := inferType(val)
		outputs = append(outputs, evidence.CandidateOutput{
			Key:    key,
			Type:   typ,
			Source: "microdata", // best-effort; reviewer must confirm
		})
	}
	evidence.SortOutputs(outputs)

	var diags []evidence.Diagnostic
	if fix.ScrapeID != "" || fix.JobID != "" {
		diags = append(diags, evidence.Diagnostic{
			Level:   "info",
			Message: "firecrawl scrapeId/jobId excluded from evidence; they are service-specific and not portable profile fields",
		})
	}
	diags = append(diags, evidence.Diagnostic{
		Level:   "info",
		Message: "candidate outputs inferred from firecrawl extract; source and locator must be validated by reviewer",
	})
	evidence.SortDiagnostics(diags)

	raw2 := &evidence.RawRecord{
		Record: evidence.Record{
			Origin:           opts.Origin,
			ObservationKind:  evidence.ObservationDOMText,
			ObservedAt:       observedAt,
			ActionHint:       actionHint,
			CandidateOutputs: outputs,
			RedactionStatus:  status,
			RedactedFields:   opts.RedactedFields,
			Diagnostics:      diags,
			Provenance:       evidence.Provenance{Tool: "firecrawl"},
		},
	}
	rec, err := raw2.Normalize()
	if err != nil {
		return nil, fmt.Errorf("firecrawl: normalize: %w", err)
	}
	return []evidence.Record{rec}, nil
}

// inferType maps a Go value to a JSON Schema type string.
// json.Unmarshal into map[string]any represents all JSON numbers as float64;
// there is no int/int64 case after standard JSON decoding.
func inferType(v any) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case nil:
		return "null"
	default:
		return "string"
	}
}
