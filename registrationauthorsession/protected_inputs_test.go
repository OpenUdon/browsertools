package registrationauthorsession

import (
	"testing"

	"github.com/OpenUdon/uws/browserregistration"
)

func TestProtectedHumanControlsAreNotPrivateFields(t *testing.T) {
	for _, label := range []string{"Accept terms", "Consent", "Verification code", "CAPTCHA", "Security code", "Product updates"} {
		candidate := Candidate{Role: "checkbox", Label: label, Matches: 1, Control: &ControlMetadata{Kind: "checkbox"}}
		candidate.ID = candidateID(1, candidate.Role, candidate.Label, 0)
		observation := Observation{Generation: 1, Origin: "https://registration.example", Path: "/register", Candidates: []Candidate{candidate}}
		profile := &browserregistration.Profile{Profile: browserregistration.ProfileNameV11, Info: browserregistration.Info{ApplicationOrigins: []string{observation.Origin}, RegistrationOrigins: []string{observation.Origin}}, InputSlots: map[string]browserregistration.InputSlot{"choice": {Type: "boolean"}}, Flows: map[string]browserregistration.Flow{"member": {Sequence: []browserregistration.Step{{Navigate: observation.Origin + observation.Path}, {FillInput: &browserregistration.FillInputStep{Slot: "choice", Control: "check", Locator: browserregistration.Locator{Role: candidate.Role, Name: label}}}}}}}
		err := ValidateV3Evidence(profile, "member", []Observation{observation}, nil, []string{"", candidate.ID}, []string{candidate.ID})
		suggestions := SuggestFields(observation)
		if label == "Product updates" {
			if err != nil || len(suggestions) != 1 {
				t.Fatal("ordinary public checkbox was not supported")
			}
		} else if err == nil || len(suggestions) != 0 {
			t.Fatal("human control was accepted as an ordinary private field")
		}
	}
}
