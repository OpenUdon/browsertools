package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	playwright "github.com/mxschmitt/playwright-go"
)

type playwrightRichAcquirer struct {
	driverDirectory string
	engine          Engine
}

// NewPlaywrightRichAcquirer returns the Chromium-only E04 rich backend.
func NewPlaywrightRichAcquirer(driverDirectory string) RichAcquirer {
	return NewPlaywrightEngineRichAcquirer(driverDirectory, EngineChromium)
}

// NewPlaywrightEngineRichAcquirer does not imply that rich cross-engine
// artifacts are portable. The E04 CLI deliberately exposes Chromium only;
// this constructor keeps installed-engine integration independently testable.
func NewPlaywrightEngineRichAcquirer(driverDirectory string, engine Engine) RichAcquirer {
	return &playwrightRichAcquirer{driverDirectory: strings.TrimSpace(driverDirectory), engine: engine}
}

func (a *playwrightRichAcquirer) AcquireRich(ctx context.Context, request RichBackendRequest) (observation RichObservation, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, parseErr := ParseEngine(string(a.engine)); parseErr != nil {
		return RichObservation{}, parseErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return RichObservation{}, ctxErr
	}
	temporaryRoot, err := os.MkdirTemp("", ".browsertools-rich-")
	if err != nil {
		return RichObservation{}, fmt.Errorf("create private rich-capture transaction: %w", err)
	}
	if err := os.Chmod(temporaryRoot, 0o700); err != nil {
		_ = os.RemoveAll(temporaryRoot)
		return RichObservation{}, err
	}
	defer func() {
		if removeErr := os.RemoveAll(temporaryRoot); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove private rich-capture transaction: %w", removeErr))
		}
	}()

	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory: a.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return RichObservation{}, newEngineUnavailable(a.engine, err)
	}
	defer func() {
		if closeErr := pw.Stop(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("stop Playwright driver: %w", closeErr))
		}
	}()
	launchTimeout, err := operationTimeout(ctx, request.Capture.NavigationTimeout)
	if err != nil {
		return RichObservation{}, err
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
		return RichObservation{}, newEngineUnavailable(a.engine, err)
	}
	defer func() {
		if closeErr := browser.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s: %w", a.engine, closeErr))
		}
	}()
	browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(false), ServiceWorkers: playwright.ServiceWorkerPolicyBlock,
		StrictSelectors: playwright.Bool(true),
	})
	if err != nil {
		return RichObservation{}, fmt.Errorf("create ephemeral rich-capture context: %w", err)
	}
	contextOpen := true
	defer func() {
		if contextOpen {
			if closeErr := browserContext.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close ephemeral rich-capture context: %w", closeErr))
			}
		}
	}()
	browserContext.SetDefaultNavigationTimeout(float64(request.Capture.NavigationTimeout.Milliseconds()))
	browserContext.SetDefaultTimeout(float64(request.Capture.NavigationTimeout.Milliseconds()))
	guard := newNetworkGuard(request.Capture)
	if routeErr := installReadOnlyNetworkPolicy(browserContext, guard); routeErr != nil {
		return RichObservation{}, routeErr
	}

	tracePath := filepath.Join(temporaryRoot, "capture.trace.zip")
	harPath := filepath.Join(temporaryRoot, "capture.har")
	traceStarted := false
	harStarted := false
	tracing := browserContext.Tracing()
	defer func() {
		if harStarted {
			err = errors.Join(err, tracing.StopHar())
		}
		if traceStarted {
			err = errors.Join(err, tracing.Stop())
		}
	}()
	if hasPrivateArtifact(request.Artifacts, PrivateArtifactTrace) {
		if err := tracing.Start(playwright.TracingStartOptions{
			Screenshots: playwright.Bool(false), Snapshots: playwright.Bool(true), Sources: playwright.Bool(false),
		}); err != nil {
			return RichObservation{}, fmt.Errorf("start private trace: %w", err)
		}
		traceStarted = true
	}
	if hasPrivateArtifact(request.Artifacts, PrivateArtifactHAR) {
		if err := tracing.StartHar(harPath, playwright.TracingStartHarOptions{
			Content: playwright.HarContentPolicyOmit, Mode: playwright.HarModeMinimal,
		}); err != nil {
			return RichObservation{}, fmt.Errorf("start private HAR: %w", err)
		}
		harStarted = true
	}

	page, err := browserContext.NewPage()
	if err != nil {
		return RichObservation{}, fmt.Errorf("create rich-capture page: %w", err)
	}
	installReadOnlyPagePolicy(browserContext, page, guard)
	navigationTimeout, err := operationTimeout(ctx, request.Capture.NavigationTimeout)
	if err != nil {
		return RichObservation{}, err
	}
	response, navigationErr := page.Goto(request.Capture.URL, playwright.PageGotoOptions{
		Timeout: playwright.Float(navigationTimeout), WaitUntil: playwright.WaitUntilStateLoad,
	})
	if _, policyErr := guard.result(); policyErr != nil {
		return RichObservation{}, policyErr
	}
	if navigationErr != nil {
		return RichObservation{}, fmt.Errorf("navigate rich-capture target: %w", navigationErr)
	}
	if response == nil || response.Status() < 200 || response.Status() >= 400 {
		return RichObservation{}, fmt.Errorf("rich-capture target did not return a successful HTTP response")
	}
	if response.FromServiceWorker() {
		return RichObservation{}, fmt.Errorf("rich-capture target response came from a service worker")
	}
	if err := ctx.Err(); err != nil {
		return RichObservation{}, err
	}

	artifacts := make(map[PrivateArtifactKind]PrivateArtifact, len(request.Artifacts))
	if hasPrivateArtifact(request.Artifacts, PrivateArtifactScreenshot) {
		timeout, err := operationTimeout(ctx, request.Capture.NavigationTimeout)
		if err != nil {
			return RichObservation{}, err
		}
		data, err := page.Screenshot(playwright.PageScreenshotOptions{
			Animations: playwright.ScreenshotAnimationsDisabled, Caret: playwright.ScreenshotCaretHide,
			FullPage: playwright.Bool(false), Scale: playwright.ScreenshotScaleCss,
			Timeout: playwright.Float(timeout), Type: playwright.ScreenshotTypePng,
		})
		if err != nil {
			return RichObservation{}, fmt.Errorf("capture private screenshot: %w", err)
		}
		if len(data) == 0 || int64(len(data)) > request.MaxArtifactBytes {
			return RichObservation{}, fmt.Errorf("private screenshot is empty or exceeds the byte limit")
		}
		artifacts[PrivateArtifactScreenshot] = PrivateArtifact{Kind: PrivateArtifactScreenshot, MediaType: richArtifactMediaType(PrivateArtifactScreenshot), Bytes: data}
	}
	if traceStarted {
		if err := tracing.Stop(tracePath); err != nil {
			return RichObservation{}, fmt.Errorf("stop private trace: %w", err)
		}
		traceStarted = false
	}
	if harStarted {
		if err := tracing.StopHar(); err != nil {
			return RichObservation{}, fmt.Errorf("stop private HAR: %w", err)
		}
		harStarted = false
	}
	finalURL := page.URL()
	if closeErr := browserContext.Close(); closeErr != nil {
		return RichObservation{}, fmt.Errorf("close ephemeral rich-capture context: %w", closeErr)
	}
	contextOpen = false
	summary, policyErr := guard.result()
	if policyErr != nil {
		return RichObservation{}, policyErr
	}
	if hasPrivateArtifact(request.Artifacts, PrivateArtifactTrace) {
		data, err := readBoundedPrivateArtifact(tracePath, request.MaxArtifactBytes)
		if err != nil {
			return RichObservation{}, fmt.Errorf("read private trace: %w", err)
		}
		artifacts[PrivateArtifactTrace] = PrivateArtifact{Kind: PrivateArtifactTrace, MediaType: richArtifactMediaType(PrivateArtifactTrace), Bytes: data}
	}
	if hasPrivateArtifact(request.Artifacts, PrivateArtifactHAR) {
		data, err := readBoundedPrivateArtifact(harPath, request.MaxArtifactBytes)
		if err != nil {
			return RichObservation{}, fmt.Errorf("read private HAR: %w", err)
		}
		artifacts[PrivateArtifactHAR] = PrivateArtifact{Kind: PrivateArtifactHAR, MediaType: richArtifactMediaType(PrivateArtifactHAR), Bytes: data}
	}
	ordered := make([]PrivateArtifact, 0, len(request.Artifacts))
	for _, kind := range request.Artifacts {
		ordered = append(ordered, artifacts[kind])
	}
	return RichObservation{FinalURL: finalURL, Network: summary, Artifacts: ordered}, nil
}

