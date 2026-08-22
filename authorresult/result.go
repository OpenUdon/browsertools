// Package authorresult builds the private deterministic output of one
// authenticated goal-directed authoring session. It contains inert candidate
// profiles and digest-bound review metadata, never browser session material.
package authorresult

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/evidence/redact"
)

const Schema = "browsertools.authenticated-authoring.v2"

// Bounds records the finite authority used by the live author session.
type Bounds struct {
	NavigationTimeoutMS int64 `json:"navigationTimeoutMs"`
	TotalTimeoutMS      int64 `json:"totalTimeoutMs"`
	MaxRequests         int   `json:"maxRequests"`
	MaxResponseBytes    int64 `json:"maxResponseBytes"`
	MaxObservations     int   `json:"maxObservations"`
	MaxCandidates       int   `json:"maxCandidates"`
	MaxOutputs          int   `json:"maxOutputs"`
}

// Context is one portable popup/frame relationship discovered and reviewed in
// the live context. Main is implicit and never appears in the map.
type Context struct {
	Kind   string `json:"kind"`
	Parent string `json:"parent"`
	Origin string `json:"origin"`
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
}

// GoalPredicate is the reviewed typed completion condition.
type GoalPredicate struct {
	Origin    string `json:"origin"`
	Path      string `json:"path"`
	Context   string `json:"context,omitempty"`
	Role      string `json:"role"`
	Label     string `json:"label,omitempty"`
	Candidate string `json:"candidateId,omitempty"`
}

// GoalProof records value-free evidence for the exact predicate.
type GoalProof struct {
	Origin  string `json:"origin"`
	Path    string `json:"path"`
	Context string `json:"context"`
	Role    string `json:"role"`
	Label   string `json:"label,omitempty"`
	Matches int    `json:"matches"`
}

// TraceStep is one executed portable authoring action. Backend selectors and
// input values are intentionally absent.
type TraceStep struct {
	Kind          string `json:"kind"`
	Phase         string `json:"phase"`
	CandidateID   string `json:"candidateId,omitempty"`
	Context       string `json:"context,omitempty"`
	Role          string `json:"role,omitempty"`
	Label         string `json:"label,omitempty"`
	InputKind     string `json:"inputKind,omitempty"`
	ChallengeKind string `json:"challengeKind,omitempty"`
	URL           string `json:"url,omitempty"`
	POSTBudget    int    `json:"postBudget,omitempty"`
	POSTObserved  int    `json:"postObserved,omitempty"`
	OpensContext  string `json:"opensContext,omitempty"`
}

// OutputSelection is one human-reviewed, action-time-revalidated output
// locator. It proves accessible matching without carrying the page value.
type OutputSelection struct {
	CandidateID string `json:"candidateId"`
	Key         string `json:"key"`
	Type        string `json:"type"`
	LocatorMode string `json:"locatorMode"`
	Observation int    `json:"observation"`
	Context     string `json:"context"`
	Role        string `json:"role"`
	Name        string `json:"name,omitempty"`
	Matches     int    `json:"matches"`
	RoleMatches int    `json:"roleMatches"`
}

// Review binds one exact candidate profile to the completed authoring review.
type Review struct {
	Schema        string   `json:"schema"`
	Kind          string   `json:"kind"`
	ProfileDigest string   `json:"profileDigest"`
	AssessedAt    string   `json:"assessedAt"`
	Decisions     []string `json:"decisions"`
}

// Envelope is the private, non-publishable authoring result.
type Envelope struct {
	Schema                string             `json:"schema"`
	ObservedAt            string             `json:"observedAt"`
	Goal                  string             `json:"goal"`
	GoalPredicate         GoalPredicate      `json:"goalPredicate"`
	GoalProof             GoalProof          `json:"goalProof"`
	HumanConfirmed        bool               `json:"humanConfirmed"`
	Origins               []string           `json:"origins"`
	Contexts              map[string]Context `json:"contexts"`
	Bounds                Bounds             `json:"bounds"`
	Trace                 []TraceStep        `json:"trace"`
	OutputSelections      []OutputSelection  `json:"outputSelections"`
	AuthenticationProfile json.RawMessage    `json:"authenticationProfile"`
	AuthenticationReview  Review             `json:"authenticationReview"`
	CapabilityProfile     json.RawMessage    `json:"capabilityProfile"`
	CapabilityReview      Review             `json:"capabilityReview"`
	Diagnostics           []string           `json:"diagnostics"`
}

