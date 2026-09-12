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
	Contexts        map[string]browserauthentication.Context        `json:"contexts,omitempty" yaml:"contexts,omitempty"`
	CredentialSlots map[string]browserauthentication.CredentialSlot `json:"credentialSlots" yaml:"credentialSlots"`
	Flows           map[string]browserauthentication.Flow           `json:"flows" yaml:"flows"`
}

// Build inserts the oldest sufficient profile discriminator and applies every
// schema, origin, slot, secret, and PII gate. A specification that uses no
// context feature keeps the authentication 1.0 discriminator and its exact
// bytes.
func Build(spec Spec) (*authprofile.Profile, error) {
	profileName := browserauthentication.ProfileName
	if requiresContextProfile(spec) {
		profileName = browserauthentication.ContextProfileName
	}
	value := &authprofile.Profile{
		Profile:         profileName,
		Info:            spec.Info,
		ObservationKind: spec.ObservationKind,
		Evidence:        spec.Evidence,
		Confidence:      spec.Confidence,
		ExpiresAfter:    spec.ExpiresAfter,
		Verification:    spec.Verification,
		Contexts:        spec.Contexts,
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

// requiresContextProfile reports whether a specification uses a feature that
// authentication 1.1 introduced: declared contexts, a context-qualified step,
// an opened context, the navigate object form, or a success path. Everything
// else remains expressible in 1.0.
func requiresContextProfile(spec Spec) bool {
	if len(spec.Contexts) != 0 {
		return true
	}
	for _, flow := range spec.Flows {
		if flow.Success.Context != "" || flow.Success.Path != "" {
			return true
		}
		for _, step := range flow.Sequence {
			if step.NavigateTarget != nil {
				return true
			}
			if step.TypeCredential != nil && step.TypeCredential.Context != "" {
				return true
			}
			if step.Click != nil && (step.Click.Context != "" || step.Click.OpensContext != "") {
				return true
			}
			if step.Challenge != nil && step.Challenge.Context != "" {
				return true
			}
			if step.WaitFor != nil && step.WaitFor.Context != "" {
				return true
			}
		}
	}
	return false
}
