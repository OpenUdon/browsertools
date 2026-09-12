package registrationauthor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationdraft"
)

func TestBuildV3UsesReviewedHistoryAndCopiesMetadata(t *testing.T) {
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	completion, err := registrationfixture.Author(t.Context(), &registrationfixture.Browser{}, "https://app.example.test", at)
	if err != nil {
		t.Fatal(err)
	}
	var spec registrationdraft.Spec
	if err := json.Unmarshal(completion.ProfileBytes, &spec); err != nil {
		t.Fatal(err)
	}
	selected := []string{}
	submit := ""
	for _, candidate := range completion.ReviewedCandidates {
		selected = append(selected, candidate.ID)
		if candidate.Label == "Register" {
			submit = candidate.ID
		}
	}
	request := BuildRequest{Protocol: registrationauthorsession.ProtocolV3, ProfileID: completion.ProfileID, Spec: spec, ApprovedOrigins: completion.Origins, ReviewedCandidateIDs: selected, SubmitCandidateID: submit, Flow: completion.Flow, Controls: CallControls{ApprovalSymbol: ApprovalSymbol, DuplicatePrevention: DuplicatePrevention, OnDuplicate: OnDuplicate, AmbiguousOutcome: AmbiguousOutcome, CleanupDisposition: CleanupDelete}, AssessedAt: at, History: completion.History, Previews: completion.Previews, StepCandidates: completion.StepCandidates}
	candidate, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.ReviewMessage().StepCandidates) != len(completion.StepCandidates) {
		t.Fatal("step proof lost")
	}
	observation := candidate.Observation()
	for _, control := range observation.Candidates {
		if control.Control != nil {
			control.Control.Kind = "changed"
		}
	}
	for _, control := range candidate.Observation().Candidates {
		if control.Control != nil && control.Control.Kind == "changed" {
			t.Fatal("candidate observation aliases mutable metadata")
		}
	}
	for _, item := range request.History[len(request.History)-1].Candidates {
		if item.Control != nil {
			item.Control.Kind = "changed"
		}
	}
	for _, item := range candidate.Observation().Candidates {
		if item.Control != nil && item.Control.Kind == "changed" {
			t.Fatal("candidate aliases the builder's input history")
		}
	}
	request.Protocol = registrationauthorsession.ProtocolV2
	if _, err := Build(request); err == nil {
		t.Fatal("legacy builder accepted v3 evidence")
	}
}
