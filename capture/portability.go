package capture

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/profile"
)

const PortabilityVersion = "browsertools.portability-check.v1"

type PortabilityStatus string

const (
	PortabilityPassed      PortabilityStatus = "passed"
	PortabilityFailed      PortabilityStatus = "failed"
	PortabilityUnavailable PortabilityStatus = "unavailable"
)

// PortabilityEngineResult contains only declared profile paths and value-free
// count/type facts. Backend errors, page values, and locator rewrites are not
// part of this artifact.
type PortabilityEngineResult struct {
	Engine     Engine            `json:"engine"`
	Status     PortabilityStatus `json:"status"`
	Diagnostic string            `json:"diagnostic,omitempty"`
	Checks     []LiveCheckItem   `json:"checks"`
}

// PortabilityReport compares the same closed profile-derived probe plan in
// fresh engine contexts, using Chromium as the explicit baseline.
type PortabilityReport struct {
	Version          string                    `json:"version"`
	ProfileDigest    string                    `json:"profileDigest"`
	CheckedAt        string                    `json:"checkedAt"`
	Origin           string                    `json:"origin"`
	Actions          []string                  `json:"actions"`
	OK               bool                      `json:"ok"`
	Engines          []PortabilityEngineResult `json:"engines"`
	ContractPressure []ContextPressure         `json:"contractPressure"`
}

// AcquirerFactory returns one independent acquirer for the selected engine.
// The caller must not return a shared browser context.
type AcquirerFactory func(Engine) Acquirer

// ComparePortability runs the same read-only live check in Chromium and at
// least one alternate engine. Every engine is attempted so missing installs
// become value-free diagnostics rather than silently reducing coverage.
func ComparePortability(ctx context.Context, factory AcquirerFactory, engines []Engine, request LiveCheckRequest) (PortabilityReport, error) {
	if factory == nil {
		return PortabilityReport{}, fmt.Errorf("portability check: acquirer factory is required")
	}
	ordered, err := normalizePortabilityEngines(engines)
	if err != nil {
		return PortabilityReport{}, err
	}
	digest, origin, actions, err := validatePortabilityRequest(request)
	if err != nil {
		return PortabilityReport{}, err
	}
	report := PortabilityReport{
		Version: PortabilityVersion, ProfileDigest: digest,
		CheckedAt: request.Capture.ObservedAt.UTC().Format(time.RFC3339Nano),
		Origin:    origin, Actions: actions, OK: true,
		Engines:          make([]PortabilityEngineResult, 0, len(ordered)),
		ContractPressure: ContractPressure(),
	}
	var baseline []LiveCheckItem
	baselineReady := false
	for _, engine := range ordered {
		result := PortabilityEngineResult{Engine: engine, Checks: []LiveCheckItem{}}
		acquirer := factory(engine)
		if acquirer == nil {
			result.Status = PortabilityUnavailable
			result.Diagnostic = "engine_unavailable"
			report.OK = false
			report.Engines = append(report.Engines, result)
			continue
		}
		checked, checkErr := Check(ctx, acquirer, request)
		if checkErr != nil {
			result.Status = PortabilityFailed
			result.Diagnostic = "browser_observation_failed"
			if IsEngineUnavailable(checkErr) {
				result.Status = PortabilityUnavailable
				result.Diagnostic = "engine_unavailable"
			}
			report.OK = false
			report.Engines = append(report.Engines, result)
			continue
		}
		result.Checks = append([]LiveCheckItem(nil), checked.Checks...)
		if !checked.OK {
			result.Status = PortabilityFailed
			result.Diagnostic = "profile_check_failed"
			report.OK = false
		} else {
			result.Status = PortabilityPassed
		}
		if engine == EngineChromium {
			baseline = result.Checks
			baselineReady = true
		}
		report.Engines = append(report.Engines, result)
	}
	if !baselineReady {
		report.OK = false
		for index := range report.Engines {
			if report.Engines[index].Engine != EngineChromium && report.Engines[index].Status == PortabilityPassed {
				report.Engines[index].Status = PortabilityFailed
				report.Engines[index].Diagnostic = "chromium_baseline_unavailable"
			}
		}
		return report, nil
	}
	for index := range report.Engines {
		item := &report.Engines[index]
		if item.Engine == EngineChromium || item.Status != PortabilityPassed {
			continue
		}
		if !reflect.DeepEqual(item.Checks, baseline) {
			item.Status = PortabilityFailed
			item.Diagnostic = "check_shape_mismatch"
			report.OK = false
		}
	}
	return report, nil
}

func normalizePortabilityEngines(values []Engine) ([]Engine, error) {
	if len(values) < 2 || len(values) > 3 {
		return nil, fmt.Errorf("portability check: select Chromium and at least one of Firefox or WebKit")
	}
	seen := map[Engine]struct{}{}
	for _, value := range values {
		engine, err := ParseEngine(strings.TrimSpace(string(value)))
		if err != nil {
			return nil, fmt.Errorf("portability check: %w", err)
		}
		if _, duplicate := seen[engine]; duplicate {
			return nil, fmt.Errorf("portability check: engine %q is duplicated", engine)
		}
		seen[engine] = struct{}{}
	}
	if _, ok := seen[EngineChromium]; !ok {
		return nil, fmt.Errorf("portability check: Chromium baseline is required")
	}
	if _, firefox := seen[EngineFirefox]; !firefox {
		if _, webkit := seen[EngineWebKit]; !webkit {
			return nil, fmt.Errorf("portability check: Firefox or WebKit is required")
		}
	}
	ordered := make([]Engine, 0, len(seen))
	for _, engine := range []Engine{EngineChromium, EngineFirefox, EngineWebKit} {
		if _, ok := seen[engine]; ok {
			ordered = append(ordered, engine)
		}
	}
	return ordered, nil
}

func validatePortabilityRequest(request LiveCheckRequest) (string, string, []string, error) {
	if request.Profile == nil {
		return "", "", nil, fmt.Errorf("portability check: profile is required")
	}
	value, err := request.Profile.Value()
	if err != nil {
		return "", "", nil, fmt.Errorf("portability check: profile: %w", err)
	}
	if err := profile.Validate(value); err != nil {
		return "", "", nil, fmt.Errorf("portability check: profile: %w", err)
	}
	if len(request.Capture.Probes) != 0 || strings.TrimSpace(request.Capture.ActionHint) != "" {
		return "", "", nil, fmt.Errorf("portability check: caller-supplied probes and action hints are not allowed")
	}
	normalized, origin, err := normalizeLiveRequest(request.Capture)
	if err != nil {
		return "", "", nil, fmt.Errorf("portability check: %w", err)
	}
	for _, allowed := range normalized.AllowedOrigins {
		if !slices.Contains([]string(request.Profile.Info.Origin), allowed) {
			return "", "", nil, fmt.Errorf("portability check: allowed origin %q is outside profile info.origin", allowed)
		}
	}
	actions, err := selectedActions(request.Profile, request.Actions)
	if err != nil {
		return "", "", nil, fmt.Errorf("%s", strings.NewReplacer("live check:", "portability check:").Replace(err.Error()))
	}
	requirements, err := buildCheckRequirements(request.Profile, actions)
	if err != nil {
		return "", "", nil, fmt.Errorf("%s", strings.NewReplacer("live check:", "portability check:").Replace(err.Error()))
	}
	if len(requirements) == 0 {
		return "", "", nil, fmt.Errorf("portability check: selected actions contain no read-only observations")
	}
	digest, err := profileDigest(request.Profile)
	if err != nil {
		return "", "", nil, err
	}
	return digest, origin, actions, nil
}
