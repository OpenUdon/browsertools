package capture

import (
	"bytes"
	"context"
	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/uws/browserregistration"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlaywrightRegistrationVerificationLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_VERIFICATION_LIVE_TEST") != "1" {
		t.Skip("explicit synthetic Chromium verification smoke required")
	}
	for _, provider := range []string{"turnstile", "recaptcha_v2", "hcaptcha"} {
		t.Run(provider, func(t *testing.T) {
			marker := map[string]string{"turnstile": "cf-turnstile", "recaptcha_v2": "g-recaptcha", "hcaptcha": "h-captcha"}[provider]
			html := strings.Replace(registrationfixture.HTML, "</form>", `<div class="`+marker+`"></div><input type="hidden" name="provider-private-response" value="never-export-response-canary"></form>`, 1)
			for _, name := range []string{"action", "method", "target", "contains", "append", "submit"} {
				html = strings.Replace(html, "</form>", `<input type="hidden" name="`+name+`" value="named-control-canary"></form>`, 1)
			}
			var mutations atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" && r.Method != "HEAD" {
					mutations.Add(1)
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(html))
			}))
			defer server.Close()
			descriptor := &browserregistration.HumanVerification{Provider: provider, Activation: "before_approval", WidgetBinding: "single_in_submit_form", SubmissionURL: server.URL + "/register", Dependencies: browserregistration.VerificationDependencies{Policy: provider + ".v1", MaxRequests: 256, MaxResponseBytes: 32 << 20, TimeoutMS: 120000}}
			at := time.Now().UTC().Truncate(time.Second)
			completion, err := registrationfixture.AuthorVerification(t.Context(), NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), server.URL, at, descriptor)
			if err != nil {
				t.Fatal(err)
			}
			result, err := registrationauthorresult.Build(registrationauthorresult.BuildRequest{Completion: completion, CreatedAt: at})
			if err != nil {
				t.Fatal(err)
			}
			if err := registrationauthorresult.Verify(result, at); err != nil {
				t.Fatal(err)
			}
			data, err := registrationauthorresult.MarshalDeterministic(result)
			if err != nil {
				t.Fatal(err)
			}
			if mutations.Load() != 0 || bytes.Contains(data, []byte("never-export-response-canary")) || bytes.Contains(data, []byte("named-control-canary")) {
				t.Fatal("authoring leaked a value or transmitted a mutation")
			}
		})
	}
}

func TestVerificationNetworkPolicyBoundaries(t *testing.T) {
	for provider, good := range map[string]string{"turnstile": "https://challenges.cloudflare.com/turnstile/v0/api.js", "recaptcha_v2": "https://www.recaptcha.net/recaptcha/api2/reload", "hcaptcha": "https://new.assets.hcaptcha.com/a"} {
		if !permitsRegistrationVerification(provider, good, "POST", false) {
			t.Fatal("reviewed provider denied")
		}
		for _, bad := range []string{strings.Replace(good, "https:", "http:", 1), strings.Replace(good, "https://", "https://user@", 1), strings.Replace(good, ".com/", ".com.evil.example/", 1), strings.Replace(good, ".net/", ".net.evil.example/", 1)} {
			if bad != good && permitsRegistrationVerification(provider, bad, "GET", false) {
				t.Fatal("escaped provider policy")
			}
		}
	}
}

