package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/browsertools/authreview"
	capabilitybundle "github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/cache"
	"github.com/OpenUdon/browsertools/capture"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
)

type cliCaptureAcquirer struct {
	request capture.LiveRequest
	calls   int
	fail    bool
}

func (a *cliCaptureAcquirer) Acquire(_ context.Context, request capture.LiveRequest) (capture.Observation, error) {
	a.calls++
	a.request = request
	probeResults := make([]capture.ProbeResult, 0, len(request.Probes))
	for _, probe := range request.Probes {
		result := capture.ProbeResult{ID: probe.ID}
		switch probe.Kind {
		case capture.ProbeLocator:
			result.Matches = 1
		case capture.ProbeNavigationWait:
			result.Reached = true
		case capture.ProbeOutput:
			result.Matches = 1
			result.ObservedType = probe.Output.Type
		}
		if a.fail {
			result.FailureCode = "probe_failed"
		}
		probeResults = append(probeResults, result)
	}
	return capture.Observation{
		FinalURL: request.URL, ARIASnapshot: "- button \"Refresh\"\n",
		StructuredData: []json.RawMessage{json.RawMessage(`{"status":"active"}`)},
		Network:        playwrightadapter.NetworkSummary{Requests: 1, Responses: 1, ResponseBytes: 512},
		ProbeResults:   probeResults,
	}, nil
}

func TestProfileValidateExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"profile", "validate", "--input", "../../profile/testdata/valid_minimal.yaml"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || strings.TrimSpace(stdout.String()) != "valid" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"profile", "validate", "--input", "../../profile/testdata/invalid_action_missing_sequence.yaml", "--format", "json"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stdout.String(), `"valid":false`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestPlaywrightDoctorFailsOfflineWithoutInstalling(t *testing.T) {
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", "")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"playwright", "doctor", "--driver-dir", t.TempDir(), "--format", "json",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stdout.String(), `"version":"browsertools.playwright-doctor.v1"`) ||
		!strings.Contains(stdout.String(), `"driver_ready":false`) ||
		!strings.Contains(stdout.String(), `"capability_policy":[`) ||
		strings.Contains(stdout.String(), `"capabilities":`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestPlaywrightDoctorRejectsInvalidArgumentsBeforeRuntime(t *testing.T) {
	for _, args := range [][]string{
		{"playwright", "doctor", "--format", "yaml"},
		{"playwright", "doctor", "unexpected"},
		{"playwright", "doctor", "--engine", "chrome"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, strings.NewReader(""), &stdout, &stderr); code != exitUsageOrIO || stderr.Len() == 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestCaptureChromiumStoresOnlyPrivateRawMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	observedAt := time.Date(2026, 8, 16, 1, 2, 3, 0, time.UTC)
	fake := &cliCaptureAcquirer{}
	var stdout, stderr bytes.Buffer
	code := runCaptureChromiumWith([]string{
		"--url", "https://example.test/member", "--allow-origin", "https://example.test",
		"--cache-root", root, "--action-hint", "read_dashboard", "--retain-for", "2h",
	}, &stdout, &stderr, func() time.Time { return observedAt }, func(driverDirectory string) capture.Acquirer {
		if driverDirectory != "" {
			t.Fatalf("driver directory = %q", driverDirectory)
		}
		return fake
	})
	if code != exitOK || fake.calls != 1 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, fake.calls, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Refresh") || strings.Contains(stdout.String(), "example.test") {
		t.Fatalf("private capture leaked to stdout: %s", stdout.String())
	}
	var entry cache.Entry
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != cache.KindPrivateRaw || entry.PublicationEligible || entry.CreatedAt != "2026-08-16T01:02:03Z" ||
		entry.ExpiresAt != "2026-08-16T03:02:03Z" {
		t.Fatalf("entry = %#v", entry)
	}
	store, err := cache.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := store.Get(context.Background(), entry.ID, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"version": "browsertools.playwright-capture.v1"`) ||
		!strings.Contains(string(payload), `"ariaSnapshot": "- button \"Refresh\"\n"`) {
		t.Fatalf("payload = %s", payload)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-16T01:02:03Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || stdout.Len() != 0 || !strings.Contains(stderr.String(), "explicit --out") {
		t.Fatalf("raw stdout code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	out := filepath.Join(t.TempDir(), "capture.json")
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-16T01:02:03Z", "--out", out}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("raw file code=%d stderr=%q", code, stderr.String())
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("raw output mode = %o", info.Mode().Perm())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"evidence", "import", "--adapter", "playwright", "--input", out,
		"--origin", "https://example.test", "--redaction-status", "not_required",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"candidateLocators"`) ||
		!strings.Contains(stdout.String(), `"candidateOutputs"`) || strings.Contains(stdout.String(), `"active"`) {
		t.Fatalf("evidence code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-16T01:02:03Z", "--out", out, "--force"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "cannot overwrite") {
		t.Fatalf("raw force code=%d stderr=%q", code, stderr.String())
	}
}

func TestCaptureChromiumRejectsBadCLIInputsWithoutAcquiring(t *testing.T) {
	tests := [][]string{
		{"--url", "https://example.test", "--cache-root", t.TempDir()},
		{"--url", "https://example.test", "--allow-origin", "https://example.test", "--cache-root", t.TempDir(), "--retain-for", "0s"},
		{"--url", "https://example.test", "--allow-origin", "https://example.test", "--cache-root", t.TempDir(), "unexpected"},
	}
	for _, args := range tests {
		fake := &cliCaptureAcquirer{}
		var stdout, stderr bytes.Buffer
		code := runCaptureChromiumWith(args, &stdout, &stderr, time.Now, func(string) capture.Acquirer { return fake })
		if code != exitUsageOrIO || fake.calls != 0 || stderr.Len() == 0 {
			t.Fatalf("args=%v code=%d calls=%d stderr=%q", args, code, fake.calls, stderr.String())
		}
	}
}

func TestGuideAuthorCLIProducesStrictBundleWithoutMixingPrompts(t *testing.T) {
	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "evidence.json")
	writeJSON(t, evidencePath, []evidence.Record{{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: "2026-08-16T12:00:00Z", RedactionStatus: evidence.RedactionNotRequired,
		CandidateLocators: []evidence.CandidateLocator{{Role: "button", Name: "Look up"}},
		CandidateOutputs:  []evidence.CandidateOutput{{Key: "headline", Type: "string", Source: "jsonld", Property: "headline"}},
		Provenance:        evidence.Provenance{Tool: "synthetic-test", Version: "1"},
	}})
	answers := strings.Join([]string{
		"Example lookup", "example", "no", "O001", "accessibility_snapshot", "high", "P14D", "1",
		"lookup", "Read a status.", "E001", "0", "E001.O001", "1", "click", "E001.L001", "none", "read_only", "no",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"guide", "author", "--evidence", evidencePath, "--at", "2026-08-16T12:00:00Z",
	}, strings.NewReader(answers), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Accessibility locator candidates") || strings.Contains(stdout.String(), "profile title:") {
		t.Fatalf("prompt/output boundary failed stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	var bundle struct {
		Version string        `json:"version"`
		Review  review.Bundle `json:"review"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Version != "browsertools.guided-authoring.v1" || !bundle.Review.Promotable() {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestGuidedEvidenceReaderEnforcesBoundBeforeDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(path, []byte("[123456789]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvidenceStrictBounded(path, strings.NewReader(""), 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected bounded read rejection, got %v", err)
	}
}

func TestLiveCheckChromiumCLIEmitsOnlyValueFreeReport(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.json")
	navigation := profile.NavigationLoad
	prof := profile.Profile{
		Schema:          "uws.browser.1.5",
		Info:            profile.Info{Title: "Example dashboard", Origin: profile.Origins{"https://example.test"}},
		ObservationKind: profile.ObservationAccessibilitySnapshot,
		Evidence:        profile.Evidence{LearnedAt: "2026-08-16T10:00:00Z", Source: "reviewed_fixture"},
		Confidence:      profile.ConfidenceHigh, ExpiresAfter: "P14D",
		Verification: profile.Verification{LastVerifiedAt: "2026-08-16T10:00:00Z", SuccessfulRuns: 1},
		Actions: map[string]profile.Action{"read_status": {
			Sequence: []profile.Step{{Kind: profile.StepClick, Click: &profile.LocatorStep{
				Locator: profile.Locator{Role: profile.RoleButton, Name: "Refresh"},
				WaitFor: &profile.WaitForCondition{Navigation: &navigation},
			}}},
			Outputs:            map[string]profile.Output{"status": {Type: profile.OutputString, Source: profile.OutputJSONLD, Property: "status"}},
			SideEffects:        []profile.SideEffect{profile.SideEffectReadOnly},
			ConfirmationPolicy: profile.ConfirmationPolicy{Required: false},
		}},
	}
	writeJSON(t, profilePath, prof)
	fake := &cliCaptureAcquirer{}
	var stdout, stderr bytes.Buffer
	code := runLiveCheckChromiumWith([]string{
		"--profile", profilePath, "--url", "https://example.test/member", "--allow-origin", "https://example.test",
		"--action", "read_status", "--at", "2026-08-16T12:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr, func(string) capture.Acquirer { return fake })
	if code != exitOK || fake.calls != 1 || !strings.Contains(stdout.String(), `"version": "browsertools.live-check.v1"`) ||
		!strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, fake.calls, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Refresh") || strings.Contains(stdout.String(), "active") {
		t.Fatalf("page content leaked into live-check report: %s", stdout.String())
	}

	fake = &cliCaptureAcquirer{fail: true}
	stdout.Reset()
	stderr.Reset()
	code = runLiveCheckChromiumWith([]string{
		"--profile", profilePath, "--url", "https://example.test/member", "--allow-origin", "https://example.test",
		"--action", "read_status", "--at", "2026-08-16T12:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr, func(string) capture.Acquirer { return fake })
	if code != exitRejected || !strings.Contains(stdout.String(), `"ok": false`) {
		t.Fatalf("failed check code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestProfileValidateFromStdin(t *testing.T) {
	data, err := os.ReadFile("../../profile/testdata/valid_minimal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"profile", "validate", "--input", "-"}, bytes.NewReader(data), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAuthenticationProfileDraftReviewCLI(t *testing.T) {
	fixture, err := os.ReadFile("../../authprofile/testdata/valid-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"auth-profile", "validate", "--input", "-", "--at", "2026-08-16T00:00:00Z"}, bytes.NewReader(fixture), &stdout, &stderr)
	if code != exitOK || strings.TrimSpace(stdout.String()) != "valid" {
		t.Fatalf("validate code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.yaml")
	profilePath := filepath.Join(tmp, "member.yaml")
	reviewPath := filepath.Join(tmp, "review.json")
	spec := strings.Replace(string(fixture), "profile: uws.browser-authentication.1.0\n", "", 1)
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"auth-draft", "build", "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("draft code=%d stderr=%q", code, stderr.String())
	}
	value, err := authprofile.LoadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if value.Profile != "uws.browser-authentication.1.0" {
		t.Fatalf("profile = %q", value.Profile)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"auth-review", "bundle", "--profile", profilePath, "--at", "2026-08-16T00:00:00Z", "--out", reviewPath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review code=%d stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed authreview.Bundle
	if err := json.Unmarshal(data, &reviewed); err != nil {
		t.Fatal(err)
	}
	if err := authreview.Verify(&reviewed, mustTime(t, "2026-08-16T00:00:00Z")); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"auth-profile", "validate", "--input", profilePath, "--at", "2026-09-14T00:00:00Z", "--format", "json"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stdout.String(), `"valid":false`) {
		t.Fatalf("stale code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAuthenticationProfileCannotBePublishedAsCapabilityBundle(t *testing.T) {
	fixture := "../../authprofile/testdata/valid-push.yaml"
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"registry", "publish", "--root", t.TempDir(), "--bundle", fixture,
		"--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestEvidenceImportFromStdin(t *testing.T) {
	raw := `{"url":"https://example.test/status","observedAt":"2026-01-01T00:00:00Z","actionHint":"read_status","snapshot":{"role":"status","name":"OK"}}`
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"evidence", "import", "--adapter", "playwright", "--input", "-",
		"--origin", "https://example.test", "--redaction-status", "not_required",
	}, strings.NewReader(raw), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	var records []evidence.Record
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ActionHint != "read_status" {
		t.Fatalf("unexpected records: %+v", records)
	}
	if strings.Contains(stdout.String(), "RawData") {
		t.Fatal("raw input leaked into normalized evidence")
	}
}

func TestEvidenceImportRequiresExplicitSafeRedaction(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"evidence", "import", "--adapter", "playwright", "--input", "-",
		"--origin", "https://example.test", "--redaction-status", "pending",
	}, strings.NewReader(`{}`), &stdout, &stderr)
	if code != exitUsageOrIO {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFullFilePipelineAndOverwriteProtection(t *testing.T) {
	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "evidence.json")
	specPath := filepath.Join(tmp, "spec.yaml")
	profilePath := filepath.Join(tmp, "profile.yaml")
	bundlePath := filepath.Join(tmp, "bundle.json")
	reportPath := filepath.Join(tmp, "report.json")

	records := []evidence.Record{{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: "2026-01-01T00:00:00Z", ActionHint: "read_status",
		CandidateLocators: []evidence.CandidateLocator{{Role: "status", Name: "OK"}},
		RedactionStatus:   evidence.RedactionNotRequired, Provenance: evidence.Provenance{Tool: "synthetic"},
	}}
	writeJSON(t, evidencePath, records)
	spec := `info:
  title: Example
  origin: https://example.test
observationKind: accessibility_snapshot
confidence: medium
expiresAfter: P30D
actions:
  read_status:
    sequence:
      - navigate: /status
      - wait_for: {role: status, name: OK}
    sideEffects: [read_only]
    confirmationPolicy: {required: false}
`
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"draft", "build", "--evidence", evidencePath, "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("draft code=%d stderr=%q", code, stderr.String())
	}
	if _, err := profile.LoadFile(profilePath); err != nil {
		t.Fatal(err)
	}

	stderr.Reset()
	code = run([]string{"draft", "build", "--evidence", evidencePath, "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "refusing to overwrite") {
		t.Fatalf("overwrite code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"review", "bundle", "--profile", profilePath, "--evidence", evidencePath, "--at", "2026-01-02T00:00:00Z", "--out", bundlePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review code=%d stderr=%q", code, stderr.String())
	}
	var bundle review.Bundle
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable() {
		t.Fatalf("bundle not promotable: %+v", bundle.Gaps)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"revalidate", "check", "--profile", profilePath, "--evidence", evidencePath, "--at", "2026-01-02T00:00:00Z", "--out", reportPath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("revalidate code=%d stderr=%q", code, stderr.String())
	}
}

func TestCacheCLIPrivateLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"cache", "put", "--root", root, "--input", "-",
		"--kind", "normalized_evidence", "--media-type", "application/json",
		"--created-at", "2026-08-15T00:00:00Z", "--expires-at", "2026-08-16T00:00:00Z",
		"--source", "playwright", "--annotation", "title=Example", "--publication-eligible",
	}, strings.NewReader(`{"safe":true}`), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("put code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var entry cache.Entry
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != cache.KindNormalizedEvidence || !entry.PublicationEligible {
		t.Fatalf("entry = %#v", entry)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "list", "--root", root, "--at", "2026-08-15T12:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), entry.ID) {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-15T12:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || stdout.String() != `{"safe":true}` {
		t.Fatalf("get code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-16T00:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "expired") {
		t.Fatalf("expired get code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "prune", "--root", root, "--at", "2026-08-16T00:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), entry.ID) {
		t.Fatalf("prune code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCacheCLIRejectsRawPublicationAndBadAnnotations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	var stdout, stderr bytes.Buffer
	base := []string{
		"cache", "put", "--root", root, "--input", "-", "--kind", "private_raw",
		"--media-type", "application/zip", "--created-at", "2026-08-15T00:00:00Z",
	}
	code := run(append(append([]string(nil), base...), "--publication-eligible"), strings.NewReader("raw"), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "private raw") {
		t.Fatalf("raw publication code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args := append(append([]string(nil), base...), "--annotation", "duplicate=one", "--annotation", "duplicate=two")
	code = run(args, strings.NewReader("raw"), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "duplicated") {
		t.Fatalf("duplicate annotation code=%d stderr=%q", code, stderr.String())
	}
}

func TestCapabilityBundleCLIBuildAndVerify(t *testing.T) {
	fixtureData, err := os.ReadFile("../../testdata/capability-bundles/read-only.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := capabilitybundle.Parse(fixtureData)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.json")
	reviewPath := filepath.Join(tmp, "review.json")
	evidencePath := filepath.Join(tmp, "evidence.json")
	companionPath := filepath.Join(tmp, "workflow.uws.yaml")
	writeJSON(t, profilePath, fixture.Payload.Profile)
	writeJSON(t, reviewPath, fixture.Payload.Review)
	writeJSON(t, evidencePath, fixture.Payload.Evidence)
	if err := os.WriteFile(companionPath, fixture.Payload.Companions[0].Content, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"bundle", "build", "--id", "example/status", "--release", "1.0.0",
		"--profile", profilePath, "--review", reviewPath, "--evidence", evidencePath,
		"--source", "reviewed_synthetic_fixture", "--license", "CC0-1.0",
		"--author", "Bob", "--author", "Alice", "--published-at", "2026-08-16T00:00:00Z",
		"--uws", "workflow.uws.yaml=" + companionPath,
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("build code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), fixtureData) {
		t.Fatal("CLI build did not reproduce the canonical fixture")
	}

	builtPath := filepath.Join(tmp, "bundle.json")
	if err := os.WriteFile(builtPath, stdout.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"bundle", "verify", "--input", builtPath, "--at", "2026-08-16T00:00:00Z", "--format", "json",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"valid":true`) || !strings.Contains(stdout.String(), `"example/status"`) {
		t.Fatalf("verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"bundle", "verify", "--input", builtPath, "--at", "2026-09-14T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "expired") {
		t.Fatalf("expired verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRegistryCLILocalPublishSearchPullAndVerify(t *testing.T) {
	fixturePath := "../../testdata/capability-bundles/read-only.json"
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "registry")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"registry", "publish", "--root", root, "--bundle", fixturePath, "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"reused_blob":false`) {
		t.Fatalf("publish code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "search", "--location", root, "--query", "read_status", "--at", "2026-08-16T00:00:00Z", "--format", "text",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "example/status@1.0.0\tactive") {
		t.Fatalf("search code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "pull", "--location", root, "--id", "example/status", "--release", "1.0.0", "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !bytes.Equal(stdout.Bytes(), fixtureData) {
		t.Fatalf("pull code=%d matches=%t stderr=%q", code, bytes.Equal(stdout.Bytes(), fixtureData), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "verify", "--location", root, "--at", "2026-08-16T00:00:00Z", "--format", "text",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "example/status@1.0.0\tactive") {
		t.Fatalf("verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "publish", "--root", root, "--bundle", fixturePath, "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"reused_entry":true`) {
		t.Fatalf("republish code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRegistryCLIRequiresExplicitNetworkApprovalAndValidCoordinates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"registry", "search", "--location", "https://example.com/catalog", "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "forbids") {
		t.Fatalf("network code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "search", "--location", "https://example.com/catalog", "--at", "2026-08-16T00:00:00Z", "--network", "sometimes",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "never, ask, or allow") {
		t.Fatalf("policy code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "publish", "--root", t.TempDir(), "--bundle", "../../testdata/capability-bundles/read-only.json",
		"--at", "2026-08-16T00:00:00Z", "--supersedes", "invalid",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "ID@RELEASE") {
		t.Fatalf("coordinate code=%d stderr=%q", code, stderr.String())
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
