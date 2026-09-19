package capture

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

type optionalCountingListener struct {
	net.Listener
	arrivals atomic.Int64
}

func (l *optionalCountingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.arrivals.Add(1)
	}
	return c, err
}

// Every endpoint is owned loopback. The HTTPS endpoint counts TCP arrivals,
// including failed TLS, so zero contact is stronger than a handler count.
func TestPlaywrightAuthorOptionalScriptLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_AUTHOR_LIVE_TEST") != "1" {
		t.Skip("explicit installed-browser loopback test")
	}
	for _, mode := range []string{"main", "redirect", "oopif", "strict_default", "wrong_resource", "request_limit", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			blocked := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("denied endpoint reached") }))
			listener := &optionalCountingListener{Listener: blocked.Listener}
			blocked.Listener = listener
			blocked.StartTLS()
			defer blocked.Close()
			var posts atomic.Int64
			var origin, childOrigin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts.Add(1)
					http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
					return
				}
				if r.URL.Path == "/redirect.js" {
					http.Redirect(w, r, blocked.URL+"/beacon.js", http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				if r.URL.Path == "/dashboard" {
					fmt.Fprint(w, `<h1>Dashboard</h1>`)
					return
				}
				script := blocked.URL + "/beacon.js"
				if mode == "redirect" {
					script = origin + "/redirect.js"
				}
				if mode == "wrong_resource" {
					fmt.Fprintf(w, `<link rel="stylesheet" href="%s">`, script)
				} else {
					fmt.Fprintf(w, `<script src="%s" onerror="document.documentElement.dataset.denied='yes'"></script>`, script)
				}
				if mode == "oopif" && r.URL.Path != "/child" {
					fmt.Fprintf(w, `<iframe name="child" src="%s/child"></iframe>`, childOrigin)
				}
				fmt.Fprint(w, `<main><form method="post" action="/login"><label>Email<input autocomplete="username"></label><label>Password<input type="password"></label><button>Sign in</button></form></main>`)
			}))
			defer server.Close()
			origin = server.URL
			childOrigin = strings.Replace(origin, "127.0.0.1", "localhost", 1)
			request := authorsession.BrowserRequest{URL: origin + "/login", ApprovedOrigins: []string{origin, childOrigin}, NavigationTimeout: 15 * time.Second, TotalTimeout: time.Minute, MaxRequests: 64, MaxResponseBytes: 1 << 20, MaxCandidates: 32}
			if mode == "request_limit" {
				request.MaxRequests = 1
			}
			selected := blocked.URL
			if mode == "strict_default" {
				selected = ""
			}
			browser, err := NewPlaywrightAuthorBrowserWithPolicy(os.Getenv("PLAYWRIGHT_DRIVER_PATH"), selected)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			session, err := browser.Open(ctx, request)
			if mode == "strict_default" || mode == "wrong_resource" || mode == "request_limit" {
				if session != nil {
					defer session.Close()
				}
				if err == nil {
					t.Fatal("fatal policy case continued")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := session.Close(); err != nil {
						t.Error(err)
					}
				}()
				native := session.(*playwrightAuthorSession)
				if denied, err := native.pages["main"].Locator("html").GetAttribute("data-denied"); err != nil || denied != "yes" {
					t.Fatal("main script denial was not observed")
				}
				if mode == "oopif" {
					frame := native.pages["main"].Frame(playwright.PageFrameOptions{Name: playwright.String("child")})
					if frame == nil {
						t.Fatal("missing child")
					}
					if denied, err := frame.Locator("html").GetAttribute("data-denied"); err != nil || denied != "yes" {
						t.Fatal("child script denial was not observed")
					}
					cdp, err := native.browser.NewBrowserCDPSession()
					if err != nil {
						t.Fatal(err)
					}
					result, err := cdp.Send("Target.getTargets", nil)
					_ = cdp.Detach()
					if err != nil {
						t.Fatal("target inventory unavailable")
					}
					found := false
					for _, value := range result.(map[string]any)["targetInfos"].([]any) {
						entry := value.(map[string]any)
						if entry["type"] == "iframe" && entry["url"] == childOrigin+"/child" {
							found = true
						}
					}
					if !found {
						t.Fatal("child was not out of process")
					}
				}
				if mode == "cancel" {
					cancel()
					if _, err := session.Observe(ctx, "main"); err == nil {
						t.Fatal("cancelled operation continued")
					}
				} else {
					observation, err := session.Observe(ctx, "main")
					if err != nil {
						t.Fatal(err)
					}
					var button authorsession.RawCandidate
					for _, c := range observation.Candidates {
						if c.Role == "button" && c.Label == "Sign in" {
							button = c
						}
					}
					if button.BackendID == "" || posts.Load() != 0 {
						t.Fatal("login readiness or pre-approval mutation")
					}
					_, err = session.Execute(ctx, authorsession.BrowserAction{Kind: "click", BackendID: button.BackendID, Context: "main", POSTBudget: 1, Role: button.Role, Label: button.Label, InputKind: button.InputKind, TargetOrigin: button.TargetOrigin, Matches: button.Matches})
					if err != nil {
						t.Fatal(err)
					}
					observation, err = session.Observe(ctx, "main")
					if mode == "oopif" {
						// The existing frame identity contract intentionally rejects a
						// previously observed child after top-level navigation detaches it.
						// Optional script denial must not suppress that separate error.
						if err == nil || err.Error() != "frame context is detached or inconsistent" || posts.Load() != 1 || native.guard.result() != nil {
							t.Fatal("detached-frame rejection was changed or policy was poisoned")
						}
					} else if err != nil || observation.Path != "/dashboard" || posts.Load() != 1 {
						t.Fatal("approved synthetic login failed")
					}
				}
			}
			if listener.arrivals.Load() != 0 {
				t.Fatal("optional origin received network contact")
			}
			if mode == "strict_default" || mode == "wrong_resource" || mode == "request_limit" || mode == "cancel" {
				if posts.Load() != 0 {
					t.Fatal("application mutation outside approval")
				}
			}
		})
	}
}
