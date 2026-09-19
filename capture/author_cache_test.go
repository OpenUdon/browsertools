package capture

import (
	"errors"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

type authorCacheRoute struct {
	playwright.Route
	continued, aborted int
	changed            bool
	err                error
}

func (r *authorCacheRoute) Continue(options ...playwright.RouteContinueOptions) error {
	r.continued++
	r.changed = len(options) != 0
	return r.err
}

func (r *authorCacheRoute) Abort(code ...string) error {
	r.aborted++
	r.changed = len(code) != 1 || code[0] != "blockedbyclient"
	return r.err
}

func TestAuthorCacheRoutePreservesRequestAndFirstFailure(t *testing.T) {
	for _, mode := range []string{"active", "failed", "closing", "continue_error", "abort_error"} {
		t.Run(mode, func(t *testing.T) {
			g := newAuthorNetworkGuard(authorsession.BrowserRequest{MaxRequests: 8, MaxResponseBytes: 1024})
			r := &authorCacheRoute{}
			if mode == "failed" || mode == "abort_error" {
				g.block("post_budget")
			}
			if mode == "closing" {
				g.beginClose()
			}
			if strings.HasSuffix(mode, "error") {
				r.err = errors.New("private-token-canary")
			}
			g.resumeUnmodifiedRoute(r)
			if r.changed {
				t.Fatal("route changed native request or abort policy")
			}
			if mode == "active" || mode == "continue_error" {
				if r.continued != 1 || r.aborted != 0 {
					t.Fatal("active route outcome")
				}
			} else if r.continued != 0 || r.aborted != 1 {
				t.Fatal("inactive route resumed")
			}
			want := "none"
			if mode == "continue_error" {
				want = "route_continue"
			}
			if mode == "failed" || mode == "abort_error" {
				want = "post_budget"
			}
			if authordiagnostic.Classify(g.result()).Reason != want {
				t.Fatal("first failure changed")
			}
			if err := g.result(); err != nil && strings.Contains(err.Error(), "private-token-canary") {
				t.Fatal("private transport error leaked")
			}
			if g.core.requests != 0 || g.core.responseBytes != 0 {
				t.Fatal("cache route duplicated admission accounting")
			}
		})
	}
}
