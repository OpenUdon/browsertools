package overlay

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
)

// TestSidecarJSONRoundTrip verifies the Sidecar struct serializes and
// deserializes cleanly with all expected JSON keys present.
func TestSidecarJSONRoundTrip(t *testing.T) {
	s := Sidecar{
		OverlayVersion: "1",
		OverlayID:      "test-overlay-001",
		WrapperOpenAPI: "./wrapper.openapi.yaml",
		BrowserProfile: "./browser-profile.yaml",
		ReviewBundle: review.Bundle{
			Profile:    profile.Profile{Info: profile.Info{Origin: profile.Origins{"https://example.test"}}},
			Validation: review.ValidationReport{Valid: true},
		},
		OperationMappings: map[string]OperationMapping{
			"getArticle": {
				OpenAPIOperationID: "getArticle",
				ProfileActionName:  "navigate_to_article",
				ConfidenceNote:     "stable path; reviewed 2026-01",
			},
		},
		Lifecycle: LifecycleReviewed,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(data) || !bytes.Contains(data, []byte(`"openAPIOperationId"`)) || bytes.Contains(data, []byte(`"openapiOperationId"`)) {
		t.Fatalf("exact OpenAPI operation key missing from %s", data)
	}

	var got Sidecar
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.OverlayVersion != "1" {
		t.Errorf("overlayVersion: got %q", got.OverlayVersion)
	}
	if got.Lifecycle != LifecycleReviewed {
		t.Errorf("lifecycle: got %q", got.Lifecycle)
	}
	if got.ReviewBundle.Validation.Valid != true {
		t.Error("reviewBundle.validation.valid should be true")
	}
	m, ok := got.OperationMappings["getArticle"]
	if !ok {
		t.Fatal("expected getArticle in operationMappings")
	}
	if m.ProfileActionName != "navigate_to_article" {
		t.Errorf("profileActionName: got %q", m.ProfileActionName)
	}
}

// TestOverlayJSONExampleRoundTrip verifies the example overlay.json unmarshals
// cleanly into a Sidecar, keeping the example in sync with the Go struct.
func TestOverlayJSONExampleRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../examples/wrapper-service/overlay.json")
	if err != nil {
		t.Fatalf("read overlay.json: %v", err)
	}
	if !bytes.Contains(data, []byte(`"openAPIOperationId"`)) || bytes.Contains(data, []byte(`"openapiOperationId"`)) {
		t.Fatalf("example uses the wrong raw OpenAPI operation key: %s", data)
	}
	var s Sidecar
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal overlay.json: %v", err)
	}
	if s.OverlayVersion != "1" {
		t.Errorf("overlayVersion: got %q", s.OverlayVersion)
	}
	if s.Lifecycle != LifecycleReviewed {
		t.Errorf("lifecycle: got %q", s.Lifecycle)
	}
	if len(s.OperationMappings) == 0 {
		t.Error("expected non-empty operationMappings")
	}
	prof, err := profile.LoadFile("../examples/wrapper-service/browser-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evidenceData, err := os.ReadFile("../examples/wrapper-service/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []evidence.Record
	if err := json.Unmarshal(evidenceData, &records); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(prof, records, time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("verify example overlay: %v", err)
	}
}

// TestNewInitializesOperationMappings verifies New() produces a non-nil map.
func TestNewInitializesOperationMappings(t *testing.T) {
	s := New()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	// OperationMappings must serialize as {} not null
	if string(data) == "" {
		t.Fatal("empty marshal output")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	om, ok := raw["operationMappings"]
	if !ok {
		t.Fatal("operationMappings key missing from JSON")
	}
	if om == nil {
		t.Error("operationMappings serialized as null; expected {}")
	}
}

// TestWrapperServiceExampleProfileValid validates the wrapper-service example
// browser-profile against the embedded uws.browser.1.5 schema.
func TestWrapperServiceExampleProfileValid(t *testing.T) {
	path := "../examples/wrapper-service/browser-profile.yaml"
	if _, err := profile.LoadFile(path); err != nil {
		t.Errorf("wrapper-service example profile failed validation: %v", err)
	}
}
func TestLifecycleConstants(t *testing.T) {
	cases := []struct {
		lc   Lifecycle
		want string
	}{
		{LifecycleDraft, "draft"},
		{LifecycleReviewed, "reviewed"},
		{LifecycleExported, "exported"},
		{LifecycleStale, "stale"},
	}
	for _, c := range cases {
		data, err := json.Marshal(c.lc)
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		if got != `"`+c.want+`"` {
			t.Errorf("Lifecycle %v: got JSON %s, want %q", c.lc, got, c.want)
		}
	}
}

func TestSidecarVerifyRejectsMalformedMetadataAndBindings(t *testing.T) {
	data, err := os.ReadFile("../examples/wrapper-service/overlay.json")
	if err != nil {
		t.Fatal(err)
	}
	var base Sidecar
	if err := json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	prof, err := profile.LoadFile("../examples/wrapper-service/browser-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	evidenceData, err := os.ReadFile("../examples/wrapper-service/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var records []evidence.Record
	if err := json.Unmarshal(evidenceData, &records); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*Sidecar)
	}{
		{name: "version", mutate: func(s *Sidecar) { s.OverlayVersion = "2" }},
		{name: "id whitespace", mutate: func(s *Sidecar) { s.OverlayID = " overlay " }},
		{name: "bad reference", mutate: func(s *Sidecar) { s.WrapperOpenAPI = "https://user:pass@example.test/openapi" }},
		{name: "empty mappings", mutate: func(s *Sidecar) { s.OperationMappings = map[string]OperationMapping{} }},
		{name: "blank operation", mutate: func(s *Sidecar) {
			s.OperationMappings = map[string]OperationMapping{" ": {OpenAPIOperationID: " ", ProfileActionName: "get_status"}}
		}},
		{name: "mapping mismatch", mutate: func(s *Sidecar) {
			mapping := s.OperationMappings["getStatus"]
			mapping.OpenAPIOperationID = "other"
			s.OperationMappings["getStatus"] = mapping
		}},
		{name: "unknown action", mutate: func(s *Sidecar) {
			mapping := s.OperationMappings["getStatus"]
			mapping.ProfileActionName = "missing"
			s.OperationMappings["getStatus"] = mapping
		}},
		{name: "invalid lifecycle", mutate: func(s *Sidecar) { s.Lifecycle = "unknown" }},
		{name: "review binding", mutate: func(s *Sidecar) { s.ReviewBundle.ProfileDigest = "sha256:tampered" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			var candidate Sidecar
			if err := json.Unmarshal(wire, &candidate); err != nil {
				t.Fatal(err)
			}
			test.mutate(&candidate)
			if err := candidate.Verify(prof, records, now); err == nil {
				t.Fatal("expected verification failure")
			}
		})
	}
	if err := base.Verify(prof, records, time.Time{}); err == nil {
		t.Fatal("expected zero-time failure")
	}
}
