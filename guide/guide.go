// Package guide turns explicit operator answers and reviewed normalized
// evidence into one deterministic, strictly gated authoring bundle.
//
// Guide is not a browser command language. It accepts only the closed
// browser.1.5 macro vocabulary and never infers a sequence, mutation, side
// effect, confirmation policy, or ambiguity decision from evidence.
package guide

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/internal/secretwalk"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
	"github.com/OpenUdon/evidence/redact"
)

const (
	// BundleVersion identifies the deterministic guided-authoring envelope.
	BundleVersion        = "browsertools.guided-authoring.v1"
	maxRecords           = 256
	maxActions           = 64
	maxParameters        = 128
	maxSteps             = 128
	maxCatalogLocators   = 8192
	maxCatalogOutputs    = 4096
	maxEvidencePerAction = 64
	maxGeneratedEvidence = 2048
	maxMatchingWork      = 2_000_000
	maxFreeTextBytes     = 4096
	maxCatalogBytes      = 16 << 20
	maxRecordBytes       = 2 << 20
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var templatePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_-]{1,128})\}\}`)

// Catalog is a canonical, stable-ID view of reviewed normalized evidence.
// IDs are local authoring references and never enter a portable profile.
type Catalog struct {
	Records  []RecordCandidate  `json:"records"`
	Origins  []OriginCandidate  `json:"origins"`
	Locators []LocatorCandidate `json:"locators"`
	Outputs  []OutputCandidate  `json:"outputs"`

	recordByID  map[string]evidence.Record
	locatorByID map[string]LocatorCandidate
	outputByID  map[string]OutputCandidate
}

// RecordCandidate identifies one normalized observation.
type RecordCandidate struct {
	ID               string `json:"id"`
	Origin           string `json:"origin"`
	ObservedAt       string `json:"observedAt"`
	Tool             string `json:"tool"`
	SourceActionHint string `json:"sourceActionHint,omitempty"`
}

// OriginCandidate identifies one canonical observed origin.
type OriginCandidate struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

// LocatorCandidate identifies one accessibility candidate within a record.
type LocatorCandidate struct {
	ID         string                    `json:"id"`
	RecordID   string                    `json:"recordId"`
	Locator    evidence.CandidateLocator `json:"locator"`
	FromOutput bool                      `json:"fromOutput,omitempty"`
}

// OutputCandidate identifies one output candidate within a record.
type OutputCandidate struct {
	ID        string                   `json:"id"`
	RecordID  string                   `json:"recordId"`
	Output    evidence.CandidateOutput `json:"output"`
	LocatorID string                   `json:"locatorId,omitempty"`
	Bound     bool                     `json:"bound"`
}

// Intent is the complete set of explicit answers accepted from an operator.
type Intent struct {
	Info            profile.Info            `json:"info"`
	ObservationKind profile.ObservationKind `json:"observationKind"`
	Confidence      profile.Confidence      `json:"confidence"`
	ExpiresAfter    profile.Duration        `json:"expiresAfter"`
	Actions         []ActionIntent          `json:"actions"`
}

// ActionIntent maps selected evidence to one explicitly named capability.
type ActionIntent struct {
	ID                   string                     `json:"id"`
	Description          string                     `json:"description,omitempty"`
	EvidenceIDs          []string                   `json:"evidenceIds"`
	Parameters           []ParameterIntent          `json:"parameters"`
	Sequence             []StepIntent               `json:"sequence"`
	OutputIDs            []string                   `json:"outputIds"`
	OutputDeclarations   []OutputDeclaration        `json:"outputDeclarations,omitempty"`
	SideEffects          []profile.SideEffect       `json:"sideEffects"`
	ConfirmationPolicy   profile.ConfirmationPolicy `json:"confirmationPolicy"`
	AmbiguityResolutions []AmbiguityResolution      `json:"ambiguityResolutions"`
}

// OutputDeclaration binds one unbound extraction hint to an explicit portable
// source. The hint supplies only key/type; all source semantics are authored.
type OutputDeclaration struct {
	HintID         string                 `json:"hintId"`
	Source         profile.OutputSource   `json:"source"`
	LocatorID      string                 `json:"locatorId,omitempty"`
	Property       string                 `json:"property,omitempty"`
	Selector       string                 `json:"selector,omitempty"`
	FallbackReason profile.FallbackReason `json:"fallbackReason,omitempty"`
}

// ParameterIntent is the deliberately restricted scalar parameter shape used
// by the wizard. Richer schemas remain available through draft.Spec.
type ParameterIntent struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// StepIntent is one explicit member of the closed browser.1.5 macro set.
type StepIntent struct {
	Kind           profile.StepKind `json:"kind"`
	Navigate       string           `json:"navigate,omitempty"`
	LocatorID      string           `json:"locatorId,omitempty"`
	ValueParameter string           `json:"valueParameter,omitempty"`
	Wait           *WaitIntent      `json:"wait,omitempty"`
}

// WaitIntent is a locator or navigation wait, never an arbitrary predicate.
type WaitIntent struct {
	LocatorID  string                  `json:"locatorId,omitempty"`
	Navigation *profile.NavigationWait `json:"navigation,omitempty"`
}

// AmbiguityResolution records the operator's rationale for one selected
// ambiguous accessibility candidate.
type AmbiguityResolution struct {
	LocatorID string `json:"locatorId"`
	Rationale string `json:"rationale"`
}

// Bundle contains every artifact needed by the existing strict handoff gates.
type Bundle struct {
	Version   string                     `json:"version"`
	Spec      draft.Spec                 `json:"spec"`
	Profile   profile.Profile            `json:"profile"`
	Evidence  []evidence.Record          `json:"evidence"`
	Decisions []evidence.LocatorDecision `json:"decisions"`
	Review    review.Bundle              `json:"review"`
}

// NewCatalog validates, canonicalizes, and assigns deterministic local IDs to
// normalized evidence. The input is never mutated.
func NewCatalog(records []evidence.Record) (*Catalog, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("guide: at least one normalized evidence record is required")
	}
	if len(records) > maxRecords {
		return nil, fmt.Errorf("guide: evidence exceeds %d records", maxRecords)
	}
	canonical := make([]evidence.Record, 0, len(records))
	totalBytes := 0
	for index, record := range records {
		raw := evidence.RawRecord{Record: record}
		normalized, err := raw.Normalize()
		if err != nil {
			return nil, fmt.Errorf("guide: evidence[%d]: %w", index, err)
		}
		if err := rejectSecretLike(normalized); err != nil {
			return nil, fmt.Errorf("guide: evidence[%d]: %w", index, err)
		}
		encoded, err := evidence.MarshalDeterministic(normalized)
		if err != nil {
			return nil, fmt.Errorf("guide: evidence[%d]: %w", index, err)
		}
		if len(encoded) > maxRecordBytes {
			return nil, fmt.Errorf("guide: evidence[%d] exceeds %d bytes", index, maxRecordBytes)
		}
		totalBytes += len(encoded)
		if totalBytes > maxCatalogBytes {
			return nil, fmt.Errorf("guide: normalized evidence exceeds %d bytes", maxCatalogBytes)
		}
		canonical = append(canonical, normalized)
	}
	if err := sortCanonical(canonical); err != nil {
		return nil, fmt.Errorf("guide: sort evidence: %w", err)
	}
	catalog := &Catalog{
		recordByID:  make(map[string]evidence.Record, len(canonical)),
		locatorByID: map[string]LocatorCandidate{}, outputByID: map[string]OutputCandidate{},
	}
	originSet := map[string]struct{}{}
	for recordIndex, record := range canonical {
		recordID := fmt.Sprintf("E%03d", recordIndex+1)
		catalog.Records = append(catalog.Records, RecordCandidate{
			ID: recordID, Origin: record.Origin, ObservedAt: record.ObservedAt,
			Tool: record.Provenance.Tool, SourceActionHint: record.ActionHint,
		})
		clonedRecord, err := cloneRecord(record)
		if err != nil {
			return nil, fmt.Errorf("guide: clone evidence record %q: %w", recordID, err)
		}
		catalog.recordByID[recordID] = clonedRecord
		originSet[record.Origin] = struct{}{}

		locators := append([]evidence.CandidateLocator(nil), record.CandidateLocators...)
		for _, output := range record.CandidateOutputs {
			if output.Locator != nil && !containsLocator(locators, *output.Locator) {
				locators = append(locators, *output.Locator)
			}
		}
		if err := sortCanonical(locators); err != nil {
			return nil, fmt.Errorf("guide: sort locators for %q: %w", recordID, err)
		}
		if len(catalog.Locators)+len(locators) > maxCatalogLocators {
			return nil, fmt.Errorf("guide: evidence exceeds %d locator candidates", maxCatalogLocators)
		}
		locatorIDs := map[string]string{}
		for locatorIndex, locator := range locators {
			id := fmt.Sprintf("%s.L%03d", recordID, locatorIndex+1)
			candidate := LocatorCandidate{
				ID: id, RecordID: recordID, Locator: locator,
				FromOutput: !containsLocator(record.CandidateLocators, locator),
			}
			catalog.Locators = append(catalog.Locators, candidate)
			catalog.locatorByID[id] = candidate
			locatorIDs[locatorIdentity(locator)] = id
		}
		outputs := append([]evidence.CandidateOutput(nil), record.CandidateOutputs...)
		if err := sortCanonical(outputs); err != nil {
			return nil, fmt.Errorf("guide: sort outputs for %q: %w", recordID, err)
		}
		if len(catalog.Outputs)+len(outputs) > maxCatalogOutputs {
			return nil, fmt.Errorf("guide: evidence exceeds %d output candidates", maxCatalogOutputs)
		}
		for outputIndex, output := range outputs {
			if redact.SensitiveKey(strings.ToLower(output.Key)) ||
				(output.Property != "" && redact.SensitiveKey(strings.ToLower(output.Property))) {
				return nil, fmt.Errorf("guide: evidence record %q contains a credential-shaped output candidate", recordID)
			}
			if output.Source == string(profile.OutputCSS) && !portableCSS(output.Selector) {
				return nil, fmt.Errorf("guide: evidence record %q contains a non-portable CSS output candidate", recordID)
			}
			id := fmt.Sprintf("%s.O%03d", recordID, outputIndex+1)
			candidate := OutputCandidate{ID: id, RecordID: recordID, Output: output, Bound: output.Source != ""}
			if output.Locator != nil {
				candidate.LocatorID = locatorIDs[locatorIdentity(*output.Locator)]
			}
			catalog.Outputs = append(catalog.Outputs, candidate)
			catalog.outputByID[id] = candidate
		}
	}
	origins := make([]string, 0, len(originSet))
	for origin := range originSet {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	for index, origin := range origins {
		catalog.Origins = append(catalog.Origins, OriginCandidate{ID: fmt.Sprintf("O%03d", index+1), Origin: origin})
	}
	return catalog, nil
}

// Author materializes and verifies every guided artifact. It returns only a
// bundle that passes draft, schema, fixture revalidation, review, expiry, and
// digest verification at assessedAt.
func Author(catalog *Catalog, intent Intent, assessedAt time.Time) (*Bundle, error) {
	if catalog == nil {
		return nil, fmt.Errorf("guide: catalog is required")
	}
	if assessedAt.IsZero() {
		return nil, fmt.Errorf("guide: assessment time is required")
	}
	assessedAt = assessedAt.UTC()
	intent, err := cloneIntent(intent)
	if err != nil {
		return nil, err
	}
	if err := validateIntent(catalog, &intent); err != nil {
		return nil, err
	}

	actions := append([]ActionIntent(nil), intent.Actions...)
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	spec := draft.Spec{
		Info: intent.Info, ObservationKind: intent.ObservationKind,
		Confidence: intent.Confidence, ExpiresAfter: intent.ExpiresAfter,
		Actions: map[string]draft.ActionSpec{},
	}
	var selectedEvidence []evidence.Record
	var decisions []evidence.LocatorDecision
	matchingWork := 0
	for _, action := range actions {
		records, actionSpec, actionDecisions, err := buildAction(catalog, action, intent.Info.Origin)
		if err != nil {
			return nil, fmt.Errorf("guide: action %q: %w", action.ID, err)
		}
		selectedEvidence = append(selectedEvidence, records...)
		if len(selectedEvidence) > maxGeneratedEvidence {
			return nil, fmt.Errorf("guide: generated evidence exceeds %d action-bound records", maxGeneratedEvidence)
		}
		candidates := 0
		for _, record := range records {
			candidates += len(record.CandidateLocators) + len(record.CandidateOutputs)
		}
		matchingWork += candidates * (len(action.Sequence) + len(action.OutputIDs) + len(action.OutputDeclarations) + 1)
		if matchingWork > maxMatchingWork {
			return nil, fmt.Errorf("guide: selected evidence and declarations exceed bounded matching work")
		}
		decisions = append(decisions, actionDecisions...)
		spec.Actions[action.ID] = actionSpec
	}
	evidence.Sort(selectedEvidence)
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].ActionHint != decisions[j].ActionHint {
			return decisions[i].ActionHint < decisions[j].ActionHint
		}
		return locatorIdentity(decisions[i].Locator) < locatorIdentity(decisions[j].Locator)
	})
	spec.Decisions = append([]evidence.LocatorDecision(nil), decisions...)
	for index, record := range selectedEvidence {
		if profile.ObservationKind(record.ObservationKind) != spec.ObservationKind {
			return nil, fmt.Errorf("guide: evidence[%d] observationKind %q does not match the explicit profile observationKind %q", index, record.ObservationKind, spec.ObservationKind)
		}
		observedAt, err := time.Parse(time.RFC3339, record.ObservedAt)
		if err != nil {
			return nil, fmt.Errorf("guide: evidence[%d] observedAt: %w", index, err)
		}
		if observedAt.After(assessedAt) {
			return nil, fmt.Errorf("guide: evidence[%d] is from the future relative to assessment time", index)
		}
	}

	result, err := draft.Build(selectedEvidence, spec)
	if err != nil {
		return nil, err
	}
	if !result.ReadyForReview() {
		return nil, fmt.Errorf("guide: draft is not ready for review")
	}
	reviewed, err := review.Build(result.Profile, selectedEvidence, decisions, assessedAt)
	if err != nil {
		return nil, err
	}
	if !reviewed.Promotable() {
		return nil, fmt.Errorf("guide: review is not promotable; first gap: %s", firstGap(reviewed))
	}
	if err := review.Verify(reviewed, result.Profile, selectedEvidence, assessedAt); err != nil {
		return nil, err
	}
	bundle := &Bundle{
		Version: BundleVersion, Spec: spec, Profile: *result.Profile,
		Evidence: selectedEvidence, Decisions: decisions, Review: *reviewed,
	}
	if err := rejectSecretLike(bundle); err != nil {
		return nil, fmt.Errorf("guide: generated bundle: %w", err)
	}
	return bundle, nil
}

func cloneIntent(value Intent) (Intent, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return Intent{}, fmt.Errorf("guide: clone intent: %w", err)
	}
	var cloned Intent
	if err := json.Unmarshal(data, &cloned); err != nil {
		return Intent{}, fmt.Errorf("guide: clone intent: %w", err)
	}
	return cloned, nil
}

// MarshalDeterministic returns stable indented JSON with one trailing newline.
func MarshalDeterministic(bundle *Bundle) ([]byte, error) {
	if bundle == nil || bundle.Version != BundleVersion {
		return nil, fmt.Errorf("guide: valid guided-authoring bundle is required")
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func validateIntent(catalog *Catalog, intent *Intent) error {
	intent.Info.Title = strings.TrimSpace(intent.Info.Title)
	intent.Info.Provider = strings.TrimSpace(intent.Info.Provider)
	if intent.Info.Title == "" {
		return fmt.Errorf("guide: profile title is required")
	}
	if err := rejectSecretString("profile title", intent.Info.Title); err != nil {
		return err
	}
	if err := rejectSecretString("profile provider", intent.Info.Provider); err != nil {
		return err
	}
	if len(intent.Info.Origin) == 0 {
		return fmt.Errorf("guide: at least one observed origin must be selected")
	}
	knownOrigins := map[string]struct{}{}
	for _, candidate := range catalog.Origins {
		knownOrigins[candidate.Origin] = struct{}{}
	}
	seenOrigins := map[string]struct{}{}
	for index, raw := range intent.Info.Origin {
		origin, err := profile.ParseOrigin(raw)
		if err != nil {
			return fmt.Errorf("guide: info.origin[%d]: %w", index, err)
		}
		if _, ok := knownOrigins[origin]; !ok {
			return fmt.Errorf("guide: info.origin[%d] was not present in reviewed evidence", index)
		}
		if _, duplicate := seenOrigins[origin]; duplicate {
			return fmt.Errorf("guide: origin %q is duplicated", origin)
		}
		seenOrigins[origin] = struct{}{}
		intent.Info.Origin[index] = origin
	}
	sort.Strings(intent.Info.Origin)
	if !slices.Contains([]profile.ObservationKind{
		profile.ObservationAccessibilitySnapshot, profile.ObservationDOMText,
		profile.ObservationScreenshotOCR, profile.ObservationOther,
	}, intent.ObservationKind) {
		return fmt.Errorf("guide: unsupported observation kind %q", intent.ObservationKind)
	}
	if !slices.Contains([]profile.Confidence{profile.ConfidenceLow, profile.ConfidenceMedium, profile.ConfidenceHigh}, intent.Confidence) {
		return fmt.Errorf("guide: unsupported confidence %q", intent.Confidence)
	}
	if intent.ExpiresAfter == "" {
		return fmt.Errorf("guide: expiry is required")
	}
	if _, err := intent.ExpiresAfter.AddTo(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return fmt.Errorf("guide: invalid expiry: %w", err)
	}
	if len(intent.Actions) == 0 || len(intent.Actions) > maxActions {
		return fmt.Errorf("guide: action count must be between 1 and %d", maxActions)
	}
	seenActions := map[string]struct{}{}
	for index := range intent.Actions {
		action := &intent.Actions[index]
		action.ID = strings.TrimSpace(action.ID)
		if !identifierPattern.MatchString(action.ID) {
			return fmt.Errorf("guide: action ID %q must match %s", action.ID, identifierPattern)
		}
		if _, duplicate := seenActions[action.ID]; duplicate {
			return fmt.Errorf("guide: action ID %q is duplicated", action.ID)
		}
		seenActions[action.ID] = struct{}{}
	}
	return nil
}

func buildAction(catalog *Catalog, action ActionIntent, origins profile.Origins) ([]evidence.Record, draft.ActionSpec, []evidence.LocatorDecision, error) {
	if len(action.EvidenceIDs) == 0 || len(action.EvidenceIDs) > maxEvidencePerAction {
		return nil, draft.ActionSpec{}, nil, fmt.Errorf("evidence selection count must be between 1 and %d", maxEvidencePerAction)
	}
	selectedIDs, err := uniqueSorted(action.EvidenceIDs, "evidence ID")
	if err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	selectedSet := map[string]struct{}{}
	records := make([]evidence.Record, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		record, ok := catalog.recordByID[id]
		if !ok {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("unknown evidence ID %q", id)
		}
		selectedSet[id] = struct{}{}
		record.ActionHint = action.ID
		raw := evidence.RawRecord{Record: record}
		normalized, err := raw.Normalize()
		if err != nil {
			return nil, draft.ActionSpec{}, nil, err
		}
		records = append(records, normalized)
	}

	parameters, parameterNames, err := buildParameters(action.Parameters)
	if err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	usedLocators := map[string]LocatorCandidate{}
	sequence := make([]profile.Step, 0, len(action.Sequence))
	if len(action.Sequence) == 0 || len(action.Sequence) > maxSteps {
		return nil, draft.ActionSpec{}, nil, fmt.Errorf("sequence count must be between 1 and %d", maxSteps)
	}
	for index, intent := range action.Sequence {
		step, locators, err := buildStep(catalog, selectedSet, parameterNames, intent)
		if err != nil {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("sequence[%d]: %w", index, err)
		}
		sequence = append(sequence, step)
		for _, locator := range locators {
			usedLocators[locator.ID] = locator
		}
	}

	outputs := map[string]profile.Output{}
	outputIDs, err := uniqueSorted(action.OutputIDs, "output ID")
	if err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	for _, id := range outputIDs {
		candidate, ok := catalog.outputByID[id]
		if !ok {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("unknown output ID %q", id)
		}
		if _, selected := selectedSet[candidate.RecordID]; !selected {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output %q is outside selected evidence", id)
		}
		if !candidate.Bound {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output %q is an unbound hint; use an explicit portable output declaration", id)
		}
		if redact.SensitiveKey(strings.ToLower(candidate.Output.Key)) ||
			(candidate.Output.Property != "" && redact.SensitiveKey(strings.ToLower(candidate.Output.Property))) {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output %q is credential-shaped", id)
		}
		if candidate.Output.Source == string(profile.OutputCSS) && !portableCSS(candidate.Output.Selector) {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output %q uses non-portable selector syntax", id)
		}
		output := candidateOutput(candidate.Output)
		if previous, duplicate := outputs[candidate.Output.Key]; duplicate && !reflect.DeepEqual(previous, output) {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("selected outputs conflict for key %q", candidate.Output.Key)
		}
		outputs[candidate.Output.Key] = output
		if candidate.LocatorID != "" {
			usedLocators[candidate.LocatorID] = catalog.locatorByID[candidate.LocatorID]
		}
	}
	declarationHints := map[string]struct{}{}
	for index, declaration := range action.OutputDeclarations {
		hintID := strings.TrimSpace(declaration.HintID)
		if _, duplicate := declarationHints[hintID]; duplicate {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration hint %q is duplicated", hintID)
		}
		declarationHints[hintID] = struct{}{}
		candidate, ok := catalog.outputByID[hintID]
		if !ok || candidate.Bound {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration[%d] must reference an unbound output hint", index)
		}
		if _, selected := selectedSet[candidate.RecordID]; !selected {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration hint %q is outside selected evidence", hintID)
		}
		output := profile.Output{Type: profile.OutputType(candidate.Output.Type), Source: declaration.Source}
		switch declaration.Source {
		case profile.OutputA11y:
			locatorCandidate, ok := catalog.locatorByID[strings.TrimSpace(declaration.LocatorID)]
			if !ok {
				return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration[%d] requires a known accessibility locator", index)
			}
			if _, selected := selectedSet[locatorCandidate.RecordID]; !selected {
				return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration locator %q is outside selected evidence", declaration.LocatorID)
			}
			locator := profileLocator(locatorCandidate.Locator)
			output.Locator = &locator
			usedLocators[locatorCandidate.ID] = locatorCandidate
		case profile.OutputJSONLD, profile.OutputMicrodata:
			output.Property = strings.TrimSpace(declaration.Property)
			if output.Property == "" || redact.SensitiveKey(strings.ToLower(output.Property)) {
				return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration[%d] requires property", index)
			}
			if err := rejectSecretString("output declaration property", output.Property); err != nil {
				return nil, draft.ActionSpec{}, nil, err
			}
		case profile.OutputCSS:
			output.Selector = strings.TrimSpace(declaration.Selector)
			output.FallbackReason = declaration.FallbackReason
			if !portableCSS(output.Selector) || !slices.Contains([]profile.FallbackReason{
				profile.FallbackNoA11yRegion, profile.FallbackNoStructuredData, profile.FallbackAmbiguousA11y, profile.FallbackOther,
			}, output.FallbackReason) {
				return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration[%d] requires portable CSS and a supported fallback reason", index)
			}
			output.Validation = profile.JSONSchema{"type": candidate.Output.Type}
		default:
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("output declaration[%d] has unsupported source %q", index, declaration.Source)
		}
		if previous, duplicate := outputs[candidate.Output.Key]; duplicate && !reflect.DeepEqual(previous, output) {
			return nil, draft.ActionSpec{}, nil, fmt.Errorf("selected outputs conflict for key %q", candidate.Output.Key)
		}
		outputs[candidate.Output.Key] = output
	}

	description := strings.TrimSpace(action.Description)
	if err := rejectSecretString("action description", description); err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	sideEffects, err := normalizeSideEffects(action.SideEffects)
	if err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	confirmation := action.ConfirmationPolicy
	confirmation.Prompt = strings.TrimSpace(confirmation.Prompt)
	if err := rejectSecretString("confirmation prompt", confirmation.Prompt); err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	if hasMutation(sideEffects) && !confirmation.Required {
		return nil, draft.ActionSpec{}, nil, fmt.Errorf("mutating side effects require explicit confirmation")
	}
	if confirmation.Required && confirmation.Prompt == "" {
		return nil, draft.ActionSpec{}, nil, fmt.Errorf("confirmation prompt is required when confirmation is enabled")
	}
	if err := validateActionTemplates(sequence, confirmation, parameterNames, origins); err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}

	decisions, err := buildDecisions(action.ID, records, usedLocators, action.AmbiguityResolutions)
	if err != nil {
		return nil, draft.ActionSpec{}, nil, err
	}
	return records, draft.ActionSpec{
		Description: description, Parameters: parameters, Sequence: sequence,
		Outputs: outputs, SideEffects: sideEffects, ConfirmationPolicy: confirmation,
	}, decisions, nil
}

func buildParameters(intents []ParameterIntent) (profile.JSONSchema, map[string]struct{}, error) {
	if len(intents) > maxParameters {
		return nil, nil, fmt.Errorf("parameters exceed %d", maxParameters)
	}
	if len(intents) == 0 {
		return nil, map[string]struct{}{}, nil
	}
	properties := map[string]any{}
	var required []string
	names := map[string]struct{}{}
	for index, parameter := range intents {
		name := strings.TrimSpace(parameter.Name)
		if !identifierPattern.MatchString(name) {
			return nil, nil, fmt.Errorf("parameter[%d] name %q must match %s", index, name, identifierPattern)
		}
		if redact.SensitiveKey(strings.ToLower(name)) {
			return nil, nil, fmt.Errorf("parameter[%d] uses a credential-shaped name", index)
		}
		if _, duplicate := names[name]; duplicate {
			return nil, nil, fmt.Errorf("parameter name %q is duplicated", name)
		}
		if !slices.Contains([]string{"string", "integer", "number", "boolean"}, parameter.Type) {
			return nil, nil, fmt.Errorf("parameter %q has unsupported scalar type %q", name, parameter.Type)
		}
		names[name] = struct{}{}
		properties[name] = map[string]any{"type": parameter.Type}
		if parameter.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	schema := profile.JSONSchema{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, names, nil
}

func buildStep(catalog *Catalog, selected map[string]struct{}, parameters map[string]struct{}, intent StepIntent) (profile.Step, []LocatorCandidate, error) {
	var used []LocatorCandidate
	locatorFor := func(id string) (*profile.Locator, error) {
		candidate, ok := catalog.locatorByID[id]
		if !ok {
			return nil, fmt.Errorf("unknown locator ID %q", id)
		}
		if _, ok := selected[candidate.RecordID]; !ok {
			return nil, fmt.Errorf("locator %q is outside selected evidence", id)
		}
		used = append(used, candidate)
		locator := profileLocator(candidate.Locator)
		return &locator, nil
	}
	waitFor := func(wait *WaitIntent) (*profile.WaitForCondition, error) {
		if wait == nil {
			return nil, nil
		}
		if wait.LocatorID != "" && wait.Navigation != nil {
			return nil, fmt.Errorf("wait must choose exactly one locator or navigation event")
		}
		if wait.LocatorID != "" {
			locator, err := locatorFor(wait.LocatorID)
			if err != nil {
				return nil, err
			}
			return &profile.WaitForCondition{Locator: locator}, nil
		}
		if wait.Navigation != nil && slices.Contains([]profile.NavigationWait{
			profile.NavigationLoad, profile.NavigationDOMContentLoaded, profile.NavigationNetworkIdle,
		}, *wait.Navigation) {
			value := *wait.Navigation
			return &profile.WaitForCondition{Navigation: &value}, nil
		}
		return nil, fmt.Errorf("wait must choose a supported locator or navigation event")
	}

	step := profile.Step{Kind: intent.Kind}
	switch intent.Kind {
	case profile.StepNavigate:
		if intent.LocatorID != "" || intent.ValueParameter != "" || intent.Wait != nil || strings.TrimSpace(intent.Navigate) == "" {
			return profile.Step{}, nil, fmt.Errorf("navigate requires only a non-empty target")
		}
		if err := rejectSecretString("navigate target", intent.Navigate); err != nil {
			return profile.Step{}, nil, err
		}
		step.Navigate = strings.TrimSpace(intent.Navigate)
	case profile.StepClick, profile.StepCheckRadio, profile.StepUncheck:
		if intent.ValueParameter != "" || strings.TrimSpace(intent.Navigate) != "" {
			return profile.Step{}, nil, fmt.Errorf("%s accepts only a locator and optional wait", intent.Kind)
		}
		locator, err := locatorFor(intent.LocatorID)
		if err != nil {
			return profile.Step{}, nil, err
		}
		wait, err := waitFor(intent.Wait)
		if err != nil {
			return profile.Step{}, nil, err
		}
		payload := &profile.LocatorStep{Locator: *locator, WaitFor: wait}
		switch intent.Kind {
		case profile.StepClick:
			step.Click = payload
		case profile.StepCheckRadio:
			step.CheckRadio = payload
		case profile.StepUncheck:
			step.Uncheck = payload
		}
	case profile.StepTypeText, profile.StepSelectOption:
		if strings.TrimSpace(intent.Navigate) != "" {
			return profile.Step{}, nil, fmt.Errorf("%s does not accept a navigate target", intent.Kind)
		}
		locator, err := locatorFor(intent.LocatorID)
		if err != nil {
			return profile.Step{}, nil, err
		}
		parameter := strings.TrimSpace(intent.ValueParameter)
		if _, ok := parameters[parameter]; !ok {
			return profile.Step{}, nil, fmt.Errorf("value parameter %q is not declared", parameter)
		}
		wait, err := waitFor(intent.Wait)
		if err != nil {
			return profile.Step{}, nil, err
		}
		value := "{{" + parameter + "}}"
		if intent.Kind == profile.StepTypeText {
			step.TypeText = &profile.TypeTextStep{Locator: *locator, Value: value, WaitFor: wait}
		} else {
			step.SelectOption = &profile.SelectOptionStep{Locator: *locator, Value: value, WaitFor: wait}
		}
	case profile.StepWaitFor:
		if intent.LocatorID != "" || intent.ValueParameter != "" || strings.TrimSpace(intent.Navigate) != "" || intent.Wait == nil {
			return profile.Step{}, nil, fmt.Errorf("wait_for requires exactly one wait condition")
		}
		wait, err := waitFor(intent.Wait)
		if err != nil {
			return profile.Step{}, nil, err
		}
		step.WaitFor = wait
	default:
		return profile.Step{}, nil, fmt.Errorf("unsupported macro %q", intent.Kind)
	}
	return step, used, nil
}

func buildDecisions(actionID string, records []evidence.Record, used map[string]LocatorCandidate, resolutions []AmbiguityResolution) ([]evidence.LocatorDecision, error) {
	resolutionByID := map[string]string{}
	for _, resolution := range resolutions {
		id := strings.TrimSpace(resolution.LocatorID)
		rationale := strings.TrimSpace(resolution.Rationale)
		if _, duplicate := resolutionByID[id]; duplicate {
			return nil, fmt.Errorf("ambiguity resolution for %q is duplicated", id)
		}
		if err := rejectSecretString("ambiguity rationale", rationale); err != nil {
			return nil, err
		}
		resolutionByID[id] = rationale
	}
	usedIDs := make([]string, 0, len(used))
	for id := range used {
		usedIDs = append(usedIDs, id)
	}
	sort.Strings(usedIDs)
	var decisions []evidence.LocatorDecision
	for _, id := range usedIDs {
		candidate := used[id]
		ambiguous := false
		for _, record := range records {
			for _, locator := range record.CandidateLocators {
				if samePortableLocator(candidate.Locator, locator) && locator.AmbiguityNote != "" {
					ambiguous = true
				}
			}
			for _, output := range record.CandidateOutputs {
				if output.Locator != nil && samePortableLocator(candidate.Locator, *output.Locator) && output.Locator.AmbiguityNote != "" {
					ambiguous = true
				}
			}
		}
		rationale, supplied := resolutionByID[id]
		if ambiguous {
			if !supplied || rationale == "" {
				return nil, fmt.Errorf("selected ambiguous locator %q requires a non-empty rationale", id)
			}
			decisions = append(decisions, evidence.LocatorDecision{
				ActionHint: actionID, Locator: withoutAmbiguity(candidate.Locator), Rationale: rationale,
			})
			delete(resolutionByID, id)
		} else if supplied {
			return nil, fmt.Errorf("locator %q is not an ambiguous selected locator", id)
		}
	}
	if len(resolutionByID) > 0 {
		ids := make([]string, 0, len(resolutionByID))
		for id := range resolutionByID {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("ambiguity resolution %q is unused", ids[0])
	}
	return decisions, nil
}

func outputLocatorID(catalog *Catalog, outputID string) string {
	return catalog.outputByID[outputID].LocatorID
}

func candidateOutput(candidate evidence.CandidateOutput) profile.Output {
	output := profile.Output{
		Type: profile.OutputType(candidate.Type), Source: profile.OutputSource(candidate.Source),
		Selector: candidate.Selector, FallbackReason: profile.FallbackReason(candidate.FallbackReason),
		Property: candidate.Property,
	}
	if candidate.Locator != nil {
		locator := profileLocator(*candidate.Locator)
		output.Locator = &locator
	}
	if output.Source == profile.OutputCSS {
		output.Validation = profile.JSONSchema{"type": candidate.Type}
	}
	return output
}

func profileLocator(candidate evidence.CandidateLocator) profile.Locator {
	return profile.Locator{
		Role: profile.Role(candidate.Role), Name: candidate.Name,
		Text: candidate.Text, Value: candidate.Value,
	}
}

func normalizeSideEffects(values []profile.SideEffect) ([]profile.SideEffect, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("side effects must be explicitly declared")
	}
	allowed := []profile.SideEffect{
		profile.SideEffectReadOnly, profile.SideEffectStateChange, profile.SideEffectSendsEmail,
		profile.SideEffectCreatesRecord, profile.SideEffectUpdatesRecord, profile.SideEffectDeletesResource,
	}
	seen := map[profile.SideEffect]struct{}{}
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return nil, fmt.Errorf("unsupported side effect %q", value)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("side effect %q is duplicated", value)
		}
		seen[value] = struct{}{}
	}
	if _, readOnly := seen[profile.SideEffectReadOnly]; readOnly && len(seen) != 1 {
		return nil, fmt.Errorf("read_only cannot be combined with mutating side effects")
	}
	result := append([]profile.SideEffect(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func hasMutation(values []profile.SideEffect) bool {
	return profile.HasWriteSideEffectList(values)
}

func uniqueSorted(values []string, label string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf("%s cannot be empty", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func firstGap(bundle *review.Bundle) string {
	if len(bundle.Gaps) == 0 {
		return "unknown strict gate failure"
	}
	return bundle.Gaps[0].Kind + " at " + bundle.Gaps[0].Field
}

func containsLocator(values []evidence.CandidateLocator, candidate evidence.CandidateLocator) bool {
	for _, value := range values {
		if samePortableLocator(value, candidate) {
			return true
		}
	}
	return false
}

func samePortableLocator(left, right evidence.CandidateLocator) bool {
	return left.Role == right.Role && left.Name == right.Name && left.Text == right.Text && left.Value == right.Value
}

func withoutAmbiguity(value evidence.CandidateLocator) evidence.CandidateLocator {
	value.AmbiguityNote = ""
	return value
}

func locatorIdentity(value evidence.CandidateLocator) string {
	values := []string{value.Role, value.Name, value.Text, value.Value}
	var result strings.Builder
	for _, item := range values {
		fmt.Fprintf(&result, "%d:", len(item))
		result.WriteString(item)
	}
	return result.String()
}

func cloneRecord(value evidence.Record) (evidence.Record, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return evidence.Record{}, err
	}
	var cloned evidence.Record
	if err := json.Unmarshal(data, &cloned); err != nil {
		return evidence.Record{}, err
	}
	return cloned, nil
}

func rejectSecretString(field, value string) error {
	if len(value) > maxFreeTextBytes {
		return fmt.Errorf("guide: %s exceeds %d bytes", field, maxFreeTextBytes)
	}
	if value != "" && redact.String(value) != value {
		return fmt.Errorf("guide: %s contains a secret-shaped value", field)
	}
	return nil
}

func validateActionTemplates(sequence []profile.Step, confirmation profile.ConfirmationPolicy, parameters map[string]struct{}, origins profile.Origins) error {
	for index, step := range sequence {
		if step.Kind != profile.StepNavigate {
			continue
		}
		if err := validateTemplateReferences(step.Navigate, parameters); err != nil {
			return fmt.Errorf("sequence[%d] navigate target: %w", index, err)
		}
		resolved := templatePattern.ReplaceAllString(step.Navigate, "safe")
		parsed, err := url.Parse(resolved)
		if err != nil {
			return fmt.Errorf("sequence[%d] navigate target is malformed", index)
		}
		if parsed.User != nil {
			return fmt.Errorf("sequence[%d] navigate target contains userinfo", index)
		}
		if parsed.IsAbs() {
			origin, err := profile.OriginOfURL(resolved)
			if err != nil || !slices.Contains([]string(origins), origin) {
				return fmt.Errorf("sequence[%d] navigate target is outside the explicit origin allowlist", index)
			}
		} else if len(origins) != 1 {
			return fmt.Errorf("sequence[%d] relative navigate target requires exactly one origin", index)
		}
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil {
			return fmt.Errorf("sequence[%d] navigate query is malformed", index)
		}
		for key := range query {
			if sensitiveTemplateKey(key) {
				return fmt.Errorf("sequence[%d] navigate query uses a credential-shaped key", index)
			}
		}
	}
	if confirmation.Required {
		if err := validateTemplateReferences(confirmation.Prompt, parameters); err != nil {
			return fmt.Errorf("confirmation prompt: %w", err)
		}
	}
	return nil
}

func validateTemplateReferences(value string, parameters map[string]struct{}) error {
	remainder := templatePattern.ReplaceAllStringFunc(value, func(string) string { return "" })
	if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
		return fmt.Errorf("contains a malformed parameter template")
	}
	for _, match := range templatePattern.FindAllStringSubmatch(value, -1) {
		if _, ok := parameters[match[1]]; !ok {
			return fmt.Errorf("references undeclared parameter %q", match[1])
		}
	}
	return nil
}

func sensitiveTemplateKey(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "auth", "code", "key", "oauth_state", "session", "session_id", "sig", "signature", "state":
		return true
	default:
		return redact.SensitiveKey(normalized)
	}
}

func rejectSecretLike(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document any
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if secretwalk.Contains(document, secretwalk.Config{}) {
		return fmt.Errorf("contains a secret-shaped value")
	}
	return nil
}

func sortCanonical[T any](values []T) error {
	type decorated struct {
		value T
		key   []byte
	}
	items := make([]decorated, len(values))
	for index, value := range values {
		key, err := json.Marshal(value)
		if err != nil {
			return err
		}
		items[index] = decorated{value: value, key: key}
	}
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i].key, items[j].key) < 0 })
	for index := range items {
		values[index] = items[index].value
	}
	return nil
}

func portableCSS(selector string) bool {
	normalized := strings.ToLower(strings.TrimSpace(selector))
	if normalized == "" {
		return false
	}
	for _, forbidden := range []string{" >> ", ">>", ":has-text(", ":text(", ":text-is(", ":text-matches(", ":visible", ":nth-match("} {
		if strings.Contains(normalized, forbidden) {
			return false
		}
	}
	for _, engine := range []string{"css=", "xpath=", "text=", "id=", "role=", "nth="} {
		if strings.HasPrefix(normalized, engine) {
			return false
		}
	}
	return true
}
