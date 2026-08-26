package registrationauthorsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
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
	closeBlocked bool
	summary      NetworkSummary
	navigations  []Navigation
	observeCount int
	closeCount   int
}

type failAfterWriter struct {
	remaining int
}

type stagedBlockingInput struct {
	data      []byte
	position  int
	readAgain chan struct{}
	closed    chan struct{}
	once      sync.Once
}

func (input *stagedBlockingInput) Read(destination []byte) (int, error) {
	if input.position < len(input.data) {
		count := copy(destination, input.data[input.position:])
		input.position += count
		return count, nil
	}
	input.once.Do(func() { close(input.readAgain) })
	<-input.closed
	return 0, io.ErrClosedPipe
}

func (input *stagedBlockingInput) Close() error {
	select {
	case <-input.closed:
	default:
		close(input.closed)
	}
	return nil
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

func (session *fakeSession) Close(ctx context.Context) (NetworkSummary, error) {
	session.closeCount++
	if session.closeBlocked {
		<-ctx.Done()
		return NetworkSummary{}, ctx.Err()
	}
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
				{Role: "textbox", Label: "password=hunter2", Matches: 1},
				{Role: "button", Label: "Register", Matches: 1},
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
		reviewMessage(profile, candidateIDs),
		ClientMessage{Protocol: Protocol, Type: "finish"},
	)
	var output bytes.Buffer
	completion, err := Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &output, browser, ServeOptions{Clock: func() time.Time { return fixedNow }})
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
	if completion.Flow != "create_dedicated_test_user" || completion.CleanupDisposition != "delete_separately" {
		t.Fatalf("reviewed call controls = %q %q", completion.Flow, completion.CleanupDisposition)
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

func TestV2RetainsStructuralQueryOnlyInsideBrowserNavigation(t *testing.T) {
	profile := validProfileJSON(t)
	registerID := candidateID(1, "button", "Register", 0)
	session := &fakeSession{
		observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
		}},
		summary: NetworkSummary{Requests: 2, GETRequests: 2},
	}
	browser := &fakeBrowser{session: session}
	messages := []ClientMessage{
		{Protocol: ProtocolV2, Type: "start", ProfileID: "synthetic_registration", URL: "https://app.example.test/register?action=startnew", Origins: []string{"https://app.example.test"}},
		{Protocol: ProtocolV2, Type: "navigate", Method: "GET", URL: "https://app.example.test/register?action=startnew"},
		{Protocol: ProtocolV2, Type: "observe"},
		{Protocol: ProtocolV2, Type: "review", Profile: profile, CandidateIDs: []string{registerID}, Flow: "create_dedicated_test_user", CleanupDisposition: "delete_separately"},
		{Protocol: ProtocolV2, Type: "finish"},
	}
	var input strings.Builder
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		input.Write(data)
		input.WriteByte('\n')
	}
	var output bytes.Buffer
	completion, err := Serve(context.Background(), io.NopCloser(strings.NewReader(input.String())), &output, browser, ServeOptions{Clock: func() time.Time { return fixedNow }, Protocol: ProtocolV2})
	if err != nil {
		t.Fatal(err)
	}
	if completion == nil || completion.Protocol != ProtocolV2 || browser.request.Protocol != ProtocolV2 || browser.request.URL != "https://app.example.test/register?action=startnew" {
		t.Fatalf("completion=%#v request=%#v", completion, browser.request)
	}
	if len(session.navigations) != 1 || session.navigations[0].URL != "https://app.example.test/register?action=startnew" {
		t.Fatalf("navigations = %#v", session.navigations)
	}
	if strings.Contains(output.String(), "action") || strings.Contains(output.String(), "startnew") || !strings.Contains(output.String(), `"protocol":"`+ProtocolV2+`"`) {
		t.Fatalf("v2 transcript leaked or mislabeled query: %s", output.String())
	}
}

func TestV2RejectsUnsafeQueryBeforeBrowserWorkWithoutEcho(t *testing.T) {
	for _, raw := range []string{
		"https://app.example.test/register?token=do-not-retain",
		"https://app.example.test/register?action=do-not-retain#fragment",
	} {
		browser := &fakeBrowser{}
		message := ClientMessage{Protocol: ProtocolV2, Type: "start", ProfileID: "synthetic_registration", URL: raw, Origins: []string{"https://app.example.test"}}
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		_, err = Serve(context.Background(), io.NopCloser(strings.NewReader(string(data)+"\n")), &output, browser, ServeOptions{Clock: func() time.Time { return fixedNow }, Protocol: ProtocolV2})
		assertFailure(t, err, output.String(), "invalid_start")
		if browser.openCount != 0 || strings.Contains(output.String(), "do-not-retain") || strings.Contains(err.Error(), "do-not-retain") {
			t.Fatalf("unsafe query crossed boundary: opens=%d error=%v output=%s", browser.openCount, err, output.String())
		}
	}
}

