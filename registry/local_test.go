package registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/bundle"
	eartifact "github.com/OpenUdon/evidence/artifact"
)

var registryTime = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

func TestPublishLocalIdempotentSupersessionAndVerify(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	first := loadFixtureBundle(t, "read-only")
	report, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: first, At: registryTime})
	if err != nil {
		t.Fatal(err)
	}
	if report.ReusedBlob || report.ReusedEntry {
		t.Fatalf("first publish unexpectedly reused content: %#v", report)
	}
	if filepath.Base(filepath.Dir(report.BlobPath)) != "sha256" || filepath.Ext(report.BlobPath) != "" {
		t.Fatalf("blob path = %s", report.BlobPath)
	}
	assertFileMode(t, report.IndexPath, 0o644)
	assertFileMode(t, report.BlobPath, 0o644)

	again, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: first, At: registryTime})
	if err != nil {
		t.Fatal(err)
	}
	if !again.ReusedBlob || !again.ReusedEntry || again.Entry.Bundle.Digest != report.Entry.Bundle.Digest {
		t.Fatalf("idempotent publish = %#v", again)
	}

	second := rebuildRelease(t, first, "2.0.0", "reviewed_synthetic_fixture")
	secondReport, err := PublishLocal(context.Background(), PublishOptions{
		Root: root, Bundle: second, At: registryTime,
		Supersedes: &Coordinate{ID: "example/status", Release: "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondReport.Entry.Release != "2.0.0" {
		t.Fatalf("entry = %#v", secondReport.Entry)
	}

	verified, err := VerifyLocal(context.Background(), root, registryTime)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.Entries) != 2 {
		t.Fatalf("verified entries = %#v", verified.Entries)
	}
	if verified.Entries[0].Status != eartifact.LifecycleSuperseded || verified.Entries[1].Status != eartifact.LifecycleActive {
		t.Fatalf("statuses = %#v", verified.Entries)
	}
	index := readIndex(t, root)
	if index.Entries[0].Lifecycle.Successor != index.Entries[1].Bundle.Digest {
		t.Fatalf("successor = %s, want %s", index.Entries[0].Lifecycle.Successor, index.Entries[1].Bundle.Digest)
	}
}

func TestPublishRejectsCoordinateCollisionAndInvalidSupersession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	first := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: first, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	conflict := rebuildRelease(t, first, "1.0.0", "different_provenance")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: conflict, At: registryTime}); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("expected coordinate collision, got %v", err)
	}
	second := rebuildRelease(t, first, "2.0.0", "reviewed_synthetic_fixture")
	if _, err := PublishLocal(context.Background(), PublishOptions{
		Root: root, Bundle: second, At: registryTime,
		Supersedes: &Coordinate{ID: "missing", Release: "1.0.0"},
	}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing supersession, got %v", err)
	}
	other := loadFixtureBundle(t, "confirmed-side-effect")
	if _, err := PublishLocal(context.Background(), PublishOptions{
		Root: root, Bundle: other, At: registryTime,
		Supersedes: &Coordinate{ID: "example/status", Release: "1.0.0"},
	}); err == nil || !strings.Contains(err.Error(), "keep capability id") {
		t.Fatalf("expected cross-id supersession rejection, got %v", err)
	}
}

func TestUpdateLifecycleTransitions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	coordinate := Coordinate{ID: "example/status", Release: "1.0.0"}
	if _, err := UpdateLifecycleLocal(context.Background(), root, coordinate, eartifact.LifecycleStale, registryTime.Add(-time.Second)); err == nil || !strings.Contains(err.Error(), "predates") {
		t.Fatalf("expected lifecycle chronology rejection, got %v", err)
	}
	stale, err := UpdateLifecycleLocal(context.Background(), root, coordinate, eartifact.LifecycleStale, registryTime.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stale.Lifecycle.Status != eartifact.LifecycleStale {
		t.Fatalf("status = %s", stale.Lifecycle.Status)
	}
	revoked, err := UpdateLifecycleLocal(context.Background(), root, coordinate, eartifact.LifecycleRevoked, registryTime.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Lifecycle.Status != eartifact.LifecycleRevoked {
		t.Fatalf("status = %s", revoked.Lifecycle.Status)
	}
	if _, err := UpdateLifecycleLocal(context.Background(), root, coordinate, eartifact.LifecycleStale, registryTime.Add(3*time.Hour)); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal transition rejection, got %v", err)
	}
	if _, err := UpdateLifecycleLocal(context.Background(), root, coordinate, eartifact.LifecycleActive, registryTime.Add(3*time.Hour)); err == nil {
		t.Fatal("expected unsupported active transition rejection")
	}
}

