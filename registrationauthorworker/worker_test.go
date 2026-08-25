package registrationauthorworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/capture"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

var workerTime = time.Date(2026, 8, 25, 0, 1, 0, 123456789, time.UTC)

func TestRunCompletesProtocolAndFinalizesPrivateResult(t *testing.T) {
	root := privateRoot(t)
	input := io.NopCloser(bytes.NewReader(completeProtocol(t)))
	var output bytes.Buffer
	browser := &workerBrowser{session: &workerSession{}}
	var driverDirectory string
	err := run(context.Background(), Options{
		PrivateRoot: root, DriverDirectory: "/installed/playwright", Stdin: input, Stdout: &output,
	}, func() time.Time { return workerTime }, func(value string) registrationauthorsession.Browser {
		driverDirectory = value
		return browser
	})
	if err != nil {
		t.Fatalf("run() error = %v\n%s", err, output.String())
	}
	if driverDirectory != "/installed/playwright" || browser.session.closeCalls != 1 {
		t.Fatalf("driver=%q close calls=%d", driverDirectory, browser.session.closeCalls)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("private entries=%v error=%v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("private result info=%v error=%v", info, err)
	}
	data, err := os.ReadFile(filepath.Join(root, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	result, err := registrationauthorresult.Decode(data, workerTime.Truncate(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.Network.MutationRequests != 0 || result.Network.SubmitExecuted || result.Network.AccountAttempted || result.Network.SessionEstablished {
		t.Fatalf("unsafe result posture: %#v", result.Network)
	}
	for _, forbidden := range []string{root, entries[0].Name(), "credentialValue", "accountIdentifier", "privatePath", "rawWorkerOutput"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("protocol output disclosed %q: %s", forbidden, output.String())
		}
	}
	if !strings.Contains(output.String(), `"type":"hello"`) || !strings.Contains(output.String(), `"phase":"closed"`) {
		t.Fatalf("protocol output=%q", output.String())
	}
}

func TestRunCloseBeforeStartCreatesNoResult(t *testing.T) {
	root := privateRoot(t)
	var output bytes.Buffer
	err := run(context.Background(), Options{
		PrivateRoot: root,
		Stdin: io.NopCloser(strings.NewReader(protocolLine(registrationauthorsession.ClientMessage{
			Protocol: registrationauthorsession.Protocol, Type: "close",
		}))),
		Stdout: &output,
	}, func() time.Time { return workerTime }, func(string) registrationauthorsession.Browser {
		return &workerBrowser{}
	})
	if err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("explicit close entries=%v error=%v", entries, err)
	}
}

func TestRunCancellationInterruptsReadAndClosesSession(t *testing.T) {
	root := privateRoot(t)
	input, writer := io.Pipe()
	defer writer.Close()
	session := &workerSession{closed: make(chan struct{})}
	browser := &workerBrowser{session: session, opened: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var output bytes.Buffer
	go func() {
		done <- run(ctx, Options{PrivateRoot: root, Stdin: input, Stdout: &output},
			func() time.Time { return workerTime }, func(string) registrationauthorsession.Browser { return browser })
	}()
	if _, err := io.WriteString(writer, protocolLine(registrationauthorsession.ClientMessage{
		Protocol: registrationauthorsession.Protocol, Type: "start", ProfileID: "synthetic_registration",
		URL: "https://app.example.test/register", Origins: []string{"https://app.example.test"},
	})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-browser.opened:
	case <-time.After(time.Second):
		t.Fatal("registration worker did not open the session")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled worker returned nil")
		}
	case <-time.After(time.Second):
		t.Fatal("registration worker remained blocked on input")
	}
	select {
	case <-session.closed:
	default:
		t.Fatal("registration session was not closed before return")
	}
	if !strings.Contains(output.String(), `"code":"canceled"`) || browser.session.closeCalls != 1 {
		t.Fatalf("output=%q close calls=%d", output.String(), browser.session.closeCalls)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("canceled entries=%v error=%v", entries, err)
	}
}

func TestRunFinalizationFailureLeavesNoArtifactAndNoPrivateOutput(t *testing.T) {
	root := t.TempDir() // mode 0755: deliberately not an owner-only private root.
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := run(context.Background(), Options{
		PrivateRoot: root, Stdin: io.NopCloser(bytes.NewReader(completeProtocol(t))), Stdout: &output,
	}, func() time.Time { return workerTime }, func(string) registrationauthorsession.Browser {
		return &workerBrowser{session: &workerSession{}}
	})
	if err == nil {
		t.Fatal("run() accepted public result root")
	}
	if strings.Contains(output.String(), root) || strings.Contains(output.String(), err.Error()) {
		t.Fatalf("protocol disclosed private failure: %q", output.String())
	}
	if entries, readErr := os.ReadDir(root); readErr != nil || len(entries) != 0 {
		t.Fatalf("failed finalization entries=%v error=%v", entries, readErr)
	}
}

func TestRunOutputFailureClosesInputWithoutOpeningBrowser(t *testing.T) {
	root := privateRoot(t)
	input := &trackingReadCloser{Reader: strings.NewReader("")}
	browser := &workerBrowser{}
	err := run(context.Background(), Options{PrivateRoot: root, Stdin: input, Stdout: failingWriter{}},
		func() time.Time { return workerTime }, func(string) registrationauthorsession.Browser { return browser })
	if err == nil || !input.closed || browser.openCalls != 0 {
		t.Fatalf("error=%v input closed=%v open calls=%d", err, input.closed, browser.openCalls)
	}
}

func TestRunProductionPreflightIsReadOnlyAndCloseOnly(t *testing.T) {
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", "")
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	driver := installedDriver(t)
	root := privateRoot(t)
	var output bytes.Buffer
	err := Run(context.Background(), Options{
		PrivateRoot: root, DriverDirectory: driver,
		Stdin: io.NopCloser(strings.NewReader(protocolLine(registrationauthorsession.ClientMessage{
			Protocol: registrationauthorsession.Protocol, Type: "close",
		}))), Stdout: &output,
	})
	if err != nil || !strings.Contains(output.String(), `"phase":"closed"`) {
		t.Fatalf("Run() error=%v output=%q", err, output.String())
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("preflight close entries=%v error=%v", entries, err)
	}
}

func TestRunRejectsMissingBoundaryBeforeBrowserConstruction(t *testing.T) {
	calls := 0
	if err := run(context.Background(), Options{}, func() time.Time { return workerTime }, func(string) registrationauthorsession.Browser {
		calls++
		return &workerBrowser{}
	}); err == nil || calls != 0 {
		t.Fatalf("error=%v browser constructions=%d", err, calls)
	}
	if err := Run(context.Background(), Options{}); err == nil {
		t.Fatal("Run() accepted missing boundary")
	}
}

func TestWorkerSourceExposesNoPlaywrightOrEnvironmentSurface(t *testing.T) {
	data, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"playwright-go", "playwright.Page", "playwright.Browser", "os.Environ", "os.Getenv"} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("worker source exposes forbidden surface %q", forbidden)
		}
	}
}

