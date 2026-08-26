// Package registrationauthorworker runs the Browsertools Chromium
// registration-author-session worker.
//
// The worker is intentionally importable so a distribution binary can expose
// the same NDJSON protocol from a re-executed, process-group-isolated child.
// Importing this package does not start or install Chromium.
package registrationauthorworker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/OpenUdon/browsertools/capture"
	"github.com/OpenUdon/browsertools/registrationauthorresult"
	"github.com/OpenUdon/browsertools/registrationauthorsession"
)

// Options contains the complete worker boundary. Stdin and Stdout carry only
// browsertools.registration-author-session.v1 messages. PrivateRoot receives
// the private result; DriverDirectory points at an already installed
// Playwright runtime.
type Options struct {
	PrivateRoot     string
	DriverDirectory string
	// Protocol is "v1" (the default) or "v2".
	Protocol string
	Stdin    io.ReadCloser
	Stdout   io.Writer
}

// Run verifies the installed driver and serves one Chromium registration
// authoring session until completion, cancellation, or a fail-closed protocol
// error. It deliberately does not return the private result path or digest.
func Run(ctx context.Context, options Options) error {
	if err := validateBoundary(ctx, options, time.Now, capture.NewPlaywrightRegistrationBrowser); err != nil {
		if options.Stdin != nil {
			_ = options.Stdin.Close()
		}
		return err
	}
	preflight, err := capture.PreflightPlaywrightDriver(options.DriverDirectory)
	if err != nil {
		_ = options.Stdin.Close()
		return err
	}
	options.DriverDirectory = preflight.DriverDirectory
	return run(ctx, options, time.Now, capture.NewPlaywrightRegistrationBrowser)
}

func run(
	ctx context.Context,
	options Options,
	clock func() time.Time,
	newBrowser func(string) registrationauthorsession.Browser,
) error {
	if err := validateBoundary(ctx, options, clock, newBrowser); err != nil {
		if options.Stdin != nil {
			_ = options.Stdin.Close()
		}
		return err
	}
	protocol, err := resolveProtocol(options.Protocol)
	if err != nil {
		_ = options.Stdin.Close()
		return err
	}
	completion, err := registrationauthorsession.Serve(
		ctx, options.Stdin, options.Stdout, newBrowser(options.DriverDirectory),
		registrationauthorsession.ServeOptions{Clock: clock, Protocol: protocol},
	)
	if err != nil {
		return err
	}
	if completion == nil {
		return nil
	}
	createdAt := clock().UTC().Truncate(time.Second)
	if createdAt.IsZero() {
		return errors.New("registration author worker clock is unavailable")
	}
	_, err = registrationauthorresult.FinalizePrivate(registrationauthorresult.FinalizeRequest{
		Completion: completion, CreatedAt: createdAt, AssessmentAt: createdAt,
		PrivateRoot: options.PrivateRoot,
	})
	return err
}

func validateBoundary(
	ctx context.Context,
	options Options,
	clock func() time.Time,
	newBrowser func(string) registrationauthorsession.Browser,
) error {
	if ctx == nil || options.PrivateRoot == "" || options.Stdin == nil || options.Stdout == nil || clock == nil || newBrowser == nil {
		return fmt.Errorf("registration author worker private root, stdin, stdout, context, and browser dependencies are required")
	}
	if _, err := resolveProtocol(options.Protocol); err != nil {
		return err
	}
	return nil
}

func resolveProtocol(value string) (string, error) {
	switch value {
	case "", "v1":
		return registrationauthorsession.ProtocolV1, nil
	case "v2":
		return registrationauthorsession.ProtocolV2, nil
	default:
		return "", errors.New("registration author worker protocol is unsupported")
	}
}
