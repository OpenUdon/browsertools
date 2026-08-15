package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/OpenUdon/browsertools/bundle"
	eartifact "github.com/OpenUdon/evidence/artifact"
	"github.com/OpenUdon/evidence/digest"
)

// PublishOptions describes one offline local registry transaction.
type PublishOptions struct {
	Root       string
	Bundle     *bundle.Bundle
	At         time.Time
	Supersedes *Coordinate
}

// PublishReport describes the files selected by a successful transaction.
type PublishReport struct {
	Entry       Entry  `json:"entry"`
	IndexPath   string `json:"index_path"`
	BlobPath    string `json:"blob_path"`
	ReusedBlob  bool   `json:"reused_blob"`
	ReusedEntry bool   `json:"reused_entry"`
}

// Verification records one locally verified registry entry.
type Verification struct {
	Coordinate Coordinate                `json:"coordinate"`
	Digest     string                    `json:"digest"`
	Status     eartifact.LifecycleStatus `json:"status"`
	BlobPath   string                    `json:"blob_path"`
}

// VerifyReport is the deterministic local registry verification result.
type VerifyReport struct {
	IndexPath string         `json:"index_path"`
	Entries   []Verification `json:"entries"`
}

// PublishLocal verifies a capability bundle and atomically adds its immutable
// blob and index entry. It performs no network I/O.
func PublishLocal(ctx context.Context, options PublishOptions) (PublishReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return PublishReport{}, err
	}
	root, err := prepareRoot(options.Root)
	if err != nil {
		return PublishReport{}, err
	}
	unlock, err := acquirePublishLock(root)
	if err != nil {
		return PublishReport{}, err
	}
	defer unlock()
	if options.Bundle == nil || options.At.IsZero() {
		return PublishReport{}, fmt.Errorf("registry bundle and publication time are required")
	}
	at := normalizeTime(options.At)
	wire, err := bundle.CanonicalJSON(options.Bundle, at)
	if err != nil {
		return PublishReport{}, fmt.Errorf("registry verify bundle: %w", err)
	}
	entry, err := entryFromBundle(options.Bundle, wire, at)
	if err != nil {
		return PublishReport{}, err
	}
	indexPath := filepath.Join(root, IndexName)
	index, err := loadLocalIndex(ctx, indexPath)
	if err != nil {
		return PublishReport{}, err
	}
	existingIndex := findCoordinate(index.Entries, Coordinate{ID: entry.ID, Release: entry.Release})
	if existingIndex >= 0 {
		existing := index.Entries[existingIndex]
		if existing.Bundle.Digest != entry.Bundle.Digest {
			return PublishReport{}, fmt.Errorf("registry coordinate %s@%s already identifies different content", entry.ID, entry.Release)
		}
		blobPath, err := localBlobPath(root, entry.Bundle.Digest)
		if err != nil {
			return PublishReport{}, err
		}
		if err := verifyBlobFile(ctx, blobPath, existing.Bundle); err != nil {
			return PublishReport{}, err
		}
		return PublishReport{Entry: existing, IndexPath: indexPath, BlobPath: blobPath, ReusedBlob: true, ReusedEntry: true}, nil
	}
	if options.Supersedes != nil {
		if options.Supersedes.ID == entry.ID && options.Supersedes.Release == entry.Release {
			return PublishReport{}, fmt.Errorf("registry release cannot supersede itself")
		}
		oldIndex := findCoordinate(index.Entries, *options.Supersedes)
		if oldIndex < 0 {
			return PublishReport{}, fmt.Errorf("registry superseded coordinate %s@%s was not found", options.Supersedes.ID, options.Supersedes.Release)
		}
		old := index.Entries[oldIndex]
		if old.ID != entry.ID {
			return PublishReport{}, fmt.Errorf("registry successor must keep capability id %q", old.ID)
		}
		switch old.Lifecycle.Status {
		case eartifact.LifecycleActive, eartifact.LifecycleStale:
		default:
			return PublishReport{}, fmt.Errorf("registry %s@%s lifecycle %q cannot transition to superseded", old.ID, old.Release, old.Lifecycle.Status)
		}
		if at.Before(old.Lifecycle.AssessedAt) {
			return PublishReport{}, fmt.Errorf("registry supersession time predates the existing lifecycle assessment")
		}
		old.Lifecycle = eartifact.Assessment{
			Version: eartifact.AssessmentVersion, Subject: old.Bundle,
			Status: eartifact.LifecycleSuperseded, AssessedAt: at, Successor: entry.Bundle.Digest,
			Supporting: append([]eartifact.Descriptor(nil), old.Lifecycle.Supporting...),
		}
		if err := eartifact.ValidateAssessment(old.Lifecycle); err != nil {
			return PublishReport{}, fmt.Errorf("registry supersession: %w", err)
		}
		index.Entries[oldIndex] = old
	}
	index.Entries = append(index.Entries, entry)
	index = Normalize(index)
	indexWire, err := CanonicalJSON(index)
	if err != nil {
		return PublishReport{}, err
	}
	if int64(len(indexWire)+1) > bundle.MaxBundleBytes {
		return PublishReport{}, fmt.Errorf("registry index plus trailing newline exceeds %d bytes", bundle.MaxBundleBytes)
	}
	blobPath, err := localBlobPath(root, entry.Bundle.Digest)
	if err != nil {
		return PublishReport{}, err
	}
	reusedBlob, createdBlob, err := installBlob(ctx, blobPath, wire, entry.Bundle)
	if err != nil {
		return PublishReport{}, err
	}
	committed := false
	defer func() {
		if createdBlob && !committed {
			_ = os.Remove(blobPath)
		}
	}()
	if err := writeAtomic(indexPath, append(indexWire, '\n'), 0o644); err != nil {
		return PublishReport{}, fmt.Errorf("write registry index: %w", err)
	}
	committed = true
	return PublishReport{Entry: entry, IndexPath: indexPath, BlobPath: blobPath, ReusedBlob: reusedBlob}, nil
}

