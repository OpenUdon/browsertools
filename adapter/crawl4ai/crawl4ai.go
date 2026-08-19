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
// CSS/XPath selectors from extracted items are recorded only as unbound hints.
// They MUST NOT become portable action locators or ready CSS outputs. An
// operator must explicitly declare a portable source and fallback rationale.
package crawl4ai

import (
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/adapterdecode"
)

const (
	maxFixtureBytes = int64(4 << 20)
	maxOutputs      = 256
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
	if err := adapterdecode.JSON(raw, maxFixtureBytes, &fix); err != nil {
		return nil, fmt.Errorf("crawl4ai: parse fixture: %w", err)
	}
	origin, err := adapter.CanonicalFixtureOrigin("crawl4ai", fix.URL, opts.Origin)
	if err != nil {
		return nil, err
	}

	observedAt := fix.ObservedAt
	if observedAt == "" {
		return nil, fmt.Errorf("crawl4ai: observedAt is required")
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
	if len(fix.Extracted) > maxOutputs {
		return nil, fmt.Errorf("crawl4ai: fixture exceeds %d extracted items", maxOutputs)
	}
	for _, item := range fix.Extracted {
		typ := item.Type
		if typ == "" {
			typ = "string"
		}
		out := evidence.CandidateOutput{
			Key: item.Key, Type: typ, Selector: item.Selector,
		}
		outputs = append(outputs, out)
	}
	evidence.SortOutputs(outputs)

	diags := []evidence.Diagnostic{{
		Level:   "warn",
		Message: "crawl4ai extraction fields are unbound hints; selectors are non-promotable evidence until an operator declares a portable source and CSS fallback rationale",
	}}

	raw2 := &evidence.RawRecord{
		Record: evidence.Record{
			Origin:           origin,
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
