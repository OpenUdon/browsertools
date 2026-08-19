package registry

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/bundle"
	eartifact "github.com/OpenUdon/evidence/artifact"
	"github.com/OpenUdon/evidence/digest"
)

const (
	DefaultTimeout    = 8 * time.Second
	DefaultMaxBytes   = int64(20 << 20)
	DefaultMaxResults = 3
	DefaultMaxEntries = 10_000
)

// NetworkPolicy controls whether a registry client may read HTTPS resources.
type NetworkPolicy string

const (
	NetworkNever NetworkPolicy = "never"
	NetworkAsk   NetworkPolicy = "ask"
	NetworkAllow NetworkPolicy = "allow"
)

// Client reads local directories or static HTTPS registries.
type Client struct {
	HTTPClient         *http.Client
	NetworkPolicy      NetworkPolicy
	Timeout            time.Duration
	MaxBytes           int64
	MaxResults         int
	AllowLoopbackHosts bool
	// AllowUnsafeHosts is retained for source compatibility. Deprecated: it is
	// now a loopback-only alias and never permits private or reserved targets.
	AllowUnsafeHosts bool
}

// SearchOptions selects bounded index results.
type SearchOptions struct {
	Location        string
	Query           string
	Limit           int
	At              time.Time
	IncludeInactive bool
}

// SearchResult is one deterministic index match.
type SearchResult struct {
	Entry      Entry                     `json:"entry"`
	Score      int                       `json:"score"`
	Status     eartifact.LifecycleStatus `json:"status"`
	Registry   string                    `json:"registry"`
	Provenance string                    `json:"provenance"`
}

// SearchReport includes the selected static index and bounded results.
type SearchReport struct {
	Query    string         `json:"query"`
	Registry string         `json:"registry"`
	Results  []SearchResult `json:"results"`
}

// PullOptions selects one bundle by coordinate or canonical digest.
type PullOptions struct {
	Location      string
	Coordinate    *Coordinate
	Digest        string
	At            time.Time
	AllowInactive bool
}

// PullResult is a verified exact registry bundle.
type PullResult struct {
	Entry    Entry          `json:"entry"`
	Bundle   *bundle.Bundle `json:"-"`
	Content  []byte         `json:"-"`
	Source   string         `json:"source"`
	Referred bool           `json:"referred_by_successor,omitempty"`
}

// Search loads only index.json and returns at most the configured result
// bound. It never downloads bundle blobs.
func (client *Client) Search(ctx context.Context, options SearchOptions) (SearchReport, error) {
	if options.At.IsZero() {
		return SearchReport{}, fmt.Errorf("%w: registry search time is required", ErrValidation)
	}
	ctx, cancel := client.withDeadline(ctx)
	defer cancel()
	index, source, _, err := client.loadIndex(ctx, options.Location)
	if err != nil {
		return SearchReport{}, err
	}
	limit := options.Limit
	if limit <= 0 {
		limit = client.maxResults()
	}
	if limit > client.maxResults() {
		limit = client.maxResults()
	}
	query := strings.ToLower(strings.TrimSpace(options.Query))
	var matches []SearchResult
	for _, entry := range index.Entries {
		status := eartifact.EffectiveStatus(entry.Lifecycle, options.At)
		if status != eartifact.LifecycleActive && !options.IncludeInactive {
			continue
		}
		score := scoreEntry(entry, query, status)
		if query != "" && score == 0 {
			continue
		}
		matches = append(matches, SearchResult{
			Entry: entry, Score: score, Status: status, Registry: source, Provenance: entry.Provenance.Source,
		})
	}
	slices.SortFunc(matches, compareSearchResults)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	if matches == nil {
		matches = []SearchResult{}
	}
	return SearchReport{Query: strings.TrimSpace(options.Query), Registry: source, Results: matches}, nil
}