// UpdateLifecycleLocal marks an active/stale release stale or revoked. Bundle
// bytes remain immutable. Supersession is handled by PublishLocal so the
// successor blob and both index changes share one transaction.
func UpdateLifecycleLocal(ctx context.Context, root string, coordinate Coordinate, status eartifact.LifecycleStatus, at time.Time) (Entry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if status != eartifact.LifecycleStale && status != eartifact.LifecycleRevoked {
		return Entry{}, fmt.Errorf("registry lifecycle update supports only stale or revoked")
	}
	if at.IsZero() {
		return Entry{}, fmt.Errorf("registry lifecycle assessment time is required")
	}
	prepared, err := prepareRoot(root)
	if err != nil {
		return Entry{}, err
	}
	unlock, err := acquirePublishLock(prepared)
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	indexPath := filepath.Join(prepared, IndexName)
	index, err := loadLocalIndex(ctx, indexPath)
	if err != nil {
		return Entry{}, err
	}
	position := findCoordinate(index.Entries, coordinate)
	if position < 0 {
		return Entry{}, fmt.Errorf("registry coordinate %s@%s was not found", coordinate.ID, coordinate.Release)
	}
	entry := index.Entries[position]
	switch entry.Lifecycle.Status {
	case eartifact.LifecycleActive:
	case eartifact.LifecycleStale:
		if status == eartifact.LifecycleStale {
			return entry, nil
		}
	default:
		return Entry{}, fmt.Errorf("registry lifecycle %q is terminal", entry.Lifecycle.Status)
	}
	if normalizeTime(at).Before(entry.Lifecycle.AssessedAt) {
		return Entry{}, fmt.Errorf("registry lifecycle time predates the existing assessment")
	}
	assessment := eartifact.Assessment{
		Version: eartifact.AssessmentVersion, Subject: entry.Bundle, Status: status,
		AssessedAt: normalizeTime(at), Supporting: append([]eartifact.Descriptor(nil), entry.Lifecycle.Supporting...),
	}
	if err := eartifact.ValidateAssessment(assessment); err != nil {
		return Entry{}, err
	}
	entry.Lifecycle = assessment
	index.Entries[position] = entry
	index = Normalize(index)
	wire, err := CanonicalJSON(index)
	if err != nil {
		return Entry{}, err
	}
	if int64(len(wire)+1) > bundle.MaxBundleBytes {
		return Entry{}, fmt.Errorf("registry index plus trailing newline exceeds %d bytes", bundle.MaxBundleBytes)
	}
	if err := writeAtomic(indexPath, append(wire, '\n'), 0o644); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// VerifyLocal validates the index, every referenced blob, exact bundle/index
// metadata, and lifecycle state without contacting a network.
func VerifyLocal(ctx context.Context, root string, at time.Time) (VerifyReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if at.IsZero() {
		return VerifyReport{}, fmt.Errorf("registry verification time is required")
	}
	prepared, err := validateRoot(root)
	if err != nil {
		return VerifyReport{}, err
	}
	indexPath := filepath.Join(prepared, IndexName)
	index, err := loadLocalIndex(ctx, indexPath)
	if err != nil {
		return VerifyReport{}, err
	}
	report := VerifyReport{IndexPath: indexPath, Entries: make([]Verification, 0, len(index.Entries))}
	for _, entry := range index.Entries {
		if err := ctx.Err(); err != nil {
			return VerifyReport{}, err
		}
		blobPath, err := localBlobPath(prepared, entry.Bundle.Digest)
		if err != nil {
			return VerifyReport{}, err
		}
		data, err := readRegularBounded(ctx, blobPath, bundle.MaxBundleBytes)
		if err != nil {
			return VerifyReport{}, err
		}
		if err := verifyEntryBytes(entry, data); err != nil {
			return VerifyReport{}, err
		}
		report.Entries = append(report.Entries, Verification{
			Coordinate: Coordinate{ID: entry.ID, Release: entry.Release}, Digest: entry.Bundle.Digest.String(),
			Status: eartifact.EffectiveStatus(entry.Lifecycle, at), BlobPath: blobPath,
		})
	}
	return report, nil
}

func entryFromBundle(value *bundle.Bundle, wire []byte, at time.Time) (Entry, error) {
	identity := value.Payload.Identity
	fullDescriptor := eartifact.NormalizeDescriptor(eartifact.Descriptor{
		Version: eartifact.DescriptorVersion, MediaType: BlobMediaType, SizeBytes: int64(len(wire)),
		Digest:      digest.SHA256Bytes(wire),
		Annotations: map[string]string{"browsertools.id": identity.ID, "browsertools.release": identity.Release},
	})
	actions := make([]string, 0, len(value.Payload.Profile.Actions))
	for name := range value.Payload.Profile.Actions {
		actions = append(actions, name)
	}
	slices.Sort(actions)
	assessment := eartifact.Assessment{
		Version: eartifact.AssessmentVersion, Subject: fullDescriptor, Status: eartifact.LifecycleActive,
		AssessedAt: at, ExpiresAt: value.Assessment.ExpiresAt,
		Supporting: []eartifact.Descriptor{value.Descriptor},
	}
	if err := eartifact.ValidateAssessment(assessment); err != nil {
		return Entry{}, err
	}
	entry := Entry{
		ID: identity.ID, Release: identity.Release, Title: identity.Title,
		Origins: append([]string(nil), identity.Origins...), Actions: actions, ActionCount: len(actions),
		PublishedAt: value.Payload.PublishedAt, Provenance: value.Payload.Provenance,
		Bundle: fullDescriptor, Lifecycle: assessment,
	}
	entry = Normalize(Index{Version: IndexVersion, Entries: []Entry{entry}}).Entries[0]
	if err := validateEntry(entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func verifyEntryBytes(entry Entry, data []byte) error {
	expected := eartifact.NormalizeDescriptor(eartifact.Descriptor{
		Version: eartifact.DescriptorVersion, MediaType: BlobMediaType, SizeBytes: int64(len(data)),
		Digest:      digest.SHA256Bytes(data),
		Annotations: map[string]string{"browsertools.id": entry.ID, "browsertools.release": entry.Release},
	})
	if !descriptorEqual(expected, entry.Bundle) {
		return fmt.Errorf("registry blob for %s@%s does not match its descriptor", entry.ID, entry.Release)
	}
	value, err := bundle.Parse(data)
	if err != nil {
		return fmt.Errorf("registry blob %s@%s: %w", entry.ID, entry.Release, err)
	}
	verificationTime := value.Payload.PublishedAt
	if value.Assessment.AssessedAt.After(verificationTime) {
		verificationTime = value.Assessment.AssessedAt
	}
	if err := bundle.Verify(value, verificationTime); err != nil {
		return fmt.Errorf("registry blob %s@%s: %w", entry.ID, entry.Release, err)
	}
	actions := value.Payload.Profile.SortedActionNames()
	if entry.ID != value.Payload.Identity.ID || entry.Release != value.Payload.Identity.Release ||
		entry.Title != value.Payload.Identity.Title || !slices.Equal(entry.Origins, normalizeStrings([]string(value.Payload.Identity.Origins))) ||
		!slices.Equal(entry.Actions, actions) || entry.ActionCount != len(actions) ||
		!entry.PublishedAt.Equal(value.Payload.PublishedAt) {
		return fmt.Errorf("registry metadata for %s@%s does not match its bundle", entry.ID, entry.Release)
	}
	provenanceLeft, _ := json.Marshal(entry.Provenance)
	provenanceRight, _ := json.Marshal(value.Payload.Provenance)
	if !bytes.Equal(provenanceLeft, provenanceRight) {
		return fmt.Errorf("registry provenance for %s@%s does not match its bundle", entry.ID, entry.Release)
	}
	return nil
}

func loadLocalIndex(ctx context.Context, indexPath string) (Index, error) {
	data, err := readRegularBounded(ctx, indexPath, bundle.MaxBundleBytes)
	if errors.Is(err, os.ErrNotExist) {
		return EmptyIndex(), nil
	}
	if err != nil {
		return Index{}, err
	}
	return ParseIndex(data)
}

func installBlob(ctx context.Context, path string, data []byte, descriptor eartifact.Descriptor) (reused, created bool, err error) {
	if info, statErr := os.Lstat(path); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return false, false, fmt.Errorf("registry blob path is not a regular file: %s", path)
		}
		if err := verifyBlobFile(ctx, path, descriptor); err != nil {
			return false, false, err
		}
		return true, false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, false, statErr
	}
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".blob-")
	if err != nil {
		return false, false, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return false, false, err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return false, false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, false, err
	}
	if err := temporary.Close(); err != nil {
		return false, false, err
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			if verifyErr := verifyBlobFile(ctx, path, descriptor); verifyErr != nil {
				return false, false, verifyErr
			}
			return true, false, nil
		}
		return false, false, err
	}
	return false, true, nil
}

