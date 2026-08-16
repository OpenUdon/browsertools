package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/evidence/redact"
)

const (
	// LiveCheckVersion identifies the value-free live check report.
	LiveCheckVersion = "browsertools.live-check.v1"
	maxLiveProbes    = 512
)

var probeIDPattern = regexp.MustCompile(`^P[0-9]{3}$`)
var portableAttributePattern = regexp.MustCompile(`^[A-Za-z_:][A-Za-z0-9_.:-]*$`)

// ProbeKind is one member of the closed read-only acquisition probe set.
type ProbeKind string

const (
	ProbeLocator        ProbeKind = "locator"
	ProbeNavigationWait ProbeKind = "navigation_wait"
	ProbeOutput         ProbeKind = "output"
)

// Probe is a private acquisition instruction built only from a validated
// browser profile. It can count/query existing page state but cannot click,
// type, submit, upload, evaluate JavaScript, or change storage.
type Probe struct {
	ID         string
	Kind       ProbeKind
	Locator    *profile.Locator
	Navigation *profile.NavigationWait
	Output     *profile.Output
	OutputKey  string
}

// ProbeResult contains shape facts only. It deliberately has no page value,
// text, URL, selector, or browser error detail.
type ProbeResult struct {
	ID           string
	Matches      int
	ObservedType profile.OutputType
	Reached      bool
	FailureCode  string
}

// LiveCheckRequest binds a validated profile and selected actions to the E03
// exact-origin ephemeral acquisition request. Capture.Probes must be empty;
// Check derives them from the profile.
type LiveCheckRequest struct {
	Profile *profile.Profile
	Actions []string
	Capture LiveRequest
}

// LiveCheckItem is one deterministic value-free check.
type LiveCheckItem struct {
	Kind         ProbeKind          `json:"kind"`
	Path         string             `json:"path"`
	OK           bool               `json:"ok"`
	Matches      int                `json:"matches,omitempty"`
	ExpectedType profile.OutputType `json:"expectedType,omitempty"`
	ObservedType profile.OutputType `json:"observedType,omitempty"`
	Message      string             `json:"message"`
}

// LiveCheckResult is safe to write as a review artifact: it contains only the
// selected action names, declared profile paths, type/count facts, and fixed
// messages. Raw page content remains transient and is discarded.
type LiveCheckResult struct {
	Version       string          `json:"version"`
	ProfileDigest string          `json:"profileDigest"`
	CheckedAt     string          `json:"checkedAt"`
	Origin        string          `json:"origin"`
	Actions       []string        `json:"actions"`
	OK            bool            `json:"ok"`
	Checks        []LiveCheckItem `json:"checks"`
}

type checkRequirement struct {
	probe Probe
	path  string
}

// Check performs read-only observations against the explicitly supplied URL.
// It never executes the profile's sequence macros, including navigate steps.
func Check(ctx context.Context, acquirer Acquirer, request LiveCheckRequest) (LiveCheckResult, error) {
	if request.Profile == nil {
		return LiveCheckResult{}, fmt.Errorf("live check: profile is required")
	}
	value, err := request.Profile.Value()
	if err != nil {
		return LiveCheckResult{}, fmt.Errorf("live check: profile: %w", err)
	}
	if err := profile.Validate(value); err != nil {
		return LiveCheckResult{}, fmt.Errorf("live check: profile: %w", err)
	}
	if len(request.Capture.Probes) != 0 {
		return LiveCheckResult{}, fmt.Errorf("live check: caller-supplied probes are not allowed")
	}
	if strings.TrimSpace(request.Capture.ActionHint) != "" {
		return LiveCheckResult{}, fmt.Errorf("live check: capture action hints are not allowed; select profile actions instead")
	}
	normalizedCapture, origin, err := normalizeLiveRequest(request.Capture)
	if err != nil {
		return LiveCheckResult{}, fmt.Errorf("live check: %w", err)
	}
	for _, allowed := range normalizedCapture.AllowedOrigins {
		if !slices.Contains([]string(request.Profile.Info.Origin), allowed) {
			return LiveCheckResult{}, fmt.Errorf("live check: allowed origin %q is outside profile info.origin", allowed)
		}
	}
	actions, err := selectedActions(request.Profile, request.Actions)
	if err != nil {
		return LiveCheckResult{}, err
	}
	requirements := buildCheckRequirements(request.Profile, actions)
	if len(requirements) == 0 {
		return LiveCheckResult{}, fmt.Errorf("live check: selected actions contain no read-only locator, wait, or output observations")
	}
	probes := make([]Probe, 0, len(requirements))
	for _, requirement := range requirements {
		probes = append(probes, requirement.probe)
	}
	normalizedCapture.Probes = probes
	live, err := Acquire(ctx, acquirer, normalizedCapture)
	if err != nil {
		return LiveCheckResult{}, err
	}
	resultByID := make(map[string]ProbeResult, len(live.ProbeResults))
	for _, result := range live.ProbeResults {
		resultByID[result.ID] = result
	}
	checks := make([]LiveCheckItem, 0, len(requirements))
	for _, requirement := range requirements {
		checks = append(checks, assessProbe(requirement, resultByID[requirement.probe.ID]))
	}
	sort.Slice(checks, func(i, j int) bool {
		if checks[i].Path != checks[j].Path {
			return checks[i].Path < checks[j].Path
		}
		return checks[i].Kind < checks[j].Kind
	})
	digest, err := profileDigest(request.Profile)
	if err != nil {
		return LiveCheckResult{}, err
	}
	result := LiveCheckResult{
		Version: LiveCheckVersion, ProfileDigest: digest,
		CheckedAt: normalizedCapture.ObservedAt.UTC().Format(time.RFC3339Nano),
		Origin:    origin, Actions: actions, OK: true, Checks: checks,
	}
	for _, check := range checks {
		if !check.OK {
			result.OK = false
			break
		}
	}
	return result, nil
}

