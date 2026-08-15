package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/cache"
	bevidence "github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
	eartifact "github.com/OpenUdon/evidence/artifact"
)

var fixtureTime = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

func TestBuildVerifyDeterministicAndTimeBound(t *testing.T) {
	first := buildFixture(t, "read-only", nil)
	second := buildFixture(t, "read-only", func(options *BuildOptions) {
		options.Authors = []string{"Bob", "Alice", "Alice"}
	})
	firstJSON, err := CanonicalJSON(first, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := CanonicalJSON(second, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("equivalent inputs produced different canonical bundles")
	}
	firstDigest, err := Digest(first, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := Digest(second, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("digest mismatch: %s != %s", firstDigest, secondDigest)
	}
	if err := Verify(first, time.Date(2026, 9, 13, 23, 59, 59, 0, time.UTC)); err != nil {
		t.Fatalf("bundle should still be current: %v", err)
	}
	if err := Verify(first, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiry, got %v", err)
	}
}

func TestSideEffectBundleRequiresConfirmationAndSafeReview(t *testing.T) {
	built := buildFixture(t, "confirmed-side-effect", nil)
	if !built.Payload.Review.SideEffects.HasWriteActions {
		t.Fatal("side-effect fixture was not classified as a write")
	}
	if got := built.Payload.Review.SideEffects.ActionsRequiringConfirmation; len(got) != 1 || got[0] != "update_record" {
		t.Fatalf("unexpected confirmation summary: %#v", got)
	}

	prof, records, companion := fixtureInputs(t, "confirmed-side-effect")
	action := prof.Actions["update_record"]
	action.ConfirmationPolicy.Required = false
	action.ConfirmationPolicy.Prompt = ""
	prof.Actions["update_record"] = action
	reviewBundle, err := review.Build(prof, records, nil, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Build(BuildOptions{
		ID: "example/editor", Release: "1.0.0", Source: "synthetic", License: "CC0-1.0",
		Authors: []string{"OpenUdon"}, Profile: prof, Review: reviewBundle, Evidence: records,
		Companions: []Companion{companion}, PublishedAt: fixtureTime,
	})
	if err == nil || (!strings.Contains(err.Error(), "confirmation") && !strings.Contains(err.Error(), "allOf")) {
		t.Fatalf("expected confirmation rejection, got %v", err)
	}
}

func TestVerifyRejectsEveryTamperedBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Bundle)
		want   string
	}{
		{"profile", func(value *Bundle) { value.Payload.Profile.Info.Title = "Changed" }, "title"},
		{"review", func(value *Bundle) { value.Payload.Review.ProfileDigest = "sha256:" + strings.Repeat("0", 64) }, "review verification"},
		{"evidence", func(value *Bundle) { value.Payload.Evidence[0].ObservedAt = "2026-08-14T00:00:00Z" }, "review verification"},
		{"companion", func(value *Bundle) { value.Payload.Companions[0].Content[0] ^= 1 }, "companion"},
		{"descriptor", func(value *Bundle) { value.Descriptor.SizeBytes++ }, "descriptor mismatch"},
		{"assessment", func(value *Bundle) { value.Assessment.Status = eartifact.LifecycleRevoked }, "lifecycle"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneBundle(t, buildFixture(t, "read-only", nil))
			test.mutate(value)
			if err := Verify(value, fixtureTime); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestCachePublicationBoundaryRevalidatesExactBytes(t *testing.T) {
	store, err := cache.Open(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"origin":"https://example.test"}`)
	put := func(kind cache.Kind, eligible bool) cache.Entry {
		entry, putErr := store.Put(context.Background(), bytes.NewReader(content), cache.PutOptions{
			Kind: kind, MediaType: "application/json", CreatedAt: fixtureTime.Add(-time.Hour),
			Source: "synthetic", PublicationEligible: eligible,
		})
		if putErr != nil {
			t.Fatal(putErr)
		}
		return entry
	}

	raw := put(cache.KindPrivateRaw, false)
	_, err = Build(buildOptions(t, "read-only", func(options *BuildOptions) {
		options.CacheEntries = []CachedArtifact{{Entry: raw, Content: content}}
	}))
	if err == nil || !strings.Contains(err.Error(), "private raw") {
		t.Fatalf("expected raw-cache rejection, got %v", err)
	}

	eligibleContent := append(content, '\n')
	eligible, err := store.Put(context.Background(), bytes.NewReader(eligibleContent), cache.PutOptions{
		Kind: cache.KindNormalizedEvidence, MediaType: "application/json", CreatedAt: fixtureTime.Add(-time.Hour),
		Source: "synthetic", PublicationEligible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := Build(buildOptions(t, "read-only", func(options *BuildOptions) {
		options.CacheEntries = []CachedArtifact{{Entry: eligible, Content: eligibleContent}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Payload.Provenance.CacheEntries) != 1 {
		t.Fatalf("expected one cache reference, got %#v", built.Payload.Provenance.CacheEntries)
	}

	_, err = Build(buildOptions(t, "read-only", func(options *BuildOptions) {
		options.CacheEntries = []CachedArtifact{{Entry: eligible, Content: []byte("tampered")}}
	}))
	if err == nil || !strings.Contains(err.Error(), "content mismatch") {
		t.Fatalf("expected cache-content rejection, got %v", err)
	}
}

func TestRejectsSecretsUnsafeCompanionsAndInvalidMetadata(t *testing.T) {
	tests := []struct {
		name   string
		change func(*BuildOptions)
		want   string
	}{
		{"secret provenance", func(options *BuildOptions) {
			options.Source = "Bearer abcdefghijklmnopqrstuvwxyz"
		}, "secret-like"},
		{"traversal", func(options *BuildOptions) { options.Companions[0].Path = "../workflow.uws.yaml" }, "safe normalized"},
		{"wrong companion kind", func(options *BuildOptions) { options.Companions[0].Path = "workflow.txt" }, "supported UWS"},
		{"session material", func(options *BuildOptions) {
			options.Companions[0].Content = []byte("uws: 1.7.0\ninfo: {title: X, version: 1.0.0}\nvariables: {cookies: abc}\n")
		}, "secret-like"},
		{"malformed companion", func(options *BuildOptions) {
			options.Companions[0].Content = []byte("uws: [")
		}, "decode document"},
		{"duplicate companion", func(options *BuildOptions) { options.Companions = append(options.Companions, options.Companions[0]) }, "duplicated"},
		{"bad id", func(options *BuildOptions) { options.ID = "../escape" }, "identity"},
		{"bad release", func(options *BuildOptions) { options.Release = "latest" }, "semantic version"},
		{"bad prerelease", func(options *BuildOptions) { options.Release = "1.0.0-." }, "semantic version"},
		{"bad license", func(options *BuildOptions) { options.License = "" }, "license"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(buildOptions(t, "read-only", test.change))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestRejectsSecretEvidenceEvenWhenReviewMatches(t *testing.T) {
	options := buildOptions(t, "read-only", nil)
	options.Evidence[0].Diagnostics = append(options.Evidence[0].Diagnostics, bevidence.Diagnostic{
		Level: "warn", Message: "observed sk-proj-123456789012345678901234",
	})
	reviewed, err := review.Build(options.Profile, options.Evidence, nil, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	options.Review = reviewed
	if _, err := Build(options); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("expected reviewed secret-evidence rejection, got %v", err)
	}
}

func TestParseStrictAndCanonicalRoundTrip(t *testing.T) {
	built := buildFixture(t, "read-only", nil)
	data, err := CanonicalJSON(built, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(parsed, fixtureTime); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(append(data, []byte("{}")...)); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
	unknown := bytes.Replace(data, []byte(`"version":`), []byte(`"unknown":true,"version":`), 1)
	if _, err := Parse(unknown); err == nil {
		t.Fatal("expected unknown field rejection")
	}
	if _, err := Parse(make([]byte, MaxBundleBytes+1)); err == nil {
		t.Fatal("expected oversized bundle rejection")
	}
}

func TestBuildAndCanonicalJSONBoundCompleteBundle(t *testing.T) {
	base := buildFixture(t, "read-only", nil)
	payloadData, err := canonicalPayloadJSON(base.Payload)
	if err != nil {
		t.Fatal(err)
	}
	targetPayloadSize := int(MaxBundleBytes - 1)
	growth := targetPayloadSize - len(payloadData)
	if growth <= 0 {
		t.Fatalf("fixture payload unexpectedly uses %d bytes", len(payloadData))
	}

	options := buildOptions(t, "read-only", nil)
	options.Source += strings.Repeat("x", growth)
	if _, err := Build(options); err == nil || !strings.Contains(err.Error(), "bundle exceeds") {
		t.Fatalf("Build oversized complete bundle error = %v", err)
	}

	oversized := *base
	oversized.Payload.Provenance.Source += strings.Repeat("x", growth)
	payloadData, err = canonicalPayloadJSON(oversized.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(payloadData)) > MaxBundleBytes {
		t.Fatalf("test payload exceeds payload bound: %d", len(payloadData))
	}
	oversized.Descriptor = descriptorFor(payloadData, PayloadMediaType, map[string]string{
		"browsertools.id": oversized.Payload.Identity.ID, "browsertools.release": oversized.Payload.Identity.Release,
	})
	oversized.Assessment.Subject = oversized.Descriptor
	if _, err := CanonicalJSON(&oversized, fixtureTime); err == nil || !strings.Contains(err.Error(), "bundle exceeds") {
		t.Fatalf("CanonicalJSON oversized complete bundle error = %v", err)
	}
}

func TestCanonicalBundleFixtures(t *testing.T) {
	for _, name := range []string{"read-only", "confirmed-side-effect"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "testdata", "capability-bundles", name+".json")
			if os.Getenv("UPDATE_GOLDEN") != "" {
				canonical, err := CanonicalJSON(buildFixture(t, name, nil), fixtureTime)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(canonical, '\n'), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			value, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if err := Verify(value, fixtureTime); err != nil {
				t.Fatal(err)
			}
			canonical, err := CanonicalJSON(value, fixtureTime)
			if err != nil {
				t.Fatal(err)
			}
			canonical = append(canonical, '\n')
			if !bytes.Equal(data, canonical) {
				t.Fatal("fixture is not exact canonical JSON")
			}
		})
	}
}

func buildFixture(t *testing.T, name string, change func(*BuildOptions)) *Bundle {
	t.Helper()
	value, err := Build(buildOptions(t, name, change))
	if err != nil {
		t.Fatalf("Build(%s): %v", name, err)
	}
	return value
}

func buildOptions(t *testing.T, name string, change func(*BuildOptions)) BuildOptions {
	t.Helper()
	prof, records, companion := fixtureInputs(t, name)
	reviewBundle, err := review.Build(prof, records, nil, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	id := "example/status"
	if name == "confirmed-side-effect" {
		id = "example/editor"
	}
	options := BuildOptions{
		ID: id, Release: "1.0.0", Source: "reviewed_synthetic_fixture", License: "CC0-1.0",
		Authors: []string{"Alice", "Bob"}, Profile: prof, Review: reviewBundle, Evidence: records,
		Companions: []Companion{companion}, PublishedAt: fixtureTime,
	}
	if change != nil {
		change(&options)
	}
	return options
}

func fixtureInputs(t *testing.T, name string) (*profile.Profile, []bevidence.Record, Companion) {
	t.Helper()
	root := filepath.Join("..", "testdata", "browser-profile")
	profilePath := filepath.Join(root, name+".yaml")
	prof, err := profile.LoadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	companionPath := filepath.Join(root, name+".uws.yaml")
	companionData, err := os.ReadFile(companionPath)
	if err != nil {
		t.Fatal(err)
	}
	record := bevidence.Record{
		Origin: "https://example.test", ObservationKind: bevidence.ObservationA11ySnapshot,
		ObservedAt: "2026-08-15T00:00:00Z", RedactionStatus: bevidence.RedactionNotRequired,
		Provenance: bevidence.Provenance{Tool: "synthetic-fixture", Version: "1"},
	}
	if name == "read-only" {
		record.ActionHint = "read_status"
		record.CandidateLocators = []bevidence.CandidateLocator{{Role: "status", Name: "Ready"}}
	} else {
		record.ActionHint = "update_record"
		record.CandidateLocators = []bevidence.CandidateLocator{
			{Role: "button", Name: "Edit"}, {Role: "dialog", Name: "Edit record"},
			{Role: "textbox", Name: "Note"}, {Role: "radio", Name: "Enabled"},
			{Role: "checkbox", Name: "Archived"}, {Role: "combobox", Name: "Priority"},
			{Role: "status", Text: "Ready to save"}, {Role: "button", Name: "Save"},
			{Role: "status", Name: "Saved"},
		}
	}
	normalized, err := (&bevidence.RawRecord{Record: record}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	return prof, []bevidence.Record{normalized}, Companion{
		Path: "workflow.uws.yaml", MediaType: UWSYAMLMediaType, Content: companionData,
	}
}

func cloneBundle(t *testing.T, value *Bundle) *Bundle {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
