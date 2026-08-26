package capture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
)

func validRegistrationBrowserRequest() registrationauthorsession.BrowserRequest {
	return registrationauthorsession.BrowserRequest{
		URL:               "https://app.example.test/register",
		ApprovedOrigins:   []string{"https://app.example.test"},
		NavigationTimeout: time.Second, TotalTimeout: time.Minute,
		MaxRequests: 16, MaxResponseBytes: 1 << 20, MaxCandidates: 16,
	}
}

func TestPlaywrightRegistrationHasNoInputSubmitOrStateAPI(t *testing.T) {
	data, err := os.ReadFile("playwright_registration.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		".Fill(", ".Type(", ".InputValue(", ".TextContent(", ".Click(", ".Focus(",
		".Press(", ".Check(", ".SelectOption(", ".Evaluate(", ".Screenshot(",
		".Cookies(", ".StorageState(", "StorageState:", "os.Getenv(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("registration backend contains forbidden API %q", forbidden)
		}
	}
	for _, required := range []string{
		"Headless: playwright.Bool(false)", "ChromiumSandbox: playwright.Bool(true)",
		"ServiceWorkerPolicyBlock", "captureBrowserEnvironment()", "RouteWebSocket",
		"request.Sizes()", `HeaderValue("content-length")`, ".Request().Head(",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("registration containment is missing %q", required)
		}
	}
}

func TestRegistrationNetworkGuardAllowsOnlyApprovedGETAndHEAD(t *testing.T) {
	request := validRegistrationBrowserRequest()
	guard := newRegistrationNetworkGuard(request)
	if err := guard.beginNavigation(request.URL); err != nil {
		t.Fatal(err)
	}
	if !guard.allowBrowser(request.URL, "GET", "document", true) ||
		!guard.allowBrowser("https://app.example.test/app.css?cache=1", "GET", "stylesheet", false) {
		t.Fatalf("approved read-only navigation failed: %v", guard.err())
	}
	guard.endNavigation()
	if err := guard.beginHEAD("https://app.example.test/register"); err != nil {
		t.Fatal(err)
	}
	summary, err := guard.result(nil)
	if err != nil || summary != (registrationauthorsession.NetworkSummary{Requests: 3, GETRequests: 2, HEADRequests: 1}) {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}

	for _, test := range []struct {
		name, rawURL, method, resource string
		navigation                     bool
		code                           string
	}{
		{name: "POST", rawURL: request.URL, method: "POST", resource: "fetch", code: "mutation_method"},
		{name: "PUT", rawURL: request.URL, method: "PUT", resource: "xhr", code: "mutation_method"},
		{name: "origin", rawURL: "https://evil.example.test/register", method: "GET", resource: "document", code: "origin_escape"},
		{name: "event stream", rawURL: request.URL, method: "GET", resource: "eventsource", code: "persistent_resource"},
		{name: "unexpected navigation", rawURL: request.URL, method: "GET", resource: "document", navigation: true, code: "unexpected_navigation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := newRegistrationNetworkGuard(request)
			if guard.allowBrowser(test.rawURL, test.method, test.resource, test.navigation) {
				t.Fatal("unsafe request was allowed")
			}
			if code := policyCode(guard.err()); code != test.code {
				t.Fatalf("policy code=%q err=%v", code, guard.err())
			}
		})
	}
}

func TestRegistrationNetworkGuardBoundsAndCloseAccounting(t *testing.T) {
	request := validRegistrationBrowserRequest()
	request.MaxRequests, request.MaxResponseBytes = 1, 10
	guard := newRegistrationNetworkGuard(request)
	if !guard.allowBrowser(request.URL, "GET", "script", false) || guard.allowBrowser(request.URL, "HEAD", "document", false) {
		t.Fatalf("request bound mismatch: %v", guard.err())
	}
	if code := policyCode(guard.err()); code != "request_limit" {
		t.Fatalf("request-limit code=%q", code)
	}

	guard = newRegistrationNetworkGuard(request)
	guard.observeResponseContentLength(11)
	if code := policyCode(guard.err()); code != "response_limit" {
		t.Fatalf("declared response-limit code=%q", code)
	}
	guard = newRegistrationNetworkGuard(request)
	guard.observeBytes(4)
	guard.observeBytes(7)
	if code := policyCode(guard.err()); code != "response_limit" {
		t.Fatalf("actual response-limit code=%q", code)
	}

	guard = newRegistrationNetworkGuard(request)
	if !guard.allowBrowser(request.URL, "GET", "document", false) {
		t.Fatal(guard.err())
	}
	guard.beginClose()
	if guard.allowBrowser(request.URL, "POST", "fetch", false) {
		t.Fatal("closing guard allowed a request")
	}
	summary, err := guard.result(nil)
	if err != nil || summary.Requests != 1 || summary.GETRequests != 1 || summary.HEADRequests != 0 {
		t.Fatalf("closing summary=%#v err=%v", summary, err)
	}
}

func TestValidateRegistrationBrowserRequestIsExactAndFinite(t *testing.T) {
	valid := validRegistrationBrowserRequest()
	if err := validateRegistrationBrowserRequest(valid); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*registrationauthorsession.BrowserRequest){
		"query": func(value *registrationauthorsession.BrowserRequest) { value.URL += "?identifier=private" },
		"unsafe path": func(value *registrationauthorsession.BrowserRequest) {
			value.URL = "https://app.example.test/password=private"
		},
		"remote HTTP": func(value *registrationauthorsession.BrowserRequest) {
			value.URL = "http://app.example.test/register"
			value.ApprovedOrigins = []string{"http://app.example.test"}
		},
		"noncanonical origin": func(value *registrationauthorsession.BrowserRequest) {
			value.ApprovedOrigins = []string{"https://APP.example.test:443"}
		},
		"unapproved URL": func(value *registrationauthorsession.BrowserRequest) {
			value.URL = "https://other.example.test/register"
		},
		"unsorted origins": func(value *registrationauthorsession.BrowserRequest) {
			value.ApprovedOrigins = []string{"https://z.example.test", "https://app.example.test"}
		},
		"duplicate origins": func(value *registrationauthorsession.BrowserRequest) {
			value.ApprovedOrigins = []string{"https://app.example.test", "https://app.example.test"}
		},
		"navigation bound": func(value *registrationauthorsession.BrowserRequest) {
			value.NavigationTimeout = time.Minute + time.Millisecond
		},
		"total bound": func(value *registrationauthorsession.BrowserRequest) {
			value.TotalTimeout = 30*time.Minute + time.Millisecond
		},
		"request bound":   func(value *registrationauthorsession.BrowserRequest) { value.MaxRequests = 4097 },
		"response bound":  func(value *registrationauthorsession.BrowserRequest) { value.MaxResponseBytes = 128<<20 + 1 },
		"candidate bound": func(value *registrationauthorsession.BrowserRequest) { value.MaxCandidates = 513 },
	}
	keys := make([]string, 0, len(tests))
	for name := range tests {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		t.Run(name, func(t *testing.T) {
			request := valid
			request.ApprovedOrigins = append([]string(nil), valid.ApprovedOrigins...)
			tests[name](&request)
			if err := validateRegistrationBrowserRequest(request); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
}

func TestRegistrationURLFactsRejectsDisclosureAndQuery(t *testing.T) {
	allowed := func(origin string) bool { return origin == "https://app.example.test" }
	if origin, path, err := registrationURLFacts("https://app.example.test/register", registrationauthorsession.ProtocolV1, allowed); err != nil || origin != "https://app.example.test" || path != "/register" {
		t.Fatalf("facts=%q %q err=%v", origin, path, err)
	}
	for _, rawURL := range []string{
		"https://app.example.test/register?token=private",
		"https://app.example.test/identifier=operator@example.test",
		"https://other.example.test/register",
		"https://user:pass@app.example.test/register",
	} {
		if _, _, err := registrationURLFacts(rawURL, registrationauthorsession.ProtocolV1, allowed); err == nil {
			t.Fatalf("unsafe URL %q was accepted", rawURL)
		}
	}
}

func TestRegistrationV2URLFactsAndGuardRetainOnlySafeNavigationQuery(t *testing.T) {
	request := validRegistrationBrowserRequest()
	request.Protocol = registrationauthorsession.ProtocolV2
	request.URL += "?action=startnew"
	if err := validateRegistrationBrowserRequest(request); err != nil {
		t.Fatal(err)
	}
	allowed := func(origin string) bool { return origin == "https://app.example.test" }
	origin, path, err := registrationURLFacts(request.URL, request.Protocol, allowed)
	if err != nil || origin != "https://app.example.test" || path != "/register" || strings.Contains(origin+path, "action") {
		t.Fatalf("facts=%q %q error=%v", origin, path, err)
	}
	guard := newRegistrationNetworkGuard(request)
	if err := guard.beginNavigation(request.URL); err != nil || !guard.allowBrowser(request.URL, "GET", "document", true) {
		t.Fatalf("safe v2 navigation failed: %v", errors.Join(err, guard.err()))
	}
	guard.endNavigation()
	if !guard.allowBrowser("https://app.example.test/app.css?cache=dynamic", "GET", "stylesheet", false) {
		t.Fatalf("same-origin resource query failed: %v", guard.err())
	}
	for _, rawURL := range []string{
		"https://app.example.test/register?token=private",
		"https://app.example.test/register?action=startnew#fragment",
		"https://other.example.test/register?action=startnew",
	} {
		guard := newRegistrationNetworkGuard(request)
		if err := guard.beginNavigation(rawURL); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("unsafe navigation error=%v", err)
		}
	}
}

func TestPlaywrightRegistrationLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_REGISTRATION_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_REGISTRATION_LIVE_TEST=1 from a desktop session with the pinned driver and Chromium installed")
	}
	var mu sync.Mutex
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		methods = append(methods, request.Method)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><form method="post" action="/created"><label>Email<input autocomplete="email"></label><label>Password<input type="password"></label><button>Register</button></form></main>`))
	}))
	defer server.Close()
	origin, err := canonicalAuthorOrigin(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	browser := NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH"))
	session, err := browser.Open(context.Background(), registrationauthorsession.BrowserRequest{
		URL: server.URL + "/register", ApprovedOrigins: []string{origin},
		NavigationTimeout: 20 * time.Second, TotalTimeout: time.Minute,
		MaxRequests: 32, MaxResponseBytes: 1 << 20, MaxCandidates: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := session.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundSubmit := false
	for _, candidate := range observation.Candidates {
		if candidate.Role == "button" && candidate.Label == "Register" && candidate.Matches == 1 {
			foundSubmit = true
		}
	}
	if !foundSubmit {
		t.Fatalf("registration candidates=%#v", observation.Candidates)
	}
	if err := session.Navigate(context.Background(), registrationauthorsession.Navigation{Method: "HEAD", URL: server.URL + "/register"}); err != nil {
		t.Fatal(err)
	}
	summary, err := session.Close(context.Background())
	if err != nil || summary.Requests != summary.GETRequests+summary.HEADRequests || summary.HEADRequests != 1 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, method := range methods {
		if method != http.MethodGet && method != http.MethodHead {
			t.Fatalf("loopback observed mutation method %q in %#v", method, methods)
		}
	}
}

func TestPlaywrightRegistrationV2QueryLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_REGISTRATION_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_REGISTRATION_LIVE_TEST=1 from a desktop session with the pinned driver and Chromium installed")
	}
	var mu sync.Mutex
	methods := []string{}
	queries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		methods = append(methods, request.Method)
		queries = append(queries, request.URL.RawQuery)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><form method="post" action="/created"><label>Email<input autocomplete="email"></label><label>Password<input type="password"></label><button>Register</button></form></main>`))
	}))
	defer server.Close()
	origin, err := canonicalAuthorOrigin(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	queryURL := server.URL + "/register?action=startnew"
	browser := NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH"))
	session, err := browser.Open(context.Background(), registrationauthorsession.BrowserRequest{
		Protocol: registrationauthorsession.ProtocolV2,
		URL:      queryURL, ApprovedOrigins: []string{origin},
		NavigationTimeout: 20 * time.Second, TotalTimeout: time.Minute,
		MaxRequests: 32, MaxResponseBytes: 1 << 20, MaxCandidates: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := session.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Origin != origin || observation.Path != "/register" || strings.Contains(observation.Path, "action") {
		t.Fatalf("observation disclosed or lost safe facts: %#v", observation)
	}
	if err := session.Navigate(context.Background(), registrationauthorsession.Navigation{Method: "HEAD", URL: queryURL}); err != nil {
		t.Fatal(err)
	}
	summary, err := session.Close(context.Background())
	if err != nil || summary.Requests != summary.GETRequests+summary.HEADRequests || summary.HEADRequests != 1 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(methods) < 2 || len(queries) < 2 {
		t.Fatalf("loopback requests=%#v queries=%#v", methods, queries)
	}
	for index, method := range methods {
		if method != http.MethodGet && method != http.MethodHead {
			t.Fatalf("loopback observed mutation method %q in %#v", method, methods)
		}
		if queries[index] != "action=startnew" {
			t.Fatalf("loopback query[%d]=%q", index, queries[index])
		}
	}
}
