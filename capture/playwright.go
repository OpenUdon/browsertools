package capture

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	playwright "github.com/mxschmitt/playwright-go"
)

type playwrightRuntime struct {
	driverDirectory string
}

// NewPlaywrightRuntime returns the installed Playwright-Go runtime. An empty
// driverDirectory uses Playwright-Go's documented cache/environment lookup.
func NewPlaywrightRuntime(driverDirectory string) Runtime {
	return &playwrightRuntime{driverDirectory: strings.TrimSpace(driverDirectory)}
}

func (r *playwrightRuntime) Open(ctx context.Context, engine Engine) (Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := ParseEngine(string(engine)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := PreflightPlaywrightDriver(r.driverDirectory); err != nil {
		return nil, err
	}
	options := &playwright.RunOptions{
		DriverDirectory: r.driverDirectory, SkipInstallBrowsers: true, Verbose: false,
		Stdout: io.Discard, Stderr: io.Discard,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	pw, err := playwright.Run(options)
	if err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		_ = pw.Stop()
		return nil, ctxErr
	}
	var executable string
	switch engine {
	case EngineChromium:
		executable = pw.Chromium.ExecutablePath()
	case EngineFirefox:
		executable = pw.Firefox.ExecutablePath()
	case EngineWebKit:
		executable = pw.WebKit.ExecutablePath()
	default:
		_ = pw.Stop()
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
	return &playwrightSession{playwright: pw, executable: executable}, nil
}

type playwrightSession struct {
	playwright *playwright.Playwright
	executable string
}

func (s *playwrightSession) BrowserExecutable() string { return s.executable }

func (s *playwrightSession) Close() error {
	if s == nil || s.playwright == nil {
		return nil
	}
	return s.playwright.Stop()
}