func TestServeRejectsUnsupportedConfiguredProtocolBeforeOutput(t *testing.T) {
	var output bytes.Buffer
	_, err := Serve(context.Background(), io.NopCloser(strings.NewReader("")), &output, &fakeBrowser{}, ServeOptions{Protocol: "browsertools.registration-author-session.v3"})
	if err == nil || output.Len() != 0 {
		t.Fatalf("error=%v output=%q", err, output.String())
	}
}

func TestCompletionUsesAcceptedObservationTime(t *testing.T) {
	profile := validProfileJSON(t)
	registerID := candidateID(1, "button", "Register", 0)
	session := &fakeSession{
		observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
		}},
		summary: NetworkSummary{Requests: 1, GETRequests: 1},
	}
	times := []time.Time{fixedNow.Add(time.Minute), fixedNow.Add(2 * time.Minute)}
	clockCalls := 0
	clock := func() time.Time {
		value := times[clockCalls]
		clockCalls++
		return value
	}
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "observe"},
		reviewMessage(profile, []string{registerID}),
		ClientMessage{Protocol: Protocol, Type: "finish"},
	)
	var output bytes.Buffer
	completion, err := Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &output, &fakeBrowser{session: session}, ServeOptions{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if completion.ObservedAt != times[0] || clockCalls != 2 {
		t.Fatalf("observation time = %v, clock calls = %d", completion.ObservedAt, clockCalls)
	}
}

func TestCompletionCanonicalizesRealClockPrecision(t *testing.T) {
	profile := validProfileJSON(t)
	registerID := candidateID(1, "button", "Register", 0)
	rawNow := fixedNow.Add(987654321 * time.Nanosecond)
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "observe"},
		reviewMessage(profile, []string{registerID}),
		ClientMessage{Protocol: Protocol, Type: "finish"},
	)
	completion, _, err := runSession(context.Background(), input, &fakeBrowser{session: &fakeSession{
		observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
		}},
	}}, rawNow)
	if err != nil {
		t.Fatal(err)
	}
	if completion.ObservedAt != fixedNow || completion.ObservedAt.Nanosecond() != 0 {
		t.Fatalf("canonical observation time=%v", completion.ObservedAt)
	}
}

func TestMessageSpecificFieldsFailBeforeBrowserOpen(t *testing.T) {
	for _, field := range []string{
		"credentialValue", "selector", "script", "pageContent", "capture",
		"storage", "storageState", "session", "cookies", "submit", "postBudget",
		"credentialBindings", "approval", "duplicatePrevention", "onDuplicate", "ambiguousOutcome",
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
		{name: "review before observation", messages: []ClientMessage{startMessage("https://app.example.test/register"), reviewMessage(validProfileJSON(t), []string{"candidate"})}, diagnostic: "invalid_review"},
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
		"unsorted origins": func() ClientMessage {
			value := startMessage("https://app.example.test/register")
			value.Origins = []string{"https://other.example.test", "https://app.example.test"}
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
		{name: "candidate", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 0}}}},
		{name: "duplicate reduced locator", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{
			{Role: "button", Label: "Register", Matches: 1},
			{Role: "button", Label: "Register", Matches: 1},
		}}},
		{name: "invalid UTF-8 label", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{Role: "button", Label: string([]byte{0xff}), Matches: 1}}}},
		{name: "oversized raw label", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{Role: "button", Label: strings.Repeat("x", MaxRawCandidateLabelBytes+1), Matches: 1}}}},
		{name: "diagnostic count", raw: RawObservation{Origin: "https://app.example.test", Path: "/register", Diagnostics: repeatDiagnostic(DiagnosticSyntheticFixture, MaxUniqueDiagnostics+1)}},
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
		Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
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
				reviewMessage(test.profile, test.candidates),
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
			Candidates: []RawCandidate{{Role: "textbox", Label: "password=do-not-retain", Matches: 1}},
		}}}
		browser := &fakeBrowser{session: session}
		input := ndjson(t,
			startMessage("https://app.example.test/register"),
			ClientMessage{Protocol: Protocol, Type: "observe"},
			reviewMessage(profile, []string{redactedID}),
		)
		_, output, err := runSession(context.Background(), input, browser, fixedNow)
		assertFailure(t, err, output, "invalid_candidate")
		if strings.Contains(output, "do-not-retain") {
			t.Fatalf("raw label leaked: %s", output)
		}
	})
}

