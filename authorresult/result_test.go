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
		t.Fatalf("pinned UWS dependency lacks browser-authentication 1.1: %v", schemaErr)
	}
	if err := schemas.ValidateBrowserAuthenticationProfile(envelope.AuthenticationProfile); err != nil {
		t.Fatalf("authentication candidate is not valid UWS: %v", err)
	}
}

func TestBuildContextAndPushChallengeRequireNewProfiles(t *testing.T) {
	request := baseBuildRequest()
	request.Contexts = map[string]Context{
		"idp_popup":   {Kind: "popup", Parent: "main", Origin: "https://login.example.test"},
		"login_frame": {Kind: "frame", Parent: "main", Origin: "https://login.example.test", Path: "/embedded/login", Name: "Login"},
	}
	request.Origins = append(request.Origins, "https://login.example.test")
	request.GoalPredicate.Context = "login_frame"
	request.GoalPredicate.Origin = "https://login.example.test"
	request.GoalPredicate.Path = "/embedded/login"
	request.GoalProof.Context = "login_frame"
	request.GoalProof.Origin = "https://login.example.test"
	request.GoalProof.Path = "/embedded/login"
	request.Trace = append(request.Trace, TraceStep{
		Kind: "click", Phase: "authentication", Context: "main", CandidateID: "candidate-sso", Role: "button", Label: "Use SSO", OpensContext: "idp_popup",
	})
	request.Trace = append(request.Trace, TraceStep{
		Kind: "focus_human_input", Phase: "authentication", Context: "idp_popup", InputKind: "mfa", ChallengeKind: "push",
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
	if err := schemas.ValidateBrowserAuthenticationProfile(envelope.AuthenticationProfile); err != nil {
		t.Fatalf("context authentication profile is not valid UWS: %v", err)
	}
	if err := schemas.ValidateBrowserSourceProfile(envelope.CapabilityProfile); err != nil {
		t.Fatalf("context capability profile is not valid UWS: %v", err)
	}
}

func TestBuildSeparatesAuthenticationSuccessFromCompleteExplorationTrace(t *testing.T) {
	request := baseBuildRequest()
	request.Goal = "open account details and read account status"
	request.GoalPredicate = GoalPredicate{Origin: "https://members.example.test", Path: "/account", Role: "status", Label: "Active"}
	request.GoalProof = GoalProof{Origin: "https://members.example.test", Path: "/account", Context: "main", Role: "status", Label: "Active", Matches: 1}
	request.Trace = append(request.Trace,
		TraceStep{Kind: "click", Phase: "exploration", CandidateID: "candidate-account", Context: "main", Role: "link", Label: "Account details"},
		TraceStep{Kind: "navigate", Phase: "exploration", Context: "main", URL: "https://members.example.test/account?private=discarded"},
	)
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
	success := authentication["flows"].(map[string]any)["authenticated_goal"].(map[string]any)["success"].(map[string]any)
	if success["path"] != "/dashboard" || success["origin"] != "https://members.example.test" {
		t.Fatalf("authentication success drifted to final goal: %#v", success)
	}
	sequence := capability["actions"].(map[string]any)["reach_authenticated_goal"].(map[string]any)["sequence"].([]any)
	if len(sequence) != 3 || sequence[0].(map[string]any)["click"] == nil || sequence[1].(map[string]any)["navigate"] != "https://members.example.test/account" || sequence[2].(map[string]any)["wait_for"] == nil {
		t.Fatalf("exploration trace was not preserved before final proof: %#v", sequence)
	}
	if err := schemas.ValidateBrowserSourceProfile(envelope.CapabilityProfile); err != nil {
		t.Fatalf("exploration capability is not valid UWS: %v", err)
	}
}

func TestBuildContextualExplorationKeepsMainGoalUnqualified(t *testing.T) {
	request := baseBuildRequest()
	request.Contexts = map[string]Context{
		"statement_popup": {Kind: "popup", Parent: "main", Origin: "https://members.example.test"},
	}
	request.Trace = append(request.Trace,
		TraceStep{Kind: "click", Phase: "exploration", CandidateID: "candidate-open", Context: "main", Role: "link", Label: "Open statement", OpensContext: "statement_popup"},
		TraceStep{Kind: "click", Phase: "exploration", CandidateID: "candidate-close", Context: "statement_popup", Role: "button", Label: "Done"},
	)
	envelope, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.ValidateBrowserSourceProfile(envelope.CapabilityProfile); err != nil {
		t.Fatalf("contextual exploration with main goal is not valid UWS: %v\n%s", err, envelope.CapabilityProfile)
	}
	var capability map[string]any
	if err := json.Unmarshal(envelope.CapabilityProfile, &capability); err != nil {
		t.Fatal(err)
	}
	if capability["profile"] != "uws.browser.1.6" {
		t.Fatalf("contextual exploration profile = %v", capability["profile"])
	}
	action := capability["actions"].(map[string]any)["reach_authenticated_goal"].(map[string]any)
	sequence := action["sequence"].([]any)
	finalWait := sequence[len(sequence)-1].(map[string]any)["wait_for"].(map[string]any)
	if _, qualified := finalWait["context"]; qualified {
		t.Fatalf("main goal wait was incorrectly context-qualified: %#v", finalWait)
	}
	if _, qualified := action["outputs"].(map[string]any)["goal_present"].(map[string]any)["context"]; qualified {
		t.Fatal("main goal output was incorrectly context-qualified")
	}
}

func TestBuildRejectsPartialAuthenticationProofInsteadOfApplyingCompatibilityFallback(t *testing.T) {
	request := baseBuildRequest()
	request.AuthenticationProof = GoalProof{Origin: request.GoalProof.Origin}
	if _, err := Build(request); err == nil {
		t.Fatal("partial authentication proof was silently replaced by final goal proof")
	}
}

func TestBuildAuthorsEveryReviewedMFAKindExactly(t *testing.T) {
	tests := []struct {
		kind        string
		inputKind   string
		wantLocator bool
		wantSlot    bool
	}{
		{kind: "totp", inputKind: "otp", wantLocator: true, wantSlot: true},
		{kind: "sms_otp", inputKind: "otp", wantLocator: true},
		{kind: "email_otp", inputKind: "otp", wantLocator: true},
		{kind: "voice_otp", inputKind: "otp", wantLocator: true},
		{kind: "push", inputKind: "mfa"},
		{kind: "push_number_match", inputKind: "mfa"},
		{kind: "passkey", inputKind: "mfa"},
		{kind: "security_key", inputKind: "mfa"},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			request := baseBuildRequest()
			request.Trace = append(request.Trace, TraceStep{
				Kind: "focus_human_input", Phase: "authentication", CandidateID: "candidate-0000000000000001",
				Context: "main", Role: "textbox", Label: "Verification code", InputKind: test.inputKind, ChallengeKind: test.kind,
			})
			envelope, err := Build(request)
			if err != nil {
				t.Fatal(err)
			}
			if err := schemas.ValidateBrowserAuthenticationProfile(envelope.AuthenticationProfile); err != nil {
				t.Fatalf("authentication profile: %v\n%s", err, envelope.AuthenticationProfile)
			}
			var authentication map[string]any
			if err := json.Unmarshal(envelope.AuthenticationProfile, &authentication); err != nil {
				t.Fatal(err)
			}
			sequence := authentication["flows"].(map[string]any)["authenticated_goal"].(map[string]any)["sequence"].([]any)
			var challenge map[string]any
			for _, raw := range sequence {
				if value, ok := raw.(map[string]any)["challenge"].(map[string]any); ok {
					challenge = value
				}
			}
			if challenge == nil || challenge["kind"] != test.kind || (challenge["locator"] != nil) != test.wantLocator || (challenge["slot"] != nil) != test.wantSlot {
				t.Fatalf("challenge = %#v", challenge)
			}
			if test.wantSlot {
				slots := authentication["credentialSlots"].(map[string]any)
				if challenge["slot"] != "totp_seed" || slots["totp_seed"].(map[string]any)["kind"] != "totp_seed" {
					t.Fatalf("TOTP slot = %#v %#v", challenge, slots)
				}
			}
		})
	}
}

