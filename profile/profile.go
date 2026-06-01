// Package profile loads and validates UWS browser-profile documents against the
// uws.browser.1.5 wire schema.
//
// The schema itself is owned by github.com/OpenUdon/uws
// (versions/browser.1.5.json). browsertools embeds a synchronized copy so that
// validation works offline, and a parity test guards against drift. The typed
// model here is intentionally minimal for milestone M02: schema validation does
// the heavy lifting, and richer typed actions/outputs land with the draft
// builder in M04.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile is a minimal typed view of a uws.browser.1.5 document. It captures the
// required top-level fields for convenient access; action internals are left
// loosely typed until the draft builder needs them.
type Profile struct {
	// Schema holds the profile identifier, always "uws.browser.1.5".
	// The JSON/YAML key is "profile" (matching the wire format).
	Schema          string            `json:"profile" yaml:"profile"`
	Info            Info              `json:"info" yaml:"info"`
	ObservationKind string            `json:"observationKind" yaml:"observationKind"`
	Evidence        Evidence          `json:"evidence" yaml:"evidence"`
	Confidence      string            `json:"confidence" yaml:"confidence"`
	ExpiresAfter    string            `json:"expiresAfter" yaml:"expiresAfter"`
	Verification    Verification      `json:"verification" yaml:"verification"`
	Actions         map[string]Action `json:"actions" yaml:"actions"`
}

// Info is the profile-info block. Origin may be a single string or a list of
// strings, so it stays loosely typed here.
type Info struct {
	Title              string `json:"title" yaml:"title"`
	Provider           string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Origin             any    `json:"origin" yaml:"origin"`
	LoginStateRequired bool   `json:"loginStateRequired,omitempty" yaml:"loginStateRequired,omitempty"`
}

// Evidence records how and when the profile was learned from UI observation.
//
// LearnedAt is the timestamp when the evidence collection session concluded and
// the profile was generated. It is a profile-level timestamp recorded once.
//
// Contrast with evidence.Record.ObservedAt, which is a per-observation timestamp
// recorded by each adapter at the moment a specific UI state was captured. The
// two fields serve different clocks: LearnedAt marks the profile generation event;
// ObservedAt marks individual UI observations that fed into the draft.
type Evidence struct {
	LearnedAt string `json:"learnedAt" yaml:"learnedAt"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
}

// Verification records the most recent successful validation of the profile.
type Verification struct {
	LastVerifiedAt  string   `json:"lastVerifiedAt" yaml:"lastVerifiedAt"`
	SuccessfulRuns  int      `json:"successfulRuns" yaml:"successfulRuns"`
	UIStabilityScore *float64 `json:"uiStabilityScore,omitempty" yaml:"uiStabilityScore,omitempty"`
}

// Action is a minimal view of a named UI capability. Sequence steps, parameters,
// and outputs are validated by the JSON Schema and left untyped here for now.
type Action struct {
	Description        string             `json:"description,omitempty" yaml:"description,omitempty"`
	SideEffects        []string           `json:"sideEffects" yaml:"sideEffects"`
	ConfirmationPolicy ConfirmationPolicy `json:"confirmationPolicy" yaml:"confirmationPolicy"`
}

// ConfirmationPolicy controls whether a runtime must confirm before executing an
// action with side effects.
type ConfirmationPolicy struct {
	Required bool   `json:"required" yaml:"required"`
	Prompt   string `json:"prompt,omitempty" yaml:"prompt,omitempty"`
}

// LoadProfileFile decodes a browser-profile document from JSON or YAML based on
// its file extension. It does not validate; call ValidateFile for that.
func LoadProfileFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse JSON profile %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse YAML profile %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported profile extension %q for %s", filepath.Ext(path), path)
	}
	return &p, nil
}
