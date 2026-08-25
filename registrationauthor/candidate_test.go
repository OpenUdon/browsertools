package registrationauthor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationdraft"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
	"gopkg.in/yaml.v3"
)

var assessedAt = time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)

func TestBuildDeterministicExplicitCandidate(t *testing.T) {
	request := validBuildRequest(t)
	before, err := json.Marshal(request.Spec)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(validBuildRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.ProfileBytes(), second.ProfileBytes()) {
		t.Fatal("canonical profile bytes are not deterministic")
	}
	after, err := json.Marshal(request.Spec)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("build mutated explicit spec: %v", err)
	}
	profileValue, err := first.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if err := registrationprofile.ValidateAt(&profileValue, assessedAt); err != nil {
		t.Fatal(err)
	}
	if first.ProfileID() != "synthetic_registration" || first.Flow() != "create_dedicated_test_user" ||
		first.SubmitCandidateID() != request.SubmitCandidateID {
		t.Fatalf("candidate identity=%q %q %q", first.ProfileID(), first.Flow(), first.SubmitCandidateID())
	}
	message := first.ReviewMessage()
	if message.Protocol != registrationauthorsession.Protocol || message.Type != "review" ||
		message.Flow != first.Flow() || message.CleanupDisposition != CleanupDelete ||
		!reflect.DeepEqual(message.CandidateIDs, request.ReviewedCandidateIDs) ||
		!bytes.Equal(message.Profile, first.ProfileBytes()) {
		t.Fatalf("review message=%#v", message)
	}
	if first.Controls() != request.Controls || !reflect.DeepEqual(first.ApprovedOrigins(), request.ApprovedOrigins) {
		t.Fatalf("controls=%#v origins=%#v", first.Controls(), first.ApprovedOrigins())
	}
}

