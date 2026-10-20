package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
	playwright "github.com/mxschmitt/playwright-go"
)

type diagnosticBrowserType struct {
	playwright.BrowserType
	launch func() (playwright.Browser, error)
}

func (b diagnosticBrowserType) Launch(...playwright.BrowserTypeLaunchOptions) (playwright.Browser, error) {
	return b.launch()
}

type diagnosticBrowser struct {
	playwright.Browser
	context func() (playwright.BrowserContext, error)
	close   func() error
}

func (b diagnosticBrowser) NewContext(...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
	return b.context()
}
func (b diagnosticBrowser) Close(...playwright.BrowserCloseOptions) error { return b.close() }

type diagnosticContext struct {
	playwright.BrowserContext
	permissions error
	close       func() error
}

func (b diagnosticContext) ClearPermissions() error                              { return b.permissions }
func (b diagnosticContext) Close(...playwright.BrowserContextCloseOptions) error { return b.close() }

func startupRequest() authorsession.BrowserRequest {
	return authorsession.BrowserRequest{URL: "https://example.test/login", ApprovedOrigins: []string{"https://example.test"}, NavigationTimeout: time.Second, TotalTimeout: 2 * time.Second, MaxRequests: 10, MaxResponseBytes: 1024, MaxCandidates: 10}
}

func TestAuthorStartupCapturesOriginalBeforeCleanupAndFlattening(t *testing.T) {
	for _, operation := range []string{"driver", "launch", "context", "permissions"} {
		t.Run(operation, func(t *testing.T) {
			sink := make(chan authordiagnostic.StartupFailure, 1)
			original := fmt.Errorf("SYNTHETIC_ORIGINAL: %w", playwright.ErrTargetClosed)
			var stderr io.Writer
			cleanup := 0
			checkCleanup := func() error {
				cleanup++
				if len(sink) != 1 {
					t.Error("original snapshot missing before cleanup")
				}
				_, _ = io.WriteString(stderr, "SYNTHETIC_CLEANUP")
				return errors.New("SYNTHETIC_CLOSE_FAILURE")
			}
			b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, stopDriver: func(*playwright.Playwright) error { return checkCleanup() }}
			b.startDriver = func(options *playwright.RunOptions) (*playwright.Playwright, error) {
				stderr = options.Stderr
				_, _ = io.WriteString(stderr, "SYNTHETIC_DRIVER")
				if operation == "driver" {
					return nil, original
				}
				return &playwright.Playwright{Chromium: diagnosticBrowserType{launch: func() (playwright.Browser, error) {
					if operation == "launch" {
						return nil, original
					}
					return diagnosticBrowser{close: checkCleanup, context: func() (playwright.BrowserContext, error) {
						if operation == "context" {
							return nil, original
						}
						return diagnosticContext{permissions: original, close: checkCleanup}, nil
					}}, nil
				}}}, nil
			}
			_, err := b.Open(context.Background(), startupRequest())
			got := <-sink
			if !got.Valid() || got.Operation != operation || !got.Types.TargetClosed || got.Types.Timeout || got.RawError.Text != original.Error() || got.DriverStderr.Text != "SYNTHETIC_DRIVER" {
				t.Fatalf("snapshot %+v", got)
			}
			expectedStage := operation
			if operation == "permissions" {
				expectedStage = "context"
			}
			if got.Class != (authordiagnostic.Class{Stage: expectedStage, Reason: "failed"}) || authordiagnostic.Classify(err) != got.Class {
				t.Fatal("classification changed")
			}
			if errors.Is(err, original) || errors.Is(err, playwright.ErrTargetClosed) || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("original leaked through public error")
			}
			if operation == "driver" && cleanup != 0 || operation != "driver" && cleanup == 0 {
				t.Fatal("cleanup changed")
			}
		})
	}
}

