// Package authassist observes explicitly authored browser authentication
// profiles while a human completes sign-in in a headed ephemeral browser.
//
// It has no credential input, never asks a browser backend to type or click,
// and retains only exact origins, locator match counts, and bounded request
// counts after every browser context has been destroyed.
package authassist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/browsertools/authreview"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/uws/browserauthentication"
)

const (
	// Version identifies the local value-free assisted-authentication record.
	Version = "browsertools.assisted-authentication.v1"

	DefaultNavigationTimeout = 30 * time.Second
	DefaultTotalTimeout      = 10 * time.Minute
	DefaultMaxRequests       = 512
	DefaultMaxResponseBytes  = int64(32 << 20)

	MaxNavigationTimeout       = time.Minute
	MaxTotalTimeout            = 30 * time.Minute
	MaxRequests                = 1024
	MaxResponseBytes           = int64(64 << 20)
	MaxSelectedFlows           = 8
	MaxPOSTRequestsPerStep     = 32
	MaxAssistedArtifactBytes   = int64(4 << 20)
	assistedObservationKind    = "other"
	assistedObservationSource  = "browsertools_assisted_auth_value_free"
	maximumApprovedOriginCount = 32
)

// Request selects an explicit subset of an existing secret-free profile and
// records the exact origins and per-step POST ceilings approved by the
// operator before a browser is launched.
type Request struct {
	Profile           *authprofile.Profile
	Flows             []string
	ApprovedOrigins   []string
	POSTBudgets       map[string]int
	ObservedAt        time.Time
	NavigationTimeout time.Duration
	TotalTimeout      time.Duration
	MaxRequests       int
	MaxResponseBytes  int64
}

// BrowserRequest contains the complete authority available to one fresh
// headed context. It deliberately contains no URL headers, environment,
// credentials, cookies, storage, scripts, proxy, or browser profile path.
type BrowserRequest struct {
	ApprovedOrigins   []string
	NavigationTimeout time.Duration
	MaxRequests       int
	MaxResponseBytes  int64
}

// Browser opens one new headed ephemeral context for one authentication flow.
type Browser interface {
	Open(context.Context, BrowserRequest) (Session, error)
}

// Session is the closed browser capability used by assisted observation.
// Implementations can navigate, count an authored accessibility locator, and
// temporarily admit an explicitly bounded number of POST requests. There is
// intentionally no type, fill, click, press, evaluate, cookie, or storage API.
type Session interface {
	Navigate(context.Context, string) error
	Observe(context.Context, *browserauthentication.Locator) (PageObservation, error)
	BeginAuthenticationInteraction(maxPOSTRequests int) error
	EndAuthenticationInteraction() (observedPOSTRequests int, err error)
	Close() error
}

// PageObservation is the only page-derived data allowed across the browser
// boundary. Origin is exact and Matches is a scalar count; no URL, text,
// attribute, input value, DOM, screenshot, or accessibility snapshot crosses.
type PageObservation struct {
	Origin  string
	Matches int
}

// Instruction tells the human which already-authored profile step to complete
// directly in the browser. It contains no page-derived content.
type Instruction struct {
	Flow string
	Path string
	Kind string
}

// Operator blocks until the human signals that one browser-side interaction
// is complete. Browsertools never receives the interaction's credential or MFA
// value.
type Operator interface {
	Await(context.Context, Instruction) error
}

// Check is one value-free observation associated with an exact profile path.
type Check struct {
	Path                 string `json:"path"`
	Kind                 string `json:"kind"`
	Origin               string `json:"origin"`
	Matches              int    `json:"matches,omitempty"`
	ApprovedPOSTRequests int    `json:"approvedPostRequests,omitempty"`
	ObservedPOSTRequests int    `json:"observedPostRequests,omitempty"`
	OK                   bool   `json:"ok"`
	Message              string `json:"message"`
}

// FlowReport proves one explicitly selected alternative was observed in its
// own context. The context is closed before the report can be returned.
type FlowReport struct {
	Flow   string  `json:"flow"`
	Checks []Check `json:"checks"`
}

