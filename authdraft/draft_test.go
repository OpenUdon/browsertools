package authdraft

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenUdon/browsertools/authprofile"
	"github.com/OpenUdon/uws/browserauthentication"
	"gopkg.in/yaml.v3"
)

func TestBuildIsDeterministicAndValidated(t *testing.T) {
	data, err := os.ReadFile("../authprofile/testdata/valid-push.yaml")
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
	firstDigest, _ := authprofile.Digest(first)
	secondDigest, _ := authprofile.Digest(second)
	if firstDigest != secondDigest {
		t.Fatalf("digests differ: %s %s", firstDigest, secondDigest)
	}
}

// specFromFixture reparses a published profile as an author specification by
// dropping the discriminator, so Build must reselect it.
func specFromFixture(t *testing.T, name string) Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "authprofile", "testdata", name))
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
	return spec
}

// TestBuildKeepsMainOnlySpecAtAuthentication10 proves the oldest sufficient
// rule leaves an existing main-only recipe byte-identical.
func TestBuildKeepsMainOnlySpecAtAuthentication10(t *testing.T) {
	built, err := Build(specFromFixture(t, "valid-push.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if built.Profile != browserauthentication.ProfileName {
		t.Fatalf("profile = %q", built.Profile)
	}
	data, err := os.ReadFile(filepath.Join("..", "authprofile", "testdata", "valid-push.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := authprofile.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	builtDigest, err := authprofile.Digest(built)
	if err != nil {
		t.Fatal(err)
	}
	publishedDigest, err := authprofile.Digest(published)
	if err != nil {
		t.Fatal(err)
	}
	if builtDigest != publishedDigest {
		t.Fatalf("main-only draft drifted from the published bytes: %s != %s", builtDigest, publishedDigest)
	}
}

// TestBuildSelectsContextProfileOnlyForContextFeatures pins each individual
// 1.1-only feature to the 1.1 discriminator while the unchanged specification
// stays at 1.0.
func TestBuildSelectsContextProfileOnlyForContextFeatures(t *testing.T) {
	contextual, err := Build(specFromFixture(t, "valid-popup-frame.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if contextual.Profile != browserauthentication.ContextProfileName {
		t.Fatalf("popup and frame profile = %q", contextual.Profile)
	}
	if len(contextual.Contexts) != 2 {
		t.Fatalf("contexts = %#v", contextual.Contexts)
	}

	base := specFromFixture(t, "valid-push.yaml")
	if requiresContextProfile(base) {
		t.Fatal("unchanged main-only specification requires 1.1")
	}
	cases := map[string]func(Spec) Spec{
		"declared context": func(spec Spec) Spec {
			spec.Contexts = map[string]browserauthentication.Context{
				"idp_popup": {Kind: "popup", Parent: "main", Origin: "https://login.example.test"},
			}
			return spec
		},
		"success path": func(spec Spec) Spec {
			flow := spec.Flows["member_login_push"]
			flow.Success.Path = "/dashboard"
			spec.Flows = map[string]browserauthentication.Flow{"member_login_push": flow}
			return spec
		},
		"navigate object form": func(spec Spec) Spec {
			flow := spec.Flows["member_login_push"]
			sequence := append([]browserauthentication.Step(nil), flow.Sequence...)
			sequence[0] = browserauthentication.Step{
				NavigateTarget: &browserauthentication.NavigateStep{URL: "https://members.example.test/login"},
			}
			flow.Sequence = sequence
			spec.Flows = map[string]browserauthentication.Flow{"member_login_push": flow}
			return spec
		},
		"opens context": func(spec Spec) Spec {
			flow := spec.Flows["member_login_push"]
			sequence := append([]browserauthentication.Step(nil), flow.Sequence...)
			for i, step := range sequence {
				if step.Click != nil {
					click := *step.Click
					click.OpensContext = "idp_popup"
					sequence[i] = browserauthentication.Step{Click: &click}
					break
				}
			}
			flow.Sequence = sequence
			spec.Flows = map[string]browserauthentication.Flow{"member_login_push": flow}
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if !requiresContextProfile(mutate(specFromFixture(t, "valid-push.yaml"))) {
				t.Fatalf("%s did not select authentication 1.1", name)
			}
		})
	}

	undeclared := specFromFixture(t, "valid-push.yaml")
	flow := undeclared.Flows["member_login_push"]
	sequence := append([]browserauthentication.Step(nil), flow.Sequence...)
	for i, step := range sequence {
		if step.Click != nil {
			click := *step.Click
			click.Context = "missing_context"
			sequence[i] = browserauthentication.Step{Click: &click}
			break
		}
	}
	flow.Sequence = sequence
	undeclared.Flows = map[string]browserauthentication.Flow{"member_login_push": flow}
	if _, err := Build(undeclared); err == nil {
		t.Fatal("step referencing an undeclared context was accepted")
	}
}
