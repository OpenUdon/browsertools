package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenUdon/browsertools/authassist"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/uws/browserauthentication"
	playwright "github.com/mxschmitt/playwright-go"
)

type playwrightAuthBrowser struct {
	driverDirectory string
}

// NewPlaywrightAuthBrowser returns the A02 Chromium adapter. The adapter opens
// a visible, non-persistent context and exposes only the narrow authassist
// Session interface; it cannot receive credential values or execute clicks.
func NewPlaywrightAuthBrowser(driverDirectory string) authassist.Browser {
	return &playwrightAuthBrowser{driverDirectory: strings.TrimSpace(driverDirectory)}
}

func (b *playwrightAuthBrowser) Open(ctx context.Context, request authassist.BrowserRequest) (_ authassist.Session, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateAuthBrowserRequest(request); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory: b.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, fmt.Errorf("start installed Playwright driver: %w", err)
	}
	var browser playwright.Browser
	var browserContext playwright.BrowserContext
	cleanup := func() error {
		var cleanupErr error
		if browserContext != nil {
			if closeErr := browserContext.Close(); closeErr != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close ephemeral browser context"))
			}
		}
		if browser != nil {
			if closeErr := browser.Close(); closeErr != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close headed Chromium"))
			}
		}
		if stopErr := pw.Stop(); stopErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("stop Playwright driver"))
		}
		return cleanupErr
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, cleanup())
		}
	}()

	launchTimeout, err := operationTimeout(ctx, request.NavigationTimeout)
	if err != nil {
		return nil, err
	}
	browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), ChromiumSandbox: playwright.Bool(true),
		Timeout: playwright.Float(launchTimeout), Env: captureBrowserEnvironment(),
		Args: []string{"--deny-permission-prompts"},
	})
	if err != nil {
		return nil, fmt.Errorf("launch installed headed Chromium: %w", err)
	}
	browserContext, err = browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(false), NoViewport: playwright.Bool(true),
		ServiceWorkers: playwright.ServiceWorkerPolicyBlock, StrictSelectors: playwright.Bool(true),
		Permissions: []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("create headed ephemeral browser context: %w", err)
	}
	if err := browserContext.ClearPermissions(); err != nil {
		return nil, fmt.Errorf("clear headed browser permissions")
	}
	browserContext.SetDefaultNavigationTimeout(float64(request.NavigationTimeout.Milliseconds()))
	browserContext.SetDefaultTimeout(float64(request.NavigationTimeout.Milliseconds()))
	guard := newAuthNetworkGuard(request)

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
				guard.record("route_abort", "could not abort a blocked authentication request")
			}
			return
		}
		if continueErr := route.Continue(); continueErr != nil {
			guard.record("route_continue", "could not continue an approved authentication request")
		}
	}); routeErr != nil {
		return nil, fmt.Errorf("install authentication exact-origin route: %w", routeErr)
	}
	if routeErr := browserContext.RouteWebSocket("**/*", func(route playwright.WebSocketRoute) {
		guard.block("websocket", "authentication observation blocked a WebSocket")
		route.Close()
	}); routeErr != nil {
		return nil, fmt.Errorf("install authentication WebSocket blocker: %w", routeErr)
	}
	browserContext.OnDialog(func(dialog playwright.Dialog) {
		guard.block("dialog", "authentication observation blocked a browser dialog")
		if dismissErr := dialog.Dismiss(); dismissErr != nil {
			guard.record("dialog_dismiss", "could not dismiss a browser dialog")
		}
	})
	browserContext.OnDownload(func(download playwright.Download) {
		guard.block("download", "authentication observation blocked a download")
		if cancelErr := download.Cancel(); cancelErr != nil {
			guard.record("download_cancel", "could not cancel a download")
		}
	})
	browserContext.OnResponse(func(response playwright.Response) {
		value, headerErr := response.HeaderValue("content-length")
		if headerErr != nil {
			guard.record("response_header", "could not inspect authentication response size")
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		length, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			guard.record("response_header", "authentication response has an invalid content length")
			return
		}
		guard.observeResponseContentLength(length)
	})
	browserContext.OnRequestFinished(func(browserRequest playwright.Request) {
		sizes, sizeErr := browserRequest.Sizes()
		if sizeErr != nil || sizes == nil || sizes.ResponseBodySize < 0 || sizes.ResponseHeadersSize < 0 {
			guard.observeFinishedResponse(-1)
			return
		}
		guard.observeFinishedResponse(int64(sizes.ResponseBodySize) + int64(sizes.ResponseHeadersSize))
	})

	page, err := browserContext.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create headed authentication page: %w", err)
	}
	browserContext.OnPage(func(popup playwright.Page) {
		guard.block("popup", "authentication observation blocked a popup")
		if closeErr := popup.Close(); closeErr != nil {
			guard.record("popup_close", "could not close a popup")
		}
	})
	page.OnFileChooser(func(playwright.FileChooser) {
		guard.block("file_chooser", "authentication observation blocked a file chooser")
	})
	page.OnCrash(func(playwright.Page) {
		guard.block("page_crash", "headed authentication page crashed")
	})
	page.OnClose(func(playwright.Page) {
		guard.pageClosed()
	})

	return &playwrightAuthSession{
		pw: pw, browser: browser, browserContext: browserContext, page: page,
		request: request, guard: guard,
	}, nil
}

