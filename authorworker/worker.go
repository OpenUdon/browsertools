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
	Stdin           io.Reader
	Stdout          io.Writer
}

// Run serves one Chromium author session until completion, cancellation, or a
// fail-closed protocol error.
func Run(ctx context.Context, options Options) error {
	return run(ctx, options, time.Now, capture.NewPlaywrightAuthorBrowser)
}

func run(ctx context.Context, options Options, clock func() time.Time, newBrowser func(string) authorsession.Browser) error {
	if ctx == nil {
		return fmt.Errorf("author worker context is required")
	}
	if options.Stdin == nil || options.Stdout == nil || options.PrivateRoot == "" || clock == nil || newBrowser == nil {
		return fmt.Errorf("author worker private root, stdin, stdout, and browser dependencies are required")
	}
	input, closeInput := cancellationAwareInput(ctx, options.Stdin)
	defer closeInput()
	return authorsession.Serve(ctx, input, options.Stdout, newBrowser(options.DriverDirectory), authorsession.ServeOptions{
		PrivateRoot: options.PrivateRoot,
		Clock:       clock,
	})
}

func cancellationAwareInput(ctx context.Context, input io.Reader) (io.Reader, func()) {
	reader, writer := io.Pipe()
	copied := make(chan struct{})
	go func() {
		_, err := io.Copy(writer, input)
		_ = writer.CloseWithError(err)
		close(copied)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
			if closer, ok := input.(io.ReadCloser); ok {
				_ = closer.Close()
			}
		case <-copied:
		}
	}()
	return reader, func() { _ = reader.Close() }
}
