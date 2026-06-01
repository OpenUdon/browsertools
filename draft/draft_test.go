package draft

import (
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/evidence"
)

func baseOpts() Options {
	return Options{
		Info: ProfileInfo{
			Title:  "Test Service",
			Origin: "https://example.test",
		},
		ObservationKind: "accessibility_snapshot",
		Confidence:      "medium",
		ExpiresAfter:    "P30D",
	}
}

func baseRecord(hint string) evidence.Record {
	return evidence.Record{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      hint,
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}
}

// TestBuildMinimal tests that a single record with no locators produces a
// valid draft with a navigate sequence and no outputs.
func TestBuildMinimal(t *testing.T) {
	records := []evidence.Record{baseRecord("read_status")}
	result, err := Build(records, baseOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Draft == nil {
		t.Fatal("draft is nil")
	}
	if len(result.ValidationErrors) > 0 {
		t.Errorf("unexpected validation errors: %v", result.ValidationErrors)
	}
	actions, ok := result.Draft["actions"].(map[string]any)
	if !ok || actions["read_status"] == nil {
		t.Error("expected action read_status in draft")
	}
	action := actions["read_status"].(map[string]any)
	if action["description"] == "" {
		t.Error("expected action description scaffold")
	}
	params, ok := action["parameters"].(map[string]any)
	if !ok || params["type"] != "object" {
		t.Fatalf("expected object parameters scaffold, got %+v", action["parameters"])
	}
}

// TestBuildDeterministic verifies that two calls with the same input produce
// identical drafts.
func TestBuildDeterministic(t *testing.T) {
	records := []evidence.Record{
		baseRecord("fetch_list"),
		baseRecord("fetch_detail"),
	}
	r1, err := Build(records, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Build(records, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := MarshalDraft(r1.Draft)
	b2, _ := MarshalDraft(r2.Draft)
	if string(b1) != string(b2) {
		t.Errorf("non-deterministic output:\n  a: %s\n  b: %s", b1, b2)
	}
}

// TestBuildWithLocator tests a record with a single unambiguous locator.
func TestBuildWithLocator(t *testing.T) {
	rec := baseRecord("submit_form")
	rec.CandidateLocators = []evidence.CandidateLocator{
		{Role: "button", Name: "Submit"},
	}
	result, err := Build([]evidence.Record{rec}, baseOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ValidationErrors) > 0 {
		t.Errorf("unexpected validation errors: %v", result.ValidationErrors)
	}
	action := result.Draft["actions"].(map[string]any)["submit_form"].(map[string]any)
	seq := action["sequence"].([]any)
	found := false
	for _, step := range seq {
		s := step.(map[string]any)
		if click, ok := s["click"]; ok {
			loc := click.(map[string]any)["locator"].(map[string]any)
			if loc["role"] == "button" && loc["name"] == "Submit" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected click step with button/Submit locator in sequence")
	}
}

// TestBuildAmbiguousLocatorRefused tests that ambiguous locators without a
// ReviewDecision return an error.
func TestBuildAmbiguousLocatorRefused(t *testing.T) {
	rec := baseRecord("navigate")
	rec.CandidateLocators = []evidence.CandidateLocator{
		{Role: "button", Name: "OK"},
		{Role: "link", Name: "Cancel"},
	}
	_, err := Build([]evidence.Record{rec}, baseOpts())
	if err == nil {
		t.Error("expected error for ambiguous locators, got nil")
	}
}

// TestBuildAmbiguousLocatorResolvedByDecision tests that supplying a
// ReviewDecision resolves the ambiguous locator.
func TestBuildAmbiguousLocatorResolvedByDecision(t *testing.T) {
	rec := baseRecord("navigate")
	rec.CandidateLocators = []evidence.CandidateLocator{
		{Role: "button", Name: "OK"},
		{Role: "link", Name: "Cancel"},
	}
	opts := baseOpts()
	opts.ReviewDecisions = map[string]ReviewDecision{
		"navigate": {ActionHint: "navigate", ChosenLocatorIndex: 1, Note: "prefer link"},
	}
	result, err := Build([]evidence.Record{rec}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	action := result.Draft["actions"].(map[string]any)["navigate"].(map[string]any)
	seq := action["sequence"].([]any)
	found := false
	for _, step := range seq {
		s := step.(map[string]any)
		if click, ok := s["click"]; ok {
			loc := click.(map[string]any)["locator"].(map[string]any)
			if loc["role"] == "link" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected chosen locator (link/Cancel) in sequence")
	}
}

func TestBuildAmbiguousNoteRefusedAfterRoleNameDedup(t *testing.T) {
	rec := baseRecord("save")
	rec.CandidateLocators = []evidence.CandidateLocator{
		{Role: "button", Name: "Save", AmbiguityNote: "two buttons match"},
		{Role: "button", Name: "Save", AmbiguityNote: "two buttons match"},
	}
	_, err := Build([]evidence.Record{rec}, baseOpts())
	if err == nil {
		t.Fatal("expected error for duplicate same-role/name ambiguity, got nil")
	}
}

func TestBuildAmbiguousNoteResolvedByDecision(t *testing.T) {
	rec := baseRecord("save")
	rec.CandidateLocators = []evidence.CandidateLocator{
		{Role: "button", Name: "Save", AmbiguityNote: "two buttons match"},
		{Role: "button", Name: "Save", AmbiguityNote: "two buttons match"},
	}
	opts := baseOpts()
	opts.ReviewDecisions = map[string]ReviewDecision{
		"save": {ActionHint: "save", ChosenLocatorIndex: 0, Note: "use reviewed first match"},
	}
	result, err := Build([]evidence.Record{rec}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	action := result.Draft["actions"].(map[string]any)["save"].(map[string]any)
	seq := action["sequence"].([]any)
	if _, ok := seq[1].(map[string]any)["click"]; !ok {
		t.Fatalf("expected click step, got %+v", seq)
	}
}

// TestBuildWithOutputs tests that candidate outputs become profile outputs.
func TestBuildWithOutputs(t *testing.T) {
	rec := baseRecord("read_page")
	rec.CandidateOutputs = []evidence.CandidateOutput{
		{Key: "title", Type: "string", Source: "microdata", Property: "name"},
	}
	result, err := Build([]evidence.Record{rec}, baseOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	action := result.Draft["actions"].(map[string]any)["read_page"].(map[string]any)
	outputs, ok := action["outputs"].(map[string]any)
	if !ok || outputs["title"] == nil {
		t.Error("expected title output in action")
	}
}

// TestBuildMissingTitle returns an error.
func TestBuildMissingTitle(t *testing.T) {
	opts := baseOpts()
	opts.Info.Title = ""
	_, err := Build([]evidence.Record{baseRecord("x")}, opts)
	if err == nil {
		t.Error("expected error for missing title, got nil")
	}
}

// TestBuildMissingOrigin returns an error.
func TestBuildMissingOrigin(t *testing.T) {
	opts := baseOpts()
	opts.Info.Origin = nil
	_, err := Build([]evidence.Record{baseRecord("x")}, opts)
	if err == nil {
		t.Error("expected error for missing origin, got nil")
	}
}

// TestBuildActionOrdering verifies actions are sorted deterministically.
func TestBuildActionOrdering(t *testing.T) {
	records := []evidence.Record{
		baseRecord("z_action"),
		baseRecord("a_action"),
		baseRecord("m_action"),
	}
	result, err := Build(records, baseOpts())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := MarshalDraft(result.Draft)
	s := string(data)
	aIdx := strings.Index(s, `"a_action"`)
	mIdx := strings.Index(s, `"m_action"`)
	zIdx := strings.Index(s, `"z_action"`)
	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("actions not in alphabetical order: a=%d m=%d z=%d", aIdx, mIdx, zIdx)
	}
}
