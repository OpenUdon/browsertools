package authassist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/uws/browserauthentication"
)

type fakeBrowser struct {
	requests []BrowserRequest
	sessions []*fakeSession
	openErr  error
}

func (b *fakeBrowser) Open(_ context.Context, request BrowserRequest) (Session, error) {
	b.requests = append(b.requests, request)
	if b.openErr != nil {
		return nil, b.openErr
	}
	session := &fakeSession{origin: "https://login.example.test", matches: 1}
	b.sessions = append(b.sessions, session)
	return session, nil
}

type fakeSession struct {
	origin           string
	matches          int
	navigations      []string
	budgets          []int
	active           bool
	interactionCount int
	endCalls         int
	closed           bool
	closeErr         error
	observeErr       error
	reportOrigin     string
	reportedPosts    *int
	mutateLocator    bool
}

func (s *fakeSession) Navigate(_ context.Context, target string) error {
	s.navigations = append(s.navigations, target)
	if strings.HasPrefix(target, "https://members.example.test") {
		s.origin = "https://members.example.test"
	} else {
		s.origin = "https://login.example.test"
	}
	return nil
}

func (s *fakeSession) Observe(_ context.Context, locator *browserauthentication.Locator) (PageObservation, error) {
	if s.observeErr != nil {
		return PageObservation{}, s.observeErr
	}
	origin := s.origin
	if s.reportOrigin != "" {
		origin = s.reportOrigin
	}
	result := PageObservation{Origin: origin}
	if locator != nil {
		if s.mutateLocator {
			locator.Name = "backend mutation"
		}
		result.Matches = s.matches
	}
	return result, nil
}

func (s *fakeSession) BeginAuthenticationInteraction(budget int) error {
	if s.active {
		return errors.New("already active")
	}
	s.active = true
	s.budgets = append(s.budgets, budget)
	return nil
}

func (s *fakeSession) EndAuthenticationInteraction() (int, error) {
	if !s.active {
		return 0, errors.New("not active")
	}
	s.active = false
	s.endCalls++
	s.interactionCount++
	if s.interactionCount == 4 {
		s.origin = "https://members.example.test"
	}
	budget := s.budgets[len(s.budgets)-1]
	if s.reportedPosts != nil {
		return *s.reportedPosts, nil
	}
	if budget > 0 {
		return 1, nil
	}
	return 0, nil
}

func (s *fakeSession) Close() error {
	s.closed = true
	return s.closeErr
}

type fakeOperator struct {
	instructions []Instruction
	errAt        int
}

func (o *fakeOperator) Await(_ context.Context, instruction Instruction) error {
	o.instructions = append(o.instructions, instruction)
	if o.errAt > 0 && len(o.instructions) == o.errAt {
		return errors.New("operator stopped")
	}
	return nil
}

