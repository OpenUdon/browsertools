package capture

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorsession"
)

// Each case owns a new browser and loopback fixture. A cacheable resource is
// loaded before login and reused after one approved POST, including new frame
// and popup targets. No size error is ignored or repaired by the fixture.
func TestPlaywrightAuthorCacheIsolationLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_AUTHOR_LIVE_TEST") != "1" {
		t.Skip("explicit isolated loopback browser test")
	}
	for _, mode := range []string{"stylesheet", "image", "iframe", "oopif", "popup", "gzip", "cumulative_bytes", "request_limit"} {
		t.Run(mode, func(t *testing.T) {
			var posts, assets, cookieSeen atomic.Int32
			var origin, assetOrigin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/asset":
					assets.Add(1)
					w.Header().Set("Cache-Control", "public, max-age=3600")
					if mode == "image" {
						w.Header().Set("Content-Type", "image/svg+xml")
						fmt.Fprint(w, `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`)
						return
					}
					w.Header().Set("Content-Type", "text/css")
					css := "body { color: black; }"
					if mode == "cumulative_bytes" {
						css += "/*" + strings.Repeat("x", 1000) + "*/"
					}
					if mode == "gzip" {
						w.Header().Set("Content-Encoding", "gzip")
						z := gzip.NewWriter(w)
						fmt.Fprint(z, css)
						_ = z.Close()
					} else {
						fmt.Fprint(w, css)
					}
				case "/submit":
					if r.Method != "POST" {
						t.Error("fixture method")
						return
					}
					posts.Add(1)
					http.SetCookie(w, &http.Cookie{Name: "fixture", Value: "synthetic", Path: "/", HttpOnly: true})
					http.Redirect(w, r, "/campaign", http.StatusSeeOther)
				default:
					w.Header().Set("Content-Type", "text/html")
					if r.URL.Path == "/campaign" {
						if c, err := r.Cookie("fixture"); err == nil && c.Value == "synthetic" {
							cookieSeen.Add(1)
						}
						fmt.Fprint(w, `<h1>Campaign Management</h1>`)
						if mode == "iframe" || mode == "oopif" {
							fmt.Fprintf(w, `<iframe name="child" src="%s/child"></iframe>`, assetOrigin)
							return
						}
					}
					if mode == "image" {
						fmt.Fprintf(w, `<img src="%s/asset" alt="fixture">`, assetOrigin)
					} else {
						fmt.Fprintf(w, `<link rel="stylesheet" href="%s/asset">`, assetOrigin)
					}
					if r.URL.Path == "/login" {
						target := ""
						if mode == "popup" {
							target = ` formtarget="_blank"`
						}
						fmt.Fprintf(w, `<form method="post" action="/submit"><input name="synthetic" value="canary"><button%s>Sign in</button></form>`, target)
					}
				}
			}))
			defer server.Close()
			origin, assetOrigin = server.URL, server.URL
			if mode == "oopif" {
				assetOrigin = strings.Replace(origin, "127.0.0.1", "localhost", 1)
			}
			maxBytes, maxRequests := int64(1<<20), 32
			if mode == "cumulative_bytes" {
				maxBytes = 1900
			}
			if mode == "request_limit" {
				maxRequests = 4
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			s, err := NewPlaywrightAuthorBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")).Open(ctx, authorsession.BrowserRequest{URL: origin + "/login", ApprovedOrigins: []string{origin, assetOrigin}, NavigationTimeout: 15 * time.Second, TotalTimeout: 40 * time.Second, MaxRequests: maxRequests, MaxResponseBytes: maxBytes, MaxCandidates: 16})
			if err != nil {
				t.Fatal("cache fixture startup", authordiagnostic.Classify(err))
			}
			live := s.(*playwrightAuthorSession)
			defer func() {
				prior := live.guard.result()
				closed := s.Close()
				if (prior == nil) != (closed == nil) || (prior != nil && closed != nil && prior.Error() != closed.Error()) {
					t.Error("close changed the existing policy result or failed cleanup")
				}
			}()
			o, err := s.Observe(ctx, "main")
			if err != nil {
				t.Fatal("login observation")
			}
			var button authorsession.RawCandidate
			for _, c := range o.Candidates {
				if c.Role == "button" && c.Label == "Sign in" {
					button = c
				}
			}
			if button.BackendID == "" || posts.Load() != 0 {
				t.Fatal("pre-approval login readiness")
			}
			result, err := s.Execute(ctx, authorsession.BrowserAction{Kind: "click", BackendID: button.BackendID, Context: "main", POSTBudget: 1, Role: button.Role, Label: button.Label, InputKind: button.InputKind, TargetOrigin: button.TargetOrigin, Matches: button.Matches})
			if posts.Load() != 1 {
				t.Fatal("fixture did not receive exactly one POST")
			}
			if mode == "cumulative_bytes" || mode == "request_limit" {
				want := "response_limit"
				if mode == "request_limit" {
					want = "request_limit"
				}
				deadline := time.Now().Add(time.Second)
				for live.guard.result() == nil && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if authordiagnostic.Classify(live.guard.result()).Reason != want {
					t.Fatal("cache policy weakened request/byte ceiling")
				}
				return
			}
			if err != nil || result.POSTObserved != 1 {
				t.Fatal("approved cached-resource login failed")
			}
			contextID := "main"
			if mode == "popup" {
				contextID = result.OpenedID
				if contextID == "" {
					t.Fatal("popup missing")
				}
			}
			o, err = s.Observe(ctx, contextID)
			if err != nil || o.Path != "/campaign" || assets.Load() != 2 || cookieSeen.Load() != 1 {
				t.Fatal("cache, redirect, cookie or presence invariant")
			}
			presence := 0
			for _, candidate := range o.Candidates {
				if candidate.Role == "heading" && candidate.Label == "Campaign Management" && candidate.Matches == 1 {
					presence++
				}
			}
			if presence != 1 {
				t.Fatal("unique campaign presence missing")
			}
			if mode == "oopif" {
				v, err := live.interception.root.Send("Target.getTargets", nil)
				if err != nil {
					t.Fatal("target inventory")
				}
				found := false
				for _, item := range v.(map[string]any)["targetInfos"].([]any) {
					if item.(map[string]any)["type"] == "iframe" {
						found = true
					}
				}
				if !found {
					t.Fatal("fixture did not reach out-of-process frame")
				}
			}
		})
	}
}
