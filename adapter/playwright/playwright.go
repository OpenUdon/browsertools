// Package playwright imports Playwright accessibility-snapshot and action-probe
// fixtures as normalized evidence records.
//
// A Playwright fixture is a JSON object with the following shape:
//
//	{
//	  "url": "https://example.test/page",
//	  "observedAt": "2026-01-01T00:00:00Z",
//	  "snapshot": {
//	    "role": "...",
//	    "name": "...",
//	    "children": [...]
//	  },
//	  "actionHint": "optional_action_name"
//	}
//
// The adapter walks the snapshot tree and collects all interactive nodes
// (role in the locator enum) as CandidateLocators. No live browser is
// required; tests operate on saved JSON fixtures.
package playwright

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
)

// interactiveRoles is the set of ARIA roles that are valid locator targets.
var interactiveRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "checkbox": true,
	"radio": true, "dialog": true, "status": true, "alert": true,
	"combobox": true, "option": true, "menu": true, "menuitem": true,
	"tab": true, "tabpanel": true, "switch": true, "search": true,
}

// Fixture is the expected shape of a saved Playwright snapshot file.
type Fixture struct {
	URL        string        `json:"url"`
	ObservedAt string        `json:"observedAt"`
	ActionHint string        `json:"actionHint,omitempty"`
	Snapshot   *SnapshotNode `json:"snapshot"`
}

// SnapshotNode is one node in the Playwright accessibility tree.
type SnapshotNode struct {
	Role     string          `json:"role"`
	Name     string          `json:"name,omitempty"`
	Value    string          `json:"value,omitempty"`
	Children []*SnapshotNode `json:"children,omitempty"`
}

// Adapter imports Playwright accessibility snapshot fixtures.
type Adapter struct{}

// Name implements adapter.Adapter.
func (a *Adapter) Name() string { return "playwright" }

// Import parses a saved Playwright fixture JSON and returns a normalized
// evidence record. The fixture must not contain live credentials or cookies;
// the caller is responsible for redaction before calling Import.
func (a *Adapter) Import(raw []byte, opts adapter.Options) ([]evidence.Record, error) {
	if opts.Origin == "" {
		return nil, fmt.Errorf("playwright: opts.Origin is required")
	}
	status := opts.RedactionStatus
	if status == "" {
		return nil, fmt.Errorf("playwright: opts.RedactionStatus is required; set evidence.RedactionNotRequired for synthetic fixtures")
	}

	var fix Fixture
	if err := json.Unmarshal(raw, &fix); err != nil {
		return nil, fmt.Errorf("playwright: parse fixture: %w", err)
	}
	if err := adapter.ValidateFixtureOrigin("playwright", fix.URL, opts.Origin); err != nil {
		return nil, err
	}

	observedAt := fix.ObservedAt
	if observedAt == "" {
		return nil, fmt.Errorf("playwright: observedAt is required")
	} else {
		t, err := time.Parse(time.RFC3339, observedAt)
		if err != nil {
			return nil, fmt.Errorf("playwright: observedAt %q is not RFC-3339: %w", observedAt, err)
		}
		observedAt = t.UTC().Format(time.RFC3339)
	}

	actionHint := opts.ActionHint
	if actionHint == "" {
		actionHint = fix.ActionHint
	}

	var locs []evidence.CandidateLocator
	if fix.Snapshot != nil {
		locs = walkSnapshot(fix.Snapshot, nil)
		markAmbiguousLocators(locs)
	}

	raw2 := &evidence.RawRecord{
		Record: evidence.Record{
			Origin:            opts.Origin,
			ObservationKind:   evidence.ObservationA11ySnapshot,
			ObservedAt:        observedAt,
			ActionHint:        actionHint,
			CandidateLocators: locs,
			RedactionStatus:   status,
			RedactedFields:    opts.RedactedFields,
			Provenance:        evidence.Provenance{Tool: "playwright"},
		},
	}
	rec, err := raw2.Normalize()
	if err != nil {
		return nil, fmt.Errorf("playwright: normalize: %w", err)
	}
	return []evidence.Record{rec}, nil
}

// maxSnapshotDepth limits walkSnapshot recursion to prevent stack overflow on
// pathologically deep fixture trees.
const maxSnapshotDepth = 64

// walkSnapshot recursively collects interactive nodes from the a11y tree up to
// maxSnapshotDepth levels. Nodes deeper than the limit are silently skipped.
func walkSnapshot(node *SnapshotNode, acc []evidence.CandidateLocator) []evidence.CandidateLocator {
	return walkSnapshotDepth(node, acc, 0)
}

func walkSnapshotDepth(node *SnapshotNode, acc []evidence.CandidateLocator, depth int) []evidence.CandidateLocator {
	if node == nil || depth > maxSnapshotDepth {
		return acc
	}
	if interactiveRoles[node.Role] {
		loc := evidence.CandidateLocator{Role: node.Role, Name: node.Name}
		acc = append(acc, loc)
	}
	for _, child := range node.Children {
		acc = walkSnapshotDepth(child, acc, depth+1)
	}
	return acc
}

func markAmbiguousLocators(locs []evidence.CandidateLocator) {
	counts := map[string]int{}
	for _, loc := range locs {
		counts[locatorKey(loc)]++
	}
	for i := range locs {
		if count := counts[locatorKey(locs[i])]; count > 1 {
			locs[i].AmbiguityNote = fmt.Sprintf("%d accessibility nodes share role=%q and name=%q", count, locs[i].Role, locs[i].Name)
		}
	}
}

func locatorKey(loc evidence.CandidateLocator) string {
	return loc.Role + "|" + loc.Name
}
