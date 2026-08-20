package capture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

// NewPlaywrightAuthorBrowser returns the A03 headed Chromium backend. One
// process owns one non-persistent context; no live handle is exported.
func NewPlaywrightAuthorBrowser(driverDirectory string) authorsession.Browser {
	return &playwrightAuthorBrowser{driverDirectory: strings.TrimSpace(driverDirectory)}
}

type playwrightAuthorBrowser struct{ driverDirectory string }

func (b *playwrightAuthorBrowser) Open(ctx context.Context, request authorsession.BrowserRequest) (_ authorsession.Session, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateAuthorBrowserRequest(request); err != nil {
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
				result = errors.Join(result, fmt.Errorf("close author context"))
			}
		}
		if browser != nil {
			if closeErr := browser.Close(); closeErr != nil {
				result = errors.Join(result, fmt.Errorf("close headed Chromium"))
			}
		}
		if stopErr := pw.Stop(); stopErr != nil {
			result = errors.Join(result, fmt.Errorf("stop Playwright driver"))
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
		return nil, fmt.Errorf("create headed author context")
	}
	if err := browserContext.ClearPermissions(); err != nil {
		return nil, fmt.Errorf("clear author permissions")
	}
	browserContext.SetDefaultNavigationTimeout(float64(request.NavigationTimeout.Milliseconds()))
	browserContext.SetDefaultTimeout(float64(request.NavigationTimeout.Milliseconds()))
	guard := newAuthorNetworkGuard(request)
	if err := installAuthorNetworkPolicy(browserContext, guard); err != nil {
		return nil, err
	}
	page, err := browserContext.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create headed author page")
	}
	session := &playwrightAuthorSession{
		pw: pw, browser: browser, browserContext: browserContext, request: request, guard: guard,
		pages: map[string]playwright.Page{"main": page}, pageIDs: map[playwright.Page]string{page: "main"},
		frames: make(map[string]playwright.Frame), frameIDs: make(map[playwright.Frame]string),
		contexts: make(map[string]authorresult.Context),
	}
	session.installSurfacePolicy(page)
	browserContext.OnPage(func(popup playwright.Page) {
		if !session.recordPopupEvent(popup) {
			guard.block("unexpected_popup")
			_ = popup.Close()
			return
		}
		session.installSurfacePolicy(popup)
	})
	if err := guard.beginNavigation(); err != nil {
		return nil, err
	}
	response, navigationErr := page.Goto(request.URL, playwright.PageGotoOptions{
		Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
	})
	guard.endNavigation()
	if navigationErr != nil || response == nil || response.Status() < 200 || response.Status() >= 400 || response.FromServiceWorker() {
		return nil, fmt.Errorf("initial author navigation failed")
	}
	if err := guard.result(); err != nil {
		return nil, err
	}
	return session, nil
}

type playwrightAuthorSession struct {
	mu             sync.Mutex
	closeOnce      sync.Once
	closeErr       error
	pw             *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	request        authorsession.BrowserRequest
	guard          *authorNetworkGuard
	pages          map[string]playwright.Page
	pageIDs        map[playwright.Page]string
	frames         map[string]playwright.Frame
	frameIDs       map[playwright.Frame]string
	contexts       map[string]authorresult.Context
	popupCounter   int
	frameCounter   int
	expectingPopup bool
	popupEvents    []playwright.Page
}

