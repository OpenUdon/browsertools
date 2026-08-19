package profile

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/uws/schemas"
)

// TestSchemaCompiles ensures the embedded schema is well-formed and compilable.
func TestSchemaCompiles(t *testing.T) {
	data, err := schemas.BrowserSourceProfileSchema("uws.browser.1.5")
	if err != nil || len(data) == 0 {
		t.Fatalf("pinned UWS schema failed to load: %v", err)
	}
}

// TestValidateValidFixtures checks that the known-good fixtures validate.
func TestValidateValidFixtures(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "valid_*.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no valid_* fixtures found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := LoadFile(path); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}

// TestValidateInvalidFixtures checks that each focused invalid fixture is
// rejected. The fixture name documents the violated rule.
func TestValidateInvalidFixtures(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "invalid_*.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no invalid_* fixtures found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := LoadFile(path); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

// TestValidateExampleProfiles validates the committed example profiles against
// the embedded schema, proving the shipped examples stay schema-clean.
func TestValidateExampleProfiles(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "examples", "*", "browser-profiles", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("no example browser-profiles found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := LoadFile(path); err != nil {
				t.Errorf("example profile failed validation: %v", err)
			}
		})
	}
}

// TestJSONYAMLRoundTrip proves JSON and YAML inputs validate identically and
// decode to the same typed Profile.
func TestJSONYAMLRoundTrip(t *testing.T) {
	jsonPath := filepath.Join("testdata", "valid_minimal.json")
	yamlPath := filepath.Join("testdata", "valid_minimal.yaml")

	if _, err := LoadFile(jsonPath); err != nil {
		t.Fatalf("json fixture failed validation: %v", err)
	}
	if _, err := LoadFile(yamlPath); err != nil {
		t.Fatalf("yaml fixture failed validation: %v", err)
	}

	fromJSON, err := LoadFile(jsonPath)
	if err != nil {
		t.Fatalf("load json: %v", err)
	}
	fromYAML, err := LoadFile(yamlPath)
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}

	if fromJSON.Schema != fromYAML.Schema {
		t.Errorf("profile mismatch: json=%q yaml=%q", fromJSON.Schema, fromYAML.Schema)
	}
	if fromJSON.Info.Title != fromYAML.Info.Title {
		t.Errorf("title mismatch: json=%q yaml=%q", fromJSON.Info.Title, fromYAML.Info.Title)
	}
	if len(fromJSON.Actions) != len(fromYAML.Actions) {
		t.Errorf("action count mismatch: json=%d yaml=%d", len(fromJSON.Actions), len(fromYAML.Actions))
	}
}

// TestLoadUnsupportedExtension exercises the load error path.
func TestLoadUnsupportedExtension(t *testing.T) {
	if _, err := LoadFile("profile.txt"); err == nil {
		t.Error("expected error for unsupported extension, got nil")
	}
}

