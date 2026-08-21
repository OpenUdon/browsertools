// Command browsertools provides file-first profile authoring, review, and
// explicitly selected browser acquisition operations.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/adapter"
	"github.com/OpenUdon/browsertools/adapter/crawl4ai"
	"github.com/OpenUdon/browsertools/adapter/firecrawl"
	"github.com/OpenUdon/browsertools/adapter/llmscraper"
	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/authassist"
	"github.com/OpenUdon/browsertools/authdraft"
	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/authorworker"
	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/browsertools/authreview"
	capabilitybundle "github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/cache"
	"github.com/OpenUdon/browsertools/capture"
	"github.com/OpenUdon/browsertools/draft"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/guide"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/registry"
	"github.com/OpenUdon/browsertools/revalidate"
	"github.com/OpenUdon/browsertools/review"
	"gopkg.in/yaml.v3"
)

const (
	exitOK                      = 0
	exitRejected                = 1
	exitUsageOrIO               = 2
	maxGuidedEvidenceInputBytes = int64(16 << 20)
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func runAuthorSessionChromium(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("author-session chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privateRoot := fs.String("private-root", "", "existing mode-0700 directory for the private result envelope")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "author-session chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *privateRoot == "" {
		fmt.Fprintln(stderr, "author-session chromium: --private-root and browser dependencies are required")
		return exitUsageOrIO
	}
	if err := authorworker.Run(context.Background(), authorworker.Options{
		PrivateRoot: *privateRoot, DriverDirectory: *driverDirectory, Stdin: stdin, Stdout: stdout,
	}); err != nil {
		fmt.Fprintln(stderr, "author-session chromium: session failed closed")
		return exitRejected
	}
	return exitOK
}

func runAuthorSessionChromiumWith(
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	clock func() time.Time,
	newBrowser func(string) authorsession.Browser,
) int {
	fs := flag.NewFlagSet("author-session chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	privateRoot := fs.String("private-root", "", "existing mode-0700 directory for the private result envelope")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "author-session chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *privateRoot == "" || clock == nil || newBrowser == nil {
		fmt.Fprintln(stderr, "author-session chromium: --private-root and browser dependencies are required")
		return exitUsageOrIO
	}
	if err := authorsession.Serve(context.Background(), stdin, stdout, newBrowser(*driverDirectory), authorsession.ServeOptions{
		PrivateRoot: *privateRoot,
		Clock:       clock,
	}); err != nil {
		fmt.Fprintln(stderr, "author-session chromium: session failed closed")
		return exitRejected
	}
	return exitOK
}

func runAuthAssistChromium(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runAuthAssistChromiumWith(args, stdin, stdout, stderr, time.Now, capture.NewPlaywrightAuthBrowser)
}

func runAuthAssistChromiumWith(
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	clock func() time.Time,
	newBrowser func(string) authassist.Browser,
) int {
	fs := flag.NewFlagSet("auth-assist chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "explicit authentication profile JSON/YAML path (stdin is reserved for empty-line operator signals)")
	out := fs.String("out", "", "new local assisted-authentication bundle JSON path (written mode 0600)")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	navigationTimeout := fs.Duration("navigation-timeout", authassist.DefaultNavigationTimeout, "per-navigation and locator-check timeout")
	totalTimeout := fs.Duration("timeout", authassist.DefaultTotalTimeout, "total headed authentication deadline")
	maxRequests := fs.Int("max-requests", authassist.DefaultMaxRequests, "maximum requests across each ephemeral flow context")
	maxResponseBytes := fs.Int64("max-response-bytes", authassist.DefaultMaxResponseBytes, "maximum response bytes across each ephemeral flow context")
	var flows stringList
	var approvedOrigins stringList
	var postBudgetFlags stringList
	fs.Var(&flows, "flow", "authentication profile flow to observe; repeatable and explicit")
	fs.Var(&approvedOrigins, "approve-origin", "exact application/authentication origin approved before launch; repeatable")
	fs.Var(&postBudgetFlags, "post-budget", "bounded auth POST authority FLOW:ZERO_BASED_STEP=COUNT; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "auth-assist chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *profilePath == "" || *profilePath == "-" || *out == "" || *out == "-" || len(flows) == 0 || len(approvedOrigins) == 0 {
		fmt.Fprintln(stderr, "auth-assist chromium: --profile (not stdin), --out (not stdout), at least one --flow, and all --approve-origin values are required")
		return exitUsageOrIO
	}
	if strings.ToLower(filepath.Ext(*out)) != ".json" {
		fmt.Fprintln(stderr, "auth-assist chromium: --out must end in .json")
		return exitUsageOrIO
	}
	profileAbsolute, profileErr := filepath.Abs(*profilePath)
	outputAbsolute, outputErr := filepath.Abs(*out)
	if profileErr != nil || outputErr != nil || filepath.Clean(profileAbsolute) == filepath.Clean(outputAbsolute) {
		fmt.Fprintln(stderr, "auth-assist chromium: input profile and output bundle must be different explicit paths")
		return exitUsageOrIO
	}
	if err := validateNewPrivateOutputPath(*out); err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitUsageOrIO
	}
	postBudgets, err := parseAuthPOSTBudgets(postBudgetFlags)
	if err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitUsageOrIO
	}
	prof, err := authprofile.LoadFile(*profilePath)
	if err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitRejected
	}
	if clock == nil || newBrowser == nil {
		fmt.Fprintln(stderr, "auth-assist chromium: headed browser dependency is unavailable")
		return exitUsageOrIO
	}
	observedAt := clock().UTC()
	if observedAt.IsZero() {
		fmt.Fprintln(stderr, "auth-assist chromium: observation clock is unavailable")
		return exitUsageOrIO
	}
	bundle, err := authassist.Run(context.Background(), newBrowser(*driverDirectory), authassist.NewLineOperator(stdin, stderr), authassist.Request{
		Profile: prof, Flows: []string(flows), ApprovedOrigins: []string(approvedOrigins),
		POSTBudgets: postBudgets, ObservedAt: observedAt,
		NavigationTimeout: *navigationTimeout, TotalTimeout: *totalTimeout,
		MaxRequests: *maxRequests, MaxResponseBytes: *maxResponseBytes,
	})
	if err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitRejected
	}
	data, err := authassist.MarshalJSONIndent(bundle)
	if err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitRejected
	}
	if err := writeNewPrivateOutput(*out, data); err != nil {
		fmt.Fprintln(stderr, "auth-assist chromium:", err)
		return exitUsageOrIO
	}
	return exitOK
}

func validateNewPrivateOutputPath(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("refusing to overwrite %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect output directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("output directory must be an existing non-symlink directory")
	}
	return nil
}

func writeNewPrivateOutput(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".browsertools-auth-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	closed := false
	linked := false
	defer func() {
		if !closed {
			closeErr := temporary.Close()
			if closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) && err == nil && !linked {
			err = removeErr
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err := os.Link(temporaryPath, path); errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite %s", path)
	} else if err != nil {
		return err
	}
	linked = true
	return nil
}

func parseAuthPOSTBudgets(values []string) (map[string]int, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]int, len(values))
	for _, raw := range values {
		flow, remainder, ok := strings.Cut(strings.TrimSpace(raw), ":")
		stepText, countText, countOK := strings.Cut(remainder, "=")
		flow, stepText, countText = strings.TrimSpace(flow), strings.TrimSpace(stepText), strings.TrimSpace(countText)
		if !ok || !countOK || flow == "" || stepText == "" || countText == "" {
			return nil, fmt.Errorf("POST budget %q must use FLOW:ZERO_BASED_STEP=COUNT", raw)
		}
		step, stepErr := strconv.Atoi(stepText)
		count, countErr := strconv.Atoi(countText)
		if stepErr != nil || step < 0 || step > 255 || countErr != nil || count < 1 || count > authassist.MaxPOSTRequestsPerStep {
			return nil, fmt.Errorf("POST budget %q has an invalid step or count", raw)
		}
		path := fmt.Sprintf("flows.%s.sequence[%d]", flow, step)
		if _, duplicate := result[path]; duplicate {
			return nil, fmt.Errorf("POST budget for %q is duplicated", path)
		}
		result[path] = count
	}
	return result, nil
}

