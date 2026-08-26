// Package registrationauthorsession implements the strict, value-free local
// protocol for no-submit browser registration-profile authoring.
//
// The package is intentionally separate from authorsession: its browser
// surface can observe and navigate with GET/HEAD only and has no API for
// focus, input, click, submit, POST approval, browser state, or session export.
package registrationauthorsession

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/authorsession"
	"github.com/OpenUdon/browsertools/disclosurepath"
	"github.com/OpenUdon/browsertools/internal/registrationurl"
	"github.com/OpenUdon/browsertools/internal/strictjson"
	"github.com/OpenUdon/browsertools/registrationprofile"
)

const (
	ProtocolV1 = "browsertools.registration-author-session.v1"
	ProtocolV2 = "browsertools.registration-author-session.v2"
	// Protocol is the immutable legacy default.
	Protocol = ProtocolV1
)

// AccessibilityLabelDisclosure must be shown anywhere registration
// observations or reviewed candidates are displayed or retained.
const AccessibilityLabelDisclosure = authorsession.AccessibilityLabelDisclosure

const (
	DiagnosticAccessibilitySnapshotPartial = "accessibility_snapshot_partial"
	DiagnosticCrossOriginFrameOmitted      = "cross_origin_frame_omitted"
	DiagnosticUnsupportedAccessibleControl = "unsupported_accessible_control"
	DiagnosticSyntheticFixture             = "synthetic_fixture"
)

const (
	DefaultNavigationTimeout  = 20 * time.Second
	DefaultTotalTimeout       = 5 * time.Minute
	DefaultMaxRequests        = 256
	DefaultMaxResponseBytes   = int64(32 << 20)
	DefaultMaxObservations    = 64
	DefaultMaxCandidates      = 128
	MaxUniqueDiagnostics      = 256
	MaxProtocolLineBytes      = 256 << 10
	MaxRawCandidateLabelBytes = 4 << 10
	maxJSONDepth              = 32
)

// Bounds fixes all finite browser-work and protocol collection limits.
type Bounds struct {
	NavigationTimeoutMS int64 `json:"navigationTimeoutMs"`
	TotalTimeoutMS      int64 `json:"totalTimeoutMs"`
	MaxRequests         int   `json:"maxRequests"`
	MaxResponseBytes    int64 `json:"maxResponseBytes"`
	MaxObservations     int   `json:"maxObservations"`
	MaxCandidates       int   `json:"maxCandidates"`
}

// ClientMessage is the closed NDJSON input union. Profile is admitted only on
// review and must be one complete, secret-free UWS registration profile.
type ClientMessage struct {
	Protocol           string          `json:"protocol"`
	Type               string          `json:"type"`
	ProfileID          string          `json:"profileId,omitempty"`
	URL                string          `json:"url,omitempty"`
	Origins            []string        `json:"origins,omitempty"`
	Bounds             *Bounds         `json:"bounds,omitempty"`
	Method             string          `json:"method,omitempty"`
	Profile            json.RawMessage `json:"profile,omitempty"`
	CandidateIDs       []string        `json:"candidateIds,omitempty"`
	Flow               string          `json:"flow,omitempty"`
	CleanupDisposition string          `json:"cleanupDisposition,omitempty"`
}

// RawObservation is backend-only evidence. Raw labels never cross the
// protocol boundary.
type RawObservation struct {
	Origin      string
	Path        string
	Candidates  []RawCandidate
	Diagnostics []string
}

// RawCandidate identifies one backend-owned accessible semantic target.
// Matches is the complete count for the reduced role/name locator; a backend
// must not split the same reduced locator across multiple candidates.
type RawCandidate struct {
	Role    string
	Label   string
	Matches int
}

// Candidate is one reduced current-generation protocol candidate.
type Candidate struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Label   string `json:"label,omitempty"`
	Matches int    `json:"matches"`
}

// Observation is the complete page-derived protocol payload.
type Observation struct {
	Generation  int         `json:"generation"`
	Origin      string      `json:"origin"`
	Path        string      `json:"path"`
	Candidates  []Candidate `json:"candidates"`
	Diagnostics []string    `json:"diagnostics"`
}

// ServerMessage is the closed NDJSON output union. M26 has no result path or
// runtime outcome field; the private result contract is owned separately.
type ServerMessage struct {
	Protocol     string       `json:"protocol"`
	Type         string       `json:"type"`
	Capabilities []string     `json:"capabilities,omitempty"`
	Bounds       *Bounds      `json:"bounds,omitempty"`
	Phase        string       `json:"phase,omitempty"`
	Observation  *Observation `json:"observation,omitempty"`
	Diagnostic   *Diagnostic  `json:"diagnostic,omitempty"`
}

