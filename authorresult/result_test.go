package authorresult

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/OpenUdon/uws/schemas"
)

func TestBuildUsesOldestSufficientProfilesAndFinalPresence(t *testing.T) {
	request := baseBuildRequest()
	envelope, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	var authentication, capability map[string]any
	if err := json.Unmarshal(envelope.AuthenticationProfile, &authentication); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.CapabilityProfile, &capability); err != nil {
		t.Fatal(err)
	}
	if got := authentication["profile"]; got != "uws.browser-authentication.1.1" {
		t.Fatalf("authentication profile = %v", got)
	}
	if got := capability["profile"]; got != "uws.browser.1.5" {
		t.Fatalf("capability profile = %v", got)
	}
	action := capability["actions"].(map[string]any)["reach_authenticated_goal"].(map[string]any)
	if len(action["sequence"].([]any)) != 1 || action["outputs"].(map[string]any)["goal_present"] == nil {
		t.Fatalf("capability omits final portable wait/presence action: %#v", action)
	}
	flow := authentication["flows"].(map[string]any)["authenticated_goal"].(map[string]any)
	success := flow["success"].(map[string]any)
	if success["path"] != "/dashboard" {
		t.Fatalf("authentication success path = %#v", success["path"])
	}
}

func TestBuiltProfilesValidateAgainstInstalledUWSSchemas(t *testing.T) {
	envelope, err := Build(baseBuildRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.ValidateBrowserSourceProfile(envelope.CapabilityProfile); err != nil {
		t.Fatalf("capability candidate is not valid UWS: %v", err)
	}
	authSchema, schemaErr := schemas.BrowserAuthenticationProfileSchema("uws.browser-authentication.1.1")
	if schemaErr != nil || !bytes.Contains(authSchema, []byte("uws.browser-authentication.1.1")) {
		// Browsertools deliberately keeps its published dependency pin until the
		// coordinated UWS release is available. A standalone checkout of that
		// older pin cannot know 1.1; the repository workspace must validate it.
		t.Skip("published UWS dependency predates browser-authentication 1.1")
	}
	if err := schemas.ValidateBrowserAuthenticationProfile(envelope.AuthenticationProfile); err != nil {
		t.Fatalf("authentication candidate is not valid UWS: %v", err)
	}
}

func TestBuildContextAndPushChallengeRequireNewProfiles(t *testing.T) {
	request := baseBuildRequest()
	request.Contexts = map[string]Context{
		"idp_popup": {Kind: "popup", Parent: "main", Origin: "https://login.example.test"},
	}
	request.Origins = append(request.Origins, "https://login.example.test")
	request.GoalPredicate.Context = "idp_popup"
	request.GoalPredicate.Origin = "https://login.example.test"
	request.GoalProof.Context = "idp_popup"
	request.GoalProof.Origin = "https://login.example.test"
	request.Trace = append(request.Trace, TraceStep{
		Kind: "focus_human_input", Phase: "authentication", Context: "idp_popup", InputKind: "mfa",
	})
	envelope, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	var authentication, capability map[string]any
	_ = json.Unmarshal(envelope.AuthenticationProfile, &authentication)
	_ = json.Unmarshal(envelope.CapabilityProfile, &capability)
	if authentication["profile"] != "uws.browser-authentication.1.1" || capability["profile"] != "uws.browser.1.6" {
		t.Fatalf("new profile versions not selected: %v %v", authentication["profile"], capability["profile"])
	}
	sequence := authentication["flows"].(map[string]any)["authenticated_goal"].(map[string]any)["sequence"].([]any)
	foundPush := false
	for _, raw := range sequence {
		step := raw.(map[string]any)
		if challenge, ok := step["challenge"].(map[string]any); ok && challenge["kind"] == "push" {
			foundPush = true
			if _, leaked := challenge["locator"]; leaked {
				t.Fatal("push challenge must not invent a locator")
			}
		}
	}
	if !foundPush {
		t.Fatal("push challenge was not synthesized")
	}
}

func TestMarshalDeterministicAndDigestBound(t *testing.T) {
	envelope, err := Build(baseBuildRequest())
	if err != nil {
		t.Fatal(err)
	}
	first, err := MarshalDeterministic(envelope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalDeterministic(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || first[len(first)-1] != '\n' {
		t.Fatal("result serialization is not deterministic newline-terminated JSON")
	}
	envelope.AuthenticationProfile = append(json.RawMessage(nil), envelope.AuthenticationProfile...)
	envelope.AuthenticationProfile[1] = 'X'
	if _, err := MarshalDeterministic(envelope); err == nil {
		t.Fatal("tampered profile was accepted")
	}
}

func baseBuildRequest() BuildRequest {
	observedAt := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	return BuildRequest{
		ObservedAt: observedAt,
		Title:      "Member dashboard", Goal: "read account status",
		InitialURL:   "https://members.example.test/login?next=dashboard",
		DashboardURL: "https://members.example.test/dashboard",
		Origins:      []string{"https://members.example.test"},
		Bounds:       Bounds{NavigationTimeoutMS: 20000, TotalTimeoutMS: 600000, MaxRequests: 512, MaxResponseBytes: 32 << 20, MaxObservations: 64, MaxCandidates: 128},
		Trace: []TraceStep{
			{Kind: "focus_human_input", Phase: "authentication", CandidateID: "candidate-user", Context: "main", Role: "textbox", Label: "Email", InputKind: "identifier"},
			{Kind: "focus_human_input", Phase: "authentication", CandidateID: "candidate-password", Context: "main", Role: "textbox", Label: "Password", InputKind: "password"},
			{Kind: "click", Phase: "authentication", CandidateID: "candidate-submit", Context: "main", Role: "button", Label: "Sign in", POSTBudget: 1, POSTObserved: 1},
		},
		GoalPredicate:  GoalPredicate{Origin: "https://members.example.test", Path: "/dashboard", Role: "heading", Label: "Dashboard"},
		GoalProof:      GoalProof{Origin: "https://members.example.test", Path: "/dashboard", Context: "main", Role: "heading", Label: "Dashboard", Matches: 1},
		HumanConfirmed: true,
	}
}
