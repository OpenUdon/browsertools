package capture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	playwrightadapter "github.com/OpenUdon/browsertools/adapter/playwright"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/evidence/redact"
)

const (
	DefaultNavigationTimeout = 15 * time.Second
	DefaultTotalTimeout      = 30 * time.Second
	DefaultMaxRequests       = 128
	DefaultMaxResponseBytes  = int64(16 << 20)
	DefaultMaxEvidenceBytes  = int64(2 << 20)
	DefaultARIADepth         = 12
	MaxAllowedOrigins        = 16
	MaxRequests              = 1024
	MaxResponseBytes         = int64(64 << 20)
	MaxEvidenceBytes         = int64(8 << 20)
	MaxTotalTimeout          = 2 * time.Minute
	MaxNavigationTimeout     = time.Minute
	MaxStructuredDocuments   = 32
	DefaultPrivateRetention  = 24 * time.Hour
	MaxPrivateRetention      = 7 * 24 * time.Hour
)

var actionHintPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// LiveRequest defines one explicit, bounded authoring acquisition. It contains
// no credentials, headers, cookies, storage state, scripts, or actions.
type LiveRequest struct {
	URL               string
	AllowedOrigins    []string
	ActionHint        string
	ObservedAt        time.Time
	NavigationTimeout time.Duration
	TotalTimeout      time.Duration
	MaxRequests       int
	MaxResponseBytes  int64
	MaxEvidenceBytes  int64
	ARIADepth         int
	// Probes are populated only by Check. They are a closed, read-only set of
	// locator, wait, and output-shape observations; capture CLI callers cannot
	// provide them.
	Probes []Probe
}

// Observation is the minimal private result returned by an acquisition
// backend before Browsertools validates and serializes it.
type Observation struct {
	FinalURL       string
	ARIASnapshot   string
	StructuredData []json.RawMessage
	Network        playwrightadapter.NetworkSummary
	ProbeResults   []ProbeResult
}

// Acquirer performs the browser-specific part of a validated live request.
// Default tests use a fake; NewPlaywrightAcquirer is the production adapter.
type Acquirer interface {
	Acquire(context.Context, LiveRequest) (Observation, error)
}

// LiveResult is a validated private capture ready for KindPrivateRaw cache
// storage. JSON always ends with one newline.
type LiveResult struct {
	Origin       string
	Fixture      playwrightadapter.Fixture
	JSON         []byte
	ProbeResults []ProbeResult
}

// Acquire validates a request, applies its total deadline, invokes the
// browser-specific acquirer, and validates the result before serialization.
func Acquire(ctx context.Context, acquirer Acquirer, request LiveRequest) (LiveResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if acquirer == nil {
		return LiveResult{}, fmt.Errorf("capture acquirer is required")
	}
	normalized, origin, err := normalizeLiveRequest(request)
	if err != nil {
		return LiveResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, normalized.TotalTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return LiveResult{}, err
	}
	observation, err := acquirer.Acquire(ctx, normalized)
	if err != nil {
		return LiveResult{}, fmt.Errorf("capture chromium: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return LiveResult{}, err
	}
	if err := validateObservation(normalized, observation); err != nil {
		return LiveResult{}, err
	}
	if len(normalized.Probes) > 0 {
		// A live check needs only value-free facts. Do not serialize another
		// private raw fixture when no capture artifact was requested.
		return LiveResult{
			Origin: origin, ProbeResults: append([]ProbeResult(nil), observation.ProbeResults...),
		}, nil
	}
	fixture := playwrightadapter.Fixture{
		Version: playwrightadapter.FixtureVersion, URL: observation.FinalURL,
		ObservedAt: normalized.ObservedAt.UTC().Format(time.RFC3339Nano),
		ActionHint: normalized.ActionHint, PlaywrightVersion: PlaywrightVersion,
		ARIASnapshot:   observation.ARIASnapshot,
		StructuredData: cloneRawMessages(observation.StructuredData), Network: &observation.Network,
	}
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return LiveResult{}, fmt.Errorf("encode private capture: %w", err)
	}
	encoded = append(encoded, '\n')
	if int64(len(encoded)) > normalized.MaxEvidenceBytes {
		return LiveResult{}, fmt.Errorf("private capture exceeds max evidence bytes")
	}
	return LiveResult{
		Origin: origin, Fixture: fixture, JSON: encoded,
		ProbeResults: append([]ProbeResult(nil), observation.ProbeResults...),
	}, nil
}

