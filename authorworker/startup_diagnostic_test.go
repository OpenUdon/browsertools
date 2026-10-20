package authorworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/authordiagnostic"
	"github.com/OpenUdon/browsertools/authorsession"
)

func TestRunStartupPreflightDiagnosticIsPrivateAndOptIn(t *testing.T) {
	// Explicit test-owned absent driver prevents execution and ambient discovery.
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	var baseline string
	for _, enabled := range []bool{false, true} {
		var sink chan authordiagnostic.StartupFailure
		if enabled {
			sink = make(chan authordiagnostic.StartupFailure, 1)
		}
		var output bytes.Buffer
		err := Run(context.Background(), Options{PrivateRoot: startupDiagnosticPrivateRoot(t), DriverDirectory: t.TempDir(), Stdin: io.NopCloser(strings.NewReader("")), Stdout: &output, StartupDiagnosticSink: sink})
		if err == nil || output.Len() != 0 || authordiagnostic.Classify(err) != (authordiagnostic.Class{Stage: "driver", Reason: "failed"}) {
			t.Fatal("preflight changed")
		}
		if !enabled {
			baseline = err.Error()
		} else {
			if err.Error() != baseline {
				t.Fatal("public error changed")
			}
			got := <-sink
			if !got.Valid() || got.Operation != "preflight" || got.RawError.Missing || !got.DriverStderr.Missing || strings.Contains(err.Error(), got.RawError.Text) {
				t.Fatal("private preflight capture")
			}
		}
	}
}

func TestRunStartupPreflightIgnoresUnavailableSink(t *testing.T) {
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	closed := make(chan authordiagnostic.StartupFailure)
	close(closed)
	full := make(chan authordiagnostic.StartupFailure, 1)
	full <- authordiagnostic.StartupFailure{}
	for _, sink := range []chan authordiagnostic.StartupFailure{closed, full, make(chan authordiagnostic.StartupFailure)} {
		err := Run(context.Background(), Options{PrivateRoot: startupDiagnosticPrivateRoot(t), DriverDirectory: t.TempDir(), Stdin: io.NopCloser(strings.NewReader("")), Stdout: io.Discard, StartupDiagnosticSink: sink})
		if authordiagnostic.Classify(err) != (authordiagnostic.Class{Stage: "driver", Reason: "failed"}) {
			t.Fatal("sink changed preflight error")
		}
	}
}

type diagnosticWorkerBrowser struct {
	err   error
	count int
}

func (b *diagnosticWorkerBrowser) Open(context.Context, authorsession.BrowserRequest) (authorsession.Session, error) {
	b.count++
	return nil, b.err
}

func TestRunStartupSuccessAndRefusalWireStayIdentical(t *testing.T) {
	// Exercise the real worker/Serve path with a fake browser; close-before-Open is
	// a complete success and malformed start is a refusal with zero Open calls.
	for _, tc := range []struct {
		name       string
		input      string
		diagnostic string
	}{
		{name: "close", input: `{"protocol":"browsertools.author-session.v2","type":"close"}` + "\n"},
		{name: "invalid_start", input: `{"protocol":"browsertools.author-session.v2","type":"start"}` + "\n", diagnostic: "invalid_start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var baseline string
			for _, enabled := range []bool{false, true} {
				var sink chan authordiagnostic.StartupFailure
				if enabled {
					sink = make(chan authordiagnostic.StartupFailure, 1)
				}
				browser := &diagnosticWorkerBrowser{err: errors.New("must not open")}
				var output bytes.Buffer
				err := run(context.Background(), Options{PrivateRoot: startupDiagnosticPrivateRoot(t), Stdin: io.NopCloser(strings.NewReader(tc.input)), Stdout: &output, StartupDiagnosticSink: sink}, nilClock, func(string) authorsession.Browser { return browser })
				if browser.count != 0 || len(sink) != 0 {
					t.Fatal("unexpected startup")
				}
				if !strings.Contains(output.String(), `"type":"hello"`) {
					t.Fatal("Serve never reached protocol hello")
				}
				if tc.diagnostic == "" {
					if err != nil || !strings.Contains(output.String(), `"type":"state","phase":"closed"`) || strings.Contains(output.String(), `"type":"diagnostic"`) {
						t.Fatalf("close did not succeed: err=%v output=%q", err, output.String())
					}
				} else if err == nil || !strings.Contains(output.String(), `"type":"diagnostic"`) || !strings.Contains(output.String(), `"code":"`+tc.diagnostic+`"`) || strings.Contains(output.String(), `"phase":"closed"`) {
					t.Fatalf("start did not reach expected refusal: err=%v output=%q", err, output.String())
				}
				if !enabled {
					baseline = output.String()
				} else if output.String() != baseline {
					t.Fatal("native bytes changed")
				}
			}
		})
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
