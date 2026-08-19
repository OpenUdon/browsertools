package profile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OpenUdon/uws/schemas"
	"gopkg.in/yaml.v3"
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

func TestStepAndWaitRejectNestedUnknownFieldsJSONAndYAML(t *testing.T) {
	jsonCases := []string{
		`{"click":{"locator":{"role":"button","naem":"Save"}}}`,
		`{"click":{"locator":{"role":"button"},"wait_for":{"role":"status","naem":"Ready"}}}`,
		`{"navigate":{"url":"/","contex":"main"}}`,
		`{"wait_for":{"locator":{"role":"status","naem":"Ready"},"context":"main"}}`,
	}
	for _, input := range jsonCases {
		var step Step
		if err := json.Unmarshal([]byte(input), &step); err == nil {
			t.Fatalf("unknown nested JSON field accepted: %s", input)
		}
	}
	yamlCases := []string{
		"click:\n  locator:\n    role: button\n    naem: Save\n",
		"click:\n  locator: {role: button}\n  wait_for: {role: status, naem: Ready}\n",
		"navigate: {url: /, contex: main}\n",
		"wait_for:\n  locator: {role: status, naem: Ready}\n  context: main\n",
	}
	for _, input := range yamlCases {
		var step Step
		if err := yaml.Unmarshal([]byte(input), &step); err == nil {
			t.Fatalf("unknown nested YAML field accepted: %s", input)
		}
	}
}

func TestLosslessOutputAndNavigateMarshaling(t *testing.T) {
	absent, err := json.Marshal(Output{Type: OutputString, Source: OutputJSONLD, Property: "name"})
	if err != nil {
		t.Fatal(err)
	}
	present, err := json.Marshal(Output{Type: OutputString, Source: OutputJSONLD, Property: "name", Validation: JSONSchema{}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(absent, []byte(`"validation"`)) || !bytes.Contains(present, []byte(`"validation":{}`)) {
		t.Fatalf("validation presence was not preserved: absent=%s present=%s", absent, present)
	}
	presentYAML, err := yaml.Marshal(Output{Type: OutputString, Source: OutputJSONLD, Property: "name", Validation: JSONSchema{}})
	if err != nil || !bytes.Contains(presentYAML, []byte("validation: {}")) {
		t.Fatalf("empty validation schema was not preserved in YAML: %s err=%v", presentYAML, err)
	}
	navigate, err := json.Marshal(Step{Kind: StepNavigate, Navigate: ""})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(navigate, []byte(`{"navigate":""}`)) {
		t.Fatalf("empty navigate payload = %s", navigate)
	}
	click, err := json.Marshal(Step{Kind: StepClick, Navigate: "/wrong", Click: &LocatorStep{Locator: Locator{Role: RoleButton}}})
	if err != nil || bytes.Contains(click, []byte("wrong")) {
		t.Fatalf("step kind did not control marshal payload: %s err=%v", click, err)
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
