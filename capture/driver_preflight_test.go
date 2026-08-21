package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightPlaywrightDriverRequiresExactInstalledVersion(t *testing.T) {
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", "")
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	root := installedDriverFixture(t, PlaywrightVersion)
	report, err := PreflightPlaywrightDriver(root)
	if err != nil || report.DriverDirectory != root || report.Version != PlaywrightVersion {
		t.Fatalf("report=%#v err=%v", report, err)
	}

	wrong := installedDriverFixture(t, "1.61.0")
	if _, err := PreflightPlaywrightDriver(wrong); err == nil || !strings.Contains(err.Error(), PlaywrightVersion) {
		t.Fatalf("version error = %v", err)
	}
}

func TestPreflightPlaywrightDriverDoesNotCreateMissingDirectory(t *testing.T) {
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", "")
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	root := filepath.Join(t.TempDir(), "missing", "driver")
	if _, err := PreflightPlaywrightDriver(root); err == nil {
		t.Fatal("preflight accepted missing driver")
	}
	if _, err := os.Stat(filepath.Dir(root)); !os.IsNotExist(err) {
		t.Fatalf("preflight created filesystem state: %v", err)
	}
}

func installedDriverFixture(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "package"), 0o700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(root, "node")
	if err := os.WriteFile(node, []byte("synthetic node"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package", "cli.js"), []byte("// synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := `{"name":"playwright-core","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(root, "package", "package.json"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
