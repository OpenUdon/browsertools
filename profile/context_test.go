package profile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OpenUdon/uws/schemas"
)

func TestContextQualifiedUnionsRoundTrip(t *testing.T) {
	var step Step
	if err := json.Unmarshal([]byte(`{"navigate":{"url":"https://members.example.test/dashboard","context":"main"}}`), &step); err != nil {
		t.Fatal(err)
	}
	if step.Navigate != "https://members.example.test/dashboard" || step.NavigateContext != "main" || !step.NavigateObject {
		t.Fatalf("navigate = %#v", step)
	}
	data, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte(`{"navigate":{"context":"main","url":"https://members.example.test/dashboard"}}`)) {
		t.Fatalf("navigate JSON = %s", data)
	}
	var wait WaitForCondition
	if err := json.Unmarshal([]byte(`{"locator":{"role":"status","name":"Ready"},"context":"detail_frame"}`), &wait); err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(wait)
	if err != nil || !bytes.Contains(data, []byte(`"context":"detail_frame"`)) {
		t.Fatalf("wait JSON = %s err=%v", data, err)
	}
}

func TestBrowser16ProfileTypedRoundTrip(t *testing.T) {
	schema, err := schemas.BrowserSourceProfileSchema("uws.browser.1.6")
	if err != nil || !bytes.Contains(schema, []byte("uws.browser.1.6")) {
		t.Fatalf("pinned UWS dependency lacks browser 1.6: %v", err)
	}
	data := []byte(`
profile: uws.browser.1.6
info:
  title: Member dashboard
  origin: [https://members.example.test, https://login.example.test]
  loginStateRequired: true
observationKind: accessibility_snapshot
evidence: {learnedAt: "2026-08-16T00:00:00Z", source: reviewed_synthetic_fixture}
confidence: high
expiresAfter: P30D
verification: {lastVerifiedAt: "2026-08-16T00:00:00Z", successfulRuns: 1}
contexts:
  login_frame: {kind: frame, parent: main, origin: https://login.example.test, path: /embedded/login, name: Login}
actions:
  read_status:
    sequence:
      - navigate: {url: https://members.example.test/dashboard, context: main}
      - wait_for: {locator: {role: status, name: Ready}, context: login_frame}
    outputs:
      ready: {type: boolean, source: a11y, locator: {role: status, name: Ready}, context: login_frame, presence: true}
    sideEffects: [read_only]
    confirmationPolicy: {required: false}
`)
	profile, err := ParseYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Schema != "uws.browser.1.6" || profile.Contexts["login_frame"].Path != "/embedded/login" ||
		profile.Actions["read_status"].Outputs["ready"].Context != "login_frame" {
		t.Fatalf("typed profile = %#v", profile)
	}
	roundTrip, err := MarshalYAML(*profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseYAML(roundTrip); err != nil {
		t.Fatalf("round trip: %v\n%s", err, roundTrip)
	}
}
