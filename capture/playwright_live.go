package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/profile"
	playwright "github.com/mxschmitt/playwright-go"
)

type playwrightAcquirer struct {
	driverDirectory string
	engine          Engine
}

// NewPlaywrightAcquirer returns the live Chromium backend. It requires the
// pinned driver and Chromium to have been installed explicitly beforehand.
func NewPlaywrightAcquirer(driverDirectory string) Acquirer {
	return NewPlaywrightEngineAcquirer(driverDirectory, EngineChromium)
}

// NewPlaywrightEngineAcquirer returns a headless, read-only backend for one
// explicitly selected engine. It never falls back to another engine.
func NewPlaywrightEngineAcquirer(driverDirectory string, engine Engine) Acquirer {
	return &playwrightAcquirer{driverDirectory: strings.TrimSpace(driverDirectory), engine: engine}
}

func (a *playwrightAcquirer) Acquire(ctx context.Context, request LiveRequest) (observation Observation, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Observation{}, ctxErr
	}
	if _, parseErr := ParseEngine(string(a.engine)); parseErr != nil {
		return Observation{}, parseErr
	}
	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory: a.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return Observation{}, newEngineUnavailable(a.engine, err)
	}
	defer func() {
		if closeErr := pw.Stop(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("stop Playwright driver: %w", closeErr))
		}
	}()

	launchTimeout, err := operationTimeout(ctx, request.NavigationTimeout)
	if err != nil {
		return Observation{}, err
	}
	var browserType playwright.BrowserType
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true), Timeout: playwright.Float(launchTimeout), Env: captureBrowserEnvironment(),
	}
	switch a.engine {
	case EngineChromium:
		browserType = pw.Chromium
		launchOptions.ChromiumSandbox = playwright.Bool(true)
	case EngineFirefox:
		browserType = pw.Firefox
	case EngineWebKit:
		browserType = pw.WebKit
	}
	browser, err := browserType.Launch(launchOptions)
	if err != nil {
		return Observation{}, newEngineUnavailable(a.engine, err)
	}
	defer func() {
		if closeErr := browser.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s: %w", a.engine, closeErr))
		}
	}()

	browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(false),
		ServiceWorkers:  playwright.ServiceWorkerPolicyBlock,
		StrictSelectors: playwright.Bool(true),
	})
	if err != nil {
		return Observation{}, fmt.Errorf("create ephemeral browser context: %w", err)
	}
	contextOpen := true
	defer func() {
		if contextOpen {
			if closeErr := browserContext.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close ephemeral browser context: %w", closeErr))
			}
		}
	}()
	browserContext.SetDefaultNavigationTimeout(float64(request.NavigationTimeout.Milliseconds()))
	browserContext.SetDefaultTimeout(float64(request.NavigationTimeout.Milliseconds()))
	guard := newNetworkGuard(request)
	if policyErr := installReadOnlyNetworkPolicy(browserContext, guard); policyErr != nil {
		return Observation{}, policyErr
	}

	page, err := browserContext.NewPage()
	if err != nil {
		return Observation{}, fmt.Errorf("create capture page: %w", err)
	}
	installReadOnlyPagePolicy(browserContext, page, guard)

	navigationTimeout, err := operationTimeout(ctx, request.NavigationTimeout)
	if err != nil {
		return Observation{}, err
	}
	response, navigationErr := page.Goto(request.URL, playwright.PageGotoOptions{
		Timeout: playwright.Float(navigationTimeout), WaitUntil: playwright.WaitUntilStateLoad,
	})
	if _, policyErr := guard.result(); policyErr != nil {
		return Observation{}, policyErr
	}
	if navigationErr != nil {
		return Observation{}, fmt.Errorf("navigate capture target: %w", navigationErr)
	}
	if response == nil || response.Status() < 200 || response.Status() >= 400 {
		return Observation{}, fmt.Errorf("capture target did not return a successful HTTP response")
	}
	if response.FromServiceWorker() {
		return Observation{}, fmt.Errorf("capture target response came from a service worker")
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}

	ariaTimeout, err := operationTimeout(ctx, request.NavigationTimeout)
	if err != nil {
		return Observation{}, err
	}
	aria, err := page.AriaSnapshot(playwright.PageAriaSnapshotOptions{
		Boxes: playwright.Bool(false), Depth: playwright.Int(request.ARIADepth),
		Mode: playwright.AriaSnapshotModeDefault, Timeout: playwright.Float(ariaTimeout),
	})
	if err != nil {
		return Observation{}, fmt.Errorf("capture ARIA snapshot: %w", err)
	}
	if int64(len(aria)) > request.MaxEvidenceBytes {
		return Observation{}, fmt.Errorf("ARIA snapshot exceeds max evidence bytes")
	}
	structured, err := collectStructuredData(ctx, page, request)
	if err != nil {
		return Observation{}, err
	}
	probeResults, err := runReadOnlyProbes(ctx, page, structured, request)
	if err != nil {
		return Observation{}, err
	}

	finalURL := page.URL()
	if closeErr := browserContext.Close(); closeErr != nil {
		return Observation{}, fmt.Errorf("close ephemeral browser context: %w", closeErr)
	}
	contextOpen = false
	summary, policyErr := guard.result()
	if policyErr != nil {
		return Observation{}, policyErr
	}
	return Observation{
		FinalURL: finalURL, ARIASnapshot: aria,
		StructuredData: structured, Network: summary, ProbeResults: probeResults,
	}, nil
}

