package authorsession

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/OpenUdon/evidence/redact"
)

// LabelReductionReason is the closed explanation for an accessibility-label
// reduction. Consumers can safely report the reason without retaining the raw
// page-derived label.
type LabelReductionReason string

const (
	LabelReasonUnchanged       LabelReductionReason = "unchanged"
	LabelReasonNormalized      LabelReductionReason = "normalized"
	LabelReasonTooLong         LabelReductionReason = "too_long"
	LabelReasonSensitive       LabelReductionReason = "sensitive"
	LabelReasonPromptInjection LabelReductionReason = "prompt_injection"

	RedactedLabel  = "[redacted]"
	UntrustedLabel = "[untrusted-label]"
)

// LabelReduction is the canonical protocol-safe label and its closed reason.
type LabelReduction struct {
	Value  string
	Reason LabelReductionReason
}

// ReduceAccessibilityLabel applies the canonical author-session label policy.
// The rejected raw label is intentionally absent from the result.
func ReduceAccessibilityLabel(raw string) LabelReduction {
	canonical := strings.Join(strings.FieldsFunc(raw, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
	if canonical == RedactedLabel || canonical == UntrustedLabel {
		return LabelReduction{Value: canonical, Reason: reasonForCanonicalization(raw, canonical)}
	}
	if len(canonical) > 256 {
		return LabelReduction{Value: RedactedLabel, Reason: LabelReasonTooLong}
	}
	lower := strings.ToLower(canonical)
	for _, phrase := range labelPromptInjectionPhrases {
		if strings.Contains(lower, phrase) {
			return LabelReduction{Value: UntrustedLabel, Reason: LabelReasonPromptInjection}
		}
	}
	if labelEmailPattern.MatchString(canonical) || labelPhonePattern.MatchString(canonical) || labelSecretPattern.MatchString(canonical) || redact.String(canonical) != canonical {
		return LabelReduction{Value: RedactedLabel, Reason: LabelReasonSensitive}
	}
	return LabelReduction{Value: canonical, Reason: reasonForCanonicalization(raw, canonical)}
}

func reasonForCanonicalization(raw, canonical string) LabelReductionReason {
	if raw != canonical {
		return LabelReasonNormalized
	}
	return LabelReasonUnchanged
}

var (
	labelPromptInjectionPhrases = []string{
		"ignore previous",
		"ignore prior",
		"ignore all instructions",
		"system prompt",
		"developer message",
		"tool call",
		"reveal secrets",
		"reveal credentials",
	}
	labelEmailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	labelPhonePattern  = regexp.MustCompile(`(?:^|[^A-Za-z0-9])\+?[0-9][0-9 ()-]{8,}[0-9](?:$|[^A-Za-z0-9])`)
	labelSecretPattern = regexp.MustCompile(`(?i)(?:bearer\s+[a-z0-9._-]{12,}|(?:token|secret|password)\s*[:=]\s*\S+|sk-[a-z0-9]{12,})`)
)
