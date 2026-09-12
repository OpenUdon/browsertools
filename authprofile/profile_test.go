package authprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenUdon/uws/browserauthentication"
)

func TestParseAndLifecycle(t *testing.T) {
	data := readFixture(t)
	value, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := SortedFlowNames(value); len(got) != 1 || got[0] != "member_login_push" {
		t.Fatalf("flows = %#v", got)
	}
	if got := Origins(value); len(got) != 2 || got[0] != "https://login.example.test" {
		t.Fatalf("origins = %#v", got)
	}
	if err := ValidateAt(value, time.Date(2026, 9, 13, 23, 59, 59, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAt(value, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expiry error = %v", err)
	}
}

func TestParseRejectsSecretAndPIIShapedValues(t *testing.T) {
	valid := string(readFixture(t))
	for name, data := range map[string]string{
		"secret": strings.Replace(valid, "Example member login", "token=abcdefghijklmnop123456", 1),
		"email":  strings.Replace(valid, "Example member login", "alice@example.com", 1),
		"phone":  strings.Replace(valid, "Example member login", "+1 212 555 0199", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(data)); err == nil {
				t.Fatal("unsafe profile unexpectedly parsed")
			}
		})
	}
}

func TestParseRejectsPIIShapedIdentifierKeys(t *testing.T) {
	valid := string(readFixture(t))
	unsafe := strings.ReplaceAll(valid, "username", "account_12125550199")
	if _, err := Parse([]byte(unsafe)); err == nil || !strings.Contains(err.Error(), "PII-shaped key") {
		t.Fatalf("unsafe identifier key error = %v", err)
	}
}

func TestParseRejectsTrailingYAML(t *testing.T) {
	if _, err := Parse(append(readFixture(t), []byte("\n---\n{}\n")...)); err == nil {
		t.Fatal("trailing YAML document unexpectedly parsed")
	}
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/valid-push.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestParseContextProfile proves the additive authentication 1.1 contract
// parses through the same helpers as 1.0, including declared popup and frame
// contexts and an exact success path.
func TestParseContextProfile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "valid-popup-frame.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if value.Profile != browserauthentication.ContextProfileName {
		t.Fatalf("profile = %q", value.Profile)
	}
	if len(value.Contexts) != 2 {
		t.Fatalf("contexts = %#v", value.Contexts)
	}
	if got := value.Contexts["otp_frame"]; got.Kind != "frame" || got.Parent != "idp_popup" {
		t.Fatalf("otp_frame = %#v", got)
	}
	if got := SortedFlowNames(value); len(got) != 1 || got[0] != "member_login_sms" {
		t.Fatalf("flows = %#v", got)
	}
	if got := value.Flows["member_login_sms"].Success.Path; got != "/dashboard" {
		t.Fatalf("success path = %q", got)
	}
	if got := Origins(value); len(got) != 2 {
		t.Fatalf("origins = %#v", got)
	}
}