func normalizeLiveRequest(request LiveRequest) (LiveRequest, string, error) {
	request.URL = strings.TrimSpace(request.URL)
	if request.URL == "" {
		return LiveRequest{}, "", fmt.Errorf("capture URL is required")
	}
	if len(request.URL) > 4096 {
		return LiveRequest{}, "", fmt.Errorf("capture URL exceeds 4096 bytes")
	}
	origin, err := validateCaptureURL(request.URL)
	if err != nil {
		return LiveRequest{}, "", fmt.Errorf("capture URL: %w", err)
	}
	if len(request.AllowedOrigins) == 0 || len(request.AllowedOrigins) > MaxAllowedOrigins {
		return LiveRequest{}, "", fmt.Errorf("capture requires 1 to %d allowed origins", MaxAllowedOrigins)
	}
	allowed := make([]string, 0, len(request.AllowedOrigins))
	seen := map[string]struct{}{}
	for _, raw := range request.AllowedOrigins {
		canonical, err := validateCaptureOrigin(raw)
		if err != nil {
			return LiveRequest{}, "", fmt.Errorf("allowed origin: %w", err)
		}
		if _, duplicate := seen[canonical]; duplicate {
			return LiveRequest{}, "", fmt.Errorf("allowed origin %q is duplicated", canonical)
		}
		seen[canonical] = struct{}{}
		allowed = append(allowed, canonical)
	}
	if _, ok := seen[origin]; !ok {
		return LiveRequest{}, "", fmt.Errorf("initial URL origin must be explicitly allowed")
	}
	slices.Sort(allowed)
	request.AllowedOrigins = allowed
	request.ActionHint = strings.TrimSpace(request.ActionHint)
	if request.ActionHint != "" && !actionHintPattern.MatchString(request.ActionHint) {
		return LiveRequest{}, "", fmt.Errorf("action hint must match %s", actionHintPattern)
	}
	if request.ObservedAt.IsZero() {
		return LiveRequest{}, "", fmt.Errorf("observed time is required")
	}
	request.ObservedAt = request.ObservedAt.UTC()
	if request.NavigationTimeout == 0 {
		request.NavigationTimeout = DefaultNavigationTimeout
	}
	if request.TotalTimeout == 0 {
		request.TotalTimeout = DefaultTotalTimeout
	}
	if request.NavigationTimeout <= 0 || request.NavigationTimeout > MaxNavigationTimeout {
		return LiveRequest{}, "", fmt.Errorf("navigation timeout must be positive and no more than %s", MaxNavigationTimeout)
	}
	if request.TotalTimeout <= 0 || request.TotalTimeout > MaxTotalTimeout {
		return LiveRequest{}, "", fmt.Errorf("total timeout must be positive and no more than %s", MaxTotalTimeout)
	}
	if request.NavigationTimeout > request.TotalTimeout {
		return LiveRequest{}, "", fmt.Errorf("navigation timeout cannot exceed total timeout")
	}
	if request.MaxRequests == 0 {
		request.MaxRequests = DefaultMaxRequests
	}
	if request.MaxRequests < 1 || request.MaxRequests > MaxRequests {
		return LiveRequest{}, "", fmt.Errorf("max requests must be between 1 and %d", MaxRequests)
	}
	if request.MaxResponseBytes == 0 {
		request.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if request.MaxResponseBytes < 1 || request.MaxResponseBytes > MaxResponseBytes {
		return LiveRequest{}, "", fmt.Errorf("max response bytes must be between 1 and %d", MaxResponseBytes)
	}
	if request.MaxEvidenceBytes == 0 {
		request.MaxEvidenceBytes = DefaultMaxEvidenceBytes
	}
	if request.MaxEvidenceBytes < 1 || request.MaxEvidenceBytes > MaxEvidenceBytes {
		return LiveRequest{}, "", fmt.Errorf("max evidence bytes must be between 1 and %d", MaxEvidenceBytes)
	}
	if request.ARIADepth == 0 {
		request.ARIADepth = DefaultARIADepth
	}
	if request.ARIADepth < 1 || request.ARIADepth > 32 {
		return LiveRequest{}, "", fmt.Errorf("ARIA depth must be between 1 and 32")
	}
	request.Probes, err = normalizeProbes(request.Probes)
	if err != nil {
		return LiveRequest{}, "", err
	}
	return request, origin, nil
}

func validateObservation(request LiveRequest, observation Observation) error {
	if strings.TrimSpace(observation.FinalURL) == "" {
		return fmt.Errorf("capture returned no final URL")
	}
	if _, err := validateCaptureURL(observation.FinalURL); err != nil {
		return fmt.Errorf("capture final URL: %w", err)
	}
	if !originAllowed(observation.FinalURL, request.AllowedOrigins) {
		return fmt.Errorf("capture final URL is outside allowed origins")
	}
	if strings.TrimSpace(observation.ARIASnapshot) == "" {
		return fmt.Errorf("capture returned an empty ARIA snapshot")
	}
	if int64(len(observation.ARIASnapshot)) > request.MaxEvidenceBytes {
		return fmt.Errorf("ARIA snapshot exceeds max evidence bytes")
	}
	if len(observation.StructuredData) > MaxStructuredDocuments {
		return fmt.Errorf("capture returned more than %d structured-data documents", MaxStructuredDocuments)
	}
	structuredBytes := int64(0)
	for index, document := range observation.StructuredData {
		structuredBytes += int64(len(document))
		if !json.Valid(document) {
			return fmt.Errorf("structured-data document[%d] is not valid JSON", index)
		}
	}
	if structuredBytes > request.MaxEvidenceBytes {
		return fmt.Errorf("structured data exceeds max evidence bytes")
	}
	if observation.Network.Requests < 0 || observation.Network.Responses < 0 || observation.Network.ResponseBytes < 0 ||
		observation.Network.BlockedRequests < 0 || observation.Network.BlockedWebSockets < 0 ||
		observation.Network.BlockedPopups < 0 || observation.Network.BlockedDownloads < 0 ||
		observation.Network.BlockedDialogs < 0 || observation.Network.BlockedFileChoosers < 0 {
		return fmt.Errorf("capture backend returned invalid negative network counts")
	}
	if observation.Network.Requests == 0 || observation.Network.Responses > observation.Network.Requests ||
		observation.Network.BlockedRequests > observation.Network.Requests {
		return fmt.Errorf("capture backend returned inconsistent network counts")
	}
	if observation.Network.Requests > request.MaxRequests || observation.Network.ResponseBytes > request.MaxResponseBytes {
		return fmt.Errorf("capture backend exceeded network bounds")
	}
	if err := validateProbeResults(request.Probes, observation.ProbeResults); err != nil {
		return err
	}
	return nil
}

func validateCaptureURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" {
		if err == nil {
			err = fmt.Errorf("URL must be absolute")
		}
		return "", err
	}
	if parsed.User != nil {
		return "", fmt.Errorf("userinfo credentials are not allowed")
	}
	if redact.String(raw) != raw {
		return "", fmt.Errorf("credential-shaped URL values are not allowed")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", fmt.Errorf("query is malformed")
	}
	for key := range query {
		if sensitiveURLKey(key) {
			return "", fmt.Errorf("credential-shaped query parameters are not allowed")
		}
	}
	if strings.Contains(parsed.Fragment, "=") {
		fragment, err := url.ParseQuery(strings.TrimPrefix(parsed.Fragment, "?"))
		if err != nil {
			return "", fmt.Errorf("fragment parameters are malformed")
		}
		for key := range fragment {
			if sensitiveURLKey(key) {
				return "", fmt.Errorf("credential-shaped fragment parameters are not allowed")
			}
		}
	}
	origin, err := profile.OriginOfURL(raw)
	if err != nil {
		return "", err
	}
	if _, err := validateCaptureOrigin(origin); err != nil {
		return "", err
	}
	return origin, nil
}

