package evidence

import (
	"bytes"
	"encoding/json"
	"testing"
)

func baseRecord() Record {
	return Record{
		Origin:          "https://example.test",
		ObservationKind: ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: RedactionNotRequired,
		Provenance:      Provenance{Tool: "synthetic"},
	}
}

// TestNormalizationValid tests that a well-formed RawRecord normalizes cleanly.
func TestNormalizationValid(t *testing.T) {
	raw := &RawRecord{
		Record:  baseRecord(),
		RawData: []byte(`{"raw":"data"}`),
	}
	rec, err := raw.Normalize()
	if err != nil {
		t.Fatalf("expected clean normalize, got: %v", err)
	}
	if rec.Origin != "https://example.test" {
		t.Errorf("origin mismatch: %q", rec.Origin)
	}
}

// TestNormalizePendingRedactionBlocked ensures pending-redaction records cannot
// leave the adapter boundary.
func TestNormalizePendingRedactionBlocked(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.RedactionStatus = RedactionPending
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for pending redaction, got nil")
	}
}

// TestNormalizeRedactedRequiresFields ensures RedactionCompleted without listed
// fields is rejected.
func TestNormalizeRedactedRequiresFields(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.RedactionStatus = RedactionCompleted
	// RedactedFields is nil — should fail
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for missing redacted fields, got nil")
	}
}

// TestNormalizeRedactedWithFields passes when fields are listed.
func TestNormalizeRedactedWithFields(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.RedactionStatus = RedactionCompleted
	raw.Record.RedactedFields = []string{"provenance.session"}
	if _, err := raw.Normalize(); err != nil {
		t.Fatalf("expected clean normalize with redacted fields, got: %v", err)
	}
}

// TestNormalizeMissingOrigin rejects an empty origin.
func TestNormalizeMissingOrigin(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.Origin = ""
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for empty origin, got nil")
	}
}

// TestNormalizeInvalidTimestamp rejects a non-RFC-3339 timestamp.
func TestNormalizeInvalidTimestamp(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.ObservedAt = "not-a-timestamp"
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for invalid timestamp, got nil")
	}
}

// TestNormalizeMissingTool rejects a record with no provenance tool.
func TestNormalizeMissingTool(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.Provenance.Tool = ""
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for empty provenance.tool, got nil")
	}
}

