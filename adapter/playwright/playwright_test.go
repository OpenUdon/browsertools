package playwright

import (
	"encoding/json"
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

// TestImportFromFixture verifies that a saved snapshot fixture produces a
// valid, normalized evidence record with candidate locators.
func TestImportFromFixture(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/snapshot_status.json")
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
	if rec.Origin != "https://example.test" {
		t.Errorf("origin: got %q", rec.Origin)
	}
	if rec.ObservationKind != evidence.ObservationA11ySnapshot {
		t.Errorf("kind: got %q", rec.ObservationKind)
	}
	if rec.Provenance.Tool != "playwright" {
		t.Errorf("tool: got %q", rec.Provenance.Tool)
	}
	// Should have collected button, link, status, heading from the snapshot
	if len(rec.CandidateLocators) == 0 {
		t.Error("expected candidate locators, got none")
	}
	foundButton := false
	for _, loc := range rec.CandidateLocators {
		if loc.Role == "button" && loc.Name == "Refresh" {
			foundButton = true
		}
	}
	if !foundButton {
		t.Errorf("expected button/Refresh locator, got: %+v", rec.CandidateLocators)
	}
}

// TestImportActionHintOverride verifies opts.ActionHint overrides fixture actionHint.
func TestImportActionHintOverride(t *testing.T) {
	a := &Adapter{}
	raw := fixtureBytes(t, "testdata/snapshot_status.json")
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		ActionHint:      "custom_hint",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	if records[0].ActionHint != "custom_hint" {
		t.Errorf("expected action hint override, got %q", records[0].ActionHint)
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

// TestImportInvalidJSON returns an error for malformed input.
func TestImportInvalidJSON(t *testing.T) {
	a := &Adapter{}
	_, err := a.Import([]byte(`not-json`), adapter.Options{Origin: "https://example.test", RedactionStatus: evidence.RedactionNotRequired})
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestImportMissingObservedAtError(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{"url":"https://example.test/status","snapshot":{"role":"region"}}`)
	_, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err == nil {
		t.Fatal("expected error for missing observedAt, got nil")
	}
}

// TestNoLiveBrowserRequired confirms that the adapter works entirely on
// saved fixtures without any network calls.
func TestNoLiveBrowserRequired(t *testing.T) {
	a := &Adapter{}
	synthetic := Fixture{
		URL:        "https://example.test/page",
		ObservedAt: "2026-01-01T00:00:00Z",
		ActionHint: "test",
		Snapshot: &SnapshotNode{
			Role: "region",
			Name: "Content",
			Children: []*SnapshotNode{
				{Role: "button", Name: "Go"},
			},
		},
	}
	raw, _ := json.Marshal(synthetic)
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(records[0].CandidateLocators) != 1 {
		t.Errorf("unexpected result: %+v", records)
	}
}

func TestImportFixtureOriginSafety(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "missing url",
			raw:  `{"observedAt":"2026-01-01T00:00:00Z","snapshot":{"role":"region"}}`,
		},
		{
			name: "malformed url",
			raw:  `{"url":"://bad","observedAt":"2026-01-01T00:00:00Z","snapshot":{"role":"region"}}`,
		},
		{
			name: "unsupported scheme",
			raw:  `{"url":"ftp://example.test/status","observedAt":"2026-01-01T00:00:00Z","snapshot":{"role":"region"}}`,
		},
		{
			name: "origin mismatch",
			raw:  `{"url":"https://other.test/status","observedAt":"2026-01-01T00:00:00Z","snapshot":{"role":"region"}}`,
		},
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

func TestImportDuplicateRoleNameMarkedAmbiguous(t *testing.T) {
	a := &Adapter{}
	raw := []byte(`{
		"url":"https://example.test/status",
		"observedAt":"2026-01-01T00:00:00Z",
		"snapshot":{"role":"region","children":[
			{"role":"button","name":"Save"},
			{"role":"button","name":"Save"}
		]}
	}`)
	records, err := a.Import(raw, adapter.Options{
		Origin:          "https://example.test",
		RedactionStatus: evidence.RedactionNotRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records[0].CandidateLocators) != 2 {
		t.Fatalf("expected 2 locators, got %+v", records[0].CandidateLocators)
	}
	for _, loc := range records[0].CandidateLocators {
		if !strings.Contains(loc.AmbiguityNote, "share role") {
			t.Fatalf("expected ambiguity note, got %+v", records[0].CandidateLocators)
		}
	}
}

func TestImportLiveARIASnapshotAndJSONLD(t *testing.T) {
	fixture := Fixture{
		Version: FixtureVersion, URL: "https://example.test/member", ObservedAt: "2026-08-15T12:00:00Z",
		ActionHint: "read_dashboard", PlaywrightVersion: "1.62.1",
		ARIASnapshot: `- heading "Member dashboard" [level=1]
- paragraph: button
- button "Refresh"
- textbox "Search"
- button "Refresh"
`,
		StructuredData: []json.RawMessage{
			json.RawMessage(`{"status":"active","count":2,"details":{"tier":"gold"},"token":"do-not-copy"}`),
			json.RawMessage(`{"status":7,"enabled":true}`),
		},
	}
	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	records, err := (&Adapter{}).Import(raw, adapter.Options{
		Origin: "https://example.test", RedactionStatus: evidence.RedactionCompleted,
		RedactedFields: []string{"structuredData[0].token"},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	if record.ActionHint != "read_dashboard" || record.Provenance.Version != "1.62.1" {
		t.Fatalf("record = %#v", record)
	}
	refreshes := 0
	for _, locator := range record.CandidateLocators {
		if locator.Role == "button" && locator.Name == "" {
			t.Fatalf("mapping text was interpreted as a locator: %#v", record.CandidateLocators)
		}
		if locator.Role == "button" && locator.Name == "Refresh" {
			refreshes++
			if locator.AmbiguityNote == "" {
				t.Fatalf("ambiguous locator is unmarked: %#v", locator)
			}
		}
	}
	if refreshes != 2 {
		t.Fatalf("locators = %#v", record.CandidateLocators)
	}
	outputs := map[string]string{}
	for _, output := range record.CandidateOutputs {
		outputs[output.Key] = output.Type
	}
	if outputs["enabled"] != "boolean" || outputs["details"] != "object" || outputs["count"] != "integer" {
		t.Fatalf("outputs = %#v", record.CandidateOutputs)
	}
	if _, ok := outputs["status"]; ok {
		t.Fatalf("conflicting output was retained: %#v", record.CandidateOutputs)
	}
	if _, ok := outputs["token"]; ok {
		t.Fatalf("credential-shaped output was retained: %#v", record.CandidateOutputs)
	}
	encoded, err := evidence.MarshalDeterministic(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "do-not-copy") {
		t.Fatalf("structured value leaked into normalized evidence: %s", encoded)
	}
}

func TestImportLiveFixtureFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unknown field", raw: `{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z","unknown":true}`},
		{name: "wrong version", raw: `{"version":"browsertools.playwright-capture.v2","url":"https://example.test","observedAt":"2026-01-01T00:00:00Z"}`},
		{name: "two snapshots", raw: `{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z","snapshot":{"role":"region"},"ariaSnapshot":"- button \\"Go\\""}`},
		{name: "trailing document", raw: `{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z"} {}`},
		{name: "malformed aria name", raw: `{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z","ariaSnapshot":"- button \\"unterminated"}`},
		{name: "invalid jsonld", raw: `{"url":"https://example.test","observedAt":"2026-01-01T00:00:00Z","structuredData":[{"ok":true},invalid]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&Adapter{}).Import([]byte(test.raw), adapter.Options{
				Origin: "https://example.test", RedactionStatus: evidence.RedactionNotRequired,
			})
			if err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
