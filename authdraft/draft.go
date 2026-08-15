// Package authdraft deterministically builds validated browser authentication
// profiles from explicit author specifications.
package authdraft

import (
	"encoding/json"
	"fmt"

	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/uws/browserauthentication"
)

// Spec is an explicit authentication profile with the discriminator omitted.
// No action or challenge is inferred from observation evidence.
type Spec struct {
	Info            browserauthentication.Info                      `json:"info" yaml:"info"`
	ObservationKind string                                          `json:"observationKind" yaml:"observationKind"`
	Evidence        browserauthentication.Evidence                  `json:"evidence" yaml:"evidence"`
	Confidence      string                                          `json:"confidence" yaml:"confidence"`
	ExpiresAfter    string                                          `json:"expiresAfter" yaml:"expiresAfter"`
	Verification    browserauthentication.Verification              `json:"verification" yaml:"verification"`
	CredentialSlots map[string]browserauthentication.CredentialSlot `json:"credentialSlots" yaml:"credentialSlots"`
	Flows           map[string]browserauthentication.Flow           `json:"flows" yaml:"flows"`
}

// Build inserts the fixed profile discriminator and applies every schema,
// origin, slot, secret, and PII gate.
func Build(spec Spec) (*authprofile.Profile, error) {
	value := &authprofile.Profile{
		Profile:         browserauthentication.ProfileName,
		Info:            spec.Info,
		ObservationKind: spec.ObservationKind,
		Evidence:        spec.Evidence,
		Confidence:      spec.Confidence,
		ExpiresAfter:    spec.ExpiresAfter,
		Verification:    spec.Verification,
		CredentialSlots: spec.CredentialSlots,
		Flows:           spec.Flows,
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("build authentication draft: %w", err)
	}
	result, err := authprofile.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("build authentication draft: %w", err)
	}
	return result, nil
}
