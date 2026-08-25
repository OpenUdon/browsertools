package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/authassist"
	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/browsertools/authreview"
	capabilitybundle "github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/cache"
	"github.com/OpenUdon/browsertools/capture"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/registrationprofile"
	"github.com/OpenUdon/browsertools/registrationreview"
	"github.com/OpenUdon/browsertools/registry"
	"github.com/OpenUdon/browsertools/review"
	"github.com/OpenUdon/uws/browserauthentication"
)

type cliCaptureAcquirer struct {
	request capture.LiveRequest
	calls   int
	fail    bool
}

type cliAuthorBrowser struct{}

func (*cliAuthorBrowser) Open(context.Context, authorsession.BrowserRequest) (authorsession.Session, error) {
	return nil, errors.New("unexpected browser launch")
}

type cliClosingAuthorBrowser struct{ session authorsession.Session }

func (b *cliClosingAuthorBrowser) Open(context.Context, authorsession.BrowserRequest) (authorsession.Session, error) {
	return b.session, nil
}

type cliClosingAuthorSession struct{ closeErr error }

func (*cliClosingAuthorSession) Observe(context.Context, string) (authorsession.RawObservation, error) {
	return authorsession.RawObservation{}, nil
}
func (*cliClosingAuthorSession) Focus(context.Context, authorsession.BrowserAction) error { return nil }
func (*cliClosingAuthorSession) Execute(context.Context, authorsession.BrowserAction) (authorsession.Execution, error) {
	return authorsession.Execution{}, nil
}
func (*cliClosingAuthorSession) AddOrigin(string) error { return nil }
func (s *cliClosingAuthorSession) Close() error         { return s.closeErr }

