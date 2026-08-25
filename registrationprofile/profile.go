// Package registrationprofile loads and validates portable
// uws.browser-registration.1.0 account-creation recipes.
//
// The package is offline authoring tooling only. It never resolves credentials,
// launches a browser, submits a registration, handles human verification,
// stores account values, or performs cleanup.
package registrationprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/internal/profiledocument"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/uws/browserregistration"
	"github.com/OpenUdon/uws/schemas"
	"gopkg.in/yaml.v3"
)

const MaxProfileBytes = 1 << 20

// Profile is the public UWS wire type.
type Profile = browserregistration.Profile

// Parse validates and decodes one JSON or YAML registration profile.
func Parse(data []byte) (*Profile, error) {
	value, err := profiledocument.DecodeAndReject(data, "registration profile")
	if err != nil {
		return nil, err
	}
	if err := schemas.ValidateBrowserRegistrationProfile(data); err != nil {
		return nil, fmt.Errorf("registration profile: %w", err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize registration profile: %w", err)
	}
	var result Profile
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode typed registration profile: %w", err)
	}
	return &result, nil
}

// LoadFile reads a regular JSON/YAML file without following a final symlink.
func LoadFile(path string) (*Profile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read registration profile %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("registration profile %s is not a regular file", path)
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return nil, fmt.Errorf("unsupported registration profile extension %q", ext)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registration profile %s: %w", path, err)
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
		return nil, fmt.Errorf("registration profile is required")
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
		return time.Time{}, fmt.Errorf("registration profile is required")
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
		return fmt.Errorf("registration profile and assessment time are required")
	}
	if _, err := MarshalJSON(value); err != nil {
		return err
	}
	expires, err := ExpiresAt(value)
	if err != nil {
		return err
	}
	if !at.Before(expires) {
		return fmt.Errorf("registration profile expired at %s", expires.Format(time.RFC3339))
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

// Origins returns a stable de-duplicated union of application and registration
// origins.
func Origins(value *Profile) []string {
	if value == nil {
		return []string{}
	}
	set := map[string]struct{}{}
	for _, origin := range append(append([]string{}, value.Info.ApplicationOrigins...), value.Info.RegistrationOrigins...) {
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
