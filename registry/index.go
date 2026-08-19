// Package registry implements a service-free static catalog of reviewed
// browser capability bundles.
package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/bundle"
	eartifact "github.com/OpenUdon/evidence/artifact"
	"github.com/OpenUdon/evidence/digest"
)

const (
	IndexVersion  = "browsertools.registry-index.v1"
	IndexName     = "index.json"
	BlobMediaType = bundle.MediaType
)

// Coordinate selects one immutable capability release.
type Coordinate struct {
	ID      string `json:"id"`
	Release string `json:"release"`
}

// Entry is enough metadata to search an index without fetching its bundle.
// Lifecycle.Subject identifies the exact immutable bundle bytes.
type Entry struct {
	ID          string               `json:"id"`
	Release     string               `json:"release"`
	Title       string               `json:"title"`
	Origins     []string             `json:"origins"`
	Actions     []string             `json:"actions"`
	ActionCount int                  `json:"action_count"`
	PublishedAt time.Time            `json:"published_at"`
	Provenance  bundle.Provenance    `json:"provenance"`
	Bundle      eartifact.Descriptor `json:"bundle"`
	Lifecycle   eartifact.Assessment `json:"lifecycle"`
}

// Index is the deterministic registry root served as index.json.
type Index struct {
	Version string  `json:"version"`
	Entries []Entry `json:"entries"`
}

// EmptyIndex returns a valid independent empty index.
func EmptyIndex() Index {
	return Index{Version: IndexVersion, Entries: []Entry{}}
}

// Normalize returns a canonical deep copy without mutating input.
func Normalize(value Index) Index {
	value.Version = strings.TrimSpace(value.Version)
	value.Entries = append([]Entry(nil), value.Entries...)
	for index := range value.Entries {
		entry := &value.Entries[index]
		entry.ID = strings.TrimSpace(entry.ID)
		entry.Release = strings.TrimSpace(entry.Release)
		entry.Title = strings.TrimSpace(entry.Title)
		entry.Origins = normalizeStrings(entry.Origins)
		entry.Actions = normalizeStrings(entry.Actions)
		entry.PublishedAt = normalizeTime(entry.PublishedAt)
		entry.Provenance.Source = strings.TrimSpace(entry.Provenance.Source)
		entry.Provenance.License = strings.TrimSpace(entry.Provenance.License)
		entry.Provenance.Authors = normalizeStrings(entry.Provenance.Authors)
		entry.Provenance.CacheEntries = append([]bundle.CacheReference(nil), entry.Provenance.CacheEntries...)
		slices.SortFunc(entry.Provenance.CacheEntries, func(a, b bundle.CacheReference) int {
			return strings.Compare(a.ID, b.ID)
		})
		entry.Bundle = eartifact.NormalizeDescriptor(entry.Bundle)
		entry.Lifecycle = eartifact.NormalizeAssessment(entry.Lifecycle)
	}
	slices.SortFunc(value.Entries, compareEntries)
	if value.Entries == nil {
		value.Entries = []Entry{}
	}
	return value
}

// Validate rejects malformed, duplicate, non-canonical, or dangling lifecycle
// records. Non-active entries remain valid index history.
func Validate(value Index) error {
	if err := validateIndex(value); err != nil {
		return fmt.Errorf("%w: %w", ErrValidation, err)
	}
	return nil
}

func validateIndex(value Index) error {
	if value.Version != IndexVersion {
		return fmt.Errorf("registry index version must be %q", IndexVersion)
	}
	normalized := Normalize(value)
	original, err := json.Marshal(value)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if !bytes.Equal(original, canonical) {
		return fmt.Errorf("registry index is not canonical")
	}
	coordinates := make(map[string]int, len(value.Entries))
	digests := make(map[string]int, len(value.Entries))
	for index, entry := range value.Entries {
		if err := validateEntry(entry); err != nil {
			return fmt.Errorf("registry entry[%d]: %w", index, err)
		}
		coordinate := entry.ID + "@" + entry.Release
		if prior, ok := coordinates[coordinate]; ok {
			return fmt.Errorf("registry entries %d and %d duplicate coordinate %s", prior, index, coordinate)
		}
		coordinates[coordinate] = index
		digests[entry.Bundle.Digest.String()] = index
	}
	for index, entry := range value.Entries {
		if entry.Lifecycle.Status != eartifact.LifecycleSuperseded {
			continue
		}
		successor := entry.Lifecycle.Successor.String()
		successorIndex, ok := digests[successor]
		if !ok {
			return fmt.Errorf("registry entry[%d] successor %s is absent from the index", index, successor)
		}
		if value.Entries[successorIndex].ID != entry.ID {
			return fmt.Errorf("registry entry[%d] successor changes capability id", index)
		}
	}
	return nil
}

