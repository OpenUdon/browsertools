package registrationprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/uws/schemas"
)

func TestParseAndLifecycle(t *testing.T) {
	data := readFixture(t)
	value, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := SortedFlowNames(value); len(got) != 1 || got[0] != "create_dedicated_test_user" {
		t.Fatalf("flows = %#v", got)
	}
	if got := Origins(value); len(got) != 1 || got[0] != "https://app.example.test" {
		t.Fatalf("origins = %#v", got)
	}
	if err := ValidateAt(value, time.Date(2026, 9, 23, 23, 59, 59, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAt(value, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expiry error = %v", err)
	}
	if err := ValidateAt(value, time.Date(2026, 8, 24, 23, 59, 59, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "after assessment") {
		t.Fatalf("future verification error = %v", err)
	}
	first, err := Digest(value)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Digest(value)
	if err != nil || first != second || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("digests = %q %q, error = %v", first, second, err)
	}
}

func TestParseRejectsSensitiveSentinelAndTrailingDocument(t *testing.T) {
	valid := string(readFixture(t))
	if _, err := Parse([]byte(strings.Replace(valid, "Synthetic dedicated test registration", `"[REDACTED]"`, 1))); err == nil || !strings.Contains(err.Error(), "secret-shaped") {
		t.Fatalf("sensitive sentinel error = %v", err)
	}
	if _, err := Parse([]byte(valid + "\n---\n{}\n")); err == nil {
		t.Fatal("trailing YAML document unexpectedly parsed")
	}
	unsafeInvalid := strings.Replace(valid, "kind: password", "kind: password\n    unexpected: \"[REDACTED]\"", 1)
	if _, err := Parse([]byte(unsafeInvalid)); err == nil || !strings.Contains(err.Error(), "secret-shaped") || strings.Contains(err.Error(), "jsonschema") {
		t.Fatalf("pre-schema disclosure gate error = %v", err)
	}
}

func TestLoadFileRejectsFinalSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "profile.yaml")
	link := filepath.Join(directory, "link.yaml")
	if err := os.WriteFile(target, readFixture(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestRegistrationCallControlsValidateThroughUWS(t *testing.T) {
	call := []byte(`{"x-uws-browser-registration":{"profile":"browser-registration/test.yaml","flow":"create_dedicated_test_user","credentialBindings":{"identifier":"test_identifier","password":"test_password"},"approval":"register_test_user","duplicatePrevention":"operator_attestation","onDuplicate":"fail","ambiguousOutcome":"stop_without_retry","cleanupDisposition":"delete_separately"}}`)
	if err := schemas.ValidateBrowserRegistrationCallSupplement(call); err != nil {
		t.Fatal(err)
	}
	weakened := strings.Replace(string(call), "stop_without_retry", "retry", 1)
	if err := schemas.ValidateBrowserRegistrationCallSupplement([]byte(weakened)); err == nil {
		t.Fatal("weakened registration call unexpectedly validated")
	}
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
