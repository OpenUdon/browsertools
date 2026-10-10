package browsertools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenUdon/uws/binding"
)

func shapeFixture(t *testing.T, name string) BrowserShapeSourceInput {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata/browser-shapes", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return BrowserShapeSourceInput{ID: name, Content: data}
}

func TestBrowserShapeNativeCorpusAndFrozenManifest(t *testing.T) {
	data, err := os.ReadFile("testdata/browser-shapes/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		KinetManifestSHA256 string            `json:"kinet_manifest_sha256"`
		FrozenFiles         map[string]string `json:"frozen_files"`
		Vectors             []struct {
			Name, Source, SHA256 string
			ProfileVersion       string `json:"profile_version"`
			CallKind             string `json:"call_kind"`
		}
	}
	if json.Unmarshal(data, &manifest) != nil || len(manifest.Vectors) != 11 || manifest.KinetManifestSHA256 != "0c445a5c90d2c09be561e713c364747f4ab9a3698ea8ab46e7b7b774ccf16bad" {
		t.Fatal("changed exact corpus provenance")
	}
	for name, hash := range manifest.FrozenFiles {
		data, err := os.ReadFile(filepath.Join("testdata/browser-shapes", name))
		if err != nil || browserShapeDigest(data) != hash {
			t.Fatal("changed frozen M51 input", name, err)
		}
	}
	var sources []BrowserShapeSourceInput
	for _, vector := range manifest.Vectors {
		t.Run(vector.Name, func(t *testing.T) {
			src := shapeFixture(t, vector.Name)
			if browserShapeDigest(src.Content) != vector.SHA256 {
				t.Fatal("changed exact native source bytes")
			}
			options := BrowserShapeOptions{Sources: []BrowserShapeSourceInput{src}}
			table, err := BuildBrowserShapeTable(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			for _, op := range table.Operations {
				if op.Source.SHA256 != vector.SHA256 || op.Browser.ProfileVersion != vector.ProfileVersion || op.Browser.CallKind != vector.CallKind || !op.Complete || op.Protocol != "browser" || op.Method != "" || op.Path != "" || len(op.Servers) != 0 {
					t.Fatal("lost native identity or invented HTTP metadata", op)
				}
			}
			if err := VerifyBrowserShapeTable(t.Context(), options, table); err != nil {
				t.Fatal(err)
			}
			sources = append(sources, src)
		})
	}
	table, err := BuildBrowserShapeTable(t.Context(), BrowserShapeOptions{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := table.Marshal()
	for i, j := 0, len(sources)-1; i < j; i, j = i+1, j-1 {
		sources[i], sources[j] = sources[j], sources[i]
	}
	again, err := BuildBrowserShapeTable(t.Context(), BrowserShapeOptions{Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	other, _ := again.Marshal()
	if !bytes.Equal(wire, other) {
		t.Fatal("source permutation changed deterministic table")
	}
}

func TestBrowserShapeCanonicalGoldenAndUnicode(t *testing.T) {
	data, err := os.ReadFile("testdata/browser-shapes/m51-canonical-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		InputJSON     string `json:"input_json"`
		CanonicalJSON string `json:"canonical_utf8"`
		SHA256        string `json:"sha256"`
	}
	if json.Unmarshal(data, &golden) != nil {
		t.Fatal("bad frozen golden")
	}
	root, err := decodeBrowserShapeDocument([]byte(golden.InputJSON))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := canonicalBrowserShapeJSON(root)
	if err != nil || string(encoded) != golden.CanonicalJSON || browserShapeDigest(encoded) != golden.SHA256 {
		t.Fatal("canonical golden drift", string(encoded), golden, err)
	}
	for _, data := range []string{`{"bad":"\ud800"}`, `{"bad":"\udc00"}`, `{"bad":"\ud800\u0041"}`, `{"bad":"\ud800\ud800"}`, "{\"bad\":\"\xff\"}", `{"a":false,"a":true}`, `{"a":0,"\u0061":1}`} {
		if _, err := decodeBrowserShapeDocument([]byte(data)); err == nil {
			t.Fatal("accepted lossy or duplicate JSON", data)
		}
	}
	root, err = decodeBrowserShapeDocument([]byte(`{"ok":"\ud83d\ude00","literal":"\\ud800","quote":"\""}`))
	if err != nil || root["ok"] != "😀" || root["literal"] != `\ud800` {
		t.Fatal("refused valid escape", root, err)
	}
}

func TestBrowserShapePreservesFullDeclarationsAndExactNumbers(t *testing.T) {
	source := shapeFixture(t, "action-1.8")
	source.Content = bytes.Replace(source.Content, []byte(`"type": "string",
            "default": "sample"`), []byte(`"type": "integer", "default": 9223372036854775807, "x-extra": {"n":9223372036854775807,"decimal":1.00,"exponent":1e+03,"minus_zero":-0,"false":false,"empty":""}`), 1)
	options := BrowserShapeOptions{Sources: []BrowserShapeSourceInput{source}}
	table, err := BuildBrowserShapeTable(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	op := table.Operations[0]
	for _, token := range []string{"9223372036854775807", "1.00", "1e+03", "-0", `"false":false`, `"empty":""`} {
		if !bytes.Contains(op.Inputs[0].Schema.JSON, []byte(token)) {
			t.Fatal("lost native schema token", token, string(op.Inputs[0].Schema.JSON))
		}
	}
	for _, output := range op.Outputs {
		if !bytes.Contains(output.Schema.JSON, []byte(`"source"`)) {
			t.Fatal("lost extraction metadata")
		}
	}
	// Whitespace changes exact source identity but leaves the full selected
	// subtree hash unchanged. Changes outside the action bind source identity.
	selected := op.Browser.SelectedSHA256
	source.Content = append(source.Content, '\n')
	options.Sources[0] = source
	other, err := BuildBrowserShapeTable(t.Context(), options)
	if err != nil || other.Operations[0].Browser.SelectedSHA256 != selected || other.Sources[0].SHA256 == table.Sources[0].SHA256 {
		t.Fatal("confused exact source and canonical selected identities", err)
	}
	if err := VerifyBrowserShapeTable(t.Context(), options, table); !errors.Is(err, ErrBrowserShapeTable) {
		t.Fatal("accepted foreign exact source", err)
	}
	registration := shapeFixture(t, "registration-1.2")
	regTable, err := BuildBrowserShapeTable(t.Context(), BrowserShapeOptions{Sources: []BrowserShapeSourceInput{registration}})
	if err != nil {
		t.Fatal(err)
	}
	var conditional bool
	for _, slot := range regTable.Operations[0].Browser.RegistrationInputSlots {
		if slot.Name == "company" {
			conditional = !slot.Required && bytes.Contains(slot.Schema, []byte(`"requiredWhen"`)) && bytes.Contains(slot.Condition, []byte(`"equals":"business"`))
		}
	}
	if !conditional || len(regTable.Operations[0].Browser.CredentialSlots) != 2 {
		t.Fatal("lost registration condition or symbolic credentials")
	}
}

func TestBrowserShapeYAMLFidelityAndSafeRefusals(t *testing.T) {
	data, err := os.ReadFile("testdata/browser-profile/read-only.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildBrowserShapeTable(t.Context(), BrowserShapeOptions{Sources: []BrowserShapeSourceInput{{ID: "yaml", Content: data}}}); err != nil {
		t.Fatal(err)
	}
	value, err := decodeBrowserShapeDocument([]byte("n: 9223372036854775807\nz: -0\nd: 1.00\n"))
	encoded, _ := canonicalBrowserShapeJSON(value)
	if err != nil || string(encoded) != `{"d":1.00,"n":9223372036854775807,"z":-0}` {
		t.Fatal("YAML numeric fidelity", string(encoded), err)
	}
	for _, data := range []string{"x: 1\nx: 2\n", "x: &alias [1]\ny: *alias", "x: !custom value", "x: .nan", "x: .inf", "1: value", "x: 1\n---\nx: 2", "x:\n  <<: {a: 1}"} {
		if _, err := decodeBrowserShapeDocument([]byte(data)); err == nil {
			t.Fatal("accepted unsupported YAML", data)
		}
	}
}

func TestBrowserShapeTamperingBoundsAndUnsupportedInput(t *testing.T) {
	source := shapeFixture(t, "action-1.5")
	options := BrowserShapeOptions{Sources: []BrowserShapeSourceInput{source}}
	table, err := BuildBrowserShapeTable(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	table.Operations[0].Browser.Effects = json.RawMessage(`["writes_state"]`)
	if err := VerifyBrowserShapeTable(t.Context(), options, table); !errors.Is(err, ErrBrowserShapeTable) {
		t.Fatal("accepted forged effect", err)
	}
	for _, options := range []BrowserShapeOptions{
		{}, {Sources: []BrowserShapeSourceInput{source, source}},
		{Sources: []BrowserShapeSourceInput{source}, MaxBytes: len(source.Content) - 1},
		{Sources: []BrowserShapeSourceInput{source}, MaxBytes: 1<<20 + 1},
		{Sources: []BrowserShapeSourceInput{source}, MaxOperations: -1},
		{Sources: []BrowserShapeSourceInput{{ID: "oversized", Content: bytes.Repeat([]byte(" "), 1<<20+1)}}},
		{Sources: []BrowserShapeSourceInput{{ID: "future", Content: bytes.Replace(source.Content, []byte("uws.browser.1.5"), []byte("uws.browser.2.0"), 1)}}},
		{Sources: []BrowserShapeSourceInput{{ID: "invalid", Content: []byte(`{"profile":"uws.browser.1.5"}`)}}},
	} {
		if got, err := BuildBrowserShapeTable(t.Context(), options); !errors.Is(err, ErrBrowserShapeTable) || len(got.Sources) != 0 || len(got.Operations) != 0 {
			t.Fatal("failed open or returned partial table", got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BuildBrowserShapeTable(ctx, options); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	if err := VerifyBrowserShapeTable(t.Context(), options, binding.ShapeTable{Operations: make([]binding.OperationShape, binding.MaxOperations+1)}); !errors.Is(err, ErrBrowserShapeTable) {
		t.Fatal("accepted over-bound claim", err)
	}
	unsafe := shapeFixture(t, "action-1.9")
	unsafe.Content = bytes.Replace(unsafe.Content, []byte(`"type": "string",
            "default": "sample"`), []byte(`"type": "integer", "default": 9007199254740992`), 1)
	if _, err := BuildBrowserShapeTable(t.Context(), BrowserShapeOptions{Sources: []BrowserShapeSourceInput{unsafe}}); !errors.Is(err, ErrBrowserShapeTable) {
		t.Fatal("accepted unsafe Browser1.9 integer default", err)
	}
	bad := []byte(`{"x":` + strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65) + "}")
	if _, err := decodeBrowserShapeDocument(bad); err == nil {
		t.Fatal("accepted depth overflow")
	}
}