func TestAuthorSessionChromiumCLIUsesNDJSONAndGenericFailure(t *testing.T) {
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	driverDirectory := ""
	code := runAuthorSessionChromiumWith(
		[]string{"--private-root", privateRoot, "--driver-dir", "/installed/driver"},
		strings.NewReader(`{"protocol":"browsertools.author-session.v2","type":"close"}`+"\n"),
		&stdout, &stderr, time.Now,
		func(value string) authorsession.Browser { driverDirectory = value; return &cliAuthorBrowser{} },
	)
	if code != exitOK || driverDirectory != "/installed/driver" || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), `"type":"hello"`) || !strings.Contains(stdout.String(), `"phase":"closed"`) {
		t.Fatalf("code=%d driver=%q stdout=%q stderr=%q", code, driverDirectory, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runAuthorSessionChromiumWith(
		[]string{"--private-root", privateRoot}, strings.NewReader("not-json\n"), &stdout, &stderr,
		time.Now, func(string) authorsession.Browser { return &cliAuthorBrowser{} },
	)
	if code != exitRejected || stderr.String() != "author-session chromium: session failed closed\n" ||
		!strings.Contains(stdout.String(), `"code":"malformed_message"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAuthorSessionChromiumSharedWorkerPreservesMissingRootUsageFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runAuthorSessionChromium(nil, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--private-root") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAuthorSessionChromiumReturnsNonzeroOnTeardownFailure(t *testing.T) {
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	input := protocolLinesForCLI(
		authorsession.ClientMessage{Protocol: authorsession.Protocol, Type: "start", Title: "member", URL: "https://members.example.test/login", DashboardURL: "https://members.example.test/dashboard", Goal: "Open dashboard", Origins: []string{"https://members.example.test"}, GoalPredicate: &authorresult.GoalPredicate{Origin: "https://members.example.test", Path: "/dashboard", Role: "heading", Label: "Dashboard"}},
		authorsession.ClientMessage{Protocol: authorsession.Protocol, Type: "close"},
	)
	var stdout, stderr bytes.Buffer
	code := runAuthorSessionChromiumWith([]string{"--private-root", privateRoot}, strings.NewReader(input), &stdout, &stderr, time.Now, func(string) authorsession.Browser {
		return &cliClosingAuthorBrowser{session: &cliClosingAuthorSession{closeErr: errors.New("teardown failed")}}
	})
	if code != exitRejected || !strings.Contains(stdout.String(), `"code":"teardown_failure"`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func protocolLinesForCLI(messages ...authorsession.ClientMessage) string {
	var output strings.Builder
	for _, message := range messages {
		data, _ := json.Marshal(message)
		output.Write(data)
		output.WriteByte('\n')
	}
	return output.String()
}

type cliRichAcquirer struct {
	request capture.RichBackendRequest
	calls   int
	err     error
}

func (a *cliRichAcquirer) AcquireRich(_ context.Context, request capture.RichBackendRequest) (capture.RichObservation, error) {
	a.calls++
	a.request = request
	if a.err != nil {
		return capture.RichObservation{}, a.err
	}
	artifacts := make([]capture.PrivateArtifact, 0, len(request.Artifacts))
	for _, kind := range request.Artifacts {
		artifact := capture.PrivateArtifact{Kind: kind}
		switch kind {
		case capture.PrivateArtifactScreenshot:
			artifact.MediaType, artifact.Bytes = "image/png", []byte("private-screenshot")
		case capture.PrivateArtifactTrace:
			artifact.MediaType, artifact.Bytes = "application/zip", []byte("private-trace")
		case capture.PrivateArtifactHAR:
			artifact.MediaType, artifact.Bytes = "application/json", []byte(`{"private":"network"}`)
		}
		artifacts = append(artifacts, artifact)
	}
	return capture.RichObservation{
		FinalURL:  request.Capture.URL,
		Network:   playwrightadapter.NetworkSummary{Requests: 1, Responses: 1, ResponseBytes: 128},
		Artifacts: artifacts,
	}, nil
}

type cliAuthBrowser struct {
	requests []authassist.BrowserRequest
	sessions []*cliAuthSession
}

func (b *cliAuthBrowser) Open(_ context.Context, request authassist.BrowserRequest) (authassist.Session, error) {
	b.requests = append(b.requests, request)
	session := &cliAuthSession{origin: "https://members.example.test"}
	b.sessions = append(b.sessions, session)
	return session, nil
}

type cliAuthSession struct {
	origin   string
	active   bool
	closed   bool
	budgets  []int
	endCalls int
}

func (s *cliAuthSession) Navigate(_ context.Context, _ string) error { return nil }
func (s *cliAuthSession) Observe(_ context.Context, locator *browserauthentication.Locator) (authassist.PageObservation, error) {
	result := authassist.PageObservation{Origin: s.origin}
	if locator != nil {
		result.Matches = 1
	}
	return result, nil
}
func (s *cliAuthSession) BeginAuthenticationInteraction(budget int) error {
	s.active = true
	s.budgets = append(s.budgets, budget)
	return nil
}
func (s *cliAuthSession) EndAuthenticationInteraction() (int, error) {
	s.active = false
	s.endCalls++
	if s.budgets[len(s.budgets)-1] > 0 {
		return 1, nil
	}
	return 0, nil
}
func (s *cliAuthSession) Close() error { s.closed = true; return nil }

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

func TestPlaywrightDoctorInvalidFormatDoesNotCreateRuntime(t *testing.T) {
	var calls int
	var stdout, stderr bytes.Buffer
	code := runPlaywrightDoctorWith([]string{"--format", "yaml"}, &stdout, &stderr, func(string) capture.Runtime {
		calls++
		return nil
	})
	if code != exitUsageOrIO || calls != 0 || stdout.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
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

func TestRichCaptureStoresOneFinitePrivateBundleAndDeletesByExactID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	observedAt := time.Date(2026, 8, 16, 4, 5, 6, 7, time.UTC)
	fake := &cliRichAcquirer{}
	var stdout, stderr bytes.Buffer
	code := runRichCaptureChromiumWith([]string{
		"--url", "https://example.test/member", "--allow-origin", "https://example.test",
		"--cache-root", root, "--artifact", "har", "--artifact", "screenshot", "--retain-for", "2h",
	}, &stdout, &stderr, func() time.Time { return observedAt }, func(string) capture.RichAcquirer { return fake })
	if code != exitOK || fake.calls != 1 || strings.Contains(stdout.String(), "private-screenshot") || strings.Contains(stdout.String(), "private\"") {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, fake.calls, stdout.String(), stderr.String())
	}
	if len(fake.request.Artifacts) != 2 || fake.request.Artifacts[0] != capture.PrivateArtifactScreenshot || fake.request.Artifacts[1] != capture.PrivateArtifactHAR {
		t.Fatalf("artifacts = %#v", fake.request.Artifacts)
	}
	var entry cache.Entry
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != cache.KindPrivateRaw || entry.PublicationEligible || entry.MediaType != "application/vnd.openudon.browsertools.private-rich+zip" ||
		entry.ExpiresAt != "2026-08-16T06:05:06.000000007Z" || entry.Annotations["secret_review"] != "pending" ||
		entry.Annotations["artifacts"] != "screenshot,har" {
		t.Fatalf("entry = %#v", entry)
	}
	store, err := cache.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, payload, err := store.Get(context.Background(), entry.ID, observedAt)
	if err != nil || !bytes.HasPrefix(payload, []byte("PK")) {
		t.Fatalf("bundle payload err=%v prefix=%q", err, payload[:min(len(payload), 8)])
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "delete", "--root", root, "--id", entry.ID, "--confirm-id", "sha256:" + strings.Repeat("0", 64)}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || stdout.Len() != 0 {
		t.Fatalf("mismatched confirmation code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, _, err := store.Get(context.Background(), entry.ID, observedAt); err != nil {
		t.Fatalf("mismatched confirmation deleted entry: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "delete", "--root", root, "--id", entry.ID, "--confirm-id", entry.ID}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), entry.ID) {
		t.Fatalf("delete code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, _, err := store.Get(context.Background(), entry.ID, observedAt); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("entry remains after deletion: %v", err)
	}
}

func TestRichCaptureRejectsImplicitOrUnboundedRequestsBeforeBrowser(t *testing.T) {
	for _, args := range [][]string{
		{"--url", "https://example.test", "--allow-origin", "https://example.test", "--cache-root", t.TempDir()},
		{"--url", "https://example.test", "--allow-origin", "https://example.test", "--cache-root", t.TempDir(), "--artifact", "video"},
		{"--url", "https://example.test", "--allow-origin", "https://example.test", "--cache-root", t.TempDir(), "--artifact", "har", "--retain-for", "25h"},
	} {
		fake := &cliRichAcquirer{}
		var stdout, stderr bytes.Buffer
		code := runRichCaptureChromiumWith(args, &stdout, &stderr, time.Now, func(string) capture.RichAcquirer { return fake })
		if code != exitUsageOrIO || fake.calls != 0 || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d calls=%d stdout=%q stderr=%q", args, code, fake.calls, stdout.String(), stderr.String())
		}
	}
}

func TestCacheDeleteDoesNotCreateMistypedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-cache")
	id := "sha256:" + strings.Repeat("0", 64)
	var stdout, stderr bytes.Buffer
	code := run([]string{"cache", "delete", "--root", root, "--id", id, "--confirm-id", id}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache delete created mistyped root: %v", err)
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

func TestPortabilityCLIUsesProcessClockAndExplicitFreshEngines(t *testing.T) {
	checkedAt := time.Date(2026, 8, 16, 7, 8, 9, 10, time.UTC)
	var opened []capture.Engine
	backends := map[capture.Engine]*cliCaptureAcquirer{}
	var stdout, stderr bytes.Buffer
	code := runPortabilityCheckWith([]string{
		"--profile", "../../profile/testdata/valid_minimal.yaml",
		"--url", "https://example.test/member", "--allow-origin", "https://example.test",
		"--action", "read_status", "--engine", "webkit", "--engine", "chromium",
	}, strings.NewReader(""), &stdout, &stderr, func() time.Time { return checkedAt }, func(_ string, engine capture.Engine) capture.Acquirer {
		opened = append(opened, engine)
		backend := &cliCaptureAcquirer{}
		backends[engine] = backend
		return backend
	})
	if code != exitOK || !strings.Contains(stdout.String(), `"version": "browsertools.portability-check.v1"`) ||
		!strings.Contains(stdout.String(), `"checkedAt": "2026-08-16T07:08:09.00000001Z"`) || strings.Contains(stdout.String(), `"OK"`) {
		t.Fatalf("code=%d opened=%v stdout=%q stderr=%q", code, opened, stdout.String(), stderr.String())
	}
	if len(opened) != 2 || opened[0] != capture.EngineChromium || opened[1] != capture.EngineWebKit ||
		backends[capture.EngineChromium] == backends[capture.EngineWebKit] {
		t.Fatalf("opened=%v backends=%#v", opened, backends)
	}
	if !strings.Contains(stdout.String(), `"capability": "popup_context"`) || strings.Contains(stdout.String(), "selector rewrit") {
		t.Fatalf("contract pressure missing or rewrite leaked: %s", stdout.String())
	}
}

func TestPortabilityCLIRejectsIncompleteEngineSelectionBeforeBrowser(t *testing.T) {
	calls := 0
	var stdout, stderr bytes.Buffer
	code := runPortabilityCheckWith([]string{
		"--profile", "../../profile/testdata/valid_minimal.yaml", "--url", "https://example.test/member",
		"--allow-origin", "https://example.test", "--engine", "chromium",
	}, strings.NewReader(""), &stdout, &stderr, time.Now, func(string, capture.Engine) capture.Acquirer {
		calls++
		return &cliCaptureAcquirer{}
	})
	if code != exitUsageOrIO || calls != 0 || stdout.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
}

func TestPlaywrightCapabilitiesRecordsFullContractPressure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"playwright", "capabilities"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), capture.ContractPressureVersion) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, capability := range []string{"popup_context", "iframe_context", "download", "upload", "permission", "visual_interaction"} {
		if !strings.Contains(stdout.String(), `"capability":"`+capability+`"`) {
			t.Fatalf("missing %s in %s", capability, stdout.String())
		}
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

func TestRegistrationProfileDraftReviewCLI(t *testing.T) {
	fixture, err := os.ReadFile("../../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"registration-profile", "validate", "--input", "-", "--at", "2026-08-25T00:00:00Z"}, bytes.NewReader(fixture), &stdout, &stderr)
	if code != exitOK || strings.TrimSpace(stdout.String()) != "valid" {
		t.Fatalf("validate code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.yaml")
	profilePath := filepath.Join(tmp, "registration.yaml")
	reviewPath := filepath.Join(tmp, "review.json")
	spec := strings.Replace(string(fixture), "profile: uws.browser-registration.1.0\n", "", 1)
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"registration-draft", "build", "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("draft code=%d stderr=%q", code, stderr.String())
	}
	value, err := registrationprofile.LoadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if value.Profile != "uws.browser-registration.1.0" {
		t.Fatalf("profile = %q", value.Profile)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"registration-review", "bundle", "--profile", profilePath, "--at", "2026-08-25T00:00:00Z", "--out", reviewPath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review code=%d stderr=%q", code, stderr.String())
	}
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed registrationreview.Bundle
	if err := json.Unmarshal(data, &reviewed); err != nil {
		t.Fatal(err)
	}
	if err := registrationreview.Verify(&reviewed, mustTime(t, "2026-08-25T00:00:00Z")); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"registration-profile", "validate", "--input", profilePath, "--at", "2026-09-24T00:00:00Z", "--format", "json"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stdout.String(), `"valid":false`) {
		t.Fatalf("stale code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestAssistedAuthenticationCLIWritesOnlyClosedLocalBundle(t *testing.T) {
	t.Setenv("MEMBER_PASSWORD", "actual-password-must-not-cross")
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "member.yaml")
	outPath := filepath.Join(tmp, "member.assisted.json")
	fixture, err := os.ReadFile("../../authprofile/testdata/valid-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	browser := &cliAuthBrowser{}
	var stdout, stderr bytes.Buffer
	code := runAuthAssistChromiumWith([]string{
		"--profile", profilePath, "--flow", "member_login_push",
		"--approve-origin", "https://members.example.test", "--approve-origin", "https://login.example.test",
		"--post-budget", "member_login_push:3=2", "--out", outPath,
	}, strings.NewReader("\n\n\n\n"), &stdout, &stderr, func() time.Time {
		return mustTime(t, "2026-08-16T12:00:00Z")
	}, func(driverDirectory string) authassist.Browser {
		if driverDirectory != "" {
			t.Fatalf("driver directory = %q", driverDirectory)
		}
		return browser
	})
	if code != exitOK || stdout.Len() != 0 || len(browser.sessions) != 1 || !browser.sessions[0].closed || browser.sessions[0].active || browser.sessions[0].endCalls != 4 {
		t.Fatalf("code=%d stdout=%q stderr=%q browser=%#v", code, stdout.String(), stderr.String(), browser)
	}
	if len(browser.requests) != 1 || strings.Join(browser.requests[0].ApprovedOrigins, ",") != "https://login.example.test,https://members.example.test" {
		t.Fatalf("browser request = %#v", browser.requests)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("output mode = %o", info.Mode().Perm())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("actual-password-must-not-cross")) || !bytes.Contains(data, []byte(`"version": "browsertools.assisted-authentication.v1"`)) {
		t.Fatalf("bundle = %s", data)
	}
	var bundle authassist.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if err := authassist.Verify(&bundle); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "type no credentials here") {
		t.Fatalf("missing terminal boundary prompt: %q", stderr.String())
	}
}

func TestAssistedAuthenticationCLIRejectsBeforeOrDuringSessionWithoutArtifact(t *testing.T) {
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "member.yaml")
	fixture, err := os.ReadFile("../../authprofile/testdata/valid-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{
		"--profile", profilePath, "--flow", "member_login_push",
		"--approve-origin", "https://members.example.test", "--approve-origin", "https://login.example.test",
	}
	for name, configure := range map[string]func() ([]string, string, *cliAuthBrowser){
		"existing output": func() ([]string, string, *cliAuthBrowser) {
			out := filepath.Join(tmp, "existing.json")
			if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			return append(append([]string{}, base...), "--out", out), "\n", &cliAuthBrowser{}
		},
		"bad budget": func() ([]string, string, *cliAuthBrowser) {
			out := filepath.Join(tmp, "bad-budget.json")
			return append(append([]string{}, base...), "--post-budget", "member_login_push:3=0", "--out", out), "\n", &cliAuthBrowser{}
		},
		"credential terminal input": func() ([]string, string, *cliAuthBrowser) {
			out := filepath.Join(tmp, "terminal-reject.json")
			return append(append([]string{}, base...), "--out", out), "actual-password\n", &cliAuthBrowser{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			args, input, browser := configure()
			var stdout, stderr bytes.Buffer
			code := runAuthAssistChromiumWith(args, strings.NewReader(input), &stdout, &stderr, func() time.Time {
				return mustTime(t, "2026-08-16T12:00:00Z")
			}, func(string) authassist.Browser { return browser })
			if code == exitOK || stdout.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if name == "credential terminal input" {
				if len(browser.sessions) != 1 || !browser.sessions[0].closed || browser.sessions[0].active {
					t.Fatalf("session = %#v", browser.sessions)
				}
				if _, err := os.Stat(filepath.Join(tmp, "terminal-reject.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed run wrote artifact: %v", err)
				}
			} else if len(browser.requests) != 0 {
				t.Fatalf("browser opened before input validation: %#v", browser.requests)
			}
		})
	}
}

func TestPrivateAssistedOutputIsAtomicAndNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "result.json")
	if err := writeNewPrivateOutput(path, []byte("complete\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPrivateOutput(path, []byte("replacement\n")); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("overwrite error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "complete\n" {
		t.Fatalf("output = %q", data)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".browsertools-auth-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files = %v err=%v", matches, err)
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

func TestCommandRegistryAndHelp(t *testing.T) {
	seen := map[string]struct{}{}
	for _, spec := range commandRegistry {
		key := spec.group + " " + spec.name
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate command %q", key)
		}
		seen[key] = struct{}{}
		if spec.summary == "" || spec.run == nil {
			t.Fatalf("incomplete command registration: %#v", spec)
		}
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--help"}, want: "command groups:"},
		{args: []string{"help", "registry"}, want: "registry <command>"},
		{args: []string{"registry", "--help"}, want: "registry <command>"},
		{args: []string{"registry", "search", "--help"}, want: "-location"},
		{args: []string{"help", "registration-profile"}, want: "registration-profile <command>"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(test.args, strings.NewReader(""), &stdout, &stderr); code != exitOK || !strings.Contains(stdout.String(), test.want) {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", test.args, code, stdout.String(), stderr.String())
		}
	}
}

func TestInvalidFormatsAndEnumsPerformNoIOOrNetwork(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	for _, args := range [][]string{
		{"profile", "validate", "--input", missing, "--format", "yaml"},
		{"profile", "validate", "--input", missing, "--format", " json "},
		{"draft", "build", "--evidence", missing, "--spec", missing, "--format", "toml"},
		{"evidence", "import", "--adapter", "unknown", "--input", missing, "--origin", "https://example.test", "--redaction-status", "not_required"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, strings.NewReader("unread"), &stdout, &stderr); code != exitUsageOrIO || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%q stderr=%q", args, code, stdout.String(), stderr.String())
		}
	}

	cacheRoot := filepath.Join(t.TempDir(), "must-not-exist")
	var stdout, stderr bytes.Buffer
	code := run([]string{"cache", "prune", "--root", cacheRoot, "--at", "2026-08-16T00:00:00Z", "--format", " json "}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO {
		t.Fatalf("cache prune code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(cacheRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid format touched cache root: %v", err)
	}

	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client := &registry.Client{HTTPClient: server.Client()}
	stdout.Reset()
	stderr.Reset()
	code = runRegistrySearchWith([]string{
		"--location", server.URL, "--at", "2026-08-16T00:00:00Z", "--network", "allow", "--allow-loopback", "--format", "xml",
	}, &stdout, &stderr, client)
	if code != exitUsageOrIO || requests.Load() != 0 {
		t.Fatalf("registry search code=%d requests=%d stderr=%q", code, requests.Load(), stderr.String())
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
