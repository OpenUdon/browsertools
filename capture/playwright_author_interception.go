package capture

import (
	"errors"
	"sync"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
)

// authorInterception admits requests at Chromium's browser-wide Fetch boundary.
// It is installed before any page exists, so popup and out-of-process frame
// requests cannot outrun per-target attachment. Chromium retains its transport,
// redirect methods, cookies, headers and compression. No request is reissued and
// no response body, request header or credential value is read here.
//
// This requires the pinned Chromium's browser-target Fetch implementation.
// Unsupported setup fails before navigation; there is no route-only fallback.
// Existing declared-size and completed-transfer accounting stays on the context.
type authorInterception struct {
	root    playwright.CDPSession
	guard   *authorNetworkGuard
	timeout time.Duration
	slots   chan struct{}
	done    chan struct{}
	once    sync.Once
}

func installAuthorInterception(browser playwright.Browser, guard *authorNetworkGuard, timeout time.Duration) (*authorInterception, error) {
	root, err := browser.NewBrowserCDPSession()
	if err != nil {
		return nil, errors.New("author interception unavailable")
	}
	a := &authorInterception{root: root, guard: guard, timeout: timeout, slots: make(chan struct{}, 128), done: make(chan struct{})}
	root.On("Fetch.requestPaused", a.paused)
	root.OnClose(func(playwright.CDPSession) { a.fail(); a.close() })
	if a.command("Fetch.enable", map[string]any{"patterns": []map[string]any{{"urlPattern": "*", "requestStage": "Request"}}}) != nil {
		a.close()
		return nil, errors.New("author interception unavailable")
	}
	return a, nil
}

func (a *authorInterception) fail() {
	select {
	case <-a.done:
		return
	default:
		a.guard.block("route_continue")
	}
}

// Keep interception installed until the owned browser is destroyed. Detaching
// or disabling it here could release a paused request during teardown.
func (a *authorInterception) close() {
	if a != nil {
		a.once.Do(func() { a.guard.beginClose(); close(a.done) })
	}
}

func (a *authorInterception) command(method string, params map[string]any) error {
	bad := errors.New("author interception command failed")
	select {
	case <-a.done:
		return bad
	default:
	}
	result := make(chan error, 1)
	go func() { _, err := a.root.Send(method, params); result <- err }()
	timer := time.NewTimer(a.timeout)
	defer timer.Stop()
	select {
	case err := <-result:
		if err != nil {
			return bad
		}
		return nil
	case <-timer.C:
		return bad
	case <-a.done:
		return bad
	}
}

func (a *authorInterception) paused(params map[string]any) {
	select {
	case <-a.done:
		return
	default:
	}
	id, idOK := params["requestId"].(string)
	kind, kindOK := params["resourceType"].(string)
	request, requestOK := params["request"].(map[string]any)
	rawURL, urlOK := request["url"].(string)
	method, methodOK := request["method"].(string)
	if !idOK || id == "" || !kindOK || kind == "" || !requestOK || !urlOK || !methodOK {
		a.fail()
		return
	}
	// A saturated command channel leaves the new request paused and poisons the
	// session. The operation deadline and browser teardown remain mandatory.
	select {
	case a.slots <- struct{}{}:
	default:
		a.guard.block("request_limit")
		return
	}
	allowed := a.guard.allow(rawURL, method, kind == "Document")
	go func() {
		defer func() { <-a.slots }()
		operation := "Fetch.continueRequest"
		args := map[string]any{"requestId": id}
		if !allowed {
			operation = "Fetch.failRequest"
			args["errorReason"] = "BlockedByClient"
		}
		if a.command(operation, args) != nil {
			a.fail()
		}
	}()
}