func TestBuildSelectsOldestSufficientTypedOutputProfile(t *testing.T) {
	tests := []struct {
		name, outputType, context, wantProfile string
	}{
		{name: "string main", outputType: "string", wantProfile: "uws.browser.1.5"},
		{name: "presence main", outputType: "presence", wantProfile: "uws.browser.1.5"},
		{name: "string context", outputType: "string", context: "report_frame", wantProfile: "uws.browser.1.6"},
		{name: "integer", outputType: "integer", wantProfile: "uws.browser.1.7"},
		{name: "number", outputType: "number", wantProfile: "uws.browser.1.7"},
		{name: "boolean text", outputType: "boolean", wantProfile: "uws.browser.1.7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseBuildRequest()
			if test.context != "" {
				request.Contexts = map[string]Context{test.context: {Kind: "frame", Parent: "main", Origin: "https://members.example.test", Name: "Report"}}
				request.GoalPredicate.Context, request.GoalProof.Context = test.context, test.context
			}
			request.OutputSelections = []OutputSelection{{
				CandidateID: "candidate-0000000000000001", Key: "z_value", Type: test.outputType, LocatorMode: "exact_name",
				Observation: 3, Context: normalizedContext(test.context), Role: "status", Name: "Current value", Matches: 1, RoleMatches: 2,
			}}
			envelope, err := Build(request)
			if err != nil {
				t.Fatal(err)
			}
			if err := schemas.ValidateBrowserSourceProfile(envelope.CapabilityProfile); err != nil {
				t.Fatalf("capability profile: %v\n%s", err, envelope.CapabilityProfile)
			}
			var capability map[string]any
			if err := json.Unmarshal(envelope.CapabilityProfile, &capability); err != nil {
				t.Fatal(err)
			}
			if capability["profile"] != test.wantProfile {
				t.Fatalf("profile = %v, want %s", capability["profile"], test.wantProfile)
			}
			output := capability["actions"].(map[string]any)["reach_authenticated_goal"].(map[string]any)["outputs"].(map[string]any)["z_value"].(map[string]any)
			if test.outputType == "presence" {
				if output["type"] != "boolean" || output["presence"] != true {
					t.Fatalf("presence output = %#v", output)
				}
			} else if output["type"] != test.outputType {
				t.Fatalf("typed output = %#v", output)
			}
		})
	}
}

