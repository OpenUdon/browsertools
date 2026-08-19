// Package playwright imports Playwright accessibility-snapshot and action-probe
// fixtures as normalized evidence records.
//
// A legacy Playwright fixture is a JSON object with the following shape:
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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/adapterdecode"
	"github.com/OpenUdon/evidence/redact"
	"gopkg.in/yaml.v3"
)

const (
	// FixtureVersion identifies raw fixtures emitted by Browsertools live
	// acquisition. Legacy saved-tree fixtures omit this field.
	FixtureVersion  = "browsertools.playwright-capture.v1"
	maxFixtureBytes = 8 << 20
	maxARIANodes    = 8192
	maxLocators     = 4096
	maxJSONLDDocs   = 32
	maxOutputs      = 256
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
	Version           string            `json:"version,omitempty"`
	URL               string            `json:"url"`
	ObservedAt        string            `json:"observedAt"`
	ActionHint        string            `json:"actionHint,omitempty"`
	PlaywrightVersion string            `json:"playwrightVersion,omitempty"`
	Snapshot          *SnapshotNode     `json:"snapshot,omitempty"`
	ARIASnapshot      string            `json:"ariaSnapshot,omitempty"`
	StructuredData    []json.RawMessage `json:"structuredData,omitempty"`
	Network           *NetworkSummary   `json:"network,omitempty"`
}

