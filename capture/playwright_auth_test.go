package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/authassist"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/uws/browserauthentication"
)

func validAuthBrowserRequest() authassist.BrowserRequest {
	return authassist.BrowserRequest{
		ApprovedOrigins:   []string{"https://login.example.test", "https://members.example.test"},
		NavigationTimeout: authassist.DefaultNavigationTimeout,
		MaxRequests:       authassist.DefaultMaxRequests, MaxResponseBytes: authassist.DefaultMaxResponseBytes,
	}
}

func TestAuthNetworkGuardRequiresStepScopedPOSTAuthority(t *testing.T) {
	request := validAuthBrowserRequest()
	guard := newAuthNetworkGuard(request)
	if !guard.allowRequest(requestFacts{URL: "https://login.example.test/start?state=ephemeral", Method: "GET", ResourceType: "document"}) {
		t.Fatal("approved OAuth navigation was blocked")
	}
	if guard.allowRequest(requestFacts{URL: "https://login.example.test/session", Method: "POST", ResourceType: "fetch"}) {
		t.Fatal("unarmed POST was allowed")
	}
	if policyCode(guard.result()) != "post_budget" {
		t.Fatalf("guard error = %v", guard.result())
	}

	guard = newAuthNetworkGuard(request)
	if err := guard.beginInteraction(2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if !guard.allowRequest(requestFacts{URL: "https://login.example.test/session", Method: "POST", ResourceType: "fetch"}) {
			t.Fatalf("approved POST %d was blocked", i)
		}
	}
	if guard.allowRequest(requestFacts{URL: "https://login.example.test/session", Method: "POST", ResourceType: "fetch"}) {
		t.Fatal("POST above step ceiling was allowed")
	}
	observed, err := guard.endInteraction()
	if observed != 2 || policyCode(err) != "post_budget" {
		t.Fatalf("observed=%d err=%v", observed, err)
	}
}

func TestAuthNetworkGuardFailsClosedOnUnexpectedSurfaces(t *testing.T) {
	tests := []struct {
		name string
		call func(*authNetworkGuard) bool
		code string
	}{
		{name: "origin", call: func(g *authNetworkGuard) bool {
			return g.allowRequest(requestFacts{URL: "https://evil.test", Method: "GET", ResourceType: "document"})
		}, code: "origin"},
		{name: "userinfo", call: func(g *authNetworkGuard) bool {
			return g.allowRequest(requestFacts{URL: "https://user:pass@login.example.test", Method: "GET", ResourceType: "document"})
		}, code: "origin"},
		{name: "iframe", call: func(g *authNetworkGuard) bool {
			return g.allowRequest(requestFacts{URL: "https://login.example.test", Method: "GET", ResourceType: "document", ChildDocument: true})
		}, code: "iframe"},
		{name: "method", call: func(g *authNetworkGuard) bool {
			return g.allowRequest(requestFacts{URL: "https://login.example.test", Method: "PUT", ResourceType: "fetch"})
		}, code: "method"},
		{name: "websocket", call: func(g *authNetworkGuard) bool { g.block("websocket", "blocked"); return false }, code: "websocket"},
		{name: "popup", call: func(g *authNetworkGuard) bool { g.block("popup", "blocked"); return false }, code: "popup"},
		{name: "download", call: func(g *authNetworkGuard) bool { g.block("download", "blocked"); return false }, code: "download"},
		{name: "dialog", call: func(g *authNetworkGuard) bool { g.block("dialog", "blocked"); return false }, code: "dialog"},
		{name: "file chooser", call: func(g *authNetworkGuard) bool { g.block("file_chooser", "blocked"); return false }, code: "file_chooser"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			guard := newAuthNetworkGuard(validAuthBrowserRequest())
			if test.call(guard) {
				t.Fatal("unsafe surface was allowed")
			}
			if policyCode(guard.result()) != test.code {
				t.Fatalf("code=%q err=%v", policyCode(guard.result()), guard.result())
			}
		})
	}
}

func TestAuthNetworkGuardBoundsResponsesAndLifecycle(t *testing.T) {
	request := validAuthBrowserRequest()
	request.MaxRequests = 1
	request.MaxResponseBytes = 10
	guard := newAuthNetworkGuard(request)
	if !guard.allowRequest(requestFacts{URL: "https://login.example.test", Method: "GET", ResourceType: "document"}) ||
		guard.allowRequest(requestFacts{URL: "https://login.example.test/app.js", Method: "GET", ResourceType: "script"}) {
		t.Fatal("request bound was not enforced")
	}
	if policyCode(guard.result()) != "request_limit" {
		t.Fatalf("err=%v", guard.result())
	}
	guard = newAuthNetworkGuard(request)
	guard.observeFinishedResponse(6)
	guard.observeFinishedResponse(5)
	if policyCode(guard.result()) != "response_size" {
		t.Fatalf("err=%v", guard.result())
	}
	guard = newAuthNetworkGuard(request)
	guard.pageClosed()
	if policyCode(guard.result()) != "page_closed" {
		t.Fatalf("err=%v", guard.result())
	}
	guard = newAuthNetworkGuard(request)
	guard.beginClose()
	guard.pageClosed()
	if guard.result() != nil {
		t.Fatalf("intentional close became a violation: %v", guard.result())
	}
}