func collectStructuredData(ctx context.Context, page playwright.Page, request LiveRequest) ([]json.RawMessage, error) {
	locator := page.Locator(`script[type="application/ld+json"]`)
	count, err := locator.Count()
	if err != nil {
		return nil, fmt.Errorf("inspect JSON-LD documents: %w", err)
	}
	if count > MaxStructuredDocuments {
		return nil, fmt.Errorf("capture found more than %d JSON-LD documents", MaxStructuredDocuments)
	}
	result := make([]json.RawMessage, 0, count)
	total := int64(0)
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		timeout, err := operationTimeout(ctx, request.NavigationTimeout)
		if err != nil {
			return nil, err
		}
		text, err := locator.Nth(index).TextContent(playwright.LocatorTextContentOptions{Timeout: playwright.Float(timeout)})
		if err != nil {
			return nil, fmt.Errorf("read JSON-LD document[%d]: %w", index, err)
		}
		text = strings.TrimSpace(text)
		if text == "" || !json.Valid([]byte(text)) {
			continue
		}
		total += int64(len(text))
		if total > request.MaxEvidenceBytes {
			return nil, fmt.Errorf("structured data exceeds max evidence bytes")
		}
		result = append(result, json.RawMessage(append([]byte(nil), text...)))
	}
	return result, nil
}

func runReadOnlyProbes(ctx context.Context, page playwright.Page, structured []json.RawMessage, request LiveRequest) ([]ProbeResult, error) {
	results := make([]ProbeResult, 0, len(request.Probes))
	for _, probe := range request.Probes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result := ProbeResult{ID: probe.ID}
		switch probe.Kind {
		case ProbeLocator:
			matches, failure := countAccessibilityLocator(page, *probe.Locator)
			result.Matches, result.FailureCode = matches, failure
		case ProbeNavigationWait:
			if *probe.Navigation == profile.NavigationLoad || *probe.Navigation == profile.NavigationDOMContentLoaded {
				result.Reached = true // Goto completed the load state, which implies DOMContentLoaded.
				break
			}
			timeout, err := operationTimeout(ctx, request.NavigationTimeout)
			if err != nil {
				return nil, err
			}
			waitErr := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle, Timeout: playwright.Float(timeout),
			})
			if waitErr == nil {
				result.Reached = true
			} else if ctx.Err() != nil {
				return nil, ctx.Err()
			} else {
				result.FailureCode = "timeout"
			}
		case ProbeOutput:
			result = probeOutputShape(page, structured, probe)
		default:
			result.FailureCode = "unsupported"
		}
		results = append(results, result)
	}
	return results, nil
}

func countAccessibilityLocator(page playwright.Page, locator profile.Locator) (int, string) {
	if locator.Value != "" {
		// Inspecting input values could read credentials. The safe live check
		// deliberately declines value-based locators instead.
		return 0, "unsupported"
	}
	options := playwright.PageGetByRoleOptions{Exact: playwright.Bool(true)}
	if locator.Name != "" {
		options.Name = locator.Name
	}
	matched := page.GetByRole(playwright.AriaRole(locator.Role), options)
	if locator.Text != "" {
		matched = matched.Filter(playwright.LocatorFilterOptions{
			HasText: regexp.MustCompile("^" + regexp.QuoteMeta(locator.Text) + "$"),
		})
	}
	count, err := matched.Count()
	if err != nil {
		return 0, "probe_failed"
	}
	return count, ""
}

