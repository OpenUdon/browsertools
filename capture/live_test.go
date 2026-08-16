package capture

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/profile"
)

type fakeAcquirer struct {
	request     LiveRequest
	observation Observation
	err         error
	calls       int
}

func (f *fakeAcquirer) Acquire(_ context.Context, request LiveRequest) (Observation, error) {
	f.calls++
	f.request = request
	return f.observation, f.err
}

func validLiveRequest() LiveRequest {
	return LiveRequest{
		URL: "https://example.test/member", AllowedOrigins: []string{"https://assets.example.test", "https://example.test"},
		ActionHint: "read_dashboard", ObservedAt: time.Date(2026, 8, 15, 12, 0, 0, 123, time.FixedZone("test", 3600)),
	}
}

func validObservation() Observation {
	return Observation{
		FinalURL: "https://example.test/member#dashboard", ARIASnapshot: "- button \"Refresh\"\n",
		StructuredData: []json.RawMessage{json.RawMessage(`{"status":"active"}`)},
		Network:        playwrightadapter.NetworkSummary{Requests: 2, Responses: 2, ResponseBytes: 1024},
	}
}

func TestAcquireValidatesDefaultsAndSerializesPrivateFixture(t *testing.T) {
	fake := &fakeAcquirer{observation: validObservation()}
	result, err := Acquire(context.Background(), fake, validLiveRequest())
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || fake.request.NavigationTimeout != DefaultNavigationTimeout ||
		fake.request.TotalTimeout != DefaultTotalTimeout || fake.request.MaxRequests != DefaultMaxRequests ||
		fake.request.MaxResponseBytes != DefaultMaxResponseBytes || fake.request.MaxEvidenceBytes != DefaultMaxEvidenceBytes ||
		fake.request.ARIADepth != DefaultARIADepth {
		t.Fatalf("normalized request = %#v", fake.request)
	}
	if strings.Join(fake.request.AllowedOrigins, ",") != "https://assets.example.test,https://example.test" {
		t.Fatalf("allowed origins = %v", fake.request.AllowedOrigins)
	}
	if result.Origin != "https://example.test" || result.Fixture.Version != playwrightadapter.FixtureVersion ||
		result.Fixture.ObservedAt != "2026-08-15T11:00:00.000000123Z" || result.Fixture.Snapshot != nil ||
		!strings.HasSuffix(string(result.JSON), "\n") {
		t.Fatalf("result = %#v JSON=%s", result, result.JSON)
	}
	if strings.Contains(string(result.JSON), "cookie") || strings.Contains(string(result.JSON), "storage") {
		t.Fatalf("session material in result: %s", result.JSON)
	}
}

func TestAcquireUsesSyntheticLoopbackWithFakeBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("fake acquirer must not contact the loopback server")
	}))
	defer server.Close()
	origin, err := profile.OriginOfURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	request := validLiveRequest()
	request.URL = server.URL + "/member"
	request.AllowedOrigins = []string{origin}
	observation := validObservation()
	observation.FinalURL = server.URL + "/member"
	fake := &fakeAcquirer{observation: observation}
	if _, err := Acquire(context.Background(), fake, request); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireRejectsInvalidRequestBeforeBrowser(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*LiveRequest)
	}{
		{name: "missing URL", mutate: func(r *LiveRequest) { r.URL = "" }},
		{name: "insecure remote HTTP", mutate: func(r *LiveRequest) {
			r.URL = "http://example.test"
			r.AllowedOrigins = []string{"http://example.test"}
		}},
		{name: "userinfo", mutate: func(r *LiveRequest) { r.URL = "https://user:pass@example.test" }},
		{name: "credential query", mutate: func(r *LiveRequest) { r.URL = "https://example.test?access_token=value" }},
		{name: "credential fragment", mutate: func(r *LiveRequest) { r.URL = "https://example.test/#state=value" }},
		{name: "missing origins", mutate: func(r *LiveRequest) { r.AllowedOrigins = nil }},
		{name: "origin not allowed", mutate: func(r *LiveRequest) { r.AllowedOrigins = []string{"https://other.test"} }},
		{name: "duplicate origin", mutate: func(r *LiveRequest) { r.AllowedOrigins = []string{"https://example.test", "https://example.test:443"} }},
		{name: "bad action", mutate: func(r *LiveRequest) { r.ActionHint = "read dashboard" }},
		{name: "missing observed time", mutate: func(r *LiveRequest) { r.ObservedAt = time.Time{} }},
		{name: "navigation exceeds total", mutate: func(r *LiveRequest) { r.NavigationTimeout = time.Minute; r.TotalTimeout = time.Second }},
		{name: "too many requests", mutate: func(r *LiveRequest) { r.MaxRequests = MaxRequests + 1 }},
		{name: "too many bytes", mutate: func(r *LiveRequest) { r.MaxResponseBytes = MaxResponseBytes + 1 }},
		{name: "too much evidence", mutate: func(r *LiveRequest) { r.MaxEvidenceBytes = MaxEvidenceBytes + 1 }},
		{name: "too deep", mutate: func(r *LiveRequest) { r.ARIADepth = 33 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validLiveRequest()
			test.mutate(&request)
			fake := &fakeAcquirer{observation: validObservation()}
			if _, err := Acquire(context.Background(), fake, request); err == nil || fake.calls != 0 {
				t.Fatalf("err=%v calls=%d", err, fake.calls)
			}
		})
	}
}

