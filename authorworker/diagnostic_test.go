package authorworker

import (
	"context"
	"github.com/OpenUdon/browsertools/authordiagnostic"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerPreflightRetainsPrivateDriverClass(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, "diagnostic.json")
	err := Run(context.Background(), Options{PrivateRoot: dir, DiagnosticPath: path, DriverDirectory: filepath.Join(dir, "missing-driver"), Stdin: io.NopCloser(strings.NewReader("")), Stdout: io.Discard})
	if err == nil {
		t.Fatal("preflight unexpectedly passed")
	}
	got, err := authordiagnostic.Read(path)
	if err != nil || got != (authordiagnostic.Class{Stage: "driver", Reason: "failed"}) {
		t.Fatalf("%v %v", got, err)
	}
}
