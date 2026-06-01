// Package crawl4ai imports Crawl4AI crawl/scrape/extraction output as evidence.
//
// A Crawl4AI fixture is a JSON object with this shape:
//
//	{
//	  "url": "https://example.test/page",
//	  "observedAt": "2026-01-01T00:00:00Z",
//	  "actionHint": "optional_name",
//	  "markdown": "## Page Title\n...",
//	  "extracted": [
//	    { "key": "title", "type": "string", "selector": "h1", "value": "Example" }
//	  ]
//	}
//
// CSS/XPath selectors from extracted items are recorded as CandidateOutputs
// with source="css" (fallback evidence only). They MUST NOT become portable
// action locators. The reviewer must replace them with a11y/microdata sources
// or add explicit fallbackReason and validation fields.
package crawl4ai

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
)

// ExtractedItem is one extracted field from a Crawl4AI result.
type ExtractedItem struct {
	Key      string `json:"key"`
	Type     string `json:"type,omitempty"`
	Selector string `json:"selector,omitempty"` // CSS selector used by Crawl4AI
	Value    string `json:"value,omitempty"`    // Extracted value (for reference only)
}

// Fixture is the expected shape of a saved Crawl4AI output file.
type Fixture struct {
	URL        string          `json:"url"`
	ObservedAt string          `json:"observedAt,omitempty"`
	ActionHint string          `json:"actionHint,omitempty"`
	Markdown   string          `json:"markdown,omitempty"`
	Extracted  []ExtractedItem `json:"extracted,omitempty"`
}

// Adapter imports Crawl4AI output as candidate evidence.
type Adapter struct{}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return "crawl4ai" }

// Import parses a saved Crawl4AI fixture and returns normalized evidence.
// CSS selectors are recorded as output fallback evidence only; they must not
// be used as action locators. A diagnostic is added to remind the reviewer.
func (a *Adapter) Import(raw []byte, opts adapter.Options) ([]evidence.Record, error) {
	if opts.Origin == "" {
		return nil, fmt.Errorf("crawl4ai: opts.Origin is required")
	}
	status := opts.RedactionStatus
	if status == "" {
		return nil, fmt.Errorf("crawl4ai: opts.RedactionStatus is required; set evidence.RedactionNotRequired for synthetic fixtures")
	}

	var fix Fixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		return nil, fmt.Errorf("crawl4ai: parse fixture: %w", err)
	}

	observedAt := fix.ObservedAt
	if observedAt == "" {
		observedAt = time.Now().UTC().Format(time.RFC3339)
	} else {
		t, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			return nil, fmt.Errorf("crawl4ai: observedAt %q is not RFC-3339: %w", observedAt, err)
		}
		observedAt = t.UTC().Format(time.RFC3339)
	}

	actionHint := opts.ActionHint
	if actionHint == "" {
		actionHint = fix.ActionHint
	}

	var outputs []evidence.CandidateOutput
	for _, item := range fix.Extracted {
		typ := item.Type
		if typ == "" {
			typ = "string"
		}
		out := evidence.CandidateOutput{
			Key:      item.Key,
			Type:     typ,
			Source:   "css",
			Selector: item.Selector,
			// FallbackReason is intentionally omitted here — the reviewer must
			// supply it based on why a11y/microdata cannot be used.
		}
		outputs = append(outputs, out)
	}
	evidence.SortOutputs(outputs)

	diags := []evidence.Diagnostic{{
		Level:   "warn",
		Message: "CSS selectors from crawl4ai are output fallback evidence only; do not use as action locators; reviewer must add fallbackReason and prefer a11y/microdata where available",
	}}

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
			Provenance:       evidence.Provenance{Tool: "crawl4ai"},
		},
	}
	rec, err := raw2.Normalize()
	if err != nil {
		return nil, fmt.Errorf("crawl4ai: normalize: %w", err)
	}
	return []evidence.Record{rec}, nil
}
