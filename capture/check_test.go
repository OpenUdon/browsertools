package capture

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/profile"
)

func TestCheckUsesBoundedAcquisitionAndReturnsValueFreeReport(t *testing.T) {
	prof := validCheckProfile()
	observation := validObservation()
	observation.ProbeResults = []ProbeResult{
		{ID: "P001", Matches: 1},
		{ID: "P002", Reached: true},
		{ID: "P003", Matches: 1, ObservedType: profile.OutputString},
	}
	fake := &fakeAcquirer{observation: observation}
	request := validCheckRequest(prof)
	result, err := Check(context.Background(), fake, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Version != LiveCheckVersion || len(result.Checks) != 3 || len(result.ProfileDigest) != len("sha256:")+64 {
		t.Fatalf("result = %#v", result)
	}
	if fake.calls != 1 || len(fake.request.Probes) != 3 {
		t.Fatalf("backend calls=%d probes=%#v", fake.calls, fake.request.Probes)
	}
	for _, check := range result.Checks {
		if strings.Contains(check.Message, "active") || strings.Contains(check.Message, "Refresh") {
			t.Fatalf("page value leaked into report: %#v", check)
		}
	}
}

func TestCheckReportsLocatorWaitAndOutputFailures(t *testing.T) {
	prof := validCheckProfile()
	observation := validObservation()
	observation.ProbeResults = []ProbeResult{
		{ID: "P001", Matches: 2},
		{ID: "P002", FailureCode: "timeout"},
		{ID: "P003", Matches: 1, ObservedType: profile.OutputBoolean},
	}
	result, err := Check(context.Background(), &fakeAcquirer{observation: observation}, validCheckRequest(prof))
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("failed checks reported OK: %#v", result)
	}
	for _, check := range result.Checks {
		if check.OK {
			t.Fatalf("check unexpectedly passed: %#v", check)
		}
	}
}

func TestCheckScopesActionsAndOriginsBeforeBrowser(t *testing.T) {
	prof := validCheckProfile()
	request := validCheckRequest(prof)
	request.Actions = []string{"missing"}
	fake := &fakeAcquirer{observation: validObservation()}
	if _, err := Check(context.Background(), fake, request); err == nil || fake.calls != 0 {
		t.Fatalf("unknown action err=%v calls=%d", err, fake.calls)
	}
	request = validCheckRequest(prof)
	request.Capture.AllowedOrigins = []string{"https://example.test", "https://assets.example.test"}
	if _, err := Check(context.Background(), fake, request); err == nil || !strings.Contains(err.Error(), "outside profile") || fake.calls != 0 {
		t.Fatalf("expanded origin err=%v calls=%d", err, fake.calls)
	}
	request = validCheckRequest(prof)
	request.Capture.Probes = []Probe{{ID: "P001", Kind: ProbeLocator, Locator: &profile.Locator{Role: profile.RoleButton}}}
	if _, err := Check(context.Background(), fake, request); err == nil || !strings.Contains(err.Error(), "caller-supplied") || fake.calls != 0 {
		t.Fatalf("injected probe err=%v calls=%d", err, fake.calls)
	}
}

func TestNormalizeProbesRejectsPlaywrightSelectorLanguageAndSensitiveProperties(t *testing.T) {
	for _, output := range []profile.Output{
		{Type: profile.OutputString, Source: profile.OutputCSS, Selector: `text=Sign in`, FallbackReason: profile.FallbackOther, Validation: profile.JSONSchema{"type": "string"}},
		{Type: profile.OutputString, Source: profile.OutputCSS, Selector: `div >> nth=0`, FallbackReason: profile.FallbackOther, Validation: profile.JSONSchema{"type": "string"}},
		{Type: profile.OutputString, Source: profile.OutputJSONLD, Property: "access_token"},
	} {
		probe := Probe{ID: "P001", Kind: ProbeOutput, OutputKey: "result", Output: &output}
		if _, err := normalizeProbes([]Probe{probe}); err == nil {
			t.Fatalf("expected probe rejection for %#v", output)
		}
	}
}

func TestAcquireRejectsMalformedProbeResultSet(t *testing.T) {
	request := validLiveRequest()
	request.Probes = []Probe{{
		ID: "P001", Kind: ProbeLocator,
		Locator: &profile.Locator{Role: profile.RoleButton, Name: "Refresh"},
	}}
	observation := validObservation()
	if _, err := Acquire(context.Background(), &fakeAcquirer{observation: observation}, request); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete result rejection, got %v", err)
	}
	observation.ProbeResults = []ProbeResult{{ID: "P999", Matches: 1}}
	if _, err := Acquire(context.Background(), &fakeAcquirer{observation: observation}, request); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown result rejection, got %v", err)
	}
	observation = validObservation()
	observation.ProbeResults = []ProbeResult{{ID: "P001", Matches: 1}}
	result, err := Acquire(context.Background(), &fakeAcquirer{observation: observation}, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.JSON) != 0 || result.Fixture.Version != "" || len(result.ProbeResults) != 1 {
		t.Fatalf("live check serialized raw fixture: %#v", result)
	}
}

func validCheckRequest(prof *profile.Profile) LiveCheckRequest {
	return LiveCheckRequest{
		Profile: prof, Actions: []string{"read_status"},
		Capture: LiveRequest{
			URL: "https://example.test/member", AllowedOrigins: []string{"https://example.test"},
			ObservedAt: time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
		},
	}
}

func validCheckProfile() *profile.Profile {
	navigation := profile.NavigationLoad
	return &profile.Profile{
		Schema:          "uws.browser.1.5",
		Info:            profile.Info{Title: "Example dashboard", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Evidence:        profile.Evidence{LearnedAt: "2026-08-16T10:00:00Z", Source: "reviewed_fixture"},
		Confidence:      profile.ConfidenceHigh, ExpiresAfter: "P14D",
		Verification: profile.Verification{LastVerifiedAt: "2026-08-16T10:00:00Z", SuccessfulRuns: 1},
		Actions: map[string]profile.Action{
			"read_status": {
				Sequence: []profile.Step{{
					Kind: profile.StepClick,
					Click: &profile.LocatorStep{
						Locator: profile.Locator{Role: profile.RoleButton, Name: "Refresh"},
						WaitFor: &profile.WaitForCondition{Navigation: &navigation},
					},
				}},
				Outputs: map[string]profile.Output{
					"status": {Type: profile.OutputString, Source: profile.OutputJSONLD, Property: "status"},
				},
				SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
				ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
			},
		},
	}
}