// Diagnostic contains one fixed code and never backend prose.
type Diagnostic struct {
	Code string `json:"code"`
}

// NetworkSummary is returned only by closing the browser backend. Requests
// must equal GETRequests plus HEADRequests; there is no mutation counter
// because the backend contract cannot admit a mutation request.
type NetworkSummary struct {
	Requests     int `json:"requests"`
	GETRequests  int `json:"getRequests"`
	HEADRequests int `json:"headRequests"`
}

// ReviewedCandidate binds one exact current-generation reduced candidate.
type ReviewedCandidate struct {
	ID         string `json:"id"`
	Generation int    `json:"generation"`
	Role       string `json:"role"`
	Label      string `json:"label,omitempty"`
	Matches    int    `json:"matches"`
}

// Completion is the in-process, value-free handoff to the private result
// builder. It is returned only after clean browser teardown and zero-mutation
// network-summary validation.
type Completion struct {
	Protocol           string
	ProfileID          string
	Profile            registrationprofile.Profile
	ProfileBytes       []byte
	ReviewedCandidates []ReviewedCandidate
	Flow               string
	CleanupDisposition string
	Origins            []string
	ObservedAt         time.Time
	Bounds             Bounds
	Observations       int
	Diagnostics        []string
	Network            NetworkSummary
}

type candidateRecord struct {
	protocol   Candidate
	generation int
}

func decodeClientMessage(line []byte) (ClientMessage, error) {
	if err := strictjson.Validate(line, MaxProtocolLineBytes, maxJSONDepth); err != nil {
		return ClientMessage{}, err
	}
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &header); err != nil {
		return ClientMessage{}, err
	}
	allowed, ok := clientFields[header.Type]
	if !ok {
		allowed = clientFields["unknown"]
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return ClientMessage{}, err
	}
	for field := range fields {
		if !allowed[field] {
			return ClientMessage{}, fmt.Errorf("field %q is not allowed for %q", field, header.Type)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	var message ClientMessage
	if err := decoder.Decode(&message); err != nil {
		return ClientMessage{}, err
	}
	return message, nil
}

func validateBounds(value *Bounds) error {
	if value == nil {
		return nil
	}
	if value.NavigationTimeoutMS <= 0 || value.NavigationTimeoutMS > time.Minute.Milliseconds() ||
		value.TotalTimeoutMS < value.NavigationTimeoutMS || value.TotalTimeoutMS > (30*time.Minute).Milliseconds() ||
		value.MaxRequests <= 0 || value.MaxRequests > 4096 ||
		value.MaxResponseBytes <= 0 || value.MaxResponseBytes > 128<<20 ||
		value.MaxObservations <= 0 || value.MaxObservations > 256 ||
		value.MaxCandidates <= 0 || value.MaxCandidates > 512 {
		return errors.New("registration author-session bounds are invalid")
	}
	return nil
}

func normalizedBounds(value *Bounds) Bounds {
	result := Bounds{
		NavigationTimeoutMS: DefaultNavigationTimeout.Milliseconds(),
		TotalTimeoutMS:      DefaultTotalTimeout.Milliseconds(),
		MaxRequests:         DefaultMaxRequests,
		MaxResponseBytes:    DefaultMaxResponseBytes,
		MaxObservations:     DefaultMaxObservations,
		MaxCandidates:       DefaultMaxCandidates,
	}
	if value != nil {
		result = *value
	}
	return result
}

func exactOrigin(raw string) (string, error) {
	if raw != strings.TrimSpace(raw) || utf8.RuneCountInString(raw) > 1024 {
		return "", errors.New("origin must be canonical and bounded")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("origin must contain only scheme, host, and optional port")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", errors.New("origin must use HTTPS or loopback HTTP")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", errors.New("origin hostname is required")
	}
	canonicalHostname := hostname
	if ip := net.ParseIP(hostname); ip != nil {
		canonicalHostname = ip.String()
	} else if strings.Contains(hostname, ":") || !asciiOnly(hostname) {
		return "", errors.New("origin hostname is not canonical")
	}
	if scheme == "http" && hostname != "localhost" {
		ip := net.ParseIP(hostname)
		if ip == nil || !ip.IsLoopback() {
			return "", errors.New("origin must use HTTPS or loopback HTTP")
		}
	}
	port := parsed.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		defaultPort := scheme == "https" && number == 443 || scheme == "http" && number == 80
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port || defaultPort {
			return "", errors.New("origin port is not canonical")
		}
	}
	canonicalHost := canonicalHostname
	if strings.Contains(canonicalHostname, ":") {
		canonicalHost = "[" + canonicalHostname + "]"
	}
	if port != "" {
		canonicalHost = net.JoinHostPort(canonicalHostname, port)
	}
	canonical := scheme + "://" + canonicalHost
	if raw != canonical {
		return "", errors.New("origin is not canonical")
	}
	return canonical, nil
}

func cleanURL(raw string) (string, string, error) {
	return cleanURLForProtocol(ProtocolV1, raw)
}

func cleanURLForProtocol(protocol, raw string) (string, string, error) {
	if protocol == ProtocolV2 {
		facts, err := registrationurl.Parse(raw, true, exactOrigin)
		if err != nil {
			return "", "", err
		}
		return facts.URL, facts.Origin, nil
	}
	if raw != strings.TrimSpace(raw) || len(raw) > 4096 {
		return "", "", errors.New("URL must be canonical and bounded")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("URL must be absolute, query-free, and fragment-free")
	}
	origin, err := exactOrigin(parsed.Scheme + "://" + parsed.Host)
	if err != nil {
		return "", "", err
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if disclosurepath.Validate(path) != nil {
		return "", "", errors.New("URL path is not disclosure-safe")
	}
	return origin + path, origin, nil
}

// ValidateNavigationURL validates one session URL under an explicit protocol
// and returns only canonical URL, origin, and disclosure-safe path facts.
func ValidateNavigationURL(protocol, raw string) (string, string, string, error) {
	if protocol != ProtocolV1 && protocol != ProtocolV2 {
		return "", "", "", errors.New("registration author-session protocol is unsupported")
	}
	facts, err := registrationurl.Parse(raw, protocol == ProtocolV2, exactOrigin)
	if err != nil {
		return "", "", "", err
	}
	return facts.URL, facts.Origin, facts.Path, nil
}

func normalizeOrigins(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 32 {
		return nil, errors.New("one to 32 approved origins are required")
	}
	if !sort.StringsAreSorted(values) {
		return nil, errors.New("approved origins are not in canonical order")
	}
	result := make([]string, len(values))
	for index, value := range values {
		canonical, err := exactOrigin(value)
		if err != nil {
			return nil, err
		}
		result[index] = canonical
	}
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, errors.New("approved origins are duplicated")
		}
	}
	return result, nil
}

func validateNetworkSummary(summary NetworkSummary, maximum int) error {
	if summary.Requests < 0 || summary.GETRequests < 0 || summary.HEADRequests < 0 ||
		summary.Requests != summary.GETRequests+summary.HEADRequests || summary.Requests > maximum {
		return errors.New("browser network summary is invalid")
	}
	return nil
}

func asciiOnly(value string) bool {
	for _, character := range value {
		if character > 127 {
			return false
		}
	}
	return true
}

func safeCandidateLabel(value string) bool {
	return value == "" || value == authorsession.RedactedLabel || value == authorsession.UntrustedLabel ||
		(len(value) <= 256 && authorsession.ReduceAccessibilityLabel(value).Value == value)
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	portableRoles     = map[string]bool{
		"button": true, "link": true, "textbox": true, "checkbox": true, "radio": true,
		"dialog": true, "status": true, "alert": true, "heading": true, "img": true,
		"list": true, "listitem": true, "combobox": true, "option": true, "menu": true,
		"menuitem": true, "tab": true, "tabpanel": true, "table": true, "row": true,
		"cell": true, "region": true, "navigation": true, "article": true, "form": true,
		"search": true, "switch": true, "group": true,
	}
	allowedDiagnostics = map[string]bool{
		DiagnosticAccessibilitySnapshotPartial: true,
		DiagnosticCrossOriginFrameOmitted:      true,
		DiagnosticUnsupportedAccessibleControl: true,
		DiagnosticSyntheticFixture:             true,
	}
	clientFields = map[string]map[string]bool{
		"start":    fields("protocol", "type", "profileId", "url", "origins", "bounds"),
		"observe":  fields("protocol", "type"),
		"navigate": fields("protocol", "type", "method", "url"),
		"review":   fields("protocol", "type", "profile", "candidateIds", "flow", "cleanupDisposition"),
		"finish":   fields("protocol", "type"),
		"close":    fields("protocol", "type"),
		"unknown":  fields("protocol", "type"),
	}
	phaseMessages = map[string]map[string]bool{
		"awaiting_start": fields("start", "close"),
		"observing":      fields("observe", "navigate", "review", "close"),
		"reviewed":       fields("finish", "close"),
	}
)

// ValidDiagnostic reports whether code is in the closed, value-free backend
// diagnostic vocabulary.
func ValidDiagnostic(code string) bool {
	return allowedDiagnostics[code]
}

func fields(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}
