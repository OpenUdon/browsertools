package capture

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
)

func TestPlaywrightRegistrationV3LoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_REGISTRATION_LIVE_TEST") != "1" {
		t.Skip("explicit installed Chromium registration qualification required")
	}
	for _, scenario := range []struct {
		name string
		html string
		pass bool
	}{
		{"conditional_wizard", registrationfixture.HTML, true},
		{"mutation_blocked", strings.Replace(registrationfixture.HTML, "document.getElementById('identity').hidden=true;", "fetch('/saved',{method:'POST'});document.getElementById('identity').hidden=true;", 1), false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var mutations atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" && r.Method != "HEAD" {
					mutations.Add(1)
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(scenario.html))
			}))
			defer server.Close()
			at := time.Now().UTC().Truncate(time.Second)
			completion, err := registrationfixture.Author(t.Context(), NewPlaywrightRegistrationBrowser(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), server.URL, at)
			if mutations.Load() != 0 {
				t.Fatal("registration preview transmitted a mutation")
			}
			if !scenario.pass {
				if err == nil || completion != nil {
					t.Fatal("unsafe preview produced a candidate")
				}
				return
			}
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
			if len(result.History) != 3 || len(result.Previews) != 2 || result.Network.MutationRequests != 0 {
				t.Fatal("missing real v3 producer evidence")
			}
		})
	}
}