// Pull downloads or reads one exact bundle, verifies every descriptor and
// metadata binding, and enforces lifecycle at the caller-supplied time.
func (client *Client) Pull(ctx context.Context, options PullOptions) (PullResult, error) {
	if options.At.IsZero() {
		return PullResult{}, fmt.Errorf("%w: registry pull time is required", ErrValidation)
	}
	if (options.Coordinate == nil) == (strings.TrimSpace(options.Digest) == "") {
		return PullResult{}, fmt.Errorf("%w: registry pull requires exactly one coordinate or digest", ErrValidation)
	}
	ctx, cancel := client.withDeadline(ctx)
	defer cancel()
	index, source, remoteBase, err := client.loadIndex(ctx, options.Location)
	if err != nil {
		return PullResult{}, err
	}
	position := -1
	for indexPosition, entry := range index.Entries {
		if options.Coordinate != nil && entry.ID == strings.TrimSpace(options.Coordinate.ID) && entry.Release == strings.TrimSpace(options.Coordinate.Release) {
			position = indexPosition
			break
		}
		if options.Coordinate == nil && entry.Bundle.Digest.String() == strings.TrimSpace(options.Digest) {
			position = indexPosition
			break
		}
	}
	if position < 0 {
		return PullResult{}, fmt.Errorf("%w: registry bundle was not found", ErrConflict)
	}
	entry := index.Entries[position]
	status := eartifact.EffectiveStatus(entry.Lifecycle, options.At)
	if status != eartifact.LifecycleActive && !options.AllowInactive {
		return PullResult{}, fmt.Errorf("%w: registry bundle lifecycle is %q", ErrExpired, status)
	}
	data, blobSource, err := client.loadBlob(ctx, options.Location, remoteBase, entry.Bundle.Digest)
	if err != nil {
		return PullResult{}, err
	}
	if err := verifyEntryBytes(entry, data); err != nil {
		return PullResult{}, err
	}
	value, err := bundle.Parse(data)
	if err != nil {
		return PullResult{}, err
	}
	if status == eartifact.LifecycleActive {
		if err := bundle.Verify(value, options.At); err != nil {
			return PullResult{}, err
		}
	}
	return PullResult{Entry: entry, Bundle: value, Content: data, Source: firstNonEmpty(blobSource, source)}, nil
}

// Verify reads every referenced static blob and returns the same integrity
// report shape for local and HTTPS registries.
func (client *Client) Verify(ctx context.Context, location string, at time.Time) (VerifyReport, error) {
	if at.IsZero() {
		return VerifyReport{}, fmt.Errorf("%w: registry verification time is required", ErrValidation)
	}
	if !isRemoteLocation(location) {
		return VerifyLocal(ctx, location, at)
	}
	ctx, cancel := client.withDeadline(ctx)
	defer cancel()
	index, source, remoteBase, err := client.loadIndex(ctx, location)
	if err != nil {
		return VerifyReport{}, err
	}
	report := VerifyReport{IndexPath: source, Entries: make([]Verification, 0, len(index.Entries))}
	for _, entry := range index.Entries {
		data, blobSource, err := client.loadBlob(ctx, location, remoteBase, entry.Bundle.Digest)
		if err != nil {
			return VerifyReport{}, err
		}
		if err := verifyEntryBytes(entry, data); err != nil {
			return VerifyReport{}, err
		}
		report.Entries = append(report.Entries, Verification{
			Coordinate: Coordinate{ID: entry.ID, Release: entry.Release}, Digest: entry.Bundle.Digest.String(),
			Status: eartifact.EffectiveStatus(entry.Lifecycle, at), BlobPath: blobSource,
		})
	}
	return report, nil
}

func (client *Client) loadIndex(ctx context.Context, location string) (Index, string, *url.URL, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return Index{}, "", nil, fmt.Errorf("registry location is required")
	}
	if !isRemoteLocation(location) {
		root, err := validateRoot(location)
		if err != nil {
			return Index{}, "", nil, err
		}
		path := filepath.Join(root, IndexName)
		data, err := readRegularBounded(ctx, path, client.maxBytes())
		if err != nil {
			return Index{}, "", nil, err
		}
		index, err := ParseIndex(data)
		return index, path, nil, err
	}
	if err := client.requireNetwork(); err != nil {
		return Index{}, "", nil, err
	}
	base, err := client.registryBaseURL(ctx, location)
	if err != nil {
		return Index{}, "", nil, err
	}
	indexURL := base.ResolveReference(&url.URL{Path: IndexName})
	data, finalURL, err := client.download(ctx, indexURL)
	if err != nil {
		return Index{}, "", nil, err
	}
	index, err := ParseIndex(data)
	if err != nil {
		return Index{}, "", nil, err
	}
	finalBase := *finalURL
	finalBase.RawQuery, finalBase.Fragment = "", ""
	finalBase.Path = strings.TrimSuffix(finalBase.Path, IndexName)
	if !strings.HasSuffix(finalBase.Path, "/") {
		finalBase.Path += "/"
	}
	return index, finalURL.String(), &finalBase, nil
}

func (client *Client) loadBlob(ctx context.Context, location string, remoteBase *url.URL, record digest.Record) ([]byte, string, error) {
	relative, err := BlobPath(record)
	if err != nil {
		return nil, "", err
	}
	if remoteBase == nil {
		root, err := validateRoot(location)
		if err != nil {
			return nil, "", err
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		data, err := readRegularBounded(ctx, path, client.maxBytes())
		return data, path, err
	}
	target := remoteBase.ResolveReference(&url.URL{Path: relative})
	data, finalURL, err := client.download(ctx, target)
	if err != nil {
		return nil, "", err
	}
	return data, finalURL.String(), nil
}

func (client *Client) download(ctx context.Context, target *url.URL) ([]byte, *url.URL, error) {
	if target == nil {
		return nil, nil, fmt.Errorf("registry URL is required")
	}
	if target.Scheme != "https" {
		return nil, nil, fmt.Errorf("registry URL scheme must be https")
	}
	if target.User != nil {
		return nil, nil, fmt.Errorf("registry URL must not contain user information")
	}
	if err := client.rejectHost(ctx, target.Hostname()); err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	request.Header.Set("Accept", "application/json")
	httpClient, err := client.safeHTTPClient(ctx, target.Hostname())
	if err != nil {
		return nil, nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("registry download: %s", response.Status)
	}
	if response.ContentLength > client.maxBytes() {
		return nil, nil, fmt.Errorf("%w: registry response exceeds %d bytes", ErrLimit, client.maxBytes())
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, client.maxBytes()+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > client.maxBytes() {
		return nil, nil, fmt.Errorf("%w: registry response exceeds %d bytes", ErrLimit, client.maxBytes())
	}
	finalURL := target
	if response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL
	}
	if err := client.rejectHost(ctx, finalURL.Hostname()); err != nil {
		return nil, nil, err
	}
	return data, finalURL, nil
}

func (client *Client) registryBaseURL(ctx context.Context, raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("valid registry HTTPS URL is required")
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("registry URL scheme must be https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("registry URL must not contain user information, query, or fragment")
	}
	if err := client.rejectHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	if strings.HasSuffix(parsed.Path, "/"+IndexName) || parsed.Path == IndexName {
		parsed.Path = strings.TrimSuffix(parsed.Path, IndexName)
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed, nil
}

func (client *Client) safeHTTPClient(ctx context.Context, host string) (*http.Client, error) {
	var base *http.Client
	if client != nil {
		base = client.HTTPClient
	}
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	transport, ok := base.Transport.(*http.Transport)
	if base.Transport == nil {
		transport, ok = http.DefaultTransport.(*http.Transport)
	}
	if !ok {
		loopback, err := client.hostIsLoopback(ctx, host)
		if err != nil {
			return nil, err
		}
		if !client.allowLoopback() || !loopback {
			return nil, fmt.Errorf("%w: custom HTTP transport is permitted only for an opted-in loopback registry", ErrPolicy)
		}
	} else {
		transport = transport.Clone()
		transport.Proxy = nil
		transport.DialContext = client.safeDialContext
		//lint:ignore SA1019 A cloned legacy transport may still carry the deprecated callback; clearing it is required so HTTPS cannot bypass safeDialContext.
		transport.DialTLS = nil
		transport.DialTLSContext = nil
		clone.Transport = transport
	}
	baseRedirect := base.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many registry redirects")
		}
		if request == nil || request.URL == nil || request.URL.Scheme != "https" {
			return fmt.Errorf("registry redirect target must use https")
		}
		if err := client.rejectHost(request.Context(), request.URL.Hostname()); err != nil {
			return err
		}
		if baseRedirect != nil {
			return baseRedirect(request, via)
		}
		return nil
	}
	return &clone, nil
}

func (client *Client) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, address := range addresses {
		if !client.ipAllowed(address.IP) {
			if firstErr == nil {
				firstErr = fmt.Errorf("refusing private registry host %q", host)
			}
			continue
		}
		connection, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if err == nil {
			return connection, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, fmt.Errorf("no public IP addresses found for %q", host)
}

func (client *Client) rejectHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("registry URL host is required")
	}
	if strings.EqualFold(host, "localhost") {
		if client.allowLoopback() {
			return nil
		}
		return fmt.Errorf("%w: refusing loopback registry host %q", ErrPolicy, host)
	}
	if parsed := net.ParseIP(host); parsed != nil {
		if !client.ipAllowed(parsed) {
			return fmt.Errorf("%w: refusing private or reserved registry host %q", ErrPolicy, host)
		}
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, address := range addresses {
		if !client.ipAllowed(address.IP) {
			return fmt.Errorf("%w: refusing private or reserved registry host %q", ErrPolicy, host)
		}
	}
	return nil
}

func unsafeIP(value net.IP) bool {
	address, ok := netip.AddrFromSlice(value)
	if !ok {
		return true
	}
	address = address.Unmap()
	if address.Is4() {
		return addressInAnyPrefix(address, unsafeIPv4Prefixes)
	}
	bytes := address.As16()
	if bytes[0] == 0x20 && bytes[1] == 0x02 { // 6to4 embeds IPv4 at bits 16..48.
		embedded := netip.AddrFrom4([4]byte{bytes[2], bytes[3], bytes[4], bytes[5]})
		if addressInAnyPrefix(embedded, unsafeIPv4Prefixes) {
			return true
		}
	}
	if bytes[0] == 0x20 && bytes[1] == 0x01 && bytes[2] == 0 && bytes[3] == 0 { // Teredo.
		server := netip.AddrFrom4([4]byte{bytes[4], bytes[5], bytes[6], bytes[7]})
		client := netip.AddrFrom4([4]byte{^bytes[12], ^bytes[13], ^bytes[14], ^bytes[15]})
		if addressInAnyPrefix(server, unsafeIPv4Prefixes) || addressInAnyPrefix(client, unsafeIPv4Prefixes) {
			return true
		}
	}
	return addressInAnyPrefix(address, unsafeIPv6Prefixes)
}

var unsafeIPv4Prefixes = mustPrefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
)

var unsafeIPv6Prefixes = mustPrefixes(
	"::/128", "::1/128", "100::/64", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8",
)

func mustPrefixes(values ...string) []netip.Prefix {
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}

func addressInAnyPrefix(address netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (client *Client) allowLoopback() bool {
	return client != nil && (client.AllowLoopbackHosts || client.AllowUnsafeHosts)
}

func (client *Client) ipAllowed(value net.IP) bool {
	return !unsafeIP(value) || client.allowLoopback() && value.IsLoopback()
}

func (client *Client) hostIsLoopback(ctx context.Context, host string) (bool, error) {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true, nil
	}
	if parsed := net.ParseIP(host); parsed != nil {
		return parsed.IsLoopback(), nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return false, err
	}
	if len(addresses) == 0 {
		return false, fmt.Errorf("registry host %q resolved to no addresses", host)
	}
	for _, address := range addresses {
		if !address.IP.IsLoopback() {
			return false, nil
		}
	}
	return true, nil
}

func (client *Client) requireNetwork() error {
	if client == nil {
		return fmt.Errorf("%w: registry network policy forbids remote reads", ErrPolicy)
	}
	switch client.NetworkPolicy {
	case NetworkAllow:
		return nil
	case NetworkAsk:
		return fmt.Errorf("%w: registry network approval is required", ErrPolicy)
	case NetworkNever, "":
		return fmt.Errorf("%w: registry network policy forbids remote reads", ErrPolicy)
	default:
		return fmt.Errorf("%w: registry network policy must be never, ask, or allow", ErrValidation)
	}
}

func (client *Client) withDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := time.Duration(0)
	if client != nil {
		timeout = client.Timeout
	}
	if timeout <= 0 || timeout > DefaultTimeout {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (client *Client) maxBytes() int64 {
	if client == nil {
		return DefaultMaxBytes
	}
	if client.MaxBytes <= 0 || client.MaxBytes > DefaultMaxBytes {
		return DefaultMaxBytes
	}
	return client.MaxBytes
}

func (client *Client) maxResults() int {
	if client == nil {
		return DefaultMaxResults
	}
	if client.MaxResults <= 0 {
		return DefaultMaxResults
	}
	return client.MaxResults
}

func scoreEntry(entry Entry, query string, status eartifact.LifecycleStatus) int {
	if query == "" {
		if status == eartifact.LifecycleActive {
			return 100 + entry.ActionCount
		}
		return entry.ActionCount
	}
	score := 0
	id := strings.ToLower(entry.ID)
	title := strings.ToLower(entry.Title)
	if id == query {
		score += 100
	} else if strings.Contains(id, query) {
		score += 60
	}
	if title == query {
		score += 80
	} else if strings.Contains(title, query) {
		score += 40
	}
	for _, value := range append(append([]string(nil), entry.Origins...), entry.Actions...) {
		if strings.Contains(strings.ToLower(value), query) {
			score += 20
		}
	}
	if status == eartifact.LifecycleActive && score > 0 {
		score += 10
	}
	return score
}

func compareSearchResults(a, b SearchResult) int {
	if a.Score != b.Score {
		return b.Score - a.Score
	}
	if result := strings.Compare(a.Entry.ID, b.Entry.ID); result != 0 {
		return result
	}
	return strings.Compare(b.Entry.Release, a.Entry.Release)
}

func isRemoteLocation(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
