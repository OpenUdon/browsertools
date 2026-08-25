package browsertools_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationreview"
	"github.com/OpenUdon/uws/browserregistration"
)

func TestAuthoringWireVersionCompatibilityFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/compatibility/authoring-wire-versions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Authenticated struct {
			Session string `json:"session"`
			Result  string `json:"result"`
		} `json:"authenticated"`
		Registration struct {
			Session string `json:"session"`
			Result  string `json:"result"`
			Review  string `json:"review"`
			Profile string `json:"profile"`
		} `json:"registration"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("trailing compatibility fixture: %v", err)
	}
	if fixture.Authenticated.Session != authorsession.Protocol || fixture.Authenticated.Result != authorresult.Schema {
		t.Fatalf("authenticated fixture drifted: %#v", fixture.Authenticated)
	}
	if fixture.Registration.Session != registrationauthorsession.Protocol ||
		fixture.Registration.Result != registrationauthorresult.Schema ||
		fixture.Registration.Review != registrationreview.Version ||
		fixture.Registration.Profile != browserregistration.ProfileName {
		t.Fatalf("registration fixture drifted: %#v", fixture.Registration)
	}
}
