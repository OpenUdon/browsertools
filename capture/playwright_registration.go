package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

// NewPlaywrightRegistrationBrowser returns the A08 no-submit headed Chromium
// backend. It exposes only the narrow registrationauthorsession.Browser
// interface and never exports a Playwright handle.
func NewPlaywrightRegistrationBrowser(driverDirectory string) registrationauthorsession.Browser {
	return &playwrightRegistrationBrowser{driverDirectory: strings.TrimSpace(driverDirectory)}
}

type playwrightRegistrationBrowser struct{ driverDirectory string }

func (b *playwrightRegistrationBrowser) Open(ctx context.Context, request registrationauthorsession.BrowserRequest) (_ registrationauthorsession.Session, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateRegistrationBrowserRequest(request); err != nil {
		return nil, err
	}
	pw, err := playwright.Run(&playwright.RunOptions{
		DriverDirectory: b.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, fmt.Errorf("start installed Playwright driver")
	}
	var browser playwright.Browser
	var browserContext playwright.BrowserContext
	cleanup := func() error {
		var result error
		if browserContext != nil {
			if closeErr := browserContext.Close(); closeErr != nil {
				result = errors.Join(result, errors.New("close registration context"))
			}
		}
		if browser != nil {
			if closeErr := browser.Close(); closeErr != nil {
				result = errors.Join(result, errors.New("close registration Chromium"))
			}
		}
		if stopErr := pw.Stop(); stopErr != nil {
			result = errors.Join(result, errors.New("stop Playwright driver"))
		}
		return result
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, cleanup())
		}
	}()

	timeout, err := operationTimeout(ctx, request.NavigationTimeout)
	if err != nil {
		return nil, err
	}
	browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), ChromiumSandbox: playwright.Bool(true),
		Timeout: playwright.Float(timeout), Env: captureBrowserEnvironment(),
		Args: []string{"--deny-permission-prompts"},
	})
	if err != nil {
		return nil, fmt.Errorf("launch installed headed Chromium")
	}
	browserContext, err = browser.NewContext(playwright.BrowserNewContextOptions{
		AcceptDownloads: playwright.Bool(false), NoViewport: playwright.Bool(true),
		ServiceWorkers: playwright.ServiceWorkerPolicyBlock, StrictSelectors: playwright.Bool(true),
		Permissions: []string{},
	})
	if err != nil {
		return nil, fmt.Errorf("create headed registration context")
	}
	if err := browserContext.ClearPermissions(); err != nil {
		return nil, fmt.Errorf("clear registration permissions")
	}
	browserContext.SetDefaultNavigationTimeout(float64(request.NavigationTimeout.Milliseconds()))
	browserContext.SetDefaultTimeout(float64(request.NavigationTimeout.Milliseconds()))
	guard := newRegistrationNetworkGuard(request)
	if err := installRegistrationNetworkPolicy(browserContext, guard); err != nil {
		return nil, err
	}
	page, err := browserContext.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create headed registration page")
	}
	session := &playwrightRegistrationSession{
		pw: pw, browser: browser, browserContext: browserContext,
		page: page, request: request, guard: guard,
	}
	session.installSurfacePolicy()
	browserContext.OnPage(func(popup playwright.Page) {
		guard.block("popup")
		_ = popup.Close()
	})
	if err := guard.beginNavigation(request.URL); err != nil {
		return nil, err
	}
	response, navigationErr := page.Goto(request.URL, playwright.PageGotoOptions{
		Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
	})
	guard.endNavigation()
	if navigationErr != nil || response == nil || response.Status() < 200 || response.Status() >= 400 || response.FromServiceWorker() {
		return nil, fmt.Errorf("initial registration navigation failed")
	}
	if err := guard.err(); err != nil {
		return nil, err
	}
	return session, nil
}

type playwrightRegistrationSession struct {
	closeOnce      sync.Once
	closeErr       error
	closeSummary   registrationauthorsession.NetworkSummary
	pw             *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	page           playwright.Page
	request        registrationauthorsession.BrowserRequest
	guard          *registrationNetworkGuard
}

