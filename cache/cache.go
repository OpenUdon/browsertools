// Package cache stores caller-supplied browser experiences and safe derived
// artifacts in an explicit private, content-addressed local directory.
package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	ManifestVersion = "browsertools.capture-cache.v1"
	DefaultMaxBytes = int64(20 << 20)
	DefaultMaxItems = 10_000
)

// Kind classifies cached bytes at the publication boundary.
type Kind string

const (
	KindPrivateRaw         Kind = "private_raw"
	KindNormalizedEvidence Kind = "normalized_evidence"
	KindProfile            Kind = "profile"
	KindReviewBundle       Kind = "review_bundle"
)

// Entry is the durable cache manifest for one immutable payload.
type Entry struct {
	Version             string            `json:"version"`
	ID                  string            `json:"id"`
	Kind                Kind              `json:"kind"`
	MediaType           string            `json:"media_type"`
	SizeBytes           int64             `json:"size_bytes"`
	Digest              string            `json:"digest"`
	CreatedAt           string            `json:"created_at"`
	ExpiresAt           string            `json:"expires_at,omitempty"`
	Source              string            `json:"source,omitempty"`
	Annotations         map[string]string `json:"annotations,omitempty"`
	PublicationEligible bool              `json:"publication_eligible"`
}

// PutOptions supplies explicit caller-owned metadata for cached bytes.
type PutOptions struct {
	Kind                Kind
	MediaType           string
	CreatedAt           time.Time
	ExpiresAt           time.Time
	Source              string
	Annotations         map[string]string
	PublicationEligible bool
}

// Store is a private filesystem cache rooted at an explicit directory.
type Store struct {
	root     string
	maxBytes int64
	maxItems int
}

// Open validates root and returns a store. The root is created with mode 0700
// when missing. Existing roots and cache subdirectories must not be symlinks.
func Open(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("cache root is required")
	}
	root = filepath.Clean(root)
	if err := ensurePrivateDir(root); err != nil {
		return nil, err
	}
	entries := filepath.Join(root, "entries")
	if err := ensurePrivateDir(entries); err != nil {
		return nil, err
	}
	return &Store{root: root, maxBytes: DefaultMaxBytes, maxItems: DefaultMaxItems}, nil
}