func validateCaptureOrigin(raw string) (string, error) {
	canonical, err := profile.ParseOrigin(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(canonical)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "https" {
		return canonical, nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return canonical, nil
	}
	return "", fmt.Errorf("origin must use HTTPS; HTTP is allowed only for literal loopback or localhost")
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sensitiveURLKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "auth", "code", "key", "oauth_state", "session", "session_id", "sig", "signature", "state":
		return true
	default:
		return redact.SensitiveKey(normalized)
	}
}

func originAllowed(raw string, allowed []string) bool {
	origin, err := profile.OriginOfURL(raw)
	if err != nil {
		return false
	}
	return slices.Contains(allowed, origin)
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	if values == nil {
		return nil
	}
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		result[index] = append(json.RawMessage(nil), value...)
	}
	return result
}

type requestFacts struct {
	URL           string
	Method        string
	ResourceType  string
	ChildDocument bool
}

type networkGuard struct {
	mu             sync.Mutex
	allowedOrigins []string
	core           networkGuardCore
	summary        playwrightadapter.NetworkSummary
}

func newNetworkGuard(request LiveRequest) *networkGuard {
	return &networkGuard{
		allowedOrigins: append([]string(nil), request.AllowedOrigins...),
		core:           newNetworkGuardCore(request.MaxRequests, request.MaxResponseBytes),
	}
}

