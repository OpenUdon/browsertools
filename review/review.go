// Package review builds deterministic promotion bundles that bind a typed
// browser profile to its normalized evidence and explicit review decisions.
package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/revalidate"
)

// Gap is a promotion-blocking issue.
type Gap struct {
	Kind    string `json:"kind"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// EvidenceSummary is a secret-free summary of the supporting observations.
type EvidenceSummary struct {
	Origins          []string `json:"origins"`
	ObservationKinds []string `json:"observationKinds"`
	Tools            []string `json:"tools"`
	RecordCount      int      `json:"recordCount"`
	EarliestAt       string   `json:"earliestAt,omitempty"`
	LatestAt         string   `json:"latestAt,omitempty"`
}

// OriginSummary describes the normalized profile origin allowlist.
type OriginSummary struct {
	Origins []string `json:"origins"`
}

// SideEffectSummary describes write actions and confirmation requirements.
type SideEffectSummary struct {
	HasWriteActions              bool                `json:"hasWriteActions"`
	ActionsRequiringConfirmation []string            `json:"actionsRequiringConfirmation"`
	ActionsWithSideEffects       map[string][]string `json:"actionsWithSideEffects"`
}

// ValidationReport records typed profile validation.
type ValidationReport struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// Bundle is the complete, digest-bound review artifact.
type Bundle struct {
	Profile             profile.Profile            `json:"profile"`
	ProfileDigest       string                     `json:"profileDigest"`
	EvidenceDigest      string                     `json:"evidenceDigest"`
	AssessedAt          string                     `json:"assessedAt"`
	Validation          ValidationReport           `json:"validation"`
	Revalidation        revalidate.Result          `json:"revalidation"`
	Evidence            EvidenceSummary            `json:"evidence"`
	Decisions           []evidence.LocatorDecision `json:"decisions"`
	Gaps                []Gap                      `json:"gaps"`
	ConfidenceRationale string                     `json:"confidenceRationale"`
	ExpiryNote          string                     `json:"expiryNote"`
	Origins             OriginSummary              `json:"origins"`
	SideEffects         SideEffectSummary          `json:"sideEffects"`
}

// Build assesses prof at now and binds the resulting bundle to the exact
// profile and evidence bytes through canonical SHA-256 digests.
func Build(prof *profile.Profile, records []evidence.Record, decisions []evidence.LocatorDecision, now time.Time) (*Bundle, error) {
	if prof == nil {
		return nil, fmt.Errorf("review: profile is required")
	}
	if now.IsZero() {
		return nil, fmt.Errorf("review: assessment time is required")
	}
	profileDigest, err := digestProfile(prof)
	if err != nil {
		return nil, fmt.Errorf("review: profile digest: %w", err)
	}
	evidenceDigest, err := digestEvidence(records)
	if err != nil {
		return nil, fmt.Errorf("review: evidence digest: %w", err)
	}

	snapshot, err := cloneProfile(prof)
	if err != nil {
		return nil, fmt.Errorf("review: profile snapshot: %w", err)
	}
	bundle := &Bundle{
		Profile: *snapshot, ProfileDigest: profileDigest, EvidenceDigest: evidenceDigest,
		AssessedAt:          now.UTC().Format(time.RFC3339),
		Evidence:            summarizeEvidence(records),
		Decisions:           append([]evidence.LocatorDecision(nil), decisions...),
		Origins:             OriginSummary{Origins: append([]string(nil), prof.Info.Origin...)},
		SideEffects:         summarizeSideEffects(prof),
		ConfidenceRationale: fmt.Sprintf("confidence=%s; evidence_records=%d", prof.Confidence, len(records)),
		ExpiryNote:          fmt.Sprintf("profile expires %s after verification.lastVerifiedAt", prof.ExpiresAfter),
	}
	sort.Slice(bundle.Decisions, func(i, j int) bool {
		if bundle.Decisions[i].ActionHint != bundle.Decisions[j].ActionHint {
			return bundle.Decisions[i].ActionHint < bundle.Decisions[j].ActionHint
		}
		if bundle.Decisions[i].Locator.Role != bundle.Decisions[j].Locator.Role {
			return bundle.Decisions[i].Locator.Role < bundle.Decisions[j].Locator.Role
		}
		return bundle.Decisions[i].Locator.Name < bundle.Decisions[j].Locator.Name
	})
	if bundle.Decisions == nil {
		bundle.Decisions = []evidence.LocatorDecision{}
	}

	value, valueErr := prof.Value()
	if valueErr != nil {
		bundle.Validation = ValidationReport{Valid: false, Errors: []string{valueErr.Error()}}
	} else if validationErr := profile.Validate(value); validationErr != nil {
		bundle.Validation = ValidationReport{Valid: false, Errors: []string{validationErr.Error()}}
	} else {
		bundle.Validation = ValidationReport{Valid: true, Errors: []string{}}
	}

	bundle.Revalidation, err = revalidate.CheckAt(prof, records, decisions, now)
	if err != nil {
		return nil, err
	}
	for _, failure := range bundle.Revalidation.Failures {
		bundle.Gaps = append(bundle.Gaps, Gap{Kind: string(failure.Kind), Field: failure.Field, Message: failure.Message})
	}
	if !bundle.Validation.Valid && !hasGap(bundle.Gaps, string(revalidate.CheckInvalidProfile), "$", bundle.Validation.Errors[0]) {
		bundle.Gaps = append(bundle.Gaps, Gap{Kind: string(revalidate.CheckInvalidProfile), Field: "$", Message: bundle.Validation.Errors[0]})
	}
	sort.Slice(bundle.Gaps, func(i, j int) bool {
		if bundle.Gaps[i].Kind != bundle.Gaps[j].Kind {
			return bundle.Gaps[i].Kind < bundle.Gaps[j].Kind
		}
		if bundle.Gaps[i].Field != bundle.Gaps[j].Field {
			return bundle.Gaps[i].Field < bundle.Gaps[j].Field
		}
		return bundle.Gaps[i].Message < bundle.Gaps[j].Message
	})
	if bundle.Gaps == nil {
		bundle.Gaps = []Gap{}
	}
	return bundle, nil
}

func cloneProfile(prof *profile.Profile) (*profile.Profile, error) {
	data, err := json.Marshal(prof)
	if err != nil {
		return nil, err
	}
	var result profile.Profile
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Promotable reports whether the bundle passed every gate at AssessedAt.
func (b Bundle) Promotable() bool {
	return b.Validation.Valid && b.Revalidation.OK && len(b.Gaps) == 0 && b.ProfileDigest != "" && b.EvidenceDigest != ""
}

// Verify proves that bundle still matches prof and records and that the same
// fixture safety checks pass at now.
func Verify(bundle *Bundle, prof *profile.Profile, records []evidence.Record, now time.Time) error {
	if bundle == nil || prof == nil {
		return fmt.Errorf("review: bundle and profile are required")
	}
	if now.IsZero() {
		return fmt.Errorf("review: verification time is required")
	}
	profileDigest, err := digestProfile(prof)
	if err != nil {
		return err
	}
	if profileDigest != bundle.ProfileDigest {
		return fmt.Errorf("review: profile digest mismatch")
	}
	embeddedDigest, err := digestProfile(&bundle.Profile)
	if err != nil {
		return err
	}
	if embeddedDigest != bundle.ProfileDigest {
		return fmt.Errorf("review: embedded bundle profile digest mismatch")
	}
	evidenceDigest, err := digestEvidence(records)
	if err != nil {
		return err
	}
	if evidenceDigest != bundle.EvidenceDigest {
		return fmt.Errorf("review: evidence digest mismatch")
	}
	result, err := revalidate.CheckAt(prof, records, bundle.Decisions, now)
	if err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("review: bundle is no longer promotable: %s at %s", result.Failures[0].Kind, result.Failures[0].Field)
	}
	if !bundle.Promotable() {
		return fmt.Errorf("review: bundle did not pass its original promotion gate")
	}
	return nil
}

func digestProfile(prof *profile.Profile) (string, error) {
	data, err := json.Marshal(prof)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func digestEvidence(records []evidence.Record) (string, error) {
	encoded := make([][]byte, 0, len(records))
	for _, record := range records {
		data, err := evidence.MarshalDeterministic(record)
		if err != nil {
			return "", err
		}
		encoded = append(encoded, data)
	}
	sort.Slice(encoded, func(i, j int) bool { return bytes.Compare(encoded[i], encoded[j]) < 0 })
	h := sha256.New()
	h.Write([]byte("["))
	for i, data := range encoded {
		if i > 0 {
			h.Write([]byte(","))
		}
		h.Write(data)
	}
	h.Write([]byte("]"))
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func summarizeEvidence(records []evidence.Record) EvidenceSummary {
	origins, kinds, tools := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var earliest, latest string
	for _, record := range records {
		origins[record.Origin] = true
		if record.ObservationKind != "" {
			kinds[string(record.ObservationKind)] = true
		}
		if record.Provenance.Tool != "" {
			tools[record.Provenance.Tool] = true
		}
		if earliest == "" || record.ObservedAt < earliest {
			earliest = record.ObservedAt
		}
		if latest == "" || record.ObservedAt > latest {
			latest = record.ObservedAt
		}
	}
	return EvidenceSummary{
		Origins: sortedSet(origins), ObservationKinds: sortedSet(kinds), Tools: sortedSet(tools),
		RecordCount: len(records), EarliestAt: earliest, LatestAt: latest,
	}
}

func summarizeSideEffects(prof *profile.Profile) SideEffectSummary {
	result := SideEffectSummary{ActionsWithSideEffects: map[string][]string{}}
	for _, name := range prof.SortedActionNames() {
		action := prof.Actions[name]
		for _, effect := range action.SideEffects {
			result.ActionsWithSideEffects[name] = append(result.ActionsWithSideEffects[name], string(effect))
			if effect != profile.SideEffectReadOnly {
				result.HasWriteActions = true
			}
		}
		sort.Strings(result.ActionsWithSideEffects[name])
		if action.ConfirmationPolicy.Required {
			result.ActionsRequiringConfirmation = append(result.ActionsRequiringConfirmation, name)
		}
	}
	if result.ActionsRequiringConfirmation == nil {
		result.ActionsRequiringConfirmation = []string{}
	}
	return result
}

func sortedSet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func hasGap(gaps []Gap, kind, field, message string) bool {
	for _, gap := range gaps {
		if gap.Kind == kind && gap.Field == field && strings.TrimSpace(gap.Message) == strings.TrimSpace(message) {
			return true
		}
	}
	return false
}