// Bundle binds the resulting UWS profile, ordinary authentication review, and
// value-free headed observation. It is local authoring material, not a session
// artifact or registry payload.
type Bundle struct {
	Version       string              `json:"version"`
	ObservedAt    string              `json:"observedAt"`
	Profile       authprofile.Profile `json:"profile"`
	ProfileDigest string              `json:"profileDigest"`
	Review        authreview.Bundle   `json:"review"`
	Flows         []FlowReport        `json:"flows"`
}

type normalizedRequest struct {
	profile           *authprofile.Profile
	flows             []string
	approvedOrigins   []string
	postBudgets       map[string]int
	observedAt        time.Time
	navigationTimeout time.Duration
	totalTimeout      time.Duration
	maxRequests       int
	maxResponseBytes  int64
}

// Run validates all authority before opening a browser, observes every
// selected flow in an independent context, destroys each context on success or
// failure, and only then constructs the profile and review artifacts.
func Run(ctx context.Context, browser Browser, operator Operator, request Request) (*Bundle, error) {
	if browser == nil || operator == nil {
		return nil, fmt.Errorf("assisted authentication requires a browser and operator")
	}
	normalized, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, normalized.totalTimeout)
	defer cancel()

	reports := make([]FlowReport, 0, len(normalized.flows))
	for _, flowName := range normalized.flows {
		report, err := observeFlow(ctx, browser, operator, normalized, flowName)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}

	resultProfile, err := observedProfile(normalized)
	if err != nil {
		return nil, err
	}
	digest, err := authprofile.Digest(resultProfile)
	if err != nil {
		return nil, err
	}
	review, err := authreview.Build(resultProfile, normalized.observedAt)
	if err != nil {
		return nil, err
	}
	bundle := &Bundle{
		Version: Version, ObservedAt: normalized.observedAt.Format(time.RFC3339),
		Profile: *resultProfile, ProfileDigest: digest, Review: *review, Flows: reports,
	}
	if err := Verify(bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func observeFlow(ctx context.Context, browser Browser, operator Operator, request normalizedRequest, flowName string) (report FlowReport, err error) {
	flow := request.profile.Flows[flowName]
	session, err := browser.Open(ctx, BrowserRequest{
		ApprovedOrigins:   append([]string(nil), request.approvedOrigins...),
		NavigationTimeout: request.navigationTimeout, MaxRequests: request.maxRequests,
		MaxResponseBytes: request.maxResponseBytes,
	})
	if err != nil {
		var closeErr error
		if session != nil {
			closeErr = session.Close()
		}
		return FlowReport{}, errors.Join(
			fmt.Errorf("assisted authentication flow %q: open headed ephemeral context: %w", flowName, err),
			wrapNonNil("destroy partially opened headed ephemeral context", closeErr),
		)
	}
	if session == nil {
		return FlowReport{}, fmt.Errorf("assisted authentication flow %q: browser returned no session", flowName)
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("assisted authentication flow %q: destroy ephemeral context: %w", flowName, closeErr))
		}
	}()

	report = FlowReport{Flow: flowName, Checks: []Check{}}
	for index, step := range flow.Sequence {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return FlowReport{}, ctxErr
		}
		path := stepPath(flowName, index)
		switch {
		case step.Navigate != "":
			if err := session.Navigate(ctx, step.Navigate); err != nil {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: navigation failed: %w", path, err)
			}
			observation, err := session.Observe(ctx, nil)
			if err != nil {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: origin check failed: %w", path, err)
			}
			if err := validatePageObservation(observation, request.approvedOrigins); err != nil {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: %w", path, err)
			}
			report.Checks = append(report.Checks, Check{
				Path: path, Kind: "navigate", Origin: observation.Origin, OK: true,
				Message: "approved navigation remained within the exact origin set",
			})
		case step.TypeCredential != nil:
			check, err := observeInteractive(ctx, session, operator, flowName, path, "type_credential", cloneAuthenticationLocator(&step.TypeCredential.Locator), request.approvedOrigins, request.postBudgets[path])
			if err != nil {
				return FlowReport{}, err
			}
			report.Checks = append(report.Checks, check)
		case step.Click != nil:
			check, err := observeInteractive(ctx, session, operator, flowName, path, "click", cloneAuthenticationLocator(&step.Click.Locator), request.approvedOrigins, request.postBudgets[path])
			if err != nil {
				return FlowReport{}, err
			}
			report.Checks = append(report.Checks, check)
		case step.Challenge != nil:
			check, err := observeInteractive(ctx, session, operator, flowName, path, "challenge", cloneAuthenticationLocator(step.Challenge.Locator), request.approvedOrigins, request.postBudgets[path])
			if err != nil {
				return FlowReport{}, err
			}
			report.Checks = append(report.Checks, check)
		case step.WaitFor != nil:
			observation, err := session.Observe(ctx, cloneAuthenticationLocator(&step.WaitFor.Locator))
			if err != nil {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: value-free wait check failed: %w", path, err)
			}
			if err := validatePageObservation(observation, request.approvedOrigins); err != nil {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: %w", path, err)
			}
			if observation.Matches != 1 {
				return FlowReport{}, fmt.Errorf("assisted authentication %s: declared wait locator did not resolve exactly once", path)
			}
			report.Checks = append(report.Checks, Check{
				Path: path, Kind: "wait_for", Origin: observation.Origin, Matches: observation.Matches, OK: true,
				Message: "declared accessibility locator resolved exactly once",
			})
		default:
			return FlowReport{}, fmt.Errorf("assisted authentication %s: unsupported step shape", path)
		}
	}

	successPath := "flows." + flowName + ".success"
	observation, err := session.Observe(ctx, cloneAuthenticationLocator(&flow.Success.Locator))
	if err != nil {
		return FlowReport{}, fmt.Errorf("assisted authentication %s: value-free success check failed: %w", successPath, err)
	}
	if err := validatePageObservation(observation, request.approvedOrigins); err != nil {
		return FlowReport{}, fmt.Errorf("assisted authentication %s: %w", successPath, err)
	}
	expectedOrigin, err := profile.ParseOrigin(flow.Success.Origin)
	if err != nil || observation.Origin != expectedOrigin {
		return FlowReport{}, fmt.Errorf("assisted authentication %s: current origin does not match the declared success origin", successPath)
	}
	if observation.Matches != 1 {
		return FlowReport{}, fmt.Errorf("assisted authentication %s: declared success locator did not resolve exactly once", successPath)
	}
	report.Checks = append(report.Checks, Check{
		Path: successPath, Kind: "success", Origin: observation.Origin, Matches: observation.Matches, OK: true,
		Message: "declared success origin and accessibility locator matched",
	})
	return report, nil
}

