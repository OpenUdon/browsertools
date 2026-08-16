package capture

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAcquireRichNormalizesAndClonesPrivateArtifacts(t *testing.T) {
	backend := &fakeRichAcquirer{observation: RichObservation{
		FinalURL: "https://example.test/member",
		Network:  validObservation().Network,
		Artifacts: []PrivateArtifact{
			{Kind: PrivateArtifactScreenshot, MediaType: "image/png", Bytes: []byte("png")},
			{Kind: PrivateArtifactHAR, MediaType: "application/json", Bytes: []byte("{}")},
		},
	}}
	live := validLiveRequest()
	live.ActionHint = ""
	request := RichRequest{Capture: live, Artifacts: []PrivateArtifactKind{PrivateArtifactHAR, PrivateArtifactScreenshot}}
	result, err := AcquireRich(context.Background(), backend, request)
	if err != nil {
		t.Fatal(err)
	}
	if backend.calls != 1 || result.Origin != "https://example.test" || len(result.Artifacts) != 2 ||
		backend.request.Artifacts[0] != PrivateArtifactScreenshot || backend.request.MaxArtifactBytes != DefaultMaxRichArtifactBytes {
		t.Fatalf("request=%#v result=%#v", backend.request, result)
	}
	backend.observation.Artifacts[0].Bytes[0] = 'x'
	if string(result.Artifacts[0].Bytes) != "png" {
		t.Fatalf("artifact bytes alias backend: %q", result.Artifacts[0].Bytes)
	}
}

func TestMarshalRichBundleIsDeterministicBoundAndSelfDescribing(t *testing.T) {
	result := RichResult{Origin: "https://example.test", Artifacts: []PrivateArtifact{
		{Kind: PrivateArtifactScreenshot, MediaType: "image/png", Bytes: []byte("png")},
		{Kind: PrivateArtifactTrace, MediaType: "application/zip", Bytes: []byte("trace")},
	}}
	at := time.Date(2026, 8, 16, 12, 0, 0, 123, time.UTC)
	first, manifest, err := MarshalRichBundle(result, EngineChromium, at)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := MarshalRichBundle(result, EngineChromium, at)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("bundle not deterministic: %v", err)
	}
	if manifest.Version != RichBundleVersion || len(manifest.Artifacts) != 2 || manifest.Artifacts[0].Name != "screenshot.png" {
		t.Fatalf("manifest = %#v", manifest)
	}
	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 3 || reader.File[0].Name != "manifest.json" || reader.File[1].Name != "screenshot.png" || reader.File[2].Name != "trace.zip" {
		t.Fatalf("zip files = %#v", reader.File)
	}
	file, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(file)
	_ = file.Close()
	var decoded RichBundleManifest
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.CapturedAt != "2026-08-16T12:00:00.000000123Z" {
		t.Fatalf("decoded=%#v err=%v", decoded, err)
	}
}

func TestAcquireRichRejectsImplicitMalformedAndOversizedArtifacts(t *testing.T) {
	valid := validLiveRequest()
	valid.ActionHint = ""
	backend := &fakeRichAcquirer{}
	for name, request := range map[string]RichRequest{
		"none":            {Capture: valid},
		"duplicate":       {Capture: valid, Artifacts: []PrivateArtifactKind{PrivateArtifactHAR, PrivateArtifactHAR}},
		"unknown":         {Capture: valid, Artifacts: []PrivateArtifactKind{"video"}},
		"oversized limit": {Capture: valid, Artifacts: []PrivateArtifactKind{PrivateArtifactTrace}, MaxArtifactBytes: MaxRichArtifactBytes + 1},
	} {
		if _, err := AcquireRich(context.Background(), backend, request); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if backend.calls != 0 {
		t.Fatalf("backend called %d times", backend.calls)
	}

	backend.observation = RichObservation{FinalURL: "https://example.test/member", Network: validObservation().Network,
		Artifacts: []PrivateArtifact{{Kind: PrivateArtifactScreenshot, MediaType: "text/plain", Bytes: []byte("secret")}}}
	if _, err := AcquireRich(context.Background(), backend, RichRequest{Capture: valid, Artifacts: []PrivateArtifactKind{PrivateArtifactScreenshot}}); err == nil || !strings.Contains(err.Error(), "media type") {
		t.Fatalf("malformed backend error = %v", err)
	}
}

type fakeRichAcquirer struct {
	calls       int
	request     RichBackendRequest
	observation RichObservation
	err         error
}

func (a *fakeRichAcquirer) AcquireRich(_ context.Context, request RichBackendRequest) (RichObservation, error) {
	a.calls++
	a.request = request
	return a.observation, a.err
}