// BuildRequest contains only already reduced and reviewed authoring facts.
type BuildRequest struct {
	ObservedAt          time.Time
	Title               string
	Goal                string
	InitialURL          string
	DashboardURL        string
	Origins             []string
	Contexts            map[string]Context
	Bounds              Bounds
	Trace               []TraceStep
	OutputSelections    []OutputSelection
	GoalPredicate       GoalPredicate
	GoalProof           GoalProof
	AuthenticationProof GoalProof
	HumanConfirmed      bool
	Diagnostics         []string
}

// Build produces the oldest sufficient profile versions. Context topology or
// exact authentication success paths require the additive profile versions.
func Build(request BuildRequest) (*Envelope, error) {
	if request.ObservedAt.IsZero() || strings.TrimSpace(request.Goal) == "" || !request.HumanConfirmed {
		return nil, fmt.Errorf("observed time, goal, and human completion are required")
	}
	if request.GoalProof.Matches != 1 || request.GoalProof.Origin != request.GoalPredicate.Origin || request.GoalProof.Path != request.GoalPredicate.Path ||
		request.GoalProof.Role != request.GoalPredicate.Role || normalizedContext(request.GoalProof.Context) != normalizedContext(request.GoalPredicate.Context) ||
		(request.GoalPredicate.Label != "" && request.GoalProof.Label != request.GoalPredicate.Label) {
		return nil, fmt.Errorf("typed goal predicate is not satisfied")
	}
	selections := make([]OutputSelection, len(request.OutputSelections))
	copy(selections, request.OutputSelections)
	sort.Slice(selections, func(i, j int) bool { return selections[i].Key < selections[j].Key })
	request.OutputSelections = selections
	origins := canonicalStrings(request.Origins)
	if err := validateBuildRequest(request, origins); err != nil {
		return nil, err
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("at least one approved origin is required")
	}
	authentication, authVersion, err := buildAuthenticationProfile(request, origins)
	if err != nil {
		return nil, err
	}
	capability, capabilityVersion, err := buildCapabilityProfile(request, origins)
	if err != nil {
		return nil, err
	}
	authBytes, err := json.Marshal(authentication)
	if err != nil {
		return nil, err
	}
	capabilityBytes, err := json.Marshal(capability)
	if err != nil {
		return nil, err
	}
	assessedAt := request.ObservedAt.UTC().Format(time.RFC3339)
	contexts := make(map[string]Context, len(request.Contexts))
	for id, context := range request.Contexts {
		contexts[id] = context
	}
	diagnostics := canonicalStrings(request.Diagnostics)
	return &Envelope{
		Schema: Schema, ObservedAt: assessedAt, Goal: strings.TrimSpace(request.Goal),
		GoalPredicate: request.GoalPredicate, GoalProof: request.GoalProof, HumanConfirmed: true,
		Origins: origins, Contexts: contexts, Bounds: request.Bounds,
		Trace:                 append([]TraceStep(nil), request.Trace...),
		OutputSelections:      selections,
		AuthenticationProfile: authBytes,
		AuthenticationReview: Review{
			Schema: "browsertools.authenticated-profile-review.v1", Kind: "authentication",
			ProfileDigest: digest(authBytes), AssessedAt: assessedAt,
			Decisions: []string{"human_credentials", "human_mfa", "exact_origins", "typed_goal", "human_completion", authVersion},
		},
		CapabilityProfile: capabilityBytes,
		CapabilityReview: Review{
			Schema: "browsertools.authenticated-profile-review.v1", Kind: "capability",
			ProfileDigest: digest(capabilityBytes), AssessedAt: assessedAt,
			Decisions: []string{"reduced_semantics", "selected_trace", "typed_goal", "human_completion", capabilityVersion},
		},
		Diagnostics: diagnostics,
	}, nil
}

