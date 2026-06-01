// Package adapter defines the common interface and shared types for all
// browsertools evidence importers.
//
// Each adapter (playwright, llmscraper, crawl4ai, firecrawl) implements the
// Adapter interface, accepting saved fixture data and emitting normalized
// evidence.Record values. No adapter may contact live browsers or network
// services in its default code path; all imports operate on in-memory or
// on-disk fixture data.
package adapter

import "github.com/OpenUdon/browsertools/evidence"

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
