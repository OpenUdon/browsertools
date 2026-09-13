package capture

import (
	"bytes"
	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
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
			if mutations.Load() != 0 || bytes.Contains(data, []byte("never-export-response-canary")) {
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