func TestRunObservesSelectedAlternativesAndBuildsValueFreeBundle(t *testing.T) {
	profileValue := validProfile(t)
	browser := &fakeBrowser{}
	operator := &fakeOperator{}
	bundle, err := Run(context.Background(), browser, operator, Request{
		Profile: profileValue, Flows: []string{"sms_login", "push_login"},
		ApprovedOrigins: []string{"https://members.example.test", "https://login.example.test"},
		POSTBudgets: map[string]int{
			"flows.push_login.sequence[3]": 2,
			"flows.sms_login.sequence[3]":  2,
		},
		ObservedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(browser.requests) != 2 || len(browser.sessions) != 2 || len(operator.instructions) != 8 {
		t.Fatalf("requests=%d sessions=%d instructions=%d", len(browser.requests), len(browser.sessions), len(operator.instructions))
	}
	for _, session := range browser.sessions {
		if !session.closed || session.active || session.endCalls != 4 {
			t.Fatalf("session = %#v", session)
		}
	}
	if bundle.Version != Version || bundle.Profile.ObservationKind != "other" ||
		bundle.Profile.Evidence.Source != "browsertools_assisted_auth_value_free" ||
		bundle.Profile.Verification.SuccessfulRuns != 1 || len(bundle.Flows) != 2 ||
		bundle.Flows[0].Flow != "push_login" || bundle.Flows[1].Flow != "sms_login" {
		t.Fatalf("bundle = %#v", bundle)
	}
	if err := Verify(bundle); err != nil {
		t.Fatal(err)
	}
	data, err := MarshalJSONIndent(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"actual-user", "actual-password", "123456", "cookie", "storageState", "oauth_state"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("forbidden value %q in bundle: %s", forbidden, data)
		}
	}
	if !bytes.Contains(data, []byte(`"approvedPostRequests": 2`)) || !bytes.Contains(data, []byte(`"observedPostRequests": 1`)) {
		t.Fatalf("missing bounded POST evidence: %s", data)
	}
}

func TestRunSubsetsFlowsAndCredentialSlots(t *testing.T) {
	bundle, err := Run(context.Background(), &fakeBrowser{}, &fakeOperator{}, Request{
		Profile: validProfile(t), Flows: []string{"push_login"},
		ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
		ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Profile.Flows) != 1 || len(bundle.Profile.CredentialSlots) != 2 {
		t.Fatalf("profile subset = %#v", bundle.Profile)
	}
	if _, ok := bundle.Profile.CredentialSlots["totp"]; ok {
		t.Fatal("unused TOTP slot was retained")
	}
}

func TestRunClosesAndReturnsNoArtifactOnFailure(t *testing.T) {
	for name, configure := range map[string]func(*fakeBrowser, *fakeOperator){
		"ambiguous locator": func(browser *fakeBrowser, _ *fakeOperator) {
			browser.openErr = nil
		},
		"operator stopped": func(_ *fakeBrowser, operator *fakeOperator) { operator.errAt = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			browser := &fakeBrowser{}
			operator := &fakeOperator{}
			configure(browser, operator)
			if name == "ambiguous locator" {
				// Replace Open so the first session can be made ambiguous after it is created.
				browser.openErr = errors.New("synthetic open failure")
			}
			bundle, err := Run(context.Background(), browser, operator, Request{
				Profile: validProfile(t), Flows: []string{"push_login"},
				ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
				ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
			})
			if err == nil || bundle != nil {
				t.Fatalf("bundle=%#v err=%v", bundle, err)
			}
			if name == "operator stopped" {
				if len(browser.sessions) != 1 || !browser.sessions[0].closed || browser.sessions[0].endCalls != 1 {
					t.Fatalf("failed session = %#v", browser.sessions)
				}
			}
		})
	}

	browser := &fakeBrowser{}
	operator := &fakeOperator{}
	profileValue := validProfile(t)
	profileValue.Flows["push_login"] = withAmbiguousSuccess(profileValue.Flows["push_login"])
	// The fake returns two matches only after opening; use a dedicated wrapper.
	ambiguous := &configuredBrowser{configure: func(session *fakeSession) { session.matches = 2 }}
	bundle, err := Run(context.Background(), ambiguous, operator, Request{
		Profile: profileValue, Flows: []string{"push_login"},
		ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
		ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err == nil || bundle != nil || len(ambiguous.sessions) != 1 || !ambiguous.sessions[0].closed {
		t.Fatalf("bundle=%#v err=%v sessions=%#v", bundle, err, ambiguous.sessions)
	}
	_ = browser
}

func TestRunDefendsAgainstMisbehavingBrowserImplementations(t *testing.T) {
	baseRequest := func() Request {
		return Request{
			Profile: validProfile(t), Flows: []string{"push_login"},
			ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
			ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		}
	}
	for name, configure := range map[string]func(*fakeSession){
		"unapproved origin": func(session *fakeSession) { session.reportOrigin = "https://evil.test" },
		"negative count":    func(session *fakeSession) { session.matches = -1 },
		"excess POST count": func(session *fakeSession) {
			value := 1
			session.reportedPosts = &value
		},
	} {
		t.Run(name, func(t *testing.T) {
			browser := &configuredBrowser{configure: configure}
			bundle, err := Run(context.Background(), browser, &fakeOperator{}, baseRequest())
			if err == nil || bundle != nil || len(browser.sessions) != 1 || !browser.sessions[0].closed {
				t.Fatalf("bundle=%#v err=%v sessions=%#v", bundle, err, browser.sessions)
			}
		})
	}

	mutatingBrowser := &configuredBrowser{configure: func(session *fakeSession) { session.mutateLocator = true }}
	bundle, err := Run(context.Background(), mutatingBrowser, &fakeOperator{}, baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if got := bundle.Profile.Flows["push_login"].Sequence[1].TypeCredential.Locator.Name; got != "Username" {
		t.Fatalf("backend mutated output profile locator: %q", got)
	}
}

type partialOpenBrowser struct {
	session *fakeSession
}

func (b *partialOpenBrowser) Open(context.Context, BrowserRequest) (Session, error) {
	return b.session, errors.New("open failed after context creation")
}

func TestRunClosesPartiallyOpenedSession(t *testing.T) {
	session := &fakeSession{origin: "https://login.example.test", matches: 1}
	bundle, err := Run(context.Background(), &partialOpenBrowser{session: session}, &fakeOperator{}, Request{
		Profile: validProfile(t), Flows: []string{"push_login"},
		ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
		ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err == nil || bundle != nil || !session.closed {
		t.Fatalf("bundle=%#v err=%v session=%#v", bundle, err, session)
	}
}

type configuredBrowser struct {
	configure func(*fakeSession)
	sessions  []*fakeSession
}

func (b *configuredBrowser) Open(_ context.Context, _ BrowserRequest) (Session, error) {
	session := &fakeSession{origin: "https://login.example.test", matches: 1}
	b.configure(session)
	b.sessions = append(b.sessions, session)
	return session, nil
}

func TestNormalizeRequestRejectsUnsafeOrAmbiguousAuthority(t *testing.T) {
	base := func() Request {
		return Request{
			Profile: validProfile(t), Flows: []string{"push_login"},
			ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
			ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		}
	}
	tests := map[string]func(*Request){
		"nil profile":       func(r *Request) { r.Profile = nil },
		"no flow":           func(r *Request) { r.Flows = nil },
		"unknown flow":      func(r *Request) { r.Flows = []string{"missing"} },
		"duplicate flow":    func(r *Request) { r.Flows = []string{"push_login", "push_login"} },
		"missing origin":    func(r *Request) { r.ApprovedOrigins = []string{"https://login.example.test"} },
		"extra origin":      func(r *Request) { r.ApprovedOrigins = append(r.ApprovedOrigins, "https://extra.example.test") },
		"duplicate origin":  func(r *Request) { r.ApprovedOrigins = append(r.ApprovedOrigins, "https://login.example.test:443") },
		"unknown budget":    func(r *Request) { r.POSTBudgets = map[string]int{"flows.push_login.sequence[0]": 1} },
		"zero budget entry": func(r *Request) { r.POSTBudgets = map[string]int{"flows.push_login.sequence[3]": 0} },
		"oversized budget": func(r *Request) {
			r.POSTBudgets = map[string]int{"flows.push_login.sequence[3]": MaxPOSTRequestsPerStep + 1}
		},
		"missing time":   func(r *Request) { r.ObservedAt = time.Time{} },
		"timeout":        func(r *Request) { r.NavigationTimeout = time.Minute; r.TotalTimeout = time.Second },
		"request limit":  func(r *Request) { r.MaxRequests = MaxRequests + 1 },
		"response limit": func(r *Request) { r.MaxResponseBytes = MaxResponseBytes + 1 },
		"value locator": func(r *Request) {
			flow := r.Profile.Flows["push_login"]
			flow.Sequence[1].TypeCredential.Locator.Value = "some-value"
			r.Profile.Flows["push_login"] = flow
		},
		"no initial navigate": func(r *Request) {
			flow := r.Profile.Flows["push_login"]
			flow.Sequence = flow.Sequence[1:]
			r.Profile.Flows["push_login"] = flow
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			request := base()
			mutate(&request)
			browser := &fakeBrowser{}
			if bundle, err := Run(context.Background(), browser, &fakeOperator{}, request); err == nil || bundle != nil || len(browser.requests) != 0 {
				t.Fatalf("bundle=%#v err=%v browser calls=%d", bundle, err, len(browser.requests))
			}
		})
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	bundle, err := Run(context.Background(), &fakeBrowser{}, &fakeOperator{}, Request{
		Profile: validProfile(t), Flows: []string{"push_login"},
		ApprovedOrigins: []string{"https://login.example.test", "https://members.example.test"},
		ObservedAt:      time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(bundle)
	for name, mutate := range map[string]func(*Bundle){
		"version": func(value *Bundle) { value.Version = "other" },
		"digest":  func(value *Bundle) { value.ProfileDigest = "sha256:bad" },
		"flow":    func(value *Bundle) { value.Flows[0].Flow = "other" },
		"check":   func(value *Bundle) { value.Flows[0].Checks[0].OK = false },
		"posts": func(value *Bundle) {
			value.Flows[0].Checks[1].ObservedPOSTRequests = MaxPOSTRequestsPerStep + 1
		},
	} {
		t.Run(name, func(t *testing.T) {
			var altered Bundle
			if err := json.Unmarshal(encoded, &altered); err != nil {
				t.Fatal(err)
			}
			mutate(&altered)
			if err := Verify(&altered); err == nil {
				t.Fatal("tampered bundle verified")
			}
		})
	}
}

func TestLineOperatorAcceptsOnlyEmptyBoundedSignals(t *testing.T) {
	var output bytes.Buffer
	operator := NewLineOperator(strings.NewReader("\n"), &output)
	if err := operator.Await(context.Background(), Instruction{Path: "flows.login.sequence[1]", Kind: "type_credential"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "Username") || !strings.Contains(output.String(), "press Enter only") {
		t.Fatalf("prompt = %q", output.String())
	}
	for name, input := range map[string]string{"non-empty": "secret\n", "oversized": strings.Repeat("x", maxOperatorSignalBytes+1) + "\n", "eof": ""} {
		t.Run(name, func(t *testing.T) {
			if err := NewLineOperator(strings.NewReader(input), &bytes.Buffer{}).Await(context.Background(), Instruction{}); err == nil {
				t.Fatal("unsafe signal accepted")
			}
		})
	}
}

func validProfile(t *testing.T) *authprofile.Profile {
	t.Helper()
	value := authprofile.Profile{
		Profile: browserauthentication.ProfileName,
		Info: browserauthentication.Info{
			Title: "Example member login", ApplicationOrigins: []string{"https://members.example.test"},
			AuthenticationOrigins: []string{"https://login.example.test"},
		},
		ObservationKind: "other",
		Evidence:        browserauthentication.Evidence{LearnedAt: "2026-08-15T00:00:00Z", Source: "explicit_author_draft"},
		Confidence:      "high", ExpiresAfter: "P30D",
		Verification: browserauthentication.Verification{LastVerifiedAt: "2026-08-15T00:00:00Z", SuccessfulRuns: 0},
		CredentialSlots: map[string]browserauthentication.CredentialSlot{
			"username": {Kind: "identifier"}, "password": {Kind: "password"}, "totp": {Kind: "totp_seed"},
		},
		Flows: map[string]browserauthentication.Flow{
			"push_login": authFlow("push"),
			"sms_login":  authFlow("sms_otp"),
		},
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := authprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return validated
}

func authFlow(challengeKind string) browserauthentication.Flow {
	challenge := &browserauthentication.ChallengeStep{Kind: challengeKind}
	if challengeKind == "sms_otp" {
		challenge.Locator = &browserauthentication.Locator{Role: "textbox", Name: "Verification code"}
	}
	return browserauthentication.Flow{
		Sequence: []browserauthentication.Step{
			{Navigate: "https://login.example.test/start"},
			{TypeCredential: &browserauthentication.TypeCredentialStep{Locator: browserauthentication.Locator{Role: "textbox", Name: "Username"}, Slot: "username"}},
			{TypeCredential: &browserauthentication.TypeCredentialStep{Locator: browserauthentication.Locator{Role: "textbox", Name: "Password"}, Slot: "password"}},
			{Click: &browserauthentication.ClickStep{Locator: browserauthentication.Locator{Role: "button", Name: "Sign in"}}},
			{Challenge: challenge},
			{WaitFor: &browserauthentication.WaitForCondition{Locator: browserauthentication.Locator{Role: "heading", Name: "Member dashboard"}}},
		},
		Effects: []string{"establishes_session", "sends_mfa_challenge"},
		Success: browserauthentication.SuccessCondition{
			Origin: "https://members.example.test", Locator: browserauthentication.Locator{Role: "heading", Name: "Member dashboard"},
		},
	}
}

func withAmbiguousSuccess(flow browserauthentication.Flow) browserauthentication.Flow {
	flow.Description = fmt.Sprintf("%s", flow.Description)
	return flow
}
