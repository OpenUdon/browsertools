// Package bundle builds and verifies inert, content-addressed publication
// bundles for reviewed browser capabilities.
package bundle

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/cache"
	bevidence "github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
	eartifact "github.com/OpenUdon/evidence/artifact"
	"github.com/OpenUdon/evidence/digest"
	"github.com/OpenUdon/evidence/redact"
	"github.com/OpenUdon/uws/uws1"
	"github.com/OpenUdon/uws/versions"
	"gopkg.in/yaml.v3"
)

const (
	Version           = "browsertools.capability-bundle.v1"
	MediaType         = "application/vnd.openudon.browser-capability-bundle.v1+json"
	PayloadMediaType  = "application/vnd.openudon.browser-capability-payload.v1+json"
	ProfileMediaType  = "application/vnd.openudon.browser-profile.v1+json"
	ReviewMediaType   = "application/vnd.openudon.browser-review.v1+json"
	EvidenceMediaType = "application/vnd.openudon.browser-evidence.v1+json"

	UWSJSONMediaType = "application/vnd.openudon.uws+json"
	UWSYAMLMediaType = "application/vnd.openudon.uws+yaml"

	MaxBundleBytes = int64(20 << 20)
)

var (
	idPattern      = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._/-]*[a-z0-9])?$`)
	releasePattern = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	licensePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*(?: WITH [A-Za-z0-9][A-Za-z0-9.+-]*)?$`)
)

// Identity is the stable catalog identity and immutable release coordinate.
type Identity struct {
	ID      string   `json:"id"`
	Release string   `json:"release"`
	Title   string   `json:"title"`
	Origins []string `json:"origins"`
}

// Provenance records publication-safe contribution metadata. It never carries
// browser session state or credentials.
type Provenance struct {
	Source       string           `json:"source"`
	License      string           `json:"license"`
	Authors      []string         `json:"authors,omitempty"`
	CacheEntries []CacheReference `json:"cache_entries,omitempty"`
}

