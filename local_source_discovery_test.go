package browsertools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/evidence/digest"
)

var discoveryTime = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

func TestDiscoverLocalSourcesMixedValidatedRootAndDuplicates(t *testing.T) {
	root := t.TempDir()
	bundleData := mustReadDiscoveryFixture(t, filepath.Join("testdata", "capability-bundles", "read-only.json"))
	profileData := mustReadDiscoveryFixture(t, filepath.Join("..", "uws", "testdata", "browser-profile", "read-only.yaml"))
	writeDiscoveryFile(t, filepath.Join(root, "capability-bundles", "status.json"), bundleData)
	writeDiscoveryFile(t, filepath.Join(root, "browser-profiles", "status.yaml"), profileData)
	writeDiscoveryFile(t, filepath.Join(root, "duplicates", "status-copy.yaml"), profileData)
	writeDiscoveryFile(t, filepath.Join(root, "config.json"), []byte(`{"name":"ordinary config"}`))
	writeDiscoveryFile(t, filepath.Join(root, "invalid.json"), []byte(`{"profile":"uws.browser.1.5"}`))
	writeDiscoveryFile(t, filepath.Join(root, "notes.txt"), []byte("ignored"))

	report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{root}, At: discoveryTime})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 2 {
		t.Fatalf("candidates = %#v", report.Candidates)
	}
	if report.Candidates[0].Kind != LocalSourceBundle || report.Candidates[0].ID != "example/status" || report.Candidates[0].ActionCount != 1 {
		t.Fatalf("bundle candidate = %#v", report.Candidates[0])
	}
	if report.Candidates[1].Kind != LocalSourceProfile || report.Candidates[1].Title != "Example Status Reader" || report.Candidates[1].Digest != digest.SHA256String(profileData) {
		t.Fatalf("profile candidate = %#v", report.Candidates[1])
	}
	if report.Candidates[0].Score <= report.Candidates[1].Score {
		t.Fatalf("bundle score %d should exceed profile score %d", report.Candidates[0].Score, report.Candidates[1].Score)
	}
	if !hasDiscoveryDiagnostic(report.Rejected, "duplicate") || !hasDiscoveryDiagnostic(report.Rejected, "not_browser_source") || !hasDiscoveryDiagnostic(report.Rejected, "invalid_profile") {
		t.Fatalf("rejected = %#v", report.Rejected)
	}
	if len(report.Ambiguous) != 0 || len(report.Truncated) != 0 {
		t.Fatalf("unexpected blockers: ambiguous=%#v truncated=%#v", report.Ambiguous, report.Truncated)
	}
}

func TestDiscoverExplicitAmbiguityAndStaleProfile(t *testing.T) {
	root := t.TempDir()
	generic := filepath.Join(root, "document.json")
	writeDiscoveryFile(t, generic, []byte(`{"name":"could be anything"}`))
	report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{generic}, At: discoveryTime})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Ambiguous) != 1 || report.Ambiguous[0].Code != "ambiguous_document" {
		t.Fatalf("ambiguous = %#v", report.Ambiguous)
	}

	profilePath := filepath.Join(root, "profile.yaml")
	writeDiscoveryFile(t, profilePath, mustReadDiscoveryFixture(t, filepath.Join("..", "uws", "testdata", "browser-profile", "read-only.yaml")))
	report, err = DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{
		Roots: []string{profilePath}, At: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].Status != "stale" {
		t.Fatalf("stale candidate = %#v", report.Candidates)
	}
}