func runGuideAuthor(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("guide author", flag.ContinueOnError)
	fs.SetOutput(stderr)
	evidencePath := fs.String("evidence", "", "reviewed normalized evidence JSON path (stdin is reserved for wizard answers)")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "guided-authoring bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "guide author: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *evidencePath == "" || *at == "" {
		fmt.Fprintln(stderr, "guide author: --evidence and --at are required")
		return exitUsageOrIO
	}
	if *evidencePath == "-" {
		fmt.Fprintln(stderr, "guide author: --evidence cannot use stdin because stdin carries wizard answers")
		return exitUsageOrIO
	}
	assessedAt, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "guide author: invalid --at:", err)
		return exitUsageOrIO
	}
	records, err := readEvidenceStrictBounded(*evidencePath, stdin, maxGuidedEvidenceInputBytes)
	if err != nil {
		fmt.Fprintln(stderr, "guide author:", err)
		return exitUsageOrIO
	}
	bundle, err := guide.RunWizard(stdin, stderr, records, assessedAt)
	if err != nil {
		fmt.Fprintln(stderr, "guide author:", err)
		return exitRejected
	}
	data, err := guide.MarshalDeterministic(bundle)
	if err != nil {
		fmt.Fprintln(stderr, "guide author:", err)
		return exitRejected
	}
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, "guide author:", err)
		return exitUsageOrIO
	}
	return exitOK
}

func runLiveCheckChromium(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runLiveCheckChromiumWith(args, stdin, stdout, stderr, capture.NewPlaywrightAcquirer)
}

func runLiveCheckChromiumWith(
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	newAcquirer func(string) capture.Acquirer,
) int {
	fs := flag.NewFlagSet("live-check chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "validated browser profile JSON/YAML path or -")
	targetURL := fs.String("url", "", "explicit current-page HTTPS or loopback HTTP URL")
	at := fs.String("at", "", "RFC3339 check time")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	navigationTimeout := fs.Duration("navigation-timeout", capture.DefaultNavigationTimeout, "per-navigation/observation timeout")
	totalTimeout := fs.Duration("timeout", capture.DefaultTotalTimeout, "total live-check deadline")
	maxRequests := fs.Int("max-requests", capture.DefaultMaxRequests, "maximum routed requests")
	maxResponseBytes := fs.Int64("max-response-bytes", capture.DefaultMaxResponseBytes, "maximum total response bytes")
	maxEvidenceBytes := fs.Int64("max-evidence-bytes", capture.DefaultMaxEvidenceBytes, "maximum transient observation bytes")
	ariaDepth := fs.Int("aria-depth", capture.DefaultARIADepth, "maximum transient ARIA snapshot depth")
	out := fs.String("out", "-", "value-free live-check report JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var allowedOrigins stringList
	var actions stringList
	fs.Var(&allowedOrigins, "allow-origin", "exact allowed origin; repeatable")
	fs.Var(&actions, "action", "profile action to check; repeatable, defaults to all")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "live-check chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *profilePath == "" || *targetURL == "" || *at == "" || len(allowedOrigins) == 0 {
		fmt.Fprintln(stderr, "live-check chromium: --profile, --url, --at, and at least one --allow-origin are required")
		return exitUsageOrIO
	}
	checkedAt, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "live-check chromium: invalid --at:", err)
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "live-check chromium:", err)
		return classifyProfileError(err)
	}
	if newAcquirer == nil {
		fmt.Fprintln(stderr, "live-check chromium: capture dependency is unavailable")
		return exitUsageOrIO
	}
	result, err := capture.Check(context.Background(), newAcquirer(*driverDirectory), capture.LiveCheckRequest{
		Profile: prof, Actions: []string(actions),
		Capture: capture.LiveRequest{
			URL: *targetURL, AllowedOrigins: []string(allowedOrigins), ObservedAt: checkedAt,
			NavigationTimeout: *navigationTimeout, TotalTimeout: *totalTimeout,
			MaxRequests: *maxRequests, MaxResponseBytes: *maxResponseBytes,
			MaxEvidenceBytes: *maxEvidenceBytes, ARIADepth: *ariaDepth,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "live-check chromium:", err)
		return exitRejected
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "live-check chromium:", err)
		return exitRejected
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, "live-check chromium:", err)
		return exitUsageOrIO
	}
	if !result.OK {
		return exitRejected
	}
	return exitOK
}

func runCaptureChromium(args []string, stdout, stderr io.Writer) int {
	return runCaptureChromiumWith(args, stdout, stderr, time.Now, capture.NewPlaywrightAcquirer)
}