type playwrightAuthSession struct {
	closeOnce      sync.Once
	closeErr       error
	pw             *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	page           playwright.Page
	request        authassist.BrowserRequest
	guard          *authNetworkGuard
}

func (s *playwrightAuthSession) Navigate(ctx context.Context, target string) error {
	if err := s.health(ctx); err != nil {
		return err
	}
	if !authURLAllowed(target, s.request.ApprovedOrigins) {
		return fmt.Errorf("navigation target is outside the approved exact origins")
	}
	timeout, err := operationTimeout(ctx, s.request.NavigationTimeout)
	if err != nil {
		return err
	}
	response, navigationErr := s.page.Goto(target, playwright.PageGotoOptions{
		Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
	})
	if navigationErr != nil {
		return fmt.Errorf("headed authentication navigation failed")
	}
	if response == nil || response.Status() < 200 || response.Status() >= 400 {
		return fmt.Errorf("headed authentication navigation did not return a successful response")
	}
	if response.FromServiceWorker() {
		return fmt.Errorf("headed authentication response came from a service worker")
	}
	return s.health(ctx)
}

func (s *playwrightAuthSession) Observe(ctx context.Context, locator *browserauthentication.Locator) (authassist.PageObservation, error) {
	if err := s.health(ctx); err != nil {
		return authassist.PageObservation{}, err
	}
	origin, err := authPageOrigin(s.page.URL(), s.request.ApprovedOrigins)
	if err != nil {
		return authassist.PageObservation{}, err
	}
	observation := authassist.PageObservation{Origin: origin}
	if locator == nil {
		return observation, nil
	}
	if locator.Value != "" {
		return authassist.PageObservation{}, fmt.Errorf("value-based authentication locators are not observable")
	}
	options := playwright.PageGetByRoleOptions{Exact: playwright.Bool(true)}
	if locator.Name != "" {
		options.Name = locator.Name
	}
	matched := s.page.GetByRole(playwright.AriaRole(locator.Role), options)
	if locator.Text != "" {
		matched = matched.Filter(playwright.LocatorFilterOptions{HasText: regexp.MustCompile("^" + regexp.QuoteMeta(locator.Text) + "$")})
	}
	timeout, timeoutErr := operationTimeout(ctx, s.request.NavigationTimeout)
	if timeoutErr != nil {
		return authassist.PageObservation{}, timeoutErr
	}
	if waitErr := matched.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible, Timeout: playwright.Float(timeout),
	}); waitErr != nil {
		return authassist.PageObservation{}, fmt.Errorf("headed authentication locator did not become uniquely visible")
	}
	count, countErr := matched.Count()
	if countErr != nil {
		return authassist.PageObservation{}, fmt.Errorf("headed authentication locator count failed")
	}
	observation.Matches = count
	if err := s.health(ctx); err != nil {
		return authassist.PageObservation{}, err
	}
	return observation, nil
}

func (s *playwrightAuthSession) BeginAuthenticationInteraction(maxPOSTRequests int) error {
	return s.guard.beginInteraction(maxPOSTRequests)
}

func (s *playwrightAuthSession) EndAuthenticationInteraction() (int, error) {
	return s.guard.endInteraction()
}

func (s *playwrightAuthSession) Close() error {
	s.closeOnce.Do(func() {
		s.guard.beginClose()
		if s.browserContext != nil {
			if err := s.browserContext.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, fmt.Errorf("close ephemeral browser context"))
			}
		}
		// Context closure drains page/network activity. Recheck the guard here so
		// a policy violation racing the final value-free observation cannot be
		// lost between that observation and teardown.
		if err := s.guard.result(); err != nil {
			s.closeErr = errors.Join(s.closeErr, err)
		}
		if s.browser != nil {
			if err := s.browser.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, fmt.Errorf("close headed Chromium"))
			}
		}
		if s.pw != nil {
			if err := s.pw.Stop(); err != nil {
				s.closeErr = errors.Join(s.closeErr, fmt.Errorf("stop Playwright driver"))
			}
		}
	})
	return s.closeErr
}

func (s *playwrightAuthSession) health(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.page == nil || s.page.IsClosed() {
		return fmt.Errorf("headed authentication page is closed")
	}
	return s.guard.result()
}

type authNetworkGuard struct {
	mu               sync.Mutex
	allowedOrigins   []string
	maxRequests      int
	maxResponseBytes int64
	requests         int
	responseBytes    int64
	active           bool
	postBudget       int
	postObserved     int
	closing          bool
	violation        error
}