// Root returns the explicit cache root.
func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// Put stores at most 20 MiB of immutable content. Identical content with
// identical metadata reuses the existing entry; conflicting metadata fails.
func (s *Store) Put(ctx context.Context, reader io.Reader, options PutOptions) (Entry, error) {
	if err := s.validate(); err != nil {
		return Entry{}, err
	}
	if reader == nil {
		return Entry{}, fmt.Errorf("cache input is required")
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	payload, err := readBounded(ctx, reader, s.maxBytes)
	if err != nil {
		return Entry{}, err
	}
	entry, err := newEntry(payload, options)
	if err != nil {
		return Entry{}, err
	}
	dir, err := s.entryDir(entry.ID)
	if err != nil {
		return Entry{}, err
	}
	if _, err := os.Lstat(dir); err == nil {
		return s.reuseExisting(ctx, entry)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Entry{}, err
	}

	entriesDir := filepath.Join(s.root, "entries")
	tmp, err := os.MkdirTemp(entriesDir, ".put-")
	if err != nil {
		return Entry{}, fmt.Errorf("create cache transaction: %w", err)
	}
	if err := os.Chmod(tmp, 0o700); err != nil {
		_ = os.RemoveAll(tmp)
		return Entry{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := writeExclusive(filepath.Join(tmp, "payload"), payload, 0o600); err != nil {
		return Entry{}, err
	}
	manifest, err := json.Marshal(entry)
	if err != nil {
		return Entry{}, err
	}
	manifest = append(manifest, '\n')
	if err := writeExclusive(filepath.Join(tmp, "manifest.json"), manifest, 0o600); err != nil {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	unlock, err := acquirePutLock(entriesDir)
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	if _, err := os.Lstat(dir); err == nil {
		return s.reuseExisting(ctx, entry)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Entry{}, err
	}
	if err := s.ensurePutCapacity(entriesDir); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Lstat(dir); statErr == nil {
			return s.reuseExisting(ctx, entry)
		}
		return Entry{}, fmt.Errorf("commit cache entry: %w", err)
	}
	committed = true
	return entry, nil
}

// Get reads and verifies an entry. When at is non-zero, expired entries fail.
func (s *Store) Get(ctx context.Context, id string, at time.Time) (Entry, []byte, error) {
	if err := s.validate(); err != nil {
		return Entry{}, nil, err
	}
	entry, payload, err := s.load(ctx, id)
	if err != nil {
		return Entry{}, nil, err
	}
	if expired(entry, at) {
		return Entry{}, nil, fmt.Errorf("cache entry %s is expired", entry.ID)
	}
	return entry, payload, nil
}

// List returns verified manifests in deterministic digest order. Expired
// entries are omitted unless includeExpired is true.
func (s *Store) List(ctx context.Context, at time.Time, includeExpired bool) ([]Entry, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, "entries")
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	entryCount := 0
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(item.Name(), ".put-") {
			if item.Type()&os.ModeSymlink != 0 || !item.IsDir() {
				return nil, fmt.Errorf("cache transaction entry must be a regular directory: %s", item.Name())
			}
			continue
		}
		if item.Type()&os.ModeSymlink != 0 || !item.IsDir() {
			return nil, fmt.Errorf("cache entry must be a regular directory: %s", item.Name())
		}
		entryCount++
		if entryCount > s.maxItems {
			return nil, fmt.Errorf("cache contains more than %d entries; prune or narrow the cache", s.maxItems)
		}
		id := "sha256:" + item.Name()
		entry, _, err := s.load(ctx, id)
		if err != nil {
			return nil, err
		}
		if !includeExpired && expired(entry, at) {
			continue
		}
		entries = append(entries, entry)
	}
	slices.SortStableFunc(entries, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
	return entries, nil
}

func acquirePutLock(entriesDir string) (func(), error) {
	lockPath := filepath.Join(entriesDir, ".put-lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("another cache put is in progress")
		}
		return nil, fmt.Errorf("acquire cache put lock: %w", err)
	}
	return func() { _ = os.Remove(lockPath) }, nil
}

func (s *Store) ensurePutCapacity(entriesDir string) error {
	items, err := os.ReadDir(entriesDir)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".put-") {
			if item.Type()&os.ModeSymlink != 0 || !item.IsDir() {
				return fmt.Errorf("cache transaction entry must be a regular directory: %s", item.Name())
			}
			continue
		}
		if item.Type()&os.ModeSymlink != 0 || !item.IsDir() {
			return fmt.Errorf("cache entry must be a regular directory: %s", item.Name())
		}
		count++
	}
	if count >= s.maxItems {
		return fmt.Errorf("cache contains %d entries, limit is %d; prune before adding another entry", count, s.maxItems)
	}
	return nil
}

// Prune removes entries expired at the explicit time and returns their
// manifests in deterministic order.
func (s *Store) Prune(ctx context.Context, at time.Time) ([]Entry, error) {
	if at.IsZero() {
		return nil, fmt.Errorf("cache prune time is required")
	}
	entries, err := s.List(ctx, at, true)
	if err != nil {
		return nil, err
	}
	var removed []Entry
	for _, entry := range entries {
		if !expired(entry, at) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		dir, err := s.entryDir(entry.ID)
		if err != nil {
			return removed, err
		}
		if err := os.RemoveAll(dir); err != nil {
			return removed, fmt.Errorf("prune cache entry %s: %w", entry.ID, err)
		}
		removed = append(removed, entry)
	}
	if removed == nil {
		removed = []Entry{}
	}
	return removed, nil
}

// ValidateForPublication rejects raw or unreviewed cache classifications.
// Publication packages must still independently validate exact artifact bytes.
func ValidateForPublication(entry Entry) error {
	if err := validateEntry(entry); err != nil {
		return err
	}
	if entry.Kind == KindPrivateRaw {
		return fmt.Errorf("private raw cache entries cannot be published")
	}
	if !entry.PublicationEligible {
		return fmt.Errorf("cache entry %s is not marked publication eligible", entry.ID)
	}
	return nil
}

