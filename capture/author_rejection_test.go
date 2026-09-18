package capture

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

func TestAuthorOriginRejectionRelationshipsAndFirstCause(t *testing.T) {
	for _, tc := range []struct{ raw, relation string }{
		{"https://external.invalid/path?token=TOKEN_CANARY#COOKIE_CANARY", "host_mismatch"},
		{"https://example.test:444/", "port_mismatch"},
		{"http://example.test:443/", "scheme_mismatch"},
		{"http://example.test/", "scheme_and_port_mismatch"},
		{"https://TOKEN_CANARY:COOKIE_CANARY@example.test/", "userinfo"},
		{"https://%zz/", "invalid_url"},
		{"https:///path", "missing_host"},
		{"data:text/html,TOKEN_CANARY", "local_scheme"},
		{"blob:https://example.test/TOKEN_CANARY", "local_scheme"},
		{"ftp://example.test/TOKEN_CANARY", "unsupported_scheme"},
	} {
		t.Run(tc.relation, func(t *testing.T) {
			g := newAuthorNetworkGuard(authorsession.BrowserRequest{ApprovedOrigins: []string{"https://example.test"}, MaxRequests: 8, MaxResponseBytes: 1024})
			if g.allowRequest(tc.raw, "GET", false, "script") {
				t.Fatal("request escaped")
			}
			want := authordiagnostic.Rejection{Boundary: "request", Resource: "script", OriginRelation: tc.relation}
			if got := authordiagnostic.RejectionOf(g.result()); got != want {
				t.Fatal(got, want)
			}
			if g.allowRequest("https://example.test/", "GET", false, "image") {
				t.Fatal("poisoned guard admitted request")
			}
			_, _, err := g.urlFacts("https://example.test/")
			if authordiagnostic.RejectionOf(err) != want {
				t.Fatal("observation replaced first request cause")
			}
			data, _ := json.Marshal(authordiagnostic.RejectionOf(err))
			if strings.Contains(string(data), "CANARY") {
				t.Fatal("raw value leaked")
			}
		})
	}
}

type rejectionPage struct {
	playwright.Page
	url    string
	frames []playwright.Frame
}

func (p *rejectionPage) URL() string                { return p.url }
func (*rejectionPage) IsClosed() bool               { return false }
func (p *rejectionPage) Frames() []playwright.Frame { return p.frames }

type rejectionFrame struct {
	playwright.Frame
	parent playwright.Frame
}

func (*rejectionFrame) URL() string                     { return "https://outside.invalid/path?token=TOKEN_CANARY" }
func (*rejectionFrame) Name() string                    { return "child" }
func (*rejectionFrame) IsDetached() bool                { return false }
func (f *rejectionFrame) ParentFrame() playwright.Frame { return f.parent }

func TestAuthorKnownContextPreservesOriginRejection(t *testing.T) {
	for _, kind := range []string{"popup", "frame"} {
		for _, firstRequest := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "_observation", true: "_first_request"}[firstRequest], func(t *testing.T) {
				g := newAuthorNetworkGuard(authorsession.BrowserRequest{ApprovedOrigins: []string{"https://example.test"}, MaxRequests: 8, MaxResponseBytes: 1024})
				want := authordiagnostic.Rejection{Boundary: "observation", Resource: "document", OriginRelation: "host_mismatch"}
				if firstRequest {
					g.allowRequest("https://example.test:444/script", "GET", false, "script")
					want = authordiagnostic.Rejection{Boundary: "request", Resource: "script", OriginRelation: "port_mismatch"}
				}
				s := &playwrightAuthorSession{guard: g, contexts: map[string]authorresult.Context{}}
				if kind == "popup" {
					s.pages = map[string]playwright.Page{"popup_1": &rejectionPage{url: "https://outside.invalid/path?token=TOKEN_CANARY"}}
					s.contexts["popup_1"] = authorresult.Context{Kind: "popup", Origin: "https://example.test", Parent: "main"}
				} else {
					parent := &rejectionFrame{}
					frame := &rejectionFrame{parent: parent}
					s.pages = map[string]playwright.Page{"main": &rejectionPage{url: "https://example.test", frames: []playwright.Frame{parent, frame}}}
					s.frames = map[string]playwright.Frame{"frame_1": frame}
					s.frameIDs = map[playwright.Frame]string{parent: "main"}
					s.contexts["frame_1"] = authorresult.Context{Kind: "frame", Origin: "https://example.test", Path: "/", Name: "child", Parent: "main"}
				}
				err := s.discoverFrames()
				if got := authordiagnostic.RejectionOf(err); got != want || strings.Contains(err.Error(), "CANARY") {
					t.Fatal("context discarded rejection", got)
				}
			})
		}
	}
}

func TestAuthorObservationRejectionAndResourceReduction(t *testing.T) {
	g := newAuthorNetworkGuard(authorsession.BrowserRequest{ApprovedOrigins: []string{"https://EXAMPLE.test:443"}, MaxRequests: 8, MaxResponseBytes: 1024})
	if _, _, err := g.urlFacts("https://example.test/path?private=TOKEN_CANARY"); err != nil {
		t.Fatal(err)
	}
	_, _, err := g.urlFacts("https://example.test:444/path?private=TOKEN_CANARY")
	want := authordiagnostic.Rejection{Boundary: "observation", Resource: "document", OriginRelation: "port_mismatch"}
	if got := authordiagnostic.RejectionOf(err); got != want {
		t.Fatal(got)
	}
	for _, kind := range []string{"Document", "Stylesheet", "Image", "Media", "Font", "Script", "XHR", "Fetch", "EventSource", "Manifest", "TextTrack", "Ping", "Prefetch", "TOKEN_CANARY"} {
		r := authordiagnostic.Rejection{Boundary: "request", Resource: authorResource(kind), OriginRelation: "host_mismatch"}
		if !r.ValidFor(authordiagnostic.Class{Stage: "policy", Reason: "origin_escape"}) || strings.Contains(r.Resource, "CANARY") {
			t.Fatal(r)
		}
	}
}