func (s *playwrightAuthorSession) Observe(ctx context.Context, contextID string) (authorsession.RawObservation, error) {
	if err := s.health(ctx); err != nil {
		return authorsession.RawObservation{}, err
	}
	if err := s.discoverFrames(); err != nil {
		return authorsession.RawObservation{}, err
	}
	var targetURL string
	var elements []playwright.Locator
	var err error
	s.mu.Lock()
	page := s.pages[contextID]
	frame := s.frames[contextID]
	s.mu.Unlock()
	switch {
	case page != nil:
		targetURL = page.URL()
		elements, err = page.Locator(authorSemanticSelector).All()
	case frame != nil:
		targetURL = frame.URL()
		elements, err = frame.Locator(authorSemanticSelector).All()
	default:
		return authorsession.RawObservation{}, fmt.Errorf("author context is unavailable")
	}
	if err != nil {
		return authorsession.RawObservation{}, fmt.Errorf("enumerate semantic candidates")
	}
	if len(elements) > s.request.MaxCandidates*8 {
		return authorsession.RawObservation{}, fmt.Errorf("semantic candidate bound exceeded")
	}
	type group struct {
		candidate authorsession.RawCandidate
		locator   playwright.Locator
	}
	groups := make(map[string]*group)
	for _, locator := range elements {
		visible, visibleErr := locator.IsVisible()
		if visibleErr != nil || !visible {
			continue
		}
		snapshot, snapshotErr := locator.AriaSnapshot(playwright.LocatorAriaSnapshotOptions{Timeout: playwright.Float(float64(s.request.NavigationTimeout.Milliseconds()))})
		if snapshotErr != nil {
			return authorsession.RawObservation{}, fmt.Errorf("reduce accessible candidate")
		}
		role, label, ok := parseAuthorARIA(snapshot)
		if !ok || !authorPortableRoles[role] {
			continue
		}
		if captchaPattern.MatchString(strings.ToLower(label)) {
			return authorsession.RawObservation{}, fmt.Errorf("CAPTCHA is unsupported")
		}
		inputKind := authorInputKind(locator, role, label)
		challengeKinds := authorChallengeKinds(inputKind, label)
		targetOrigin := authorTargetOrigin(locator, targetURL)
		key := role + "\x00" + label + "\x00" + inputKind + "\x00" + strings.Join(challengeKinds, ",")
		if existing := groups[key]; existing != nil {
			existing.candidate.Matches++
			if existing.candidate.TargetOrigin != targetOrigin {
				existing.candidate.TargetOrigin = ""
			}
			continue
		}
		groups[key] = &group{candidate: authorsession.RawCandidate{Role: role, Label: label, InputKind: inputKind, ChallengeKinds: challengeKinds, TargetOrigin: targetOrigin, Matches: 1}, locator: locator}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > s.request.MaxCandidates {
		return authorsession.RawObservation{}, fmt.Errorf("semantic candidate bound exceeded")
	}
	candidates := make([]authorsession.RawCandidate, 0, len(keys))
	s.mu.Lock()
	for index, key := range keys {
		item := groups[key]
		backendID := fmt.Sprintf("%s:%03d", contextID, index)
		item.candidate.BackendID = backendID
		candidates = append(candidates, item.candidate)
	}
	contexts := make(map[string]authorresult.Context, len(s.contexts))
	for id, item := range s.contexts {
		contexts[id] = item
	}
	s.mu.Unlock()
	origin, path, err := authorURLFacts(targetURL, s.guard.allowedOrigin)
	if err != nil {
		return authorsession.RawObservation{}, err
	}
	return authorsession.RawObservation{
		Origin: origin, Path: path, Context: contextID, Candidates: candidates,
		Contexts: contexts, Diagnostics: []string{},
	}, s.health(ctx)
}

func (s *playwrightAuthorSession) Focus(ctx context.Context, action authorsession.BrowserAction) error {
	if err := s.health(ctx); err != nil {
		return err
	}
	locator, err := s.resolveCandidate(action)
	if err != nil {
		return err
	}
	if err := locator.Focus(playwright.LocatorFocusOptions{Timeout: playwright.Float(float64(s.request.NavigationTimeout.Milliseconds()))}); err != nil {
		return fmt.Errorf("focus human input candidate")
	}
	return s.health(ctx)
}

// resolveCandidate deliberately ignores the cached observation locator. A
// protocol candidate is authority for one reviewed semantic tuple, not for a
// Playwright Locator whose live query may point at a different node after DOM
// mutation. Re-enumeration immediately before focus/click makes a changed,
// missing, or newly ambiguous target fail closed.
func (s *playwrightAuthorSession) resolveCandidate(action authorsession.BrowserAction) (playwright.Locator, error) {
	if action.BackendID == "" || action.Matches != 1 || action.Role == "" {
		return nil, fmt.Errorf("author candidate authority is invalid")
	}
	if err := s.discoverFrames(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	page, frame := s.pages[normalizedAuthorContext(action.Context)], s.frames[normalizedAuthorContext(action.Context)]
	s.mu.Unlock()
	var targetURL string
	var elements []playwright.Locator
	var err error
	switch {
	case page != nil:
		targetURL = page.URL()
		elements, err = page.Locator(authorSemanticSelector).All()
	case frame != nil:
		targetURL = frame.URL()
		elements, err = frame.Locator(authorSemanticSelector).All()
	default:
		return nil, fmt.Errorf("author candidate context is unavailable")
	}
	if err != nil || len(elements) > s.request.MaxCandidates*8 {
		return nil, fmt.Errorf("revalidate semantic candidate")
	}
	var resolved playwright.Locator
	matches := 0
	for _, locator := range elements {
		visible, visibleErr := locator.IsVisible()
		if visibleErr != nil || !visible {
			continue
		}
		snapshot, snapshotErr := locator.AriaSnapshot(playwright.LocatorAriaSnapshotOptions{Timeout: playwright.Float(float64(s.request.NavigationTimeout.Milliseconds()))})
		if snapshotErr != nil {
			return nil, fmt.Errorf("revalidate accessible candidate")
		}
		role, label, ok := parseAuthorARIA(snapshot)
		if !ok || role != action.Role || label != action.Label || authorInputKind(locator, role, label) != action.InputKind {
			continue
		}
		if authorTargetOrigin(locator, targetURL) != action.TargetOrigin {
			return nil, fmt.Errorf("author candidate target changed")
		}
		matches++
		resolved = locator
	}
	if matches != action.Matches || resolved == nil {
		return nil, fmt.Errorf("author candidate is missing, changed, or ambiguous")
	}
	return resolved, nil
}

func (s *playwrightAuthorSession) Execute(ctx context.Context, action authorsession.BrowserAction) (authorsession.Execution, error) {
	if err := s.health(ctx); err != nil {
		return authorsession.Execution{}, err
	}
	switch action.Kind {
	case "navigate_get":
		if !s.guard.allowedURL(action.URL) {
			return authorsession.Execution{}, fmt.Errorf("navigation origin is not approved")
		}
		if err := s.guard.beginNavigation(); err != nil {
			return authorsession.Execution{}, err
		}
		response, err := s.navigate(ctx, action.Context, action.URL)
		s.guard.endNavigation()
		if err != nil || response == nil || response.Status() < 200 || response.Status() >= 400 || response.FromServiceWorker() {
			return authorsession.Execution{}, fmt.Errorf("approved navigation failed")
		}
		return authorsession.Execution{}, s.health(ctx)
	case "click":
		locator, err := s.resolveCandidate(action)
		if err != nil {
			return authorsession.Execution{}, err
		}
		s.mu.Lock()
		before := make(map[playwright.Page]string, len(s.pageIDs))
		for page := range s.pageIDs {
			before[page] = page.URL()
		}
		s.expectingPopup = true
		s.popupEvents = nil
		s.mu.Unlock()
		if err := s.guard.beginPOST(action.POSTBudget); err != nil {
			s.setExpectingPopup(false)
			return authorsession.Execution{}, err
		}
		click := func() error {
			return locator.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(float64(s.request.NavigationTimeout.Milliseconds()))})
		}
		var clickErr error
		if authorOpensPopup(locator) {
			_, clickErr = s.browserContext.ExpectPage(click, playwright.BrowserContextExpectPageOptions{
				Timeout: playwright.Float(float64(s.request.NavigationTimeout.Milliseconds())),
			})
		} else {
			clickErr = click()
		}
		settleErr := s.settleApprovedClick(ctx, before)
		popupEvents := s.endPopupWindow()
		postObserved, postErr := s.guard.endPOST()
		if clickErr != nil || settleErr != nil || postErr != nil {
			return authorsession.Execution{}, errors.Join(fmt.Errorf("approved click failed"), settleErr, postErr)
		}
		newPages := []playwright.Page{}
		for _, page := range s.browserContext.Pages() {
			if _, existed := before[page]; !existed {
				newPages = append(newPages, page)
			}
		}
		if len(newPages) > 1 || len(popupEvents) > 1 || len(newPages) != len(popupEvents) || len(newPages) == 1 && newPages[0] != popupEvents[0] {
			for _, page := range newPages {
				_ = page.Close()
			}
			return authorsession.Execution{}, fmt.Errorf("approved click opened multiple popups")
		}
		result := authorsession.Execution{POSTObserved: postObserved}
		if len(newPages) == 1 {
			openedID, context, err := s.registerPopup(action.Context, newPages[0])
			if err != nil {
				_ = newPages[0].Close()
				return authorsession.Execution{}, err
			}
			result.OpenedID, result.Opened = openedID, &context
		}
		return result, s.health(ctx)
	default:
		return authorsession.Execution{}, fmt.Errorf("unsupported author action")
	}
}

