package profile

import (
	"path/filepath"
	"testing"
)

// TestSchemaCompiles ensures the embedded schema is well-formed and compilable.
func TestSchemaCompiles(t *testing.T) {
	if _, err := compileSchema(); err != nil {
		t.Fatalf("embedded schema failed to compile: %v", err)
	}
}

// TestValidateValidFixtures checks that the known-good fixtures validate.
func TestValidateValidFixtures(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "valid_*.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no valid_* fixtures found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := ValidateFile(path); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}

// TestValidateInvalidFixtures checks that each focused invalid fixture is
// rejected. The fixture name documents the violated rule.
func TestValidateInvalidFixtures(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("testdata", "invalid_*.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no invalid_* fixtures found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := ValidateFile(path); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

// TestValidateExampleProfiles validates the committed example profiles against
// the embedded schema, proving the shipped examples stay schema-clean.
func TestValidateExampleProfiles(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "examples", "*", "browser-profiles", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("no example browser-profiles found")
	}
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := ValidateFile(path); err != nil {
				t.Errorf("example profile failed validation: %v", err)
			}
		})
	}
}

// TestJSONYAMLRoundTrip proves JSON and YAML inputs validate identically and
// decode to the same typed Profile.
func TestJSONYAMLRoundTrip(t *testing.T) {
	jsonPath := filepath.Join("testdata", "valid_minimal.json")
	yamlPath := filepath.Join("testdata", "valid_minimal.yaml")

	if err := ValidateFile(jsonPath); err != nil {
		t.Fatalf("json fixture failed validation: %v", err)
	}
	if err := ValidateFile(yamlPath); err != nil {
		t.Fatalf("yaml fixture failed validation: %v", err)
	}

	fromJSON, err := LoadProfileFile(jsonPath)
	if err != nil {
		t.Fatalf("load json: %v", err)
	}
	fromYAML, err := LoadProfileFile(yamlPath)
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}

	if fromJSON.Schema != fromYAML.Schema {
		t.Errorf("profile mismatch: json=%q yaml=%q", fromJSON.Schema, fromYAML.Schema)
	}
	if fromJSON.Info.Title != fromYAML.Info.Title {
		t.Errorf("title mismatch: json=%q yaml=%q", fromJSON.Info.Title, fromYAML.Info.Title)
	}
	if len(fromJSON.Actions) != len(fromYAML.Actions) {
		t.Errorf("action count mismatch: json=%d yaml=%d", len(fromJSON.Actions), len(fromYAML.Actions))
	}
}

// TestLoadUnsupportedExtension exercises the load error path.
func TestLoadUnsupportedExtension(t *testing.T) {
	if _, err := LoadProfileFile("profile.txt"); err == nil {
		t.Error("expected error for unsupported extension, got nil")
	}
	if err := ValidateFile("profile.txt"); err == nil {
		t.Error("expected error for unsupported extension, got nil")
	}
}
