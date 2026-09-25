package draft

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

func baseRecord(action string) evidence.Record {
	return evidence.Record{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: "2026-01-01T00:00:00Z", ActionHint: action,
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}
}

func baseSpec() Spec {
	return Spec{
		Info:            profile.Info{Title: "Test", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Confidence:      profile.ConfidenceMedium,
		ExpiresAfter:    "P30D",
		Actions: map[string]ActionSpec{
			"read_status": {
				Sequence:           []profile.Step{{Kind: profile.StepNavigate, Navigate: "/status"}},
				SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
				ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
			},
		},
	}
}

func TestBuildRequiresExplicitActionIntent(t *testing.T) {
	spec := baseSpec()
	spec.Actions = nil
	if _, err := Build([]evidence.Record{baseRecord("read_status")}, spec); err == nil {
		t.Fatal("expected missing action specification error")
	}

	spec = baseSpec()
	action := spec.Actions["read_status"]
	action.SideEffects = nil
	spec.Actions["read_status"] = action
	if _, err := Build([]evidence.Record{baseRecord("read_status")}, spec); err == nil {
		t.Fatal("expected missing sideEffects error")
	}
}

func TestBuildDoesNotInventStepsOrSafety(t *testing.T) {
	result, err := Build([]evidence.Record{baseRecord("read_status")}, baseSpec())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReadyForReview() {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
	action := result.Profile.Actions["read_status"]
	if len(action.Sequence) != 1 || action.Sequence[0].Navigate != "/status" {
		t.Fatalf("sequence was changed: %+v", action.Sequence)
	}
	if len(action.SideEffects) != 1 || action.SideEffects[0] != profile.SideEffectReadOnly {
		t.Fatalf("side effects were changed: %+v", action.SideEffects)
	}
}

func TestBuildChoosesOldestSufficientTemplateVersion(t *testing.T) {
	for _, tc := range []struct {
		navigate, want string
		optIn          bool
	}{
		{"/status", profile.SchemaV15, false},
		{"/status/{{id}}", profile.SchemaV15, false},
		{"/status", profile.SchemaV15, true},
		{"/status/{{id}}", profile.SchemaV18, true},
		{"/status/{{{{literal}}}}/{{id}}", profile.SchemaV19, true},
	} {
		spec := baseSpec()
		spec.VersionedTemplates = tc.optIn
		action := spec.Actions["read_status"]
		action.Parameters = profile.JSONSchema{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}}
		action.Sequence[0].Navigate = tc.navigate
		spec.Actions["read_status"] = action
		result, err := Build([]evidence.Record{baseRecord("read_status")}, spec)
		if err != nil {
			t.Fatalf("%s: %v", tc.want, err)
		}
		if result.Profile.Schema != tc.want {
			t.Fatalf("got %s, want %s", result.Profile.Schema, tc.want)
		}
	}
}

func TestBuildSelectsAndRoundTripsBrowser110MatchCountOutput(t *testing.T) {
	for _, versionedTemplates := range []bool{false, true} {
		t.Run(map[bool]string{false: "count only", true: "count and template"}[versionedTemplates], func(t *testing.T) {
			spec := baseSpec()
			spec.VersionedTemplates = versionedTemplates
			action := spec.Actions["read_status"]
			if versionedTemplates {
				action.Parameters = profile.JSONSchema{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}}
				action.Sequence[0].Navigate = "/status/{{id}}"
			}
			action.Outputs = map[string]profile.Output{
				"card_count": {
					Type: profile.OutputInteger, Source: profile.OutputCSS, Selector: ".card",
					FallbackReason: profile.FallbackNoA11yRegion, MatchCount: true,
					Within: ".results", Visibility: profile.OutputVisibilityRendered,
					Validation: profile.JSONSchema{
						"type": "integer", "minimum": json.Number("0"), "maximum": json.Number("24"),
					},
				},
			}
			spec.Actions["read_status"] = action

			record := baseRecord("read_status")
			// Candidate evidence cannot override explicit count intent or add an
			// extracted text/attribute field to the authored output.
			record.CandidateOutputs = []evidence.CandidateOutput{{
				Key: "candidate_status", Type: "string", Source: "microdata", Property: "status",
			}}
			result, err := Build([]evidence.Record{record}, spec)
			if err != nil {
				t.Fatal(err)
			}
			if !result.ReadyForReview() {
				t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
			}
			if result.Profile.Schema != profile.SchemaV110 {
				t.Fatalf("got schema %s, want %s", result.Profile.Schema, profile.SchemaV110)
			}
			outputs := result.Profile.Actions["read_status"].Outputs
			if len(outputs) != 1 {
				t.Fatalf("candidate evidence changed explicit outputs: %#v", outputs)
			}
			count := outputs["card_count"]
			if count.Type != profile.OutputInteger || count.Source != profile.OutputCSS || !count.MatchCount ||
				count.Within != ".results" || count.Visibility != profile.OutputVisibilityRendered ||
				count.Attribute != "" || count.Property != "" {
				t.Fatalf("count output changed or retained an extraction field: %+v", count)
			}
			if minimum, ok := count.Validation["minimum"].(json.Number); !ok || minimum != "0" {
				t.Fatalf("zero lower bound changed: %#v", count.Validation["minimum"])
			}
			if maximum, ok := count.Validation["maximum"].(json.Number); !ok || maximum != "24" {
				t.Fatalf("multiple-count upper bound changed: %#v", count.Validation["maximum"])
			}

			data, err := MarshalProfile(result.Profile)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := profile.ParseJSON(data)
			if err != nil {
				t.Fatalf("parse drafted Browser 1.10 profile: %v", err)
			}
			if decoded.Schema != profile.SchemaV110 {
				t.Fatalf("round-trip schema changed: %s", decoded.Schema)
			}
			decodedOutput := decoded.Actions["read_status"].Outputs["card_count"]
			got, marshalErr := json.Marshal(decodedOutput)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			want, marshalErr := json.Marshal(count)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("count output changed on round-trip: got %s, want %s", got, want)
			}
			roundTrip, err := MarshalProfile(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, roundTrip) {
				t.Fatalf("Browser 1.10 draft bytes changed on round-trip:\n%s\n%s", data, roundTrip)
			}

			yamlData, err := profile.MarshalYAML(*result.Profile)
			if err != nil {
				t.Fatal(err)
			}
			yamlProfile, err := profile.ParseYAML(yamlData)
			if err != nil {
				t.Fatalf("parse drafted Browser 1.10 YAML profile: %v", err)
			}
			yamlJSON, err := MarshalProfile(yamlProfile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, yamlJSON) {
				t.Fatalf("Browser 1.10 YAML round-trip changed typed profile:\n%s\n%s", data, yamlJSON)
			}
		})
	}
}

