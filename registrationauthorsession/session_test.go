package registrationauthorsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

var fixedNow = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

type fakeBrowser struct {
	session   *fakeSession
	err       error
	request   BrowserRequest
	openCount int
}

func (browser *fakeBrowser) Open(_ context.Context, request BrowserRequest) (Session, error) {
	browser.openCount++
	browser.request = request
	if browser.session == nil && browser.err == nil {
		browser.session = &fakeSession{}
	}
	return browser.session, browser.err
}

type fakeSession struct {
	observations []RawObservation
	observeErr   error
	navigateErr  error
	closeErr     error
	summary      NetworkSummary
	navigations  []Navigation
	observeCount int
	closeCount   int
}

type failAfterWriter struct {
	remaining int
}

func (writer *failAfterWriter) Write(data []byte) (int, error) {
	if writer.remaining == 0 {
		return 0, errors.New("synthetic output failure")
	}
	writer.remaining--
	return len(data), nil
}

func (session *fakeSession) Observe(_ context.Context) (RawObservation, error) {
	session.observeCount++
	if session.observeErr != nil {
		return RawObservation{}, session.observeErr
	}
	if len(session.observations) == 0 {
		return RawObservation{}, nil
	}
	index := session.observeCount - 1
	if index >= len(session.observations) {
		index = len(session.observations) - 1
	}
	return session.observations[index], nil
}

func (session *fakeSession) Navigate(_ context.Context, navigation Navigation) error {
	session.navigations = append(session.navigations, navigation)
	return session.navigateErr
}

func (session *fakeSession) Close() (NetworkSummary, error) {
	session.closeCount++
	return session.summary, session.closeErr
}

func TestServeCompletesReviewedNoSubmitSession(t *testing.T) {
	profile := validProfileJSON(t)
	registerID := candidateID(1, "button", "Register", 0)
	candidateIDs := []string{registerID}
	sort.Strings(candidateIDs)
	session := &fakeSession{
		observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{
				{BackendID: "private-node-2", Role: "textbox", Label: "password=hunter2", Matches: 1},
				{BackendID: "private-node-1", Role: "button", Label: "Register", Matches: 1},
			},
			Diagnostics: []string{"synthetic_fixture", "synthetic_fixture"},
		}},
		summary: NetworkSummary{Requests: 2, GETRequests: 1, HEADRequests: 1},
	}
	browser := &fakeBrowser{session: session}
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "navigate", Method: "HEAD", URL: "https://app.example.test/register"},
		ClientMessage{Protocol: Protocol, Type: "observe"},
		ClientMessage{Protocol: Protocol, Type: "review", Profile: profile, CandidateIDs: candidateIDs},
		ClientMessage{Protocol: Protocol, Type: "finish"},
	)
	var output bytes.Buffer
	completion, err := Serve(context.Background(), strings.NewReader(input), &output, browser, ServeOptions{Clock: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	if completion == nil || completion.Protocol != Protocol || completion.ProfileID != "synthetic_registration" {
		t.Fatalf("completion = %#v", completion)
	}
	if completion.ObservedAt != fixedNow || completion.Observations != 1 || completion.Network != session.summary {
		t.Fatalf("completion lifecycle = %#v", completion)
	}
	if len(completion.ReviewedCandidates) != 1 || completion.ReviewedCandidates[0].ID != registerID || completion.ReviewedCandidates[0].Generation != 1 {
		t.Fatalf("reviewed candidates = %#v", completion.ReviewedCandidates)
	}
	if got := registrationprofile.Origins(&completion.Profile); len(got) != 1 || got[0] != "https://app.example.test" {
		t.Fatalf("completion origins = %#v", got)
	}
	if !bytes.Equal(completion.ProfileBytes, profile) {
		t.Fatalf("profile bytes are not canonical\ngot:  %s\nwant: %s", completion.ProfileBytes, profile)
	}
	if browser.openCount != 1 || browser.request.URL != "https://app.example.test/register" || !reflect.DeepEqual(browser.request.ApprovedOrigins, []string{"https://app.example.test"}) {
		t.Fatalf("browser request = %#v, opens = %d", browser.request, browser.openCount)
	}
	if len(session.navigations) != 1 || session.navigations[0] != (Navigation{Method: "HEAD", URL: "https://app.example.test/register"}) {
		t.Fatalf("navigations = %#v", session.navigations)
	}
	if session.closeCount != 1 {
		t.Fatalf("close count = %d", session.closeCount)
	}
	transcript := output.String()
	for _, forbidden := range []string{"private-node", "hunter2", "artifactPath", "pageContent", "storageState", "session-cookie"} {
		if strings.Contains(transcript, forbidden) {
			t.Fatalf("transcript contains forbidden value %q: %s", forbidden, transcript)
		}
	}
	if !strings.Contains(transcript, authorsession.RedactedLabel) {
		t.Fatalf("transcript lacks reduced-label marker: %s", transcript)
	}
	assertClosedServerMessages(t, transcript)
}

func TestMessageSpecificFieldsFailBeforeBrowserOpen(t *testing.T) {
	for _, field := range []string{
		"credentialValue", "selector", "script", "pageContent", "capture",
		"storage", "storageState", "session", "cookies", "submit", "postBudget",
	} {
		t.Run(field, func(t *testing.T) {
			browser := &fakeBrowser{}
			line := `{"protocol":"` + Protocol + `","type":"start","profileId":"synthetic_registration","url":"https://app.example.test/register","origins":["https://app.example.test"],"` + field + `":"do-not-retain"}`
			_, output, err := runSession(context.Background(), line+"\n", browser, fixedNow)
			assertFailure(t, err, output, "malformed_message")
			if browser.openCount != 0 {
				t.Fatalf("browser opened for forbidden field %q", field)
			}
			if strings.Contains(output, "do-not-retain") {
				t.Fatalf("forbidden input crossed output boundary: %s", output)
			}
		})
	}
}

func TestStrictDecoderRejectsDuplicateTrailingDeepAndUnknownFields(t *testing.T) {
	tests := map[string]string{
		"duplicate": `{"protocol":"` + Protocol + `","type":"observe","type":"observe"}`,
		"trailing":  `{"protocol":"` + Protocol + `","type":"observe"} {}`,
		"unknown":   `{"protocol":"` + Protocol + `","type":"observe","context":"main"}`,
		"deep":      `{"protocol":"` + Protocol + `","type":"review","profile":` + strings.Repeat("[", 33) + `0` + strings.Repeat("]", 33) + `,"candidateIds":["candidate"]}`,
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeClientMessage([]byte(input)); err == nil {
				t.Fatal("invalid message unexpectedly decoded")
			}
		})
	}
}

func TestProtocolAndPhaseMismatchesFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		messages   []ClientMessage
		diagnostic string
	}{
		{name: "protocol", messages: []ClientMessage{{Protocol: "browsertools.author-session.v2", Type: "close"}}, diagnostic: "protocol_mismatch"},
		{name: "unknown", messages: []ClientMessage{{Protocol: Protocol, Type: "submit"}}, diagnostic: "unknown_message"},
		{name: "observe before start", messages: []ClientMessage{{Protocol: Protocol, Type: "observe"}}, diagnostic: "invalid_state"},
		{name: "review before observation", messages: []ClientMessage{startMessage("https://app.example.test/register"), {Protocol: Protocol, Type: "review", Profile: validProfileJSON(t), CandidateIDs: []string{"candidate"}}}, diagnostic: "invalid_review"},
		{name: "finish before review", messages: []ClientMessage{startMessage("https://app.example.test/register"), {Protocol: Protocol, Type: "finish"}}, diagnostic: "invalid_state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{}
			browser := &fakeBrowser{session: session}
			_, output, err := runSession(context.Background(), ndjson(t, test.messages...), browser, fixedNow)
			assertFailure(t, err, output, test.diagnostic)
			if browser.openCount > 0 && session.closeCount != 1 {
				t.Fatalf("opened session close count = %d", session.closeCount)
			}
		})
	}
}

func TestStartAuthorityValidationPrecedesBrowserOpen(t *testing.T) {
	tests := map[string]ClientMessage{
		"public HTTP": startMessage("http://example.test/register"),
		"query":       startMessage("https://app.example.test/register?account=private"),
		"unapproved":  startMessage("https://other.example.test/register"),
		"bad profile": func() ClientMessage {
			value := startMessage("https://app.example.test/register")
			value.ProfileID = "BAD"
			return value
		}(),
		"bad bounds": func() ClientMessage {
			value := startMessage("https://app.example.test/register")
			value.Bounds = &Bounds{NavigationTimeoutMS: 1, TotalTimeoutMS: 1, MaxRequests: 0, MaxResponseBytes: 1, MaxObservations: 1, MaxCandidates: 1}
			return value
		}(),
	}
	for name, message := range tests {
		t.Run(name, func(t *testing.T) {
			browser := &fakeBrowser{}
			_, output, err := runSession(context.Background(), ndjson(t, message), browser, fixedNow)
			if err == nil || !strings.Contains(output, `"type":"diagnostic"`) {
				t.Fatalf("error = %v, output = %s", err, output)
			}
			if browser.openCount != 0 {
				t.Fatalf("browser opened: %#v", browser.request)
			}
		})
	}
}