func selectedActions(prof *profile.Profile, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return prof.SortedActionNames(), nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, ok := prof.Actions[name]; !ok {
			return nil, fmt.Errorf("live check: unknown action %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("live check: action %q is duplicated", name)
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func buildCheckRequirements(prof *profile.Profile, actions []string) []checkRequirement {
	var requirements []checkRequirement
	add := func(kind ProbeKind, path string, locator *profile.Locator, navigation *profile.NavigationWait, outputKey string, output *profile.Output) {
		id := fmt.Sprintf("P%03d", len(requirements)+1)
		requirements = append(requirements, checkRequirement{
			probe: Probe{ID: id, Kind: kind, Locator: cloneLocator(locator), Navigation: cloneNavigation(navigation), OutputKey: outputKey, Output: cloneOutput(output)},
			path:  path,
		})
	}
	for _, actionName := range actions {
		action := prof.Actions[actionName]
		for index, step := range action.Sequence {
			base := fmt.Sprintf("actions.%s.sequence[%d]", actionName, index)
			if step.Kind == profile.StepWaitFor {
				if step.WaitFor != nil && step.WaitFor.Locator != nil {
					add(ProbeLocator, base+".wait_for", step.WaitFor.Locator, nil, "", nil)
				} else if step.WaitFor != nil {
					add(ProbeNavigationWait, base+".wait_for.navigation", nil, step.WaitFor.Navigation, "", nil)
				}
				continue
			}
			if locator := step.Locator(); locator != nil {
				add(ProbeLocator, base+".locator", locator, nil, "", nil)
			}
			if wait := step.PostWait(); wait != nil {
				if wait.Locator != nil {
					add(ProbeLocator, base+".wait_for", wait.Locator, nil, "", nil)
				} else {
					add(ProbeNavigationWait, base+".wait_for.navigation", nil, wait.Navigation, "", nil)
				}
			}
		}
		outputNames := make([]string, 0, len(action.Outputs))
		for name := range action.Outputs {
			outputNames = append(outputNames, name)
		}
		sort.Strings(outputNames)
		for _, name := range outputNames {
			output := action.Outputs[name]
			add(ProbeOutput, fmt.Sprintf("actions.%s.outputs.%s", actionName, name), nil, nil, name, &output)
		}
	}
	return requirements
}

func assessProbe(requirement checkRequirement, result ProbeResult) LiveCheckItem {
	item := LiveCheckItem{Kind: requirement.probe.Kind, Path: requirement.path, Matches: result.Matches}
	if result.FailureCode != "" {
		item.Message = "read-only browser observation failed closed"
		return item
	}
	switch requirement.probe.Kind {
	case ProbeLocator:
		item.OK = result.Matches == 1
		if item.OK {
			item.Message = "declared accessibility locator resolved exactly once"
		} else {
			item.Message = "declared accessibility locator did not resolve exactly once"
		}
	case ProbeNavigationWait:
		item.OK = result.Reached
		if item.OK {
			item.Message = "declared navigation wait was reached without executing an action macro"
		} else {
			item.Message = "declared navigation wait was not reached within the bounded observation"
		}
	case ProbeOutput:
		output := requirement.probe.Output
		item.ExpectedType = output.Type
		item.ObservedType = result.ObservedType
		presence := output.Source == profile.OutputA11y && output.Presence != nil && *output.Presence
		countOK := result.Matches == 1
		if output.Type == profile.OutputArray {
			countOK = result.Matches > 0
		}
		if presence {
			countOK = true
		}
		item.OK = countOK && result.ObservedType == output.Type
		if item.OK {
			item.Message = "declared output source and JSON type matched"
		} else {
			item.Message = "declared output source or JSON type did not match"
		}
	default:
		item.Message = "unsupported read-only probe kind"
	}
	return item
}

func normalizeProbes(values []Probe) ([]Probe, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) > maxLiveProbes {
		return nil, fmt.Errorf("capture probes exceed %d", maxLiveProbes)
	}
	result := append([]Probe(nil), values...)
	seen := map[string]struct{}{}
	for index := range result {
		probe := &result[index]
		if !probeIDPattern.MatchString(probe.ID) {
			return nil, fmt.Errorf("capture probe[%d] has invalid ID", index)
		}
		if _, duplicate := seen[probe.ID]; duplicate {
			return nil, fmt.Errorf("capture probe ID %q is duplicated", probe.ID)
		}
		seen[probe.ID] = struct{}{}
		switch probe.Kind {
		case ProbeLocator:
			if probe.Locator == nil || probe.Navigation != nil || probe.Output != nil || probe.OutputKey != "" {
				return nil, fmt.Errorf("capture locator probe %q has an invalid shape", probe.ID)
			}
			if err := validateProbeLocator(*probe.Locator); err != nil {
				return nil, fmt.Errorf("capture locator probe %q: %w", probe.ID, err)
			}
		case ProbeNavigationWait:
			if probe.Locator != nil || probe.Navigation == nil || probe.Output != nil || probe.OutputKey != "" || !slices.Contains([]profile.NavigationWait{
				profile.NavigationLoad, profile.NavigationDOMContentLoaded, profile.NavigationNetworkIdle,
			}, *probe.Navigation) {
				return nil, fmt.Errorf("capture navigation probe %q has an invalid shape", probe.ID)
			}
		case ProbeOutput:
			if probe.Locator != nil || probe.Navigation != nil || probe.Output == nil || !identifierPatternForProbe(probe.OutputKey) {
				return nil, fmt.Errorf("capture output probe %q has an invalid shape", probe.ID)
			}
			if err := validateProbeOutput(*probe.Output); err != nil {
				return nil, fmt.Errorf("capture output probe %q: %w", probe.ID, err)
			}
			if redact.SensitiveKey(strings.ToLower(probe.OutputKey)) {
				return nil, fmt.Errorf("capture output probe %q has a credential-shaped key", probe.ID)
			}
		default:
			return nil, fmt.Errorf("capture probe %q has unsupported kind", probe.ID)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func validateProbeLocator(locator profile.Locator) error {
	allowed := []profile.Role{
		profile.RoleButton, profile.RoleLink, profile.RoleTextbox, profile.RoleCheckbox, profile.RoleRadio,
		profile.RoleDialog, profile.RoleStatus, profile.RoleAlert, profile.RoleHeading, profile.RoleImg,
		profile.RoleList, profile.RoleListItem, profile.RoleCombobox, profile.RoleOption, profile.RoleMenu,
		profile.RoleMenuItem, profile.RoleTab, profile.RoleTabPanel, profile.RoleTable, profile.RoleRow,
		profile.RoleCell, profile.RoleRegion, profile.RoleNavigation, profile.RoleArticle, profile.RoleForm,
		profile.RoleSearch, profile.RoleSwitch, profile.RoleGroup,
	}
	if !slices.Contains(allowed, locator.Role) {
		return fmt.Errorf("unsupported accessibility role")
	}
	for _, value := range []string{locator.Name, locator.Text, locator.Value} {
		if len(value) > 4096 {
			return fmt.Errorf("locator field exceeds 4096 bytes")
		}
		if redact.String(value) != value {
			return fmt.Errorf("locator contains a secret-shaped value")
		}
	}
	return nil
}

func validateProbeOutput(output profile.Output) error {
	if !slices.Contains([]profile.OutputType{
		profile.OutputString, profile.OutputInteger, profile.OutputNumber, profile.OutputBoolean,
		profile.OutputArray, profile.OutputObject, profile.OutputNull,
	}, output.Type) {
		return fmt.Errorf("unsupported output type")
	}
	if !slices.Contains([]profile.OutputSource{profile.OutputA11y, profile.OutputJSONLD, profile.OutputMicrodata, profile.OutputCSS}, output.Source) {
		return fmt.Errorf("unsupported output source")
	}
	if output.Locator != nil {
		if err := validateProbeLocator(*output.Locator); err != nil {
			return err
		}
	}
	for _, value := range []string{output.Selector, output.Property, output.Attribute} {
		if len(value) > 4096 {
			return fmt.Errorf("output field exceeds 4096 bytes")
		}
		if redact.String(value) != value {
			return fmt.Errorf("output contains a secret-shaped value")
		}
	}
	if output.Property != "" && redact.SensitiveKey(strings.ToLower(output.Property)) {
		return fmt.Errorf("output property is credential-shaped")
	}
	if output.Attribute != "" {
		if !portableAttributePattern.MatchString(output.Attribute) {
			return fmt.Errorf("output attribute is not a portable attribute name")
		}
	}
	if output.Source == profile.OutputCSS {
		normalized := strings.ToLower(strings.TrimSpace(output.Selector))
		for _, forbidden := range []string{" >> ", ">>", ":has-text(", ":text(", ":text-is(", ":text-matches(", ":visible", ":nth-match("} {
			if strings.Contains(normalized, forbidden) {
				return fmt.Errorf("CSS output uses a Playwright-only selector feature")
			}
		}
		for _, engine := range []string{"css=", "xpath=", "text=", "id=", "role=", "nth="} {
			if strings.HasPrefix(normalized, engine) {
				return fmt.Errorf("CSS output must contain plain CSS, not a selector engine")
			}
		}
	}
	return nil
}

func identifierPatternForProbe(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validateProbeResults(probes []Probe, results []ProbeResult) error {
	if len(results) != len(probes) {
		return fmt.Errorf("capture backend returned an incomplete probe result set")
	}
	wanted := make(map[string]Probe, len(probes))
	for _, probe := range probes {
		wanted[probe.ID] = probe
	}
	seen := map[string]struct{}{}
	for _, result := range results {
		probe, ok := wanted[result.ID]
		if !ok {
			return fmt.Errorf("capture backend returned an unknown probe result")
		}
		if _, duplicate := seen[result.ID]; duplicate {
			return fmt.Errorf("capture backend returned a duplicate probe result")
		}
		seen[result.ID] = struct{}{}
		if result.Matches < 0 || result.Matches > 1_000_000 {
			return fmt.Errorf("capture backend returned an invalid probe match count")
		}
		if result.FailureCode != "" && result.FailureCode != "probe_failed" && result.FailureCode != "unsupported" && result.FailureCode != "timeout" {
			return fmt.Errorf("capture backend returned an invalid probe failure code")
		}
		if probe.Kind != ProbeOutput && result.ObservedType != "" {
			return fmt.Errorf("capture backend returned an output type for a non-output probe")
		}
		if result.ObservedType != "" && !slices.Contains([]profile.OutputType{
			profile.OutputString, profile.OutputInteger, profile.OutputNumber, profile.OutputBoolean,
			profile.OutputArray, profile.OutputObject, profile.OutputNull,
		}, result.ObservedType) {
			return fmt.Errorf("capture backend returned an invalid observed output type")
		}
	}
	return nil
}

func profileDigest(prof *profile.Profile) (string, error) {
	data, err := json.Marshal(prof)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneLocator(value *profile.Locator) *profile.Locator {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneNavigation(value *profile.NavigationWait) *profile.NavigationWait {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneOutput(value *profile.Output) *profile.Output {
	if value == nil {
		return nil
	}
	data, _ := json.Marshal(value)
	var cloned profile.Output
	_ = json.Unmarshal(data, &cloned)
	return &cloned
}