func validateBuildRequest(request BuildRequest, origins []string) error {
	if strings.TrimSpace(request.Title) == "" || len(request.Title) > 256 || len(request.Goal) > 1024 {
		return fmt.Errorf("result title or goal is invalid")
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		canonical, err := exactOrigin(origin)
		if err != nil || canonical != origin {
			return fmt.Errorf("result origin is not canonical")
		}
		allowed[origin] = struct{}{}
	}
	if len(request.Contexts) > 64 {
		return fmt.Errorf("result context limit exceeded")
	}
	uniqueDiagnostics := make(map[string]struct{}, len(request.Diagnostics))
	for _, diagnostic := range request.Diagnostics {
		uniqueDiagnostics[diagnostic] = struct{}{}
	}
	if len(uniqueDiagnostics) > 256 {
		return fmt.Errorf("result diagnostic limit exceeded")
	}
	for _, raw := range []string{request.InitialURL, request.DashboardURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed == nil || parsed.Host == "" || disclosurepath.Validate(absoluteEscapedPath(parsed)) != nil {
			return fmt.Errorf("result URL is invalid")
		}
		origin, err := exactOrigin(parsed.Scheme + "://" + parsed.Host)
		if err != nil {
			return err
		}
		if _, ok := allowed[origin]; !ok {
			return fmt.Errorf("result URL origin is not approved")
		}
	}
	dashboard, err := url.Parse(request.DashboardURL)
	if err != nil || dashboard == nil || disclosurepath.Validate(absoluteEscapedPath(dashboard)) != nil {
		return fmt.Errorf("result dashboard URL is invalid")
	}
	dashboardOrigin, err := exactOrigin(dashboard.Scheme + "://" + dashboard.Host)
	if err != nil {
		return err
	}
	dashboardPath := absoluteEscapedPath(dashboard)
	if request.AuthenticationProof.Matches != 1 || request.AuthenticationProof.Origin != dashboardOrigin || request.AuthenticationProof.Path != dashboardPath ||
		!identifier.MatchString(request.AuthenticationProof.Role) || request.AuthenticationProof.Label == "[redacted]" || request.AuthenticationProof.Label == "[untrusted-label]" {
		return fmt.Errorf("authentication success proof does not match the dashboard boundary")
	}
	if _, ok := allowed[request.GoalPredicate.Origin]; !ok || disclosurepath.Validate(request.GoalPredicate.Path) != nil || !identifier.MatchString(request.GoalPredicate.Role) {
		return fmt.Errorf("result goal predicate is invalid")
	}
	if request.Bounds.NavigationTimeoutMS <= 0 || request.Bounds.TotalTimeoutMS < request.Bounds.NavigationTimeoutMS || request.Bounds.MaxRequests <= 0 || request.Bounds.MaxResponseBytes <= 0 || request.Bounds.MaxObservations <= 0 || request.Bounds.MaxCandidates <= 0 || request.Bounds.MaxOutputs <= 0 || request.Bounds.MaxOutputs > 32 {
		return fmt.Errorf("result bounds are invalid")
	}
	if len(request.OutputSelections) > request.Bounds.MaxOutputs || len(request.OutputSelections) > 16 {
		return fmt.Errorf("result output selection bound exceeded")
	}
	seenKeys, seenCandidates := map[string]bool{}, map[string]bool{}
	selectionObservation := 0
	for _, selection := range request.OutputSelections {
		if !identifier.MatchString(selection.Key) || selection.Key == "goal_present" || redact.SensitiveKey(strings.ToLower(selection.Key)) ||
			!candidateIdentifier.MatchString(selection.CandidateID) || seenKeys[selection.Key] || seenCandidates[selection.CandidateID] {
			return fmt.Errorf("result output selection identity is invalid")
		}
		seenKeys[selection.Key], seenCandidates[selection.CandidateID] = true, true
		if selectionObservation == 0 {
			selectionObservation = selection.Observation
		}
		if !outputTypes[selection.Type] || !outputLocatorModes[selection.LocatorMode] || selection.Observation <= 0 || selection.Observation > request.Bounds.MaxObservations || selection.Observation != selectionObservation ||
			selection.Matches != 1 || selection.RoleMatches < 1 || selection.RoleMatches > request.Bounds.MaxCandidates || !outputRoles[selection.Role] || outputControlRoles[selection.Role] ||
			normalizedContext(selection.Context) != normalizedContext(request.GoalProof.Context) {
			return fmt.Errorf("result output selection proof is invalid")
		}
		if context := normalizedContext(selection.Context); context != "main" {
			if _, ok := request.Contexts[context]; !ok {
				return fmt.Errorf("result output context is missing")
			}
		}
		if selection.LocatorMode == "exact_name" {
			if !canonicalPromotableName(selection.Name) {
				return fmt.Errorf("result exact-name output is invalid")
			}
		} else if selection.Name != "" || selection.RoleMatches != 1 {
			return fmt.Errorf("result unique-role output proof is invalid")
		}
	}
	for id, context := range request.Contexts {
		if !identifier.MatchString(id) || id == "main" || (context.Kind != "popup" && context.Kind != "frame") || !identifier.MatchString(context.Parent) {
			return fmt.Errorf("result context is invalid")
		}
		if _, ok := allowed[context.Origin]; !ok {
			return fmt.Errorf("result context origin is not approved")
		}
		if context.Kind == "popup" && (context.Path != "" || context.Name != "") {
			return fmt.Errorf("result popup metadata is invalid")
		}
		if context.Kind == "frame" && (context.Path == "" && context.Name == "") {
			return fmt.Errorf("result frame identity is missing")
		}
		if context.Path != "" && disclosurepath.Validate(context.Path) != nil {
			return fmt.Errorf("result frame path is unsafe")
		}
	}
	for id := range request.Contexts {
		depth, seen, current := 0, map[string]bool{}, id
		for current != "main" {
			if seen[current] || depth >= 4 {
				return fmt.Errorf("result context graph is cyclic or too deep")
			}
			seen[current] = true
			context, ok := request.Contexts[current]
			if !ok {
				return fmt.Errorf("result context parent is missing")
			}
			current, depth = context.Parent, depth+1
		}
	}
	if context := normalizedContext(request.GoalPredicate.Context); context != "main" {
		if _, ok := request.Contexts[context]; !ok {
			return fmt.Errorf("result goal context is missing")
		}
	}
	if context := normalizedContext(request.AuthenticationProof.Context); context != "main" {
		if _, ok := request.Contexts[context]; !ok {
			return fmt.Errorf("authentication success context is missing")
		}
	}
	for _, step := range request.Trace {
		if context := normalizedContext(step.Context); context != "main" {
			if _, ok := request.Contexts[context]; !ok {
				return fmt.Errorf("result trace context is missing")
			}
		}
		if step.OpensContext != "" {
			opened, ok := request.Contexts[step.OpensContext]
			if !ok || opened.Kind != "popup" || normalizedContext(opened.Parent) != normalizedContext(step.Context) || step.Kind != "click" {
				return fmt.Errorf("result popup binding is invalid")
			}
		}
		if step.Kind == "focus_human_input" {
			switch step.InputKind {
			case "identifier", "password":
				if step.ChallengeKind != "" {
					return fmt.Errorf("credential trace carries a challenge kind")
				}
				if _, err := portableLocator(step); err != nil {
					return err
				}
			case "otp":
				if !otpChallengeKinds[step.ChallengeKind] {
					return fmt.Errorf("OTP trace challenge kind is invalid")
				}
				if _, err := portableLocator(step); err != nil {
					return err
				}
			case "mfa":
				if !nonInputChallengeKinds[step.ChallengeKind] {
					return fmt.Errorf("non-input MFA trace challenge kind is invalid")
				}
			default:
				return fmt.Errorf("human input trace kind is invalid")
			}
		}
	}
	return nil
}