func TestReviewRequiresExistingFlowAndExplicitCleanupDisposition(t *testing.T) {
	profile := validProfileJSON(t)
	buttonID := candidateID(1, "button", "Register", 0)
	observation := RawObservation{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
	}
	for _, test := range []struct {
		name       string
		mutate     func(*ClientMessage)
		diagnostic string
	}{
		{name: "missing flow", mutate: func(message *ClientMessage) { message.Flow = "missing" }, diagnostic: "invalid_flow"},
		{name: "invalid flow identity", mutate: func(message *ClientMessage) { message.Flow = "../private" }, diagnostic: "invalid_flow"},
		{name: "missing cleanup", mutate: func(message *ClientMessage) { message.CleanupDisposition = "" }, diagnostic: "invalid_cleanup"},
		{name: "unsupported cleanup", mutate: func(message *ClientMessage) { message.CleanupDisposition = "automatic" }, diagnostic: "invalid_cleanup"},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{observations: []RawObservation{observation}}
			browser := &fakeBrowser{session: session}
			review := reviewMessage(profile, []string{buttonID})
			test.mutate(&review)
			input := ndjson(t, startMessage("https://app.example.test/register"), ClientMessage{Protocol: Protocol, Type: "observe"}, review)
			_, output, err := runSession(context.Background(), input, browser, fixedNow)
			assertFailure(t, err, output, test.diagnostic)
			if session.closeCount != 1 {
				t.Fatalf("close count = %d", session.closeCount)
			}
		})
	}
}

func TestReviewBindsAccessibilityProfileSubmitBeforeCompletion(t *testing.T) {
	profile := validProfileJSON(t)
	for _, test := range []struct {
		name        string
		profile     json.RawMessage
		candidate   RawCandidate
		candidateID string
		diagnostic  string
	}{
		{
			name: "unrelated candidate", profile: profile,
			candidate:   RawCandidate{Role: "button", Label: "Continue", Matches: 1},
			candidateID: candidateID(1, "button", "Continue", 0), diagnostic: "invalid_submit",
		},
		{
			name: "unsupported observation kind", profile: profileWithObservationKind(t, "dom_text"),
			candidate:   RawCandidate{Role: "button", Label: "Register", Matches: 1},
			candidateID: candidateID(1, "button", "Register", 0), diagnostic: "invalid_profile",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &fakeSession{observations: []RawObservation{{
				Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{test.candidate},
			}}}
			browser := &fakeBrowser{session: session}
			input := ndjson(t,
				startMessage("https://app.example.test/register"),
				ClientMessage{Protocol: Protocol, Type: "observe"},
				reviewMessage(test.profile, []string{test.candidateID}),
			)
			completion, output, err := runSession(context.Background(), input, browser, fixedNow)
			assertFailure(t, err, output, test.diagnostic)
			if completion != nil || session.closeCount != 1 {
				t.Fatalf("completion=%#v close=%d", completion, session.closeCount)
			}
		})
	}
}