func observeInteractive(ctx context.Context, session Session, operator Operator, flowName, path, kind string, locator *browserauthentication.Locator, approvedOrigins []string, budget int) (Check, error) {
	before, err := session.Observe(ctx, locator)
	if err != nil {
		return Check{}, fmt.Errorf("assisted authentication %s: value-free locator check failed: %w", path, err)
	}
	if err := validatePageObservation(before, approvedOrigins); err != nil {
		return Check{}, fmt.Errorf("assisted authentication %s: %w", path, err)
	}
	if locator != nil && before.Matches != 1 {
		return Check{}, fmt.Errorf("assisted authentication %s: declared locator did not resolve exactly once", path)
	}
	if err := session.BeginAuthenticationInteraction(budget); err != nil {
		return Check{}, fmt.Errorf("assisted authentication %s: arm bounded authentication interaction: %w", path, err)
	}
	operatorErr := operator.Await(ctx, Instruction{Flow: flowName, Path: path, Kind: kind})
	observedPOSTs, endErr := session.EndAuthenticationInteraction()
	if operatorErr != nil || endErr != nil {
		return Check{}, errors.Join(
			wrapNonNil("operator did not complete the browser-side authentication interaction", operatorErr),
			wrapNonNil("bounded authentication interaction failed closed", endErr),
		)
	}
	if observedPOSTs < 0 || observedPOSTs > budget {
		return Check{}, fmt.Errorf("assisted authentication %s: browser reported POST activity outside the declared ceiling", path)
	}
	after, err := session.Observe(ctx, nil)
	if err != nil {
		return Check{}, fmt.Errorf("assisted authentication %s: post-interaction origin check failed: %w", path, err)
	}
	if err := validatePageObservation(after, approvedOrigins); err != nil {
		return Check{}, fmt.Errorf("assisted authentication %s: %w", path, err)
	}
	check := Check{
		Path: path, Kind: kind, Origin: after.Origin, Matches: before.Matches,
		ApprovedPOSTRequests: budget, ObservedPOSTRequests: observedPOSTs, OK: true,
		Message: "operator completed the authored step in the headed browser under the declared POST ceiling",
	}
	return check, nil
}

