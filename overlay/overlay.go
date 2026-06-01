// Package overlay defines the advisory OpenAPI overlay sidecar that links a
// browser-backed wrapper service's OpenAPI contract to its underlying
// browser-profile evidence.
//
// An overlay sidecar is review evidence only. It is NOT:
//   - The authoritative provider truth for the wrapper service.
//   - A replacement for the wrapper service's OpenAPI description.
//   - A portable runtime execution contract.
//
// The lifecycle of a sidecar mirrors the profile it documents:
// draft → reviewed → exported → stale/revalidate.
//
// Boundary notes:
//   - API inventory, provider ranking, and catalog metadata belong in
//     github.com/OpenUdon/apitools, not here.
//   - Browser evidence collection and review artifacts belong here.
//   - The wrapper service's OpenAPI description belongs with the service.
package overlay

import "github.com/OpenUdon/browsertools/review"

// New returns an initialized Sidecar with empty (non-nil) OperationMappings so
// JSON serializes as {} not null.
func New() Sidecar {
	return Sidecar{
		OperationMappings: map[string]OperationMapping{},
	}
}
type Lifecycle string

const (
	// LifecycleDraft means the sidecar has been generated but not yet reviewed.
	LifecycleDraft Lifecycle = "draft"
	// LifecycleReviewed means the sidecar has been reviewed and the mappings confirmed.
	LifecycleReviewed Lifecycle = "reviewed"
	// LifecycleExported means the sidecar has been exported for downstream consumption.
	LifecycleExported Lifecycle = "exported"
	// LifecycleStale means the sidecar needs revalidation (profile or API changed).
	LifecycleStale Lifecycle = "stale"
)

// OperationMapping links one OpenAPI operationId to a profile action and
// records a confidence note for the reviewer.
type OperationMapping struct {
	// OpenAPIOperationID is the operationId from the wrapper service's OpenAPI document.
	OpenAPIOperationID string `json:"openAPIOperationId"`
	// ProfileActionName is the matching action key inside the browser-profile.
	ProfileActionName string `json:"profileActionName"`
	// ConfidenceNote is a human-readable note explaining the mapping confidence
	// and any caveats (e.g. "UI path may change with site redesign").
	ConfidenceNote string `json:"confidenceNote,omitempty"`
}

// Sidecar is the advisory overlay artifact that links a browser-backed wrapper
// service's OpenAPI contract to its underlying browser-profile review bundle.
//
// All fields with json tags are stable for serialization. The ReviewBundle
// field embeds the full review.Bundle so consumers can check the promotion
// gate (Validation.Valid && len(Gaps)==0) without loading a separate file.
type Sidecar struct {
	// OverlayVersion is the sidecar format version. Currently "1".
	OverlayVersion string `json:"overlayVersion"`
	// OverlayID is a stable, unique identifier for this sidecar (e.g. a UUID
	// or a human-readable slug). Must not change once exported.
	OverlayID string `json:"overlayId"`
	// WrapperOpenAPI is the URL or relative path to the wrapper service's
	// OpenAPI document. The wrapper service's OpenAPI is the authoritative
	// contract for that service; this sidecar is advisory evidence only.
	WrapperOpenAPI string `json:"wrapperOpenAPI"`
	// BrowserProfile is the URL or relative path to the reviewed browser-profile
	// YAML file that backs the wrapper service's UI operations.
	BrowserProfile string `json:"browserProfile"`
	// ReviewBundle is the embedded review.Bundle for the browser-profile. Callers
	// MUST check ReviewBundle.Validation.Valid and len(ReviewBundle.Gaps) == 0
	// before treating the sidecar as reviewed.
	ReviewBundle review.Bundle `json:"reviewBundle"`
	// OperationMappings maps each wrapper OpenAPI operationId to the corresponding
	// browser-profile action. Keyed by OpenAPI operationId for fast lookup.
	// Initialized to an empty map so JSON serializes as {} not null.
	OperationMappings map[string]OperationMapping `json:"operationMappings"`
	// Lifecycle records the current review state of this sidecar.
	Lifecycle Lifecycle `json:"lifecycle"`
}