func (s *playwrightRegistrationSession) Observe(ctx context.Context) (registrationauthorsession.RawObservation, error) {
	if err := s.health(ctx); err != nil {
		return registrationauthorsession.RawObservation{}, err
	}
	origin, path, err := registrationURLFacts(s.page.URL(), s.request.Protocol, s.guard.allowedOrigin)
	if err != nil {
		return registrationauthorsession.RawObservation{}, err
	}
	elements, err := s.page.Locator(authorSemanticSelector).All()
	if err != nil {
		return registrationauthorsession.RawObservation{}, errors.New("enumerate registration accessibility candidates")
	}
	if len(elements) > s.request.MaxCandidates*8 {
		return registrationauthorsession.RawObservation{}, errors.New("registration candidate bound exceeded")
	}
	type group struct {
		role, label string
		matches     int
	}
	groups := make(map[string]*group)
	diagnosticSet := make(map[string]struct{})
	for _, locator := range elements {
		visible, visibleErr := locator.IsVisible()
		if visibleErr != nil || !visible {
			continue
		}
		timeout, timeoutErr := operationTimeout(ctx, s.request.NavigationTimeout)
		if timeoutErr != nil {
			return registrationauthorsession.RawObservation{}, timeoutErr
		}
		snapshot, snapshotErr := locator.AriaSnapshot(playwright.LocatorAriaSnapshotOptions{Timeout: playwright.Float(timeout)})
		if snapshotErr != nil {
			return registrationauthorsession.RawObservation{}, errors.New("observe registration accessibility candidate")
		}
		if len(snapshot) > registrationauthorsession.MaxRawCandidateLabelBytes*8 || !utf8.ValidString(snapshot) {
			return registrationauthorsession.RawObservation{}, errors.New("registration accessibility snapshot is invalid")
		}
		role, label, ok := parseAuthorARIA(snapshot)
		if !ok || !authorPortableRoles[role] {
			diagnosticSet[registrationauthorsession.DiagnosticUnsupportedAccessibleControl] = struct{}{}
			continue
		}
		if len(label) > registrationauthorsession.MaxRawCandidateLabelBytes || !utf8.ValidString(label) {
			return registrationauthorsession.RawObservation{}, errors.New("registration accessibility label is invalid")
		}
		key := role + "\x00" + label
		if existing := groups[key]; existing != nil {
			existing.matches++
			continue
		}
		groups[key] = &group{role: role, label: label, matches: 1}
	}
	if len(groups) > s.request.MaxCandidates {
		return registrationauthorsession.RawObservation{}, errors.New("registration candidate bound exceeded")
	}
	for _, frame := range s.page.Frames() {
		if frame == s.page.MainFrame() {
			continue
		}
		diagnosticSet[registrationauthorsession.DiagnosticAccessibilitySnapshotPartial] = struct{}{}
		frameOrigin, _, frameErr := registrationURLFacts(frame.URL(), s.request.Protocol, s.guard.allowedOrigin)
		if frameErr != nil || frameOrigin != origin {
			diagnosticSet[registrationauthorsession.DiagnosticCrossOriginFrameOmitted] = struct{}{}
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	candidates := make([]registrationauthorsession.RawCandidate, 0, len(keys))
	for _, key := range keys {
		item := groups[key]
		candidates = append(candidates, registrationauthorsession.RawCandidate{
			Role: item.role, Label: item.label, Matches: item.matches,
		})
	}
	diagnostics := make([]string, 0, len(diagnosticSet))
	for code := range diagnosticSet {
		diagnostics = append(diagnostics, code)
	}
	sort.Strings(diagnostics)
	if err := s.health(ctx); err != nil {
		return registrationauthorsession.RawObservation{}, err
	}
	return registrationauthorsession.RawObservation{
		Origin: origin, Path: path, Candidates: candidates, Diagnostics: diagnostics,
	}, nil
}

func (s *playwrightRegistrationSession) Navigate(ctx context.Context, navigation registrationauthorsession.Navigation) error {
	if err := s.health(ctx); err != nil {
		return err
	}
	if _, _, err := registrationURLFacts(navigation.URL, s.request.Protocol, s.guard.allowedOrigin); err != nil {
		return err
	}
	timeout, err := operationTimeout(ctx, s.request.NavigationTimeout)
	if err != nil {
		return err
	}
	switch navigation.Method {
	case "GET":
		if err := s.guard.beginNavigation(navigation.URL); err != nil {
			return err
		}
		response, navigationErr := s.page.Goto(navigation.URL, playwright.PageGotoOptions{
			Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
		})
		s.guard.endNavigation()
		if navigationErr != nil || response == nil || response.Status() < 200 || response.Status() >= 400 || response.FromServiceWorker() {
			return errors.New("registration GET navigation failed")
		}
	case "HEAD":
		if err := s.guard.beginHEAD(navigation.URL); err != nil {
			return err
		}
		response, headErr := s.page.Request().Head(navigation.URL, playwright.APIRequestContextHeadOptions{
			FailOnStatusCode: playwright.Bool(true), MaxRedirects: playwright.Int(0),
			MaxRetries: playwright.Int(0), Timeout: playwright.Float(timeout),
		})
		if headErr != nil || response == nil {
			s.guard.block("head_request")
			return errors.New("registration HEAD request failed")
		}
		if disposeErr := response.Dispose(); disposeErr != nil {
			s.guard.block("head_dispose")
			return errors.New("dispose registration HEAD response")
		}
	default:
		return errors.New("registration navigation method is unsupported")
	}
	return s.health(ctx)
}

func (s *playwrightRegistrationSession) Close(ctx context.Context) (registrationauthorsession.NetworkSummary, error) {
	s.closeOnce.Do(func() {
		s.guard.beginClose()
		if ctx == nil {
			ctx = context.Background()
		}
		if err := ctx.Err(); err != nil {
			s.closeErr = errors.Join(s.closeErr, err)
		}
		if s.browserContext != nil {
			if err := s.browserContext.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, errors.New("close registration context"))
			}
		}
		if s.browser != nil {
			if err := s.browser.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, errors.New("close registration Chromium"))
			}
		}
		if s.pw != nil {
			if err := s.pw.Stop(); err != nil {
				s.closeErr = errors.Join(s.closeErr, errors.New("stop Playwright driver"))
			}
		}
		s.closeSummary, s.closeErr = s.guard.result(s.closeErr)
	})
	return s.closeSummary, s.closeErr
}

