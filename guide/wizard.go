package guide

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
)

const maxAnswerBytes = 64 << 10

// RunWizard presents a deterministic terminal questionnaire and returns only
// a strictly promotable guided-authoring bundle. Prompts go to prompts so JSON
// output can remain separate. Answers are read one line at a time; no answer is
// treated as a browser instruction or executable code.
func RunWizard(in io.Reader, prompts io.Writer, records []evidence.Record, assessedAt time.Time) (*Bundle, error) {
	if in == nil || prompts == nil {
		return nil, fmt.Errorf("guide: wizard input and prompt output are required")
	}
	catalog, err := NewCatalog(records)
	if err != nil {
		return nil, err
	}
	p := &prompter{scanner: bufio.NewScanner(in), out: prompts}
	p.scanner.Buffer(make([]byte, 1024), maxAnswerBytes)
	renderCatalog(prompts, catalog)

	title, err := p.required("profile title")
	if err != nil {
		return nil, err
	}
	provider, err := p.optional("profile provider (blank for none)")
	if err != nil {
		return nil, err
	}
	loginRequired, err := p.yesNo("does the profile require a signed-in runtime session? (yes/no)")
	if err != nil {
		return nil, err
	}
	originIDs := make([]string, 0, len(catalog.Origins))
	originValues := map[string]string{}
	for _, candidate := range catalog.Origins {
		originIDs = append(originIDs, candidate.ID)
		originValues[candidate.ID] = candidate.Origin
	}
	selectedOriginIDs, err := p.idList("allowed origin IDs (comma-separated)", originIDs, false)
	if err != nil {
		return nil, err
	}
	origins := make(profile.Origins, 0, len(selectedOriginIDs))
	for _, id := range selectedOriginIDs {
		origins = append(origins, originValues[id])
	}

	observation, err := p.choice("observation kind", []string{
		string(profile.ObservationAccessibilitySnapshot), string(profile.ObservationDOMText),
		string(profile.ObservationScreenshotOCR), string(profile.ObservationOther),
	})
	if err != nil {
		return nil, err
	}
	confidence, err := p.choice("confidence", []string{
		string(profile.ConfidenceLow), string(profile.ConfidenceMedium), string(profile.ConfidenceHigh),
	})
	if err != nil {
		return nil, err
	}
	expiresAfter, err := p.required("expiry duration (ISO-8601, for example P14D)")
	if err != nil {
		return nil, err
	}
	actionCount, err := p.integer("number of actions", 1, maxActions)
	if err != nil {
		return nil, err
	}

	intent := Intent{
		Info: profile.Info{
			Title: title, Provider: provider, Origin: origins, LoginStateRequired: loginRequired,
		},
		ObservationKind: profile.ObservationKind(observation),
		Confidence:      profile.Confidence(confidence),
		ExpiresAfter:    profile.Duration(expiresAfter),
	}
	seenActions := map[string]struct{}{}
	for actionIndex := 0; actionIndex < actionCount; actionIndex++ {
		fmt.Fprintf(prompts, "\nAction %d of %d\n", actionIndex+1, actionCount)
		action, err := runActionQuestions(p, catalog, seenActions)
		if err != nil {
			return nil, err
		}
		seenActions[action.ID] = struct{}{}
		intent.Actions = append(intent.Actions, action)
	}
	return Author(catalog, intent, assessedAt)
}

