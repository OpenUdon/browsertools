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

	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
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
	// Origin is the allowed browser origin (scheme+host) for all produced records.
	// Importers emit its canonical spelling. Required.
	Origin string
	// ActionHint is the action name to record on all produced records.
	// Optional; leave empty if the fixture covers multiple actions.
	ActionHint string
	// RedactionStatus must be set by the caller to indicate whether sensitive
	// fields have been reviewed (evidence.RedactionNotRequired or
	// evidence.RedactionCompleted).
	// It has no implicit default, including for synthetic fixtures.
	RedactionStatus evidence.RedactionStatus
	// RedactedFields lists any fields replaced by "[REDACTED]" markers.
	// Required when RedactionStatus is evidence.RedactionCompleted.
	RedactedFields []string
}

// CanonicalFixtureOrigin verifies that a saved adapter fixture declares a URL
// whose canonical origin exactly matches the caller-provided allowlist origin,
// then returns that canonical origin for use in emitted evidence.
func CanonicalFixtureOrigin(adapterName, fixtureURL, expectedOrigin string) (string, error) {
	if fixtureURL == "" {
		return "", fmt.Errorf("%s: fixture url is required", adapterName)
	}
	origin, err := profile.OriginOfURL(fixtureURL)
	if err != nil {
		return "", fmt.Errorf("%s: fixture url %q is malformed: %w", adapterName, fixtureURL, err)
	}
	expected, err := profile.ParseOrigin(expectedOrigin)
	if err != nil {
		return "", fmt.Errorf("%s: opts.Origin %q is invalid: %w", adapterName, expectedOrigin, err)
	}
	if origin != expected {
		return "", fmt.Errorf("%s: fixture origin %q from url %q does not match opts.Origin %q", adapterName, origin, fixtureURL, expectedOrigin)
	}
	return origin, nil
}

// ValidateFixtureOrigin verifies that a saved adapter fixture declares a URL
// whose canonical origin exactly matches the caller-provided allowlist origin.
// CanonicalFixtureOrigin should be used by importers that emit evidence.
func ValidateFixtureOrigin(adapterName, fixtureURL, expectedOrigin string) error {
	_, err := CanonicalFixtureOrigin(adapterName, fixtureURL, expectedOrigin)
	return err
}
