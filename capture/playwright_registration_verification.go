package capture

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/uws/browserregistration"
	playwright "github.com/mxschmitt/playwright-go"
)

// Reads only public configuration and a submit/form relationship. In
// particular it never reads response fields, site keys or challenge contents.
const registrationVerificationDefinition = `element => {
 const form=element.form;
 if (!form || form.method.toUpperCase()!=='POST' || (form.target && form.target!=='_self') || (element.hasAttribute('formtarget') && element.formTarget && element.formTarget!=='_self') || (element.tagName!=='BUTTON' && element.tagName!=='INPUT') || element.type!=='submit') return null;
 if (element.hasAttribute('formaction') && element.formAction!==form.action || element.hasAttribute('formmethod') && element.formMethod.toUpperCase()!=='POST') return null;
 const widgets=Array.from(document.querySelectorAll('.cf-turnstile,.g-recaptcha,.h-captcha'));
 if (widgets.length!==1 || !form.contains(widgets[0])) return null;
 const widget=widgets[0], provider=widget.classList.contains('cf-turnstile')?'turnstile':widget.classList.contains('h-captcha')?'hcaptcha':'recaptcha_v2';
 const activation=widget.dataset.execution==='execute' || widget.dataset.size==='invisible' || widget===element?'approved_submit':'before_approval';
 return {provider,activation,submissionURL:form.action,widgetBinding:'single_in_submit_form',coverage:'standard_single_widget'};
}`

func registrationVerificationMetadata(locator playwright.Locator, origins []string) (*registrationauthorsession.VerificationObservation, error) {
	value, err := locator.Evaluate(registrationVerificationDefinition, nil)
	if err != nil {
		return nil, errors.New("verification metadata unavailable")
	}
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > 4096 {
		return nil, errors.New("invalid verification metadata")
	}
	var result registrationauthorsession.VerificationObservation
	if json.Unmarshal(data, &result) != nil || registrationauthorsession.ValidateVerificationObservation(&result, origins) != nil {
		return nil, errors.New("invalid verification metadata")
	}
	return &result, nil
}

func (s *playwrightRegistrationSession) ApproveVerification(ctx context.Context, authority browserregistration.HumanVerification) error {
	if s.request.Protocol != registrationauthorsession.ProtocolV4 || registrationauthorsession.ValidateVerificationAuthority(&authority, s.request.ApprovedOrigins) != nil {
		return errors.New("invalid verification authority")
	}
	if err := s.health(ctx); err != nil {
		return err
	}
	s.guard.mu.Lock()
	defer s.guard.mu.Unlock()
	if s.guard.verification != nil {
		return errors.New("verification authority already consumed")
	}
	copy := authority
	s.guard.verification = &copy
	s.guard.verificationDeadline = time.Now().Add(time.Duration(authority.Dependencies.TimeoutMS) * time.Millisecond)
	return nil
}

