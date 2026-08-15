package revalidate

import (
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

var assessmentTime = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

func baseProfile() *profile.Profile {
	return &profile.Profile{
		Schema:          "uws.browser.1.5",
		Info:            profile.Info{Title: "Test", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Evidence:        profile.Evidence{LearnedAt: "2026-01-01T00:00:00Z"},
		Confidence:      profile.ConfidenceMedium,
		ExpiresAfter:    profile.Duration("P30D"),
		Verification:    profile.Verification{LastVerifiedAt: "2026-01-01T00:00:00Z", SuccessfulRuns: 1},
		Actions: map[string]profile.Action{
			"read_status": {
				Sequence:           []profile.Step{{Kind: profile.StepNavigate, Navigate: "/"}},
				SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
				ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
			},
		},
	}
}

func baseRecord() evidence.Record {
	return evidence.Record{
		Origin:          "https://example.test",
		ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt:      "2026-01-01T00:00:00Z",
		ActionHint:      "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
		CandidateLocators: []evidence.CandidateLocator{
			{Role: "status", Name: "OK"},
		},
	}
}

func TestCheckAtCleanNavigateOnly(t *testing.T) {
	result, err := CheckAt(baseProfile(), []evidence.Record{baseRecord()}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected OK, got %+v", result.Failures)
	}
}

func TestCheckAtRequiresAssessmentTime(t *testing.T) {
	if _, err := CheckAt(baseProfile(), []evidence.Record{baseRecord()}, nil, time.Time{}); err == nil {
		t.Fatal("expected zero-time error")
	}
}

func TestCheckMissingActionEvidence(t *testing.T) {
	result, err := CheckAt(baseProfile(), nil, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckMissingEvidence)
}

func TestCheckRejectsNonNormalizedEvidence(t *testing.T) {
	rec := baseRecord()
	rec.RedactionStatus = evidence.RedactionPending
	result, err := CheckAt(baseProfile(), []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckInvalidEvidence)

	rec = baseRecord()
	rec.ObservedAt = "2026-01-01T00:00:00+00:00"
	result, err = CheckAt(baseProfile(), []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckInvalidEvidence)
}

func TestCheckDeclaredLocatorMatching(t *testing.T) {
	prof := baseProfile()
	action := prof.Actions["read_status"]
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Save"}}}}
	prof.Actions["read_status"] = action
	result, err := CheckAt(prof, []evidence.Record{baseRecord()}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckMissingLocator)

	rec := baseRecord()
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Save"}}
	result, err = CheckAt(prof, []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected matching locator to pass, got %+v", result.Failures)
	}
}

func TestCheckAmbiguityRequiresDecisionRationale(t *testing.T) {
	prof := baseProfile()
	action := prof.Actions["read_status"]
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Save"}}}}
	prof.Actions["read_status"] = action
	rec := baseRecord()
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Save", AmbiguityNote: "two matches"}}

	result, err := CheckAt(prof, []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckAmbiguousLocator)

	decisions := []evidence.LocatorDecision{{
		ActionHint: "read_status",
		Locator:    evidence.CandidateLocator{Role: "button", Name: "Save"},
		Rationale:  "reviewed in the Save dialog",
	}}
	result, err = CheckAt(prof, []evidence.Record{rec}, decisions, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected reviewed decision to pass, got %+v", result.Failures)
	}
}

func TestCheckOriginCanonicalizationAndMismatch(t *testing.T) {
	rec := baseRecord()
	rec.Origin = "https://EXAMPLE.test:443"
	result, err := CheckAt(baseProfile(), []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected default-port equivalent origin to pass: %+v", result.Failures)
	}

	rec.Origin = "https://evil.test"
	result, err = CheckAt(baseProfile(), []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckOriginMismatch)
}

func TestCheckExpiryUsesReferenceAwareDuration(t *testing.T) {
	prof := baseProfile()
	prof.Verification.LastVerifiedAt = "2026-01-31T00:00:00Z"
	prof.ExpiresAfter = "P1M"
	result, err := CheckAt(prof, []evidence.Record{baseRecord()}, nil, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckExpired)
}

func TestCheckExpiryRejectsExactExpiryInstant(t *testing.T) {
	prof := baseProfile()
	verifiedAt, err := time.Parse(time.RFC3339, prof.Verification.LastVerifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := prof.ExpiresAfter.AddTo(verifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CheckAt(prof, []evidence.Record{baseRecord()}, nil, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckExpired)
}

func TestSideEffectRequiresWaitAfterFinalAction(t *testing.T) {
	prof := baseProfile()
	action := prof.Actions["read_status"]
	action.SideEffects = []profile.SideEffect{profile.SideEffectUpdatesRecord}
	action.ConfirmationPolicy = profile.ConfirmationPolicy{Required: true}
	action.Sequence = []profile.Step{
		{Kind: profile.StepWaitFor, WaitFor: &profile.WaitForCondition{Locator: &profile.Locator{Role: "status", Name: "Ready"}}},
		{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Save"}}},
	}
	prof.Actions["read_status"] = action
	rec := baseRecord()
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Save"}, {Role: "status", Name: "Ready"}}
	result, err := CheckAt(prof, []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckSideEffectNoSafeWait)

	action.Sequence = append(action.Sequence, profile.Step{Kind: profile.StepWaitFor, WaitFor: &profile.WaitForCondition{Locator: &profile.Locator{Role: "status", Name: "Saved"}}})
	prof.Actions["read_status"] = action
	rec.CandidateLocators = append(rec.CandidateLocators, evidence.CandidateLocator{Role: "status", Name: "Saved"})
	result, err = CheckAt(prof, []evidence.Record{rec}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("expected final wait to pass, got %+v", result.Failures)
	}
}

func TestA11yOutputLocatorNeedsEvidence(t *testing.T) {
	prof := baseProfile()
	action := prof.Actions["read_status"]
	action.Outputs = map[string]profile.Output{"status": {
		Type: profile.OutputString, Source: profile.OutputA11y,
		Locator: &profile.Locator{Role: "status", Name: "System status"},
	}}
	prof.Actions["read_status"] = action
	result, err := CheckAt(prof, []evidence.Record{baseRecord()}, nil, assessmentTime)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, result, CheckMissingLocator)
}

func assertFailure(t *testing.T, result Result, kind CheckKind) {
	t.Helper()
	for _, failure := range result.Failures {
		if failure.Kind == kind {
			return
		}
	}
	t.Fatalf("expected failure %q, got %+v", kind, result.Failures)
}
