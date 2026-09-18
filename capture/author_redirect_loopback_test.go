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
	playwright "github.com/mxschmitt/playwright-go"
)

// Every subtest owns fresh loopback endpoints and a disposable browser. The
// excluded endpoint is an independent arrival counter, not an observation-time
// origin check. Nothing here navigates to a real service.
func TestAuthorRedirectContainmentLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_AUTHOR_LIVE_TEST") != "1" {
		t.Skip("explicit isolated loopback browser test")
	}
	for _, name := range []string{"main_escape", "second_hop_escape", "script_escape", "iframe_escape", "oopif_escape", "popup_escape", "worker_escape", "allowed_cookie_compression", "redirect_budget", "post_303", "post_307_block", "post_307_allowed", "post_307_escape", "automatic_post", "allowed_popup", "approved_popup_escape", "close_pending", "declared_byte_limit", "actual_byte_limit", "cancel"} {
		t.Run(name, func(t *testing.T) {
			var excluded, posts, final, cookieOK, worker, escapeStarts atomic.Int32
			slowStarted := make(chan struct{})
			slowStopped := make(chan struct{})
			excludedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { excluded.Add(1); w.WriteHeader(200) }))
			defer excludedServer.Close()
			var origin, other string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				switch r.URL.Path {
				case "/start":
					switch name {
					case "main_escape":
						http.Redirect(w, r, excludedServer.URL+"/excluded", 302)
					case "second_hop_escape":
						http.Redirect(w, r, "/escape", 302)
					case "iframe_escape":
						fmt.Fprint(w, `<iframe src="/escape"></iframe>`)
					case "script_escape":
						fmt.Fprint(w, `<script src="/escape"></script><h1>Local</h1>`)
					case "oopif_escape":
						fmt.Fprintf(w, `<iframe src="%s/child"></iframe>`, other)
					case "popup_escape":
						fmt.Fprintf(w, `<script>window.open('%s/escape')</script>`, origin)
					case "close_pending":
						fmt.Fprint(w, `<h1>Local</h1><script>fetch('/slow').catch(()=>{})</script>`)
					case "approved_popup_escape":
						fmt.Fprint(w, `<a href="/escape" target="_blank">Open dashboard</a>`)
					case "allowed_popup":
						fmt.Fprint(w, `<a href="/popup-start" target="_blank">Open dashboard</a>`)
					case "automatic_post":
						fmt.Fprint(w, `<form action="/submit" method="post"></form><script>document.forms[0].submit()</script>`)
					case "worker_escape":
						fmt.Fprint(w, `<script>new Worker('/worker.js')</script><h1>Local</h1>`)
					case "allowed_cookie_compression":
						http.SetCookie(w, &http.Cookie{Name: "synthetic_one", Value: "one", Path: "/", HttpOnly: true})
						http.SetCookie(w, &http.Cookie{Name: "synthetic_two", Value: "two", Path: "/", HttpOnly: true})
						http.Redirect(w, r, "/final", 302)
					case "redirect_budget":
						http.Redirect(w, r, "/loop", 302)
					case "declared_byte_limit":
						w.Header().Set("Content-Length", "2000000")
						fmt.Fprint(w, "short")
					case "actual_byte_limit":
						w.(http.Flusher).Flush()
						fmt.Fprint(w, strings.Repeat("x", 2048))
					default:
						fmt.Fprint(w, `<form action="/submit" method="post"><input name="synthetic" value="canary"><button>Sign in</button></form>`)
					}
				case "/slow":
					close(slowStarted)
					select {
					case <-r.Context().Done():
					case <-time.After(10 * time.Second):
					}
					close(slowStopped)
				case "/popup-start":
					http.Redirect(w, r, "/final", 302)
				case "/child":
					fmt.Fprint(w, `<h1>Child</h1>`)
				case "/worker.js":
					w.Header().Set("Content-Type", "text/javascript")
					fmt.Fprint(w, `fetch('/escape').catch(()=>{})`)
					worker.Add(1)
				case "/escape":
					escapeStarts.Add(1)
					http.Redirect(w, r, excludedServer.URL+"/excluded", 302)
				case "/loop":
					http.Redirect(w, r, "/loop", 302)
				case "/submit":
					if r.Method == "POST" {
						posts.Add(1)
					}
					http.SetCookie(w, &http.Cookie{Name: "synthetic_login", Value: "one", Path: "/", HttpOnly: true})
					if name == "post_307_escape" {
						http.Redirect(w, r, excludedServer.URL+"/excluded", 307)
						return
					}
					if name == "post_303" {
						http.Redirect(w, r, "/final", 303)
					} else {
						http.Redirect(w, r, "/final", 307)
					}
				case "/final":
					final.Add(1)
					if r.Method == "POST" {
						posts.Add(1)
						if r.ParseForm() != nil || r.Form.Get("synthetic") != "canary" {
							t.Error("redirect lost synthetic POST body")
						}
					}
					if name == "post_303" && r.Method != "GET" {
						t.Error("303 did not become GET")
					}
					if name == "allowed_cookie_compression" {
						one, e1 := r.Cookie("synthetic_one")
						two, e2 := r.Cookie("synthetic_two")
						if e1 == nil && e2 == nil && one.Value == "one" && two.Value == "two" {
							cookieOK.Add(1)
						}
						w.Header().Set("Content-Encoding", "gzip")
						w.Header().Set("Content-Security-Policy", "script-src 'none'")
						z := gzip.NewWriter(w)
						fmt.Fprintf(z, `<h1>Dashboard</h1><script>fetch('%s/excluded')</script>`, excludedServer.URL)
						_ = z.Close()
					} else {
						fmt.Fprint(w, `<h1>Dashboard</h1>`)
					}
				default:
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			origin = server.URL
			other = strings.Replace(origin, "127.0.0.1", "localhost", 1)
			origins := []string{origin}
			if name == "oopif_escape" {
				origins = append(origins, other)
			}
			requests := 64
			if name == "redirect_budget" {
				requests = 3
			}
			bytes := int64(1 << 20)
			if name == "actual_byte_limit" {
				bytes = 1024
			}
			s, err := NewPlaywrightAuthorBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")).Open(context.Background(), authorsession.BrowserRequest{URL: origin + "/start", ApprovedOrigins: origins, NavigationTimeout: 20 * time.Second, TotalTimeout: 30 * time.Second, MaxRequests: requests, MaxResponseBytes: bytes, MaxCandidates: 16})
			var live *playwrightAuthorSession
			if s != nil {
				live = s.(*playwrightAuthorSession)
				defer s.Close()
			}
			if name == "oopif_escape" {
				if err != nil || live == nil {
					t.Fatalf("approved OOPIF failed: %v", err)
				}
				value, e := live.interception.root.Send("Target.getTargets", map[string]any{})
				if e != nil {
					t.Fatal("cannot verify OOPIF target")
				}
				found := false
				for _, item := range value.(map[string]any)["targetInfos"].([]any) {
					if item.(map[string]any)["type"] == "iframe" {
						found = true
					}
				}

				if !found {
					t.Fatal("fixture did not exercise an out-of-process iframe target")
				}
				frames := live.pages["main"].Frames()
				if len(frames) != 2 {
					t.Fatal("missing child frame")
				}
				_, _ = frames[1].Evaluate(`fetch('/escape').catch(()=>{})`)
			}

			if name == "close_pending" {
				if err != nil || live == nil {
					t.Fatal("pending-close fixture unavailable")
				}
				select {
				case <-slowStarted:
				case <-time.After(3 * time.Second):
					t.Fatal("pending request missing")
				}
				if e := s.Close(); e != nil {
					t.Fatal(e)
				}
				select {
				case <-slowStopped:
				case <-time.After(3 * time.Second):
					t.Fatal("pending request survived close")
				}
				return
			}
			if name == "allowed_popup" || name == "approved_popup_escape" {
				if err != nil || live == nil {
					t.Fatalf("popup fixture unavailable: %v", err)
				}
				observation, e := s.Observe(context.Background(), "main")
				if e != nil {
					t.Fatal(e)
				}
				var candidate authorsession.RawCandidate
				for _, c := range observation.Candidates {
					if c.Role == "link" && c.Label == "Open dashboard" {
						candidate = c
					}
				}
				if candidate.BackendID == "" {
					t.Fatal("popup link absent")
				}
				result, e := s.Execute(context.Background(), authorsession.BrowserAction{Kind: "click", Context: "main", BackendID: candidate.BackendID, Role: candidate.Role, Label: candidate.Label, TargetOrigin: candidate.TargetOrigin, Matches: 1, POSTBudget: 0})
				if name == "approved_popup_escape" {
					if e == nil || authordiagnostic.Classify(live.guard.result()).Reason != "origin_escape" || excluded.Load() != 0 || final.Load() != 0 {
						t.Fatal("approved popup redirect escaped containment")
					}
					return
				}
				if e != nil || result.OpenedID == "" {
					t.Fatalf("approved popup failed: %v", e)
				}
				if final.Load() != 1 || posts.Load() != 0 || excluded.Load() != 0 {
					t.Fatal("popup arrival counts mismatch")
				}
				return
			}

			if strings.HasPrefix(name, "post_") {
				if err != nil {
					t.Fatal(err)
				}
				budget := 1
				if name == "post_307_allowed" || name == "post_307_escape" {
					budget = 2
				}
				if e := live.guard.beginPOST(budget); e != nil {
					t.Fatal(e)
				}
				_ = live.pages["main"].GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign in"}).Click()
				_, err = live.guard.endPOST()
			}
			if name == "cancel" {
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, e := s.Observe(ctx, "main"); e != context.Canceled {
					t.Fatalf("cancellation = %v", e)
				}
				if e := s.Close(); e != nil {
					t.Fatal(e)
				}
				return
			}
			// Workers and out-of-process subresources can fail after main load.
			if name == "worker_escape" || name == "oopif_escape" || name == "actual_byte_limit" || name == "popup_escape" || name == "automatic_post" {
				deadline := time.Now().Add(3 * time.Second)
				for live != nil && err == nil && time.Now().Before(deadline) {
					err = live.guard.result()
					if err == nil {
						time.Sleep(10 * time.Millisecond)
					}
				}
			}
			if excluded.Load() != 0 {
				t.Fatalf("excluded endpoint received %d requests", excluded.Load())
			}
			switch name {
			case "allowed_cookie_compression", "post_303", "post_307_allowed":
				if err != nil || live == nil || final.Load() != 1 {
					t.Fatalf("allowed redirect failed: %v final=%d", err, final.Load())
				}
				if live.pages["main"].URL() != origin+"/final" {
					t.Fatal("browser lost final redirect URL")
				}
				if n, e := live.pages["main"].GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Dashboard"}).Count(); e != nil || n != 1 {
					t.Fatal("response content was not preserved")
				}
				if name == "allowed_cookie_compression" && cookieOK.Load() != 1 {
					t.Fatal("redirect cookies not preserved")
				}
				want := int32(1)
				if name == "post_307_allowed" {
					want = 2
				}
				if strings.HasPrefix(name, "post_") && posts.Load() != want {
					t.Fatal("POST count mismatch")
				}
			default:
				if err == nil {
					t.Fatal("expected bounded policy failure")
				}
				class := authordiagnostic.Classify(err)
				want := "origin_escape"
				switch name {
				case "redirect_budget":
					want = "request_limit"
				case "automatic_post":
					want = "post_budget"
					if posts.Load() != 0 {
						t.Fatal("automatic POST reached fixture")
					}
				case "post_307_block":
					want = "post_budget"
				case "declared_byte_limit", "actual_byte_limit":
					want = "response_limit"
				case "popup_escape":
					want = "unexpected_popup"
					if class.Reason == "unexpected_navigation" || class.Reason == "origin_escape" {
						want = class.Reason
					}
				case "worker_escape":
					want = "origin_escape"
					if worker.Load() != 1 || escapeStarts.Load() != 1 {
						t.Fatal("worker redirect control was not exercised")
					}
				}
				if class.Stage != "policy" || class.Reason != want {
					t.Fatalf("classification=%+v, want policy/%s", class, want)
				}
				if want == "origin_escape" {
					rejection := authordiagnostic.RejectionOf(err)
					resource := "document"
					if name == "script_escape" {
						resource = "script"
					}
					if name == "worker_escape" || name == "oopif_escape" {
						// The pinned Chromium Fetch boundary reports these fetch()
						// calls as XHR. Describe CDP facts, not the JavaScript API.
						resource = "xhr"
					}
					if rejection != (authordiagnostic.Rejection{Boundary: "request", Resource: resource, OriginRelation: "port_mismatch"}) {
						t.Fatalf("rejection attribution = %+v", rejection)
					}
				}
				if (name == "post_307_block" || name == "post_307_escape") && (posts.Load() != 1 || final.Load() != 0) {
					t.Fatal("307 POST bypassed its budget")
				}
			}
		})
	}
}
