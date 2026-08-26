package contenttrust

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/OpenUdon/browsertools/profile"
	uwstrust "github.com/OpenUdon/uws/contenttrust"
	"github.com/OpenUdon/uws/uws1"
)

func loadProfile(t *testing.T, name string) *profile.Profile {
	t.Helper()
	value, err := profile.LoadFile("../testdata/browser-profile/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func browserDocument(action string, request map[string]any, outputs map[string]string) *uws1.Document {
	return &uws1.Document{
		UWS:  "1.9.1",
		Info: &uws1.Info{Title: "Browser content trust", Version: "1.0.0"},
		SourceDescriptions: []*uws1.SourceDescription{{
			Name: "browser", Type: uws1.SourceDescriptionTypeBrowserProfile, URL: "./browser.yaml",
		}},
		Operations: []*uws1.Operation{{
			OperationID: "browser_action", SourceDescription: "browser", SourceOperationID: action,
			Request: request, Outputs: outputs,
		}},
	}
}

func TestResolverUsageAwareChannelsAndBrowserDefaults(t *testing.T) {
	resolver, err := NewResolver(map[string]*profile.Profile{
		"browser": loadProfile(t, "confirmed-side-effect.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	doc := browserDocument("update_record", map[string]any{"body": map[string]any{
		"record_id": "$trigger.record_id",
		"note":      "$trigger.note",
		"priority":  "$trigger.priority",
		"unused":    "$trigger.unused",
	}}, map[string]string{"saved_alias": "$response.body#/saved"})

	owned, contract, err := resolver.ResolveOperation(context.Background(), doc, doc.Operations[0])
	if err != nil {
		t.Fatal(err)
	}
	if !owned {
		t.Fatal("browser operation was not claimed")
	}
	wantInputs := []uwstrust.InputChannel{
		{Path: "/request/body", Kind: uwstrust.ChannelData},
		{Path: "/request/body/note", Kind: uwstrust.ChannelData},
		{Path: "/request/body/priority", Kind: uwstrust.ChannelAuthority},
		{Path: "/request/body/record_id", Kind: uwstrust.ChannelAuthority},
		{Path: "/request/body/record_id", Kind: uwstrust.ChannelInstruction},
	}
	if !reflect.DeepEqual(contract.Inputs, wantInputs) {
		t.Fatalf("inputs = %#v, want %#v", contract.Inputs, wantInputs)
	}
	if contract.DefaultTrust != uws1.ContentTrustUntrusted || contract.InheritsInputProvenance {
		t.Fatalf("browser defaults = %#v", contract)
	}
	if got := contract.Outputs["saved_alias"].Capability; got != uwstrust.CapabilityConstrainedScalar {
		t.Fatalf("saved capability = %q", got)
	}

	report, err := uwstrust.Analyze(context.Background(), doc, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !findingAt(report, uwstrust.CodeUntrustedInstruction, "operations[0].request.body.record_id") {
		t.Fatalf("missing confirmation instruction finding: %#v", report.Findings)
	}
	if !findingAt(report, uwstrust.CodeUntrustedAuthority, "operations[0].request.body.priority") {
		t.Fatalf("missing option authority finding: %#v", report.Findings)
	}
	if findingAt(report, uwstrust.CodeUntrustedInstruction, "operations[0].request.body.note") {
		t.Fatalf("typed text was treated as instruction: %#v", report.Findings)
	}
}

func TestOutputCapabilityClassification(t *testing.T) {
	present := true
	tests := map[string]struct {
		output profile.Output
		want   uwstrust.ValueCapability
	}{
		"free text":   {output: profile.Output{Type: profile.OutputString}, want: uwstrust.CapabilityFreeText},
		"string enum": {output: profile.Output{Type: profile.OutputString, Validation: profile.JSONSchema{"enum": []any{"ready", "busy"}}}, want: uwstrust.CapabilityConstrainedScalar},
		"mixed enum":  {output: profile.Output{Type: profile.OutputString, Validation: profile.JSONSchema{"enum": []any{"ready", 1}}}, want: uwstrust.CapabilityFreeText},
		"integer":     {output: profile.Output{Type: profile.OutputInteger}, want: uwstrust.CapabilityConstrainedScalar},
		"number":      {output: profile.Output{Type: profile.OutputNumber}, want: uwstrust.CapabilityConstrainedScalar},
		"boolean":     {output: profile.Output{Type: profile.OutputBoolean}, want: uwstrust.CapabilityConstrainedScalar},
		"null":        {output: profile.Output{Type: profile.OutputNull}, want: uwstrust.CapabilityConstrainedScalar},
		"presence":    {output: profile.Output{Type: profile.OutputBoolean, Presence: &present}, want: uwstrust.CapabilityConstrainedScalar},
		"array":       {output: profile.Output{Type: profile.OutputArray}, want: uwstrust.CapabilityComposite},
		"object":      {output: profile.Output{Type: profile.OutputObject}, want: uwstrust.CapabilityComposite},
		"unknown":     {output: profile.Output{Type: profile.OutputType("future")}, want: uwstrust.CapabilityUnknown},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := outputCapability(test.output); got != test.want {
				t.Fatalf("capability = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolverSelectionFailuresCancellationAndSnapshot(t *testing.T) {
	original := loadProfile(t, "confirmed-side-effect.yaml")
	resolver, err := NewResolver(map[string]*profile.Profile{"browser": original})
	if err != nil {
		t.Fatal(err)
	}
	action := original.Actions["update_record"]
	delete(action.Outputs, "saved")
	original.Actions["update_record"] = action

	doc := browserDocument("update_record", nil, map[string]string{"saved": "$response.body#/saved"})
	owned, contract, err := resolver.ResolveOperation(context.Background(), doc, doc.Operations[0])
	if err != nil || !owned || contract.Outputs["saved"].Capability != uwstrust.CapabilityConstrainedScalar {
		t.Fatalf("resolver snapshot changed with caller profile: owned=%v contract=%#v err=%v", owned, contract, err)
	}

	refDoc := browserDocument("", nil, map[string]string{"saved": "$response.body#/saved"})
	refDoc.Operations[0].SourceOperationID = ""
	refDoc.Operations[0].SourceOperationRef = "#/actions/update_record"
	if owned, _, err := resolver.ResolveOperation(context.Background(), refDoc, refDoc.Operations[0]); err != nil || !owned {
		t.Fatalf("action reference resolution = owned %v, err %v", owned, err)
	}

	missingAction := browserDocument("missing", nil, nil)
	if owned, _, err := resolver.ResolveOperation(context.Background(), missingAction, missingAction.Operations[0]); !owned || err == nil {
		t.Fatalf("missing action = owned %v, err %v", owned, err)
	}
	unknownOutput := browserDocument("update_record", nil, map[string]string{
		"missing": "$response.body#/missing",
		"whole":   "$response.body",
		"dot":     "$response.body.saved",
	})
	if owned, contract, err := resolver.ResolveOperation(context.Background(), unknownOutput, unknownOutput.Operations[0]); err != nil || !owned {
		t.Fatalf("unknown output = owned %v, err %v", owned, err)
	} else {
		if _, ok := contract.Outputs["missing"]; ok {
			t.Fatalf("unknown browser output received a shape contract: %#v", contract.Outputs)
		}
		if got := contract.Outputs["whole"].Capability; got != uwstrust.CapabilityComposite {
			t.Fatalf("whole response capability = %q", got)
		}
		if got := contract.Outputs["dot"].Capability; got != uwstrust.CapabilityConstrainedScalar {
			t.Fatalf("dot-selected output capability = %q", got)
		}
	}
	missingProfile, err := NewResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	if owned, _, err := missingProfile.ResolveOperation(context.Background(), doc, doc.Operations[0]); !owned || err == nil {
		t.Fatalf("missing profile = owned %v, err %v", owned, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if owned, _, err := resolver.ResolveOperation(canceled, doc, doc.Operations[0]); owned || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resolution = owned %v, err %v", owned, err)
	}
}

func TestResolverRejectsUndeclaredPlaceholdersAndInvalidProfiles(t *testing.T) {
	value := loadProfile(t, "confirmed-side-effect.yaml")
	action := value.Actions["update_record"]
	action.ConfirmationPolicy.Prompt = "Approve {{undeclared}}?"
	value.Actions["update_record"] = action
	resolver, err := NewResolver(map[string]*profile.Profile{"browser": value})
	if err != nil {
		t.Fatal(err)
	}
	doc := browserDocument("update_record", map[string]any{"body": map[string]any{"undeclared": "$trigger.value"}}, nil)
	if owned, _, err := resolver.ResolveOperation(context.Background(), doc, doc.Operations[0]); !owned || err == nil {
		t.Fatalf("undeclared placeholder = owned %v, err %v", owned, err)
	}

	if _, err := NewResolver(map[string]*profile.Profile{"browser": nil}); err == nil {
		t.Fatal("nil profile was accepted")
	}
	if _, err := NewResolver(map[string]*profile.Profile{"": loadProfile(t, "read-only.yaml")}); err == nil {
		t.Fatal("empty source name was accepted")
	}
}

func TestPointerTokenCodec(t *testing.T) {
	encoded := encodePointerToken("read/status~v1")
	if encoded != "read~1status~0v1" {
		t.Fatalf("encoded token = %q", encoded)
	}
	decoded, ok := decodePointerToken(encoded)
	if !ok || decoded != "read/status~v1" {
		t.Fatalf("decoded token = %q, %v", decoded, ok)
	}
	for _, malformed := range []string{"bad~", "bad~2escape"} {
		if _, ok := decodePointerToken(malformed); ok {
			t.Fatalf("malformed token %q was accepted", malformed)
		}
	}
}

func findingAt(report *uwstrust.Report, code, path string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code && finding.Path == path {
			return true
		}
	}
	return false
}