func (s *Store) reuseExisting(ctx context.Context, expected Entry) (Entry, error) {
	actual, _, err := s.load(ctx, expected.ID)
	if err != nil {
		return Entry{}, err
	}
	if !entriesEqual(actual, expected) {
		return Entry{}, fmt.Errorf("cache digest %s already exists with conflicting metadata", expected.ID)
	}
	return actual, nil
}

func (s *Store) load(ctx context.Context, id string) (Entry, []byte, error) {
	dir, err := s.entryDir(id)
	if err != nil {
		return Entry{}, nil, err
	}
	if err := validateEntryDirectory(dir); err != nil {
		return Entry{}, nil, err
	}
	manifest, err := readRegularFile(ctx, filepath.Join(dir, "manifest.json"), 1<<20)
	if err != nil {
		return Entry{}, nil, err
	}
	var entry Entry
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entry); err != nil {
		return Entry{}, nil, fmt.Errorf("decode cache manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Entry{}, nil, fmt.Errorf("decode cache manifest: %w", err)
	}
	if entry.ID != id {
		return Entry{}, nil, fmt.Errorf("cache manifest id %q does not match path %q", entry.ID, id)
	}
	if err := validateEntry(entry); err != nil {
		return Entry{}, nil, err
	}
	payload, err := readRegularFile(ctx, filepath.Join(dir, "payload"), s.maxBytes)
	if err != nil {
		return Entry{}, nil, err
	}
	if int64(len(payload)) != entry.SizeBytes || digestBytes(payload) != entry.Digest {
		return Entry{}, nil, fmt.Errorf("cache entry %s payload digest or size mismatch", id)
	}
	return entry, payload, nil
}

func newEntry(payload []byte, options PutOptions) (Entry, error) {
	if err := validateAnnotations(options.Annotations); err != nil {
		return Entry{}, err
	}
	entry := Entry{
		Version:             ManifestVersion,
		Kind:                options.Kind,
		MediaType:           normalizeMediaType(options.MediaType),
		SizeBytes:           int64(len(payload)),
		Digest:              digestBytes(payload),
		CreatedAt:           formatTime(options.CreatedAt),
		ExpiresAt:           formatTime(options.ExpiresAt),
		Source:              strings.TrimSpace(options.Source),
		Annotations:         normalizeAnnotations(options.Annotations),
		PublicationEligible: options.PublicationEligible,
	}
	entry.ID = entry.Digest
	if err := validateEntry(entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func validateEntry(entry Entry) error {
	if entry.Version != ManifestVersion {
		return fmt.Errorf("cache manifest version must be %q", ManifestVersion)
	}
	if !validDigest(entry.ID) || entry.Digest != entry.ID {
		return fmt.Errorf("cache id and digest must be the same sha256 value")
	}
	switch entry.Kind {
	case KindPrivateRaw, KindNormalizedEvidence, KindProfile, KindReviewBundle:
	default:
		return fmt.Errorf("cache kind %q is invalid", entry.Kind)
	}
	if entry.Kind == KindPrivateRaw && entry.PublicationEligible {
		return fmt.Errorf("private raw cache entries cannot be publication eligible")
	}
	if entry.MediaType == "" {
		return fmt.Errorf("cache media type is required")
	}
	parsed, _, err := mime.ParseMediaType(entry.MediaType)
	if err != nil || !strings.Contains(parsed, "/") {
		return fmt.Errorf("cache media type %q is invalid", entry.MediaType)
	}
	if entry.SizeBytes < 0 || entry.SizeBytes > DefaultMaxBytes {
		return fmt.Errorf("cache size %d is outside 0..%d", entry.SizeBytes, DefaultMaxBytes)
	}
	created, err := time.Parse(time.RFC3339Nano, entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("cache created_at is invalid: %w", err)
	}
	if entry.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339Nano, entry.ExpiresAt)
		if err != nil {
			return fmt.Errorf("cache expires_at is invalid: %w", err)
		}
		if !expires.After(created) {
			return fmt.Errorf("cache expires_at must be after created_at")
		}
	}
	if err := validateAnnotations(entry.Annotations); err != nil {
		return err
	}
	return nil
}

