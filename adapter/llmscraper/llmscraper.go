// Package llmscraper imports llm-scraper structured-extraction output as
// candidate output evidence.
//
// An llm-scraper fixture is a JSON object with this shape:
//
//	{
//	  "url": "https://example.test/page",
//	  "observedAt": "2026-01-01T00:00:00Z",
//	  "actionHint": "optional_name",
//	  "schema": {
//	    "type": "object",
//	    "properties": {
//	      "title": { "type": "string" },
//	      "price": { "type": "number" }
//	    }
//	  },
//	  "extracted": {
//	    "title": "Example Product",
//	    "price": 9.99
//	  }
//	}
//
// Each property in "schema.properties" becomes an unbound CandidateOutput.
// LLM-produced fields never fabricate a portable extraction source.
package llmscraper

import (
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/adapterdecode"
)

const (
	maxFixtureBytes = int64(2 << 20)
	maxOutputs      = 256
)

// Fixture is the expected shape of a saved llm-scraper result file.
type Fixture struct {
	URL        string         `json:"url"`
	ObservedAt string         `json:"observedAt,omitempty"`
	ActionHint string         `json:"actionHint,omitempty"`
	Schema     map[string]any `json:"schema,omitempty"`
	Extracted  map[string]any `json:"extracted,omitempty"`
}

// Adapter imports llm-scraper extraction output as candidate evidence.
type Adapter struct{}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return "llm-scraper" }

// Import parses a saved llm-scraper fixture and returns normalized evidence.
// LLM-produced fields are marked as candidate outputs (not trusted profile
// outputs) until validated by a human reviewer.
func (a *Adapter) Import(raw []byte, opts adapter.Options) ([]evidence.Record, error) {
	if opts.Origin == "" {
		return nil, fmt.Errorf("llm-scraper: opts.Origin is required")
	}
	status := opts.RedactionStatus
	if status == "" {
		return nil, fmt.Errorf("llm-scraper: opts.RedactionStatus is required; set evidence.RedactionNotRequired for synthetic fixtures")
	}

	var fix Fixture
	if err := adapterdecode.JSON(raw, maxFixtureBytes, &fix); err != nil {
		return nil, fmt.Errorf("llm-scraper: parse fixture: %w", err)
	}
	origin, err := adapter.CanonicalFixtureOrigin("llm-scraper", fix.URL, opts.Origin)
	if err != nil {
		return nil, err
	}

	observedAt := fix.ObservedAt
	if observedAt == "" {
		return nil, fmt.Errorf("llm-scraper: observedAt is required")
	} else {
		t, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			return nil, fmt.Errorf("llm-scraper: observedAt %q is not RFC-3339: %w", observedAt, err)
		}
		observedAt = t.UTC().Format(time.RFC3339)
	}

	actionHint := opts.ActionHint
	if actionHint == "" {
		actionHint = fix.ActionHint
	}

	var outputs []evidence.CandidateOutput
	if props, ok := schemaProperties(fix.Schema); ok {
		if len(props) > maxOutputs {
			return nil, fmt.Errorf("llm-scraper: schema exceeds %d output candidates", maxOutputs)
		}
		for key, rawProp := range props {
			prop, _ := rawProp.(map[string]any)
			typ := "string"
			if prop != nil {
				if t, ok := prop["type"].(string); ok && t != "" {
					typ = t
				}
			}
			outputs = append(outputs, evidence.CandidateOutput{
				Key: key, Type: typ,
			})
		}
	}
	evidence.SortOutputs(outputs)

	diags := []evidence.Diagnostic{{
		Level:   "info",
		Message: "unbound output hints inferred from llm-scraper schema; an operator must declare a portable source before promotion",
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
			Provenance:       evidence.Provenance{Tool: "llm-scraper"},
		},
	}
	rec, err := raw2.Normalize()
	if err != nil {
		return nil, fmt.Errorf("llm-scraper: normalize: %w", err)
	}
	return []evidence.Record{rec}, nil
}

func schemaProperties(schema map[string]any) (map[string]any, bool) {
	if schema == nil {
		return nil, false
	}
	props, ok := schema["properties"].(map[string]any)
	return props, ok
}