func runActionQuestions(p *prompter, catalog *Catalog, seenActions map[string]struct{}) (ActionIntent, error) {
	var action ActionIntent
	for {
		id, err := p.required("action ID")
		if err != nil {
			return ActionIntent{}, err
		}
		if !identifierPattern.MatchString(id) {
			fmt.Fprintf(p.out, "answer must match %s\n", identifierPattern)
			continue
		}
		if _, duplicate := seenActions[id]; duplicate {
			fmt.Fprintln(p.out, "answer duplicates an earlier action ID")
			continue
		}
		action.ID = id
		break
	}
	description, err := p.optional("action description (blank for none)")
	if err != nil {
		return ActionIntent{}, err
	}
	action.Description = description
	recordIDs := make([]string, 0, len(catalog.Records))
	for _, candidate := range catalog.Records {
		recordIDs = append(recordIDs, candidate.ID)
	}
	action.EvidenceIDs, err = p.idList("supporting evidence IDs (comma-separated)", recordIDs, false)
	if err != nil {
		return ActionIntent{}, err
	}
	selectedRecords := make(map[string]struct{}, len(action.EvidenceIDs))
	for _, id := range action.EvidenceIDs {
		selectedRecords[id] = struct{}{}
	}
	renderActionCandidates(p.out, catalog, selectedRecords)

	parameterCount, err := p.integer("number of parameters", 0, maxParameters)
	if err != nil {
		return ActionIntent{}, err
	}
	parameterNames := map[string]struct{}{}
	for index := 0; index < parameterCount; index++ {
		fmt.Fprintf(p.out, "Parameter %d of %d\n", index+1, parameterCount)
		var name string
		for {
			name, err = p.required("parameter name")
			if err != nil {
				return ActionIntent{}, err
			}
			if !identifierPattern.MatchString(name) {
				fmt.Fprintf(p.out, "answer must match %s\n", identifierPattern)
				continue
			}
			if _, duplicate := parameterNames[name]; duplicate {
				fmt.Fprintln(p.out, "answer duplicates an earlier parameter name")
				continue
			}
			break
		}
		parameterType, err := p.choice("parameter JSON type", []string{"string", "integer", "number", "boolean"})
		if err != nil {
			return ActionIntent{}, err
		}
		required, err := p.yesNo("is the parameter required? (yes/no)")
		if err != nil {
			return ActionIntent{}, err
		}
		parameterNames[name] = struct{}{}
		action.Parameters = append(action.Parameters, ParameterIntent{Name: name, Type: parameterType, Required: required})
	}

	outputIDs := candidateOutputIDs(catalog, selectedRecords)
	if len(outputIDs) == 0 {
		fmt.Fprintln(p.out, "No output candidates are present in the selected evidence; outputs are explicitly none.")
		action.OutputIDs = []string{}
	} else {
		action.OutputIDs, err = p.idList("output candidate IDs (comma-separated or none)", outputIDs, true)
		if err != nil {
			return ActionIntent{}, err
		}
	}

	stepCount, err := p.integer("number of sequence macros", 1, maxSteps)
	if err != nil {
		return ActionIntent{}, err
	}
	locatorIDs := candidateLocatorIDs(catalog, selectedRecords)
	for index := 0; index < stepCount; index++ {
		fmt.Fprintf(p.out, "Macro %d of %d\n", index+1, stepCount)
		step, err := runStepQuestions(p, locatorIDs, parameterNames)
		if err != nil {
			return ActionIntent{}, err
		}
		action.Sequence = append(action.Sequence, step)
	}

	sideEffects, err := p.sideEffects()
	if err != nil {
		return ActionIntent{}, err
	}
	action.SideEffects = sideEffects
	confirmation, err := p.yesNo("must the runtime request confirmation? (yes/no)")
	if err != nil {
		return ActionIntent{}, err
	}
	if hasMutation(sideEffects) && !confirmation {
		return ActionIntent{}, fmt.Errorf("guide: operator declined mandatory confirmation for mutating action %q", action.ID)
	}
	action.ConfirmationPolicy.Required = confirmation
	if confirmation {
		action.ConfirmationPolicy.Prompt, err = p.required("confirmation prompt")
		if err != nil {
			return ActionIntent{}, err
		}
	}

	usedLocators := map[string]struct{}{}
	for _, step := range action.Sequence {
		if step.LocatorID != "" {
			usedLocators[step.LocatorID] = struct{}{}
		}
		if step.Wait != nil && step.Wait.LocatorID != "" {
			usedLocators[step.Wait.LocatorID] = struct{}{}
		}
	}
	for _, outputID := range action.OutputIDs {
		if locatorID := outputLocatorID(catalog, outputID); locatorID != "" {
			usedLocators[locatorID] = struct{}{}
		}
	}
	usedIDs := make([]string, 0, len(usedLocators))
	for id := range usedLocators {
		usedIDs = append(usedIDs, id)
	}
	sort.Strings(usedIDs)
	for _, id := range usedIDs {
		candidate := catalog.locatorByID[id]
		if !locatorAmbiguous(catalog, selectedRecords, candidate.Locator) {
			continue
		}
		fmt.Fprintf(p.out, "Ambiguous locator %s: role=%q name=%q\n", id, candidate.Locator.Role, candidate.Locator.Name)
		rationale, err := p.required("ambiguity decision rationale")
		if err != nil {
			return ActionIntent{}, err
		}
		action.AmbiguityResolutions = append(action.AmbiguityResolutions, AmbiguityResolution{LocatorID: id, Rationale: rationale})
	}
	return action, nil
}