func verifyBlobFile(ctx context.Context, path string, descriptor eartifact.Descriptor) error {
	data, err := readRegularBounded(ctx, path, bundle.MaxBundleBytes)
	if err != nil {
		return err
	}
	actual := eartifact.NormalizeDescriptor(eartifact.Descriptor{
		Version: eartifact.DescriptorVersion, MediaType: descriptor.MediaType,
		SizeBytes: int64(len(data)), Digest: digest.SHA256Bytes(data), Annotations: descriptor.Annotations,
	})
	if !descriptorEqual(actual, descriptor) {
		return fmt.Errorf("registry blob digest or size mismatch: %s", path)
	}
	return nil
}

func prepareRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		return "", fmt.Errorf("registry root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkComponents(filepath.Dir(abs)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(abs, "blobs", "sha256"), 0o755); err != nil {
		return "", err
	}
	if err := rejectSymlinkComponents(filepath.Join(abs, "blobs", "sha256")); err != nil {
		return "", err
	}
	return abs, nil
}

func validateRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		return "", fmt.Errorf("registry root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err := rejectSymlinkComponents(filepath.Join(abs, "blobs", "sha256")); err != nil {
		return "", err
	}
	return abs, nil
}

func rejectSymlinkComponents(path string) error {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	remainder := strings.TrimPrefix(path, volume)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(remainder, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("registry path contains symlink: %s", current)
		}
		if !info.IsDir() && current != path {
			return fmt.Errorf("registry path component is not a directory: %s", current)
		}
	}
	return nil
}

func acquirePublishLock(root string) (func(), error) {
	path := filepath.Join(root, ".publish.lock")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("registry has another publish transaction: %s", path)
	}
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func localBlobPath(root string, record digest.Record) (string, error) {
	relative, err := BlobPath(record)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}

func readRegularBounded(ctx context.Context, path string, max int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("registry path is not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) || after.Size() > max {
		return nil, fmt.Errorf("registry file changed or exceeds %d bytes: %s", max, path)
	}
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(file, max+1)}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("registry file exceeds %d bytes: %s", max, path)
	}
	return data, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("registry output is not a regular file: %s", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".index-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func findCoordinate(entries []Entry, coordinate Coordinate) int {
	for index, entry := range entries {
		if entry.ID == strings.TrimSpace(coordinate.ID) && entry.Release == strings.TrimSpace(coordinate.Release) {
			return index
		}
	}
	return -1
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