func runCaptureChromiumWith(
	args []string,
	stdout, stderr io.Writer,
	clock func() time.Time,
	newAcquirer func(string) capture.Acquirer,
) int {
	fs := flag.NewFlagSet("capture chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	targetURL := fs.String("url", "", "exact HTTPS or loopback HTTP URL")
	cacheRoot := fs.String("cache-root", "", "explicit private cache root")
	actionHint := fs.String("action-hint", "", "optional portable action identifier")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	navigationTimeout := fs.Duration("navigation-timeout", capture.DefaultNavigationTimeout, "per-navigation/observation timeout")
	totalTimeout := fs.Duration("timeout", capture.DefaultTotalTimeout, "total capture deadline")
	maxRequests := fs.Int("max-requests", capture.DefaultMaxRequests, "maximum routed requests")
	maxResponseBytes := fs.Int64("max-response-bytes", capture.DefaultMaxResponseBytes, "maximum total response bytes")
	maxEvidenceBytes := fs.Int64("max-evidence-bytes", capture.DefaultMaxEvidenceBytes, "maximum private capture bytes")
	ariaDepth := fs.Int("aria-depth", capture.DefaultARIADepth, "maximum ARIA snapshot depth")
	retainFor := fs.Duration("retain-for", capture.DefaultPrivateRetention, "private raw retention duration")
	var allowedOrigins stringList
	fs.Var(&allowedOrigins, "allow-origin", "exact allowed origin; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "capture chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *targetURL == "" || *cacheRoot == "" || len(allowedOrigins) == 0 {
		fmt.Fprintln(stderr, "capture chromium: --url, --cache-root, and at least one --allow-origin are required")
		return exitUsageOrIO
	}
	if *retainFor <= 0 || *retainFor > capture.MaxPrivateRetention {
		fmt.Fprintf(stderr, "capture chromium: --retain-for must be positive and no more than %s\n", capture.MaxPrivateRetention)
		return exitUsageOrIO
	}
	if clock == nil || newAcquirer == nil {
		fmt.Fprintln(stderr, "capture chromium: capture dependencies are unavailable")
		return exitUsageOrIO
	}
	store, err := cache.Open(*cacheRoot)
	if err != nil {
		fmt.Fprintln(stderr, "capture chromium:", err)
		return exitUsageOrIO
	}
	observedAt := clock().UTC()
	result, err := capture.Acquire(context.Background(), newAcquirer(*driverDirectory), capture.LiveRequest{
		URL: *targetURL, AllowedOrigins: []string(allowedOrigins), ActionHint: *actionHint,
		ObservedAt: observedAt, NavigationTimeout: *navigationTimeout,
		TotalTimeout: *totalTimeout, MaxRequests: *maxRequests,
		MaxResponseBytes: *maxResponseBytes, MaxEvidenceBytes: *maxEvidenceBytes,
		ARIADepth: *ariaDepth,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	entry, err := store.Put(context.Background(), bytes.NewReader(result.JSON), cache.PutOptions{
		Kind: cache.KindPrivateRaw, MediaType: "application/vnd.openudon.browsertools.playwright-capture+json",
		CreatedAt: observedAt, ExpiresAt: observedAt.Add(*retainFor), Source: "playwright-live",
		Annotations:         map[string]string{"fixture_version": playwrightadapter.FixtureVersion},
		PublicationEligible: false,
	})
	if err != nil {
		fmt.Fprintln(stderr, "capture chromium:", err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(entry); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runRichCaptureChromium(args []string, stdout, stderr io.Writer) int {
	return runRichCaptureChromiumWith(args, stdout, stderr, time.Now, capture.NewPlaywrightRichAcquirer)
}

func runRichCaptureChromiumWith(
	args []string,
	stdout, stderr io.Writer,
	clock func() time.Time,
	newAcquirer func(string) capture.RichAcquirer,
) int {
	fs := flag.NewFlagSet("rich-capture chromium", flag.ContinueOnError)
	fs.SetOutput(stderr)
	targetURL := fs.String("url", "", "exact HTTPS or loopback HTTP URL")
	cacheRoot := fs.String("cache-root", "", "explicit private cache root")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	navigationTimeout := fs.Duration("navigation-timeout", capture.DefaultNavigationTimeout, "per-navigation/observation timeout")
	totalTimeout := fs.Duration("timeout", capture.DefaultTotalTimeout, "total rich-capture deadline")
	maxRequests := fs.Int("max-requests", capture.DefaultMaxRequests, "maximum routed requests")
	maxResponseBytes := fs.Int64("max-response-bytes", capture.DefaultMaxResponseBytes, "maximum total response bytes")
	maxArtifactBytes := fs.Int64("max-artifact-bytes", capture.DefaultMaxRichArtifactBytes, "maximum bytes per private artifact")
	retainFor := fs.Duration("retain-for", capture.DefaultRichRetention, "private rich-artifact retention duration")
	var allowedOrigins stringList
	var artifactNames stringList
	fs.Var(&allowedOrigins, "allow-origin", "exact allowed origin; repeatable")
	fs.Var(&artifactNames, "artifact", "screenshot, trace, or har; repeatable and explicit")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "rich-capture chromium: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *targetURL == "" || *cacheRoot == "" || len(allowedOrigins) == 0 || len(artifactNames) == 0 {
		fmt.Fprintln(stderr, "rich-capture chromium: --url, --cache-root, at least one --allow-origin, and at least one --artifact are required")
		return exitUsageOrIO
	}
	if *retainFor <= 0 || *retainFor > capture.MaxRichRetention {
		fmt.Fprintf(stderr, "rich-capture chromium: --retain-for must be positive and no more than %s\n", capture.MaxRichRetention)
		return exitUsageOrIO
	}
	kinds := make([]capture.PrivateArtifactKind, 0, len(artifactNames))
	for _, name := range artifactNames {
		kind, err := capture.ParsePrivateArtifactKind(name)
		if err != nil {
			fmt.Fprintln(stderr, "rich-capture chromium:", err)
			return exitUsageOrIO
		}
		kinds = append(kinds, kind)
	}
	if clock == nil || newAcquirer == nil {
		fmt.Fprintln(stderr, "rich-capture chromium: capture dependencies are unavailable")
		return exitUsageOrIO
	}
	store, err := cache.Open(*cacheRoot)
	if err != nil {
		fmt.Fprintln(stderr, "rich-capture chromium:", err)
		return exitUsageOrIO
	}
	observedAt := clock().UTC()
	result, err := capture.AcquireRich(context.Background(), newAcquirer(*driverDirectory), capture.RichRequest{
		Capture: capture.LiveRequest{
			URL: *targetURL, AllowedOrigins: []string(allowedOrigins), ObservedAt: observedAt,
			NavigationTimeout: *navigationTimeout, TotalTimeout: *totalTimeout,
			MaxRequests: *maxRequests, MaxResponseBytes: *maxResponseBytes,
		},
		Artifacts: kinds, MaxArtifactBytes: *maxArtifactBytes,
	})
	if err != nil {
		fmt.Fprintln(stderr, "rich-capture chromium:", err)
		return exitRejected
	}
	bundle, manifest, err := capture.MarshalRichBundle(result, capture.EngineChromium, observedAt)
	if err != nil {
		fmt.Fprintln(stderr, "rich-capture chromium:", err)
		return exitRejected
	}
	storedArtifacts := make([]string, 0, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		storedArtifacts = append(storedArtifacts, string(artifact.Kind))
	}
	entry, err := store.Put(context.Background(), bytes.NewReader(bundle), cache.PutOptions{
		Kind: cache.KindPrivateRaw, MediaType: "application/vnd.openudon.browsertools.private-rich+zip",
		CreatedAt: observedAt, ExpiresAt: observedAt.Add(*retainFor), Source: "playwright-rich",
		Annotations: map[string]string{
			"artifacts": strings.Join(storedArtifacts, ","), "engine": string(capture.EngineChromium),
			"secret_review": "pending", "deletion": "exact_id_required",
		},
		PublicationEligible: false,
	})
	if err != nil {
		fmt.Fprintln(stderr, "rich-capture chromium:", err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(entry); err != nil {
		fmt.Fprintln(stderr, "rich-capture chromium:", err)
		return exitUsageOrIO
	}
	return exitOK
}

func runPortabilityCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runPortabilityCheckWith(args, stdin, stdout, stderr, time.Now,
		func(directory string, engine capture.Engine) capture.Acquirer {
			return capture.NewPlaywrightEngineAcquirer(directory, engine)
		})
}

func runPortabilityCheckWith(
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	clock func() time.Time,
	newAcquirer func(string, capture.Engine) capture.Acquirer,
) int {
	fs := flag.NewFlagSet("portability check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "validated browser profile JSON/YAML path or -")
	targetURL := fs.String("url", "", "explicit current-page HTTPS or loopback HTTP URL")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	navigationTimeout := fs.Duration("navigation-timeout", capture.DefaultNavigationTimeout, "per-navigation/observation timeout")
	totalTimeout := fs.Duration("timeout", capture.DefaultTotalTimeout, "per-engine live-check deadline")
	maxRequests := fs.Int("max-requests", capture.DefaultMaxRequests, "maximum routed requests per engine")
	maxResponseBytes := fs.Int64("max-response-bytes", capture.DefaultMaxResponseBytes, "maximum response bytes per engine")
	maxEvidenceBytes := fs.Int64("max-evidence-bytes", capture.DefaultMaxEvidenceBytes, "maximum transient observation bytes per engine")
	ariaDepth := fs.Int("aria-depth", capture.DefaultARIADepth, "maximum transient ARIA snapshot depth")
	out := fs.String("out", "-", "value-free portability report JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var allowedOrigins stringList
	var actions stringList
	var engineNames stringList
	fs.Var(&allowedOrigins, "allow-origin", "exact allowed origin; repeatable")
	fs.Var(&actions, "action", "profile action to check; repeatable, defaults to all")
	fs.Var(&engineNames, "engine", "chromium, firefox, or webkit; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "portability check: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *profilePath == "" || *targetURL == "" || len(allowedOrigins) == 0 || len(engineNames) < 2 {
		fmt.Fprintln(stderr, "portability check: --profile, --url, at least one --allow-origin, Chromium, and at least one alternate --engine are required")
		return exitUsageOrIO
	}
	engines := make([]capture.Engine, 0, len(engineNames))
	for _, name := range engineNames {
		engine, err := capture.ParseEngine(name)
		if err != nil {
			fmt.Fprintln(stderr, "portability check:", err)
			return exitUsageOrIO
		}
		engines = append(engines, engine)
	}
	if clock == nil || newAcquirer == nil {
		fmt.Fprintln(stderr, "portability check: capture dependencies are unavailable")
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "portability check:", err)
		return classifyProfileError(err)
	}
	checkedAt := clock().UTC()
	report, err := capture.ComparePortability(context.Background(), func(engine capture.Engine) capture.Acquirer {
		return newAcquirer(*driverDirectory, engine)
	}, engines, capture.LiveCheckRequest{
		Profile: prof, Actions: []string(actions),
		Capture: capture.LiveRequest{
			URL: *targetURL, AllowedOrigins: []string(allowedOrigins), ObservedAt: checkedAt,
			NavigationTimeout: *navigationTimeout, TotalTimeout: *totalTimeout,
			MaxRequests: *maxRequests, MaxResponseBytes: *maxResponseBytes,
			MaxEvidenceBytes: *maxEvidenceBytes, ARIADepth: *ariaDepth,
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, "portability check:", err)
		return exitRejected
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "portability check:", err)
		return exitRejected
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, "portability check:", err)
		return exitUsageOrIO
	}
	if !report.OK {
		return exitRejected
	}
	return exitOK
}

func runPlaywrightDoctor(args []string, stdout, stderr io.Writer) int {
	return runPlaywrightDoctorWith(args, stdout, stderr, capture.NewPlaywrightRuntime)
}

func runPlaywrightDoctorWith(args []string, stdout, stderr io.Writer, newRuntime func(string) capture.Runtime) int {
	fs := flag.NewFlagSet("playwright doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	engineName := fs.String("engine", string(capture.EngineChromium), "chromium, firefox, or webkit")
	driverDirectory := fs.String("driver-dir", "", "optional installed Playwright-Go driver directory")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "playwright doctor: unexpected positional arguments")
		return exitUsageOrIO
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(stderr, "playwright doctor: --format must be text or json")
		return exitUsageOrIO
	}
	engine, err := capture.ParseEngine(*engineName)
	if err != nil {
		fmt.Fprintln(stderr, "playwright doctor:", err)
		return exitUsageOrIO
	}
	if newRuntime == nil {
		fmt.Fprintln(stderr, "playwright doctor: runtime dependency is unavailable")
		return exitUsageOrIO
	}
	report, doctorErr := capture.Doctor(context.Background(), newRuntime(*driverDirectory), engine)
	if *format == "json" {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	} else {
		if doctorErr == nil {
			fmt.Fprintf(stdout, "%s ready (playwright-go %s, Playwright %s)\n", report.Engine, report.PlaywrightGoVersion, report.PlaywrightVersion)
			fmt.Fprintf(stdout, "browser executable: %s\n", report.BrowserExecutable)
		}
	}
	if doctorErr != nil {
		fmt.Fprintln(stderr, "playwright doctor:", doctorErr)
		return exitRejected
	}
	return exitOK
}

func runPlaywrightCapabilities(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("playwright capabilities", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "json", "json or text")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "playwright capabilities: unexpected positional arguments")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "text") {
		fmt.Fprintln(stderr, "playwright capabilities: --format must be json or text")
		return exitUsageOrIO
	}
	capabilities := capture.CapabilityMatrix()
	pressure := capture.ContractPressure()
	switch *format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(map[string]any{
			"version": capture.ContractPressureVersion, "capabilities": capabilities, "contractPressure": pressure,
		}); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, item := range pressure {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", item.Capability, item.Disposition, item.Browser15, item.NextStep)
		}
	}
	return exitOK
}

func runRegistryPublish(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry publish", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit local static registry root")
	bundlePath := fs.String("bundle", "", "capability bundle JSON path or -")
	at := fs.String("at", "", "RFC3339 publication time")
	supersedes := fs.String("supersedes", "", "optional existing ID@RELEASE coordinate")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *bundlePath == "" || *at == "" {
		fmt.Fprintln(stderr, "registry publish: --root, --bundle, and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry publish: invalid --at:", err)
		return exitUsageOrIO
	}
	var prior *registry.Coordinate
	if *supersedes != "" {
		coordinate, parseErr := parseCoordinate(*supersedes)
		if parseErr != nil {
			fmt.Fprintln(stderr, "registry publish:", parseErr)
			return exitUsageOrIO
		}
		prior = &coordinate
	}
	data, err := readInput(*bundlePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	value, err := capabilitybundle.Parse(data)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	report, err := registry.PublishLocal(context.Background(), registry.PublishOptions{
		Root: *root, Bundle: value, At: when, Supersedes: prior,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runRegistrySearch(args []string, stdout, stderr io.Writer) int {
	return runRegistrySearchWith(args, stdout, stderr, &registry.Client{})
}

func runRegistrySearchWith(args []string, stdout, stderr io.Writer, client *registry.Client) int {
	fs := flag.NewFlagSet("registry search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	query := fs.String("query", "", "id, title, origin, or action query")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	limit := fs.Int("limit", registry.DefaultMaxResults, "maximum results")
	includeInactive := fs.Bool("include-inactive", false, "include stale, revoked, and superseded entries")
	format := fs.String("format", "json", "json or text")
	client, policy := registryClientFlagsWith(fs, client)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *location == "" || *at == "" {
		fmt.Fprintln(stderr, "registry search: --location and --at are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "text") {
		fmt.Fprintln(stderr, "registry search: --format must be json or text")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	client.MaxResults = *limit
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry search: invalid --at:", err)
		return exitUsageOrIO
	}
	report, err := client.Search(context.Background(), registry.SearchOptions{
		Location: *location, Query: *query, Limit: *limit, At: when, IncludeInactive: *includeInactive,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, result := range report.Results {
			fmt.Fprintf(stdout, "%s@%s\t%s\t%d\t%s\t%s\n", result.Entry.ID, result.Entry.Release, result.Status, result.Score, result.Entry.Bundle.Digest.String(), result.Entry.Title)
		}
	}
	return exitOK
}

func runRegistryPull(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry pull", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	id := fs.String("id", "", "capability id")
	release := fs.String("release", "", "capability release")
	digestValue := fs.String("digest", "", "complete bundle sha256 digest")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	allowInactive := fs.Bool("allow-inactive", false, "allow historical stale, revoked, or superseded content")
	out := fs.String("out", "-", "bundle output path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	client, policy := registryClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	coordinateSelected := *id != "" || *release != ""
	if *location == "" || *at == "" || (coordinateSelected && (*id == "" || *release == "")) || (coordinateSelected == (*digestValue != "")) {
		fmt.Fprintln(stderr, "registry pull: --location, --at, and exactly one of (--id with --release) or --digest are required")
		return exitUsageOrIO
	}
	if *digestValue != "" && !validSHA256ID(*digestValue) {
		fmt.Fprintln(stderr, "registry pull: --digest must be sha256:<64 lowercase hex>")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry pull: invalid --at:", err)
		return exitUsageOrIO
	}
	options := registry.PullOptions{Location: *location, Digest: *digestValue, At: when, AllowInactive: *allowInactive}
	if coordinateSelected {
		options.Coordinate = &registry.Coordinate{ID: *id, Release: *release}
	}
	result, err := client.Pull(context.Background(), options)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data := append(append([]byte(nil), result.Content...), '\n')
	if len(result.Content) > 0 && result.Content[len(result.Content)-1] == '\n' {
		data = append([]byte(nil), result.Content...)
	}
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runRegistryVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	location := fs.String("location", "", "local registry root or static HTTPS base URL")
	at := fs.String("at", "", "RFC3339 lifecycle evaluation time")
	format := fs.String("format", "json", "json or text")
	client, policy := registryClientFlags(fs)
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *location == "" || *at == "" {
		fmt.Fprintln(stderr, "registry verify: --location and --at are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "text") {
		fmt.Fprintln(stderr, "registry verify: --format must be json or text")
		return exitUsageOrIO
	}
	if err := setRegistryNetworkPolicy(client, *policy); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "registry verify: invalid --at:", err)
		return exitUsageOrIO
	}
	report, err := client.Verify(context.Background(), *location, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, entry := range report.Entries {
			fmt.Fprintf(stdout, "%s@%s\t%s\t%s\t%s\n", entry.Coordinate.ID, entry.Coordinate.Release, entry.Status, entry.Digest, entry.BlobPath)
		}
	}
	return exitOK
}

func runCapabilityBundleBuild(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "stable lowercase capability id")
	release := fs.String("release", "", "semantic release version")
	profilePath := fs.String("profile", "", "reviewed browser profile JSON/YAML path")
	reviewPath := fs.String("review", "", "promotable review bundle JSON path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path")
	source := fs.String("source", "", "publication provenance source")
	license := fs.String("license", "", "SPDX-style license identifier")
	publishedAt := fs.String("published-at", "", "RFC3339 publication assessment time")
	out := fs.String("out", "-", "capability bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var authors stringList
	var companions stringList
	fs.Var(&authors, "author", "publication author; repeatable")
	fs.Var(&companions, "uws", "TARGET=PATH inert UWS companion; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *id == "" || *release == "" || *profilePath == "" || *reviewPath == "" || *evidencePath == "" || *source == "" || *license == "" || *publishedAt == "" {
		fmt.Fprintln(stderr, "bundle build: --id, --release, --profile, --review, --evidence, --source, --license, and --published-at are required")
		return exitUsageOrIO
	}
	mappings, err := parsePathMappings(companions)
	if err != nil {
		fmt.Fprintln(stderr, "bundle build:", err)
		return exitUsageOrIO
	}
	stdinPaths := []string{*profilePath, *reviewPath, *evidencePath}
	for _, mapping := range mappings {
		stdinPaths = append(stdinPaths, mapping.source)
	}
	if stdinCount(stdinPaths...) > 1 {
		fmt.Fprintln(stderr, "bundle build: only one input may use stdin")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *publishedAt)
	if err != nil {
		fmt.Fprintln(stderr, "bundle build: invalid --published-at:", err)
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	reviewData, err := readInput(*reviewPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	var reviewed review.Bundle
	if err := decodeStrictJSON(reviewData, &reviewed); err != nil {
		fmt.Fprintln(stderr, "bundle build: decode review:", err)
		return exitUsageOrIO
	}
	records, err := readEvidenceStrict(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	uwsCompanions := make([]capabilitybundle.Companion, 0, len(mappings))
	for _, mapping := range mappings {
		data, readErr := readInput(mapping.source, stdin)
		if readErr != nil {
			fmt.Fprintln(stderr, readErr)
			return exitUsageOrIO
		}
		mediaType, mediaErr := companionMediaType(mapping.target)
		if mediaErr != nil {
			fmt.Fprintln(stderr, mediaErr)
			return exitUsageOrIO
		}
		uwsCompanions = append(uwsCompanions, capabilitybundle.Companion{
			Path: mapping.target, MediaType: mediaType, Content: data,
		})
	}
	value, err := capabilitybundle.Build(capabilitybundle.BuildOptions{
		ID: *id, Release: *release, Source: *source, License: *license, Authors: authors,
		Profile: prof, Review: &reviewed, Evidence: records, Companions: uwsCompanions, PublishedAt: when,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data, err := capabilitybundle.CanonicalJSON(value, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCapabilityBundleVerify(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bundle verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "capability bundle JSON path or -")
	at := fs.String("at", "", "RFC3339 verification time")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *input == "" || *at == "" {
		fmt.Fprintln(stderr, "bundle verify: --input and --at are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "text", "json") {
		fmt.Fprintln(stderr, "bundle verify: --format must be text or json")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "bundle verify: invalid --at:", err)
		return exitUsageOrIO
	}
	data, err := readInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	value, err := capabilitybundle.Parse(data)
	if err == nil {
		err = capabilitybundle.Verify(value, when)
	}
	if err != nil {
		if *format == "json" {
			if encodeErr := json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "errors": []string{err.Error()}}); encodeErr != nil {
				fmt.Fprintln(stderr, "bundle verify:", encodeErr)
				return exitUsageOrIO
			}
		} else if *format == "text" {
			fmt.Fprintln(stderr, err)
		}
		return exitRejected
	}
	record, err := capabilitybundle.Digest(value, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	switch *format {
	case "text":
		fmt.Fprintf(stdout, "valid\t%s\t%s\t%s\n", value.Payload.Identity.ID, value.Payload.Identity.Release, record.String())
	case "json":
		if err := json.NewEncoder(stdout).Encode(map[string]any{
			"valid": true, "id": value.Payload.Identity.ID, "release": value.Payload.Identity.Release, "digest": record.String(),
		}); err != nil {
			fmt.Fprintln(stderr, "bundle verify:", err)
			return exitUsageOrIO
		}
	}
	return exitOK
}

func runCachePut(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache put", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	input := fs.String("input", "", "input path or -")
	kind := fs.String("kind", "", "private_raw, normalized_evidence, profile, or review_bundle")
	mediaType := fs.String("media-type", "", "artifact media type")
	createdAt := fs.String("created-at", "", "RFC3339 creation time")
	expiresAt := fs.String("expires-at", "", "optional RFC3339 expiry time")
	source := fs.String("source", "", "optional source tool or provenance label")
	publicationEligible := fs.Bool("publication-eligible", false, "mark a non-raw artifact eligible for independent publication verification")
	var annotations stringList
	fs.Var(&annotations, "annotation", "key=value annotation; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *input == "" || *kind == "" || *mediaType == "" || *createdAt == "" {
		fmt.Fprintln(stderr, "cache put: --root, --input, --kind, --media-type, and --created-at are required")
		return exitUsageOrIO
	}
	if !validCacheKind(cache.Kind(*kind)) {
		fmt.Fprintln(stderr, "cache put: --kind must be private_raw, normalized_evidence, profile, or review_bundle")
		return exitUsageOrIO
	}
	created, err := time.Parse(time.RFC3339Nano, *createdAt)
	if err != nil {
		fmt.Fprintln(stderr, "cache put: invalid --created-at:", err)
		return exitUsageOrIO
	}
	var expires time.Time
	if *expiresAt != "" {
		expires, err = time.Parse(time.RFC3339Nano, *expiresAt)
		if err != nil {
			fmt.Fprintln(stderr, "cache put: invalid --expires-at:", err)
			return exitUsageOrIO
		}
	}
	annotationMap, err := parseKeyValues(annotations)
	if err != nil {
		fmt.Fprintln(stderr, "cache put:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	reader, closeInput, err := openInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	defer closeInput()
	entry, err := store.Put(context.Background(), reader, cache.PutOptions{
		Kind: cache.Kind(*kind), MediaType: *mediaType, CreatedAt: created, ExpiresAt: expires,
		Source: *source, Annotations: annotationMap, PublicationEligible: *publicationEligible,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(entry); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCacheGet(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = stdin
	fs := flag.NewFlagSet("cache get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	id := fs.String("id", "", "sha256 cache id")
	at := fs.String("at", "", "RFC3339 expiry evaluation time")
	out := fs.String("out", "-", "payload output path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *id == "" || *at == "" {
		fmt.Fprintln(stderr, "cache get: --root, --id, and --at are required")
		return exitUsageOrIO
	}
	if !validSHA256ID(*id) {
		fmt.Fprintln(stderr, "cache get: --id must be sha256:<64 lowercase hex>")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache get: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	entry, payload, err := store.Get(context.Background(), *id, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	if entry.Kind == cache.KindPrivateRaw && *out == "-" {
		fmt.Fprintln(stderr, "cache get: private_raw payloads require an explicit --out path")
		return exitUsageOrIO
	}
	if entry.Kind == cache.KindPrivateRaw && *force {
		fmt.Fprintln(stderr, "cache get: private_raw payloads cannot overwrite an existing file")
		return exitUsageOrIO
	}
	mode := os.FileMode(0o644)
	if entry.Kind == cache.KindPrivateRaw {
		mode = 0o600
	}
	if err := writeOutputMode(*out, payload, *force, stdout, mode); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runCacheList(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	at := fs.String("at", "", "RFC3339 expiry evaluation time")
	includeExpired := fs.Bool("include-expired", false, "include expired entries")
	format := fs.String("format", "json", "json or text")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *at == "" {
		fmt.Fprintln(stderr, "cache list: --root and --at are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "text") {
		fmt.Fprintln(stderr, "cache list: --format must be json or text")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache list: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	entries, err := store.List(context.Background(), when, *includeExpired)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	return renderCacheEntries(entries, *format, stdout, stderr)
}

func runCachePrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	at := fs.String("at", "", "RFC3339 prune time")
	format := fs.String("format", "json", "json or text")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *root == "" || *at == "" {
		fmt.Fprintln(stderr, "cache prune: --root and --at are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "text") {
		fmt.Fprintln(stderr, "cache prune: --format must be json or text")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339Nano, *at)
	if err != nil {
		fmt.Fprintln(stderr, "cache prune: invalid --at:", err)
		return exitUsageOrIO
	}
	store, err := cache.Open(*root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	entries, err := store.Prune(context.Background(), when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	return renderCacheEntries(entries, *format, stdout, stderr)
}

func runCacheDelete(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cache delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "explicit private cache root")
	id := fs.String("id", "", "exact private_raw sha256 cache id")
	confirmID := fs.String("confirm-id", "", "repeat the exact cache id to authorize deletion")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if fs.NArg() != 0 || *root == "" || *id == "" || *confirmID == "" {
		fmt.Fprintln(stderr, "cache delete: --root, --id, and --confirm-id are required with no positional arguments")
		return exitUsageOrIO
	}
	if *id != *confirmID {
		fmt.Fprintln(stderr, "cache delete: --confirm-id must exactly match --id")
		return exitUsageOrIO
	}
	if !validSHA256ID(*id) {
		fmt.Fprintln(stderr, "cache delete: --id must be sha256:<64 lowercase hex>")
		return exitUsageOrIO
	}
	store, err := cache.OpenExisting(*root)
	if err != nil {
		fmt.Fprintln(stderr, "cache delete:", err)
		return exitUsageOrIO
	}
	entry, err := store.DeletePrivate(context.Background(), *id)
	if err != nil {
		fmt.Fprintln(stderr, "cache delete:", err)
		return exitRejected
	}
	if err := json.NewEncoder(stdout).Encode(entry); err != nil {
		fmt.Fprintln(stderr, "cache delete:", err)
		return exitUsageOrIO
	}
	return exitOK
}

func renderCacheEntries(entries []cache.Entry, format string, stdout, stderr io.Writer) int {
	switch format {
	case "json":
		if err := json.NewEncoder(stdout).Encode(entries); err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsageOrIO
		}
	case "text":
		for _, entry := range entries {
			fmt.Fprintf(stdout, "%s\t%s\t%d\t%s\t%t\n", entry.ID, entry.Kind, entry.SizeBytes, entry.ExpiresAt, entry.PublicationEligible)
		}
	default:
		fmt.Fprintln(stderr, "cache output format must be json or text")
		return exitUsageOrIO
	}
	return exitOK
}

func runProfileValidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("profile validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "profile JSON/YAML path")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *input == "" {
		fmt.Fprintln(stderr, "profile validate: --input is required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "text", "json") {
		fmt.Fprintln(stderr, "profile validate: --format must be text or json")
		return exitUsageOrIO
	}
	_, err := loadProfileInput(*input, stdin)
	if err == nil {
		if *format == "json" {
			fmt.Fprintln(stdout, `{"valid":true,"errors":[]}`)
		} else if *format == "text" {
			fmt.Fprintln(stdout, "valid")
		}
		return exitOK
	}
	var validationErr *profile.ValidationError
	if errors.As(err, &validationErr) {
		if *format == "json" {
			if encodeErr := json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "errors": []string{err.Error()}}); encodeErr != nil {
				fmt.Fprintln(stderr, "profile validate:", encodeErr)
				return exitUsageOrIO
			}
		} else {
			fmt.Fprintln(stderr, err)
		}
		return exitRejected
	}
	fmt.Fprintln(stderr, err)
	return exitUsageOrIO
}

func runAuthProfileValidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("auth-profile validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "authentication profile JSON/YAML path or -")
	at := fs.String("at", "", "optional RFC3339 freshness assessment time")
	format := fs.String("format", "text", "text or json")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *input == "" {
		fmt.Fprintln(stderr, "auth-profile validate: --input is required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "text", "json") {
		fmt.Fprintln(stderr, "auth-profile validate: --format must be text or json")
		return exitUsageOrIO
	}
	var when time.Time
	if *at != "" {
		var parseErr error
		when, parseErr = time.Parse(time.RFC3339, *at)
		if parseErr != nil {
			fmt.Fprintln(stderr, "auth-profile validate: invalid --at:", parseErr)
			return exitUsageOrIO
		}
	}
	value, err := loadAuthProfileInput(*input, stdin)
	if err == nil && *at != "" {
		err = authprofile.ValidateAt(value, when)
	}
	if err != nil {
		if *format == "json" {
			if encodeErr := json.NewEncoder(stdout).Encode(map[string]any{"valid": false, "errors": []string{err.Error()}}); encodeErr != nil {
				fmt.Fprintln(stderr, "auth-profile validate:", encodeErr)
				return exitUsageOrIO
			}
		} else if *format == "text" {
			fmt.Fprintln(stderr, err)
		}
		return exitRejected
	}
	if *format == "json" {
		fmt.Fprintln(stdout, `{"valid":true,"errors":[]}`)
	} else if *format == "text" {
		fmt.Fprintln(stdout, "valid")
	}
	return exitOK
}

func runAuthDraftBuild(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("auth-draft build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	specPath := fs.String("spec", "", "explicit authentication draft spec JSON/YAML path or -")
	out := fs.String("out", "-", "authentication profile output path or -")
	format := fs.String("format", "yaml", "yaml or json")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *specPath == "" {
		fmt.Fprintln(stderr, "auth-draft build: --spec is required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "yaml", "json") {
		fmt.Fprintln(stderr, "auth-draft build: --format must be yaml or json")
		return exitUsageOrIO
	}
	data, err := readInput(*specPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	var spec authdraft.Spec
	if err := decodeJSONOrYAML(data, filepath.Ext(*specPath), &spec); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	value, err := authdraft.Build(spec)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	var encoded []byte
	switch *format {
	case "yaml":
		encoded, err = authprofile.MarshalYAML(value)
	case "json":
		encoded, err = json.MarshalIndent(value, "", "  ")
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	encoded = append(encoded, '\n')
	if err := writeOutput(*out, encoded, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runAuthReviewBundle(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("auth-review bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "authentication profile JSON/YAML path or -")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "review bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *profilePath == "" || *at == "" {
		fmt.Fprintln(stderr, "auth-review bundle: --profile and --at are required")
		return exitUsageOrIO
	}
	when, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "auth-review bundle: invalid --at:", err)
		return exitUsageOrIO
	}
	value, err := loadAuthProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	bundle, err := authreview.Build(value, when)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	encoded, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	encoded = append(encoded, '\n')
	if err := writeOutput(*out, encoded, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if !bundle.Promotable {
		return exitRejected
	}
	return exitOK
}

func runEvidenceImport(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evidence import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tool := fs.String("adapter", "", "playwright, llm-scraper, crawl4ai, or firecrawl")
	input := fs.String("input", "", "raw fixture path or -")
	origin := fs.String("origin", "", "reviewed fixture origin")
	actionHint := fs.String("action-hint", "", "profile action name")
	redaction := fs.String("redaction-status", "", "not_required or redacted")
	out := fs.String("out", "-", "normalized evidence path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	var redactedFields stringList
	fs.Var(&redactedFields, "redacted-field", "redacted field path; repeatable")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *tool == "" || *input == "" || *origin == "" || *redaction == "" {
		fmt.Fprintln(stderr, "evidence import: --adapter, --input, --origin, and --redaction-status are required")
		return exitUsageOrIO
	}
	importer, err := importerFor(*tool)
	if err != nil {
		fmt.Fprintln(stderr, "evidence import:", err)
		return exitUsageOrIO
	}
	status := evidence.RedactionStatus(*redaction)
	if status != evidence.RedactionNotRequired && status != evidence.RedactionCompleted {
		fmt.Fprintln(stderr, "evidence import: --redaction-status must be not_required or redacted")
		return exitUsageOrIO
	}
	if _, err := profile.ParseOrigin(*origin); err != nil {
		fmt.Fprintln(stderr, "evidence import: invalid --origin:", err)
		return exitUsageOrIO
	}
	if status == evidence.RedactionCompleted && len(redactedFields) == 0 {
		fmt.Fprintln(stderr, "evidence import: redacted status requires at least one --redacted-field")
		return exitUsageOrIO
	}
	raw, err := readInput(*input, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	records, err := importer.Import(raw, adapter.Options{
		Origin: *origin, ActionHint: *actionHint, RedactionStatus: status, RedactedFields: redactedFields,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitRejected
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runDraftBuild(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("draft build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	specPath := fs.String("spec", "", "draft spec JSON/YAML path")
	out := fs.String("out", "-", "profile path or -")
	format := fs.String("format", "json", "stdout format: json or yaml")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *evidencePath == "" || *specPath == "" {
		fmt.Fprintln(stderr, "draft build: --evidence and --spec are required")
		return exitUsageOrIO
	}
	if !validFormat(*format, "json", "yaml") {
		fmt.Fprintln(stderr, "draft build: --format must be json or yaml")
		return exitUsageOrIO
	}
	if *evidencePath == "-" && *specPath == "-" {
		fmt.Fprintln(stderr, "draft build: only one input may use stdin")
		return exitUsageOrIO
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	specData, err := readInput(*specPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	var spec draft.Spec
	if err := decodeJSONOrYAML(specData, filepath.Ext(*specPath), &spec); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	result, err := draft.Build(records, spec)
	if err != nil {
		fmt.Fprintln(stderr, err)
		if result != nil && len(result.Diagnostics) > 0 {
			if encodeErr := json.NewEncoder(stderr).Encode(result.Diagnostics); encodeErr != nil {
				return exitUsageOrIO
			}
		}
		return exitRejected
	}
	data, err := marshalProfile(result.Profile, *out, *format)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	return exitOK
}

func runReviewBundle(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "profile path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	decisionPath := fs.String("decisions", "", "optional decision JSON/YAML path")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "bundle JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *profilePath == "" || *evidencePath == "" || *at == "" {
		fmt.Fprintln(stderr, "review bundle: --profile, --evidence, and --at are required")
		return exitUsageOrIO
	}
	if stdinCount(*profilePath, *evidencePath, *decisionPath) > 1 {
		fmt.Fprintln(stderr, "review bundle: only one input may use stdin")
		return exitUsageOrIO
	}
	now, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "review bundle: invalid --at:", err)
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	decisions, err := readDecisions(*decisionPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	bundle, err := review.Build(prof, records, decisions, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "review bundle:", err)
		return exitUsageOrIO
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if !bundle.Promotable() {
		return exitRejected
	}
	return exitOK
}

func runRevalidate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("revalidate check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profilePath := fs.String("profile", "", "profile path")
	evidencePath := fs.String("evidence", "", "normalized evidence JSON path or -")
	decisionPath := fs.String("decisions", "", "optional decision JSON/YAML path")
	at := fs.String("at", "", "RFC3339 assessment time")
	out := fs.String("out", "-", "report JSON path or -")
	force := fs.Bool("force", false, "overwrite an existing output")
	if err := fs.Parse(args); err != nil {
		return exitUsageOrIO
	}
	if *profilePath == "" || *evidencePath == "" || *at == "" {
		fmt.Fprintln(stderr, "revalidate check: --profile, --evidence, and --at are required")
		return exitUsageOrIO
	}
	if stdinCount(*profilePath, *evidencePath, *decisionPath) > 1 {
		fmt.Fprintln(stderr, "revalidate check: only one input may use stdin")
		return exitUsageOrIO
	}
	now, err := time.Parse(time.RFC3339, *at)
	if err != nil {
		fmt.Fprintln(stderr, "revalidate check: invalid --at:", err)
		return exitUsageOrIO
	}
	prof, err := loadProfileInput(*profilePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return classifyProfileError(err)
	}
	records, err := readEvidence(*evidencePath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	decisions, err := readDecisions(*decisionPath, stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	result, err := revalidate.CheckAt(prof, records, decisions, now)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "revalidate check:", err)
		return exitUsageOrIO
	}
	data = append(data, '\n')
	if err := writeOutput(*out, data, *force, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsageOrIO
	}
	if !result.OK {
		return exitRejected
	}
	return exitOK
}

func importerFor(name string) (adapter.Adapter, error) {
	switch name {
	case "playwright":
		return &playwrightadapter.Adapter{}, nil
	case "llm-scraper":
		return &llmscraper.Adapter{}, nil
	case "crawl4ai":
		return &crawl4ai.Adapter{}, nil
	case "firecrawl":
		return &firecrawl.Adapter{}, nil
	default:
		return nil, fmt.Errorf("unknown adapter %q", name)
	}
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func openInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		return stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func parseKeyValues(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("annotation %q must use non-empty key=value", value)
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("annotation key %q is duplicated", key)
		}
		result[key] = strings.TrimSpace(item)
	}
	return result, nil
}

func readEvidence(path string, stdin io.Reader) ([]evidence.Record, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var records []evidence.Record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	return records, nil
}

func readEvidenceStrict(path string, stdin io.Reader) ([]evidence.Record, error) {
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var records []evidence.Record
	if err := decodeStrictJSON(data, &records); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	return records, nil
}

func readEvidenceStrictBounded(path string, stdin io.Reader, maximum int64) ([]evidence.Record, error) {
	if maximum < 1 {
		return nil, fmt.Errorf("decode evidence: invalid input bound")
	}
	reader, closeInput, err := openInput(path, stdin)
	if err != nil {
		return nil, err
	}
	defer closeInput()
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("decode evidence: input exceeds %d bytes", maximum)
	}
	var records []evidence.Record
	if err := decodeStrictJSON(data, &records); err != nil {
		return nil, fmt.Errorf("decode evidence: %w", err)
	}
	return records, nil
}

func readDecisions(path string, stdin io.Reader) ([]evidence.LocatorDecision, error) {
	if path == "" {
		return []evidence.LocatorDecision{}, nil
	}
	data, err := readInput(path, stdin)
	if err != nil {
		return nil, err
	}
	var decisions []evidence.LocatorDecision
	if err := decodeJSONOrYAML(data, filepath.Ext(path), &decisions); err != nil {
		return nil, fmt.Errorf("decode decisions: %w", err)
	}
	return decisions, nil
}

func loadProfileInput(path string, stdin io.Reader) (*profile.Profile, error) {
	if path != "-" {
		return profile.LoadFile(path)
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return profile.ParseJSON(data)
	}
	return profile.ParseYAML(data)
}

func loadAuthProfileInput(path string, stdin io.Reader) (*authprofile.Profile, error) {
	if path != "-" {
		return authprofile.LoadFile(path)
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	return authprofile.Parse(data)
}

func stdinCount(paths ...string) int {
	count := 0
	for _, path := range paths {
		if path == "-" {
			count++
		}
	}
	return count
}

func decodeJSONOrYAML(data []byte, extension string, target any) error {
	extension = strings.ToLower(extension)
	if extension == ".json" || (extension == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))) || (extension == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("["))) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return fmt.Errorf("multiple JSON values are not supported")
			}
			return err
		}
		return nil
	}
	if extension == ".yaml" || extension == ".yml" || extension == "" {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(target); err != nil {
			return err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return fmt.Errorf("multiple YAML documents are not supported")
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("unsupported JSON/YAML extension %q", extension)
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func registryClientFlags(fs *flag.FlagSet) (*registry.Client, *string) {
	return registryClientFlagsWith(fs, &registry.Client{})
}

func registryClientFlagsWith(fs *flag.FlagSet, client *registry.Client) (*registry.Client, *string) {
	if client == nil {
		client = &registry.Client{}
	}
	policy := fs.String("network", "never", "network policy: never, ask, or allow")
	fs.DurationVar(&client.Timeout, "timeout", registry.DefaultTimeout, "total registry read deadline (capped at 8s)")
	fs.Int64Var(&client.MaxBytes, "max-bytes", registry.DefaultMaxBytes, "per-file response bound (capped at 20 MiB)")
	fs.BoolVar(&client.AllowLoopbackHosts, "allow-loopback", false, "allow an exact loopback HTTPS registry target")
	return client, policy
}

func setRegistryNetworkPolicy(client *registry.Client, value string) error {
	switch registry.NetworkPolicy(strings.TrimSpace(value)) {
	case registry.NetworkNever, registry.NetworkAsk, registry.NetworkAllow:
		client.NetworkPolicy = registry.NetworkPolicy(strings.TrimSpace(value))
		return nil
	default:
		return fmt.Errorf("registry --network must be never, ask, or allow")
	}
}

func parseCoordinate(value string) (registry.Coordinate, error) {
	value = strings.TrimSpace(value)
	position := strings.LastIndex(value, "@")
	if position <= 0 || position == len(value)-1 {
		return registry.Coordinate{}, fmt.Errorf("coordinate %q must use ID@RELEASE", value)
	}
	return registry.Coordinate{ID: strings.TrimSpace(value[:position]), Release: strings.TrimSpace(value[position+1:])}, nil
}

type pathMapping struct {
	target string
	source string
}

func parsePathMappings(values []string) ([]pathMapping, error) {
	result := make([]pathMapping, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		target, source, ok := strings.Cut(value, "=")
		target, source = strings.TrimSpace(target), strings.TrimSpace(source)
		if !ok || target == "" || source == "" {
			return nil, fmt.Errorf("UWS companion %q must use TARGET=PATH", value)
		}
		if _, ok := seen[target]; ok {
			return nil, fmt.Errorf("UWS companion target %q is duplicated", target)
		}
		if _, err := companionMediaType(target); err != nil {
			return nil, err
		}
		seen[target] = struct{}{}
		result = append(result, pathMapping{target: target, source: source})
	}
	return result, nil
}

func companionMediaType(target string) (string, error) {
	lower := strings.ToLower(target)
	switch {
	case strings.HasSuffix(lower, ".uws.json"):
		return capabilitybundle.UWSJSONMediaType, nil
	case strings.HasSuffix(lower, ".uws.yaml"), strings.HasSuffix(lower, ".uws.yml"):
		return capabilitybundle.UWSYAMLMediaType, nil
	default:
		return "", fmt.Errorf("UWS companion target %q must end in .uws.json, .uws.yaml, or .uws.yml", target)
	}
}

func marshalProfile(prof *profile.Profile, outputPath, stdoutFormat string) ([]byte, error) {
	format := stdoutFormat
	if outputPath != "-" {
		switch strings.ToLower(filepath.Ext(outputPath)) {
		case ".json":
			format = "json"
		case ".yaml", ".yml":
			format = "yaml"
		default:
			return nil, fmt.Errorf("profile output must use .json, .yaml, or .yml")
		}
	}
	switch format {
	case "json":
		data, err := json.MarshalIndent(prof, "", "  ")
		return append(data, '\n'), err
	case "yaml":
		return profile.MarshalYAML(*prof)
	default:
		return nil, fmt.Errorf("draft build: --format must be json or yaml")
	}
}

func writeOutput(path string, data []byte, force bool, stdout io.Writer) error {
	return writeOutputMode(path, data, force, stdout, 0o644)
}

func writeOutputMode(path string, data []byte, force bool, stdout io.Writer, mode os.FileMode) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if force {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, mode)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("refusing to overwrite %s without --force", path)
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func classifyProfileError(err error) int {
	var validationErr *profile.ValidationError
	if errors.As(err, &validationErr) {
		return exitRejected
	}
	return exitUsageOrIO
}

type stringList []string

func (s *stringList) String() string         { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error { *s = append(*s, value); return nil }
