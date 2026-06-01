package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// upstreamSchemaPath resolves the canonical browser.1.5.json owned by
// github.com/OpenUdon/uws. The directory is overridable via UWS_SCHEMA_DIR so
// the parity check works regardless of where uws is checked out; it defaults to
// the sibling checkout layout (../../uws/versions relative to this package).
func upstreamSchemaPath() string {
	dir := os.Getenv("UWS_SCHEMA_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "uws", "versions")
	}
	return filepath.Join(dir, "browser.1.5.json")
}

// canonicalJSON reparses JSON into a stable, key-sorted byte form so semantically
// equal schemas compare equal regardless of formatting or key order.
func canonicalJSON(t *testing.T, data []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	out, err := json.Marshal(v) // encoding/json sorts map keys
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return out
}

// TestSchemaParity guards against drift between the embedded schema copy and the
// upstream uws.browser.1.5 schema. It skips when the upstream checkout is not
// present (e.g. CI without the sibling repo); set UWS_SCHEMA_DIR to enforce.
func TestSchemaParity(t *testing.T) {
	upstreamPath := upstreamSchemaPath()
	upstream, err := os.ReadFile(upstreamPath)
	if err != nil {
		if os.IsNotExist(err) && os.Getenv("UWS_SCHEMA_DIR") == "" {
			t.Skipf("upstream schema not found at %s; set UWS_SCHEMA_DIR to enforce parity", upstreamPath)
		}
		t.Fatalf("read upstream schema %s: %v", upstreamPath, err)
	}

	embedded, err := SchemaBytes()
	if err != nil {
		t.Fatalf("read embedded schema: %v", err)
	}

	if !bytes.Equal(canonicalJSON(t, embedded), canonicalJSON(t, upstream)) {
		t.Errorf("embedded schema has drifted from %s; re-sync profile/schema/browser.1.5.json", upstreamPath)
	}
}
