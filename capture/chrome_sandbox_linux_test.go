//go:build linux

package capture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChromeSandboxHelperRejectsUserControlledAndNoncanonicalPaths(t *testing.T) {
	root := t.TempDir()
	helper := filepath.Join(root, "chrome_sandbox")
	if err := os.WriteFile(helper, []byte("synthetic helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(helper, os.ModeSetuid|0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "helper-link")
	if err := os.Symlink(helper, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "relative", helper + string(os.PathSeparator) + "..", helper, link} {
		if validChromeSandboxHelper(path) {
			t.Fatalf("user-controlled or noncanonical helper %q was admitted", path)
		}
	}
	if trustedChromeSandboxAncestors(root) {
		t.Fatal("user-controlled ancestor chain was admitted")
	}
	if !trustedChromeSandboxAncestors(string(os.PathSeparator)) {
		t.Fatal("root-owned filesystem root was rejected")
	}
}
