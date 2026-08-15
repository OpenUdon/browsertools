package overlay

import (
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
		data, _ := json.Marshal(c.lc)
		got := string(data)
		if got != `"`+c.want+`"` {
			t.Errorf("Lifecycle %v: got JSON %s, want %q", c.lc, got, c.want)
		}
	}
}
