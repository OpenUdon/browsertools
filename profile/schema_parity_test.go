package profile

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OpenUdon/uws/schemas"
)

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
// pinned uws.browser.1.5 module schema on every test run.
func TestSchemaParity(t *testing.T) {
	upstream, err := schemas.BrowserSourceProfileSchema("uws.browser.1.5")
	if err != nil {
		t.Fatalf("read pinned UWS browser.1.5 schema: %v", err)
	}

	embedded, err := SchemaBytes()
	if err != nil {
		t.Fatalf("read embedded schema: %v", err)
	}

	if !bytes.Equal(canonicalJSON(t, embedded), canonicalJSON(t, upstream)) {
		t.Errorf("embedded schema has drifted from the pinned UWS module; re-sync profile/schema/browser.1.5.json")
	}
}
