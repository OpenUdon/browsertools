package registrationdraft

import (
	"os"
	"testing"

	"github.com/OpenUdon/browsertools/registrationprofile"
	"gopkg.in/yaml.v3"
)

func TestBuildIsDeterministicAndValidated(t *testing.T) {
	data, err := os.ReadFile("../registrationprofile/testdata/valid-registration.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := yaml.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	delete(wire, "profile")
	normalized, err := yaml.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var spec Spec
	if err := yaml.Unmarshal(normalized, &spec); err != nil {
		t.Fatal(err)
	}
	first, err := Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, _ := registrationprofile.Digest(first)
	secondDigest, _ := registrationprofile.Digest(second)
	if firstDigest != secondDigest {
		t.Fatalf("digests differ: %s %s", firstDigest, secondDigest)
	}
}

func TestBuildDoesNotInferMissingMutationStep(t *testing.T) {
	if _, err := Build(Spec{}); err == nil {
		t.Fatal("empty registration specification unexpectedly built")
	}
}
