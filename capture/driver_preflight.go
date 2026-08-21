package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DriverPreflight is a read-only identity check for the pinned Playwright
// Node/CLI driver. It never creates a cache directory, starts Node, invokes an
// installer, or contacts the network.
type DriverPreflight struct {
	DriverDirectory string
	NodeExecutable  string
	CLIPath         string
	Version         string
}

// PreflightPlaywrightDriver resolves the same installed-driver inputs used by
// Playwright-Go and verifies the exact pinned playwright-core version.
func PreflightPlaywrightDriver(driverDirectory string) (DriverPreflight, error) {
	driverDirectory = strings.TrimSpace(driverDirectory)
	if driverDirectory == "" {
		driverDirectory = strings.TrimSpace(os.Getenv("PLAYWRIGHT_DRIVER_PATH"))
	}
	if driverDirectory == "" {
		cacheDirectory, err := os.UserCacheDir()
		if err != nil {
			return DriverPreflight{}, fmt.Errorf("resolve Playwright cache directory")
		}
		driverDirectory = filepath.Join(cacheDirectory, "ms-playwright-go", PlaywrightVersion)
	}
	nodePath := strings.TrimSpace(os.Getenv("PLAYWRIGHT_NODEJS_PATH"))
	if nodePath == "" {
		nodeName := "node"
		if runtime.GOOS == "windows" {
			nodeName = "node.exe"
		}
		nodePath = filepath.Join(driverDirectory, nodeName)
	}
	cliPath := strings.TrimSpace(os.Getenv("PLAYWRIGHT_CLI_PATH"))
	if cliPath == "" {
		cliPath = filepath.Join(driverDirectory, "package", "cli.js")
	}
	if err := requireInstalledFile(nodePath, true, "Playwright Node executable"); err != nil {
		return DriverPreflight{}, err
	}
	if err := requireInstalledFile(cliPath, false, "Playwright CLI"); err != nil {
		return DriverPreflight{}, err
	}
	packagePath := filepath.Join(filepath.Dir(cliPath), "package.json")
	if err := requireInstalledFile(packagePath, false, "Playwright package metadata"); err != nil {
		return DriverPreflight{}, err
	}
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return DriverPreflight{}, fmt.Errorf("read Playwright package metadata")
	}
	var metadata struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&metadata); err != nil || metadata.Name != "playwright-core" {
		return DriverPreflight{}, fmt.Errorf("Playwright package metadata is invalid")
	}
	if metadata.Version != PlaywrightVersion {
		return DriverPreflight{}, fmt.Errorf("installed Playwright driver version must be %s", PlaywrightVersion)
	}
	return DriverPreflight{
		DriverDirectory: driverDirectory, NodeExecutable: nodePath,
		CLIPath: cliPath, Version: metadata.Version,
	}, nil
}

func requireInstalledFile(path string, executable bool, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%s is unavailable", label)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a non-symlink regular file", label)
	}
	if executable && runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", label)
	}
	return nil
}
