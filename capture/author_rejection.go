package capture

import (
	"net/url"
	"strings"

	"github.com/OpenUdon/browsertools/authordiagnostic"
)

func authorResource(kind string) string {
	switch kind {
	case "Document":
		return "document"
	case "Stylesheet":
		return "stylesheet"
	case "Image":
		return "image"
	case "Media":
		return "media"
	case "Font":
		return "font"
	case "Script":
		return "script"
	case "XHR":
		return "xhr"
	case "Fetch":
		return "fetch"
	case "EventSource":
		return "event_source"
	case "Manifest":
		return "manifest"
	case "TextTrack":
		return "text_track"
	case "Ping":
		return "ping"
	case "Prefetch":
		return "prefetch"
	default:
		return "other"
	}
}

func effectiveAuthorPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	if u.Scheme == "http" {
		return "80"
	}
	return ""
}

// Classify only after admission rejects. Never use this reducer to admit a URL.
// Compare all approved origins deterministically; a query/path never survives.
func authorOriginRelation(raw string, origins map[string]struct{}) string {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return "invalid_url"
	}
	if u.User != nil {
		return "userinfo"
	}
	switch u.Scheme {
	case "about", "data", "blob":
		return "local_scheme"
	case "https", "http":
	default:
		return "unsupported_scheme"
	}
	if u.Host == "" || u.Hostname() == "" {
		return "missing_host"
	}
	sameHost, sameScheme, samePort := false, false, false
	for origin := range origins {
		approved, err := url.Parse(origin)
		if err != nil {
			continue
		}
		if !strings.EqualFold(u.Hostname(), approved.Hostname()) {
			continue
		}
		sameHost = true
		if u.Scheme == approved.Scheme {
			sameScheme = true
		}
		if effectiveAuthorPort(u) == effectiveAuthorPort(approved) {
			samePort = true
		}
		if u.Scheme == approved.Scheme && effectiveAuthorPort(u) == effectiveAuthorPort(approved) {
			return "unknown"
		}
	}
	if !sameHost {
		return "host_mismatch"
	}
	if sameScheme {
		return "port_mismatch"
	}
	if samePort {
		return "scheme_mismatch"
	}
	return "scheme_and_port_mismatch"
}

func (g *authorNetworkGuard) urlFacts(raw string) (string, string, error) {
	origin, path, err := authorURLFacts(raw, g.allowedOrigin)
	if authordiagnostic.Classify(err) != (authordiagnostic.Class{Stage: "policy", Reason: "origin_escape"}) {
		return origin, path, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	// A prior request rejection must not become an unrelated observation failure.
	if first := g.core.result(); first != nil {
		return "", "", first
	}
	return "", "", &authordiagnostic.RejectionError{Rejection: authordiagnostic.Rejection{Boundary: "observation", Resource: "document", OriginRelation: authorOriginRelation(raw, g.origins)}}
}
