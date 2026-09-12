package registrationauthorsession_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/internal/registrationfixture"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

func TestV3AuthorsTypedConditionalWizardThroughProtocol(t *testing.T) {
	browser := &registrationfixture.Browser{}
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	completion, err := registrationfixture.Author(t.Context(), browser, "https://app.example.test", at)
	if err != nil {
		t.Fatal(err)
	}
	if !browser.Session.Closed || browser.Session.PreviewCount != 2 || len(completion.History) != 3 || len(completion.Previews) != 2 || completion.Profile.Profile != "uws.browser-registration.1.1" {
		t.Fatal("v3 authoring did not retain its reviewed wizard and teardown")
	}
	suggestions := registrationauthorsession.SuggestFields(completion.History[0])
	if len(suggestions) != 4 {
		t.Fatalf("suggestion count=%d", len(suggestions))
	}
	for _, suggestion := range suggestions {
		if !suggestion.ReviewRequired {
			t.Fatal("suggestion grants review")
		}
	}
	selected := []string{}
	for _, candidate := range completion.ReviewedCandidates {
		selected = append(selected, candidate.ID)
	}
	for name, mutate := range map[string]func(*registrationauthorsession.Completion){
		"wrong page":             func(c *registrationauthorsession.Completion) { c.History[1].Path = "/unrelated" },
		"wrong generation":       func(c *registrationauthorsession.Completion) { c.History[1].Generation = 1 },
		"changed preview":        func(c *registrationauthorsession.Completion) { c.Previews[0].Request.Purpose = "consent" },
		"missing transition":     func(c *registrationauthorsession.Completion) { c.Previews = c.Previews[:1] },
		"wrong macro candidate":  func(c *registrationauthorsession.Completion) { c.StepCandidates[6] = c.StepCandidates[4] },
		"missing macro evidence": func(c *registrationauthorsession.Completion) { c.StepCandidates[6] = "" },
	} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(completion)
			var changed registrationauthorsession.Completion
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			if registrationauthorsession.ValidateV3Evidence(&changed.Profile, changed.Flow, changed.History, changed.Previews, changed.StepCandidates, selected) == nil {
				t.Fatal("tampered observation proof accepted")
			}
		})
	}
}

func TestLegacyProtocolsRejectRegistration11AndPreview(t *testing.T) {
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	profile, err := registrationprofile.MarshalJSON(registrationfixture.Profile("https://app.example.test", at))
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{registrationauthorsession.ProtocolV1, registrationauthorsession.ProtocolV2} {
		for _, action := range []string{"preview", "review"} {
			t.Run(protocol+action, func(t *testing.T) {
				var input, output bytes.Buffer
				encoder := json.NewEncoder(&input)
				for _, message := range []registrationauthorsession.ClientMessage{
					{Protocol: protocol, Type: "start", ProfileID: "synthetic_member", URL: "https://app.example.test/register", Origins: []string{"https://app.example.test"}},
					{Protocol: protocol, Type: "observe"},
					{Protocol: protocol, Type: action, Profile: profile, Flow: "member", CleanupDisposition: "delete_separately"},
				} {
					if err := encoder.Encode(message); err != nil {
						t.Fatal(err)
					}
				}
				browser := &registrationfixture.Browser{}
				completion, err := registrationauthorsession.Serve(context.Background(), io.NopCloser(&input), &output, browser, registrationauthorsession.ServeOptions{Protocol: protocol, Clock: func() time.Time { return at }})
				if err == nil || completion != nil || !browser.Session.Closed || browser.Session.PreviewCount != 0 {
					t.Fatal("legacy protocol accepted new authority")
				}
				if action == "review" && !strings.Contains(output.String(), "invalid_profile") {
					t.Fatal("legacy recipe was not rejected at profile gate")
				}
			})
		}
	}
}

func TestPreviewRejectsUnreviewedOrPrivateControls(t *testing.T) {
	checked := true
	option := "business"
	candidate := registrationauthorsession.Candidate{ID: "candidate-0123456789abcdef", Role: "combobox", Label: "Account kind", Matches: 1, Control: &registrationauthorsession.ControlMetadata{Kind: "select", Options: []registrationauthorsession.PublicOption{{Value: option, Label: "Business"}}}}
	request := registrationauthorsession.PreviewRequest{CandidateID: candidate.ID, Generation: 1, Action: "select", Purpose: "public_form_preview", Option: &option}
	if err := registrationauthorsession.ValidatePreview(candidate, request); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Accept terms", "Consent", "Verify email", "Password", "Create account"} {
		copy := candidate
		copy.Label = label
		if registrationauthorsession.ValidatePreview(copy, request) == nil {
			t.Fatalf("accepted protected control %q", label)
		}
	}
	request.Checked = &checked
	if registrationauthorsession.ValidatePreview(candidate, request) == nil {
		t.Fatal("mixed preview controls accepted")
	}
	if registrationauthorsession.ValidateControlMetadata(&registrationauthorsession.ControlMetadata{Kind: "select", Options: []registrationauthorsession.PublicOption{{Value: "password=hunter2", Label: "Business"}}}) == nil {
		t.Fatal("private option accepted")
	}
}
