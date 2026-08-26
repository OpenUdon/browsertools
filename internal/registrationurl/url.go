// Package registrationurl validates the exact URLs that may be retained by
// browser-registration authoring and execution contracts.
package registrationurl

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/evidence/redact"
)

const (
	MaxURLBytes        = 4096
	MaxQueryBytes      = 1024
	MaxQueryItems      = 16
	MaxQueryKeyBytes   = 64
	MaxQueryValueBytes = 256
)

// Facts is the canonical URL plus the only facts safe for observation output.
// Query is deliberately absent.
type Facts struct {
	URL    string
	Origin string
	Path   string
}

// Parse accepts one canonical absolute, fragment-free URL. When allowQuery is
// false it preserves the v1 query rejection. When true it admits only a
// bounded canonical literal query with unique nonempty safe keys and values.
// Errors never include rejected URL material.
func Parse(raw string, allowQuery bool, canonicalOrigin func(string) (string, error)) (Facts, error) {
	if canonicalOrigin == nil || raw != strings.TrimSpace(raw) || len(raw) == 0 || len(raw) > MaxURLBytes || !utf8.ValidString(raw) {
		return Facts{}, errors.New("registration URL is not canonical and bounded")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.ForceQuery {
		return Facts{}, errors.New("registration URL must be absolute and fragment-free without userinfo")
	}
	origin, err := canonicalOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return Facts{}, errors.New("registration URL origin is invalid")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if disclosurepath.Validate(path) != nil {
		return Facts{}, errors.New("registration URL path is not disclosure-safe")
	}
	if containsTemplate(parsed.Path) {
		return Facts{}, errors.New("registration URL path contains a template")
	}
	query := ""
	if parsed.RawQuery != "" {
		if !allowQuery {
			return Facts{}, errors.New("registration URL query is not allowed")
		}
		canonical, err := canonicalQuery(parsed.RawQuery)
		if err != nil {
			return Facts{}, err
		}
		query = "?" + canonical
	}
	canonical := origin + path + query
	if raw != canonical {
		return Facts{}, errors.New("registration URL is not canonical")
	}
	return Facts{URL: canonical, Origin: origin, Path: path}, nil
}

func canonicalQuery(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > MaxQueryBytes || !utf8.ValidString(raw) || strings.ContainsAny(raw, "{}") {
		return "", errors.New("registration URL query is not a bounded literal")
	}
	parts := strings.Split(raw, "&")
	if len(parts) == 0 || len(parts) > MaxQueryItems {
		return "", errors.New("registration URL query item count is invalid")
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) != len(parts) {
		return "", errors.New("registration URL query encoding is invalid")
	}
	for _, part := range parts {
		if part == "" || !strings.Contains(part, "=") {
			return "", errors.New("registration URL query item is not canonical")
		}
	}
	for key, items := range values {
		if key == "" || len(key) > MaxQueryKeyBytes || !utf8.ValidString(key) || !portableQueryKey.MatchString(key) || len(items) != 1 || sensitiveKey(key) || containsTemplate(key) {
			return "", errors.New("registration URL query key is invalid")
		}
		value := items[0]
		if len(value) > MaxQueryValueBytes || !utf8.ValidString(value) || containsTemplate(value) || unsafeValue(value) {
			return "", errors.New("registration URL query value is invalid")
		}
	}
	canonical := values.Encode()
	if canonical != raw {
		return "", errors.New("registration URL query is not canonically encoded")
	}
	return canonical, nil
}

func sensitiveKey(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "auth", "code", "key", "oauth_state", "session", "session_id", "sig", "signature", "state":
		return true
	default:
		return redact.SensitiveKey(normalized)
	}
}

func containsTemplate(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "}}") ||
		strings.Contains(value, "${") || strings.ContainsAny(value, "\r\n")
}

func unsafeValue(value string) bool {
	if redact.String(value) != value || emailPattern.MatchString(value) || phonePattern.MatchString(value) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "secret", "password", "passwd", "token", "credential", "private_key":
		return true
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

var (
	portableQueryKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._~-]{0,63}$`)
	emailPattern     = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern     = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
)