var verificationHost = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+hcaptcha\.com$`)
var verificationEscapes = regexp.MustCompile(`(?i)%(?:2e|2f|5c|00)`)

func permitsRegistrationVerification(provider, raw, method string, document bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Port() != "" && u.Port() != "443" || strings.ContainsAny(raw, "\\ \t\r\n") || verificationEscapes.MatchString(u.EscapedPath()) {
		return false
	}
	if method != "GET" && method != "HEAD" && method != "POST" {
		return false
	}
	host, path := u.Hostname(), u.Path
	switch provider {
	case "turnstile":
		return host == "challenges.cloudflare.com" && (strings.HasPrefix(path, "/turnstile/") || strings.HasPrefix(path, "/cdn-cgi/challenge-platform/"))
	case "recaptcha_v2":
		if !strings.HasPrefix(path, "/recaptcha/") || strings.HasPrefix(path, "/recaptcha/enterprise") {
			return false
		}
		if host == "www.gstatic.com" {
			return !document && method != "POST"
		}
		return host == "www.google.com" || host == "www.recaptcha.net"
	case "hcaptcha":
		return host == "hcaptcha.com" || verificationHost.MatchString(host)
	}
	return false
}

// Provider redirects are refused before forwarding to Chromium, so a redirect
// cannot bypass context routes in nested or out-of-process frames.
func (g *registrationNetworkGuard) handleVerificationRoute(route playwright.Route) bool {
	request := route.Request()
	g.mu.Lock()
	authority := g.verification
	if authority == nil || !permitsRegistrationVerification(authority.Provider, request.URL(), request.Method(), request.IsNavigationRequest()) {
		g.mu.Unlock()
		return false
	}
	deadline := g.verificationDeadline
	allowed := !g.closing && g.core.result() == nil && time.Now().Before(deadline)
	allowed = allowed && request.ResourceType() != "eventsource" && request.ResourceType() != "websocket"
	frame := request.Frame()
	if request.IsNavigationRequest() {
		frame = frame.ParentFrame()
	}
	bound := false
	for depth := 0; frame != nil && depth < 8; depth++ {
		if frame == g.main {
			u, err := url.Parse(frame.URL())
			if err == nil {
				_, bound = g.origins[u.Scheme+"://"+u.Host]
			}
			break
		}
		if !permitsRegistrationVerification(authority.Provider, frame.URL(), "GET", true) {
			break
		}
		frame = frame.ParentFrame()
	}
	allowed = allowed && bound
	allowed = allowed && g.providerRequests < authority.Dependencies.MaxRequests
	if allowed {
		g.providerRequests++
		if request.Method() == "POST" {
			g.providerPOSTRequests++
		}
	}
	if !allowed {
		g.violate("verification_policy")
	}
	g.mu.Unlock()
	if !allowed {
		if route.Abort("blockedbyclient") != nil {
			g.block("route_abort")
		}
		return true
	}
	fail := func() {
		g.block("verification_policy")
		if route.Abort("blockedbyclient") != nil {
			g.block("route_abort")
		}
	}
	response, err := route.Fetch(playwright.RouteFetchOptions{MaxRedirects: playwright.Int(0), MaxRetries: playwright.Int(0), Timeout: playwright.Float(float64(time.Until(deadline).Milliseconds()))})
	if err != nil || response == nil {
		fail()
		return true
	}
	defer func() {
		if response.Dispose() != nil {
			g.block("verification_policy")
		}
	}()
	if response.Status() >= 300 && response.Status() < 400 || strings.Contains(strings.ToLower(response.Headers()["content-type"]), "text/event-stream") || strings.Contains(strings.ToLower(response.Headers()["content-disposition"]), "attachment") {
		fail()
		return true
	}
	body, err := response.Body()
	if err != nil {
		fail()
		return true
	}
	g.mu.Lock()
	allowed = int64(len(body)) <= int64(authority.Dependencies.MaxResponseBytes)-g.providerResponseBytes && time.Now().Before(deadline) && g.core.result() == nil
	if allowed {
		g.providerResponseBytes += int64(len(body))
	}
	g.mu.Unlock()
	if !allowed {
		fail()
		return true
	}
	if route.Fulfill(playwright.RouteFulfillOptions{Response: response, Body: body}) != nil {
		g.block("verification_policy")
	}
	return true
}

func (g *registrationNetworkGuard) unreviewedVerificationFrame(request playwright.Request) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.verification != nil || request.Method() != "GET" && request.Method() != "HEAD" || !request.IsNavigationRequest() || request.Frame().ParentFrame() != g.main {
		return false
	}
	for _, provider := range []string{"turnstile", "recaptcha_v2", "hcaptcha"} {
		if permitsRegistrationVerification(provider, request.URL(), request.Method(), true) {
			return true
		}
	}
	return false
}

func (g *registrationNetworkGuard) isVerificationURL(raw string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.verification != nil && permitsRegistrationVerification(g.verification.Provider, raw, "GET", false)
}