func validatePageObservation(observation PageObservation, approvedOrigins []string) error {
	origin, err := profile.ParseOrigin(observation.Origin)
	if err != nil || origin != observation.Origin || !slices.Contains(approvedOrigins, origin) {
		return fmt.Errorf("browser reported a page outside the approved exact origins")
	}
	if observation.Matches < 0 {
		return fmt.Errorf("browser reported an invalid negative locator count")
	}
	return nil
}

func cloneAuthenticationLocator(value *browserauthentication.Locator) *browserauthentication.Locator {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	if request.Profile == nil {
		return normalizedRequest{}, fmt.Errorf("assisted authentication profile is required")
	}
	data, err := authprofile.MarshalJSON(request.Profile)
	if err != nil {
		return normalizedRequest{}, fmt.Errorf("assisted authentication profile: %w", err)
	}
	cloned, err := authprofile.Parse(data)
	if err != nil {
		return normalizedRequest{}, fmt.Errorf("assisted authentication profile: %w", err)
	}
	if len(request.Flows) == 0 || len(request.Flows) > MaxSelectedFlows {
		return normalizedRequest{}, fmt.Errorf("assisted authentication requires 1 to %d explicitly selected flows", MaxSelectedFlows)
	}
	flows := make([]string, 0, len(request.Flows))
	seenFlows := map[string]struct{}{}
	for _, raw := range request.Flows {
		name := strings.TrimSpace(raw)
		flow, ok := cloned.Flows[name]
		if !ok {
			return normalizedRequest{}, fmt.Errorf("assisted authentication flow %q is not declared", name)
		}
		if _, duplicate := seenFlows[name]; duplicate {
			return normalizedRequest{}, fmt.Errorf("assisted authentication flow %q is duplicated", name)
		}
		if len(flow.Sequence) == 0 || flow.Sequence[0].Navigate == "" {
			return normalizedRequest{}, fmt.Errorf("assisted authentication flow %q must begin with an explicit navigate step", name)
		}
		if !hasInteractiveStep(flow) {
			return normalizedRequest{}, fmt.Errorf("assisted authentication flow %q has no operator-completed authentication step", name)
		}
		if err := validateValueFreeFlow(name, flow); err != nil {
			return normalizedRequest{}, err
		}
		seenFlows[name] = struct{}{}
		flows = append(flows, name)
	}
	sort.Strings(flows)

	declaredOrigins := authprofile.Origins(cloned)
	approvedOrigins, err := normalizeApprovedOrigins(request.ApprovedOrigins)
	if err != nil {
		return normalizedRequest{}, err
	}
	if !slices.Equal(declaredOrigins, approvedOrigins) {
		return normalizedRequest{}, fmt.Errorf("assisted authentication approved origins must exactly equal the profile's declared origin set")
	}

	budgets := make(map[string]int, len(request.POSTBudgets))
	validBudgetPaths := map[string]struct{}{}
	for _, flowName := range flows {
		for index, step := range cloned.Flows[flowName].Sequence {
			if step.TypeCredential != nil || step.Click != nil || step.Challenge != nil {
				validBudgetPaths[stepPath(flowName, index)] = struct{}{}
			}
		}
	}
	for path, budget := range request.POSTBudgets {
		if _, ok := validBudgetPaths[path]; !ok {
			return normalizedRequest{}, fmt.Errorf("assisted authentication POST budget path %q is not a selected interactive step", path)
		}
		if budget < 1 || budget > MaxPOSTRequestsPerStep {
			return normalizedRequest{}, fmt.Errorf("assisted authentication POST budget for %q must be between 1 and %d", path, MaxPOSTRequestsPerStep)
		}
		budgets[path] = budget
	}

	if request.ObservedAt.IsZero() {
		return normalizedRequest{}, fmt.Errorf("assisted authentication observation time is required")
	}
	navigationTimeout := request.NavigationTimeout
	if navigationTimeout == 0 {
		navigationTimeout = DefaultNavigationTimeout
	}
	if navigationTimeout <= 0 || navigationTimeout > MaxNavigationTimeout {
		return normalizedRequest{}, fmt.Errorf("assisted authentication navigation timeout must be positive and no more than %s", MaxNavigationTimeout)
	}
	totalTimeout := request.TotalTimeout
	if totalTimeout == 0 {
		totalTimeout = DefaultTotalTimeout
	}
	if totalTimeout <= 0 || totalTimeout > MaxTotalTimeout || navigationTimeout > totalTimeout {
		return normalizedRequest{}, fmt.Errorf("assisted authentication timeout must be between the navigation timeout and %s", MaxTotalTimeout)
	}
	maxRequests := request.MaxRequests
	if maxRequests == 0 {
		maxRequests = DefaultMaxRequests
	}
	if maxRequests < 1 || maxRequests > MaxRequests {
		return normalizedRequest{}, fmt.Errorf("assisted authentication max requests must be between 1 and %d", MaxRequests)
	}
	maxResponseBytes := request.MaxResponseBytes
	if maxResponseBytes == 0 {
		maxResponseBytes = DefaultMaxResponseBytes
	}
	if maxResponseBytes < 1 || maxResponseBytes > MaxResponseBytes {
		return normalizedRequest{}, fmt.Errorf("assisted authentication max response bytes must be between 1 and %d", MaxResponseBytes)
	}

	return normalizedRequest{
		profile: cloned, flows: flows, approvedOrigins: approvedOrigins, postBudgets: budgets,
		observedAt: request.ObservedAt.UTC().Round(0), navigationTimeout: navigationTimeout,
		totalTimeout: totalTimeout, maxRequests: maxRequests, maxResponseBytes: maxResponseBytes,
	}, nil
}