func TestNavigationAdmitsOnlyApprovedGETAndHEAD(t *testing.T) {
	tests := []struct {
		name       string
		message    ClientMessage
		diagnostic string
	}{
		{name: "POST", message: ClientMessage{Protocol: Protocol, Type: "navigate", Method: "POST", URL: "https://app.example.test/register"}, diagnostic: "invalid_navigation"},
		{name: "lowercase", message: ClientMessage{Protocol: Protocol, Type: "navigate", Method: "get", URL: "https://app.example.test/register"}, diagnostic: "invalid_navigation"},
		{name: "other origin", message: ClientMessage{Protocol: Protocol, Type: "navigate", Method: "GET", URL: "https://other.example.test/register"}, diagnostic: "invalid_origin"},
		{name: "query", message: ClientMessage{Protocol: Protocol, Type: "navigate", Method: "HEAD", URL: "https://app.example.test/register?private=value"}, diagnostic: "invalid_navigation"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{}
			browser := &fakeBrowser{session: session}
			input := ndjson(t, startMessage("https://app.example.test/register"), test.message)
			_, output, err := runSession(context.Background(), input, browser, fixedNow)
			assertFailure(t, err, output, test.diagnostic)
			if len(session.navigations) != 0 || session.closeCount != 1 {
				t.Fatalf("navigation calls = %#v, close count = %d", session.navigations, session.closeCount)
			}
			if strings.Contains(output, "private=value") {
				t.Fatalf("query crossed output boundary: %s", output)
			}
		})
	}

	session := &fakeSession{}
	browser := &fakeBrowser{session: session}
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "navigate", Method: "GET", URL: "https://app.example.test/register"},
		ClientMessage{Protocol: Protocol, Type: "navigate", Method: "HEAD", URL: "https://app.example.test/help"},
		ClientMessage{Protocol: Protocol, Type: "close"},
	)
	_, _, err := runSession(context.Background(), input, browser, fixedNow)
	if err != nil || len(session.navigations) != 2 || session.navigations[0].Method != "GET" || session.navigations[1].Method != "HEAD" {
		t.Fatalf("GET/HEAD session: navigations=%#v err=%v", session.navigations, err)
	}
}

func TestForbiddenNavigateFieldCannotReachBackend(t *testing.T) {
	session := &fakeSession{}
	browser := &fakeBrowser{session: session}
	input := ndjson(t, startMessage("https://app.example.test/register")) +
		`{"protocol":"` + Protocol + `","type":"navigate","method":"GET","url":"https://app.example.test/register","postBudget":1}` + "\n"
	_, output, err := runSession(context.Background(), input, browser, fixedNow)
	assertFailure(t, err, output, "malformed_message")
	if len(session.navigations) != 0 || session.closeCount != 1 {
		t.Fatalf("navigation calls = %#v, close count = %d", session.navigations, session.closeCount)
	}
}

func TestObservationFailuresAreValueFreeAndFailClosed(t *testing.T) {
	tests := []struct {
		name string
		raw  RawObservation
	}{
		{name: "origin", raw: RawObservation{Origin: "https://other.example.test", Path: "/register"}},
		{name: "path", raw: RawObservation{Origin: "https://app.example.test", Path: "/password=do-not-retain"}},
		{name: "candidate", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{BackendID: "private-do-not-retain", Role: "button", Label: "Register", Matches: 0}}}},
		{name: "diagnostic", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Diagnostics: []string{"backend do-not-retain"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{observations: []RawObservation{test.raw}}
			browser := &fakeBrowser{session: session}
			input := ndjson(t, startMessage("https://app.example.test/register"), ClientMessage{Protocol: Protocol, Type: "observe"})
			_, output, err := runSession(context.Background(), input, browser, fixedNow)
			assertFailure(t, err, output, "invalid_observation")
			if strings.Contains(output, "do-not-retain") || strings.Contains(err.Error(), "do-not-retain") {
				t.Fatalf("backend value leaked: error=%v output=%s", err, output)
			}
			if session.closeCount != 1 {
				t.Fatalf("close count = %d", session.closeCount)
			}
		})
	}
}