func (s *playwrightAuthorSession) settleApprovedClick(ctx context.Context, before map[playwright.Page]string) error {
	timeout, err := operationTimeout(ctx, s.request.NavigationTimeout)
	if err != nil {
		return err
	}
	pages := s.browserContext.Pages()
	for _, page := range pages {
		previous, existed := before[page]
		if !existed {
			if err := page.WaitForURL(regexp.MustCompile(`^https?://`), playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
			}); err != nil {
				return fmt.Errorf("wait for approved popup navigation")
			}
		} else if current := page.URL(); current != previous {
			if err := page.WaitForURL(current, playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad,
			}); err != nil {
				return fmt.Errorf("wait for approved page navigation")
			}
		}
	}
	for _, page := range pages {
		if page.IsClosed() {
			return fmt.Errorf("approved click closed a page")
		}
		if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State: playwright.LoadStateLoad, Timeout: playwright.Float(timeout),
		}); err != nil {
			return fmt.Errorf("wait for approved page load")
		}
	}
	return s.health(ctx)
}

func (s *playwrightAuthorSession) AddOrigin(origin string) error {
	return s.guard.addOrigin(origin)
}

func (s *playwrightAuthorSession) Close() error {
	s.closeOnce.Do(func() {
		s.guard.beginClose()
		if s.browserContext != nil {
			if err := s.browserContext.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, fmt.Errorf("close author context"))
			}
		}
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

func (s *playwrightAuthorSession) health(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.guard.result()
}

func (s *playwrightAuthorSession) installSurfacePolicy(page playwright.Page) {
	page.OnDialog(func(dialog playwright.Dialog) { s.guard.block("dialog"); _ = dialog.Dismiss() })
	page.OnDownload(func(download playwright.Download) { s.guard.block("download"); _ = download.Cancel() })
	page.OnFileChooser(func(playwright.FileChooser) { s.guard.block("file_chooser") })
	page.OnCrash(func(playwright.Page) { s.guard.block("page_crash") })
	page.OnClose(func(closed playwright.Page) {
		s.mu.Lock()
		id := s.pageIDs[closed]
		delete(s.pageIDs, closed)
		delete(s.pages, id)
		s.mu.Unlock()
		if id == "main" {
			s.guard.block("page_close")
		} else if id != "" {
			s.guard.block("context_close")
		}
	})
}

func (s *playwrightAuthorSession) recordPopupEvent(page playwright.Page) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.expectingPopup {
		return false
	}
	s.popupEvents = append(s.popupEvents, page)
	return true
}
func (s *playwrightAuthorSession) setExpectingPopup(value bool) {
	s.mu.Lock()
	s.expectingPopup = value
	if !value {
		s.popupEvents = nil
	}
	s.mu.Unlock()
}

