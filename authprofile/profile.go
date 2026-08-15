// Package authprofile loads and validates portable
// uws.browser-authentication.1.0 sign-in recipes.
//
// The package is authoring tooling only. It never resolves credentials,
// launches a browser, stores sessions, or performs authentication.
package authprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/evidence/redact"
	"github.com/OpenUdon/uws/browserauthentication"
	"github.com/OpenUdon/uws/versions"
	"gopkg.in/yaml.v3"
)

const MaxProfileBytes = 1 << 20

// Profile is the public UWS wire type.
type Profile = browserauthentication.Profile

var (
	emailValuePattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phoneValuePattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
)

// Parse validates and decodes one JSON or YAML authentication profile.
func Parse(data []byte) (*Profile, error) {
	if err := versions.ValidateBrowserAuthenticationProfile(data); err != nil {
		return nil, fmt.Errorf("authentication profile: %w", err)
	}
	value, err := decodeOne(data)
	if err != nil {
		return nil, err
	}
	if err := rejectSensitiveValues(value, "$"); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize authentication profile: %w", err)
	}
	var result Profile
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode typed authentication profile: %w", err)
	}
	return &result, nil
}

// LoadFile reads a regular JSON/YAML file without following a final symlink.
func LoadFile(path string) (*Profile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read authentication profile %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("authentication profile %s is not a regular file", path)
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return nil, fmt.Errorf("unsupported authentication profile extension %q", ext)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read authentication profile %s: %w", path, err)
	}
	value, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

// MarshalJSON returns deterministic compact JSON suitable for digesting.
func MarshalJSON(value *Profile) ([]byte, error) {
	if value == nil {
		return nil, fmt.Errorf("authentication profile is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if _, err := Parse(data); err != nil {
		return nil, err
	}
	return data, nil
}

// MarshalYAML returns a validated YAML representation.
func MarshalYAML(value *Profile) ([]byte, error) {
	data, err := MarshalJSON(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	return yaml.Marshal(normalized)
}

// Digest returns the SHA-256 identity of the canonical typed profile.
func Digest(value *Profile) (string, error) {
	data, err := MarshalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ExpiresAt calculates the inclusive stale instant from verification metadata.
func ExpiresAt(value *Profile) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("authentication profile is required")
	}
	verified, err := time.Parse(time.RFC3339, value.Verification.LastVerifiedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("verification.lastVerifiedAt: %w", err)
	}
	expires, err := profile.Duration(value.ExpiresAfter).AddTo(verified)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiresAfter: %w", err)
	}
	return expires.UTC().Round(0), nil
}

// ValidateAt rejects a profile at or after its expiry instant.
func ValidateAt(value *Profile, at time.Time) error {
	if value == nil || at.IsZero() {
		return fmt.Errorf("authentication profile and assessment time are required")
	}
	if _, err := MarshalJSON(value); err != nil {
		return err
	}
	expires, err := ExpiresAt(value)
	if err != nil {
		return err
	}
	if !at.Before(expires) {
		return fmt.Errorf("authentication profile expired at %s", expires.Format(time.RFC3339))
	}
	return nil
}

// SortedFlowNames returns stable flow inventory order.
func SortedFlowNames(value *Profile) []string {
	if value == nil {
		return []string{}
	}
	result := make([]string, 0, len(value.Flows))
	for name := range value.Flows {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// Origins returns a stable de-duplicated union of application and
// authentication origins.
func Origins(value *Profile) []string {
	if value == nil {
		return []string{}
	}
	set := map[string]struct{}{}
	for _, origin := range append(append([]string{}, value.Info.ApplicationOrigins...), value.Info.AuthenticationOrigins...) {
		canonical, err := profile.ParseOrigin(origin)
		if err == nil {
			set[canonical] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for origin := range set {
		result = append(result, origin)
	}
	sort.Strings(result)
	return result
}

func decodeOne(data []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse authentication profile: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse authentication profile: multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("parse authentication profile trailing document: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func rejectSensitiveValues(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := rejectSensitiveValues(typed[key], path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := rejectSensitiveValues(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case string:
		if redact.String(typed) != typed || strings.Contains(typed, redact.Value) || strings.Contains(typed, "[REDACTED]") {
			return fmt.Errorf("authentication profile contains a secret-shaped value at %s", path)
		}
		if emailValuePattern.MatchString(typed) || phoneValuePattern.MatchString(typed) {
			return fmt.Errorf("authentication profile contains a PII-shaped value at %s", path)
		}
	}
	return nil
}
