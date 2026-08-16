package guide

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

var guideNow = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

func TestAuthorBuildsDeterministicStrictBundle(t *testing.T) {
	catalog, err := NewCatalog(validRecords())
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	first, err := Author(catalog, intent, guideNow)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Author(catalog, intent, guideNow)
	if err != nil {
		t.Fatal(err)
	}
	left, err := MarshalDeterministic(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := MarshalDeterministic(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left, right) {
		t.Fatal("same accepted intent and evidence produced different bytes")
	}
	if !first.Review.Promotable() {
		t.Fatalf("review is not promotable: %#v", first.Review.Gaps)
	}
	if got := first.Profile.Actions["lookup"].Outputs["headline"].Property; got != "headline" {
		t.Fatalf("selected output was not retained: %q", got)
	}
	if first.Spec.Actions["lookup"].Outputs == nil {
		t.Fatal("guided output decision must always be explicit")
	}
	if len(first.Decisions) != 0 {
		t.Fatalf("unexpected decisions: %#v", first.Decisions)
	}
	var decoded any
	if err := json.Unmarshal(left, &decoded); err != nil {
		t.Fatalf("bundle JSON is invalid: %v", err)
	}
}

func TestAuthorDoesNotMutateCallerIntent(t *testing.T) {
	catalog, err := NewCatalog(validRecords())
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	intent.Info.Origin = profile.Origins{"https://example.test:443"}
	before, _ := json.Marshal(intent)
	if _, err := Author(catalog, intent, guideNow); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(intent)
	if !bytes.Equal(before, after) {
		t.Fatalf("caller intent mutated:\n%s\n%s", before, after)
	}
}

func TestAuthorPreservesExplicitNoOutputs(t *testing.T) {
	catalog, err := NewCatalog(validRecords())
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	intent.Actions[0].OutputIDs = []string{}
	bundle, err := Author(catalog, intent, guideNow)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Spec.Actions["lookup"].Outputs == nil || len(bundle.Spec.Actions["lookup"].Outputs) != 0 {
		t.Fatalf("spec did not retain explicit empty outputs: %#v", bundle.Spec.Actions["lookup"].Outputs)
	}
	if len(bundle.Profile.Actions["lookup"].Outputs) != 0 {
		t.Fatalf("candidate outputs were inferred after explicit none: %#v", bundle.Profile.Actions["lookup"].Outputs)
	}
	data, err := MarshalDeterministic(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"outputs": {}`)) {
		t.Fatalf("serialized guided spec lost explicit no-output decision: %s", data)
	}
	var decoded Bundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := draft.Build(decoded.Evidence, decoded.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Profile.Actions["lookup"].Outputs) != 0 {
		t.Fatal("round-tripped guided spec inferred outputs")
	}
}

func TestAuthorRequiresAmbiguityDecision(t *testing.T) {
	records := validRecords()
	records[0].CandidateLocators[0].AmbiguityNote = "two buttons share the same name"
	catalog, err := NewCatalog(records)
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	if _, err := Author(catalog, intent, guideNow); err == nil || !strings.Contains(err.Error(), "requires a non-empty rationale") {
		t.Fatalf("expected ambiguity rejection, got %v", err)
	}
	intent.Actions[0].AmbiguityResolutions = []AmbiguityResolution{{LocatorID: "E001.L001", Rationale: "Reviewed the unique surrounding heading."}}
	bundle, err := Author(catalog, intent, guideNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Decisions) != 1 || bundle.Decisions[0].Rationale == "" {
		t.Fatalf("ambiguity decision missing: %#v", bundle.Decisions)
	}
}

func TestAuthorRejectsMutationWithoutConfirmationAndFutureEvidence(t *testing.T) {
	catalog, err := NewCatalog(validRecords())
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	intent.Actions[0].SideEffects = []profile.SideEffect{profile.SideEffectStateChange}
	if _, err := Author(catalog, intent, guideNow); err == nil || !strings.Contains(err.Error(), "require explicit confirmation") {
		t.Fatalf("expected confirmation rejection, got %v", err)
	}
	intent = validIntent()
	if _, err := Author(catalog, intent, guideNow.Add(-time.Hour)); err == nil || !strings.Contains(err.Error(), "future") {
		t.Fatalf("expected future-evidence rejection, got %v", err)
	}
}

func TestRunWizardPresentsCandidatesAndBuildsBundle(t *testing.T) {
	answers := strings.Join([]string{
		"Example lookup",
		"Example provider",
		"no",
		"O001",
		"accessibility_snapshot",
		"high",
		"P14D",
		"1",
		"lookup",
		"Read a reviewed status.",
		"E001",
		"0",
		"E001.O001",
		"1",
		"click",
		"E001.L001",
		"none",
		"read_only",
		"no",
	}, "\n") + "\n"
	var prompts bytes.Buffer
	bundle, err := RunWizard(strings.NewReader(answers), &prompts, validRecords(), guideNow)
	if err != nil {
		t.Fatalf("wizard failed: %v\nprompts:\n%s", err, prompts.String())
	}
	for _, expected := range []string{
		"Reviewed evidence candidates", "E001.L001 role=\"button\" name=\"Look up\"",
		"E001.O001 key=\"headline\"", "side effects", "expiry duration",
	} {
		if !strings.Contains(prompts.String(), expected) {
			t.Fatalf("prompt does not contain %q:\n%s", expected, prompts.String())
		}
	}
	if !bundle.Review.Promotable() {
		t.Fatal("wizard bundle did not pass strict review")
	}
}

func TestCatalogRejectsPendingAndSecretShapedEvidence(t *testing.T) {
	records := validRecords()
	records[0].RedactionStatus = evidence.RedactionPending
	if _, err := NewCatalog(records); err == nil || !strings.Contains(err.Error(), "pending redaction") {
		t.Fatalf("expected pending rejection, got %v", err)
	}
	records = validRecords()
	records[0].Provenance.Session = "access_token=abcdefghijklmnop"
	if _, err := NewCatalog(records); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("expected secret-shaped rejection, got %v", err)
	}
}

func TestCatalogIDsAreIndependentOfEquivalentInputOrder(t *testing.T) {
	leftRecords := validRecords()
	second := validRecords()[0]
	second.CandidateLocators = []evidence.CandidateLocator{{Role: "link", Name: "Details"}}
	second.CandidateOutputs = nil
	leftRecords = append(leftRecords, second)
	rightRecords := []evidence.Record{second, validRecords()[0]}
	left, err := NewCatalog(leftRecords)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewCatalog(rightRecords)
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if !bytes.Equal(leftJSON, rightJSON) {
		t.Fatalf("catalog IDs depend on input order:\n%s\n%s", leftJSON, rightJSON)
	}
}

func TestAuthorRejectsMismatchedObservationAndUnsafeTemplates(t *testing.T) {
	catalog, err := NewCatalog(validRecords())
	if err != nil {
		t.Fatal(err)
	}
	intent := validIntent()
	intent.ObservationKind = profile.ObservationDOMText
	if _, err := Author(catalog, intent, guideNow); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected observation mismatch, got %v", err)
	}

	for _, target := range []string{
		"https://outside.test/items/{{item}}",
		"/items/{{missing}}",
		"/items/{{broken}",
		"/items?token={{item}}",
	} {
		intent = validIntent()
		intent.Actions[0].Parameters = []ParameterIntent{{Name: "item", Type: "string", Required: true}}
		intent.Actions[0].Sequence = []StepIntent{{Kind: profile.StepNavigate, Navigate: target}}
		if _, err := Author(catalog, intent, guideNow); err == nil {
			t.Fatalf("expected unsafe template rejection for %q", target)
		}
	}
}

func TestAuthorRejectsCredentialShapedOutput(t *testing.T) {
	records := validRecords()
	records[0].CandidateOutputs[0].Key = "password"
	records[0].CandidateOutputs[0].Property = "password"
	if _, err := NewCatalog(records); err == nil || !strings.Contains(err.Error(), "credential-shaped") {
		t.Fatalf("expected credential-shaped output rejection, got %v", err)
	}
}

func validRecords() []evidence.Record {
	return []evidence.Record{{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: guideNow.Format(time.RFC3339), RedactionStatus: evidence.RedactionNotRequired,
		CandidateLocators: []evidence.CandidateLocator{{Role: "button", Name: "Look up"}},
		CandidateOutputs:  []evidence.CandidateOutput{{Key: "headline", Type: "string", Source: "jsonld", Property: "headline"}},
		Provenance:        evidence.Provenance{Tool: "synthetic-test", Version: "1"},
	}}
}

func validIntent() Intent {
	return Intent{
		Info:            profile.Info{Title: "Example lookup", Provider: "example", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Confidence:      profile.ConfidenceHigh, ExpiresAfter: "P14D",
		Actions: []ActionIntent{{
			ID: "lookup", Description: "Read a reviewed status.", EvidenceIDs: []string{"E001"},
			Parameters: []ParameterIntent{},
			Sequence:   []StepIntent{{Kind: profile.StepClick, LocatorID: "E001.L001"}},
			OutputIDs:  []string{"E001.O001"}, SideEffects: []profile.SideEffect{profile.SideEffectReadOnly},
			ConfirmationPolicy:   profile.ConfirmationPolicy{Required: false},
			AmbiguityResolutions: []AmbiguityResolution{},
		}},
	}
}
