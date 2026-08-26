package browsertools_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != "fb29a117d9faff9a6d019c4160f736ef365b07976b2d495548ae122491129287" {
		t.Fatalf("v1/BAP compatibility fixture bytes changed: %s", got)
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

func TestRegistrationAuthoringV2CompatibilityFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/compatibility/registration-authoring-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SessionV1 string `json:"sessionV1"`
		ResultV1  string `json:"resultV1"`
		SessionV2 string `json:"sessionV2"`
		ResultV2  string `json:"resultV2"`
		Review    string `json:"review"`
		Profile   string `json:"profile"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("trailing v2 compatibility fixture: %v", err)
	}
	if fixture.SessionV1 != registrationauthorsession.ProtocolV1 || fixture.ResultV1 != registrationauthorresult.SchemaV1 ||
		fixture.SessionV2 != registrationauthorsession.ProtocolV2 || fixture.ResultV2 != registrationauthorresult.SchemaV2 ||
		fixture.Review != registrationreview.Version || fixture.Profile != browserregistration.ProfileName {
		t.Fatalf("registration v2 fixture drifted: %#v", fixture)
	}
}