func TestAuthorStartupTypedFactsNeverParseText(t *testing.T) {
	for _, tc := range []struct {
		err   error
		flags authordiagnostic.StartupTypes
	}{
		{fmt.Errorf("wrapped: %w", playwright.ErrTimeout), authordiagnostic.StartupTypes{Timeout: true}},
		{fmt.Errorf("wrapped: %w", playwright.ErrTargetClosed), authordiagnostic.StartupTypes{TargetClosed: true}},
		{fmt.Errorf("wrapped: %w", context.Canceled), authordiagnostic.StartupTypes{Canceled: true}},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), authordiagnostic.StartupTypes{Deadline: true}},
		{errors.New("TimeoutError target closed context canceled deadline exceeded"), authordiagnostic.StartupTypes{}},
	} {
		sink := make(chan authordiagnostic.StartupFailure, 1)
		b := &playwrightAuthorBrowser{startupDiagnosticSink: sink}
		b.captureStartupFailure("launch", "launch", tc.err, nil)
		got := <-sink
		if !got.Valid() || got.Types != tc.flags || !got.DriverStderr.Missing {
			t.Fatalf("facts %+v", got)
		}
	}
}

func TestAuthorStartupPolicyRejectionSurvivesBeforeConversion(t *testing.T) {
	guard := newAuthorNetworkGuard(startupRequest())
	if guard.allowRequest("https://other.test/private", "GET", true, "document") {
		t.Fatal("allowed")
	}
	original := guard.result()
	sink := make(chan authordiagnostic.StartupFailure, 1)
	b := &playwrightAuthorBrowser{startupDiagnosticSink: sink}
	b.captureStartupFailure("guard", "", original, nil)
	got := <-sink
	if !got.Valid() || got.Class != (authordiagnostic.Class{Stage: "policy", Reason: "origin_escape"}) || got.Rejection != authordiagnostic.RejectionOf(original) {
		t.Fatalf("policy %+v", got)
	}
}

type panicStartupError struct{}

func (panicStartupError) Error() string { panic("synthetic formatter") }
func TestAuthorStartupSinkAndCaptureFailuresPreserveOperation(t *testing.T) {
	closed := make(chan authordiagnostic.StartupFailure)
	close(closed)
	full := make(chan authordiagnostic.StartupFailure, 1)
	full <- authordiagnostic.StartupFailure{}
	for _, sink := range []chan authordiagnostic.StartupFailure{nil, closed, full, make(chan authordiagnostic.StartupFailure)} {
		b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, startDriver: func(options *playwright.RunOptions) (*playwright.Playwright, error) {
			if sink == nil && options.Stderr != io.Discard {
				t.Fatal("default stderr changed")
			}
			return nil, panicStartupError{}
		}}
		_, err := b.Open(context.Background(), startupRequest())
		if authordiagnostic.Classify(err) != (authordiagnostic.Class{Stage: "driver", Reason: "failed"}) {
			t.Fatal("original replaced")
		}
	}
	sink := make(chan authordiagnostic.StartupFailure, 1)
	b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, startDriver: func(*playwright.RunOptions) (*playwright.Playwright, error) { return nil, panicStartupError{} }}
	_, _ = b.Open(context.Background(), startupRequest())
	if got := <-sink; !got.RawError.CaptureError || !got.RawError.Missing {
		t.Fatal("capture failure lost")
	}
}

func TestAuthorStartupPerOpenIsolationAndValidation(t *testing.T) {
	sink := make(chan authordiagnostic.StartupFailure, 2)
	count := 0
	b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, startDriver: func(options *playwright.RunOptions) (*playwright.Playwright, error) {
		count++
		_, _ = fmt.Fprintf(options.Stderr, "driver%d", count)
		return nil, errors.New("synthetic")
	}}
	for i := 1; i <= 2; i++ {
		_, _ = b.Open(context.Background(), startupRequest())
		if got := <-sink; got.DriverStderr.Text != fmt.Sprintf("driver%d", i) {
			t.Fatal("cross-Open stderr")
		}
	}
	_, _ = b.Open(context.Background(), authorsession.BrowserRequest{})
	if got := <-sink; got.Operation != "validate" || !got.DriverStderr.Missing || count != 2 {
		t.Fatal("validation started driver")
	}
}

