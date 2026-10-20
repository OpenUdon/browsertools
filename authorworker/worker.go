// Package authorworker runs the Browsertools Chromium author-session worker.
//
// The worker is intentionally importable so a distribution binary can expose
// the same NDJSON protocol while still executing Playwright in an isolated
// child process. Importing this package does not start or install Chromium.
package authorworker

import (
	"context"
	"errors"
	"fmt"
	"github.com/OpenUdon/browsertools/authordiagnostic"
	"io"
	"os"
	"sync"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/capture"
)

// Options contains the complete worker boundary. Stdin and Stdout carry only
// browsertools.author-session.v2 messages. PrivateRoot receives the private
// result envelope; DriverDirectory points at an already installed Playwright
// runtime. StartupDiagnosticSink is opt-in synthetic-only private output; use a
// buffered channel and a finite caller-owned drain. It never carries native wire
// messages and a missing delivery is incomplete evidence.
type Options struct {
	StartupDiagnosticSink chan<- authordiagnostic.StartupFailure
	BlockedScriptOrigin   string
	DiagnosticPath        string
	PrivateRoot           string
	DriverDirectory       string
	Stdin                 io.ReadCloser
	Stdout                io.Writer
}

// Run serves one Chromium author session until completion, cancellation, or a
// fail-closed protocol error.
func Run(ctx context.Context, options Options) (result error) {
	if ctx == nil || options.Stdin == nil || options.Stdout == nil || options.PrivateRoot == "" {
		return fmt.Errorf("author worker private root, stdin, stdout, and context are required")
	}
	var diagnostic *os.File
	if options.DiagnosticPath != "" {
		var err error
		diagnostic, err = authordiagnostic.Reserve(options.DiagnosticPath)
		if err != nil {
			return err
		}
		defer func() {
			result = errors.Join(result, authordiagnostic.WriteV2(diagnostic, result), diagnostic.Close())
		}()
	}
	browser, err := capture.NewPlaywrightAuthorBrowserWithDiagnostics(options.DriverDirectory, options.BlockedScriptOrigin, options.StartupDiagnosticSink)
	if err != nil {
		return err
	}
	if _, err := capture.PreflightPlaywrightDriver(options.DriverDirectory); err != nil {
		closed := &authordiagnostic.Error{Class: authordiagnostic.Class{Stage: "driver", Reason: "failed"}}
		if options.StartupDiagnosticSink != nil {
			authordiagnostic.OfferStartup(options.StartupDiagnosticSink, authordiagnostic.StartupFailure{
				Operation: "preflight", Class: closed.Class, Rejection: authordiagnostic.NoRejection(),
				Types:    authordiagnostic.StartupTypes{Canceled: errors.Is(err, context.Canceled), Deadline: errors.Is(err, context.DeadlineExceeded)},
				RawError: authordiagnostic.CaptureErrorText(err), DriverStderr: authordiagnostic.PrivateCapture{Missing: true},
			})
		}
		return closed
	}
	return run(ctx, options, time.Now, func(string) authorsession.Browser { return browser })
}

func run(ctx context.Context, options Options, clock func() time.Time, newBrowser func(string) authorsession.Browser) error {
	if ctx == nil {
		return fmt.Errorf("author worker context is required")
	}
	if options.Stdin == nil || options.Stdout == nil || options.PrivateRoot == "" || clock == nil || newBrowser == nil {
		return fmt.Errorf("author worker private root, stdin, stdout, and browser dependencies are required")
	}
	var closeOnce sync.Once
	closeInput := func() { closeOnce.Do(func() { _ = options.Stdin.Close() }) }
	completed := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			closeInput()
		case <-completed:
		}
	}()
	err := authorsession.Serve(ctx, options.Stdin, options.Stdout, newBrowser(options.DriverDirectory), authorsession.ServeOptions{
		PrivateRoot: options.PrivateRoot,
		Clock:       clock,
	})
	close(completed)
	<-watcherDone
	closeInput()
	return err
}