func installReadOnlyNetworkPolicy(browserContext playwright.BrowserContext, guard *networkGuard) error {
	if err := browserContext.Route("**/*", func(route playwright.Route) {
		browserRequest := route.Request()
		facts := requestFacts{URL: browserRequest.URL(), Method: browserRequest.Method(), ResourceType: browserRequest.ResourceType()}
		if browserRequest.IsNavigationRequest() {
			if frame := browserRequest.Frame(); frame != nil && frame.ParentFrame() != nil {
				facts.ChildDocument = true
			}
		}
		if !guard.allowRequest(facts) {
			if abortErr := route.Abort("blockedbyclient"); abortErr != nil {
				guard.record("route_abort", "could not abort a blocked request", abortErr)
			}
			return
		}
		if continueErr := route.Continue(); continueErr != nil {
			guard.record("route_continue", "could not continue an allowed request", continueErr)
		}
	}); err != nil {
		return fmt.Errorf("install exact-origin route: %w", err)
	}
	if err := browserContext.RouteWebSocket("**/*", func(route playwright.WebSocketRoute) {
		guard.blockWebSocket()
		route.Close()
	}); err != nil {
		return fmt.Errorf("install WebSocket blocker: %w", err)
	}
	browserContext.OnDialog(func(dialog playwright.Dialog) {
		guard.blockDialog()
		if dismissErr := dialog.Dismiss(); dismissErr != nil {
			guard.record("dialog_dismiss", "could not dismiss a browser dialog", dismissErr)
		}
	})
	browserContext.OnDownload(func(download playwright.Download) {
		guard.blockDownload()
		if cancelErr := download.Cancel(); cancelErr != nil {
			guard.record("download_cancel", "could not cancel a download", cancelErr)
		}
	})
	browserContext.OnResponse(func(response playwright.Response) {
		value, headerErr := response.HeaderValue("content-length")
		if headerErr != nil {
			guard.record("response_header", "could not inspect response size", headerErr)
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		length, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			guard.record("response_header", "response has an invalid content length", nil)
			return
		}
		guard.observeResponseContentLength(length)
	})
	browserContext.OnRequestFinished(func(browserRequest playwright.Request) {
		sizes, sizeErr := browserRequest.Sizes()
		if sizeErr != nil {
			guard.record("response_size", "could not inspect finished response size", sizeErr)
			return
		}
		if sizes == nil || sizes.ResponseBodySize < 0 || sizes.ResponseHeadersSize < 0 {
			guard.observeFinishedResponse(-1)
			return
		}
		guard.observeFinishedResponse(int64(sizes.ResponseBodySize) + int64(sizes.ResponseHeadersSize))
	})
	return nil
}

func installReadOnlyPagePolicy(browserContext playwright.BrowserContext, page playwright.Page, guard *networkGuard) {
	browserContext.OnPage(func(popup playwright.Page) {
		guard.blockPopup()
		if closeErr := popup.Close(); closeErr != nil {
			guard.record("popup_close", "could not close a popup", closeErr)
		}
	})
	page.OnFileChooser(func(playwright.FileChooser) { guard.blockFileChooser() })
}

func readBoundedPrivateArtifact(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > limit {
		return nil, fmt.Errorf("artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, fmt.Errorf("artifact is empty or exceeds the byte limit")
	}
	return data, nil
}
