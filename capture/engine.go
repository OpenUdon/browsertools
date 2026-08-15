// Package capture owns explicit, bounded browser acquisition contracts.
// Production workflow execution remains downstream in Udon and Browserdriver.
package capture

import (
	"context"
	"fmt"
	"os"
	"runtime"
)

const (
	DoctorVersion       = "browsertools.playwright-doctor.v1"
	PlaywrightGoVersion = "v0.6201.0"
	PlaywrightVersion   = "1.62.1"
)

// Engine identifies one Playwright browser engine.
type Engine string

const (
	EngineChromium Engine = "chromium"
	EngineFirefox  Engine = "firefox"
	EngineWebKit   Engine = "webkit"
)

// CapabilityDisposition records Browsertools' policy for an upstream
// capability. It does not claim that the capability is installed or already
// implemented by the current milestone.
type CapabilityDisposition string

const (
	CapabilityAdopted  CapabilityDisposition = "adopted"
	CapabilityPrivate  CapabilityDisposition = "private_opt_in"
	CapabilityDeferred CapabilityDisposition = "deferred"
	CapabilityExcluded CapabilityDisposition = "excluded"
)

// Capability is one maintained Browsertools decision about Playwright-Go.
type Capability struct {
	Name        string                `json:"name"`
	Disposition CapabilityDisposition `json:"disposition"`
	Reason      string                `json:"reason"`
}

// CapabilityMatrix returns a stable inventory of the upstream surface that is
// adopted, kept private, deferred, or excluded at the authoring boundary.
func CapabilityMatrix() []Capability {
	return []Capability{
		{Name: "isolated_browser_context", Disposition: CapabilityAdopted, Reason: "one ephemeral context per authoring capture"},
		{Name: "exact_request_routing", Disposition: CapabilityAdopted, Reason: "fail-closed navigation and resource origin policy"},
		{Name: "aria_snapshot", Disposition: CapabilityAdopted, Reason: "primary secret-reviewed locator evidence"},
		{Name: "role_locator_and_actionability", Disposition: CapabilityAdopted, Reason: "portable locator validation without action execution"},
		{Name: "structured_data_discovery", Disposition: CapabilityAdopted, Reason: "bounded output-key and type evidence"},
		{Name: "screenshot_trace_har", Disposition: CapabilityPrivate, Reason: "debug-only raw evidence with explicit retention"},
		{Name: "firefox_webkit_capture", Disposition: CapabilityDeferred, Reason: "Chromium-first runtime parity, then portability comparison"},
		{Name: "popup_iframe_actions", Disposition: CapabilityDeferred, Reason: "browser.1.5 cannot portably address page or frame context"},
		{Name: "codegen_and_inspector_output", Disposition: CapabilityExcluded, Reason: "generated scripts are not a stable closed profile contract"},
		{Name: "cdp_or_existing_browser_attachment", Disposition: CapabilityExcluded, Reason: "lower fidelity and unsafe coupling to operator state"},
		{Name: "persistent_user_profile", Disposition: CapabilityExcluded, Reason: "credentials and storage state remain runtime-owned"},
		{Name: "caller_supplied_javascript", Disposition: CapabilityExcluded, Reason: "portable profiles and capture probes remain closed"},
		{Name: "automatic_upload_download_permission", Disposition: CapabilityExcluded, Reason: "not required for non-side-effect acquisition"},
	}
}

// Runtime starts the installed Playwright driver without installing software.
// Tests use a fake implementation so default verification stays offline.
type Runtime interface {
	Open(context.Context, Engine) (Session, error)
}

// Session is the smallest lifecycle needed to inspect an installed engine.
type Session interface {
	BrowserExecutable() string
	Close() error
}

// DoctorReport is a non-secret installation and capability report.
type DoctorReport struct {
	Version             string       `json:"version"`
	Engine              Engine       `json:"engine"`
	PlaywrightGoVersion string       `json:"playwright_go_version"`
	PlaywrightVersion   string       `json:"playwright_version"`
	DriverReady         bool         `json:"driver_ready"`
	BrowserReady        bool         `json:"browser_ready"`
	BrowserExecutable   string       `json:"browser_executable,omitempty"`
	CapabilityPolicy    []Capability `json:"capability_policy"`
	Error               string       `json:"error,omitempty"`
}

// ParseEngine validates an engine name without silently selecting a fallback.
func ParseEngine(value string) (Engine, error) {
	engine := Engine(value)
	switch engine {
	case EngineChromium, EngineFirefox, EngineWebKit:
		return engine, nil
	default:
		return "", fmt.Errorf("engine must be chromium, firefox, or webkit")
	}
}

// Doctor verifies the installed driver and browser executable without
// launching a browser or contacting the network.
func Doctor(ctx context.Context, browserRuntime Runtime, engine Engine) (report DoctorReport, err error) {
	report = DoctorReport{
		Version: DoctorVersion, Engine: engine, PlaywrightGoVersion: PlaywrightGoVersion,
		PlaywrightVersion: PlaywrightVersion, CapabilityPolicy: CapabilityMatrix(),
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if browserRuntime == nil {
		err = fmt.Errorf("playwright runtime is required")
		report.Error = err.Error()
		return report, err
	}
	if _, parseErr := ParseEngine(string(engine)); parseErr != nil {
		report.Error = parseErr.Error()
		return report, parseErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		report.Error = ctxErr.Error()
		return report, ctxErr
	}
	session, openErr := browserRuntime.Open(ctx, engine)
	if openErr != nil {
		err = fmt.Errorf("playwright driver is unavailable: %w", openErr)
		report.Error = err.Error()
		return report, err
	}
	if session == nil {
		err = fmt.Errorf("playwright driver returned no session")
		report.Error = err.Error()
		return report, err
	}
	report.DriverReady = true
	defer func() {
		if closeErr := session.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("stop playwright driver: %w", closeErr)
			report.Error = err.Error()
		}
	}()

	executable := session.BrowserExecutable()
	if executable == "" {
		err = fmt.Errorf("%s browser executable is unavailable; install the pinned browser first", engine)
		report.Error = err.Error()
		return report, err
	}
	info, statErr := os.Stat(executable)
	if statErr != nil {
		err = fmt.Errorf("%s browser executable is unavailable at %q: %w", engine, executable, statErr)
		report.Error = err.Error()
		return report, err
	}
	if !info.Mode().IsRegular() {
		err = fmt.Errorf("%s browser executable is not a regular file: %q", engine, executable)
		report.Error = err.Error()
		return report, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		err = fmt.Errorf("%s browser executable is not executable: %q", engine, executable)
		report.Error = err.Error()
		return report, err
	}
	report.BrowserReady = true
	report.BrowserExecutable = executable
	return report, nil
}