func (s *playwrightAuthorSession) endPopupWindow() []playwright.Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expectingPopup = false
	result := append([]playwright.Page(nil), s.popupEvents...)
	s.popupEvents = nil
	return result
}

func (s *playwrightAuthorSession) navigate(ctx context.Context, contextID, target string) (playwright.Response, error) {
	if contextID == "" {
		contextID = "main"
	}
	timeout, err := operationTimeout(ctx, s.request.NavigationTimeout)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	page, frame := s.pages[contextID], s.frames[contextID]
	s.mu.Unlock()
	if page != nil {
		return page.Goto(target, playwright.PageGotoOptions{Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad})
	}
	if frame != nil {
		return frame.Goto(target, playwright.FrameGotoOptions{Timeout: playwright.Float(timeout), WaitUntil: playwright.WaitUntilStateLoad})
	}
	return nil, fmt.Errorf("navigation context is unavailable")
}

func (s *playwrightAuthorSession) registerPopup(parent string, page playwright.Page) (string, authorresult.Context, error) {
	if parent == "" {
		parent = "main"
	}
	origin, _, err := authorURLFacts(page.URL(), s.guard.allowedOrigin)
	if err != nil {
		return "", authorresult.Context{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.popupCounter++
	id := fmt.Sprintf("popup_%d", s.popupCounter)
	context := authorresult.Context{Kind: "popup", Parent: parent, Origin: origin}
	s.pages[id], s.pageIDs[page], s.contexts[id] = page, id, context
	return id, context, nil
}

func (s *playwrightAuthorSession) discoverFrames() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seenIdentity := make(map[string]struct{})
	presentFrames := make(map[playwright.Frame]struct{})
	for pageID, page := range s.pages {
		if page.IsClosed() {
			return fmt.Errorf("author page context is closed")
		}
		if pageID != "main" {
			stored, ok := s.contexts[pageID]
			if !ok || stored.Kind != "popup" {
				return fmt.Errorf("popup context registry is inconsistent")
			}
			origin, _, err := authorURLFacts(page.URL(), s.guard.allowedOrigin)
			if err != nil || origin != stored.Origin {
				return fmt.Errorf("popup context origin changed")
			}
		}
		for _, frame := range page.Frames() {
			presentFrames[frame] = struct{}{}
		}
	}
	for id, frame := range s.frames {
		stored, ok := s.contexts[id]
		if !ok || stored.Kind != "frame" || frame.IsDetached() {
			return fmt.Errorf("frame context is detached or inconsistent")
		}
		if _, present := presentFrames[frame]; !present {
			return fmt.Errorf("frame context is missing")
		}
		parentID, ok := s.frameIDs[frame.ParentFrame()]
		if !ok || parentID != stored.Parent {
			return fmt.Errorf("frame context parent changed")
		}
		origin, path, err := authorURLFacts(frame.URL(), s.guard.allowedOrigin)
		name, nameErr := canonicalAuthorFrameName(frame.Name())
		if err != nil || nameErr != nil || origin != stored.Origin || path != stored.Path || name != stored.Name {
			return fmt.Errorf("frame context identity changed")
		}
		identity := parentID + "\x00" + origin + "\x00" + path + "\x00" + name
		if _, duplicate := seenIdentity[identity]; duplicate {
			return fmt.Errorf("frame identity is ambiguous")
		}
		seenIdentity[identity] = struct{}{}
	}
	for pageID, page := range s.pages {
		main := page.MainFrame()
		s.frameIDs[main] = pageID
		pending := append([]playwright.Frame(nil), page.Frames()...)
		for progress := true; len(pending) > 0 && progress; {
			progress = false
			next := pending[:0]
			for _, frame := range pending {
				if frame == main {
					continue
				}
				if _, exists := s.frameIDs[frame]; exists {
					continue
				}
				parent := frame.ParentFrame()
				parentID, ok := s.frameIDs[parent]
				if !ok {
					next = append(next, frame)
					continue
				}
				origin, path, err := authorURLFacts(frame.URL(), s.guard.allowedOrigin)
				if err != nil {
					return err
				}
				name, err := canonicalAuthorFrameName(frame.Name())
				if err != nil {
					return err
				}
				identity := parentID + "\x00" + origin + "\x00" + path + "\x00" + name
				if _, duplicate := seenIdentity[identity]; duplicate {
					return fmt.Errorf("frame identity is ambiguous")
				}
				seenIdentity[identity] = struct{}{}
				s.frameCounter++
				id := fmt.Sprintf("frame_%d", s.frameCounter)
				context := authorresult.Context{Kind: "frame", Parent: parentID, Origin: origin, Path: path, Name: name}
				s.frames[id], s.frameIDs[frame], s.contexts[id] = frame, id, context
				progress = true
			}
			pending = next
		}
		if len(pending) != 0 {
			return fmt.Errorf("frame graph could not be resolved")
		}
	}
	return nil
}

func normalizedAuthorContext(value string) string {
	if value == "" {
		return "main"
	}
	return value
}

type authorNetworkGuard struct {
	mu               sync.Mutex
	origins          map[string]struct{}
	core             networkGuardCore
	postActive       bool
	postBudget       int
	postObserved     int
	closing          bool
	navigationActive bool
}

func newAuthorNetworkGuard(request authorsession.BrowserRequest) *authorNetworkGuard {
	g := &authorNetworkGuard{
		origins: make(map[string]struct{}),
		core:    newNetworkGuardCore(request.MaxRequests, request.MaxResponseBytes),
	}
	for _, origin := range request.ApprovedOrigins {
		_ = g.addOrigin(origin)
	}
	return g
}

func installAuthorNetworkPolicy(browserContext playwright.BrowserContext, guard *authorNetworkGuard) error {
	if err := browserContext.Route("**/*", func(route playwright.Route) {
		request := route.Request()
		if !guard.allow(request.URL(), request.Method(), request.IsNavigationRequest()) {
			_ = route.Abort("blockedbyclient")
			return
		}
		if err := route.Continue(); err != nil {
			guard.block("route_continue")
		}
	}); err != nil {
		return fmt.Errorf("install author network route")
	}
	if err := browserContext.RouteWebSocket("**/*", func(route playwright.WebSocketRoute) { guard.block("websocket"); route.Close() }); err != nil {
		return fmt.Errorf("install author WebSocket blocker")
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

func (g *authorNetworkGuard) allow(rawURL, method string, navigation ...bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.core.beginRequest(authorPolicyError("request_limit")) {
		return false
	}
	if !g.allowedURLLocked(rawURL) {
		g.violate("origin_escape")
		return false
	}
	if len(navigation) != 0 && navigation[0] && !g.navigationActive {
		g.violate("unexpected_navigation")
		return false
	}
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return true
	case "POST":
		if !g.postActive || g.postObserved >= g.postBudget {
			g.violate("post_budget")
			return false
		}
		g.postObserved++
		return true
	default:
		g.violate("method")
		return false
	}
}

func (g *authorNetworkGuard) allowedURL(rawURL string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.allowedURLLocked(rawURL)
}
func (g *authorNetworkGuard) allowedURLLocked(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	origin, err := canonicalAuthorOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return false
	}
	_, ok := g.origins[origin]
	return ok
}
func (g *authorNetworkGuard) allowedOrigin(origin string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.core.result() != nil {
		return false
	}
	canonical, err := canonicalAuthorOrigin(origin)
	if err != nil {
		return false
	}
	_, ok := g.origins[canonical]
	return ok
}
func (g *authorNetworkGuard) addOrigin(origin string) error {
	canonical, err := canonicalAuthorOrigin(origin)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.core.result(); err != nil {
		return err
	}
	g.origins[canonical] = struct{}{}
	return nil
}
func (g *authorNetworkGuard) beginPOST(budget int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.core.result() != nil || g.postActive || g.navigationActive || budget < 0 || budget > 32 {
		return fmt.Errorf("POST window unavailable")
	}
	g.postActive, g.postBudget, g.postObserved, g.navigationActive = true, budget, 0, true
	return nil
}
func (g *authorNetworkGuard) endPOST() (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	observed := g.postObserved
	g.postActive, g.postBudget, g.postObserved, g.navigationActive = false, 0, 0, false
	return observed, g.core.result()
}
func (g *authorNetworkGuard) beginNavigation() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.core.result() != nil || g.navigationActive {
		return fmt.Errorf("navigation window unavailable")
	}
	g.navigationActive = true
	return nil
}
func (g *authorNetworkGuard) endNavigation() {
	g.mu.Lock()
	g.navigationActive = false
	g.mu.Unlock()
}
func (g *authorNetworkGuard) observeBytes(size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.core.observeFinishedResponse(size, authorPolicyError("response_limit"))
}
func (g *authorNetworkGuard) observeResponseContentLength(size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.core.observeResponseContentLength(size, authorPolicyError("response_limit"))
}
func (g *authorNetworkGuard) block(code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closing {
		g.violate(code)
	}
}
func (g *authorNetworkGuard) beginClose()   { g.mu.Lock(); g.closing = true; g.mu.Unlock() }
func (g *authorNetworkGuard) result() error { g.mu.Lock(); defer g.mu.Unlock(); return g.core.result() }
func (g *authorNetworkGuard) violate(code string) {
	g.core.violate(authorPolicyError(code))
}