func newAuthNetworkGuard(request authassist.BrowserRequest) *authNetworkGuard {
	return &authNetworkGuard{
		allowedOrigins: append([]string(nil), request.ApprovedOrigins...),
		maxRequests:    request.MaxRequests, maxResponseBytes: request.MaxResponseBytes,
	}
}

func (g *authNetworkGuard) allowRequest(facts requestFacts) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests++
	if g.violation != nil {
		return false
	}
	if g.requests > g.maxRequests {
		g.violateLocked("request_limit", "authentication request limit exceeded")
		return false
	}
	if !authURLAllowed(facts.URL, g.allowedOrigins) {
		g.violateLocked("origin", "authentication request violated the approved exact origins")
		return false
	}
	if facts.ChildDocument {
		g.violateLocked("iframe", "authentication observation blocked a child-frame navigation")
		return false
	}
	switch facts.ResourceType {
	case "document", "stylesheet", "script", "xhr", "fetch", "image", "font":
	case "eventsource", "websocket":
		g.violateLocked("resource_type", "authentication observation blocked a persistent network resource")
		return false
	default:
		return false
	}
	switch facts.Method {
	case "GET", "HEAD", "OPTIONS":
		return true
	case "POST":
		if !g.active || g.postObserved >= g.postBudget {
			g.violateLocked("post_budget", "authentication POST occurred outside its approved step ceiling")
			return false
		}
		g.postObserved++
		return true
	default:
		g.violateLocked("method", "authentication observation blocked an unsupported request method")
		return false
	}
}

func (g *authNetworkGuard) beginInteraction(budget int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.violation != nil {
		return g.violation
	}
	if g.active {
		return fmt.Errorf("another authentication interaction is already armed")
	}
	if budget < 0 || budget > authassist.MaxPOSTRequestsPerStep {
		return fmt.Errorf("authentication POST ceiling is invalid")
	}
	g.active = true
	g.postBudget = budget
	g.postObserved = 0
	return nil
}

func (g *authNetworkGuard) endInteraction() (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.active {
		return 0, fmt.Errorf("no authentication interaction is armed")
	}
	observed := g.postObserved
	g.active = false
	g.postBudget = 0
	g.postObserved = 0
	return observed, g.violation
}

func (g *authNetworkGuard) observeResponseContentLength(length int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if length < 0 || length > g.maxResponseBytes {
		g.violateLocked("response_size", "authentication response exceeds the byte limit")
	}
}

func (g *authNetworkGuard) observeFinishedResponse(bytes int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if bytes < 0 || bytes > 0 && g.responseBytes > g.maxResponseBytes-bytes {
		g.violateLocked("response_size", "authentication responses exceed the byte limit")
		return
	}
	g.responseBytes += bytes
}

func (g *authNetworkGuard) block(code, message string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.violateLocked(code, message)
}

func (g *authNetworkGuard) record(code, message string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.violateLocked(code, message)
}

func (g *authNetworkGuard) pageClosed() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closing {
		g.violateLocked("page_closed", "headed authentication page closed unexpectedly")
	}
}

func (g *authNetworkGuard) beginClose() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closing = true
}

func (g *authNetworkGuard) violateLocked(code, message string) {
	if g.violation == nil {
		g.violation = &policyError{Code: code, Message: message}
	}
}

func (g *authNetworkGuard) result() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.violation
}

func validateAuthBrowserRequest(request authassist.BrowserRequest) error {
	if len(request.ApprovedOrigins) == 0 || len(request.ApprovedOrigins) > 32 {
		return fmt.Errorf("headed authentication browser requires 1 to 32 approved origins")
	}
	if request.NavigationTimeout <= 0 || request.NavigationTimeout > authassist.MaxNavigationTimeout {
		return fmt.Errorf("headed authentication browser has an invalid navigation timeout")
	}
	if request.MaxRequests < 1 || request.MaxRequests > authassist.MaxRequests {
		return fmt.Errorf("headed authentication browser has an invalid request limit")
	}
	if request.MaxResponseBytes < 1 || request.MaxResponseBytes > authassist.MaxResponseBytes {
		return fmt.Errorf("headed authentication browser has an invalid response byte limit")
	}
	previous := ""
	for _, raw := range request.ApprovedOrigins {
		origin, err := validateCaptureOrigin(raw)
		if err != nil || origin != raw || previous >= origin {
			return fmt.Errorf("headed authentication browser origins must be canonical, unique, and sorted")
		}
		previous = origin
	}
	return nil
}

func authURLAllowed(raw string, allowed []string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil {
		return false
	}
	origin, err := profile.OriginOfURL(raw)
	return err == nil && slices.Contains(allowed, origin)
}

func authPageOrigin(raw string, allowed []string) (string, error) {
	if !authURLAllowed(raw, allowed) {
		return "", fmt.Errorf("headed authentication page is outside the approved exact origins")
	}
	origin, err := profile.OriginOfURL(raw)
	if err != nil {
		return "", fmt.Errorf("headed authentication page has an invalid origin")
	}
	return origin, nil
}
