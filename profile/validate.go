package profile

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed schema/browser.1.5.json
var schemaFS embed.FS

// schemaResource is the embedded path; it doubles as the resource URL passed to
// the JSON Schema compiler.
const schemaResource = "schema/browser.1.5.json"

// SchemaBytes returns the embedded uws.browser.1.5 JSON Schema. The parity test
// compares this against the upstream copy in github.com/OpenUdon/uws.
func SchemaBytes() ([]byte, error) {
	return schemaFS.ReadFile(schemaResource)
}

// compiled holds the lazily-initialized singleton schema and any compile error.
// Compiling a JSON Schema is expensive (regex compilation, AST construction);
// doing it once per process is the correct pattern for a static embedded schema.
var (
	compiledOnce   sync.Once
	compiledSchema *jsonschema.Schema
	compiledErr    error
)

func compileSchema() (*jsonschema.Schema, error) {
	compiledOnce.Do(func() {
		data, err := schemaFS.ReadFile(schemaResource)
		if err != nil {
			compiledErr = fmt.Errorf("read embedded schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			compiledErr = fmt.Errorf("parse embedded schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(schemaResource, doc); err != nil {
			compiledErr = fmt.Errorf("add schema resource: %w", err)
			return
		}
		compiledSchema, compiledErr = compiler.Compile(schemaResource)
		if compiledErr != nil {
			compiledErr = fmt.Errorf("compile schema: %w", compiledErr)
		}
	})
	return compiledSchema, compiledErr
}

// Validate checks an already-decoded browser-profile value (map[string]any /
// []any / scalars, as produced by loadValue) against the embedded schema.
func Validate(value any) error {
	schema, err := compileSchema()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

// ValidateFile loads a JSON or YAML browser-profile document and validates it
// against the embedded uws.browser.1.5 schema. Errors are tagged with the path.
func ValidateFile(path string) error {
	value, err := loadValue(path)
	if err != nil {
		return err
	}
	if err := Validate(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// loadValue reads a JSON or YAML document into the generic value shape the
// JSON Schema validator expects. Mirrors github.com/OpenUdon/uws validation so
// JSON and YAML inputs validate identically.
func loadValue(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read document %s: %w", path, err)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parse JSON document %s: %w", path, err)
		}
		return value, nil
	case ".yaml", ".yml":
		var value any
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("parse YAML document %s: %w", path, err)
		}
		// yaml.v3 unmarshals numbers as int/int64/float64; jsonschema/v6 accepts
		// these native types directly, so no conversion is needed.
		return normalizeYAML(value), nil
	default:
		return nil, fmt.Errorf("unsupported document extension %q for %s", filepath.Ext(path), path)
	}
}

// normalizeYAML ensures yaml.v3's map[string]any tree has string keys
// (yaml.v3 already uses string keys, so this is a recursive pass that only
// converts map[interface{}]interface{} trees produced by older yaml decoders).
func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = normalizeYAML(val)
		}
		return out
	case []any:
		for i, val := range typed {
			typed[i] = normalizeYAML(val)
		}
		return typed
	default:
		return typed
	}
}