func TestPlaywrightRegistrationDOMAPICollisionOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_VERIFICATION_LIVE_TEST") != "1" {
		t.Skip("explicit synthetic Chromium verification smoke required")
	}
	// The pinned accessibility/click implementation uses these DOM methods.
	// An unsupported page must stop, never fall back to unreviewed interaction.
	for _, name := range []string{"getAttribute", "hasAttribute"} {
		t.Run(name, func(t *testing.T) {
			html := strings.Replace(registrationfixture.HTML, "</form>", `<div class="cf-turnstile"></div><input type="hidden" name="`+name+`" value="named-control-canary"></form>`, 1)
			var mutations atomic.Int64
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" && r.Method != "HEAD" {
					mutations.Add(1)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(html))
			}))
			defer fixture.Close()
			descriptor := &browserregistration.HumanVerification{Provider: "turnstile", Activation: "before_approval", WidgetBinding: "single_in_submit_form", SubmissionURL: fixture.URL + "/register", Dependencies: browserregistration.VerificationDependencies{Policy: "turnstile.v1", MaxRequests: 256, MaxResponseBytes: 32 << 20, TimeoutMS: 120000}}
			completion, err := registrationfixture.AuthorVerification(t.Context(), NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), fixture.URL, time.Now().UTC().Truncate(time.Second), descriptor)
			if err == nil || completion != nil || !strings.Contains(err.Error(), "browser_failure") || strings.Contains(err.Error(), "canary") || mutations.Load() != 0 {
				t.Fatal("unsupported DOM API collision did not stop privately before mutation")
			}
		})
	}
}

func TestPlaywrightVerificationNativeFormBindingOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_VERIFICATION_LIVE_TEST") != "1" {
		t.Skip("explicit synthetic Chromium verification smoke required")
	}
	for _, tc := range []struct {
		name, form, submit string
		want               bool
		wantError          bool
	}{
		{"relative destination", `method="post" action="register"`, "", true, false},
		{"same override", `method="POST" action="register"`, `formaction="register" formmethod="POST" formtarget="_self"`, true, false},
		{"other override", `method="post" action="register"`, `formaction="https://escape.example/register"`, false, false},
		{"empty override", `method="post" action="register"`, `formaction=""`, false, false},
		{"get override", `method="post" action="register"`, `formmethod="get"`, false, false},
		{"popup override", `method="post" action="register"`, `formtarget="_blank"`, false, false},
		{"get form", `method="get" action="register"`, "", false, false},
		{"invalid method", `method="invalid" action="register"`, "", false, false},
		{"popup form", `method="post" action="register" target="_blank"`, "", false, false},
		{"masked DOM API", `method="post" action="register"`, "", false, true},
		{"unapproved form", `method="post" action="https://escape.example/register"`, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := `<form ` + tc.form + `><div class="cf-turnstile"></div>`
			for _, name := range []string{"action", "method", "target", "contains", "hasAttribute", "submit", "append"} {
				html += `<input type="hidden" name="` + name + `" value="private-value-canary">`
			}
			if tc.name == "masked DOM API" {
				html += `<input type="hidden" name="getAttribute" value="private-value-canary">`
			}

			html += `<button type="submit" ` + tc.submit + `>Register</button></form>`
			var mutations atomic.Int64
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" && r.Method != "HEAD" {
					mutations.Add(1)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(html))
			}))
			defer fixture.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			session, err := NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")).Open(ctx, registrationauthorsession.BrowserRequest{
				Protocol: registrationauthorsession.ProtocolV4, URL: fixture.URL + "/forms/start", ApprovedOrigins: []string{fixture.URL},
				NavigationTimeout: 5 * time.Second, TotalTimeout: 10 * time.Second, MaxRequests: 32, MaxResponseBytes: 1 << 20, MaxCandidates: 32,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				if _, err := session.Close(closeCtx); err != nil || mutations.Load() != 0 {
					t.Fatal("authoring mutation or incomplete teardown")
				}
			}()
			observation, err := session.Observe(ctx)
			if (err != nil) != tc.wantError {
				t.Fatalf("observation error = %v", err)
			}
			if tc.wantError {
				want := "invalid verification metadata"
				if tc.name == "masked DOM API" {
					want = "observe registration accessibility candidate"
				}
				if err.Error() != want {
					t.Fatalf("wrong closed failure: %v", err)
				}
			}

			found := false
			for _, candidate := range observation.Candidates {
				if candidate.Verification != nil {
					found = true
					if candidate.Verification.SubmissionURL != fixture.URL+"/forms/register" {
						t.Fatal("native relative destination changed")
					}
				}
			}
			if found != tc.want {
				t.Fatal("unexpected verification binding")
			}
		})
	}
}