func normalizeApprovedOrigins(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maximumApprovedOriginCount {
		return nil, fmt.Errorf("assisted authentication requires 1 to %d approved origins", maximumApprovedOriginCount)
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		origin, err := profile.ParseOrigin(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("assisted authentication approved origin: %w", err)
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopback(parsed.Hostname()))) {
			return nil, fmt.Errorf("assisted authentication approved origins must use HTTPS; HTTP is allowed only for loopback")
		}
		if _, duplicate := seen[origin]; duplicate {
			return nil, fmt.Errorf("assisted authentication approved origin %q is duplicated", origin)
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	sort.Strings(result)
	return result, nil
}

func isLoopback(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateValueFreeFlow(name string, flow browserauthentication.Flow) error {
	validate := func(path string, locator browserauthentication.Locator) error {
		if locator.Value != "" {
			return fmt.Errorf("assisted authentication %s: value-based locators are forbidden because input values are never inspected", path)
		}
		return nil
	}
	for index, step := range flow.Sequence {
		path := stepPath(name, index)
		switch {
		case step.TypeCredential != nil:
			if err := validate(path+".type_credential.locator", step.TypeCredential.Locator); err != nil {
				return err
			}
		case step.Click != nil:
			if err := validate(path+".click.locator", step.Click.Locator); err != nil {
				return err
			}
		case step.Challenge != nil && step.Challenge.Locator != nil:
			if err := validate(path+".challenge.locator", *step.Challenge.Locator); err != nil {
				return err
			}
		case step.WaitFor != nil:
			if err := validate(path+".wait_for.locator", step.WaitFor.Locator); err != nil {
				return err
			}
		}
	}
	return validate("flows."+name+".success.locator", flow.Success.Locator)
}

func hasInteractiveStep(flow browserauthentication.Flow) bool {
	for _, step := range flow.Sequence {
		if step.TypeCredential != nil || step.Click != nil || step.Challenge != nil {
			return true
		}
	}
	return false
}

func observedProfile(request normalizedRequest) (*authprofile.Profile, error) {
	data, err := authprofile.MarshalJSON(request.profile)
	if err != nil {
		return nil, err
	}
	var result authprofile.Profile
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	selected := make(map[string]browserauthentication.Flow, len(request.flows))
	usedSlots := map[string]struct{}{}
	for _, name := range request.flows {
		flow := result.Flows[name]
		selected[name] = flow
		for _, step := range flow.Sequence {
			if step.TypeCredential != nil {
				usedSlots[step.TypeCredential.Slot] = struct{}{}
			}
			if step.Challenge != nil && step.Challenge.Slot != "" {
				usedSlots[step.Challenge.Slot] = struct{}{}
			}
		}
	}
	selectedSlots := make(map[string]browserauthentication.CredentialSlot, len(usedSlots))
	for name := range usedSlots {
		selectedSlots[name] = result.CredentialSlots[name]
	}
	result.Flows = selected
	result.CredentialSlots = selectedSlots
	result.ObservationKind = assistedObservationKind
	result.Evidence = browserauthentication.Evidence{
		LearnedAt: request.observedAt.Format(time.RFC3339), Source: assistedObservationSource,
	}
	result.Verification.LastVerifiedAt = request.observedAt.Format(time.RFC3339)
	result.Verification.SuccessfulRuns = 1
	result.Verification.UIStabilityScore = nil
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode assisted authentication result profile: %w", err)
	}
	validated, err := authprofile.Parse(encoded)
	if err != nil {
		return nil, fmt.Errorf("assisted authentication result profile: %w", err)
	}
	return validated, nil
}

// Verify checks all digest, lifecycle, selected-flow, and value-free report
// invariants without opening a browser.
func Verify(bundle *Bundle) error {
	if bundle == nil || bundle.Version != Version {
		return fmt.Errorf("invalid assisted authentication bundle")
	}
	observedAt, err := time.Parse(time.RFC3339, bundle.ObservedAt)
	if err != nil {
		return fmt.Errorf("assisted authentication observedAt: %w", err)
	}
	if bundle.ObservedAt != observedAt.UTC().Format(time.RFC3339) || bundle.Profile.ObservationKind != assistedObservationKind ||
		bundle.Profile.Evidence.Source != assistedObservationSource || bundle.Profile.Evidence.LearnedAt != bundle.ObservedAt ||
		bundle.Profile.Verification.LastVerifiedAt != bundle.ObservedAt || bundle.Profile.Verification.SuccessfulRuns != 1 ||
		bundle.Profile.Verification.UIStabilityScore != nil {
		return fmt.Errorf("assisted authentication profile observation metadata mismatch")
	}
	digest, err := authprofile.Digest(&bundle.Profile)
	if err != nil {
		return err
	}
	if digest != bundle.ProfileDigest || digest != bundle.Review.ProfileDigest {
		return fmt.Errorf("assisted authentication profile digest mismatch")
	}
	if err := authreview.Verify(&bundle.Review, observedAt); err != nil {
		return fmt.Errorf("assisted authentication review: %w", err)
	}
	expectedReview, err := authreview.Build(&bundle.Profile, observedAt)
	if err != nil {
		return err
	}
	reviewData, err := json.Marshal(bundle.Review)
	if err != nil {
		return err
	}
	expectedReviewData, err := json.Marshal(expectedReview)
	if err != nil {
		return err
	}
	if !slices.Equal(reviewData, expectedReviewData) {
		return fmt.Errorf("assisted authentication review mismatch")
	}
	flowNames := authprofile.SortedFlowNames(&bundle.Profile)
	if len(bundle.Flows) != len(flowNames) {
		return fmt.Errorf("assisted authentication flow report inventory mismatch")
	}
	for index, report := range bundle.Flows {
		flow := bundle.Profile.Flows[report.Flow]
		if report.Flow != flowNames[index] || len(report.Checks) != len(flow.Sequence)+1 {
			return fmt.Errorf("assisted authentication flow report order or checks are invalid")
		}
		for checkIndex, check := range report.Checks {
			if !check.OK || check.Path == "" || check.Kind == "" || check.Message == "" || !slices.Contains(authprofile.Origins(&bundle.Profile), check.Origin) {
				return fmt.Errorf("assisted authentication check is invalid")
			}
			if check.Matches < 0 || check.ApprovedPOSTRequests < 0 || check.ApprovedPOSTRequests > MaxPOSTRequestsPerStep || check.ObservedPOSTRequests < 0 || check.ObservedPOSTRequests > check.ApprovedPOSTRequests {
				return fmt.Errorf("assisted authentication check bounds are invalid")
			}
			if checkIndex == len(flow.Sequence) {
				expectedOrigin, _ := profile.ParseOrigin(flow.Success.Origin)
				if check.Path != "flows."+report.Flow+".success" || check.Kind != "success" || check.Origin != expectedOrigin || check.Matches != 1 ||
					check.ApprovedPOSTRequests != 0 || check.ObservedPOSTRequests != 0 || check.Message != "declared success origin and accessibility locator matched" {
					return fmt.Errorf("assisted authentication success check mismatch")
				}
				continue
			}
			step := flow.Sequence[checkIndex]
			expectedKind, hasLocator, interactive, message := expectedStepCheck(step)
			if check.Path != stepPath(report.Flow, checkIndex) || check.Kind != expectedKind || check.Message != message {
				return fmt.Errorf("assisted authentication step check mismatch")
			}
			if hasLocator && check.Matches != 1 || !hasLocator && check.Matches != 0 {
				return fmt.Errorf("assisted authentication step locator count mismatch")
			}
			if !interactive && (check.ApprovedPOSTRequests != 0 || check.ObservedPOSTRequests != 0) {
				return fmt.Errorf("assisted authentication non-interactive step has POST evidence")
			}
		}
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	if int64(len(data)) > MaxAssistedArtifactBytes {
		return fmt.Errorf("assisted authentication bundle exceeds %d bytes", MaxAssistedArtifactBytes)
	}
	return nil
}

// MarshalJSONIndent verifies and deterministically encodes one local bundle.
func MarshalJSONIndent(bundle *Bundle) ([]byte, error) {
	if err := Verify(bundle); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	if int64(len(data)+1) > MaxAssistedArtifactBytes {
		return nil, fmt.Errorf("assisted authentication formatted bundle exceeds %d bytes", MaxAssistedArtifactBytes)
	}
	return append(data, '\n'), nil
}

func stepPath(flow string, index int) string {
	return fmt.Sprintf("flows.%s.sequence[%d]", flow, index)
}

func wrapNonNil(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func expectedStepCheck(step browserauthentication.Step) (kind string, hasLocator, interactive bool, message string) {
	switch {
	case step.Navigate != "":
		return "navigate", false, false, "approved navigation remained within the exact origin set"
	case step.TypeCredential != nil:
		return "type_credential", true, true, "operator completed the authored step in the headed browser under the declared POST ceiling"
	case step.Click != nil:
		return "click", true, true, "operator completed the authored step in the headed browser under the declared POST ceiling"
	case step.Challenge != nil:
		return "challenge", step.Challenge.Locator != nil, true, "operator completed the authored step in the headed browser under the declared POST ceiling"
	case step.WaitFor != nil:
		return "wait_for", true, false, "declared accessibility locator resolved exactly once"
	default:
		return "", false, false, ""
	}
}
