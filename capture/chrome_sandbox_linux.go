//go:build linux

package capture

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// validChromeSandboxHelper admits only a host-admin-controlled setuid helper.
// Chromium performs its own validation too; this check prevents the browser
// environment allowlist from becoming a caller-selected executable surface.
func validChromeSandboxHelper(path string) bool {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode()&os.ModeSetuid == 0 || info.Mode().Perm()&0o022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Nlink != 1 {
		return false
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0o022 != 0 {
		return false
	}
	parentStat, ok := parent.Sys().(*syscall.Stat_t)
	return ok && parentStat.Uid == 0
}
