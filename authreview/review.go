// Package authreview creates digest-bound, local-only review records for
// browser authentication profiles.
package authreview

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/authprofile"
)

const Version = "browsertools.authentication-review.v1"

// Bundle binds one exact profile to one freshness assessment.
type Bundle struct {
	Version       string              `json:"version"`
	Profile       authprofile.Profile `json:"profile"`
	ProfileDigest string              `json:"profile_digest"`
	AssessedAt    string              `json:"assessed_at"`
	ExpiresAt     string              `json:"expires_at"`
	Promotable    bool                `json:"promotable"`
	Gaps          []string            `json:"gaps"`
}

// Build creates a deterministic review bundle. Expired profiles produce a
// non-promotable bundle rather than being silently refreshed.
func Build(value *authprofile.Profile, at time.Time) (*Bundle, error) {
	if value == nil || at.IsZero() {
		return nil, fmt.Errorf("authentication review requires a profile and assessment time")
	}
	digest, err := authprofile.Digest(value)
	if err != nil {
		return nil, err
	}
	expires, err := authprofile.ExpiresAt(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copyValue authprofile.Profile
	if err := json.Unmarshal(data, &copyValue); err != nil {
		return nil, err
	}
	result := &Bundle{
		Version: Version, Profile: copyValue, ProfileDigest: digest,
		AssessedAt: at.UTC().Format(time.RFC3339), ExpiresAt: expires.Format(time.RFC3339),
		Promotable: true, Gaps: []string{},
	}
	if !at.Before(expires) {
		result.Promotable = false
		result.Gaps = []string{"profile_expired"}
	}
	return result, nil
}

// Verify proves the embedded profile digest and current lifecycle.
func Verify(value *Bundle, at time.Time) error {
	if value == nil || value.Version != Version || at.IsZero() {
		return fmt.Errorf("invalid authentication review bundle")
	}
	digest, err := authprofile.Digest(&value.Profile)
	if err != nil {
		return err
	}
	if digest != value.ProfileDigest {
		return fmt.Errorf("authentication review profile digest mismatch")
	}
	if err := authprofile.ValidateAt(&value.Profile, at); err != nil {
		return err
	}
	if !value.Promotable || len(value.Gaps) != 0 {
		return fmt.Errorf("authentication review is not promotable")
	}
	return nil
}