func TestBackendFailureAndCancellationReturnFixedDiagnostics(t *testing.T) {
	t.Run("backend prose", func(t *testing.T) {
		session := &fakeSession{observeErr: errors.New("password=do-not-retain")}
		browser := &fakeBrowser{session: session}
		input := ndjson(t, startMessage("https://app.example.test/register"), ClientMessage{Protocol: Protocol, Type: "observe"})
		_, output, err := runSession(context.Background(), input, browser, fixedNow)
		assertFailure(t, err, output, "browser_failure")
		if strings.Contains(output, "do-not-retain") || strings.Contains(err.Error(), "do-not-retain") {
			t.Fatalf("backend error leaked: error=%v output=%s", err, output)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		browser := &fakeBrowser{}
		_, output, err := runSession(ctx, ndjson(t, startMessage("https://app.example.test/register")), browser, fixedNow)
		assertFailure(t, err, output, "canceled")
	})
}

func TestReviewRequiresCurrentPromotableCandidatesAndMatchingProfile(t *testing.T) {
	profile := validProfileJSON(t)
	buttonID := candidateID(1, "button", "Register", 0)
	baseObservation := RawObservation{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []RawCandidate{{BackendID: "backend", Role: "button", Label: "Register", Matches: 1}},
	}
	tests := []struct {
		name       string
		profile    json.RawMessage
		candidates []string
		clock      time.Time
		diagnostic string
	}{
		{name: "unknown candidate", profile: profile, candidates: []string{"candidate-0000000000000000"}, clock: fixedNow, diagnostic: "invalid_candidate"},
		{name: "duplicate candidate", profile: profile, candidates: []string{buttonID, buttonID}, clock: fixedNow, diagnostic: "invalid_candidate"},
		{name: "expired profile", profile: profile, candidates: []string{buttonID}, clock: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), diagnostic: "invalid_profile"},
		{name: "origin mismatch", profile: json.RawMessage(strings.ReplaceAll(string(profile), "app.example.test", "other.example.test")), candidates: []string{buttonID}, clock: fixedNow, diagnostic: "origin_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{observations: []RawObservation{baseObservation}}
			browser := &fakeBrowser{session: session}
			input := ndjson(t,
				startMessage("https://app.example.test/register"),
				ClientMessage{Protocol: Protocol, Type: "observe"},
				ClientMessage{Protocol: Protocol, Type: "review", Profile: test.profile, CandidateIDs: test.candidates},
			)
			_, output, err := runSession(context.Background(), input, browser, test.clock)
			assertFailure(t, err, output, test.diagnostic)
			if session.closeCount != 1 {
				t.Fatalf("close count = %d", session.closeCount)
			}
		})
	}

	t.Run("redacted candidate", func(t *testing.T) {
		redactedID := candidateID(1, "textbox", authorsession.RedactedLabel, 0)
		session := &fakeSession{observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{{BackendID: "backend", Role: "textbox", Label: "password=do-not-retain", Matches: 1}},
		}}}
		browser := &fakeBrowser{session: session}
		input := ndjson(t,
			startMessage("https://app.example.test/register"),
			ClientMessage{Protocol: Protocol, Type: "observe"},
			ClientMessage{Protocol: Protocol, Type: "review", Profile: profile, CandidateIDs: []string{redactedID}},
		)
		_, output, err := runSession(context.Background(), input, browser, fixedNow)
		assertFailure(t, err, output, "invalid_candidate")
		if strings.Contains(output, "do-not-retain") {
			t.Fatalf("raw label leaked: %s", output)
		}
	})
}