func TestBuildRejectsInvalidBrowser110CountOutput(t *testing.T) {
	spec := baseSpec()
	action := spec.Actions["read_status"]
	action.Outputs = map[string]profile.Output{
		"invalid_count": {
			Type: profile.OutputString, Source: profile.OutputCSS, Selector: ".card",
			FallbackReason: profile.FallbackNoA11yRegion, MatchCount: true,
			Visibility: profile.OutputVisibilityAll,
			Validation: profile.JSONSchema{"type": "integer", "minimum": json.Number("0")},
		},
	}
	spec.Actions["read_status"] = action
	result, err := Build([]evidence.Record{baseRecord("read_status")}, spec)
	if err == nil || result == nil {
		t.Fatal("invalid Browser 1.10 count output was accepted")
	}
	if result.ReadyForReview() {
		t.Fatal("invalid Browser 1.10 draft was reported ready for review")
	}
}

func TestBuildKeepsBrowser18IntegerDefaultExact(t *testing.T) {
	const exact = "9223372036854775807"
	spec := baseSpec()
	spec.VersionedTemplates = true
	action := spec.Actions["read_status"]
	action.Parameters = profile.JSONSchema{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer", "default": json.Number(exact)}}}
	action.Sequence[0].Navigate = "/status/{{id}}"
	spec.Actions["read_status"] = action
	result, err := Build([]evidence.Record{baseRecord("read_status")}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile.Schema != profile.SchemaV18 {
		t.Fatalf("got %s", result.Profile.Schema)
	}
	defaultValue := result.Profile.Actions["read_status"].Parameters["properties"].(map[string]any)["id"].(map[string]any)["default"]
	if number, ok := defaultValue.(json.Number); !ok || string(number) != exact {
		t.Fatalf("draft rounded default: %T %v", defaultValue, defaultValue)
	}
}

func TestBuildRequiresDeclaredLocatorEvidence(t *testing.T) {
	spec := baseSpec()
	action := spec.Actions["read_status"]
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Refresh"}}}}
	spec.Actions["read_status"] = action
	result, err := Build([]evidence.Record{baseRecord("read_status")}, spec)
	if err == nil || result == nil {
		t.Fatal("expected blocking locator diagnostic")
	}
	if result.ReadyForReview() {
		t.Fatal("invalid result reported ready")
	}

	rec := baseRecord("read_status")
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Refresh"}}
	result, err = Build([]evidence.Record{rec}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ReadyForReview() {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
}

func TestBuildCarriesAmbiguityDecision(t *testing.T) {
	spec := baseSpec()
	action := spec.Actions["read_status"]
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Refresh"}}}}
	spec.Actions["read_status"] = action
	rec := baseRecord("read_status")
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Refresh", AmbiguityNote: "two matches"}}

	if _, err := Build([]evidence.Record{rec}, spec); err == nil {
		t.Fatal("expected unresolved ambiguity")
	}
	spec.Decisions = []evidence.LocatorDecision{{
		ActionHint: "read_status", Locator: evidence.CandidateLocator{Role: "button", Name: "Refresh"},
		Rationale: "reviewed inside the status region",
	}}
	result, err := Build([]evidence.Record{rec}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Decisions) != 1 || result.Decisions[0].Rationale == "" {
		t.Fatalf("decision not retained: %+v", result.Decisions)
	}
}

func TestBuildImportsCandidateOutputsOnly(t *testing.T) {
	rec := baseRecord("read_status")
	rec.CandidateOutputs = []evidence.CandidateOutput{{Key: "status", Type: "string", Source: "microdata", Property: "status"}}
	result, err := Build([]evidence.Record{rec}, baseSpec())
	if err != nil {
		t.Fatal(err)
	}
	out := result.Profile.Actions["read_status"].Outputs["status"]
	if out.Source != profile.OutputMicrodata || out.Property != "status" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestBuildPreservesExplicitNoOutputs(t *testing.T) {
	rec := baseRecord("read_status")
	rec.CandidateOutputs = []evidence.CandidateOutput{{Key: "status", Type: "string", Source: "microdata", Property: "status"}}
	spec := baseSpec()
	action := spec.Actions["read_status"]
	action.Outputs = map[string]profile.Output{}
	spec.Actions["read_status"] = action
	result, err := Build([]evidence.Record{rec}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profile.Actions["read_status"].Outputs) != 0 {
		t.Fatalf("explicit no-output decision inferred candidates: %#v", result.Profile.Actions["read_status"].Outputs)
	}
}

func TestBuildWriteActionRequiresFinalWait(t *testing.T) {
	spec := baseSpec()
	action := spec.Actions["read_status"]
	action.SideEffects = []profile.SideEffect{profile.SideEffectUpdatesRecord}
	action.ConfirmationPolicy = profile.ConfirmationPolicy{Required: true, Prompt: "Refresh?"}
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Refresh"}}}}
	spec.Actions["read_status"] = action
	rec := baseRecord("read_status")
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Refresh"}, {Role: "status", Name: "Updated"}}
	if _, err := Build([]evidence.Record{rec}, spec); err == nil {
		t.Fatal("expected missing safe wait")
	}

	action.Sequence[0].Click.WaitFor = &profile.WaitForCondition{Locator: &profile.Locator{Role: "status", Name: "Updated"}}
	spec.Actions["read_status"] = action
	if _, err := Build([]evidence.Record{rec}, spec); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDeterministic(t *testing.T) {
	r1, err := Build([]evidence.Record{baseRecord("read_status")}, baseSpec())
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Build([]evidence.Record{baseRecord("read_status")}, baseSpec())
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := MarshalProfile(r1.Profile)
	b2, _ := MarshalProfile(r2.Profile)
	if strings.Compare(string(b1), string(b2)) != 0 {
		t.Fatalf("profiles differ:\n%s\n%s", b1, b2)
	}
}
