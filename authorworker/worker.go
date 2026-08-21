// Package authorworker runs the Browsertools Chromium author-session worker.
//
// The worker is intentionally importable so a distribution binary can expose
// the same NDJSON protocol while still executing Playwright in an isolated
// child process. Importing this package does not start or install Chromium.
package authorworker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/capture"
)

// Options contains the complete worker boundary. Stdin and Stdout carry only
// browsertools.author-session.v2 messages. PrivateRoot receives the private
// result envelope; DriverDirectory points at an already installed Playwright
// runtime.
type Options struct {
	PrivateRoot     string
	DriverDirectory string
	Stdin           io.ReadCloser
	Stdout          io.Writer
}

// Run serves one Chromium author session until completion, cancellation, or a
// fail-closed protocol error.
func Run(ctx context.Context, options Options) error {
	if ctx == nil || options.Stdin == nil || options.Stdout == nil || options.PrivateRoot == "" {
		return fmt.Errorf("author worker private root, stdin, stdout, and context are required")
	}
	if _, err := capture.PreflightPlaywrightDriver(options.DriverDirectory); err != nil {
		return err
	}
	return run(ctx, options, time.Now, capture.NewPlaywrightAuthorBrowser)
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
