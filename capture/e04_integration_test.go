package capture

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/OpenUdon/browsertools/profile"
)

func TestPlaywrightRichCaptureLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_RICH_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_RICH_LIVE_TEST=1 with the pinned driver and Chromium installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><h1>Private rich fixture</h1></main>`))
	}))
	defer server.Close()
	origin, err := profile.OriginOfURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := AcquireRich(context.Background(), NewPlaywrightRichAcquirer(os.Getenv("PLAYWRIGHT_DRIVER_PATH")), RichRequest{
		Capture:   LiveRequest{URL: server.URL, AllowedOrigins: []string{origin}, ObservedAt: time.Now().UTC()},
		Artifacts: []PrivateArtifactKind{PrivateArtifactScreenshot, PrivateArtifactTrace, PrivateArtifactHAR},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 3 || !bytes.HasPrefix(result.Artifacts[0].Bytes, []byte("\x89PNG")) ||
		!bytes.HasPrefix(result.Artifacts[1].Bytes, []byte("PK")) || !bytes.Contains(result.Artifacts[2].Bytes, []byte(`"log"`)) {
		t.Fatalf("unexpected rich artifacts: %#v", result.Artifacts)
	}
	if _, _, err := MarshalRichBundle(result, EngineChromium, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}

func TestPlaywrightPortabilityLoopbackOptIn(t *testing.T) {
	if os.Getenv("BROWSERTOOLS_PORTABILITY_LIVE_TEST") != "1" {
		t.Skip("set BROWSERTOOLS_PORTABILITY_LIVE_TEST=1 with pinned Chromium, Firefox, and WebKit installed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<main><button>Refresh</button><script type="application/ld+json">{"status":"active"}</script></main>`))
	}))
	defer server.Close()
	origin, err := profile.OriginOfURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	prof := validCheckProfile()
	prof.Info.Origin = profile.Origins{origin}
	report, err := ComparePortability(context.Background(), func(engine Engine) Acquirer {
		return NewPlaywrightEngineAcquirer(os.Getenv("PLAYWRIGHT_DRIVER_PATH"), engine)
	}, []Engine{EngineChromium, EngineFirefox, EngineWebKit}, LiveCheckRequest{
		Profile: prof, Actions: []string{"read_status"},
		Capture: LiveRequest{URL: server.URL, AllowedOrigins: []string{origin}, ObservedAt: time.Now().UTC()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK {
		t.Fatalf("portability report = %#v", report)
	}
}
