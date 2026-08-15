package capture

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeRuntime struct {
	session Session
	err     error
	engine  Engine
}

func (f *fakeRuntime) Open(_ context.Context, engine Engine) (Session, error) {
	f.engine = engine
	return f.session, f.err
}

type fakeSession struct {
	executable string
	closed     bool
	err        error
}

func (f *fakeSession) BrowserExecutable() string { return f.executable }
func (f *fakeSession) Close() error {
	f.closed = true
	return f.err
}

func TestDoctorReportsInstalledChromiumAndCloses(t *testing.T) {
	executable := t.TempDir() + "/chromium"
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	session := &fakeSession{executable: executable}
	runtime := &fakeRuntime{session: session}
	report, err := Doctor(context.Background(), runtime, EngineChromium)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DriverReady || !report.BrowserReady || report.BrowserExecutable != session.executable {
		t.Fatalf("report = %#v", report)
	}
	if runtime.engine != EngineChromium || !session.closed {
		t.Fatalf("engine=%q closed=%v", runtime.engine, session.closed)
	}
	if report.PlaywrightGoVersion != PlaywrightGoVersion || report.PlaywrightVersion != PlaywrightVersion {
		t.Fatalf("unexpected versions: %#v", report)
	}
	if len(report.CapabilityPolicy) != len(CapabilityMatrix()) {
		t.Fatalf("capability policy = %#v", report.CapabilityPolicy)
	}
}

func TestDoctorFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		runtime Runtime
		engine  Engine
		want    string
	}{
		{name: "missing runtime", ctx: context.Background(), engine: EngineChromium, want: "runtime is required"},
		{name: "unknown engine", ctx: context.Background(), runtime: &fakeRuntime{}, engine: "chrome", want: "engine must be"},
		{name: "driver", ctx: context.Background(), runtime: &fakeRuntime{err: errors.New("missing")}, engine: EngineChromium, want: "driver is unavailable"},
		{name: "nil session", ctx: context.Background(), runtime: &fakeRuntime{}, engine: EngineChromium, want: "no session"},
		{name: "browser", ctx: context.Background(), runtime: &fakeRuntime{session: &fakeSession{}}, engine: EngineChromium, want: "browser executable is unavailable"},
		{name: "missing browser path", ctx: context.Background(), runtime: &fakeRuntime{session: &fakeSession{executable: t.TempDir() + "/missing"}}, engine: EngineChromium, want: "unavailable at"},
		{name: "browser path is directory", ctx: context.Background(), runtime: &fakeRuntime{session: &fakeSession{executable: t.TempDir()}}, engine: EngineChromium, want: "not a regular file"},
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, struct {
		name    string
		ctx     context.Context
		runtime Runtime
		engine  Engine
		want    string
	}{name: "cancelled", ctx: cancelled, runtime: &fakeRuntime{}, engine: EngineChromium, want: "context canceled"})
	if runtime.GOOS != "windows" {
		nonExecutable := t.TempDir() + "/chromium"
		if err := os.WriteFile(nonExecutable, []byte("not executable"), 0o600); err != nil {
			t.Fatal(err)
		}
		tests = append(tests, struct {
			name    string
			ctx     context.Context
			runtime Runtime
			engine  Engine
			want    string
		}{name: "browser path is not executable", ctx: context.Background(), runtime: &fakeRuntime{session: &fakeSession{executable: nonExecutable}}, engine: EngineChromium, want: "not executable"})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := Doctor(test.ctx, test.runtime, test.engine)
			if err == nil || report.Error == "" || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("report=%#v err=%v", report, err)
			}
		})
	}
}

func TestDoctorClosesSessionWhenBrowserIsUnavailable(t *testing.T) {
	session := &fakeSession{executable: t.TempDir() + "/missing"}
	report, err := Doctor(context.Background(), &fakeRuntime{session: session}, EngineChromium)
	if err == nil || report.BrowserReady || !session.closed {
		t.Fatalf("report=%#v err=%v closed=%v", report, err, session.closed)
	}
}

func TestDoctorReportsDriverShutdownFailure(t *testing.T) {
	executable := t.TempDir() + "/chromium"
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	report, err := Doctor(context.Background(), &fakeRuntime{session: &fakeSession{
		executable: executable,
		err:        errors.New("shutdown failed"),
	}}, EngineChromium)
	if err == nil || !strings.Contains(err.Error(), "stop playwright driver") || report.Error == "" {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}

func TestCapabilityMatrixStableAndComplete(t *testing.T) {
	want := []CapabilityDisposition{CapabilityAdopted, CapabilityPrivate, CapabilityDeferred, CapabilityExcluded}
	seen := map[CapabilityDisposition]bool{}
	names := map[string]bool{}
	for _, capability := range CapabilityMatrix() {
		if capability.Name == "" || capability.Reason == "" || names[capability.Name] {
			t.Fatalf("invalid capability: %#v", capability)
		}
		names[capability.Name] = true
		seen[capability.Disposition] = true
	}
	got := make([]CapabilityDisposition, 0, len(want))
	for _, disposition := range want {
		if seen[disposition] {
			got = append(got, disposition)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dispositions = %v, want %v", got, want)
	}
}
