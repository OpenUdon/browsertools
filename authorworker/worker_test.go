package authorworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/authorresult"
	"github.com/OpenUdon/browsertools/authorsession"
)

func TestRunUsesWorkerBoundaryAndDriver(t *testing.T) {
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	browser := &workerBrowser{}
	var driverDirectory string
	err := run(context.Background(), Options{
		PrivateRoot:     privateRoot,
		DriverDirectory: "/installed/playwright",
		Stdin:           io.NopCloser(strings.NewReader(`{"protocol":"browsertools.author-session.v2","type":"close"}` + "\n")),
		Stdout:          &output,
	}, nilClock, func(value string) authorsession.Browser {
		driverDirectory = value
		return browser
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output.String(), `"type":"hello"`) {
		t.Fatalf("worker output = %q", output.String())
	}
	if driverDirectory != "/installed/playwright" {
		t.Fatalf("driver directory = %q", driverDirectory)
	}
}

func TestRunCancellationInterruptsBlockedProtocolReadAndClosesSession(t *testing.T) {
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	input, inputWriter := io.Pipe()
	session := &workerSession{closed: make(chan struct{})}
	browser := &workerBrowser{session: session, opened: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var output bytes.Buffer
	go func() {
		done <- run(ctx, Options{PrivateRoot: privateRoot, Stdin: input, Stdout: &output}, nilClock, func(string) authorsession.Browser { return browser })
	}()
	start := authorsession.ClientMessage{
		Protocol: authorsession.Protocol, Type: "start", Title: "member", URL: "https://members.example.test/login",
		DashboardURL: "https://members.example.test/dashboard", Goal: "Open dashboard", Origins: []string{"https://members.example.test"},
		GoalPredicate: &authorresult.GoalPredicate{Origin: "https://members.example.test", Path: "/dashboard", Context: "main", Role: "heading", Label: "Dashboard"},
	}
	if err := json.NewEncoder(inputWriter).Encode(start); err != nil {
		t.Fatal(err)
	}
	select {
	case <-browser.opened:
	case <-time.After(time.Second):
		t.Fatal("worker did not open the author session")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run remained blocked on protocol input after cancellation")
	}
	select {
	case <-session.closed:
	default:
		t.Fatal("author session was not closed before Run returned")
	}
	if !strings.Contains(output.String(), `"code":"canceled"`) || strings.Contains(output.String(), `"code":"protocol_limit"`) {
		t.Fatalf("cancellation diagnostic = %q", output.String())
	}
}

func TestRunRejectsMissingBoundary(t *testing.T) {
	if err := Run(context.Background(), Options{}); err == nil {
		t.Fatal("Run() error = nil, want boundary error")
	}
}

func nilClock() time.Time { return time.Unix(1, 0).UTC() }

type workerBrowser struct {
	session authorsession.Session
	opened  chan struct{}
}

func (b *workerBrowser) Open(context.Context, authorsession.BrowserRequest) (authorsession.Session, error) {
	if b.opened != nil {
		close(b.opened)
	}
	return b.session, nil
}

type workerSession struct {
	once   sync.Once
	closed chan struct{}
}

func (*workerSession) Observe(context.Context, string) (authorsession.RawObservation, error) {
	return authorsession.RawObservation{}, nil
}
func (*workerSession) Focus(context.Context, authorsession.BrowserAction) error { return nil }
func (*workerSession) Execute(context.Context, authorsession.BrowserAction) (authorsession.Execution, error) {
	return authorsession.Execution{}, nil
}
func (*workerSession) AddOrigin(string) error { return nil }
func (s *workerSession) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}
