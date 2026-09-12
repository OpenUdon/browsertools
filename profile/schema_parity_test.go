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

// TestSchemaBytesForMatchesPinnedUWS proves every accepted version resolves to
// the pinned UWS schema for that exact discriminator, and that an unsupported
// version fails instead of falling back.
func TestSchemaBytesForMatchesPinnedUWS(t *testing.T) {
	for _, version := range SupportedSchemas() {
		upstream, err := schemas.BrowserSourceProfileSchema(version)
		if err != nil {
			t.Fatalf("read pinned UWS schema %s: %v", version, err)
		}
		got, err := SchemaBytesFor(version)
		if err != nil {
			t.Fatalf("SchemaBytesFor(%s): %v", version, err)
		}
		if !bytes.Equal(canonicalJSON(t, got), canonicalJSON(t, upstream)) {
			t.Errorf("SchemaBytesFor(%s) does not match the pinned UWS module schema", version)
		}
		if !bytes.Contains(got, []byte(version)) {
			t.Errorf("schema for %s does not name its own discriminator", version)
		}
	}
	if _, err := SchemaBytesFor("uws.browser.1.8"); err == nil {
		t.Fatal("unsupported version did not fail explicitly")
	}
}
