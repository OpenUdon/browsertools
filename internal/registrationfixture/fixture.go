// Package registrationfixture supplies synthetic, credential-free registration
// authoring fixtures shared by owner seam tests. It never contacts a real site.
package registrationfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"time"

	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/uws/browserregistration"
)

// HTML is entirely synthetic; Next changes only client-side visibility.
const HTML = `<!doctype html><html><body><form id="registration" method="post" action="/register">
<div id="identity"><label>Email<input role="textbox" name="email" type="email" required></label>
<label>Password<input role="textbox" name="password" type="password" required></label>
<label>Contact name<input name="name" required></label>
<label>Account kind<select name="kind" onchange="document.getElementById('company').hidden=this.value!=='business'" required><option value="individual">Individual</option><option value="business">Business</option></select></label>
<div id="company" hidden><label>Company name<input name="company"></label></div>
<button type="button" onclick="document.getElementById('identity').hidden=true;document.getElementById('contact').hidden=false">Next</button></div>
<div id="contact" hidden><label>Phone<input name="phone" type="tel"></label>
<label>Product updates<input name="updates" type="checkbox"></label><button type="submit">Register</button></div>
</form></body></html>`

func Profile(origin string, at time.Time) *registrationprofile.Profile {
	yes, no := true, false
	locator := func(role, name string) browserregistration.Locator {
		return browserregistration.Locator{Role: role, Name: name}
	}
	credential := func(slot, label string) browserregistration.Step {
		return browserregistration.Step{TypeCredential: &browserregistration.TypeCredentialStep{Slot: slot, Locator: locator("textbox", label)}}
	}
	fill := func(slot, control, role, label string) browserregistration.Step {
		return browserregistration.Step{FillInput: &browserregistration.FillInputStep{Slot: slot, Control: control, Locator: locator(role, label)}}
	}
	return &registrationprofile.Profile{
		Profile: browserregistration.ProfileNameV11, Info: browserregistration.Info{Title: "Synthetic membership registration", ApplicationOrigins: []string{origin}, RegistrationOrigins: []string{origin}},
		ObservationKind: "accessibility_snapshot", Evidence: browserregistration.Evidence{LearnedAt: at.Format(time.RFC3339), Source: "synthetic_fixture"}, Confidence: "high", ExpiresAfter: "P30D", Verification: browserregistration.Verification{LastVerifiedAt: at.Format(time.RFC3339)},
		CredentialSlots: map[string]browserregistration.CredentialSlot{"identifier": {Kind: "identifier"}, "password": {Kind: "password"}},
		InputSlots: map[string]browserregistration.InputSlot{
			"contact_name": {Type: "string", Label: "Contact name", Required: &yes},
			"account_kind": {Type: "string", Label: "Account kind", Required: &yes, Enum: []any{"individual", "business"}},
			"company":      {Type: "string", Label: "Company name", RequiredWhen: &browserregistration.InputCondition{Slot: "account_kind", Equals: "business"}},
			"phone":        {Type: "string", Label: "Phone", Required: &no}, "updates": {Type: "boolean", Label: "Product updates", Required: &no},
		},
		Flows: map[string]browserregistration.Flow{"member": {
			Sequence: []browserregistration.Step{
				{InputCheckpoint: &browserregistration.InputCheckpointStep{ID: "identity", Slots: []string{"identifier", "password", "contact_name", "account_kind", "company"}}},
				{Navigate: origin + "/register"}, credential("identifier", "Email"), credential("password", "Password"), fill("contact_name", "fill", "textbox", "Contact name"), fill("account_kind", "select", "combobox", "Account kind"), fill("company", "fill", "textbox", "Company name"),
				{Click: &browserregistration.ClickStep{Locator: locator("button", "Next")}},
				{InputCheckpoint: &browserregistration.InputCheckpointStep{ID: "contact", Slots: []string{"phone", "updates"}}}, fill("phone", "fill", "textbox", "Phone"), fill("updates", "check", "checkbox", "Product updates"),
				{HumanCheckpoint: &browserregistration.HumanCheckpointStep{Kind: "consent"}},
				{Submit: &browserregistration.SubmitStep{Locator: locator("button", "Register")}},
				{HumanCheckpoint: &browserregistration.HumanCheckpointStep{Kind: "email_verification"}},
			}, Effects: []string{"creates_account", "requires_human_verification", "sends_verification"}, ConfirmationPolicy: browserregistration.ConfirmationPolicy{Required: true}, Success: browserregistration.SuccessCondition{Origin: origin, Locator: locator("status", "Registration complete"), Path: "/complete"},
		}},
	}
}

