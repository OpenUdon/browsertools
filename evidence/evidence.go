// Package evidence defines secret-free normalized evidence records for UI
// observations collected by browsertools adapters.
//
// The package enforces a two-layer model:
//
//   - RawRecord: untrusted, tool-specific output. May contain credentials,
//     cookies, or other secrets. MUST NOT be committed to source control; use it
//     only transiently inside adapter code.
//
//   - Record: normalized, prompt-safe observation. All sensitive fields must be
//     replaced by redaction markers before a Record is constructed. This is the
//     type that flows into the draft builder and review bundle.
//
// Records serialize deterministically: encoding/json sorts struct fields by
// declaration order and map keys alphabetically. Slice fields (CandidateLocators,
// CandidateOutputs, Diagnostics) must be sorted before serialization; use
// MarshalDeterministic which handles this automatically.
package evidence

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// ObservationKind mirrors the uws.browser.1.5 schema enum for how the profile
// was learned.
type ObservationKind string

const (
	// ObservationA11ySnapshot means evidence was captured from an accessibility tree snapshot.
	ObservationA11ySnapshot ObservationKind = "accessibility_snapshot"
	// ObservationDOMText means evidence was captured from raw DOM text content.
	ObservationDOMText ObservationKind = "dom_text"
	// ObservationScreenshotOCR means evidence was captured via OCR on a screenshot.
	ObservationScreenshotOCR ObservationKind = "screenshot_ocr"
	// ObservationOther means evidence was captured through another means.
	ObservationOther ObservationKind = "other"
)

var validObservationKinds = map[ObservationKind]bool{
	ObservationA11ySnapshot:  true,
	ObservationDOMText:       true,
	ObservationScreenshotOCR: true,
	ObservationOther:         true,
}

// RedactionStatus records whether a Record's sensitive fields have been
// reviewed and redacted.
type RedactionStatus string

const (
	// RedactionNotRequired means the record contained no sensitive fields.
	RedactionNotRequired RedactionStatus = "not_required"
	// RedactionCompleted means sensitive fields were found and replaced with
	// markers listed in RedactedFields.
	RedactionCompleted RedactionStatus = "redacted"
	// RedactionPending means the record has not yet been reviewed for secrets.
	// Records in this state must not leave the adapter boundary.
	RedactionPending RedactionStatus = "pending"
)

var validRedactionStatuses = map[RedactionStatus]bool{
	RedactionNotRequired: true,
	RedactionCompleted:   true,
	RedactionPending:     true,
}

// CandidateLocator is an accessibility-tree locator candidate observed during a
// UI session. It mirrors the locator shape in the browser-profile schema.
type CandidateLocator struct {
	Role          string `json:"role"`
	Name          string `json:"name,omitempty"`
	Text          string `json:"text,omitempty"`
	Value         string `json:"value,omitempty"`
	AmbiguityNote string `json:"ambiguityNote,omitempty"`
}

// LocatorDecision records a reviewer's explicit resolution of ambiguous
// locator evidence. The selected locator is identified by its portable
// role/name/text/value fields; AmbiguityNote is evidence and is ignored when
// matching the decision. Rationale is required for a decision to resolve a
// promotion-blocking ambiguity.
type LocatorDecision struct {
	ActionHint string           `json:"actionHint" yaml:"actionHint"`
	Locator    CandidateLocator `json:"locator" yaml:"locator"`
	Rationale  string           `json:"rationale" yaml:"rationale"`
}

// Matches reports whether the decision resolves loc for actionHint.
func (d LocatorDecision) Matches(actionHint string, loc CandidateLocator) bool {
	return d.ActionHint == actionHint &&
		d.Locator.Role == loc.Role &&
		d.Locator.Name == loc.Name &&
		d.Locator.Text == loc.Text &&
		d.Locator.Value == loc.Value
}

// CandidateOutput is a candidate output extraction observed or inferred during
// evidence collection. An empty Source is an unbound schema/extraction hint
// that cannot be promoted automatically. Bound sources must be one of a11y,
// jsonld, microdata, or css and carry their source-specific fields.
type CandidateOutput struct {
	Key            string            `json:"key"`
	Type           string            `json:"type"`   // JSON type: string, integer, number, boolean, array, object, null
	Source         string            `json:"source"` // a11y, jsonld, microdata, css
	Locator        *CandidateLocator `json:"locator,omitempty"`
	Selector       string            `json:"selector,omitempty"`       // CSS selector; only when source=css
	FallbackReason string            `json:"fallbackReason,omitempty"` // required when source=css
	Property       string            `json:"property,omitempty"`       // microdata/jsonld property name
}

// Provenance records what tool and session produced this evidence. The Session
// field is an opaque reference only — it must never hold a credential, cookie,
// or authentication token.
type Provenance struct {
	Tool    string `json:"tool"`
	Version string `json:"version,omitempty"`
	Session string `json:"session,omitempty"`
}

// Diagnostic is an observation note, warning, or error attached to a Record.
type Diagnostic struct {
	Level   string `json:"level"` // "info", "warn", "error"
	Message string `json:"message"`
	Field   string `json:"field,omitempty"` // dot-path to the affected field
}