func completeProtocol(t *testing.T) []byte {
	t.Helper()
	profileData, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profileValue, err := registrationprofile.Parse(profileData)
	if err != nil {
		t.Fatal(err)
	}
	profileData, err = registrationprofile.MarshalJSON(profileValue)
	if err != nil {
		t.Fatal(err)
	}
	buttonID := candidateID(1, "button", "Register", 0)
	return []byte(strings.Join([]string{
		protocolLine(registrationauthorsession.ClientMessage{
			Protocol: registrationauthorsession.Protocol, Type: "start", ProfileID: "synthetic_registration",
			URL: "https://app.example.test/register", Origins: []string{"https://app.example.test"},
		}),
		protocolLine(registrationauthorsession.ClientMessage{Protocol: registrationauthorsession.Protocol, Type: "observe"}),
		protocolLine(registrationauthorsession.ClientMessage{
			Protocol: registrationauthorsession.Protocol, Type: "review", Profile: profileData,
			CandidateIDs: []string{buttonID}, Flow: "create_dedicated_test_user", CleanupDisposition: "delete_separately",
		}),
		protocolLine(registrationauthorsession.ClientMessage{Protocol: registrationauthorsession.Protocol, Type: "finish"}),
	}, ""))
}

func protocolLine(message registrationauthorsession.ClientMessage) string {
	data, _ := json.Marshal(message)
	return string(data) + "\n"
}

func candidateID(generation int, role, label string, index int) string {
	value := []byte(strings.Join([]string{jsonNumber(generation), role, label, jsonNumber(index)}, "\x00"))
	sum := sha256.Sum256(value)
	return "candidate-" + hex.EncodeToString(sum[:8])
}

func jsonNumber(value int) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func privateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func installedDriver(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "package"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node"), []byte("synthetic node"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package", "cli.js"), []byte("// synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := `{"name":"playwright-core","version":"` + capture.PlaywrightVersion + `"}`
	if err := os.WriteFile(filepath.Join(root, "package", "package.json"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

type workerBrowser struct {
	session   *workerSession
	opened    chan struct{}
	openCalls int
}

func (b *workerBrowser) Open(context.Context, registrationauthorsession.BrowserRequest) (registrationauthorsession.Session, error) {
	b.openCalls++
	if b.opened != nil {
		close(b.opened)
	}
	if b.session == nil {
		return nil, errors.New("unexpected browser launch")
	}
	return b.session, nil
}

type workerSession struct {
	mu         sync.Mutex
	closeCalls int
	closed     chan struct{}
}

func (*workerSession) Observe(context.Context) (registrationauthorsession.RawObservation, error) {
	return registrationauthorsession.RawObservation{
		Origin: "https://app.example.test", Path: "/register",
		Candidates: []registrationauthorsession.RawCandidate{
			{Role: "button", Label: "Register", Matches: 1},
			{Role: "textbox", Label: "Password", Matches: 1},
		},
		Diagnostics: []string{"synthetic_fixture"},
	}, nil
}

func (*workerSession) Navigate(context.Context, registrationauthorsession.Navigation) error {
	return nil
}

func (s *workerSession) Close(context.Context) (registrationauthorsession.NetworkSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	if s.closed != nil && s.closeCalls == 1 {
		close(s.closed)
	}
	return registrationauthorsession.NetworkSummary{Requests: 1, GETRequests: 1}, nil
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error { r.closed = true; return nil }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic output failure") }