func TestSupersessionRejectsRegressingAssessmentTime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	first := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: first, At: registryTime.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	second := rebuildRelease(t, first, "2.0.0", "reviewed_synthetic_fixture")
	if _, err := PublishLocal(context.Background(), PublishOptions{
		Root: root, Bundle: second, At: registryTime,
		Supersedes: &Coordinate{ID: "example/status", Release: "1.0.0"},
	}); err == nil || !strings.Contains(err.Error(), "predates") {
		t.Fatalf("expected supersession chronology rejection, got %v", err)
	}
}

func TestIndexCanonicalValidationAndDanglingSuccessor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	first := loadFixtureBundle(t, "read-only")
	second := rebuildRelease(t, first, "2.0.0", "reviewed_synthetic_fixture")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: first, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishLocal(context.Background(), PublishOptions{
		Root: root, Bundle: second, At: registryTime, Supersedes: &Coordinate{ID: "example/status", Release: "1.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	index := readIndex(t, root)
	index.Entries[0], index.Entries[1] = index.Entries[1], index.Entries[0]
	wire, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIndex(wire); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("expected ordering rejection, got %v", err)
	}
	index = Normalize(index)
	index.Entries = index.Entries[:1]
	if _, err := CanonicalJSON(index); err == nil || !strings.Contains(err.Error(), "successor") {
		t.Fatalf("expected dangling successor rejection, got %v", err)
	}
	index = readIndex(t, root)
	index.Entries = append(index.Entries, index.Entries[0])
	if _, err := CanonicalJSON(index); err == nil || !strings.Contains(err.Error(), "duplicate coordinate") {
		t.Fatalf("expected duplicate coordinate rejection, got %v", err)
	}
}

func TestCanonicalIndexEnforcesWriterEntryAndByteBounds(t *testing.T) {
	tooMany := EmptyIndex()
	tooMany.Entries = make([]Entry, DefaultMaxEntries+1)
	if _, err := CanonicalJSON(tooMany); err == nil || !strings.Contains(err.Error(), "entries") {
		t.Fatalf("entry bound error = %v", err)
	}

	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	index := readIndex(t, root)
	index.Entries[0].Title = strings.Repeat("x", int(bundle.MaxBundleBytes))
	if _, err := CanonicalJSON(index); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("byte bound error = %v", err)
	}
}

func TestLocalRegistryRejectsCancellationSymlinksLocksAndTamper(t *testing.T) {
	value := loadFixtureBundle(t, "read-only")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PublishLocal(cancelled, PublishOptions{Root: filepath.Join(t.TempDir(), "cancelled"), Bundle: value, At: registryTime}); err == nil {
		t.Fatal("expected cancellation")
	}

	root := filepath.Join(t.TempDir(), "registry")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".publish.lock"), []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err == nil || !strings.Contains(err.Error(), "another publish") {
		t.Fatalf("expected lock rejection, got %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".publish.lock")); err != nil {
		t.Fatal(err)
	}

	index := readIndex(t, root)
	blobPath, err := localBlobPath(root, index.Entries[0].Bundle.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLocal(context.Background(), root, registryTime); err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("expected tamper rejection, got %v", err)
	}

	if runtime.GOOS != "windows" {
		target := filepath.Join(t.TempDir(), "target")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "registry-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := PublishLocal(context.Background(), PublishOptions{Root: link, Bundle: value, At: registryTime}); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected root symlink rejection, got %v", err)
		}
	}
}

func loadFixtureBundle(t *testing.T, name string) *bundle.Bundle {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "capability-bundles", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := bundle.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func rebuildRelease(t *testing.T, base *bundle.Bundle, release, source string) *bundle.Bundle {
	t.Helper()
	companions := append([]bundle.Companion(nil), base.Payload.Companions...)
	for index := range companions {
		companions[index].Descriptor = eartifact.Descriptor{}
	}
	value, err := bundle.Build(bundle.BuildOptions{
		ID: base.Payload.Identity.ID, Release: release, Source: source,
		License: base.Payload.Provenance.License, Authors: base.Payload.Provenance.Authors,
		Profile: &base.Payload.Profile, Review: &base.Payload.Review, Evidence: base.Payload.Evidence,
		Companions: companions, PublishedAt: base.Payload.PublishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func readIndex(t *testing.T, root string) Index {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, IndexName))
	if err != nil {
		t.Fatal(err)
	}
	value, err := ParseIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%s) = %o, want %o", path, got, want)
	}
}
