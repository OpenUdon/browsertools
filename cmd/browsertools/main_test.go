package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	capabilitybundle "github.com/OpenUdon/browsertools/bundle"
	"github.com/OpenUdon/browsertools/cache"
	"github.com/OpenUdon/browsertools/evidence"
	"github.com/OpenUdon/browsertools/profile"
	"github.com/OpenUdon/browsertools/review"
)

func TestProfileValidateExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"profile", "validate", "--input", "../../profile/testdata/valid_minimal.yaml"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || strings.TrimSpace(stdout.String()) != "valid" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"profile", "validate", "--input", "../../profile/testdata/invalid_action_missing_sequence.yaml", "--format", "json"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stdout.String(), `"valid":false`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestProfileValidateFromStdin(t *testing.T) {
	data, err := os.ReadFile("../../profile/testdata/valid_minimal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"profile", "validate", "--input", "-"}, bytes.NewReader(data), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestEvidenceImportFromStdin(t *testing.T) {
	raw := `{"url":"https://example.test/status","observedAt":"2026-01-01T00:00:00Z","actionHint":"read_status","snapshot":{"role":"status","name":"OK"}}`
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"evidence", "import", "--adapter", "playwright", "--input", "-",
		"--origin", "https://example.test", "--redaction-status", "not_required",
	}, strings.NewReader(raw), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	var records []evidence.Record
	if err := json.Unmarshal(stdout.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ActionHint != "read_status" {
		t.Fatalf("unexpected records: %+v", records)
	}
	if strings.Contains(stdout.String(), "RawData") {
		t.Fatal("raw input leaked into normalized evidence")
	}
}

func TestEvidenceImportRequiresExplicitSafeRedaction(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"evidence", "import", "--adapter", "playwright", "--input", "-",
		"--origin", "https://example.test", "--redaction-status", "pending",
	}, strings.NewReader(`{}`), &stdout, &stderr)
	if code != exitUsageOrIO {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFullFilePipelineAndOverwriteProtection(t *testing.T) {
	tmp := t.TempDir()
	evidencePath := filepath.Join(tmp, "evidence.json")
	specPath := filepath.Join(tmp, "spec.yaml")
	profilePath := filepath.Join(tmp, "profile.yaml")
	bundlePath := filepath.Join(tmp, "bundle.json")
	reportPath := filepath.Join(tmp, "report.json")

	records := []evidence.Record{{
		Origin: "https://example.test", ObservationKind: evidence.ObservationA11ySnapshot,
		ObservedAt: "2026-01-01T00:00:00Z", ActionHint: "read_status",
		CandidateLocators: []evidence.CandidateLocator{{Role: "status", Name: "OK"}},
		RedactionStatus:   evidence.RedactionNotRequired, Provenance: evidence.Provenance{Tool: "synthetic"},
	}}
	writeJSON(t, evidencePath, records)
	spec := `info:
  title: Example
  origin: https://example.test
observationKind: accessibility_snapshot
confidence: medium
expiresAfter: P30D
actions:
  read_status:
    sequence:
      - navigate: /status
      - wait_for: {role: status, name: OK}
    sideEffects: [read_only]
    confirmationPolicy: {required: false}
`
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"draft", "build", "--evidence", evidencePath, "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("draft code=%d stderr=%q", code, stderr.String())
	}
	if _, err := profile.LoadFile(profilePath); err != nil {
		t.Fatal(err)
	}

	stderr.Reset()
	code = run([]string{"draft", "build", "--evidence", evidencePath, "--spec", specPath, "--out", profilePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "refusing to overwrite") {
		t.Fatalf("overwrite code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"review", "bundle", "--profile", profilePath, "--evidence", evidencePath, "--at", "2026-01-02T00:00:00Z", "--out", bundlePath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("review code=%d stderr=%q", code, stderr.String())
	}
	var bundle review.Bundle
	data, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if !bundle.Promotable() {
		t.Fatalf("bundle not promotable: %+v", bundle.Gaps)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"revalidate", "check", "--profile", profilePath, "--evidence", evidencePath, "--at", "2026-01-02T00:00:00Z", "--out", reportPath}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("revalidate code=%d stderr=%q", code, stderr.String())
	}
}

func TestCacheCLIPrivateLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"cache", "put", "--root", root, "--input", "-",
		"--kind", "normalized_evidence", "--media-type", "application/json",
		"--created-at", "2026-08-15T00:00:00Z", "--expires-at", "2026-08-16T00:00:00Z",
		"--source", "playwright", "--annotation", "title=Example", "--publication-eligible",
	}, strings.NewReader(`{"safe":true}`), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("put code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var entry cache.Entry
	if err := json.Unmarshal(stdout.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Kind != cache.KindNormalizedEvidence || !entry.PublicationEligible {
		t.Fatalf("entry = %#v", entry)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "list", "--root", root, "--at", "2026-08-15T12:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), entry.ID) {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-15T12:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || stdout.String() != `{"safe":true}` {
		t.Fatalf("get code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "get", "--root", root, "--id", entry.ID, "--at", "2026-08-16T00:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "expired") {
		t.Fatalf("expired get code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cache", "prune", "--root", root, "--at", "2026-08-16T00:00:00Z"}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), entry.ID) {
		t.Fatalf("prune code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCacheCLIRejectsRawPublicationAndBadAnnotations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	var stdout, stderr bytes.Buffer
	base := []string{
		"cache", "put", "--root", root, "--input", "-", "--kind", "private_raw",
		"--media-type", "application/zip", "--created-at", "2026-08-15T00:00:00Z",
	}
	code := run(append(append([]string(nil), base...), "--publication-eligible"), strings.NewReader("raw"), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "private raw") {
		t.Fatalf("raw publication code=%d stderr=%q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	args := append(append([]string(nil), base...), "--annotation", "duplicate=one", "--annotation", "duplicate=two")
	code = run(args, strings.NewReader("raw"), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "duplicated") {
		t.Fatalf("duplicate annotation code=%d stderr=%q", code, stderr.String())
	}
}

func TestCapabilityBundleCLIBuildAndVerify(t *testing.T) {
	fixtureData, err := os.ReadFile("../../testdata/capability-bundles/read-only.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := capabilitybundle.Parse(fixtureData)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	profilePath := filepath.Join(tmp, "profile.json")
	reviewPath := filepath.Join(tmp, "review.json")
	evidencePath := filepath.Join(tmp, "evidence.json")
	companionPath := filepath.Join(tmp, "workflow.uws.yaml")
	writeJSON(t, profilePath, fixture.Payload.Profile)
	writeJSON(t, reviewPath, fixture.Payload.Review)
	writeJSON(t, evidencePath, fixture.Payload.Evidence)
	if err := os.WriteFile(companionPath, fixture.Payload.Companions[0].Content, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"bundle", "build", "--id", "example/status", "--release", "1.0.0",
		"--profile", profilePath, "--review", reviewPath, "--evidence", evidencePath,
		"--source", "reviewed_synthetic_fixture", "--license", "CC0-1.0",
		"--author", "Bob", "--author", "Alice", "--published-at", "2026-08-16T00:00:00Z",
		"--uws", "workflow.uws.yaml=" + companionPath,
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("build code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), fixtureData) {
		t.Fatal("CLI build did not reproduce the canonical fixture")
	}

	builtPath := filepath.Join(tmp, "bundle.json")
	if err := os.WriteFile(builtPath, stdout.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"bundle", "verify", "--input", builtPath, "--at", "2026-08-16T00:00:00Z", "--format", "json",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"valid":true`) || !strings.Contains(stdout.String(), `"example/status"`) {
		t.Fatalf("verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"bundle", "verify", "--input", builtPath, "--at", "2026-09-14T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "expired") {
		t.Fatalf("expired verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRegistryCLILocalPublishSearchPullAndVerify(t *testing.T) {
	fixturePath := "../../testdata/capability-bundles/read-only.json"
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "registry")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"registry", "publish", "--root", root, "--bundle", fixturePath, "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"reused_blob":false`) {
		t.Fatalf("publish code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "search", "--location", root, "--query", "read_status", "--at", "2026-08-16T00:00:00Z", "--format", "text",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "example/status@1.0.0\tactive") {
		t.Fatalf("search code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "pull", "--location", root, "--id", "example/status", "--release", "1.0.0", "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !bytes.Equal(stdout.Bytes(), fixtureData) {
		t.Fatalf("pull code=%d matches=%t stderr=%q", code, bytes.Equal(stdout.Bytes(), fixtureData), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "verify", "--location", root, "--at", "2026-08-16T00:00:00Z", "--format", "text",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "example/status@1.0.0\tactive") {
		t.Fatalf("verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "publish", "--root", root, "--bundle", fixturePath, "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), `"reused_entry":true`) {
		t.Fatalf("republish code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRegistryCLIRequiresExplicitNetworkApprovalAndValidCoordinates(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"registry", "search", "--location", "https://example.com/catalog", "--at", "2026-08-16T00:00:00Z",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitRejected || !strings.Contains(stderr.String(), "forbids") {
		t.Fatalf("network code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "search", "--location", "https://example.com/catalog", "--at", "2026-08-16T00:00:00Z", "--network", "sometimes",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "never, ask, or allow") {
		t.Fatalf("policy code=%d stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = run([]string{
		"registry", "publish", "--root", t.TempDir(), "--bundle", "../../testdata/capability-bundles/read-only.json",
		"--at", "2026-08-16T00:00:00Z", "--supersedes", "invalid",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != exitUsageOrIO || !strings.Contains(stderr.String(), "ID@RELEASE") {
		t.Fatalf("coordinate code=%d stderr=%q", code, stderr.String())
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
