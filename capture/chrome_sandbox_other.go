//go:build !linux

package capture

func validChromeSandboxHelper(string) bool { return false }
