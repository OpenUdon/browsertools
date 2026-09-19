package capture

import (
	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorpolicy"
	"testing"
)

func optionalGuard(t *testing.T) *authorNetworkGuard {
	t.Helper()
	a, _ := interceptionFixture()
	p, err := authorpolicy.New("https://analytics.example.test")
	if err != nil {
		t.Fatal(err)
	}
	a.guard.policy = p
	return a.guard
}

func TestAuthorNetworkGuardOptionalScriptDenialAndFatalOthers(t *testing.T) {
	for _, tc := range []struct {
		url, method, resource string
		nav, nonfatal         bool
	}{
		{"https://analytics.example.test/script.js", "GET", "script", false, true},
		{"https://analytics.example.test:443/script.js?token=secret-token-canary", "GET", "script", false, true},
		{"https://analytics.example.test/script.js", "HEAD", "script", false, false},
		{"https://analytics.example.test/script.js", "POST", "script", false, false},
		{"https://analytics.example.test/script.js", "GET", "fetch", false, false},
		{"https://analytics.example.test/script.js", "GET", "document", true, false},
		{"https://analytics.example.test/script.js", "GET", "script", true, false},
		{"https://analytics.example.test:444/script.js", "GET", "script", false, false},
		{"http://analytics.example.test/script.js", "GET", "script", false, false},
		{"https://other.example.test/script.js", "GET", "script", false, false},
		{"https://secret-token-canary@analytics.example.test/script.js", "GET", "script", false, false},
		{"https://analytics.example.test/script.js?bad=%zz", "GET", "script", false, false},
	} {
		g := optionalGuard(t)
		if g.allowRequest(tc.url, tc.method, tc.nav, tc.resource) {
			t.Fatal("excluded request admitted")
		}
		if (g.result() == nil) != tc.nonfatal {
			t.Fatal("incorrect fatal classification")
		}
		if g.allowRequest("https://example.test/next", "GET", false, "script") != tc.nonfatal {
			t.Fatal("incorrect continuation")
		}
	}
}

func TestAuthorNetworkGuardOptionalScriptBudgetsAndFirstFailure(t *testing.T) {
	for _, mode := range []string{"requests", "bytes", "first", "closing", "post", "origin"} {
		t.Run(mode, func(t *testing.T) {
			g := optionalGuard(t)
			switch mode {
			case "requests":
				g.core.maxRequests = 1
			case "bytes":
				g.observeBytes(2048)
			case "first":
				g.block("unexpected_popup")
			case "closing":
				g.beginClose()
			case "post":
				if g.allow("https://example.test/login", "POST") {
					t.Fatal("unapproved POST")
				}
			case "origin":
				if g.addOrigin("https://analytics.example.test") == nil {
					t.Fatal("denied origin admitted")
				}
				if g.allowedOrigin("https://analytics.example.test") {
					t.Fatal("denied origin allowed")
				}
				return
			}
			first := g.result()
			if g.allowRequest("https://analytics.example.test/a.js", "GET", false, "script") {
				t.Fatal("script admitted")
			}
			if mode == "requests" {
				if g.result() != nil {
					t.Fatal("first denial should fit budget")
				}
				g.allowRequest("https://analytics.example.test/a.js", "GET", false, "script")
				if authordiagnostic.Classify(g.result()).Reason != "request_limit" {
					t.Fatal("denials escaped accounting")
				}
			} else if g.result() != first {
				t.Fatal("first failure replaced")
			}
			if g.allow("https://example.test/", "GET") {
				t.Fatal("failed/closing guard resumed")
			}
		})
	}
}

func TestAuthorInterceptionOptionalScriptFailsRequestWithoutPoisoning(t *testing.T) {
	a, root := interceptionFixture()
	defer a.close()
	a.guard = optionalGuard(t)
	a.paused(pausedRequest("blocked", "https://analytics.example.test/a.js?token=secret-token-canary", "GET"))
	call := awaitAuthorCall(t, root)
	if call.method != "Fetch.failRequest" || len(call.params) != 2 || call.params["errorReason"] != "BlockedByClient" || a.guard.result() != nil {
		t.Fatal("optional denial escaped or poisoned guard")
	}
	a.paused(pausedRequest("allowed", "https://example.test/next.js", "GET"))
	if call := awaitAuthorCall(t, root); call.method != "Fetch.continueRequest" {
		t.Fatal("authoring failed to continue")
	}
}