// Record is a normalized, secret-free evidence record for a single UI
// observation. It is safe to store in source control and to pass to the draft
// builder.
//
// Use MarshalDeterministic to serialize; it sorts all slice fields before
// encoding so repeated calls produce identical bytes.
type Record struct {
	// Origin is the browser origin (scheme + host + optional port) where the
	// observation was made. Must match the profile's info.origin allowlist.
	Origin string `json:"origin"`

	// ObservationKind records how the evidence was captured.
	ObservationKind ObservationKind `json:"observationKind"`

	// ObservedAt is the RFC-3339 timestamp of the observation. Always stored in
	// UTC (Z suffix) to guarantee lexicographic sort stability.
	ObservedAt string `json:"observedAt"`

	// ActionHint is the action name this evidence was collected for, if known.
	ActionHint string `json:"actionHint,omitempty"`

	// CandidateLocators are a11y locator candidates for the observed interaction
	// target, in preference order.
	CandidateLocators []CandidateLocator `json:"candidateLocators,omitempty"`

	// CandidateOutputs are output extraction candidates observed or inferred.
	CandidateOutputs []CandidateOutput `json:"candidateOutputs,omitempty"`

	// RedactionStatus records whether sensitive fields were reviewed.
	RedactionStatus RedactionStatus `json:"redactionStatus"`

	// RedactedFields lists dot-paths to fields that were replaced with
	// "[REDACTED]" markers. Non-empty only when RedactionStatus is
	// RedactionCompleted.
	RedactedFields []string `json:"redactedFields,omitempty"`

	// Diagnostics are notes, warnings, or errors from the adapter.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`

	// Provenance records which tool produced this record.
	Provenance Provenance `json:"provenance"`
}

// RawRecord wraps untrusted tool output before normalization. It MUST NOT be
// committed to source control or passed outside the adapter that created it.
// Call Normalize to produce a safe Record.
type RawRecord struct {
	// Record holds the partially-populated normalized view. RedactionStatus
	// is set to RedactionPending until normalization is complete.
	Record Record

	// RawData is the raw bytes from the tool. Excluded from JSON serialization;
	// never log or store it without explicit redaction review.
	RawData []byte `json:"-"`
}

// Normalize validates r.Record for completeness and redaction compliance, then
// returns a deep-copied, sorted Record. The original RawRecord is never
// modified. Call this at the adapter boundary after applying all redactions.
func (r *RawRecord) Normalize() (Record, error) {
	rec := r.Record

	// Validate redaction status enum first; an empty or unrecognized value is an
	// adapter bug.
	if !validRedactionStatuses[rec.RedactionStatus] {
		return Record{}, fmt.Errorf("normalize: invalid redactionStatus %q; must be one of not_required, redacted, pending", rec.RedactionStatus)
	}
	if rec.RedactionStatus == RedactionPending {
		return Record{}, fmt.Errorf("normalize: record for %q has pending redaction; complete redaction review before normalizing", rec.Origin)
	}
	if rec.RedactionStatus == RedactionCompleted && len(rec.RedactedFields) == 0 {
		return Record{}, fmt.Errorf("normalize: redaction status is %q but no redacted fields listed", RedactionCompleted)
	}
	if rec.Origin == "" {
		return Record{}, fmt.Errorf("normalize: origin is required")
	}
	if rec.ObservedAt == "" {
		return Record{}, fmt.Errorf("normalize: observedAt is required")
	}
	t, err := time.Parse(time.RFC3339, rec.ObservedAt)
	if err != nil {
		return Record{}, fmt.Errorf("normalize: observedAt %q is not RFC-3339: %w", rec.ObservedAt, err)
	}
	// Normalize to UTC so lexicographic sort in Sort() is timestamp-consistent
	// (Z and +00:00 represent the same instant but sort differently as strings).
	rec.ObservedAt = t.UTC().Format(time.RFC3339)

	if rec.Provenance.Tool == "" {
		return Record{}, fmt.Errorf("normalize: provenance.tool is required")
	}
	if rec.ObservationKind != "" && !validObservationKinds[rec.ObservationKind] {
		return Record{}, fmt.Errorf("normalize: invalid observationKind %q", rec.ObservationKind)
	}
	for index, output := range rec.CandidateOutputs {
		if err := validateCandidateOutput(output); err != nil {
			return Record{}, fmt.Errorf("normalize: candidateOutputs[%d]: %w", index, err)
		}
	}

	// Deep-copy slices before sorting so the original RawRecord is not mutated.
	rec.CandidateLocators = cloneLocators(rec.CandidateLocators)
	rec.CandidateOutputs = cloneOutputs(rec.CandidateOutputs)
	rec.Diagnostics = cloneDiagnostics(rec.Diagnostics)
	rec.RedactedFields = cloneStrings(rec.RedactedFields)

	SortLocators(rec.CandidateLocators)
	SortOutputs(rec.CandidateOutputs)
	SortDiagnostics(rec.Diagnostics)
	sort.Strings(rec.RedactedFields)
	return rec, nil
}

