package review

import (
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

var reviewedAt = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

func baseProfile() *profile.Profile {
	return &profile.Profile{
		Schema:          "uws.browser.1.5",
		Info:            profile.Info{Title: "Test", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Evidence:        profile.Evidence{LearnedAt: "2026-01-01T00:00:00Z", Source: "synthetic"},
		Confidence:      profile.ConfidenceMedium, ExpiresAfter: "P30D",
		Verification: profile.Verification{LastVerifiedAt: "2026-01-01T00:00:00Z", SuccessfulRuns: 1},
		Actions: map[string]profile.Action{"read_status": {
			Sequence:           []profile.Step{{Kind: profile.StepNavigate, Navigate: "/status"}},
			SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
			ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
		}},
	}
}

func baseRecord() evidence.Record {
	return evidence.Record{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: "2026-01-01T00:00:00Z", ActionHint: "read_status",
		RedactionStatus: evidence.RedactionNotRequired,
		Provenance:      evidence.Provenance{Tool: "synthetic"},
	}
}

func TestBuildPromotableAndVerify(t *testing.T) {
	prof, records := baseProfile(), []evidence.Record{baseRecord()}
	bundle, err := Build(prof, records, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable() {
		t.Fatalf("expected promotable bundle: %+v", bundle.Gaps)
	}
	if bundle.ProfileDigest == "" || bundle.EvidenceDigest == "" {
		t.Fatal("missing digests")
	}
	if err := Verify(bundle, prof, records, reviewedAt); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAndRevalidationCannotDisagree(t *testing.T) {
	bundle, err := Build(baseProfile(), nil, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Promotable() || bundle.Revalidation.OK {
		t.Fatal("missing evidence should block both gates")
	}
	if len(bundle.Gaps) == 0 || bundle.Gaps[0].Kind != "missing_evidence" {
		t.Fatalf("unexpected gaps: %+v", bundle.Gaps)
	}
}

func TestVerifyRejectsDigestChanges(t *testing.T) {
	prof, records := baseProfile(), []evidence.Record{baseRecord()}
	bundle, err := Build(prof, records, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	changed := *prof
	changed.Info.Title = "Changed"
	if err := Verify(bundle, &changed, records, reviewedAt); err == nil {
		t.Fatal("expected profile digest mismatch")
	}
	records[0].Provenance.Tool = "changed"
	if err := Verify(bundle, prof, records, reviewedAt); err == nil {
		t.Fatal("expected evidence digest mismatch")
	}
}

func TestVerifyRechecksFreshness(t *testing.T) {
	prof, records := baseProfile(), []evidence.Record{baseRecord()}
	bundle, err := Build(prof, records, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(bundle, prof, records, time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected stale bundle rejection")
	}
}

func TestDecisionPersistedAndUsed(t *testing.T) {
	prof := baseProfile()
	action := prof.Actions["read_status"]
	action.Sequence = []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{Locator: profile.Locator{Role: "button", Name: "Refresh"}}}}
	prof.Actions["read_status"] = action
	rec := baseRecord()
	rec.CandidateLocators = []evidence.CandidateLocator{{Role: "button", Name: "Refresh", AmbiguityNote: "two matches"}}
	decision := evidence.LocatorDecision{
		ActionHint: "read_status", Locator: evidence.CandidateLocator{Role: "button", Name: "Refresh"}, Rationale: "reviewed target",
	}
	bundle, err := Build(prof, []evidence.Record{rec}, []evidence.LocatorDecision{decision}, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable() {
		t.Fatalf("decision should resolve ambiguity: %+v", bundle.Gaps)
	}
	if len(bundle.Decisions) != 1 || bundle.Decisions[0].Rationale == "" {
		t.Fatalf("decision missing: %+v", bundle.Decisions)
	}
}

func TestBuildDeterministicForSameAssessment(t *testing.T) {
	prof := baseProfile()
	records := []evidence.Record{baseRecord()}
	a, err := Build(prof, records, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(prof, records, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	if a.ProfileDigest != b.ProfileDigest || a.EvidenceDigest != b.EvidenceDigest || a.AssessedAt != b.AssessedAt {
		t.Fatalf("bundle identity is not deterministic: %+v %+v", a, b)
	}
}

func TestBuildSnapshotsProfile(t *testing.T) {
	prof := baseProfile()
	bundle, err := Build(prof, []evidence.Record{baseRecord()}, nil, reviewedAt)
	if err != nil {
		t.Fatal(err)
	}
	prof.Info.Title = "mutated"
	action := prof.Actions["read_status"]
	action.Sequence[0].Navigate = "/mutated"
	prof.Actions["read_status"] = action
	if bundle.Profile.Info.Title == "mutated" || bundle.Profile.Actions["read_status"].Sequence[0].Navigate == "/mutated" {
		t.Fatal("bundle profile shares mutable state with caller")
	}
}