func (s *playwrightRegistrationSession) health(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.guard.err()
}

func (s *playwrightRegistrationSession) installSurfacePolicy() {
	s.page.OnDialog(func(dialog playwright.Dialog) { s.guard.block("dialog"); _ = dialog.Dismiss() })
	s.page.OnDownload(func(download playwright.Download) { s.guard.block("download"); _ = download.Cancel() })
	s.page.OnFileChooser(func(playwright.FileChooser) { s.guard.block("file_chooser") })
	s.page.OnCrash(func(playwright.Page) { s.guard.block("page_crash") })
	s.page.OnClose(func(playwright.Page) { s.guard.block("page_close") })
}

type registrationNetworkGuard struct {
	mu               sync.Mutex
	origins          map[string]struct{}
	protocol         string
	core             networkGuardCore
	getRequests      int
	headRequests     int
	navigationActive bool
	closing          bool
}

func newRegistrationNetworkGuard(request registrationauthorsession.BrowserRequest) *registrationNetworkGuard {
	guard := &registrationNetworkGuard{
		origins:  make(map[string]struct{}, len(request.ApprovedOrigins)),
		core:     newNetworkGuardCore(request.MaxRequests, request.MaxResponseBytes),
		protocol: normalizedRegistrationProtocol(request.Protocol),
	}
	for _, origin := range request.ApprovedOrigins {
		guard.origins[origin] = struct{}{}
	}
	return guard
}

func installRegistrationNetworkPolicy(browserContext playwright.BrowserContext, guard *registrationNetworkGuard) error {
	if err := browserContext.Route("**/*", func(route playwright.Route) {
		request := route.Request()
		if !guard.allowBrowser(request.URL(), request.Method(), request.ResourceType(), request.IsNavigationRequest()) {
			_ = route.Abort("blockedbyclient")
			return
		}
		if err := route.Continue(); err != nil {
			guard.block("route_continue")
		}
	}); err != nil {
		return errors.New("install registration network route")
	}
	if err := browserContext.RouteWebSocket("**/*", func(route playwright.WebSocketRoute) {
		guard.block("websocket")
		route.Close()
	}); err != nil {
		return errors.New("install registration WebSocket blocker")
	}
	browserContext.OnResponse(func(response playwright.Response) {
		value, err := response.HeaderValue("content-length")
		if err != nil {
			guard.block("response_header")
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		length, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			guard.block("response_header")
			return
		}
		guard.observeResponseContentLength(length)
	})
	browserContext.OnRequestFinished(func(request playwright.Request) {
		sizes, err := request.Sizes()
		if err != nil || sizes == nil || sizes.ResponseBodySize < 0 {
			guard.block("response_size")
			return
		}
		guard.observeBytes(int64(sizes.ResponseBodySize))
	})
	return nil
}

func (g *registrationNetworkGuard) allowBrowser(rawURL, method, resourceType string, navigation bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing {
		return false
	}
	if !g.core.beginRequest(registrationPolicyError("request_limit")) {
		return false
	}
	allowed := g.allowedResourceURLLocked(rawURL)
	if navigation {
		allowed = g.allowedNavigationURLLocked(rawURL)
	}
	if !allowed {
		g.violate("origin_escape")
		return false
	}
	switch method {
	case "GET":
		g.getRequests++
	case "HEAD":
		g.headRequests++
	default:
		g.violate("mutation_method")
		return false
	}
	if resourceType == "eventsource" || resourceType == "websocket" {
		g.violate("persistent_resource")
		return false
	}
	if navigation && !g.navigationActive {
		g.violate("unexpected_navigation")
		return false
	}
	return g.core.result() == nil
}

func (g *registrationNetworkGuard) beginNavigation(rawURL string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing || g.navigationActive || g.core.result() != nil || !g.allowedNavigationURLLocked(rawURL) {
		return errors.New("registration navigation window unavailable")
	}
	g.navigationActive = true
	return nil
}