func (g *networkGuard) allowRequest(facts requestFacts) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.core.beginRequest(&policyError{Code: "request_limit", Message: "capture request limit exceeded"}) {
		g.summary.BlockedRequests++
		return false
	}
	if _, err := validateCaptureURL(facts.URL); err != nil || !originAllowed(facts.URL, g.allowedOrigins) {
		g.violateLocked("origin", "request URL violates the capture origin policy")
		g.summary.BlockedRequests++
		return false
	}
	if facts.Method != "GET" && facts.Method != "HEAD" {
		g.violateLocked("method", "capture blocked a non-read-only request method")
		g.summary.BlockedRequests++
		return false
	}
	if facts.ChildDocument {
		g.violateLocked("iframe", "capture blocked a child-frame navigation")
		g.summary.BlockedRequests++
		return false
	}
	switch facts.ResourceType {
	case "document", "stylesheet", "script", "xhr", "fetch":
		return true
	case "eventsource", "websocket":
		g.violateLocked("resource_type", "capture blocked a persistent network resource")
		g.summary.BlockedRequests++
		return false
	default:
		g.summary.BlockedRequests++
		return false
	}
}

func (g *networkGuard) observeResponseContentLength(length int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.core.observeResponseContentLength(length, &policyError{Code: "response_size", Message: "capture response exceeds the byte limit"})
}

func (g *networkGuard) observeFinishedResponse(bytes int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.core.observeFinishedResponse(bytes, &policyError{Code: "response_size", Message: "capture responses exceed the byte limit"}) {
		return
	}
	g.summary.Responses++
}

func (g *networkGuard) blockWebSocket() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.summary.BlockedWebSockets++
	g.violateLocked("websocket", "capture blocked a WebSocket")
}

func (g *networkGuard) blockPopup() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.summary.BlockedPopups++
	g.violateLocked("popup", "capture blocked a popup")
}

func (g *networkGuard) blockDownload() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.summary.BlockedDownloads++
	g.violateLocked("download", "capture blocked a download")
}

func (g *networkGuard) blockDialog() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.summary.BlockedDialogs++
	g.violateLocked("dialog", "capture blocked a browser dialog")
}

func (g *networkGuard) blockFileChooser() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.summary.BlockedFileChoosers++
	g.violateLocked("file_chooser", "capture blocked a file chooser")
}

func (g *networkGuard) record(code, message string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		message += ": " + err.Error()
	}
	g.violateLocked(code, message)
}

func (g *networkGuard) violateLocked(code, message string) {
	g.core.violate(&policyError{Code: code, Message: message})
}

func (g *networkGuard) result() (playwrightadapter.NetworkSummary, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	summary := g.summary
	summary.Requests = g.core.requests
	summary.ResponseBytes = g.core.responseBytes
	return summary, g.core.result()
}

type policyError struct {
	Code    string
	Message string
}

func (e *policyError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func policyCode(err error) string {
	var target *policyError
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}