// CanonicalJSON returns deterministic validated index bytes.
func CanonicalJSON(value Index) ([]byte, error) {
	value = Normalize(value)
	if len(value.Entries) > DefaultMaxEntries {
		return nil, fmt.Errorf("%w: registry index exceeds %d entries", ErrLimit, DefaultMaxEntries)
	}
	if err := Validate(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > bundle.MaxBundleBytes {
		return nil, fmt.Errorf("%w: registry index exceeds %d bytes", ErrLimit, bundle.MaxBundleBytes)
	}
	return data, nil
}

// ParseIndex decodes one strict bounded index document.
func ParseIndex(data []byte) (Index, error) {
	if int64(len(data)) > bundle.MaxBundleBytes {
		return Index{}, fmt.Errorf("%w: registry index exceeds %d bytes", ErrLimit, bundle.MaxBundleBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Index
	if err := decoder.Decode(&value); err != nil {
		return Index{}, fmt.Errorf("%w: decode registry index: %w", ErrValidation, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Index{}, fmt.Errorf("%w: decode registry index: %w", ErrValidation, err)
	}
	if len(value.Entries) > DefaultMaxEntries {
		return Index{}, fmt.Errorf("%w: registry index exceeds %d entries", ErrLimit, DefaultMaxEntries)
	}
	if err := Validate(value); err != nil {
		return Index{}, err
	}
	return value, nil
}

// BlobPath returns the portable slash-separated blob path for digestRecord.
func BlobPath(digestRecord digest.Record) (string, error) {
	descriptor := eartifact.Descriptor{
		Version: eartifact.DescriptorVersion, MediaType: BlobMediaType,
		SizeBytes: 0, Digest: digestRecord,
	}
	if err := eartifact.ValidateDescriptor(descriptor); err != nil {
		return "", fmt.Errorf("registry blob digest: %w", err)
	}
	return "blobs/sha256/" + digestRecord.Value, nil
}

func validateEntry(entry Entry) error {
	if entry.ID == "" || entry.Release == "" || entry.Title == "" {
		return fmt.Errorf("id, release, and title are required")
	}
	if len(entry.Origins) == 0 {
		return fmt.Errorf("origins are required")
	}
	if len(entry.Actions) == 0 || entry.ActionCount != len(entry.Actions) {
		return fmt.Errorf("action_count must equal the non-empty actions list")
	}
	if entry.PublishedAt.IsZero() || !entry.PublishedAt.Equal(normalizeTime(entry.PublishedAt)) {
		return fmt.Errorf("published_at must be a normalized UTC time")
	}
	if entry.Provenance.Source == "" || entry.Provenance.License == "" {
		return fmt.Errorf("publication source and license are required")
	}
	if err := eartifact.ValidateDescriptor(entry.Bundle); err != nil {
		return fmt.Errorf("bundle descriptor: %w", err)
	}
	if entry.Bundle.MediaType != BlobMediaType {
		return fmt.Errorf("bundle media type must be %q", BlobMediaType)
	}
	if err := eartifact.ValidateAssessment(entry.Lifecycle); err != nil {
		return fmt.Errorf("lifecycle: %w", err)
	}
	equal, err := descriptorsEqual(entry.Bundle, entry.Lifecycle.Subject)
	if err != nil {
		return fmt.Errorf("compare lifecycle subject: %w", err)
	}
	if !equal {
		return fmt.Errorf("lifecycle subject does not match bundle descriptor")
	}
	if entry.Lifecycle.AssessedAt.Before(entry.PublishedAt) {
		return fmt.Errorf("lifecycle assessment predates publication")
	}
	return nil
}

func compareEntries(a, b Entry) int {
	if result := strings.Compare(a.ID, b.ID); result != 0 {
		return result
	}
	if result := strings.Compare(a.Release, b.Release); result != 0 {
		return result
	}
	return strings.Compare(a.Bundle.Digest.String(), b.Bundle.Digest.String())
}

func descriptorsEqual(a, b eartifact.Descriptor) (bool, error) {
	left, leftErr := eartifact.CanonicalDescriptorJSON(a)
	if leftErr != nil {
		return false, leftErr
	}
	right, rightErr := eartifact.CanonicalDescriptorJSON(b)
	if rightErr != nil {
		return false, rightErr
	}
	return bytes.Equal(left, right), nil
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
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

func normalizeTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC().Round(0)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}