func authorPolicyError(code string) error {
	return &policyError{Code: code, Message: fmt.Sprintf("author browser policy violation: %s", code)}
}

func validateAuthorBrowserRequest(request authorsession.BrowserRequest) error {
	if request.NavigationTimeout <= 0 || request.TotalTimeout < request.NavigationTimeout || request.MaxRequests <= 0 || request.MaxResponseBytes <= 0 || request.MaxCandidates <= 0 || len(request.ApprovedOrigins) == 0 {
		return fmt.Errorf("invalid author browser bounds")
	}
	guard := newAuthorNetworkGuard(request)
	if !guard.allowedURL(request.URL) {
		return fmt.Errorf("initial URL is not approved")
	}
	return nil
}

func authorURLFacts(rawURL string, allowed func(string) bool) (string, string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil {
		return "", "", fmt.Errorf("author page URL is invalid")
	}
	origin, err := canonicalAuthorOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil || !allowed(origin) {
		return "", "", fmt.Errorf("author page origin is not approved")
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if strings.ContainsAny(path, "?#\\") {
		return "", "", fmt.Errorf("author page path is unsafe")
	}
	return origin, path, nil
}

func canonicalAuthorOrigin(raw string) (string, error) {
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
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
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

func parseAuthorARIA(snapshot string) (string, string, bool) {
	line := strings.TrimSpace(strings.SplitN(snapshot, "\n", 2)[0])
	match := authorARIALine.FindStringSubmatch(line)
	if match == nil {
		return "", "", false
	}
	label := ""
	if match[2] != "" {
		decoded, err := strconv.Unquote("\"" + match[2] + "\"")
		if err != nil {
			return "", "", false
		}
		label = decoded
	}
	return match[1], label, true
}

func authorInputKind(locator playwright.Locator, role, label string) string {
	lower := strings.ToLower(label)
	if role == "status" || role == "dialog" {
		if strings.Contains(lower, "approve") || strings.Contains(lower, "check your phone") || strings.Contains(lower, "verification request") {
			return "mfa"
		}
		return ""
	}
	if role != "textbox" {
		return ""
	}
	typeValue, _ := locator.GetAttribute("type")
	autocomplete, _ := locator.GetAttribute("autocomplete")
	typeValue, autocomplete = strings.ToLower(typeValue), strings.ToLower(autocomplete)
	switch {
	case typeValue == "password" || strings.Contains(lower, "password"):
		return "password"
	case autocomplete == "one-time-code" || strings.Contains(lower, "one-time") || strings.Contains(lower, "verification code") || strings.Contains(lower, "otp"):
		return "otp"
	case autocomplete == "username" || autocomplete == "email" || strings.Contains(lower, "username") || strings.Contains(lower, "email"):
		return "identifier"
	default:
		return ""
	}
}

func authorChallengeKinds(inputKind, label string) []string {
	lower := strings.ToLower(label)
	if inputKind == "otp" {
		switch {
		case strings.Contains(lower, "authenticator") || strings.Contains(lower, "totp"):
			return []string{"totp"}
		case strings.Contains(lower, "text") || strings.Contains(lower, "sms"):
			return []string{"sms_otp"}
		case strings.Contains(lower, "email"):
			return []string{"email_otp"}
		case strings.Contains(lower, "voice") || strings.Contains(lower, "call"):
			return []string{"voice_otp"}
		}
	}
	if inputKind == "mfa" {
		switch {
		case strings.Contains(lower, "number"):
			return []string{"push_number_match"}
		case strings.Contains(lower, "passkey"):
			return []string{"passkey"}
		case strings.Contains(lower, "security key"):
			return []string{"security_key"}
		case strings.Contains(lower, "push") || strings.Contains(lower, "phone"):
			return []string{"push"}
		}
	}
	return nil
}

func canonicalAuthorFrameName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", nil
	}
	reduced := authorsession.ReduceAccessibilityLabel(name)
	if reduced.Reason != authorsession.LabelReasonUnchanged || reduced.Value != name {
		return "", fmt.Errorf("frame name is not canonical disclosure-safe text")
	}
	return name, nil
}

