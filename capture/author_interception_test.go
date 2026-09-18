package capture

import (
	"errors"
	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
	"strings"
	"testing"
	"time"
)

type authorCDPCall struct {
	method string
	params map[string]any
}
type authorCDPStub struct {
	playwright.CDPSession
	calls   chan authorCDPCall
	release chan struct{}
	err     error
}

func (s *authorCDPStub) Send(method string, params map[string]any) (any, error) {
	s.calls <- authorCDPCall{method, params}
	if s.release != nil {
		<-s.release
	}
	return nil, s.err
}
func interceptionFixture() (*authorInterception, *authorCDPStub) {
	root := &authorCDPStub{calls: make(chan authorCDPCall, 8)}
	a := &authorInterception{root: root, guard: newAuthorNetworkGuard(authorsession.BrowserRequest{ApprovedOrigins: []string{"https://example.test"}, MaxRequests: 16, MaxResponseBytes: 1024}), timeout: 50 * time.Millisecond, slots: make(chan struct{}, 128), done: make(chan struct{})}
	return a, root
}
func pausedRequest(id, url, method string) map[string]any {
	return map[string]any{"requestId": id, "resourceType": "Script", "request": map[string]any{"url": url, "method": method, "headers": map[string]any{"Cookie": "secret-token-canary"}, "postData": "secret-token-canary"}}
}
func awaitAuthorCall(t *testing.T, root *authorCDPStub) authorCDPCall {
	t.Helper()
	select {
	case c := <-root.calls:
		return c
	case <-time.After(time.Second):
		t.Fatal("missing browser command")
		return authorCDPCall{}
	}
}
func TestAuthorInterceptionRedirectAndPOSTAdmission(t *testing.T) {
	for _, mode := range []string{"redirect", "post"} {
		t.Run(mode, func(t *testing.T) {
			a, root := interceptionFixture()
			defer a.close()
			first, second := "https://example.test/first", "https://excluded.test/secret-token-canary"
			method := "GET"
			if mode == "post" {
				method = "POST"
				second = "https://example.test/again"
				if a.guard.beginPOST(1) != nil {
					t.Fatal("POST window")
				}
			}
			a.paused(pausedRequest("first", first, method))
			a.paused(pausedRequest("second", second, method))
			calls := map[string]authorCDPCall{}
			for i := 0; i < 2; i++ {
				c := awaitAuthorCall(t, root)
				calls[c.params["requestId"].(string)] = c
				for _, value := range c.params {
					if text, ok := value.(string); ok && strings.Contains(text, "secret-token-canary") {
						t.Fatal("request data crossed command boundary")
					}
				}
			}
			if calls["first"].method != "Fetch.continueRequest" || calls["second"].method != "Fetch.failRequest" {
				t.Fatal("redirect or POST escaped admission")
			}
			if class := authordiagnostic.Classify(a.guard.result()); class.Stage != "policy" {
				t.Fatal("missing bounded failure")
			}
		})
	}
}
func TestAuthorInterceptionTimeoutCloseAndClosedError(t *testing.T) {
	for _, mode := range []string{"timeout", "close", "error"} {
		t.Run(mode, func(t *testing.T) {
			a, root := interceptionFixture()
			defer a.close()
			if mode == "error" {
				root.err = errors.New("secret-token-canary")
			} else {
				root.release = make(chan struct{})
				defer close(root.release)
			}
			result := make(chan error, 1)
			go func() { result <- a.command("Fetch.enable", map[string]any{}) }()
			awaitAuthorCall(t, root)
			if mode == "close" {
				a.close()
			}
			select {
			case err := <-result:
				if err == nil || strings.Contains(err.Error(), "secret-token-canary") {
					t.Fatal("unbounded command error")
				}
			case <-time.After(time.Second):
				t.Fatal("command did not stop")
			}
		})
	}
}
func TestAuthorInterceptionMalformedSaturatedAndClosed(t *testing.T) {
	for _, mode := range []string{"malformed", "saturated", "closed"} {
		t.Run(mode, func(t *testing.T) {
			a, root := interceptionFixture()
			defer a.close()
			params := pausedRequest("request", "https://example.test/", "GET")
			switch mode {
			case "malformed":
				delete(params, "request")
			case "saturated":
				for i := 0; i < cap(a.slots); i++ {
					a.slots <- struct{}{}
				}
			case "closed":
				a.close()
			}
			a.paused(params)
			if len(root.calls) != 0 {
				t.Fatal("unsafe request resumed")
			}
			if mode != "closed" && a.guard.result() == nil {
				t.Fatal("missing policy failure")
			}
			if mode == "closed" && a.guard.allow("https://example.test/", "GET") {
				t.Fatal("closed session admitted a request")
			}
		})
	}
}
func TestAuthorGuardRejectsURLCredentials(t *testing.T) {
	a, _ := interceptionFixture()
	defer a.close()
	if a.guard.allow("https://synthetic:canary@example.test/", "GET") {
		t.Fatal("URL credentials admitted")
	}
}
