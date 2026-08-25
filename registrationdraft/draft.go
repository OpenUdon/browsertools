// Package registrationdraft deterministically builds validated browser
// registration profiles from explicit author specifications.
package registrationdraft

import (
	"encoding/json"
	"fmt"

	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
)

// Spec is an explicit registration profile with the discriminator omitted.
// No locator, credential slot, effect, checkpoint, submission, or success
// condition is inferred from observation evidence.
type Spec struct {
	Info            browserregistration.Info                      `json:"info" yaml:"info"`
	ObservationKind string                                        `json:"observationKind" yaml:"observationKind"`
	Evidence        browserregistration.Evidence                  `json:"evidence" yaml:"evidence"`
	Confidence      string                                        `json:"confidence" yaml:"confidence"`
	ExpiresAfter    string                                        `json:"expiresAfter" yaml:"expiresAfter"`
	Verification    browserregistration.Verification              `json:"verification" yaml:"verification"`
	CredentialSlots map[string]browserregistration.CredentialSlot `json:"credentialSlots" yaml:"credentialSlots"`
	Flows           map[string]browserregistration.Flow           `json:"flows" yaml:"flows"`
}

// Build inserts the fixed profile discriminator and applies every UWS schema,
// origin, slot, mutation, secret, and PII gate.
func Build(spec Spec) (*registrationprofile.Profile, error) {
	value := &registrationprofile.Profile{
		Profile:         browserregistration.ProfileName,
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
		return nil, fmt.Errorf("build registration draft: %w", err)
	}
	result, err := registrationprofile.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("build registration draft: %w", err)
	}
	return result, nil
}
