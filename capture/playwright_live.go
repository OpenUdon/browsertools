package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

type playwrightAcquirer struct {
	driverDirectory string
}

// NewPlaywrightAcquirer returns the live Chromium backend. It requires the
// pinned driver and Chromium to have been installed explicitly beforehand.
func NewPlaywrightAcquirer(driverDirectory string) Acquirer {
	return &playwrightAcquirer{driverDirectory: strings.TrimSpace(driverDirectory)}
}

func (a *playwrightAcquirer) Acquire(ctx context.Context, request LiveRequest) (observation Observation, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Observation{}, ctxErr
	}
	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory: a.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return Observation{}, fmt.Errorf("start installed Playwright driver: %w", err)
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
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true), ChromiumSandbox: playwright.Bool(true),
		Timeout: playwright.Float(launchTimeout), Env: captureBrowserEnvironment(),
	})
	if err != nil {
		return Observation{}, fmt.Errorf("launch installed Chromium: %w", err)
	}
	defer func() {
		if closeErr := browser.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close Chromium: %w", closeErr))
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

	if routeErr := browserContext.Route("**/*", func(route playwright.Route) {
		browserRequest := route.Request()
		facts := requestFacts{
			URL: browserRequest.URL(), Method: browserRequest.Method(), ResourceType: browserRequest.ResourceType(),
		}
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
	}); routeErr != nil {
		return Observation{}, fmt.Errorf("install exact-origin route: %w", routeErr)
	}
	if routeErr := browserContext.RouteWebSocket("**/*", func(route playwright.WebSocketRoute) {
		guard.blockWebSocket()
		route.Close()
	}); routeErr != nil {
		return Observation{}, fmt.Errorf("install WebSocket blocker: %w", routeErr)
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

	page, err := browserContext.NewPage()
	if err != nil {
		return Observation{}, fmt.Errorf("create capture page: %w", err)
	}
	browserContext.OnPage(func(popup playwright.Page) {
		guard.blockPopup()
		if closeErr := popup.Close(); closeErr != nil {
			guard.record("popup_close", "could not close a popup", closeErr)
		}
	})
	page.OnFileChooser(func(playwright.FileChooser) {
		guard.blockFileChooser()
	})

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
		StructuredData: structured, Network: summary,
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
		"HOME", "LANG", "LC_ALL", "LC_CTYPE", "PATH", "TMPDIR", "TZ", "XDG_RUNTIME_DIR",
	}
	result := make(map[string]string, len(allowed))
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			result[name] = value
		}
	}
	return result
}