func TestAcquireRejectsInvalidObservation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Observation)
	}{
		{name: "missing final URL", mutate: func(o *Observation) { o.FinalURL = "" }},
		{name: "outside origin", mutate: func(o *Observation) { o.FinalURL = "https://other.test" }},
		{name: "credential redirect", mutate: func(o *Observation) { o.FinalURL = "https://example.test?token=value" }},
		{name: "empty aria", mutate: func(o *Observation) { o.ARIASnapshot = "" }},
		{name: "invalid JSON-LD", mutate: func(o *Observation) { o.StructuredData = []json.RawMessage{json.RawMessage(`{`)} }},
		{name: "request overflow", mutate: func(o *Observation) { o.Network.Requests = DefaultMaxRequests + 1 }},
		{name: "response overflow", mutate: func(o *Observation) { o.Network.ResponseBytes = DefaultMaxResponseBytes + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := validObservation()
			test.mutate(&observation)
			if _, err := Acquire(context.Background(), &fakeAcquirer{observation: observation}, validLiveRequest()); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestAcquirePropagatesCancellationAndBackendFailure(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(cancelled, &fakeAcquirer{observation: validObservation()}, validLiveRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	fake := &fakeAcquirer{err: errors.New("browser failed")}
	if _, err := Acquire(context.Background(), fake, validLiveRequest()); err == nil || !strings.Contains(err.Error(), "browser failed") {
		t.Fatalf("backend error = %v", err)
	}
}

func TestNetworkGuardEnforcesReadOnlyExactOriginPolicy(t *testing.T) {
	request, _, err := normalizeLiveRequest(validLiveRequest())
	if err != nil {
		t.Fatal(err)
	}
	guard := newNetworkGuard(request)
	if !guard.allowRequest(requestFacts{URL: request.URL, Method: "GET", ResourceType: "document"}) {
		t.Fatal("expected main document to be allowed")
	}
	if guard.allowRequest(requestFacts{URL: "https://example.test/logo.png", Method: "GET", ResourceType: "image"}) {
		t.Fatal("expected non-essential image to be blocked")
	}
	summary, err := guard.result()
	if err != nil || summary.BlockedRequests != 1 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}

	tests := []struct {
		name string
		fact requestFacts
		code string
	}{
		{name: "origin", fact: requestFacts{URL: "https://evil.test", Method: "GET", ResourceType: "document"}, code: "origin"},
		{name: "method", fact: requestFacts{URL: request.URL, Method: "POST", ResourceType: "fetch"}, code: "method"},
		{name: "iframe", fact: requestFacts{URL: request.URL, Method: "GET", ResourceType: "document", ChildDocument: true}, code: "iframe"},
		{name: "event stream", fact: requestFacts{URL: request.URL, Method: "GET", ResourceType: "eventsource"}, code: "resource_type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guard := newNetworkGuard(request)
			if guard.allowRequest(test.fact) {
				t.Fatal("expected block")
			}
			_, err := guard.result()
			if policyCode(err) != test.code {
				t.Fatalf("code=%q err=%v", policyCode(err), err)
			}
		})
	}
}

func TestNetworkGuardBoundsAndClosedEvents(t *testing.T) {
	request, _, err := normalizeLiveRequest(validLiveRequest())
	if err != nil {
		t.Fatal(err)
	}
	request.MaxRequests = 1
	request.MaxResponseBytes = 10
	guard := newNetworkGuard(request)
	if !guard.allowRequest(requestFacts{URL: request.URL, Method: "GET", ResourceType: "document"}) ||
		guard.allowRequest(requestFacts{URL: request.URL, Method: "GET", ResourceType: "script"}) {
		t.Fatal("unexpected request-limit behavior")
	}
	if _, err := guard.result(); policyCode(err) != "request_limit" {
		t.Fatalf("err=%v", err)
	}

	guard = newNetworkGuard(request)
	guard.observeResponseContentLength(11)
	if _, err := guard.result(); policyCode(err) != "response_size" {
		t.Fatalf("err=%v", err)
	}
	guard = newNetworkGuard(request)
	guard.observeFinishedResponse(6)
	guard.observeFinishedResponse(5)
	if _, err := guard.result(); policyCode(err) != "response_size" {
		t.Fatalf("err=%v", err)
	}

	for _, event := range []struct {
		code string
		call func(*networkGuard)
	}{
		{code: "websocket", call: (*networkGuard).blockWebSocket},
		{code: "popup", call: (*networkGuard).blockPopup},
		{code: "download", call: (*networkGuard).blockDownload},
		{code: "dialog", call: (*networkGuard).blockDialog},
		{code: "file_chooser", call: (*networkGuard).blockFileChooser},
	} {
		guard := newNetworkGuard(request)
		event.call(guard)
		if _, err := guard.result(); policyCode(err) != event.code {
			t.Fatalf("event=%s err=%v", event.code, err)
		}
	}
}

func TestCaptureBrowserEnvironmentExcludesCredentialVariables(t *testing.T) {
	t.Setenv("BROWSERTOOLS_TEST_SECRET", "must-not-pass")
	t.Setenv("OPENAI_API_KEY", "must-not-pass")
	t.Setenv("LANG", "en_US.UTF-8")
	environment := captureBrowserEnvironment()
	if environment["LANG"] != "en_US.UTF-8" {
		t.Fatalf("LANG = %q", environment["LANG"])
	}
	if _, ok := environment["BROWSERTOOLS_TEST_SECRET"]; ok {
		t.Fatal("secret-shaped environment variable was inherited")
	}
	if _, ok := environment["OPENAI_API_KEY"]; ok {
		t.Fatal("API key environment variable was inherited")
	}
}

func TestPlaywrightLiveCaptureLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_LIVE_TEST=1 with the pinned driver and Chromium installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><h1>Member dashboard</h1><button>Refresh</button><script type="application/ld+json">{"status":"active"}</script></main>`))
	}))
	defer server.Close()
	origin, err := profile.OriginOfURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	request := validLiveRequest()
	request.URL = server.URL
	request.AllowedOrigins = []string{origin}
	result, err := Acquire(context.Background(), NewPlaywrightAcquirer(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Fixture.ARIASnapshot, "Member dashboard") || len(result.Fixture.StructuredData) != 1 {
		t.Fatalf("fixture = %#v", result.Fixture)
	}
	prof := validCheckProfile()
	prof.Info.Origin = profile.Origins{origin}
	liveCheck, err := Check(context.Background(), NewPlaywrightAcquirer(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), LiveCheckRequest{
		Profile: prof, Actions: []string{"read_status"},
		Capture: LiveRequest{URL: server.URL, AllowedOrigins: []string{origin}, ObservedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !liveCheck.OK {
		t.Fatalf("live check = %#v", liveCheck)
	}
}
