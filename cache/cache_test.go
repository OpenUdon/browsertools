package cache

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorePutGetDeduplicateAndList(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 8, 15, 1, 2, 3, 4, time.FixedZone("offset", 3600))
	options := PutOptions{
		Kind:                KindNormalizedEvidence,
		MediaType:           " Application/JSON ",
		CreatedAt:           created,
		ExpiresAt:           created.Add(24 * time.Hour),
		Source:              " playwright ",
		Annotations:         map[string]string{" title ": " Example "},
		PublicationEligible: true,
	}
	first, err := store.Put(context.Background(), strings.NewReader("evidence"), options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Put(context.Background(), strings.NewReader("evidence"), options)
	if err != nil {
		t.Fatal(err)
	}
	if !entriesEqual(first, second) {
		t.Fatalf("deduplicated entry differs: %#v != %#v", first, second)
	}
	if first.CreatedAt != "2026-08-15T00:02:03.000000004Z" || first.MediaType != "application/json" || first.Source != "playwright" {
		t.Fatalf("normalized entry = %#v", first)
	}
	if first.Annotations["title"] != "Example" || options.Annotations[" title "] != " Example " {
		t.Fatalf("annotations mutated or not normalized: input=%#v entry=%#v", options.Annotations, first.Annotations)
	}
	loaded, payload, err := store.Get(context.Background(), first.ID, created)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != first.ID || string(payload) != "evidence" {
		t.Fatalf("loaded entry=%#v payload=%q", loaded, payload)
	}
	entries, err := store.List(context.Background(), created, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != first.ID {
		t.Fatalf("entries = %#v", entries)
	}
	assertPrivateModes(t, root, first.ID)
}

func TestStoreRejectsConflictingMetadataForSameContent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	options := validPutOptions()
	if _, err := store.Put(context.Background(), strings.NewReader("same"), options); err != nil {
		t.Fatal(err)
	}
	options.Kind = KindProfile
	if _, err := store.Put(context.Background(), strings.NewReader("same"), options); err == nil || !strings.Contains(err.Error(), "conflicting metadata") {
		t.Fatalf("conflicting metadata error = %v", err)
	}
}

func TestStoreExpiryAndPrune(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	options := validPutOptions()
	options.ExpiresAt = options.CreatedAt.Add(time.Hour)
	entry, err := store.Put(context.Background(), strings.NewReader("expires"), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, options.ExpiresAt); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired get error = %v", err)
	}
	entries, err := store.List(context.Background(), options.ExpiresAt, false)
	if err != nil || len(entries) != 0 {
		t.Fatalf("non-expired list = %#v err=%v", entries, err)
	}
	removed, err := store.Prune(context.Background(), options.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != entry.ID {
		t.Fatalf("removed = %#v", removed)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pruned get error = %v", err)
	}
	if _, err := store.Prune(context.Background(), time.Time{}); err == nil {
		t.Fatal("zero prune time unexpectedly accepted")
	}
}

func TestStoreRejectsOversizedCancellationAndNoProgress(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	store.maxBytes = 4
	if _, err := store.Put(context.Background(), strings.NewReader("12345"), validPutOptions()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, strings.NewReader("x"), validPutOptions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if _, err := store.Put(context.Background(), zeroReader{}, validPutOptions()); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("no-progress error = %v", err)
	}
}

func TestStoreEnforcesItemCapBeforeCommitAndStillReuses(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	store.maxItems = 1
	first, err := store.Put(context.Background(), strings.NewReader("first"), validPutOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), strings.NewReader("first"), validPutOptions()); err != nil {
		t.Fatalf("identical entry must remain reusable at the cap: %v", err)
	}
	if _, err := store.Put(context.Background(), strings.NewReader("second"), validPutOptions()); err == nil || !strings.Contains(err.Error(), "limit is 1") {
		t.Fatalf("item cap error = %v", err)
	}
	entries, err := store.List(context.Background(), time.Time{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != first.ID {
		t.Fatalf("cache changed after rejected put: %#v", entries)
	}
}

func TestStoreRejectsUnsafePathsAndTampering(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Open(link); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("symlink root error = %v", err)
	}

	store, err := Open(filepath.Join(root, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Put(context.Background(), strings.NewReader("safe"), validPutOptions())
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.entryDir(entry.ID)
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte("evil"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("tamper error = %v", err)
	}
	if _, _, err := store.Get(context.Background(), "../escape", time.Time{}); err == nil {
		t.Fatal("unsafe id unexpectedly accepted")
	}
}

func TestStoreRejectsSymlinkAndBroadPermissions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Put(context.Background(), strings.NewReader("safe"), validPutOptions())
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.entryDir(entry.ID)
	payload := filepath.Join(dir, "payload")
	if err := os.Chmod(payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); err == nil || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("broad permission error = %v", err)
	}
	if err := os.Chmod(payload, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(payload, manifest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
		t.Fatalf("manifest symlink error = %v", err)
	}
}

func TestPublicationBoundaryAndEntryValidation(t *testing.T) {
	options := validPutOptions()
	options.Kind = KindPrivateRaw
	options.PublicationEligible = true
	if _, err := newEntry([]byte("raw"), options); err == nil || !strings.Contains(err.Error(), "private raw") {
		t.Fatalf("raw publication error = %v", err)
	}
	options.PublicationEligible = false
	raw, err := newEntry([]byte("raw"), options)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateForPublication(raw); err == nil || !strings.Contains(err.Error(), "private raw") {
		t.Fatalf("raw ValidateForPublication error = %v", err)
	}

	normalized, err := newEntry([]byte("normalized"), validPutOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateForPublication(normalized); err != nil {
		t.Fatal(err)
	}
	normalized.PublicationEligible = false
	if err := ValidateForPublication(normalized); err == nil || !strings.Contains(err.Error(), "not marked") {
		t.Fatalf("eligibility error = %v", err)
	}

	bad := validPutOptions()
	bad.Annotations = map[string]string{"key": "one", " key ": "two"}
	if _, err := newEntry([]byte("x"), bad); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("annotation collision error = %v", err)
	}
	bad = validPutOptions()
	bad.ExpiresAt = bad.CreatedAt
	if _, err := newEntry([]byte("x"), bad); err == nil || !strings.Contains(err.Error(), "after") {
		t.Fatalf("time ordering error = %v", err)
	}
}

func TestStoreRejectsMalformedManifestAndUnexpectedFile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Put(context.Background(), strings.NewReader("safe"), validPutOptions())
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := store.entryDir(entry.ID)
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["unknown"] = true
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown manifest field error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), entry.ID, time.Time{}); err == nil || !strings.Contains(err.Error(), "only") {
		t.Fatalf("unexpected file error = %v", err)
	}
}

func validPutOptions() PutOptions {
	return PutOptions{
		Kind:                KindNormalizedEvidence,
		MediaType:           "application/json",
		CreatedAt:           time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
		Source:              "synthetic",
		PublicationEligible: true,
	}
}

func assertPrivateModes(t *testing.T, root, id string) {
	t.Helper()
	for _, path := range []string{root, filepath.Join(root, "entries"), filepath.Join(root, "entries", strings.TrimPrefix(id, "sha256:"))} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("directory %s mode = %o", path, info.Mode().Perm())
		}
	}
	entryDir := filepath.Join(root, "entries", strings.TrimPrefix(id, "sha256:"))
	for _, name := range []string{"manifest.json", "payload"} {
		info, err := os.Stat(filepath.Join(entryDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("file %s mode = %o", name, info.Mode().Perm())
		}
	}
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }
