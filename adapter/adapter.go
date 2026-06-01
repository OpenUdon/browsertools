// Package adapter defines the common interface and shared types for all
// browsertools evidence importers.
//
// Each adapter (playwright, llmscraper, crawl4ai, firecrawl) implements the
// Adapter interface, accepting saved fixture data and emitting normalized
// evidence.Record values. No adapter may contact live browsers or network
// services in its default code path; all imports operate on in-memory or
// on-disk fixture data.
package adapter

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/OpenUdon/browsertools/evidence"
)

// Adapter is implemented by each tool-specific evidence importer.
type Adapter interface {
	// Name returns the human-readable tool name (e.g. "playwright").
	Name() string
	// Import converts raw tool output bytes into normalized evidence records.
	// The caller must have already reviewed the raw bytes for secrets; Import
	// sets RedactionStatus based on the options supplied.
	Import(raw []byte, opts Options) ([]evidence.Record, error)
}

// Options controls how an adapter normalizes raw input.
type Options struct {
	// Origin is the browser origin (scheme+host) to record on all produced records.
	// Required.
	Origin string
	// ActionHint is the action name to record on all produced records.
	// Optional; leave empty if the fixture covers multiple actions.
	ActionHint string
	// RedactionStatus must be set by the caller to indicate whether sensitive
	// fields have been reviewed (evidence.RedactionNotRequired or
	// evidence.RedactionCompleted).
	// Defaults to evidence.RedactionNotRequired for synthetic fixtures.
	RedactionStatus evidence.RedactionStatus
	// RedactedFields lists any fields replaced by "[REDACTED]" markers.
	// Required when RedactionStatus is evidence.RedactionCompleted.
	RedactedFields []string
}

// ValidateFixtureOrigin verifies that a saved adapter fixture declares a URL
// whose canonical origin exactly matches the caller-provided allowlist origin.
func ValidateFixtureOrigin(adapterName, fixtureURL, expectedOrigin string) error {
	if fixtureURL == "" {
		return fmt.Errorf("%s: fixture url is required", adapterName)
	}
	u, err := url.Parse(fixtureURL)
	if err != nil {
		return fmt.Errorf("%s: fixture url %q is malformed: %w", adapterName, fixtureURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s: fixture url %q has unsupported scheme %q; expected http or https", adapterName, fixtureURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%s: fixture url %q is malformed: missing host", adapterName, fixtureURL)
	}
	origin := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	if origin != expectedOrigin {
		return fmt.Errorf("%s: fixture origin %q from url %q does not match opts.Origin %q", adapterName, origin, fixtureURL, expectedOrigin)
	}
	return nil
}