func normalizedContext(value string) string {
	if value == "" {
		return "main"
	}
	return value
}

func exactOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("origin must be exact")
	}
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", fmt.Errorf("origin must use HTTPS or loopback HTTP")
	}
	host, port := strings.ToLower(parsed.Hostname()), parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return strings.ToLower(parsed.Scheme) + "://" + host, nil
}

// MarshalDeterministic validates the envelope's digest bindings and returns
// canonical compact JSON with a trailing newline.
func MarshalDeterministic(envelope *Envelope) ([]byte, error) {
	if envelope == nil || envelope.Schema != Schema || !envelope.HumanConfirmed {
		return nil, fmt.Errorf("completed authenticated-authoring envelope is required")
	}
	if digest(envelope.AuthenticationProfile) != envelope.AuthenticationReview.ProfileDigest || digest(envelope.CapabilityProfile) != envelope.CapabilityReview.ProfileDigest {
		return nil, fmt.Errorf("candidate profile digest mismatch")
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func buildAuthenticationProfile(request BuildRequest, origins []string) (map[string]any, string, error) {
	initial, err := cleanURL(request.InitialURL)
	if err != nil {
		return nil, "", fmt.Errorf("initial URL: %w", err)
	}
	sequence := []any{map[string]any{"navigate": initial}}
	credentialSlots := map[string]any{}
	credentialCounter := 0
	hasChallenge := false
	authContextIDs := []string{request.AuthenticationProof.Context}
	for _, step := range request.Trace {
		if step.Phase != "authentication" || step.Kind == "navigate" {
			continue
		}
		authContextIDs = append(authContextIDs, step.Context, step.OpensContext)
		switch step.Kind {
		case "focus_human_input":
			if step.InputKind == "otp" || step.InputKind == "mfa" {
				challenge := map[string]any{"kind": step.ChallengeKind}
				if step.InputKind == "otp" {
					locator, locatorErr := portableLocator(step)
					if locatorErr != nil {
						return nil, "", locatorErr
					}
					challenge["locator"] = locator
					if step.ChallengeKind == "totp" {
						credentialSlots["totp_seed"] = map[string]any{"kind": "totp_seed"}
						challenge["slot"] = "totp_seed"
					}
				}
				putContext(challenge, step.Context)
				sequence = append(sequence, map[string]any{"challenge": challenge})
				hasChallenge = true
				continue
			}
			locator, err := portableLocator(step)
			if err != nil {
				return nil, "", err
			}
			credentialCounter++
			slot := fmt.Sprintf("credential_%d", credentialCounter)
			kind := step.InputKind
			if kind != "password" {
				kind = "identifier"
			}
			credentialSlots[slot] = map[string]any{"kind": kind}
			typeStep := map[string]any{"locator": locator, "slot": slot}
			putContext(typeStep, step.Context)
			sequence = append(sequence, map[string]any{"type_credential": typeStep})
		case "click":
			locator, err := portableLocator(step)
			if err != nil {
				return nil, "", err
			}
			click := map[string]any{"locator": locator}
			putContext(click, step.Context)
			if step.OpensContext != "" {
				click["opensContext"] = step.OpensContext
			}
			sequence = append(sequence, map[string]any{"click": click})
		}
	}
	successLocator := map[string]any{"role": request.AuthenticationProof.Role}
	if request.AuthenticationProof.Label != "" {
		successLocator["name"] = request.AuthenticationProof.Label
	}
	success := map[string]any{"origin": request.AuthenticationProof.Origin, "locator": successLocator}
	authContexts := selectContexts(request.Contexts, authContextIDs...)
	use11 := len(authContexts) > 0 || request.AuthenticationProof.Path != ""
	if request.AuthenticationProof.Path != "" {
		success["path"] = request.AuthenticationProof.Path
	}
	putContext(success, request.AuthenticationProof.Context)
	profileName := "uws.browser-authentication.1.0"
	versionDecision := "uws.browser-authentication.1.0"
	if use11 {
		profileName, versionDecision = "uws.browser-authentication.1.1", "uws.browser-authentication.1.1"
	}
	profile := map[string]any{
		"profile": profileName,
		"info": map[string]any{
			"title": request.Title + " authentication", "applicationOrigins": origins,
			"authenticationOrigins": origins,
		},
		"observationKind": "accessibility_snapshot",
		"evidence":        map[string]any{"learnedAt": request.ObservedAt.UTC().Format(time.RFC3339), "source": "browsertools_authenticated_authoring_value_free"},
		"confidence":      "medium", "expiresAfter": "P14D",
		"verification":    map[string]any{"lastVerifiedAt": request.ObservedAt.UTC().Format(time.RFC3339), "successfulRuns": 1},
		"credentialSlots": credentialSlots,
		"flows": map[string]any{"authenticated_goal": map[string]any{
			"sequence": sequence, "effects": authenticationEffects(hasChallenge), "success": success,
		}},
	}
	if use11 {
		profile["contexts"] = contextMap(authContexts)
	}
	return profile, versionDecision, nil
}

func buildCapabilityProfile(request BuildRequest, origins []string) (map[string]any, string, error) {
	locator := map[string]any{"role": request.GoalPredicate.Role}
	if request.GoalPredicate.Label != "" {
		locator["name"] = request.GoalPredicate.Label
	}
	goalUsesContext := normalizedContext(request.GoalPredicate.Context) != "main"
	use16 := goalUsesContext
	use17 := false
	capabilityContextIDs := []string{request.GoalPredicate.Context}
	sequence := make([]any, 0, len(request.Trace)+1)
	for _, step := range request.Trace {
		if step.Phase != "exploration" {
			continue
		}
		capabilityContextIDs = append(capabilityContextIDs, step.Context, step.OpensContext)
		switch step.Kind {
		case "navigate":
			target, err := cleanURL(step.URL)
			if err != nil {
				return nil, "", fmt.Errorf("exploration navigation: %w", err)
			}
			if normalizedContext(step.Context) == "main" {
				sequence = append(sequence, map[string]any{"navigate": target})
			} else {
				use16 = true
				sequence = append(sequence, map[string]any{"navigate": map[string]any{"url": target, "context": step.Context}})
			}
		case "click":
			portable, err := portableLocator(step)
			if err != nil {
				return nil, "", err
			}
			click := map[string]any{"locator": portable}
			putContext(click, step.Context)
			if normalizedContext(step.Context) != "main" {
				use16 = true
			}
			if step.OpensContext != "" {
				use16 = true
				click["opensContext"] = step.OpensContext
			}
			sequence = append(sequence, map[string]any{"click": click})
		default:
			return nil, "", fmt.Errorf("unsupported exploration trace step %q", step.Kind)
		}
	}
	var wait any = locator
	if goalUsesContext {
		wait = map[string]any{"locator": locator, "context": request.GoalPredicate.Context}
	}
	sequence = append(sequence, map[string]any{"wait_for": wait})
	output := map[string]any{"type": "boolean", "source": "a11y", "locator": locator, "presence": true}
	if goalUsesContext {
		output["context"] = request.GoalPredicate.Context
	}
	originValue := any(origins[0])
	if len(origins) > 1 {
		originValue = origins
	}
	outputs := map[string]any{"goal_present": output}
	selections := append([]OutputSelection(nil), request.OutputSelections...)
	sort.Slice(selections, func(i, j int) bool { return selections[i].Key < selections[j].Key })
	for _, selection := range selections {
		selectedLocator := map[string]any{"role": selection.Role}
		if selection.LocatorMode == "exact_name" {
			selectedLocator["name"] = selection.Name
		}
		selectedOutput := map[string]any{"source": "a11y", "locator": selectedLocator}
		if selection.Type == "presence" {
			selectedOutput["type"], selectedOutput["presence"] = "boolean", true
		} else {
			selectedOutput["type"] = selection.Type
			if selection.Type == "integer" || selection.Type == "number" || selection.Type == "boolean" {
				use17 = true
			}
		}
		if normalizedContext(selection.Context) != "main" {
			selectedOutput["context"] = selection.Context
			use16 = true
		}
		outputs[selection.Key] = selectedOutput
	}
	profileName, versionDecision := "uws.browser.1.5", "uws.browser.1.5"
	if use17 {
		profileName, versionDecision = "uws.browser.1.7", "uws.browser.1.7"
	} else if use16 {
		profileName, versionDecision = "uws.browser.1.6", "uws.browser.1.6"
	}
	profile := map[string]any{
		"profile":         profileName,
		"info":            map[string]any{"title": request.Title + " capability", "origin": originValue, "loginStateRequired": true},
		"observationKind": "accessibility_snapshot",
		"evidence":        map[string]any{"learnedAt": request.ObservedAt.UTC().Format(time.RFC3339), "source": "browsertools_authenticated_authoring_value_free"},
		"confidence":      "medium", "expiresAfter": "P14D",
		"verification": map[string]any{"lastVerifiedAt": request.ObservedAt.UTC().Format(time.RFC3339), "successfulRuns": 1},
		"actions": map[string]any{"reach_authenticated_goal": map[string]any{
			"description": request.Goal,
			"sequence":    sequence,
			"outputs":     outputs,
			"sideEffects": []any{"read_only"}, "confirmationPolicy": map[string]any{"required": false},
		}},
	}
	if use16 {
		profile["contexts"] = contextMap(selectContexts(request.Contexts, capabilityContextIDs...))
	}
	return profile, versionDecision, nil
}

func canonicalPromotableName(value string) bool {
	canonical := strings.Join(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	return value != "" && value != "[redacted]" && value != "[untrusted-label]" && len(value) <= 256 && canonical == value
}

func portableLocator(step TraceStep) (map[string]any, error) {
	if step.Role == "" || step.Label == "[redacted]" || step.Label == "[untrusted-label]" {
		return nil, fmt.Errorf("trace candidate %q has no promotable portable locator", step.CandidateID)
	}
	locator := map[string]any{"role": step.Role}
	if step.Label != "" {
		locator["name"] = step.Label
	}
	return locator, nil
}

func putContext(value map[string]any, contextID string) {
	if contextID != "" && contextID != "main" {
		value["context"] = contextID
	}
}

func contextMap(contexts map[string]Context) map[string]any {
	result := make(map[string]any, len(contexts))
	for id, context := range contexts {
		result[id] = context
	}
	return result
}

func selectContexts(contexts map[string]Context, ids ...string) map[string]Context {
	result := make(map[string]Context)
	for _, id := range ids {
		for current := normalizedContext(id); current != "main"; {
			context, ok := contexts[current]
			if !ok {
				break
			}
			result[current] = context
			current = normalizedContext(context.Parent)
		}
	}
	return result
}

func authenticationEffects(hasChallenge bool) []any {
	if hasChallenge {
		return []any{"establishes_session", "sends_mfa_challenge"}
	}
	return []any{"establishes_session"}
}

func cleanURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || disclosurepath.Validate(absoluteEscapedPath(parsed)) != nil {
		return "", fmt.Errorf("must be an absolute URL")
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	return parsed.String(), nil
}

func absoluteEscapedPath(parsed *url.URL) string {
	if parsed == nil || parsed.EscapedPath() == "" {
		return "/"
	}
	return parsed.EscapedPath()
}

func canonicalStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var (
	identifier             = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
	candidateIdentifier    = regexp.MustCompile(`^candidate-[a-f0-9]{16}$`)
	outputTypes            = map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "presence": true}
	outputLocatorModes     = map[string]bool{"exact_name": true, "unique_role": true}
	outputControlRoles     = map[string]bool{"button": true, "textbox": true, "checkbox": true, "radio": true, "combobox": true, "option": true, "menuitem": true, "tab": true, "switch": true}
	outputRoles            = map[string]bool{"link": true, "dialog": true, "status": true, "alert": true, "heading": true, "img": true, "list": true, "listitem": true, "menu": true, "tabpanel": true, "table": true, "row": true, "cell": true, "region": true, "navigation": true, "article": true, "form": true, "search": true, "group": true}
	otpChallengeKinds      = map[string]bool{"totp": true, "sms_otp": true, "email_otp": true, "voice_otp": true}
	nonInputChallengeKinds = map[string]bool{"push": true, "push_number_match": true, "passkey": true, "security_key": true}
)