func TestTypedModelRoundTripsEveryMacro(t *testing.T) {
	nav := NavigationLoad
	prof := Profile{
		Schema:          "uws.browser.1.5",
		Info:            Info{Title: "All macros", Origin: Origins{"https://example.test"}},
		ObservationKind: ObservationAccessibilitySnapshot,
		Evidence:        Evidence{LearnedAt: "2026-01-01T00:00:00Z"},
		Confidence:      ConfidenceHigh, ExpiresAfter: "P30D",
		Verification: Verification{LastVerifiedAt: "2026-01-01T00:00:00Z", SuccessfulRuns: 1},
		Actions: map[string]Action{"all": {
			Parameters: JSONSchema{"type": "object"},
			Sequence: []Step{
				{Kind: StepNavigate, Navigate: "/form"},
				{Kind: StepClick, Click: &LocatorStep{Locator: Locator{Role: "button", Name: "Open"}}},
				{Kind: StepTypeText, TypeText: &TypeTextStep{Locator: Locator{Role: "textbox", Name: "Name"}, Value: "{{name}}"}},
				{Kind: StepCheckRadio, CheckRadio: &LocatorStep{Locator: Locator{Role: "radio", Name: "One"}}},
				{Kind: StepUncheck, Uncheck: &LocatorStep{Locator: Locator{Role: "checkbox", Name: "Old"}}},
				{Kind: StepSelectOption, SelectOption: &SelectOptionStep{Locator: Locator{Role: "combobox", Name: "Choice"}, Value: "one"}},
				{Kind: StepWaitFor, WaitFor: &WaitForCondition{Navigation: &nav}},
			},
			Outputs: map[string]Output{
				"status":   {Type: OutputString, Source: OutputA11y, Locator: &Locator{Role: "status", Name: "Saved"}},
				"data":     {Type: OutputObject, Source: OutputJSONLD, Property: "data"},
				"item":     {Type: OutputString, Source: OutputMicrodata, Property: "item"},
				"fallback": {Type: OutputString, Source: OutputCSS, Selector: ".value", FallbackReason: "no_structured_data", Validation: JSONSchema{"type": "string"}},
			},
			SideEffects:        []SideEffect{SideEffectStateChange},
			ConfirmationPolicy: ConfirmationPolicy{Required: true, Prompt: "Save?"},
		}},
	}
	data, err := json.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	steps := decoded.Actions["all"].Sequence
	if len(steps) != 7 || steps[5].Kind != StepSelectOption || steps[6].WaitFor.Navigation == nil {
		t.Fatalf("typed macro union did not round trip: %+v", steps)
	}
	yamlData, err := MarshalYAML(*decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYAML(yamlData); err != nil {
		t.Fatalf("YAML round trip: %v\n%s", err, yamlData)
	}
	if _, err := ParseYAML(append(yamlData, []byte("\n---\nignored: true\n")...)); err == nil {
		t.Fatal("multiple YAML documents unexpectedly accepted")
	}
}

func TestLiteralOriginSafety(t *testing.T) {
	prof, err := LoadFile(filepath.Join("testdata", "valid_minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	action := prof.Actions["read_status"]
	action.Sequence[0].Navigate = "https://evil.test/status"
	prof.Actions["read_status"] = action
	value, err := prof.Value()
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(value); err == nil {
		t.Fatal("expected literal off-origin navigation rejection")
	}

	for _, target := range []string{
		"https://evil.test/status?q={{term}}",
		"https://{{host}}/status",
		"https://example.test/status?q={{term",
	} {
		action.Sequence[0].Navigate = target
		prof.Actions["read_status"] = action
		value, err = prof.Value()
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(value); err == nil {
			t.Fatalf("templated target %q unexpectedly accepted", target)
		}
	}
	action.Sequence[0].Navigate = "https://example.test/status?q={{term}}"
	prof.Actions["read_status"] = action
	value, err = prof.Value()
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(value); err != nil {
		t.Fatalf("same-origin path/query template rejected: %v", err)
	}
}

func TestMultipleOriginsRejectRelativeNavigation(t *testing.T) {
	prof, err := LoadFile(filepath.Join("testdata", "valid_minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	prof.Info.Origin = Origins{"https://example.test", "https://account.example.test"}
	action := prof.Actions["read_status"]
	for _, target := range []string{"/status", "/status/{{item}}"} {
		action.Sequence[0].Navigate = target
		prof.Actions["read_status"] = action
		value, valueErr := prof.Value()
		if valueErr != nil {
			t.Fatal(valueErr)
		}
		err = Validate(value)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("relative target %q validation error = %v", target, err)
		}
	}
	action.Sequence[0].Navigate = "https://account.example.test/status/{{item}}"
	prof.Actions["read_status"] = action
	value, err := prof.Value()
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(value); err != nil {
		t.Fatalf("absolute target with multiple origins rejected: %v", err)
	}
}

func TestOriginCanonicalization(t *testing.T) {
	got, err := ParseOrigin("HTTPS://EXAMPLE.test:443/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.test" {
		t.Fatalf("got %q", got)
	}
	allowed := Origins{"https://example.test"}
	ok, err := allowed.ContainsURL("https://EXAMPLE.test:443/path?q=1")
	if err != nil || !ok {
		t.Fatalf("equivalent URL rejected: ok=%v err=%v", ok, err)
	}
}

func TestSupportedProfileVersionsUsePinnedSchemasAndFutureVersionFails(t *testing.T) {
	prof, err := LoadFile(filepath.Join("testdata", "valid_minimal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"uws.browser.1.5", "uws.browser.1.6", "uws.browser.1.7"} {
		value, err := prof.Value()
		if err != nil {
			t.Fatal(err)
		}
		value["profile"] = version
		if err := Validate(value); err != nil {
			t.Fatalf("supported version %s rejected: %v", version, err)
		}
	}
	value, err := prof.Value()
	if err != nil {
		t.Fatal(err)
	}
	value["profile"] = "uws.browser.1.8"
	if err := Validate(value); err == nil || !strings.Contains(err.Error(), "unsupported browser profile discriminator") {
		t.Fatalf("future version did not fail explicitly: %v", err)
	}
}

func TestDurationAddToCalendarComponents(t *testing.T) {
	reference := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	got, err := Duration("P1M").AddTo(reference)
	if err != nil {
		t.Fatal(err)
	}
	want := reference.AddDate(0, 1, 0)
	if !got.Equal(want) {
		t.Fatalf("P1M=%s want %s", got, want)
	}
	got, err = Duration("P1Y2DT3H").AddTo(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC).AddDate(1, 0, 2).Add(3 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("combined duration=%s want %s", got, want)
	}
	if _, err := Duration("PT1M1H").AddTo(reference); err == nil {
		t.Fatal("expected out-of-order duration rejection")
	}
}