func probeOutputShape(page playwright.Page, structured []json.RawMessage, probe Probe) ProbeResult {
	result := ProbeResult{ID: probe.ID}
	output := *probe.Output
	switch output.Source {
	case profile.OutputA11y:
		if output.Locator == nil {
			result.FailureCode = "unsupported"
			return result
		}
		result.Matches, result.FailureCode = countAccessibilityLocator(page, *output.Locator)
		if output.Presence != nil && *output.Presence {
			result.ObservedType = profile.OutputBoolean
		} else if result.Matches > 0 {
			result.ObservedType = profile.OutputString
		}
	case profile.OutputJSONLD:
		property := output.Property
		if property == "" {
			property = probe.OutputKey
		}
		result.Matches, result.ObservedType, result.FailureCode = jsonLDShape(structured, property)
	case profile.OutputMicrodata:
		property := output.Property
		if property == "" {
			property = probe.OutputKey
		}
		selector := fmt.Sprintf(`[itemprop=%s]`, strconv.Quote(property))
		count, err := page.Locator("css=" + selector).Count()
		if err != nil {
			result.FailureCode = "probe_failed"
			return result
		}
		result.Matches = count
		result.ObservedType = textShape(count)
	case profile.OutputCSS:
		selector := output.Selector
		if output.Attribute != "" {
			selector += "[" + output.Attribute + "]"
		}
		count, err := page.Locator("css=" + selector).Count()
		if err != nil {
			result.FailureCode = "probe_failed"
			return result
		}
		result.Matches = count
		result.ObservedType = textShape(count)
	default:
		result.FailureCode = "unsupported"
	}
	return result
}

func textShape(matches int) profile.OutputType {
	switch {
	case matches == 1:
		return profile.OutputString
	case matches > 1:
		return profile.OutputArray
	default:
		return ""
	}
}

func jsonLDShape(documents []json.RawMessage, property string) (int, profile.OutputType, string) {
	var observed profile.OutputType
	matches := 0
	for _, document := range documents {
		decoder := json.NewDecoder(strings.NewReader(string(document)))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return 0, "", "probe_failed"
		}
		for _, object := range jsonLDObjects(value) {
			item, ok := object[property]
			if !ok {
				continue
			}
			matches++
			kind := observedJSONType(item)
			if observed != "" && observed != kind {
				return matches, "", "probe_failed"
			}
			observed = kind
		}
	}
	return matches, observed, ""
}

func jsonLDObjects(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return []map[string]any{typed}
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				result = append(result, object)
			}
		}
		return result
	default:
		return nil
	}
}

func observedJSONType(value any) profile.OutputType {
	switch typed := value.(type) {
	case nil:
		return profile.OutputNull
	case bool:
		return profile.OutputBoolean
	case json.Number:
		if strings.ContainsAny(string(typed), ".eE") {
			return profile.OutputNumber
		}
		return profile.OutputInteger
	case string:
		return profile.OutputString
	case []any:
		return profile.OutputArray
	case map[string]any:
		return profile.OutputObject
	default:
		return ""
	}
}

func operationTimeout(ctx context.Context, maximum time.Duration) (float64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	remaining := maximum
	if deadline, ok := ctx.Deadline(); ok {
		until := time.Until(deadline)
		if until <= 0 {
			return 0, context.DeadlineExceeded
		}
		if until < remaining {
			remaining = until
		}
	}
	if remaining < time.Millisecond {
		return 1, nil
	}
	return float64(remaining.Milliseconds()), nil
}

func captureBrowserEnvironment() map[string]string {
	allowed := []string{
		"DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "HOME", "LANG", "LC_ALL", "LC_CTYPE", "PATH", "TMPDIR", "TZ",
		"WAYLAND_DISPLAY", "XAUTHORITY", "XDG_RUNTIME_DIR",
	}
	result := make(map[string]string, len(allowed))
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			result[name] = value
		}
	}
	return result
}