func TestDiscoverBoundsAreVisibleAndDuplicatesDoNotConsumeCandidateBound(t *testing.T) {
	profileData := mustReadDiscoveryFixture(t, filepath.Join("..", "uws", "testdata", "browser-profile", "read-only.yaml"))
	t.Run("visited", func(t *testing.T) {
		root := t.TempDir()
		writeDiscoveryFile(t, filepath.Join(root, "one.yaml"), profileData)
		report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{root}, At: discoveryTime, MaxVisited: 1})
		if err != nil {
			t.Fatal(err)
		}
		if !hasDiscoveryDiagnostic(report.Truncated, "visited_limit") {
			t.Fatalf("truncated = %#v", report.Truncated)
		}
	})
	t.Run("candidate", func(t *testing.T) {
		root := t.TempDir()
		writeDiscoveryFile(t, filepath.Join(root, "a.yaml"), profileData)
		other := strings.Replace(string(profileData), "Example Status Reader", "Other Status Reader", 1)
		writeDiscoveryFile(t, filepath.Join(root, "b.yaml"), []byte(other))
		report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{root}, At: discoveryTime, MaxCandidates: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Candidates) != 1 || !hasDiscoveryDiagnostic(report.Truncated, "candidate_limit") {
			t.Fatalf("report = %#v", report)
		}
	})
	t.Run("duplicates", func(t *testing.T) {
		root := t.TempDir()
		writeDiscoveryFile(t, filepath.Join(root, "a.yaml"), profileData)
		writeDiscoveryFile(t, filepath.Join(root, "b.yaml"), profileData)
		report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{root}, At: discoveryTime, MaxCandidates: 1})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Candidates) != 1 || len(report.Truncated) != 0 || !hasDiscoveryDiagnostic(report.Rejected, "duplicate") {
			t.Fatalf("report = %#v", report)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "profile.yaml")
		writeDiscoveryFile(t, path, profileData)
		report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{path}, At: discoveryTime, MaxBytes: 10})
		if err != nil {
			t.Fatal(err)
		}
		if !hasDiscoveryDiagnostic(report.Rejected, "oversized") {
			t.Fatalf("rejected = %#v", report.Rejected)
		}
	})
}

func TestDiscoverRejectsSymlinksCancellationAndHintOnlyFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DiscoverLocalSources(ctx, LocalSourceDiscoveryOptions{Roots: []string{t.TempDir()}, At: discoveryTime}); err == nil {
		t.Fatal("expected cancellation")
	}

	root := t.TempDir()
	writeDiscoveryFile(t, filepath.Join(root, "browser-profiles", "config.json"), []byte(`{"ordinary":true}`))
	if runtime.GOOS != "windows" {
		target := filepath.Join(root, "target.yaml")
		writeDiscoveryFile(t, target, mustReadDiscoveryFixture(t, filepath.Join("..", "uws", "testdata", "browser-profile", "read-only.yaml")))
		if err := os.Symlink(target, filepath.Join(root, "linked.yaml")); err != nil {
			t.Fatal(err)
		}
	}
	report, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{root}, At: discoveryTime})
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiscoveryDiagnostic(report.Rejected, "not_browser_source") {
		t.Fatalf("directory hint incorrectly proved a source: %#v", report)
	}
	if runtime.GOOS != "windows" && !hasDiscoveryDiagnostic(report.Rejected, "symlink") {
		t.Fatalf("symlink was not reported: %#v", report.Rejected)
	}

	if runtime.GOOS != "windows" {
		target := t.TempDir()
		link := filepath.Join(t.TempDir(), "root-link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{link}, At: discoveryTime}); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected root symlink rejection, got %v", err)
		}
	}
}

func TestDiscoverRequiresExplicitInputsAndIsDeterministic(t *testing.T) {
	if _, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{At: discoveryTime}); err == nil {
		t.Fatal("expected explicit-root requirement")
	}
	if _, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{t.TempDir()}}); err == nil {
		t.Fatal("expected explicit-time requirement")
	}
	profileData := mustReadDiscoveryFixture(t, filepath.Join("..", "uws", "testdata", "browser-profile", "read-only.yaml"))
	first, second := t.TempDir(), t.TempDir()
	writeDiscoveryFile(t, filepath.Join(first, "profile.yaml"), profileData)
	writeDiscoveryFile(t, filepath.Join(second, "profile.yaml"), profileData)
	a, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{second, first}, At: discoveryTime})
	if err != nil {
		t.Fatal(err)
	}
	b, err := DiscoverLocalSources(context.Background(), LocalSourceDiscoveryOptions{Roots: []string{first, second}, At: discoveryTime})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Candidates) != 1 || len(b.Candidates) != 1 || a.Candidates[0].Path != b.Candidates[0].Path || strings.Join(a.Roots, "|") != strings.Join(b.Roots, "|") {
		t.Fatalf("non-deterministic reports: %#v %#v", a, b)
	}
}

func hasDiscoveryDiagnostic(values []LocalSourceDiagnostic, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func mustReadDiscoveryFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeDiscoveryFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
