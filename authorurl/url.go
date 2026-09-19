// Package authorurl validates operator-reviewed authenticated navigation.
// Observed page URLs must continue to expose only origin and path.
package authorurl

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/browsertools/internal/registrationurl"
	"github.com/OpenUdon/browsertools/profile"
)

// Normalize preserves a bounded, canonical literal query on a reviewed URL.
// It shares the established structural-query validator without changing any
// registration protocol. Errors never include rejected input.
func Normalize(raw string) (string, string, error) {
	invalid := errors.New("navigation URL must not contain unsafe or noncanonical data")
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > registrationurl.MaxURLBytes {
		return "", "", invalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || strings.Contains(raw, "#") || parsed.ForceQuery {
		return "", "", invalid
	}
	for key := range parsed.Query() {
		if strings.EqualFold(key, "private") {
			return "", "", invalid
		}
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (strings.EqualFold(parsed.Hostname(), "localhost") || net.ParseIP(parsed.Hostname()).IsLoopback())) {
		return "", "", errors.New("navigation URL must use HTTPS or loopback HTTP")
	}
	origin, err := profile.ParseOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return "", "", invalid
	}
	canonical, err := url.Parse(origin)
	if err != nil {
		return "", "", invalid
	}
	parsed.Scheme, parsed.Host = canonical.Scheme, canonical.Host
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if disclosurepath.Validate(parsed.EscapedPath()) != nil {
		return "", "", errors.New("navigation URL path is unsafe")
	}
	facts, err := registrationurl.Parse(parsed.String(), true, profile.ParseOrigin)
	if err != nil {
		return "", "", invalid
	}
	return facts.URL, facts.Origin, nil
}