func TestNavigationExpiresCandidateGeneration(t *testing.T) {
	profile := validProfileJSON(t)
	oldID := candidateID(1, "button", "Register", 0)
	session := &fakeSession{observations: []RawObservation{{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []RawCandidate{{BackendID: "old", Role: "button", Label: "Register", Matches: 1}},
	}}}
	browser := &fakeBrowser{session: session}
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "observe"},
		ClientMessage{Protocol: Protocol, Type: "navigate", Method: "GET", URL: "https://app.example.test/help"},
		ClientMessage{Protocol: Protocol, Type: "review", Profile: profile, CandidateIDs: []string{oldID}},
	)
	_, output, err := runSession(context.Background(), input, browser, fixedNow)
	assertFailure(t, err, output, "invalid_review")
	if len(session.navigations) != 1 || session.closeCount != 1 {
		t.Fatalf("navigation calls = %#v, close count = %d", session.navigations, session.closeCount)
	}
}

func TestFinishRequiresCleanBoundedNetworkSummary(t *testing.T) {
	profile := validProfileJSON(t)
	buttonID := candidateID(1, "button", "Register", 0)
	tests := []struct {
		name       string
		summary    NetworkSummary
		closeErr   error
		diagnostic string
	}{
		{name: "count mismatch", summary: NetworkSummary{Requests: 2, GETRequests: 1}, diagnostic: "network_policy"},
		{name: "negative", summary: NetworkSummary{Requests: -1}, diagnostic: "network_policy"},
		{name: "teardown", closeErr: errors.New("private browser detail"), diagnostic: "teardown_failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{
				observations: []RawObservation{{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{BackendID: "backend", Role: "button", Label: "Register", Matches: 1}}}},
				summary:      test.summary, closeErr: test.closeErr,
			}
			browser := &fakeBrowser{session: session}
			input := ndjson(t,
				startMessage("https://app.example.test/register"),
				ClientMessage{Protocol: Protocol, Type: "observe"},
				ClientMessage{Protocol: Protocol, Type: "review", Profile: profile, CandidateIDs: []string{buttonID}},
				ClientMessage{Protocol: Protocol, Type: "finish"},
			)
			completion, output, err := runSession(context.Background(), input, browser, fixedNow)
			assertFailure(t, err, output, test.diagnostic)
			if completion != nil || session.closeCount != 1 || strings.Contains(output, "private browser detail") || strings.Contains(err.Error(), "private browser detail") {
				t.Fatalf("completion=%#v close=%d error=%v output=%s", completion, session.closeCount, err, output)
			}
		})
	}
}

func TestExplicitCloseAlsoValidatesNetworkSummary(t *testing.T) {
	session := &fakeSession{summary: NetworkSummary{Requests: 1, GETRequests: 0, HEADRequests: 0}}
	browser := &fakeBrowser{session: session}
	input := ndjson(t, startMessage("https://app.example.test/register"), ClientMessage{Protocol: Protocol, Type: "close"})
	_, output, err := runSession(context.Background(), input, browser, fixedNow)
	assertFailure(t, err, output, "network_policy")
	if session.closeCount != 1 {
		t.Fatalf("close count = %d", session.closeCount)
	}
}

func TestOpenPartialFailureAndOutputFailureCloseSession(t *testing.T) {
	t.Run("open returned session and error", func(t *testing.T) {
		session := &fakeSession{}
		browser := &fakeBrowser{session: session, err: errors.New("private open detail")}
		_, output, err := runSession(context.Background(), ndjson(t, startMessage("https://app.example.test/register")), browser, fixedNow)
		assertFailure(t, err, output, "browser_failure")
		if session.closeCount != 1 || strings.Contains(output, "private open detail") || strings.Contains(err.Error(), "private open detail") {
			t.Fatalf("close=%d error=%v output=%s", session.closeCount, err, output)
		}
	})

	t.Run("state output failed", func(t *testing.T) {
		session := &fakeSession{}
		browser := &fakeBrowser{session: session}
		writer := &failAfterWriter{remaining: 1}
		_, err := Serve(
			context.Background(),
			strings.NewReader(ndjson(t, startMessage("https://app.example.test/register"))),
			writer,
			browser,
			ServeOptions{Clock: func() time.Time { return fixedNow }},
		)
		if err == nil || session.closeCount != 1 {
			t.Fatalf("close=%d error=%v", session.closeCount, err)
		}
	})
}

func TestOversizedProtocolLineFailsWithoutEcho(t *testing.T) {
	browser := &fakeBrowser{}
	privateValue := strings.Repeat("do-not-retain", MaxProtocolLineBytes/len("do-not-retain")+2)
	input := `{"protocol":"` + Protocol + `","type":"start","private":"` + privateValue + `"}` + "\n"
	_, output, err := runSession(context.Background(), input, browser, fixedNow)
	assertFailure(t, err, output, "protocol_limit")
	if browser.openCount != 0 || strings.Contains(output, "do-not-retain") {
		t.Fatalf("opens=%d output=%s", browser.openCount, output)
	}
}