// MarshalDeterministic returns the canonical JSON encoding of rec. It
// deep-copies and sorts all slice fields before encoding, so repeated calls on
// the same value produce identical bytes and the caller's slice order is not
// modified.
func MarshalDeterministic(rec Record) ([]byte, error) {
	rec.CandidateLocators = cloneLocators(rec.CandidateLocators)
	rec.CandidateOutputs = cloneOutputs(rec.CandidateOutputs)
	rec.Diagnostics = cloneDiagnostics(rec.Diagnostics)
	rec.RedactedFields = cloneStrings(rec.RedactedFields)
	SortLocators(rec.CandidateLocators)
	SortOutputs(rec.CandidateOutputs)
	SortDiagnostics(rec.Diagnostics)
	sort.Strings(rec.RedactedFields)
	return json.Marshal(rec)
}

// Sort sorts a slice of Records into a stable canonical order:
// (Origin, ObservedAt, Provenance.Tool). ObservedAt values must be in UTC
// (as produced by Normalize) for this ordering to be chronologically correct.
func Sort(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.Origin != b.Origin {
			return a.Origin < b.Origin
		}
		if a.ObservedAt != b.ObservedAt {
			return a.ObservedAt < b.ObservedAt
		}
		return a.Provenance.Tool < b.Provenance.Tool
	})
}

// SortLocators sorts candidate locators in-place by (Role, Name, Text).
// Including Text breaks ties when Role and Name are identical.
func SortLocators(locs []CandidateLocator) {
	sort.SliceStable(locs, func(i, j int) bool {
		if locs[i].Role != locs[j].Role {
			return locs[i].Role < locs[j].Role
		}
		if locs[i].Name != locs[j].Name {
			return locs[i].Name < locs[j].Name
		}
		return locs[i].Text < locs[j].Text
	})
}

// SortOutputs sorts candidate outputs in-place by Key.
func SortOutputs(outs []CandidateOutput) {
	sort.SliceStable(outs, func(i, j int) bool {
		return outs[i].Key < outs[j].Key
	})
}

// SortDiagnostics sorts diagnostics in-place by (Level, Message).
func SortDiagnostics(diags []Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Level != diags[j].Level {
			return diags[i].Level < diags[j].Level
		}
		return diags[i].Message < diags[j].Message
	})
}

// --- deep-copy helpers ---

func cloneLocators(src []CandidateLocator) []CandidateLocator {
	if src == nil {
		return nil
	}
	out := make([]CandidateLocator, len(src))
	copy(out, src)
	return out
}

func cloneOutputs(src []CandidateOutput) []CandidateOutput {
	if src == nil {
		return nil
	}
	out := make([]CandidateOutput, len(src))
	for index, value := range src {
		out[index] = value
		if value.Locator != nil {
			locator := *value.Locator
			out[index].Locator = &locator
		}
	}
	return out
}

func validateCandidateOutput(output CandidateOutput) error {
	if strings.TrimSpace(output.Key) == "" {
		return fmt.Errorf("key is required")
	}
	if !slices.Contains([]string{"string", "integer", "number", "boolean", "array", "object", "null"}, output.Type) {
		return fmt.Errorf("type %q is invalid", output.Type)
	}
	switch output.Source {
	case "":
		if output.Locator != nil || strings.TrimSpace(output.FallbackReason) != "" {
			return fmt.Errorf("unbound hints cannot carry a portable locator or fallback reason")
		}
	case "a11y":
		if output.Locator == nil || strings.TrimSpace(output.Locator.Role) == "" {
			return fmt.Errorf("a11y source requires a locator with a role")
		}
		if output.Selector != "" || output.Property != "" || output.FallbackReason != "" {
			return fmt.Errorf("a11y source contains fields for another source")
		}
	case "jsonld", "microdata":
		if strings.TrimSpace(output.Property) == "" {
			return fmt.Errorf("%s source requires property", output.Source)
		}
		if output.Locator != nil || output.Selector != "" || output.FallbackReason != "" {
			return fmt.Errorf("%s source contains fields for another source", output.Source)
		}
	case "css":
		if strings.TrimSpace(output.Selector) == "" || strings.TrimSpace(output.FallbackReason) == "" {
			return fmt.Errorf("css source requires selector and nonblank fallbackReason")
		}
		if !slices.Contains([]string{"no_a11y_region", "no_structured_data", "ambiguous_a11y", "other"}, output.FallbackReason) {
			return fmt.Errorf("css fallbackReason %q is invalid", output.FallbackReason)
		}
		if output.Locator != nil || output.Property != "" {
			return fmt.Errorf("css source contains fields for another source")
		}
	default:
		return fmt.Errorf("source %q is invalid", output.Source)
	}
	return nil
}

func cloneDiagnostics(src []Diagnostic) []Diagnostic {
	if src == nil {
		return nil
	}
	out := make([]Diagnostic, len(src))
	copy(out, src)
	return out
}

func cloneStrings(src []string) []string {
	if src == nil {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}