// NetworkSummary records bounded counts only. It intentionally contains no
// URLs, headers, cookies, bodies, or timing fingerprints.
type NetworkSummary struct {
	Requests            int   `json:"requests"`
	Responses           int   `json:"responses"`
	ResponseBytes       int64 `json:"responseBytes"`
	BlockedRequests     int   `json:"blockedRequests"`
	BlockedWebSockets   int   `json:"blockedWebSockets"`
	BlockedPopups       int   `json:"blockedPopups"`
	BlockedDownloads    int   `json:"blockedDownloads"`
	BlockedDialogs      int   `json:"blockedDialogs"`
	BlockedFileChoosers int   `json:"blockedFileChoosers"`
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
	if err := adapterdecode.JSON(raw, maxFixtureBytes, &fix); err != nil {
		return nil, fmt.Errorf("playwright: parse fixture: %w", err)
	}
	if fix.Version != "" && fix.Version != FixtureVersion {
		return nil, fmt.Errorf("playwright: unsupported fixture version %q", fix.Version)
	}
	if fix.Snapshot != nil && fix.ARIASnapshot != "" {
		return nil, fmt.Errorf("playwright: fixture must contain only one snapshot representation")
	}
	origin, originErr := adapter.CanonicalFixtureOrigin("playwright", fix.URL, opts.Origin)
	if originErr != nil {
		return nil, originErr
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

	var (
		locs []evidence.CandidateLocator
		err  error
	)
	if fix.Snapshot != nil {
		locs = walkSnapshot(fix.Snapshot, nil)
	} else if fix.ARIASnapshot != "" {
		locs, err = locatorsFromARIA(fix.ARIASnapshot)
		if err != nil {
			return nil, fmt.Errorf("playwright: aria snapshot: %w", err)
		}
	}
	markAmbiguousLocators(locs)
	outputs, diagnostics, err := outputsFromJSONLD(fix.StructuredData)
	if err != nil {
		return nil, fmt.Errorf("playwright: structured data: %w", err)
	}

	raw2 := &evidence.RawRecord{
		Record: evidence.Record{
			Origin:            origin,
			ObservationKind:   evidence.ObservationA11ySnapshot,
			ObservedAt:        observedAt,
			ActionHint:        actionHint,
			CandidateLocators: locs,
			CandidateOutputs:  outputs,
			RedactionStatus:   status,
			RedactedFields:    opts.RedactedFields,
			Diagnostics:       diagnostics,
			Provenance:        evidence.Provenance{Tool: "playwright", Version: fix.PlaywrightVersion},
		},
	}
	rec, err := raw2.Normalize()
	if err != nil {
		return nil, fmt.Errorf("playwright: normalize: %w", err)
	}
	return []evidence.Record{rec}, nil
}

func locatorsFromARIA(snapshot string) ([]evidence.CandidateLocator, error) {
	if len(snapshot) > maxFixtureBytes {
		return nil, fmt.Errorf("snapshot exceeds %d bytes", maxFixtureBytes)
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(snapshot))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not supported")
		}
		return nil, err
	}
	count := 0
	locators := make([]evidence.CandidateLocator, 0)
	var walk func(*yaml.Node, int, bool) error
	walk = func(node *yaml.Node, depth int, parseScalar bool) error {
		if node == nil {
			return nil
		}
		if depth > maxSnapshotDepth {
			return fmt.Errorf("snapshot exceeds depth %d", maxSnapshotDepth)
		}
		count++
		if count > maxARIANodes {
			return fmt.Errorf("snapshot exceeds %d nodes", maxARIANodes)
		}
		if node.Kind == yaml.AliasNode {
			return fmt.Errorf("YAML aliases are not supported")
		}
		if node.Kind == yaml.ScalarNode {
			if !parseScalar {
				return nil
			}
			role, name, err := parseARIAKey(node.Value)
			if err != nil {
				return err
			}
			if interactiveRoles[role] {
				if len(locators) >= maxLocators {
					return fmt.Errorf("snapshot exceeds %d locator candidates", maxLocators)
				}
				locators = append(locators, evidence.CandidateLocator{Role: role, Name: name})
			}
			return nil
		}
		switch node.Kind {
		case yaml.MappingNode:
			for index := 0; index+1 < len(node.Content); index += 2 {
				if err := walk(node.Content[index], depth+1, true); err != nil {
					return err
				}
				if err := walk(node.Content[index+1], depth+1, false); err != nil {
					return err
				}
			}
		default:
			for _, child := range node.Content {
				if err := walk(child, depth+1, node.Kind == yaml.SequenceNode); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(&document, 0, false); err != nil {
		return nil, err
	}
	return locators, nil
}

func parseARIAKey(value string) (role, name string, err error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "/") {
		return "", "", nil
	}
	end := strings.IndexAny(value, " \t[")
	if end < 0 {
		return value, "", nil
	}
	role = value[:end]
	rest := strings.TrimSpace(value[end:])
	if !strings.HasPrefix(rest, `"`) {
		return role, "", nil
	}
	quoted, ok := quotedPrefix(rest)
	if !ok {
		return "", "", fmt.Errorf("invalid quoted accessible name in %q", value)
	}
	name, err = strconv.Unquote(quoted)
	if err != nil {
		return "", "", fmt.Errorf("invalid accessible name in %q: %w", value, err)
	}
	return role, name, nil
}

func quotedPrefix(value string) (string, bool) {
	escaped := false
	for index := 1; index < len(value); index++ {
		switch value[index] {
		case '\\':
			escaped = !escaped
		case '"':
			if !escaped {
				return value[:index+1], true
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return "", false
}

var outputKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func outputsFromJSONLD(documents []json.RawMessage) ([]evidence.CandidateOutput, []evidence.Diagnostic, error) {
	if len(documents) > maxJSONLDDocs {
		return nil, nil, fmt.Errorf("more than %d JSON-LD documents", maxJSONLDDocs)
	}
	types := map[string]string{}
	conflicts := map[string]bool{}
	sensitive := map[string]bool{}
	properties := 0
	for index, document := range documents {
		decoder := json.NewDecoder(bytes.NewReader(document))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, fmt.Errorf("document[%d]: %w", index, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, nil, fmt.Errorf("document[%d] contains trailing JSON", index)
		}
		for _, object := range topLevelObjects(value) {
			for key, item := range object {
				properties++
				if properties > maxOutputs*4 {
					return nil, nil, fmt.Errorf("more than %d JSON-LD properties", maxOutputs*4)
				}
				if strings.HasPrefix(key, "@") || !outputKeyPattern.MatchString(key) {
					continue
				}
				if sensitiveOutputKey(key) {
					sensitive[key] = true
					continue
				}
				itemType := jsonType(item)
				if previous, ok := types[key]; ok && previous != itemType {
					conflicts[key] = true
					continue
				}
				types[key] = itemType
			}
		}
	}
	keys := make([]string, 0, len(types))
	for key := range types {
		if !conflicts[key] {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	if len(keys) > maxOutputs {
		return nil, nil, fmt.Errorf("more than %d JSON-LD output candidates", maxOutputs)
	}
	outputs := make([]evidence.CandidateOutput, 0, len(keys))
	for _, key := range keys {
		outputs = append(outputs, evidence.CandidateOutput{Key: key, Type: types[key], Source: "jsonld", Property: key})
	}
	diagnostics := make([]evidence.Diagnostic, 0, len(conflicts)+len(sensitive))
	for key := range conflicts {
		diagnostics = append(diagnostics, evidence.Diagnostic{Level: "warn", Field: "structuredData." + key, Message: "JSON-LD property has conflicting observed types and was not proposed as an output"})
	}
	for key := range sensitive {
		diagnostics = append(diagnostics, evidence.Diagnostic{Level: "warn", Field: "structuredData." + key, Message: "credential-shaped JSON-LD property was not proposed as an output"})
	}
	return outputs, diagnostics, nil
}

func sensitiveOutputKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "cookie", "cookies", "oauth_state", "session", "session_id", "session_storage", "local_storage":
		return true
	default:
		return redact.SensitiveKey(normalized)
	}
}

func topLevelObjects(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return []map[string]any{typed}
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				result = append(result, object)
			}
		}
		return result
	default:
		return nil
	}
}

func jsonType(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		if !strings.ContainsAny(string(typed), ".eE") {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "null"
	}
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