func TestCandidateAccessorsReturnCopies(t *testing.T) {
	candidate, err := Build(validBuildRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	profileBytes := candidate.ProfileBytes()
	profileBytes[0] = 'x'
	message := candidate.ReviewMessage()
	message.Profile[0] = 'x'
	message.CandidateIDs[0] = "candidate-ffffffffffffffff"
	observation := candidate.Observation()
	observation.Candidates[0].Label = "Changed"
	observation.Diagnostics = append(observation.Diagnostics, "changed")
	origins := candidate.ApprovedOrigins()
	origins[0] = "https://other.example.test"

	fresh := candidate.ReviewMessage()
	wantIDs := validBuildRequest(t).ReviewedCandidateIDs
	if fresh.Profile[0] != '{' || !reflect.DeepEqual(fresh.CandidateIDs, wantIDs) ||
		candidate.Observation().Candidates[0].Label != "Register" ||
		!reflect.DeepEqual(candidate.ApprovedOrigins(), []string{"https://app.example.test"}) {
		t.Fatal("candidate accessor leaked mutable backing state")
	}
}

func TestCandidateReviewMessageCompletesM26Session(t *testing.T) {
	request := validBuildRequest(t)
	candidate, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	input := encodeMessages(t,
		registrationauthorsession.ClientMessage{
			Protocol: registrationauthorsession.Protocol, Type: "start",
			ProfileID: candidate.ProfileID(), URL: "https://app.example.test/register",
			Origins: candidate.ApprovedOrigins(),
		},
		registrationauthorsession.ClientMessage{Protocol: registrationauthorsession.Protocol, Type: "observe"},
		candidate.ReviewMessage(),
		registrationauthorsession.ClientMessage{Protocol: registrationauthorsession.Protocol, Type: "finish"},
	)
	var output bytes.Buffer
	completion, err := registrationauthorsession.Serve(
		context.Background(), io.NopCloser(bytes.NewReader(input)), &output,
		integrationBrowser{}, registrationauthorsession.ServeOptions{Clock: func() time.Time { return assessedAt }},
	)
	if err != nil {
		t.Fatalf("session failed: %v\n%s", err, output.String())
	}
	if completion == nil || completion.Flow != candidate.Flow() ||
		!bytes.Equal(completion.ProfileBytes, candidate.ProfileBytes()) ||
		len(completion.ReviewedCandidates) != len(request.ReviewedCandidateIDs) {
		t.Fatalf("completion=%#v", completion)
	}
}

func TestBuildRejectsMissingOrInferredDecisions(t *testing.T) {
	tests := map[string]func(*BuildRequest){
		"profile identity":     func(value *BuildRequest) { value.ProfileID = "" },
		"flow identity":        func(value *BuildRequest) { value.Flow = "" },
		"assessment precision": func(value *BuildRequest) { value.AssessedAt = value.AssessedAt.Add(time.Nanosecond) },
		"approval symbol":      func(value *BuildRequest) { value.Controls.ApprovalSymbol = "" },
		"duplicate decision":   func(value *BuildRequest) { value.Controls.DuplicatePrevention = "" },
		"duplicate outcome":    func(value *BuildRequest) { value.Controls.OnDuplicate = "" },
		"ambiguous outcome":    func(value *BuildRequest) { value.Controls.AmbiguousOutcome = "" },
		"cleanup":              func(value *BuildRequest) { value.Controls.CleanupDisposition = "" },
		"credential slots":     func(value *BuildRequest) { value.Spec.CredentialSlots = nil },
		"flow":                 func(value *BuildRequest) { value.Spec.Flows = nil },
		"success": func(value *BuildRequest) {
			flow := value.Spec.Flows[value.Flow]
			flow.Success = browserregistration.SuccessCondition{}
			value.Spec.Flows[value.Flow] = flow
		},
		"submit": func(value *BuildRequest) {
			flow := value.Spec.Flows[value.Flow]
			flow.Sequence = append([]browserregistration.Step(nil), flow.Sequence[:3]...)
			flow.Sequence = append(flow.Sequence, value.Spec.Flows[value.Flow].Sequence[4:]...)
			value.Spec.Flows[value.Flow] = flow
		},
		"submit name": func(value *BuildRequest) {
			flow := value.Spec.Flows[value.Flow]
			flow.Sequence[3].Submit.Locator.Name = "Continue"
			value.Spec.Flows[value.Flow] = flow
		},
		"evidence time": func(value *BuildRequest) {
			value.Spec.Evidence.LearnedAt = assessedAt.Add(-time.Second).Format(time.RFC3339)
		},
		"verification time": func(value *BuildRequest) {
			value.Spec.Verification.LastVerifiedAt = assessedAt.Add(-time.Second).Format(time.RFC3339)
		},
	}
	keys := make([]string, 0, len(tests))
	for name := range tests {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		t.Run(name, func(t *testing.T) {
			request := validBuildRequest(t)
			tests[name](&request)
			if _, err := Build(request); err == nil {
				t.Fatal("missing explicit decision was inferred")
			}
		})
	}
}

func TestBuildRejectsObservationAndSelectionDrift(t *testing.T) {
	tests := map[string]func(*BuildRequest){
		"generation bound":    func(value *BuildRequest) { value.Observation.Generation = 0 },
		"generation mismatch": func(value *BuildRequest) { value.Observation.Generation = 2 },
		"origin":              func(value *BuildRequest) { value.Observation.Origin = "https://other.example.test" },
		"path":                func(value *BuildRequest) { value.Observation.Path = "/password=do-not-retain" },
		"candidate order": func(value *BuildRequest) {
			value.Observation.Candidates[0], value.Observation.Candidates[1] = value.Observation.Candidates[1], value.Observation.Candidates[0]
		},
		"candidate ID":        func(value *BuildRequest) { value.Observation.Candidates[0].ID = "candidate-invalid" },
		"candidate role":      func(value *BuildRequest) { value.Observation.Candidates[0].Role = "script" },
		"candidate label":     func(value *BuildRequest) { value.Observation.Candidates[0].Label = authorsession.RedactedLabel },
		"candidate ambiguity": func(value *BuildRequest) { value.Observation.Candidates[0].Matches = 2 },
		"duplicate locator": func(value *BuildRequest) {
			value.Observation.Candidates[1].Role = value.Observation.Candidates[0].Role
			value.Observation.Candidates[1].Label = value.Observation.Candidates[0].Label
		},
		"diagnostic order": func(value *BuildRequest) {
			value.Observation.Diagnostics = []string{"unsupported_accessible_control", "accessibility_snapshot_partial"}
		},
		"diagnostic vocabulary": func(value *BuildRequest) { value.Observation.Diagnostics = []string{"private backend detail"} },
		"review order": func(value *BuildRequest) {
			value.ReviewedCandidateIDs[0], value.ReviewedCandidateIDs[1] = value.ReviewedCandidateIDs[1], value.ReviewedCandidateIDs[0]
		},
		"review duplicate": func(value *BuildRequest) { value.ReviewedCandidateIDs[1] = value.ReviewedCandidateIDs[0] },
		"unreviewed submit": func(value *BuildRequest) {
			for _, id := range value.ReviewedCandidateIDs {
				if id != value.SubmitCandidateID {
					value.ReviewedCandidateIDs = []string{id}
					return
				}
			}
		},
		"unknown submit":   func(value *BuildRequest) { value.SubmitCandidateID = "candidate-ffffffffffffffff" },
		"origin inventory": func(value *BuildRequest) { value.ApprovedOrigins = []string{"https://other.example.test"} },
	}
	keys := make([]string, 0, len(tests))
	for name := range tests {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		t.Run(name, func(t *testing.T) {
			request := validBuildRequest(t)
			tests[name](&request)
			if _, err := Build(request); err == nil {
				t.Fatal("drifted observation or selection was accepted")
			}
		})
	}
}

func TestBuildCarriesNoValueOrRuntimeField(t *testing.T) {
	candidate, err := Build(validBuildRequest(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(candidate.ReviewMessage())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"credentialValue", "accountIdentifier", "verificationValue", "cookie",
		"storageState", "pageContent", "rawWorkerOutput", "privatePath",
		"artifactPath", `"session":`, `"runtime":`, "executed",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("review message contains forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func validBuildRequest(t *testing.T) BuildRequest {
	t.Helper()
	data, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec registrationdraft.Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	buttonID := candidateID(1, "button", "Register", 0)
	textboxID := candidateID(1, "textbox", "Password", 1)
	reviewedIDs := []string{buttonID, textboxID}
	sort.Strings(reviewedIDs)
	return BuildRequest{
		ProfileID: "synthetic_registration", Spec: spec,
		Observation: registrationauthorsession.Observation{
			Generation: 1, Origin: "https://app.example.test", Path: "/register",
			Candidates: []registrationauthorsession.Candidate{
				{ID: buttonID, Role: "button", Label: "Register", Matches: 1},
				{ID: textboxID, Role: "textbox", Label: "Password", Matches: 1},
			},
			Diagnostics: []string{"synthetic_fixture"},
		},
		ApprovedOrigins:      []string{"https://app.example.test"},
		ReviewedCandidateIDs: reviewedIDs,
		SubmitCandidateID:    buttonID, Flow: "create_dedicated_test_user",
		Controls: CallControls{
			ApprovalSymbol: ApprovalSymbol, DuplicatePrevention: DuplicatePrevention,
			OnDuplicate: OnDuplicate, AmbiguousOutcome: AmbiguousOutcome,
			CleanupDisposition: CleanupDelete,
		},
		AssessedAt: assessedAt,
	}
}

type integrationBrowser struct{}

func (integrationBrowser) Open(context.Context, registrationauthorsession.BrowserRequest) (registrationauthorsession.Session, error) {
	return integrationSession{}, nil
}

type integrationSession struct{}

func (integrationSession) Observe(context.Context) (registrationauthorsession.RawObservation, error) {
	return registrationauthorsession.RawObservation{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []registrationauthorsession.RawCandidate{
			{Role: "button", Label: "Register", Matches: 1},
			{Role: "textbox", Label: "Password", Matches: 1},
		},
		Diagnostics: []string{"synthetic_fixture"},
	}, nil
}

func (integrationSession) Navigate(context.Context, registrationauthorsession.Navigation) error {
	return nil
}

func (integrationSession) Close(context.Context) (registrationauthorsession.NetworkSummary, error) {
	return registrationauthorsession.NetworkSummary{Requests: 1, GETRequests: 1}, nil
}

func encodeMessages(t *testing.T, messages ...registrationauthorsession.ClientMessage) []byte {
	t.Helper()
	var result bytes.Buffer
	encoder := json.NewEncoder(&result)
	for _, message := range messages {
		if err := encoder.Encode(message); err != nil {
			t.Fatal(err)
		}
	}
	return result.Bytes()
}
