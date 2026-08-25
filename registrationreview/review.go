// Package registrationreview creates digest-bound, local-only review records
// for browser registration profiles.
package registrationreview

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenUdon/browsertools/registrationprofile"
)

const Version = "browsertools.registration-review.v1"

// Bundle binds one exact inert profile to one freshness assessment. It is not
// evidence of an account-creation attempt or result.
type Bundle struct {
	Version       string                      `json:"version"`
	Profile       registrationprofile.Profile `json:"profile"`
	ProfileDigest string                      `json:"profile_digest"`
	AssessedAt    string                      `json:"assessed_at"`
	ExpiresAt     string                      `json:"expires_at"`
	Promotable    bool                        `json:"promotable"`
	Gaps          []string                    `json:"gaps"`
}

// Build creates a deterministic review bundle. Expired profiles produce a
// non-promotable bundle rather than being silently refreshed.
func Build(value *registrationprofile.Profile, at time.Time) (*Bundle, error) {
	if value == nil || at.IsZero() {
		return nil, fmt.Errorf("registration review requires a profile and assessment time")
	}
	digest, err := registrationprofile.Digest(value)
	if err != nil {
		return nil, err
	}
	expires, err := registrationprofile.ExpiresAt(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copyValue registrationprofile.Profile
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
		return fmt.Errorf("invalid registration review bundle")
	}
	assessed, err := time.Parse(time.RFC3339, value.AssessedAt)
	if err != nil || assessed.UTC().Format(time.RFC3339) != value.AssessedAt {
		return fmt.Errorf("registration review assessment time is invalid")
	}
	if assessed.After(at) {
		return fmt.Errorf("registration review assessment time is in the future")
	}
	digest, err := registrationprofile.Digest(&value.Profile)
	if err != nil {
		return err
	}
	if digest != value.ProfileDigest {
		return fmt.Errorf("registration review profile digest mismatch")
	}
	expires, err := registrationprofile.ExpiresAt(&value.Profile)
	if err != nil {
		return err
	}
	if value.ExpiresAt != expires.Format(time.RFC3339) {
		return fmt.Errorf("registration review expiry mismatch")
	}
	wasPromotable := assessed.Before(expires)
	if value.Promotable != wasPromotable || (wasPromotable && len(value.Gaps) != 0) || (!wasPromotable && (len(value.Gaps) != 1 || value.Gaps[0] != "profile_expired")) {
		return fmt.Errorf("registration review assessment mismatch")
	}
	if err := registrationprofile.ValidateAt(&value.Profile, at); err != nil {
		return err
	}
	if !value.Promotable || len(value.Gaps) != 0 {
		return fmt.Errorf("registration review is not promotable")
	}
	return nil
}