func (g *registrationNetworkGuard) endNavigation() {
	g.mu.Lock()
	g.navigationActive = false
	g.mu.Unlock()
}

func (g *registrationNetworkGuard) beginHEAD(rawURL string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing || g.navigationActive || g.core.result() != nil || !g.allowedNavigationURLLocked(rawURL) {
		return errors.New("registration HEAD request unavailable")
	}
	if !g.core.beginRequest(registrationPolicyError("request_limit")) {
		return g.core.result()
	}
	g.headRequests++
	return nil
}

func (g *registrationNetworkGuard) allowedOrigin(origin string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.core.result() != nil {
		return false
	}
	_, ok := g.origins[origin]
	return ok
}

func (g *registrationNetworkGuard) allowedResourceURLLocked(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	origin, err := canonicalAuthorOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return false
	}
	_, ok := g.origins[origin]
	return ok
}

func (g *registrationNetworkGuard) allowedNavigationURLLocked(rawURL string) bool {
	_, origin, _, err := registrationauthorsession.ValidateNavigationURL(g.protocol, rawURL)
	if err != nil {
		return false
	}
	_, ok := g.origins[origin]
	return ok
}

func (g *registrationNetworkGuard) observeResponseContentLength(size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closing {
		g.core.observeResponseContentLength(size, registrationPolicyError("response_limit"))
	}
}

func (g *registrationNetworkGuard) observeBytes(size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closing {
		g.core.observeFinishedResponse(size, registrationPolicyError("response_limit"))
	}
}

func (g *registrationNetworkGuard) block(code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closing {
		g.violate(code)
	}
}

func (g *registrationNetworkGuard) beginClose() {
	g.mu.Lock()
	g.closing = true
	g.mu.Unlock()
}

func (g *registrationNetworkGuard) err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.core.result()
}

func (g *registrationNetworkGuard) result(closeErr error) (registrationauthorsession.NetworkSummary, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	summary := registrationauthorsession.NetworkSummary{
		Requests: g.core.requests, GETRequests: g.getRequests, HEADRequests: g.headRequests,
	}
	return summary, errors.Join(closeErr, g.core.result())
}

func (g *registrationNetworkGuard) violate(code string) {
	g.core.violate(registrationPolicyError(code))
}

func registrationPolicyError(code string) error {
	return &policyError{Code: code, Message: "registration browser policy violation: " + code}
}

func validateRegistrationBrowserRequest(request registrationauthorsession.BrowserRequest) error {
	request.Protocol = normalizedRegistrationProtocol(request.Protocol)
	if request.Protocol == "" {
		return errors.New("registration browser protocol is invalid")
	}
	if request.NavigationTimeout <= 0 || request.NavigationTimeout > registrationauthorsession.DefaultNavigationTimeout*3 ||
		request.TotalTimeout < request.NavigationTimeout || request.TotalTimeout > registrationauthorsession.DefaultTotalTimeout*6 ||
		request.MaxRequests <= 0 || request.MaxRequests > 4096 || request.MaxResponseBytes <= 0 || request.MaxResponseBytes > 128<<20 ||
		request.MaxCandidates <= 0 || request.MaxCandidates > 512 || len(request.ApprovedOrigins) == 0 || len(request.ApprovedOrigins) > 32 {
		return errors.New("invalid registration browser bounds")
	}
	if !sort.StringsAreSorted(request.ApprovedOrigins) {
		return errors.New("registration browser origins are not canonical")
	}
	for index, origin := range request.ApprovedOrigins {
		canonical, err := canonicalAuthorOrigin(origin)
		if err != nil || canonical != origin || index > 0 && request.ApprovedOrigins[index-1] == origin {
			return errors.New("registration browser origin is invalid")
		}
	}
	origin, _, err := registrationURLFacts(request.URL, request.Protocol, func(value string) bool {
		index := sort.SearchStrings(request.ApprovedOrigins, value)
		return index < len(request.ApprovedOrigins) && request.ApprovedOrigins[index] == value
	})
	if err != nil || origin == "" {
		return errors.New("initial registration URL is invalid")
	}
	return nil
}

func registrationURLFacts(rawURL, protocol string, allowed func(string) bool) (string, string, error) {
	protocol = normalizedRegistrationProtocol(protocol)
	_, origin, path, err := registrationauthorsession.ValidateNavigationURL(protocol, rawURL)
	if err != nil || allowed == nil || !allowed(origin) {
		return "", "", errors.New("registration URL origin is not approved")
	}
	return origin, path, nil
}

func normalizedRegistrationProtocol(value string) string {
	switch value {
	case "", registrationauthorsession.ProtocolV1:
		return registrationauthorsession.ProtocolV1
	case registrationauthorsession.ProtocolV2:
		return registrationauthorsession.ProtocolV2
	default:
		return ""
	}
}
