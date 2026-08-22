// Package disclosurepath validates URL paths before page-derived path text may
// cross the Browsertools author-session boundary.
package disclosurepath

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OpenUdon/evidence/redact"
)

const (
	MaxEscapedBytes = 2 << 10
	MaxSegmentBytes = 256
)

// Validate accepts one absolute escaped URL path. Errors intentionally never
// include the rejected path or segment.
func Validate(path string) error {
	if len(path) > MaxEscapedBytes {
		return fmt.Errorf("disclosure path exceeds the byte limit")
	}
	if !utf8.ValidString(path) || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#\\") {
		return fmt.Errorf("disclosure path syntax is invalid")
	}
	parts := strings.Split(path, "/")
	for index, escaped := range parts[1:] {
		last := index == len(parts)-2
		if escaped == "" {
			if last { // root and one trailing slash are canonical enough for disclosure.
				continue
			}
			return fmt.Errorf("disclosure path contains an empty interior segment")
		}
		decoded, err := url.PathUnescape(escaped)
		if err != nil || !utf8.ValidString(decoded) || len(decoded) > MaxSegmentBytes {
			return fmt.Errorf("disclosure path segment is invalid")
		}
		if decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\") {
			return fmt.Errorf("disclosure path segment is not portable")
		}
		for _, r := range decoded {
			if unicode.IsControl(r) {
				return fmt.Errorf("disclosure path contains control text")
			}
		}
		if unsafeSegment(decoded) {
			return fmt.Errorf("disclosure path contains unsafe text")
		}
	}
	return nil
}

func unsafeSegment(value string) bool {
	lower := strings.ToLower(value)
	for _, phrase := range promptInjectionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return emailPattern.MatchString(value) || phonePattern.MatchString(value) ||
		credentialAssignmentPattern.MatchString(value) || redact.String(value) != value
}

var (
	promptInjectionPhrases = []string{
		"ignore previous", "ignore prior", "ignore all instructions", "system prompt",
		"developer message", "tool call", "reveal secrets", "reveal credentials",
	}
	emailPattern                = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern                = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
	credentialAssignmentPattern = regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|app[_-]?id|appid|token|secret|password|authorization|credential|private[_-]?key)\s*[:=]`)
)