func TestBuildSortsResolvedOutputSelectionsAndRejectsUnsafeProofs(t *testing.T) {
	request := baseBuildRequest()
	request.OutputSelections = []OutputSelection{
		{CandidateID: "candidate-0000000000000002", Key: "zeta", Type: "string", LocatorMode: "exact_name", Observation: 2, Context: "main", Role: "status", Name: "Zeta", Matches: 1, RoleMatches: 2},
		{CandidateID: "candidate-0000000000000001", Key: "alpha", Type: "presence", LocatorMode: "unique_role", Observation: 2, Context: "main", Role: "heading", Matches: 1, RoleMatches: 1},
	}
	envelope, err := Build(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(envelope.OutputSelections) != 2 || envelope.OutputSelections[0].Key != "alpha" || envelope.OutputSelections[1].Key != "zeta" {
		t.Fatalf("output selections = %#v", envelope.OutputSelections)
	}
	request.OutputSelections[0].Key = "access_token"
	if _, err := Build(request); err == nil {
		t.Fatal("secret-shaped output key was accepted")
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
		Bounds:       Bounds{NavigationTimeoutMS: 20000, TotalTimeoutMS: 600000, MaxRequests: 512, MaxResponseBytes: 32 << 20, MaxObservations: 64, MaxCandidates: 128, MaxOutputs: 16},
		Trace: []TraceStep{
			{Kind: "focus_human_input", Phase: "authentication", CandidateID: "candidate-user", Context: "main", Role: "textbox", Label: "Email", InputKind: "identifier"},
			{Kind: "focus_human_input", Phase: "authentication", CandidateID: "candidate-password", Context: "main", Role: "textbox", Label: "Password", InputKind: "password"},
			{Kind: "click", Phase: "authentication", CandidateID: "candidate-submit", Context: "main", Role: "button", Label: "Sign in", POSTBudget: 1, POSTObserved: 1},
		},
		GoalPredicate:       GoalPredicate{Origin: "https://members.example.test", Path: "/dashboard", Role: "heading", Label: "Dashboard"},
		GoalProof:           GoalProof{Origin: "https://members.example.test", Path: "/dashboard", Context: "main", Role: "heading", Label: "Dashboard", Matches: 1},
		AuthenticationProof: GoalProof{Origin: "https://members.example.test", Path: "/dashboard", Context: "main", Role: "heading", Label: "Dashboard", Matches: 1},
		HumanConfirmed:      true,
	}
}