func runStepQuestions(p *prompter, locatorIDs []string, parameters map[string]struct{}) (StepIntent, error) {
	kind, err := p.choice("macro", []string{
		string(profile.StepNavigate), string(profile.StepClick), string(profile.StepTypeText),
		string(profile.StepCheckRadio), string(profile.StepUncheck), string(profile.StepSelectOption),
		string(profile.StepWaitFor),
	})
	if err != nil {
		return StepIntent{}, err
	}
	step := StepIntent{Kind: profile.StepKind(kind)}
	switch step.Kind {
	case profile.StepNavigate:
		step.Navigate, err = p.required("navigate target (literal or {{parameter}} template)")
	case profile.StepClick, profile.StepCheckRadio, profile.StepUncheck:
		step.LocatorID, err = p.oneID("locator ID", locatorIDs)
		if err == nil {
			step.Wait, err = p.optionalWait(locatorIDs)
		}
	case profile.StepTypeText, profile.StepSelectOption:
		step.LocatorID, err = p.oneID("locator ID", locatorIDs)
		if err == nil {
			parameterNames := make([]string, 0, len(parameters))
			for name := range parameters {
				parameterNames = append(parameterNames, name)
			}
			sort.Strings(parameterNames)
			if len(parameterNames) == 0 {
				return StepIntent{}, fmt.Errorf("guide: %s requires a declared value parameter", step.Kind)
			}
			step.ValueParameter, err = p.choice("value parameter", parameterNames)
		}
		if err == nil {
			step.Wait, err = p.optionalWait(locatorIDs)
		}
	case profile.StepWaitFor:
		step.Wait, err = p.requiredWait(locatorIDs)
	}
	return step, err
}

type prompter struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func (p *prompter) line(label string) (string, error) {
	fmt.Fprintf(p.out, "%s: ", label)
	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return "", fmt.Errorf("guide: read answer: %w", err)
		}
		return "", fmt.Errorf("guide: answer stream ended at %q", label)
	}
	return strings.TrimSpace(p.scanner.Text()), nil
}

func (p *prompter) required(label string) (string, error) {
	for {
		answer, err := p.line(label)
		if err != nil {
			return "", err
		}
		if answer != "" {
			return answer, nil
		}
		fmt.Fprintln(p.out, "answer must not be empty")
	}
}

func (p *prompter) optional(label string) (string, error) { return p.line(label) }

func (p *prompter) choice(label string, allowed []string) (string, error) {
	if len(allowed) == 0 {
		return "", fmt.Errorf("guide: %s has no available choices", label)
	}
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for {
		answer, err := p.line(fmt.Sprintf("%s [%s]", label, strings.Join(allowed, "/")))
		if err != nil {
			return "", err
		}
		if _, ok := set[answer]; ok {
			return answer, nil
		}
		fmt.Fprintln(p.out, "answer is not one of the listed choices")
	}
}

func (p *prompter) yesNo(label string) (bool, error) {
	answer, err := p.choice(label, []string{"yes", "no"})
	return answer == "yes", err
}

func (p *prompter) integer(label string, minimum, maximum int) (int, error) {
	for {
		answer, err := p.line(fmt.Sprintf("%s (%d-%d)", label, minimum, maximum))
		if err != nil {
			return 0, err
		}
		value, parseErr := strconv.Atoi(answer)
		if parseErr == nil && value >= minimum && value <= maximum {
			return value, nil
		}
		fmt.Fprintln(p.out, "answer is outside the permitted integer range")
	}
}

func (p *prompter) oneID(label string, allowed []string) (string, error) {
	return p.choice(label, allowed)
}

