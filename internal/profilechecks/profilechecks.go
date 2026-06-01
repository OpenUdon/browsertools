// Package profilechecks contains shared semantic checks for raw
// browser-profile maps used by review and revalidation gates.
package profilechecks

import (
	"fmt"
	"sort"
)

// Rule identifies the semantic check that produced an issue.
type Rule string

const (
	// RuleInvalidOutputShape means an output is missing source-specific fields
	// or has an unsupported type/source.
	RuleInvalidOutputShape Rule = "invalid_output_shape"
	// RuleCSSMissingFallback means a CSS output is missing fallbackReason.
	RuleCSSMissingFallback Rule = "css_missing_fallback_reason"
	// RuleSideEffectNoConfirm means a write action is missing required
	// confirmation policy.
	RuleSideEffectNoConfirm Rule = "side_effect_no_confirmation"
	// RuleSideEffectNoSafeWait means a write action has no post-action wait.
	RuleSideEffectNoSafeWait Rule = "side_effect_no_safe_wait"
)

// Issue is one profile semantic check failure.
type Issue struct {
	Rule    Rule
	Field   string
	Message string
}

// CheckOutputs verifies each action output has the source-specific fields
// needed by review and dry-run revalidation.
func CheckOutputs(prof map[string]any) []Issue {
	actions, _ := prof["actions"].(map[string]any)
	var issues []Issue
	for _, actionName := range sortedMapKeys(actions) {
		action, _ := actions[actionName].(map[string]any)
		if action == nil {
			continue
		}
		outputs, _ := action["outputs"].(map[string]any)
		for _, outName := range sortedMapKeys(outputs) {
			field := fmt.Sprintf("actions.%s.outputs.%s", actionName, outName)
			out, _ := outputs[outName].(map[string]any)
			if out == nil {
				issues = append(issues, invalidOutput(field, "output must be an object"))
				continue
			}
			typ, _ := out["type"].(string)
			if !validOutputTypes[typ] {
				issues = append(issues, invalidOutput(field+".type", fmt.Sprintf("output %q has invalid or empty type %q", outName, typ)))
			}
			src, _ := out["source"].(string)
			if !validOutputSources[src] {
				issues = append(issues, invalidOutput(field+".source", fmt.Sprintf("output %q has invalid or empty source %q", outName, src)))
				continue
			}
			switch src {
			case "a11y":
				loc, _ := out["locator"].(map[string]any)
				role, _ := loc["role"].(string)
				if loc == nil || role == "" {
					issues = append(issues, invalidOutput(field+".locator", fmt.Sprintf("output %q uses a11y source but has no locator role", outName)))
				}
			case "jsonld", "microdata":
				prop, _ := out["property"].(string)
				if prop == "" {
					issues = append(issues, invalidOutput(field+".property", fmt.Sprintf("output %q uses %s source but has no property", outName, src)))
				}
			case "css":
				selector, _ := out["selector"].(string)
				if selector == "" {
					issues = append(issues, invalidOutput(field+".selector", fmt.Sprintf("output %q uses css source but has no selector", outName)))
				}
				if validation, _ := out["validation"].(map[string]any); validation == nil {
					issues = append(issues, invalidOutput(field+".validation", fmt.Sprintf("output %q uses css source but has no validation schema", outName)))
				}
			}
		}
	}
	return issues
}

// CheckCSSFallbacks verifies every css-source output carries a fallbackReason.
func CheckCSSFallbacks(prof map[string]any) []Issue {
	actions, _ := prof["actions"].(map[string]any)
	var issues []Issue
	for _, name := range sortedMapKeys(actions) {
		action, _ := actions[name].(map[string]any)
		if action == nil {
			continue
		}
		outputs, _ := action["outputs"].(map[string]any)
		for _, outName := range sortedMapKeys(outputs) {
			out, _ := outputs[outName].(map[string]any)
			if out == nil {
				continue
			}
			if src, _ := out["source"].(string); src == "css" {
				if reason, _ := out["fallbackReason"].(string); reason == "" {
					issues = append(issues, Issue{
						Rule:    RuleCSSMissingFallback,
						Field:   fmt.Sprintf("actions.%s.outputs.%s.fallbackReason", name, outName),
						Message: fmt.Sprintf("output %q uses css source but has no fallbackReason", outName),
					})
				}
			}
		}
	}
	return issues
}

// CheckSideEffectConfirmation verifies non-read_only actions require explicit
// confirmation.
func CheckSideEffectConfirmation(prof map[string]any) []Issue {
	actions, _ := prof["actions"].(map[string]any)
	var issues []Issue
	for _, name := range sortedMapKeys(actions) {
		action, _ := actions[name].(map[string]any)
		if action == nil || !hasWriteSideEffect(action) {
			continue
		}
		policy, _ := action["confirmationPolicy"].(map[string]any)
		if policy == nil {
			issues = append(issues, Issue{
				Rule:    RuleSideEffectNoConfirm,
				Field:   fmt.Sprintf("actions.%s.confirmationPolicy", name),
				Message: fmt.Sprintf("action %q has write side effects but no confirmationPolicy", name),
			})
		} else if req, _ := policy["required"].(bool); !req {
			issues = append(issues, Issue{
				Rule:    RuleSideEffectNoConfirm,
				Field:   fmt.Sprintf("actions.%s.confirmationPolicy.required", name),
				Message: fmt.Sprintf("action %q has write side effects but confirmationPolicy.required=false", name),
			})
		}
	}
	return issues
}

// CheckSideEffectSafeWait verifies non-read_only actions have an explicit wait
// after the side-effectful interaction.
func CheckSideEffectSafeWait(prof map[string]any) []Issue {
	actions, _ := prof["actions"].(map[string]any)
	var issues []Issue
	for _, name := range sortedMapKeys(actions) {
		action, _ := actions[name].(map[string]any)
		if action == nil || !hasWriteSideEffect(action) || hasSafeWait(action) {
			continue
		}
		issues = append(issues, Issue{
			Rule:    RuleSideEffectNoSafeWait,
			Field:   fmt.Sprintf("actions.%s.sequence", name),
			Message: fmt.Sprintf("action %q has write side effects but no post-action wait_for condition", name),
		})
	}
	return issues
}

func invalidOutput(field, message string) Issue {
	return Issue{
		Rule:    RuleInvalidOutputShape,
		Field:   field,
		Message: message,
	}
}

func hasWriteSideEffect(action map[string]any) bool {
	switch v := action["sideEffects"].(type) {
	case []any:
		for _, e := range v {
			if s, _ := e.(string); s != "" && s != "read_only" {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s != "read_only" {
				return true
			}
		}
	}
	return false
}

func hasSafeWait(action map[string]any) bool {
	seq, _ := action["sequence"].([]any)
	for _, rawStep := range seq {
		step, _ := rawStep.(map[string]any)
		if step == nil {
			continue
		}
		if _, ok := step["wait_for"]; ok {
			return true
		}
		click, _ := step["click"].(map[string]any)
		if click != nil {
			if _, ok := click["wait_for"]; ok {
				return true
			}
		}
	}
	return false
}

var validOutputSources = map[string]bool{
	"a11y":      true,
	"jsonld":    true,
	"microdata": true,
	"css":       true,
}

var validOutputTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
	"array":   true,
	"object":  true,
	"null":    true,
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