// TestDeterministicSerialization ensures MarshalDeterministic produces stable
// bytes regardless of input slice ordering.
func TestDeterministicSerialization(t *testing.T) {
	rec1 := baseRecord()
	rec1.CandidateLocators = []CandidateLocator{
		{Role: "link", Name: "Next"},
		{Role: "button", Name: "Submit"},
	}
	rec1.CandidateOutputs = []CandidateOutput{
		{Key: "total", Type: "string", Source: "microdata"},
		{Key: "items", Type: "array", Source: "a11y"},
	}

	rec2 := baseRecord()
	rec2.CandidateLocators = []CandidateLocator{
		{Role: "button", Name: "Submit"},
		{Role: "link", Name: "Next"},
	}
	rec2.CandidateOutputs = []CandidateOutput{
		{Key: "items", Type: "array", Source: "a11y"},
		{Key: "total", Type: "string", Source: "microdata"},
	}

	b1, err := MarshalDeterministic(rec1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := MarshalDeterministic(rec2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("expected identical bytes for same logical record:\n  a: %s\n  b: %s", b1, b2)
	}
}

// TestRoundTrip verifies JSON round-trip fidelity.
func TestRoundTrip(t *testing.T) {
	rec := baseRecord()
	rec.CandidateLocators = []CandidateLocator{
		{Role: "button", Name: "Submit", AmbiguityNote: "two buttons with same name"},
	}
	rec.Diagnostics = []Diagnostic{
		{Level: "warn", Message: "ambiguous locator found", Field: "candidateLocators[0]"},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Origin != rec.Origin {
		t.Errorf("origin: got %q want %q", got.Origin, rec.Origin)
	}
	if len(got.CandidateLocators) != 1 || got.CandidateLocators[0].AmbiguityNote != "two buttons with same name" {
		t.Errorf("locator ambiguity note not preserved: %+v", got.CandidateLocators)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Level != "warn" {
		t.Errorf("diagnostic not preserved: %+v", got.Diagnostics)
	}
}

// TestSortRecords verifies canonical Record slice ordering.
func TestSortRecords(t *testing.T) {
	records := []Record{
		{Origin: "https://b.test", ObservedAt: "2026-01-02T00:00:00Z", RedactionStatus: RedactionNotRequired, Provenance: Provenance{Tool: "t"}},
		{Origin: "https://a.test", ObservedAt: "2026-01-01T00:00:00Z", RedactionStatus: RedactionNotRequired, Provenance: Provenance{Tool: "t"}},
	}
	Sort(records)
	if records[0].Origin != "https://a.test" {
		t.Errorf("sort order wrong: first origin is %q", records[0].Origin)
	}
}

// TestNormalizeInvalidRedactionStatus rejects an unrecognized redaction status.
func TestNormalizeInvalidRedactionStatus(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.RedactionStatus = "not_needed" // typo of not_required
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for unrecognized redaction status, got nil")
	}
}

// TestNormalizeInvalidObservationKind rejects an unrecognized observation kind.
func TestNormalizeInvalidObservationKind(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.ObservationKind = "screen_grab" // not in enum
	if _, err := raw.Normalize(); err == nil {
		t.Error("expected error for unrecognized observation kind, got nil")
	}
}

// TestNormalizeUTCNormalization ensures a +00:00 offset timestamp is converted
// to Z form so Sort ordering is consistent.
func TestNormalizeUTCNormalization(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.ObservedAt = "2026-01-01T00:00:00+00:00"
	rec, err := raw.Normalize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ObservedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected UTC Z form, got %q", rec.ObservedAt)
	}
}

// TestNormalizeDoesNotMutateOriginal verifies Normalize deep-copies slices so
// the original RawRecord is not modified.
func TestNormalizeDoesNotMutateOriginal(t *testing.T) {
	raw := &RawRecord{Record: baseRecord()}
	raw.Record.CandidateLocators = []CandidateLocator{
		{Role: "link", Name: "Next"},
		{Role: "button", Name: "Submit"},
	}
	originalOrder := []string{raw.Record.CandidateLocators[0].Role, raw.Record.CandidateLocators[1].Role}

	if _, err := raw.Normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Original must be untouched
	if raw.Record.CandidateLocators[0].Role != originalOrder[0] ||
		raw.Record.CandidateLocators[1].Role != originalOrder[1] {
		t.Error("Normalize mutated the original RawRecord slices")
	}
}

// TestMarshalDeterministicNoMutation verifies MarshalDeterministic does not
// reorder the caller's slice.
func TestMarshalDeterministicNoMutation(t *testing.T) {
	rec := baseRecord()
	rec.CandidateLocators = []CandidateLocator{
		{Role: "link", Name: "Next"},
		{Role: "button", Name: "Submit"},
	}
	original := []string{rec.CandidateLocators[0].Role, rec.CandidateLocators[1].Role}

	if _, err := MarshalDeterministic(rec); err != nil {
		t.Fatal(err)
	}
	if rec.CandidateLocators[0].Role != original[0] || rec.CandidateLocators[1].Role != original[1] {
		t.Error("MarshalDeterministic mutated the caller's slice")
	}
}

// TestRawDataExcludedFromJSON confirms RawData is never serialized.
func TestRawDataExcludedFromJSON(t *testing.T) {
	raw := &RawRecord{
		Record:  baseRecord(),
		RawData: []byte(`secret-cookie=abc123`),
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret-cookie")) {
		t.Error("RawData leaked into JSON output")
	}
}