func TestAuthNetworkGuardDeclaredResponseLength(t *testing.T) {
	tests := []struct {
		name   string
		length int64
		code   string
	}{
		{name: "valid", length: 9},
		{name: "boundary", length: 10},
		{name: "negative", length: -1, code: "response_size"},
		{name: "over limit", length: 11, code: "response_size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validAuthBrowserRequest()
			request.MaxResponseBytes = 10
			guard := newAuthNetworkGuard(request)
			guard.observeResponseContentLength(test.length)
			if code := policyCode(guard.result()); code != test.code {
				t.Fatalf("code=%q, want %q", code, test.code)
			}
		})
	}

	guard := newAuthNetworkGuard(validAuthBrowserRequest())
	guard.block("origin", "first violation")
	guard.observeResponseContentLength(-1)
	if code := policyCode(guard.result()); code != "origin" {
		t.Fatalf("declared response check replaced the first violation with %q", code)
	}
}

func TestValidateAuthBrowserRequestIsDefensive(t *testing.T) {
	for name, mutate := range map[string]func(*authassist.BrowserRequest){
		"no origins": func(r *authassist.BrowserRequest) { r.ApprovedOrigins = nil },
		"unsorted": func(r *authassist.BrowserRequest) {
			r.ApprovedOrigins[0], r.ApprovedOrigins[1] = r.ApprovedOrigins[1], r.ApprovedOrigins[0]
		},
		"duplicate":   func(r *authassist.BrowserRequest) { r.ApprovedOrigins[1] = r.ApprovedOrigins[0] },
		"path origin": func(r *authassist.BrowserRequest) { r.ApprovedOrigins[0] += "/login" },
		"timeout":     func(r *authassist.BrowserRequest) { r.NavigationTimeout = 0 },
		"requests":    func(r *authassist.BrowserRequest) { r.MaxRequests = 0 },
		"bytes":       func(r *authassist.BrowserRequest) { r.MaxResponseBytes = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			request := validAuthBrowserRequest()
			mutate(&request)
			if err := validateAuthBrowserRequest(request); err == nil {
				t.Fatal("invalid browser request accepted")
			}
		})
	}
}

func TestPlaywrightAuthAdapterHasNoCredentialOrActionAPI(t *testing.T) {
	data, err := os.ReadFile("playwright_auth.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		".Fill(", ".Type(", ".Press(", ".Click(", ".InputValue(", ".TextContent(",
		".Evaluate(", ".Screenshot(", ".Cookies(", ".StorageState(", "StorageState:",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("headed authentication adapter contains forbidden API %q", forbidden)
		}
	}
	if !strings.Contains(source, "Headless: playwright.Bool(false)") || !strings.Contains(source, "ServiceWorkerPolicyBlock") {
		t.Fatal("headed/ephemeral launch policy is not explicit")
	}
}

func TestPlaywrightAuthHeadedLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_AUTH_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_AUTH_LIVE_TEST=1 from a desktop session with the pinned driver and Chromium installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><h1>Member sign in</h1></main>`))
	}))
	defer server.Close()
	origin, err := profile.OriginOfURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	browser := NewPlaywrightAuthBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH"))
	session, err := browser.Open(context.Background(), authassist.BrowserRequest{
		ApprovedOrigins: []string{origin}, NavigationTimeout: authassist.DefaultNavigationTimeout,
		MaxRequests: authassist.DefaultMaxRequests, MaxResponseBytes: authassist.DefaultMaxResponseBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := session.Navigate(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	observation, err := session.Observe(context.Background(), &browserauthentication.Locator{Role: "heading", Name: "Member sign in"})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Origin != origin || observation.Matches != 1 {
		t.Fatalf("observation = %#v", observation)
	}
	if err := session.BeginAuthenticationInteraction(0); err != nil {
		t.Fatal(err)
	}
	if posts, err := session.EndAuthenticationInteraction(); err != nil || posts != 0 {
		t.Fatalf("posts=%d err=%v", posts, err)
	}
}