// CacheReference identifies exact publication-eligible bytes that contributed
// to a bundle. Raw cache content cannot produce this record.
type CacheReference struct {
	ID          string            `json:"id"`
	Kind        cache.Kind        `json:"kind"`
	MediaType   string            `json:"media_type"`
	SizeBytes   int64             `json:"size_bytes"`
	Digest      string            `json:"digest"`
	Source      string            `json:"source,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Companion is an optional inert UWS workflow document bound into the payload.
// Content uses JSON's canonical base64 encoding for exact, format-preserving
// bytes. Runtime sessions and executable drivers are never companions.
type Companion struct {
	Path       string               `json:"path"`
	MediaType  string               `json:"media_type"`
	Content    []byte               `json:"content"`
	Descriptor eartifact.Descriptor `json:"descriptor"`
}

// Payload is the deterministic, independently digestible publication content.
type Payload struct {
	Version     string             `json:"version"`
	Identity    Identity           `json:"identity"`
	PublishedAt time.Time          `json:"published_at"`
	Profile     profile.Profile    `json:"profile"`
	Review      review.Bundle      `json:"review"`
	Evidence    []bevidence.Record `json:"evidence"`
	Companions  []Companion        `json:"companions,omitempty"`
	Provenance  Provenance         `json:"provenance"`
}

// Bundle binds a canonical payload to product-neutral Evidence descriptor and
// lifecycle records. It is a data artifact only and has no execution methods.
type Bundle struct {
	Version    string               `json:"version"`
	Payload    Payload              `json:"payload"`
	Descriptor eartifact.Descriptor `json:"descriptor"`
	Assessment eartifact.Assessment `json:"assessment"`
}

// CachedArtifact supplies exact cache bytes to Build. Passing only a manifest
// is intentionally insufficient because publication must revalidate content.
type CachedArtifact struct {
	Entry   cache.Entry
	Content []byte
}

// BuildOptions are all explicit inputs to a deterministic bundle build.
type BuildOptions struct {
	ID           string
	Release      string
	Source       string
	License      string
	Authors      []string
	Profile      *profile.Profile
	Review       *review.Bundle
	Evidence     []bevidence.Record
	Companions   []Companion
	CacheEntries []CachedArtifact
	PublishedAt  time.Time
}

// Build constructs and verifies a publication bundle. It does not write a
// file, contact a registry, launch a browser, or execute an action.
func Build(options BuildOptions) (*Bundle, error) {
	payload, supporting, err := buildPayload(options)
	if err != nil {
		return nil, err
	}
	payloadJSON, err := canonicalPayloadJSON(payload)
	if err != nil {
		return nil, err
	}
	if int64(len(payloadJSON)) > MaxBundleBytes {
		return nil, fmt.Errorf("bundle payload exceeds %d bytes", MaxBundleBytes)
	}
	descriptor := descriptorFor(payloadJSON, PayloadMediaType, map[string]string{
		"browsertools.id":      payload.Identity.ID,
		"browsertools.release": payload.Identity.Release,
	})
	assessment := eartifact.Assessment{
		Version:    eartifact.AssessmentVersion,
		Subject:    descriptor,
		Status:     eartifact.LifecycleActive,
		AssessedAt: payload.PublishedAt,
		ExpiresAt:  profileExpiry(&payload.Profile),
		Supporting: uniqueDescriptors(supporting),
	}
	if err := eartifact.ValidateAssessment(assessment); err != nil {
		return nil, fmt.Errorf("bundle assessment: %w", err)
	}
	result := &Bundle{Version: Version, Payload: payload, Descriptor: descriptor, Assessment: assessment}
	if err := Verify(result, options.PublishedAt); err != nil {
		return nil, err
	}
	return result, nil
}

func buildPayload(options BuildOptions) (Payload, []eartifact.Descriptor, error) {
	if options.Profile == nil || options.Review == nil {
		return Payload{}, nil, fmt.Errorf("bundle profile and review are required")
	}
	if options.PublishedAt.IsZero() {
		return Payload{}, nil, fmt.Errorf("bundle published time is required")
	}
	publishedAt := options.PublishedAt.UTC().Round(0)
	profileCopy, err := cloneProfile(options.Profile)
	if err != nil {
		return Payload{}, nil, fmt.Errorf("bundle profile: %w", err)
	}
	reviewCopy, err := cloneReview(options.Review)
	if err != nil {
		return Payload{}, nil, fmt.Errorf("bundle review: %w", err)
	}
	evidenceCopy, err := canonicalEvidence(options.Evidence)
	if err != nil {
		return Payload{}, nil, err
	}
	companions, companionDescriptors, err := normalizeCompanions(options.Companions)
	if err != nil {
		return Payload{}, nil, err
	}
	cacheReferences, cacheDescriptors, err := normalizeCacheEntries(options.CacheEntries, publishedAt)
	if err != nil {
		return Payload{}, nil, err
	}
	payload := Payload{
		Version: Version,
		Identity: Identity{
			ID: strings.TrimSpace(options.ID), Release: strings.TrimSpace(options.Release),
			Title: strings.TrimSpace(profileCopy.Info.Title), Origins: append([]string(nil), profileCopy.Info.Origin...),
		},
		PublishedAt: publishedAt,
		Profile:     *profileCopy,
		Review:      *reviewCopy,
		Evidence:    evidenceCopy,
		Companions:  companions,
		Provenance: Provenance{
			Source: strings.TrimSpace(options.Source), License: strings.TrimSpace(options.License),
			Authors: normalizeStrings(options.Authors), CacheEntries: cacheReferences,
		},
	}
	if err := validatePayload(payload, publishedAt, true); err != nil {
		return Payload{}, nil, err
	}
	profileJSON, _ := json.Marshal(payload.Profile)
	reviewJSON, _ := json.Marshal(payload.Review)
	evidenceJSON, _ := json.Marshal(payload.Evidence)
	supporting := []eartifact.Descriptor{
		descriptorFor(profileJSON, ProfileMediaType, map[string]string{"browsertools.part": "profile"}),
		descriptorFor(reviewJSON, ReviewMediaType, map[string]string{"browsertools.part": "review"}),
		descriptorFor(evidenceJSON, EvidenceMediaType, map[string]string{"browsertools.part": "evidence"}),
	}
	supporting = append(supporting, companionDescriptors...)
	supporting = append(supporting, cacheDescriptors...)
	return payload, uniqueDescriptors(supporting), nil
}

// Verify proves exact content bindings and re-runs time-sensitive promotion
// gates at at. Callers must supply a clock so expiry behavior is reproducible.
func Verify(value *Bundle, at time.Time) error {
	if value == nil {
		return fmt.Errorf("bundle is required")
	}
	if at.IsZero() {
		return fmt.Errorf("bundle verification time is required")
	}
	if value.Version != Version || value.Payload.Version != Version {
		return fmt.Errorf("bundle version must be %q", Version)
	}
	if err := validatePayload(value.Payload, at.UTC().Round(0), false); err != nil {
		return err
	}
	payloadJSON, err := canonicalPayloadJSON(value.Payload)
	if err != nil {
		return err
	}
	if int64(len(payloadJSON)) > MaxBundleBytes {
		return fmt.Errorf("bundle payload exceeds %d bytes", MaxBundleBytes)
	}
	expected := descriptorFor(payloadJSON, PayloadMediaType, map[string]string{
		"browsertools.id": value.Payload.Identity.ID, "browsertools.release": value.Payload.Identity.Release,
	})
	if !descriptorEqual(value.Descriptor, expected) {
		return fmt.Errorf("bundle payload descriptor mismatch")
	}
	if err := eartifact.ValidateAssessment(value.Assessment); err != nil {
		return fmt.Errorf("bundle assessment: %w", err)
	}
	if !descriptorEqual(value.Assessment.Subject, expected) {
		return fmt.Errorf("bundle assessment subject mismatch")
	}
	if !value.Assessment.AssessedAt.Equal(value.Payload.PublishedAt) {
		return fmt.Errorf("bundle assessment time does not match published_at")
	}
	if !value.Assessment.ExpiresAt.Equal(profileExpiry(&value.Payload.Profile)) {
		return fmt.Errorf("bundle assessment expiry does not match profile expiry")
	}
	if eartifact.EffectiveStatus(value.Assessment, at) != eartifact.LifecycleActive {
		return fmt.Errorf("bundle lifecycle is %q", eartifact.EffectiveStatus(value.Assessment, at))
	}
	expectedSupporting, err := payloadDescriptors(value.Payload)
	if err != nil {
		return err
	}
	if !descriptorSlicesEqual(value.Assessment.Supporting, expectedSupporting) {
		return fmt.Errorf("bundle supporting descriptors mismatch")
	}
	_, err = marshalCanonicalBundle(value)
	return err
}

// CanonicalJSON returns deterministic verified wire bytes.
func CanonicalJSON(value *Bundle, at time.Time) ([]byte, error) {
	if err := Verify(value, at); err != nil {
		return nil, err
	}
	return marshalCanonicalBundle(value)
}

// Digest returns the SHA-256 identity of the complete canonical bundle.
func Digest(value *Bundle, at time.Time) (digest.Record, error) {
	data, err := CanonicalJSON(value, at)
	if err != nil {
		return digest.Record{}, err
	}
	return digest.SHA256Bytes(data), nil
}

// Parse decodes one bounded strict JSON bundle. Verification is separate so
// callers can choose an explicit assessment time.
func Parse(data []byte) (*Bundle, error) {
	if int64(len(data)) > MaxBundleBytes {
		return nil, fmt.Errorf("bundle exceeds %d bytes", MaxBundleBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Bundle
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode bundle: %w", err)
	}
	return &value, nil
}

func validatePayload(value Payload, at time.Time, building bool) error {
	if value.Version != Version {
		return fmt.Errorf("bundle payload version must be %q", Version)
	}
	if !idPattern.MatchString(value.Identity.ID) || strings.Contains(value.Identity.ID, "..") || strings.Contains(value.Identity.ID, "//") {
		return fmt.Errorf("bundle identity id %q is invalid", value.Identity.ID)
	}
	if !releasePattern.MatchString(value.Identity.Release) {
		return fmt.Errorf("bundle release %q is not semantic version X.Y.Z", value.Identity.Release)
	}
	if value.Identity.Title == "" || value.Identity.Title != strings.TrimSpace(value.Profile.Info.Title) {
		return fmt.Errorf("bundle title must match profile info.title")
	}
	if !slices.Equal(value.Identity.Origins, []string(value.Profile.Info.Origin)) {
		return fmt.Errorf("bundle origins must match profile info.origin")
	}
	if value.PublishedAt.IsZero() || !value.PublishedAt.Equal(value.PublishedAt.UTC().Round(0)) {
		return fmt.Errorf("bundle published_at must be a non-zero normalized UTC time")
	}
	if value.PublishedAt.After(at) {
		return fmt.Errorf("bundle published_at is after verification time")
	}
	if value.Provenance.Source == "" || value.Provenance.Source != strings.TrimSpace(value.Provenance.Source) {
		return fmt.Errorf("bundle provenance source is required and normalized")
	}
	if !licensePattern.MatchString(value.Provenance.License) {
		return fmt.Errorf("bundle provenance license %q is invalid", value.Provenance.License)
	}
	if !slices.Equal(value.Provenance.Authors, normalizeStrings(value.Provenance.Authors)) {
		return fmt.Errorf("bundle authors are not canonical")
	}
	profileJSON, err := json.Marshal(value.Profile)
	if err != nil {
		return err
	}
	if err := versions.ValidateBrowserSourceProfile(profileJSON); err != nil {
		return fmt.Errorf("bundle UWS profile validation: %w", err)
	}
	profileValue, err := value.Profile.Value()
	if err != nil {
		return err
	}
	if err := profile.Validate(profileValue); err != nil {
		return err
	}
	if err := validateSideEffects(value.Profile); err != nil {
		return err
	}
	if profileExpiry(&value.Profile).IsZero() {
		return fmt.Errorf("bundle profile expiry cannot be derived")
	}
	if err := review.Verify(&value.Review, &value.Profile, value.Evidence, at); err != nil {
		return fmt.Errorf("bundle review verification: %w", err)
	}
	reviewedAt, err := time.Parse(time.RFC3339, value.Review.AssessedAt)
	if err != nil || reviewedAt.After(value.PublishedAt) {
		return fmt.Errorf("bundle review assessment must be valid and no later than published_at")
	}
	canonicalRecords, err := canonicalEvidence(value.Evidence)
	if err != nil {
		return err
	}
	if !recordsEqual(value.Evidence, canonicalRecords) {
		return fmt.Errorf("bundle evidence is not in canonical order")
	}
	companions, _, err := normalizeCompanions(value.Companions)
	if err != nil {
		return err
	}
	if !companionsEqual(value.Companions, companions) {
		return fmt.Errorf("bundle companions are not canonical")
	}
	if err := validateCacheReferences(value.Provenance.CacheEntries, at); err != nil {
		return err
	}
	if containsSecretLikeString(value) {
		return fmt.Errorf("bundle contains a secret-like value")
	}
	if !building {
		payloadBytes, err := json.Marshal(value)
		if err != nil {
			return err
		}
		canonicalBytes, err := canonicalPayloadJSON(value)
		if err != nil {
			return err
		}
		if !bytes.Equal(payloadBytes, canonicalBytes) {
			return fmt.Errorf("bundle payload is not canonical")
		}
	}
	return nil
}

func payloadDescriptors(value Payload) ([]eartifact.Descriptor, error) {
	profileJSON, err := json.Marshal(value.Profile)
	if err != nil {
		return nil, err
	}
	reviewJSON, err := json.Marshal(value.Review)
	if err != nil {
		return nil, err
	}
	evidenceJSON, err := json.Marshal(value.Evidence)
	if err != nil {
		return nil, err
	}
	result := []eartifact.Descriptor{
		descriptorFor(profileJSON, ProfileMediaType, map[string]string{"browsertools.part": "profile"}),
		descriptorFor(reviewJSON, ReviewMediaType, map[string]string{"browsertools.part": "review"}),
		descriptorFor(evidenceJSON, EvidenceMediaType, map[string]string{"browsertools.part": "evidence"}),
	}
	for _, companion := range value.Companions {
		result = append(result, companion.Descriptor)
	}
	for _, reference := range value.Provenance.CacheEntries {
		record, err := parseDigest(reference.Digest)
		if err != nil {
			return nil, err
		}
		result = append(result, eartifact.Descriptor{
			Version: eartifact.DescriptorVersion, MediaType: reference.MediaType,
			SizeBytes: reference.SizeBytes, Digest: record,
			Annotations: map[string]string{"browsertools.cache_kind": string(reference.Kind)},
		})
	}
	return uniqueDescriptors(result), nil
}

func canonicalPayloadJSON(value Payload) ([]byte, error) {
	return json.Marshal(value)
}

func marshalCanonicalBundle(value *Bundle) ([]byte, error) {
	normalized := *value
	normalized.Descriptor = eartifact.NormalizeDescriptor(normalized.Descriptor)
	normalized.Assessment = eartifact.NormalizeAssessment(normalized.Assessment)
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxBundleBytes {
		return nil, fmt.Errorf("bundle exceeds %d bytes", MaxBundleBytes)
	}
	return data, nil
}

func descriptorFor(data []byte, mediaType string, annotations map[string]string) eartifact.Descriptor {
	return eartifact.NormalizeDescriptor(eartifact.Descriptor{
		Version: eartifact.DescriptorVersion, MediaType: mediaType,
		SizeBytes: int64(len(data)), Digest: digest.SHA256Bytes(data), Annotations: annotations,
	})
}

func normalizeCompanions(values []Companion) ([]Companion, []eartifact.Descriptor, error) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	result := make([]Companion, len(values))
	descriptors := make([]eartifact.Descriptor, 0, len(values))
	seenPaths, seenDigests := map[string]struct{}{}, map[string]struct{}{}
	var total int64
	for index, value := range values {
		value.Path = strings.TrimSpace(value.Path)
		value.MediaType = normalizeMediaType(value.MediaType)
		value.Content = append([]byte(nil), value.Content...)
		if err := validateCompanion(value); err != nil {
			return nil, nil, fmt.Errorf("bundle companion[%d]: %w", index, err)
		}
		if _, ok := seenPaths[value.Path]; ok {
			return nil, nil, fmt.Errorf("bundle companion path %q is duplicated", value.Path)
		}
		seenPaths[value.Path] = struct{}{}
		total += int64(len(value.Content))
		if total > MaxBundleBytes {
			return nil, nil, fmt.Errorf("bundle companions exceed %d bytes", MaxBundleBytes)
		}
		expected := descriptorFor(value.Content, value.MediaType, map[string]string{"browsertools.path": value.Path})
		if !isZeroDescriptor(value.Descriptor) && !descriptorEqual(value.Descriptor, expected) {
			return nil, nil, fmt.Errorf("bundle companion %q descriptor mismatch", value.Path)
		}
		value.Descriptor = expected
		key := expected.Digest.String()
		if _, ok := seenDigests[key]; ok {
			return nil, nil, fmt.Errorf("bundle companion content %s is duplicated", key)
		}
		seenDigests[key] = struct{}{}
		result[index] = value
		descriptors = append(descriptors, expected)
	}
	slices.SortFunc(result, func(a, b Companion) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(descriptors, compareDescriptors)
	return result, descriptors, nil
}

func validateCompanion(value Companion) error {
	clean := path.Clean(value.Path)
	if value.Path == "" || clean != value.Path || clean == "." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, "\\") {
		return fmt.Errorf("path %q must be a safe normalized relative path", value.Path)
	}
	switch {
	case strings.HasSuffix(strings.ToLower(value.Path), ".uws.json") && value.MediaType == UWSJSONMediaType:
	case (strings.HasSuffix(strings.ToLower(value.Path), ".uws.yaml") || strings.HasSuffix(strings.ToLower(value.Path), ".uws.yml")) && value.MediaType == UWSYAMLMediaType:
	default:
		return fmt.Errorf("%q is not a supported UWS JSON/YAML companion", value.Path)
	}
	if len(value.Content) == 0 {
		return fmt.Errorf("content is empty")
	}
	documentJSON, err := decodeJSONOrYAML(value.Content)
	if err != nil {
		return err
	}
	if containsSecretDocument(documentJSON) {
		return fmt.Errorf("content contains a secret-like value or session material")
	}
	var document uws1.Document
	if err := json.Unmarshal(documentJSON, &document); err != nil {
		return fmt.Errorf("decode UWS document: %w", err)
	}
	if err := document.Validate(); err != nil {
		return fmt.Errorf("validate UWS document: %w", err)
	}
	return nil
}

func decodeJSONOrYAML(data []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode document: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple documents are not supported")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func normalizeCacheEntries(values []CachedArtifact, at time.Time) ([]CacheReference, []eartifact.Descriptor, error) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	result := make([]CacheReference, 0, len(values))
	descriptors := make([]eartifact.Descriptor, 0, len(values))
	seen := map[string]struct{}{}
	for index, value := range values {
		if err := cache.ValidateForPublication(value.Entry); err != nil {
			return nil, nil, fmt.Errorf("bundle cache entry[%d]: %w", index, err)
		}
		if int64(len(value.Content)) != value.Entry.SizeBytes || digest.SHA256String(value.Content) != value.Entry.Digest {
			return nil, nil, fmt.Errorf("bundle cache entry[%d] content mismatch", index)
		}
		if value.Entry.ExpiresAt != "" {
			expires, _ := time.Parse(time.RFC3339Nano, value.Entry.ExpiresAt)
			if !at.Before(expires) {
				return nil, nil, fmt.Errorf("bundle cache entry %s is expired", value.Entry.ID)
			}
		}
		if _, ok := seen[value.Entry.ID]; ok {
			return nil, nil, fmt.Errorf("bundle cache entry %s is duplicated", value.Entry.ID)
		}
		seen[value.Entry.ID] = struct{}{}
		reference := CacheReference{
			ID: value.Entry.ID, Kind: value.Entry.Kind, MediaType: normalizeMediaType(value.Entry.MediaType),
			SizeBytes: value.Entry.SizeBytes, Digest: value.Entry.Digest, Source: strings.TrimSpace(value.Entry.Source),
			Annotations: cloneStringMap(value.Entry.Annotations),
		}
		result = append(result, reference)
		record, _ := parseDigest(reference.Digest)
		descriptors = append(descriptors, eartifact.Descriptor{
			Version: eartifact.DescriptorVersion, MediaType: reference.MediaType,
			SizeBytes: reference.SizeBytes, Digest: record,
			Annotations: map[string]string{"browsertools.cache_kind": string(reference.Kind)},
		})
	}
	slices.SortFunc(result, func(a, b CacheReference) int { return strings.Compare(a.ID, b.ID) })
	return result, uniqueDescriptors(descriptors), nil
}

func validateCacheReferences(values []CacheReference, at time.Time) error {
	if !slices.IsSortedFunc(values, func(a, b CacheReference) int { return strings.Compare(a.ID, b.ID) }) {
		return fmt.Errorf("bundle cache references are not canonical")
	}
	seen := map[string]struct{}{}
	for index, value := range values {
		if _, ok := seen[value.ID]; ok {
			return fmt.Errorf("bundle cache reference %s is duplicated", value.ID)
		}
		seen[value.ID] = struct{}{}
		entry := cache.Entry{
			Version: cache.ManifestVersion, ID: value.ID, Kind: value.Kind, MediaType: value.MediaType,
			SizeBytes: value.SizeBytes, Digest: value.Digest, CreatedAt: "1970-01-01T00:00:00Z",
			Source: value.Source, Annotations: value.Annotations, PublicationEligible: true,
		}
		if err := cache.ValidateForPublication(entry); err != nil {
			return fmt.Errorf("bundle cache reference[%d]: %w", index, err)
		}
		if containsSecretLikeString(value) {
			return fmt.Errorf("bundle cache reference[%d] contains a secret-like value", index)
		}
	}
	_ = at
	return nil
}

func validateSideEffects(value profile.Profile) error {
	for _, name := range value.SortedActionNames() {
		action := value.Actions[name]
		write := false
		for _, effect := range action.SideEffects {
			if effect != profile.SideEffectReadOnly {
				write = true
			}
		}
		if write && (!action.ConfirmationPolicy.Required || strings.TrimSpace(action.ConfirmationPolicy.Prompt) == "") {
			return fmt.Errorf("bundle action %q has side effects without explicit confirmation", name)
		}
	}
	return nil
}

func profileExpiry(value *profile.Profile) time.Time {
	if value == nil {
		return time.Time{}
	}
	verified, err := time.Parse(time.RFC3339, value.Verification.LastVerifiedAt)
	if err != nil {
		return time.Time{}
	}
	expires, err := value.ExpiresAfter.AddTo(verified)
	if err != nil {
		return time.Time{}
	}
	return expires.UTC().Round(0)
}

func cloneProfile(value *profile.Profile) (*profile.Profile, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result profile.Profile
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func cloneReview(value *review.Bundle) (*review.Bundle, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result review.Bundle
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func canonicalEvidence(values []bevidence.Record) ([]bevidence.Record, error) {
	if len(values) == 0 {
		return []bevidence.Record{}, nil
	}
	result := make([]bevidence.Record, len(values))
	for index, value := range values {
		data, err := bevidence.MarshalDeterministic(value)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &result[index]); err != nil {
			return nil, err
		}
	}
	bevidence.Sort(result)
	return result, nil
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func normalizeMediaType(value string) string {
	value = strings.TrimSpace(value)
	parsed, params, err := mime.ParseMediaType(value)
	if err != nil {
		return value
	}
	return mime.FormatMediaType(strings.ToLower(parsed), params)
}

func parseDigest(value string) (digest.Record, error) {
	algorithm, encoded, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || algorithm != digest.AlgorithmSHA256 || len(encoded) != 64 || encoded != strings.ToLower(encoded) {
		return digest.Record{}, fmt.Errorf("digest %q is not a canonical SHA-256 digest", value)
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return digest.Record{}, fmt.Errorf("digest %q is invalid", value)
	}
	return digest.Record{Algorithm: algorithm, Value: encoded}, nil
}

func uniqueDescriptors(values []eartifact.Descriptor) []eartifact.Descriptor {
	seen := map[string]struct{}{}
	result := make([]eartifact.Descriptor, 0, len(values))
	for _, value := range values {
		value = eartifact.NormalizeDescriptor(value)
		key := value.Digest.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	slices.SortFunc(result, compareDescriptors)
	return result
}

func compareDescriptors(a, b eartifact.Descriptor) int {
	if result := strings.Compare(a.Digest.String(), b.Digest.String()); result != 0 {
		return result
	}
	if result := strings.Compare(a.MediaType, b.MediaType); result != 0 {
		return result
	}
	if a.SizeBytes < b.SizeBytes {
		return -1
	}
	if a.SizeBytes > b.SizeBytes {
		return 1
	}
	return 0
}

func descriptorEqual(a, b eartifact.Descriptor) bool {
	aJSON, errA := eartifact.CanonicalDescriptorJSON(a)
	bJSON, errB := eartifact.CanonicalDescriptorJSON(b)
	return errA == nil && errB == nil && bytes.Equal(aJSON, bJSON)
}

func descriptorSlicesEqual(a, b []eartifact.Descriptor) bool {
	a = uniqueDescriptors(a)
	b = uniqueDescriptors(b)
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if !descriptorEqual(a[index], b[index]) {
			return false
		}
	}
	return true
}

func isZeroDescriptor(value eartifact.Descriptor) bool {
	return value.Version == "" && value.MediaType == "" && value.SizeBytes == 0 && value.Digest.IsZero() && len(value.Annotations) == 0
}

func recordsEqual(a, b []bevidence.Record) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}
func companionsEqual(a, b []Companion) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}

func containsSecretLikeString(value any) bool {
	data, err := json.Marshal(value)
	if err != nil {
		return true
	}
	var document any
	if json.Unmarshal(data, &document) != nil {
		return true
	}
	return walkStrings(document, false)
}

func containsSecretDocument(data []byte) bool {
	var document any
	if json.Unmarshal(data, &document) != nil {
		return true
	}
	return walkStrings(document, true)
}

func walkStrings(value any, checkKeys bool) bool {
	switch typed := value.(type) {
	case string:
		return redact.String(typed) != typed
	case []any:
		for _, item := range typed {
			if walkStrings(item, checkKeys) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			if checkKeys && sensitivePublicationKey(key) {
				if text, ok := item.(string); ok && !isReferenceValue(text) {
					return true
				}
			}
			if walkStrings(item, checkKeys) {
				return true
			}
		}
	}
	return false
}

func sensitivePublicationKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), ".", "_"))
	switch normalized {
	case "cookie", "cookies", "session_storage", "local_storage", "oauth_state", "browser_session", "browser_profile":
		return true
	default:
		return redact.SensitiveKey(normalized)
	}
}

func isReferenceValue(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "${") || strings.Contains(value, "{{") || value == redact.Value || value == "[REDACTED]"
}

func cloneStringMap(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}
