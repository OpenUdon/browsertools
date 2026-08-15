package draft

import (
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