func TestAuthorStartupRealServePreservesNativeFailureBytes(t *testing.T) {
	var input bytes.Buffer
	_ = json.NewEncoder(&input).Encode(authorsession.ClientMessage{Protocol: authorsession.Protocol, Type: "start", Title: "synthetic", URL: "https://example.test/login", DashboardURL: "https://example.test/dashboard", Goal: "dashboard", Origins: []string{"https://example.test"}, GoalPredicate: &authorresult.GoalPredicate{Origin: "https://example.test", Path: "/dashboard", Role: "heading", Label: "Dashboard"}})
	var baseline string
	for _, enabled := range []bool{false, true} {
		var sink chan authordiagnostic.StartupFailure
		if enabled {
			sink = make(chan authordiagnostic.StartupFailure, 1)
		}
		b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, startDriver: func(o *playwright.RunOptions) (*playwright.Playwright, error) {
			_, _ = io.WriteString(o.Stderr, "SYNTHETIC_STDERR")
			return nil, fmt.Errorf("SYNTHETIC_ORIGINAL: %w", playwright.ErrTimeout)
		}}
		var output bytes.Buffer
		err := authorsession.Serve(context.Background(), bytes.NewReader(input.Bytes()), &output, b, authorsession.ServeOptions{PrivateRoot: startupDiagnosticPrivateRoot(t), Clock: func() time.Time { return time.Unix(1, 0).UTC() }})
		if err == nil || !strings.Contains(output.String(), `"code":"browser_failure"`) || strings.Contains(output.String(), "SYNTHETIC") || strings.Contains(err.Error(), "SYNTHETIC") {
			t.Fatal("Serve privacy/failure")
		}
		if !enabled {
			baseline = output.String()
		} else {
			if output.String() != baseline {
				t.Fatal("native bytes changed")
			}
			got := <-sink
			if !got.Types.Timeout || got.RawError.Missing || got.DriverStderr.Text != "SYNTHETIC_STDERR" {
				t.Fatal("private capture lost")
			}
		}
	}
}

type diagnosticOutputFailure struct {
	calls int
	err   error
}

func (w *diagnosticOutputFailure) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > 1 {
		return 0, w.err
	}
	return len(p), nil
}
func TestAuthorStartupSnapshotSurvivesNativeOutputFailure(t *testing.T) {
	sink := make(chan authordiagnostic.StartupFailure, 1)
	b := &playwrightAuthorBrowser{startupDiagnosticSink: sink, startDriver: func(*playwright.RunOptions) (*playwright.Playwright, error) {
		return nil, fmt.Errorf("SYNTHETIC: %w", playwright.ErrTargetClosed)
	}}
	var input bytes.Buffer
	_ = json.NewEncoder(&input).Encode(authorsession.ClientMessage{Protocol: authorsession.Protocol, Type: "start", Title: "synthetic", URL: "https://example.test/login", DashboardURL: "https://example.test/dashboard", Goal: "dashboard", Origins: []string{"https://example.test"}, GoalPredicate: &authorresult.GoalPredicate{Origin: "https://example.test", Path: "/dashboard", Role: "heading", Label: "Dashboard"}})
	writeFailure := errors.New("synthetic output failure")
	output := &diagnosticOutputFailure{err: writeFailure}
	err := authorsession.Serve(context.Background(), &input, output, b, authorsession.ServeOptions{PrivateRoot: startupDiagnosticPrivateRoot(t), Clock: func() time.Time { return time.Unix(1, 0).UTC() }})
	if !errors.Is(err, writeFailure) || authordiagnostic.Classify(err) != (authordiagnostic.Class{Stage: "driver", Reason: "failed"}) {
		t.Fatal("original/output failure lost")
	}
	if got := <-sink; !got.Types.TargetClosed || got.RawError.Missing || !got.DriverStderr.Missing {
		t.Fatal("partial snapshot lost")
	}
}

func startupDiagnosticPrivateRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}