func (p *prompter) idList(label string, allowed []string, permitNone bool) ([]string, error) {
	set := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		set[id] = struct{}{}
	}
	for {
		answer, err := p.line(label)
		if err != nil {
			return nil, err
		}
		if permitNone && answer == "none" {
			return []string{}, nil
		}
		parts := strings.Split(answer, ",")
		seen := map[string]struct{}{}
		var result []string
		valid := len(parts) > 0
		for _, raw := range parts {
			id := strings.TrimSpace(raw)
			if _, ok := set[id]; !ok {
				valid = false
				break
			}
			if _, duplicate := seen[id]; duplicate {
				valid = false
				break
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
		if valid {
			sort.Strings(result)
			return result, nil
		}
		fmt.Fprintln(p.out, "answer must contain unique IDs from the listed candidates")
	}
}

func (p *prompter) optionalWait(locatorIDs []string) (*WaitIntent, error) {
	allowed := append([]string{"none", string(profile.NavigationLoad), string(profile.NavigationDOMContentLoaded), string(profile.NavigationNetworkIdle)}, locatorIDs...)
	answer, err := p.choice("post-macro wait", allowed)
	if err != nil || answer == "none" {
		return nil, err
	}
	return waitAnswer(answer), nil
}

func (p *prompter) requiredWait(locatorIDs []string) (*WaitIntent, error) {
	allowed := append([]string{string(profile.NavigationLoad), string(profile.NavigationDOMContentLoaded), string(profile.NavigationNetworkIdle)}, locatorIDs...)
	answer, err := p.choice("wait condition", allowed)
	if err != nil {
		return nil, err
	}
	return waitAnswer(answer), nil
}

func (p *prompter) sideEffects() ([]profile.SideEffect, error) {
	allowed := []string{
		string(profile.SideEffectReadOnly), string(profile.SideEffectStateChange), string(profile.SideEffectSendsEmail),
		string(profile.SideEffectCreatesRecord), string(profile.SideEffectUpdatesRecord), string(profile.SideEffectDeletesResource),
	}
	for {
		answer, err := p.line("side effects (read_only alone, or comma-separated mutations)")
		if err != nil {
			return nil, err
		}
		parts := strings.Split(answer, ",")
		var values []profile.SideEffect
		for _, part := range parts {
			values = append(values, profile.SideEffect(strings.TrimSpace(part)))
		}
		if normalized, normalizeErr := normalizeSideEffects(values); normalizeErr == nil {
			return normalized, nil
		}
		fmt.Fprintf(p.out, "answer must use [%s] and keep read_only exclusive\n", strings.Join(allowed, "/"))
	}
}

func waitAnswer(answer string) *WaitIntent {
	switch profile.NavigationWait(answer) {
	case profile.NavigationLoad, profile.NavigationDOMContentLoaded, profile.NavigationNetworkIdle:
		value := profile.NavigationWait(answer)
		return &WaitIntent{Navigation: &value}
	default:
		return &WaitIntent{LocatorID: answer}
	}
}

func candidateLocatorIDs(catalog *Catalog, selected map[string]struct{}) []string {
	var ids []string
	for _, candidate := range catalog.Locators {
		if _, ok := selected[candidate.RecordID]; ok {
			ids = append(ids, candidate.ID)
		}
	}
	return ids
}

func candidateOutputIDs(catalog *Catalog, selected map[string]struct{}) []string {
	var ids []string
	for _, candidate := range catalog.Outputs {
		if _, ok := selected[candidate.RecordID]; ok {
			ids = append(ids, candidate.ID)
		}
	}
	return ids
}

func locatorAmbiguous(catalog *Catalog, selected map[string]struct{}, locator evidence.CandidateLocator) bool {
	for recordID := range selected {
		record := catalog.recordByID[recordID]
		for _, candidate := range record.CandidateLocators {
			if samePortableLocator(locator, candidate) && candidate.AmbiguityNote != "" {
				return true
			}
		}
		for _, output := range record.CandidateOutputs {
			if output.Locator != nil && samePortableLocator(locator, *output.Locator) && output.Locator.AmbiguityNote != "" {
				return true
			}
		}
	}
	return false
}

func renderCatalog(w io.Writer, catalog *Catalog) {
	fmt.Fprintln(w, "Reviewed evidence candidates (stable for these exact records):")
	for _, origin := range catalog.Origins {
		fmt.Fprintf(w, "  %s origin=%q\n", origin.ID, origin.Origin)
	}
	for _, record := range catalog.Records {
		fmt.Fprintf(w, "  %s origin=%q observed=%q tool=%q", record.ID, record.Origin, record.ObservedAt, record.Tool)
		if record.SourceActionHint != "" {
			fmt.Fprintf(w, " source-action-hint=%q", record.SourceActionHint)
		}
		fmt.Fprintln(w)
	}
}

func renderActionCandidates(w io.Writer, catalog *Catalog, selected map[string]struct{}) {
	fmt.Fprintln(w, "Accessibility locator candidates:")
	for _, candidate := range catalog.Locators {
		if _, ok := selected[candidate.RecordID]; !ok {
			continue
		}
		fmt.Fprintf(w, "  %s role=%q name=%q", candidate.ID, candidate.Locator.Role, candidate.Locator.Name)
		if candidate.Locator.Text != "" {
			fmt.Fprintf(w, " text=%q", candidate.Locator.Text)
		}
		if candidate.Locator.Value != "" {
			fmt.Fprintf(w, " value=%q", candidate.Locator.Value)
		}
		if candidate.Locator.AmbiguityNote != "" {
			fmt.Fprintf(w, " ambiguous=%q", candidate.Locator.AmbiguityNote)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "Output candidates:")
	for _, candidate := range catalog.Outputs {
		if _, ok := selected[candidate.RecordID]; !ok {
			continue
		}
		fmt.Fprintf(w, "  %s key=%q type=%q source=%q", candidate.ID, candidate.Output.Key, candidate.Output.Type, candidate.Output.Source)
		if candidate.Output.Property != "" {
			fmt.Fprintf(w, " property=%q", candidate.Output.Property)
		}
		if candidate.LocatorID != "" {
			fmt.Fprintf(w, " locator=%s", candidate.LocatorID)
		}
		fmt.Fprintln(w)
	}
}