// Author exercises the real v3 NDJSON state machine and consumes its actual
// emitted candidate IDs. The same sequence works with fake and Chromium backends.
func Author(ctx context.Context, browser registrationauthorsession.Browser, origin string, at time.Time) (*registrationauthorsession.Completion, error) {
	return author(ctx, browser, origin, at, nil)
}

func AuthorVerification(ctx context.Context, browser registrationauthorsession.Browser, origin string, at time.Time, descriptor *browserregistration.HumanVerification) (*registrationauthorsession.Completion, error) {
	if descriptor == nil {
		return nil, errors.New("verification descriptor missing")
	}
	return author(ctx, browser, origin, at, descriptor)
}

func author(ctx context.Context, browser registrationauthorsession.Browser, origin string, at time.Time, descriptor *browserregistration.HumanVerification) (*registrationauthorsession.Completion, error) {
	protocol := registrationauthorsession.ProtocolV3
	if descriptor != nil {
		protocol = registrationauthorsession.ProtocolV4
	}
	server, client := net.Pipe()
	defer client.Close()
	deadline := time.Now().Add(90 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = client.SetDeadline(deadline)
	type outcome struct {
		completion *registrationauthorsession.Completion
		err        error
	}
	done := make(chan outcome, 1)
	go func() {
		defer server.Close()
		completion, err := registrationauthorsession.Serve(ctx, server, server, browser, registrationauthorsession.ServeOptions{Protocol: protocol, Clock: func() time.Time { return at }})
		done <- outcome{completion, err}
	}()
	defer func() { _ = client.Close() }()
	encoder, decoder := json.NewEncoder(client), json.NewDecoder(client)
	read := func(kind string) (registrationauthorsession.ServerMessage, error) {
		var response registrationauthorsession.ServerMessage
		err := decoder.Decode(&response)
		if err == nil && response.Type != kind {
			err = fmt.Errorf("synthetic authoring expected %s, got %s (%v)", kind, response.Type, response.Diagnostic)
		}
		return response, err
	}
	send := func(message registrationauthorsession.ClientMessage) error {
		message.Protocol = protocol
		return encoder.Encode(message)
	}
	if _, err := read("hello"); err != nil {
		return nil, err
	}
	if err := send(registrationauthorsession.ClientMessage{Type: "start", ProfileID: "synthetic_member", URL: origin + "/register", Origins: []string{origin}}); err != nil {
		return nil, err
	}
	if _, err := read("state"); err != nil {
		return nil, err
	}
	if err := send(registrationauthorsession.ClientMessage{Type: "observe"}); err != nil {
		return nil, err
	}
	first, err := read("observation")
	if err != nil {
		return nil, err
	}
	id := func(observation *registrationauthorsession.Observation, role, label string) string {
		for _, candidate := range observation.Candidates {
			if candidate.Role == role && candidate.Label == label {
				return candidate.ID
			}
		}
		return ""
	}
	option := "business"
	if err := send(registrationauthorsession.ClientMessage{Type: "preview", Preview: &registrationauthorsession.PreviewRequest{CandidateID: id(first.Observation, "combobox", "Account kind"), Generation: 1, Action: "select", Option: &option, Purpose: "public_form_preview"}}); err != nil {
		return nil, err
	}
	second, err := read("observation")
	if err != nil {
		return nil, err
	}
	if err := send(registrationauthorsession.ClientMessage{Type: "preview", Preview: &registrationauthorsession.PreviewRequest{CandidateID: id(second.Observation, "button", "Next"), Generation: 2, Action: "click", Purpose: "public_form_preview"}}); err != nil {
		return nil, err
	}
	third, err := read("observation")
	if err != nil {
		return nil, err
	}
	profile := Profile(origin, at)
	steps := []string{"", "", id(first.Observation, "textbox", "Email"), id(first.Observation, "textbox", "Password"), id(first.Observation, "textbox", "Contact name"), id(first.Observation, "combobox", "Account kind"), id(second.Observation, "textbox", "Company name"), id(second.Observation, "button", "Next"), "", id(third.Observation, "textbox", "Phone"), id(third.Observation, "checkbox", "Product updates"), "", id(third.Observation, "button", "Register"), ""}
	if descriptor != nil {
		profile.Profile = browserregistration.ProfileNameV12
		flow := profile.Flows["member"]
		flow.HumanVerification = descriptor
		flow.Sequence = flow.Sequence[:len(flow.Sequence)-1]
		profile.Flows["member"] = flow
		steps = steps[:len(steps)-1]
		if err := send(registrationauthorsession.ClientMessage{Type: "approve_verification", CandidateID: id(third.Observation, "button", "Register"), Verification: descriptor}); err != nil {
			return nil, err
		}
		if _, err := read("state"); err != nil {
			return nil, err
		}
	}
	selected := []string{}
	for _, candidate := range steps {
		if candidate != "" {
			selected = append(selected, candidate)
		}
	}
	sort.Strings(selected)
	data, err := registrationprofile.MarshalJSON(profile)
	if err != nil {
		return nil, err
	}
	if err := send(registrationauthorsession.ClientMessage{Type: "review", Profile: data, CandidateIDs: selected, StepCandidates: steps, Flow: "member", CleanupDisposition: "delete_separately"}); err != nil {
		return nil, err
	}
	if _, err := read("state"); err != nil {
		return nil, err
	}
	if err := send(registrationauthorsession.ClientMessage{Type: "finish"}); err != nil {
		return nil, err
	}
	if _, err := read("state"); err != nil {
		return nil, err
	}
	result := <-done
	return result.completion, result.err
}

type Browser struct {
	Session      *Session
	Verification *browserregistration.HumanVerification
}

func (b *Browser) Open(_ context.Context, request registrationauthorsession.BrowserRequest) (registrationauthorsession.Session, error) {
	parsed, _ := url.Parse(request.URL)
	b.Session = &Session{origin: parsed.Scheme + "://" + parsed.Host, verification: b.Verification}
	return b.Session, nil
}

type Session struct {
	verification         *browserregistration.HumanVerification
	VerificationApproved bool
	origin               string
	state                int
	Closed               bool
	PreviewCount         int
}

func (s *Session) ApproveVerification(_ context.Context, value browserregistration.HumanVerification) error {
	if s.verification == nil || value != *s.verification || s.VerificationApproved {
		return errors.New("unreviewed verification")
	}
	s.VerificationApproved = true
	return nil
}
func (s *Session) Navigate(context.Context, registrationauthorsession.Navigation) error { return nil }
func (s *Session) Close(context.Context) (registrationauthorsession.NetworkSummary, error) {
	s.Closed = true
	return registrationauthorsession.NetworkSummary{Requests: 1, GETRequests: 1}, nil
}
func (s *Session) Preview(_ context.Context, _ registrationauthorsession.Candidate, request registrationauthorsession.PreviewRequest) error {
	s.PreviewCount++
	if request.Action == "select" {
		s.state = 1
	} else {
		s.state = 2
	}
	return nil
}
func (s *Session) Observe(context.Context) (registrationauthorsession.RawObservation, error) {
	yes, no := true, false
	candidate := func(role, label, kind string, required *bool) registrationauthorsession.RawCandidate {
		return registrationauthorsession.RawCandidate{Role: role, Label: label, Matches: 1, Control: &registrationauthorsession.ControlMetadata{Kind: kind, Required: required}}
	}
	var candidates []registrationauthorsession.RawCandidate
	if s.state < 2 {
		candidates = []registrationauthorsession.RawCandidate{candidate("textbox", "Email", "email", &yes), candidate("textbox", "Password", "password", &yes), candidate("textbox", "Contact name", "text", &yes), candidate("button", "Next", "button", nil), candidate("combobox", "Account kind", "select", &yes)}
		candidates[4].Control.Options = []registrationauthorsession.PublicOption{{Value: "individual", Label: "Individual"}, {Value: "business", Label: "Business"}}
		if s.state == 1 {
			candidates = append(candidates, candidate("textbox", "Company name", "text", &no))
		}
	} else {
		candidates = []registrationauthorsession.RawCandidate{candidate("textbox", "Phone", "text", &no), candidate("checkbox", "Product updates", "checkbox", &no), candidate("button", "Register", "unsupported", nil)}
	}
	if s.verification != nil {
		for i := range candidates {
			if candidates[i].Label == "Register" {
				v := s.verification
				candidates[i].Verification = &registrationauthorsession.VerificationObservation{Provider: v.Provider, Activation: v.Activation, SubmissionURL: v.SubmissionURL, WidgetBinding: v.WidgetBinding, Coverage: "standard_single_widget"}
			}
		}
	}
	return registrationauthorsession.RawObservation{Origin: s.origin, Path: "/register", Candidates: candidates, Diagnostics: []string{"synthetic_fixture"}}, nil
}