func authorTargetOrigin(locator playwright.Locator, currentURL string) string {
	href, err := locator.GetAttribute("href")
	if err != nil || strings.TrimSpace(href) == "" {
		return ""
	}
	base, err := url.Parse(currentURL)
	if err != nil {
		return ""
	}
	target, err := base.Parse(href)
	if err != nil || target.Host == "" || target.User != nil {
		return ""
	}
	origin, err := canonicalAuthorOrigin(target.Scheme + "://" + target.Host)
	if err != nil {
		return ""
	}
	return origin
}

func authorOpensPopup(locator playwright.Locator) bool {
	for _, attribute := range []string{"target", "formtarget"} {
		value, err := locator.GetAttribute(attribute)
		value = strings.ToLower(strings.TrimSpace(value))
		if err == nil && value != "" && value != "_self" && value != "_top" && value != "_parent" {
			return true
		}
	}
	return false
}

const authorSemanticSelector = "button,a,input,select,textarea,h1,h2,h3,h4,h5,h6,[role]"

var (
	authorARIALine      = regexp.MustCompile(`^- ([a-z]+)(?: \"((?:[^\"\\]|\\.)*)\")?`)
	captchaPattern      = regexp.MustCompile(`(?:captcha|recaptcha|hcaptcha)`)
	authorPortableRoles = map[string]bool{
		"button": true, "link": true, "textbox": true, "checkbox": true, "radio": true, "dialog": true,
		"status": true, "alert": true, "heading": true, "img": true, "list": true, "listitem": true,
		"combobox": true, "option": true, "menu": true, "menuitem": true, "tab": true, "tabpanel": true,
		"table": true, "row": true, "cell": true, "region": true, "navigation": true, "article": true,
		"form": true, "search": true, "switch": true, "group": true,
	}
)