func TestCloseAndUnexpectedEOFAreFailClosed(t *testing.T) {
	t.Run("close before start", func(t *testing.T) {
		browser := &fakeBrowser{}
		completion, output, err := runSession(context.Background(), ndjson(t, ClientMessage{Protocol: Protocol, Type: "close"}), browser, fixedNow)
		if err != nil || completion != nil || browser.openCount != 0 || !strings.Contains(output, `"phase":"closed"`) {
			t.Fatalf("completion=%#v opens=%d error=%v output=%s", completion, browser.openCount, err, output)
		}
	})

	t.Run("unexpected EOF", func(t *testing.T) {
		session := &fakeSession{}
		browser := &fakeBrowser{session: session}
		_, output, err := runSession(context.Background(), ndjson(t, startMessage("https://app.example.test/register")), browser, fixedNow)
		assertFailure(t, err, output, "unexpected_eof")
		if session.closeCount != 1 {
			t.Fatalf("close count = %d", session.closeCount)
		}
	})
}

func TestSessionInterfaceHasNoMutationOrStateExportSurface(t *testing.T) {
	typeOfSession := reflect.TypeOf((*Session)(nil)).Elem()
	if typeOfSession.NumMethod() != 3 {
		t.Fatalf("Session has %d methods", typeOfSession.NumMethod())
	}
	want := []string{"Close", "Navigate", "Observe"}
	for index, name := range want {
		if typeOfSession.Method(index).Name != name {
			t.Fatalf("Session method %d = %q, want %q", index, typeOfSession.Method(index).Name, name)
		}
	}
	for _, forbidden := range []string{"Click", "Focus", "Input", "POST", "Script", "Storage", "Submit", "Export", "AddOrigin"} {
		if _, ok := typeOfSession.MethodByName(forbidden); ok {
			t.Fatalf("Session unexpectedly exposes %s", forbidden)
		}
	}
}

func startMessage(url string) ClientMessage {
	return ClientMessage{
		Protocol: Protocol, Type: "start", ProfileID: "synthetic_registration",
		URL: url, Origins: []string{"https://app.example.test"},
	}
}

func validProfileJSON(t *testing.T) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := registrationprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	data, err = registrationprofile.MarshalJSON(profile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ndjson(t *testing.T, messages ...ClientMessage) string {
	t.Helper()
	var result strings.Builder
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		result.Write(data)
		result.WriteByte('\n')
	}
	return result.String()
}

func runSession(ctx context.Context, input string, browser Browser, now time.Time) (*Completion, string, error) {
	var output bytes.Buffer
	completion, err := Serve(ctx, strings.NewReader(input), &output, browser, ServeOptions{Clock: func() time.Time { return now }})
	return completion, output.String(), err
}

func assertFailure(t *testing.T, err error, output, diagnostic string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), diagnostic) {
		t.Fatalf("error = %v, want diagnostic %q", err, diagnostic)
	}
	if !strings.Contains(output, `"code":"`+diagnostic+`"`) {
		t.Fatalf("output lacks diagnostic %q: %s", diagnostic, output)
	}
}

func assertClosedServerMessages(t *testing.T, transcript string) {
	t.Helper()
	allowed := map[string]map[string]bool{
		"hello":       fields("protocol", "type", "capabilities"),
		"state":       fields("protocol", "type", "bounds", "phase"),
		"observation": fields("protocol", "type", "observation"),
		"diagnostic":  fields("protocol", "type", "diagnostic"),
	}
	decoder := json.NewDecoder(strings.NewReader(transcript))
	for decoder.More() {
		var message map[string]json.RawMessage
		if err := decoder.Decode(&message); err != nil {
			t.Fatal(err)
		}
		var messageType string
		if err := json.Unmarshal(message["type"], &messageType); err != nil {
			t.Fatal(err)
		}
		permitted, ok := allowed[messageType]
		if !ok {
			t.Fatalf("unknown server message type %q", messageType)
		}
		for field := range message {
			if !permitted[field] {
				t.Fatalf("server message %q contains field %q", messageType, field)
			}
		}
	}
}