func TestNavigationExpiresCandidateGeneration(t *testing.T) {
	profile := validProfileJSON(t)
	oldID := candidateID(1, "button", "Register", 0)
	session := &fakeSession{observations: []RawObservation{{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
	}}}
	browser := &fakeBrowser{session: session}
	input := ndjson(t,
		startMessage("https://app.example.test/register"),
		ClientMessage{Protocol: Protocol, Type: "observe"},
		ClientMessage{Protocol: Protocol, Type: "navigate", Method: "GET", URL: "https://app.example.test/help"},
		reviewMessage(profile, []string{oldID}),
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
				observations: []RawObservation{{Origin: "https://app.example.test", Path: "/register", Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}}}},
				summary:      test.summary, closeErr: test.closeErr,
			}
			browser := &fakeBrowser{session: session}
			input := ndjson(t,
				startMessage("https://app.example.test/register"),
				ClientMessage{Protocol: Protocol, Type: "observe"},
				reviewMessage(profile, []string{buttonID}),
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

func TestFinishBoundsBlockingTeardownWithFreshCleanupContext(t *testing.T) {
	profile := validProfileJSON(t)
	buttonID := candidateID(1, "button", "Register", 0)
	session := &fakeSession{
		observations: []RawObservation{{
			Origin: "https://app.example.test", Path: "/register",
			Candidates: []RawCandidate{{Role: "button", Label: "Register", Matches: 1}},
		}},
		closeBlocked: true,
	}
	start := startMessage("https://app.example.test/register")
	start.Bounds = &Bounds{
		NavigationTimeoutMS: 20, TotalTimeoutMS: 1000, MaxRequests: 8,
		MaxResponseBytes: 1 << 20, MaxObservations: 8, MaxCandidates: 8,
	}
	input := ndjson(t,
		start,
		ClientMessage{Protocol: Protocol, Type: "observe"},
		reviewMessage(profile, []string{buttonID}),
		ClientMessage{Protocol: Protocol, Type: "finish"},
	)
	started := time.Now()
	completion, output, err := runSession(context.Background(), input, &fakeBrowser{session: session}, fixedNow)
	assertFailure(t, err, output, "teardown_failure")
	if completion != nil || session.closeCount != 1 || time.Since(started) > time.Second {
		t.Fatalf("completion=%#v close=%d elapsed=%s", completion, session.closeCount, time.Since(started))
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
			io.NopCloser(strings.NewReader(ndjson(t, startMessage("https://app.example.test/register")))),
			writer,
			browser,
			ServeOptions{Clock: func() time.Time { return fixedNow }},
		)
		if err == nil || session.closeCount != 1 {
			t.Fatalf("close=%d error=%v", session.closeCount, err)
		}
		if strings.Contains(err.Error(), "synthetic output failure") {
			t.Fatalf("output writer detail leaked: %v", err)
		}
	})
}

func TestCancellationInterruptsBlockedOwnedInputAndClosesBrowser(t *testing.T) {
	input := &stagedBlockingInput{
		data:      []byte(ndjson(t, startMessage("https://app.example.test/register"))),
		readAgain: make(chan struct{}), closed: make(chan struct{}),
	}
	session := &fakeSession{}
	browser := &fakeBrowser{session: session}
	ctx, cancel := context.WithCancel(context.Background())
	var output bytes.Buffer
	result := make(chan error, 1)
	go func() {
		_, err := Serve(ctx, input, &output, browser, ServeOptions{Clock: func() time.Time { return fixedNow }})
		result <- err
	}()
	<-input.readAgain
	cancel()
	select {
	case err := <-result:
		assertFailure(t, err, output.String(), "canceled")
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt blocked protocol input")
	}
	if browser.openCount != 1 || session.closeCount != 1 {
		t.Fatalf("opens=%d close=%d", browser.openCount, session.closeCount)
	}
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

func TestAccessibilityLabelDisclosureMakesHeuristicBoundaryExplicit(t *testing.T) {
	if AccessibilityLabelDisclosure != authorsession.AccessibilityLabelDisclosure {
		t.Fatal("registration label disclosure drifted from the canonical reducer")
	}
	for _, phrase := range []string{"heuristic", "not data loss prevention", "Ordinary names", "identifiers", "reviewed traces"} {
		if !strings.Contains(AccessibilityLabelDisclosure, phrase) {
			t.Fatalf("label disclosure is missing %q", phrase)
		}
	}
}

func startMessage(url string) ClientMessage {
	return ClientMessage{
		Protocol: Protocol, Type: "start", ProfileID: "synthetic_registration",
		URL: url, Origins: []string{"https://app.example.test"},
	}
}

func reviewMessage(profile json.RawMessage, candidateIDs []string) ClientMessage {
	return ClientMessage{
		Protocol: Protocol, Type: "review", Profile: profile, CandidateIDs: candidateIDs,
		Flow: "create_dedicated_test_user", CleanupDisposition: "delete_separately",
	}
}

func repeatDiagnostic(code string, count int) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = code
	}
	return result
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

func profileWithObservationKind(t *testing.T, kind string) json.RawMessage {
	t.Helper()
	data := validProfileJSON(t)
	profileValue, err := registrationprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	profileValue.ObservationKind = kind
	data, err = registrationprofile.MarshalJSON(profileValue)
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
	completion, err := Serve(ctx, io.NopCloser(strings.NewReader(input)), &output, browser, ServeOptions{Clock: func() time.Time { return now }})
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
