package profile

import (
	"encoding/json"
	"fmt"
)

// ValidateTyped validates one typed profile through the pinned UWS schema and
// Browsertools semantic checks.
func ValidateTyped(value *Profile) error {
	if value == nil {
		return fmt.Errorf("browser profile is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal typed browser profile: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode typed browser profile: %w", err)
	}
	return Validate(document)
}

// Clone returns a deep independent copy of a typed profile.
func Clone(value *Profile) (*Profile, error) {
	if value == nil {
		return nil, fmt.Errorf("browser profile is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal browser profile clone: %w", err)
	}
	var cloned Profile
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("decode browser profile clone: %w", err)
	}
	return &cloned, nil
}

// CloneValidated schema-validates value once and returns its independent typed
// snapshot for a top-level operation.
func CloneValidated(value *Profile) (*Profile, error) {
	if err := ValidateTyped(value); err != nil {
		return nil, err
	}
	return Clone(value)
}

// CloneAction returns a deep independent copy of an action.
func CloneAction(value Action) (Action, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Action{}, err
	}
	var cloned Action
	if err := json.Unmarshal(data, &cloned); err != nil {
		return Action{}, err
	}
	return cloned, nil
}

// CloneOutput returns a deep independent copy of an output declaration.
func CloneOutput(value Output) (Output, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Output{}, err
	}
	var cloned Output
	if err := json.Unmarshal(data, &cloned); err != nil {
		return Output{}, err
	}
	return cloned, nil
}

// HasWriteSideEffects reports whether an action declares anything other than
// the sole read_only side effect.
func HasWriteSideEffects(value Action) bool {
	return HasWriteSideEffectList(value.SideEffects)
}

// HasWriteSideEffectList reports whether a declaration is anything other than
// the sole read_only value.
func HasWriteSideEffectList(values []SideEffect) bool {
	if len(values) != 1 {
		return true
	}
	return values[0] != SideEffectReadOnly
}