func (s *Store) validate() error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return fmt.Errorf("cache store is not initialized")
	}
	if err := validatePrivateDir(s.root); err != nil {
		return err
	}
	return validatePrivateDir(filepath.Join(s.root, "entries"))
}

func (s *Store) entryDir(id string) (string, error) {
	if !validDigest(id) {
		return "", fmt.Errorf("cache id must be sha256:<64 lowercase hex>")
	}
	hexValue := strings.TrimPrefix(id, "sha256:")
	return filepath.Join(s.root, "entries", hexValue), nil
}

func validateEntryDirectory(dir string) error {
	if err := validatePrivateDir(dir); err != nil {
		return err
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(items) != 2 {
		return fmt.Errorf("cache entry %s must contain only manifest.json and payload", filepath.Base(dir))
	}
	for _, item := range items {
		if item.Name() != "manifest.json" && item.Name() != "payload" {
			return fmt.Errorf("cache entry contains unexpected file %s", item.Name())
		}
	}
	return nil
}

func ensurePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create cache directory %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("cache path must be a non-symlink directory: %s", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set private cache directory mode: %w", err)
	}
	return nil
}

func validatePrivateDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("cache path must be a non-symlink directory: %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("cache directory permissions must not allow group or other access: %s", path)
	}
	return nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readRegularFile(ctx context.Context, path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cache input must be a regular non-symlink file: %s", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("cache file permissions must not allow group or other access: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("cache input changed during validation: %s", path)
	}
	return readBounded(ctx, file, max)
}

func readBounded(ctx context.Context, reader io.Reader, max int64) ([]byte, error) {
	if max < 0 {
		return nil, fmt.Errorf("cache byte limit must not be negative")
	}
	buffer := bytes.NewBuffer(nil)
	chunk := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := max + 1 - int64(buffer.Len())
		if remaining <= 0 {
			return nil, fmt.Errorf("cache input exceeds %d bytes", max)
		}
		readSize := len(chunk)
		if int64(readSize) > remaining {
			readSize = int(remaining)
		}
		n, err := reader.Read(chunk[:readSize])
		if n > 0 {
			_, _ = buffer.Write(chunk[:n])
			if int64(buffer.Len()) > max {
				return nil, fmt.Errorf("cache input exceeds %d bytes", max)
			}
		}
		if err == io.EOF {
			return buffer.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

func expired(entry Entry, at time.Time) bool {
	if at.IsZero() || entry.ExpiresAt == "" {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, entry.ExpiresAt)
	return err == nil && !at.Before(expires)
}

func entriesEqual(a, b Entry) bool {
	a.Annotations = normalizeAnnotations(a.Annotations)
	b.Annotations = normalizeAnnotations(b.Annotations)
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return bytes.Equal(aJSON, bJSON)
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func normalizeMediaType(value string) string {
	value = strings.TrimSpace(value)
	parsed, params, err := mime.ParseMediaType(value)
	if err != nil {
		return value
	}
	return mime.FormatMediaType(strings.ToLower(parsed), params)
}

func normalizeAnnotations(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make(map[string]string, len(value))
	for _, key := range keys {
		out[strings.TrimSpace(key)] = strings.TrimSpace(value[key])
	}
	return out
}

func validateAnnotations(value map[string]string) error {
	seen := map[string]string{}
	for key := range value {
		normalized := strings.TrimSpace(key)
		if normalized == "" {
			return fmt.Errorf("cache annotation keys must be non-empty")
		}
		if prior, exists := seen[normalized]; exists && prior != key {
			return fmt.Errorf("cache annotation keys %q and %q collide after normalization", prior, key)
		}
		seen[normalized] = key
	}
	return nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Round(0).Format(time.RFC3339Nano)
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
