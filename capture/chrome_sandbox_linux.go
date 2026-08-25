//go:build linux

package capture

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const linuxMountNoSUID = 2

// validChromeSandboxHelper admits only a host-admin-controlled setuid helper.
// Chromium performs its own validation too; this check prevents the browser
// environment allowlist from becoming a caller-selected executable surface.
func validChromeSandboxHelper(path string) bool {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode()&os.ModeSetuid == 0 || info.Mode().Perm() != 0o755 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Nlink != 1 {
		return false
	}
	var filesystem syscall.Statfs_t
	if err := syscall.Statfs(path, &filesystem); err != nil || filesystem.Flags&linuxMountNoSUID != 0 {
		return false
	}
	return trustedChromeSandboxAncestors(filepath.Dir(path))
}

func trustedChromeSandboxAncestors(directory string) bool {
	for {
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return false
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return true
		}
		directory = parent
	}
}
