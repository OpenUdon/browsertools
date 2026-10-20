package authordiagnostic

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type panickingDiagnosticError struct{}

func (panickingDiagnosticError) Error() string { panic("synthetic formatter") }

func TestStartupPrivateErrorCaptureBoundsAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		err                        error
		length                     int
		truncated, missing, failed bool
	}{
		{"nil", nil, 0, false, true, false},
		{"empty", errors.New(""), 0, false, true, false},
		{"ordinary", errors.New("SYNTHETIC_ORIGINAL"), 18, false, false, false},
		{"exact", errors.New(strings.Repeat("e", StartupErrorLimit)), StartupErrorLimit, false, false, false},
		{"overflow", errors.New(strings.Repeat("e", StartupErrorLimit+99)), StartupErrorLimit, true, false, false},
		{"formatter panic", panickingDiagnosticError{}, 0, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CaptureErrorText(tc.err)
			if len(got.Text) != tc.length || got.Truncated != tc.truncated || got.Missing != tc.missing || got.CaptureError != tc.failed {
				t.Fatalf("capture flags/length: %+v length=%d", got, len(got.Text))
			}
		})
	}
}

func TestStartupSinkNeverWaitsOrClosesCallerChannel(t *testing.T) {
	f := StartupFailure{Operation: "launch", Class: Class{"launch", "failed"}, Rejection: NoRejection()}
	if !f.Valid() {
		t.Fatal("invalid fixture")
	}
	full := make(chan StartupFailure, 1)
	if !OfferStartup(full, f) || OfferStartup(full, f) {
		t.Fatal("buffer handling")
	}
	if got := <-full; got.Operation != f.Operation {
		t.Fatal("changed snapshot")
	}
	closed := make(chan StartupFailure, 1)
	close(closed)
	for _, sink := range []chan StartupFailure{nil, make(chan StartupFailure), closed} {
		if OfferStartup(sink, f) {
			t.Fatal("unavailable sink delivered")
		}
	}
	if !OfferStartup(full, f) {
		t.Fatal("sender closed caller channel")
	}
}

func TestStartupStderrRingKeepsTailAndDrainsOverflow(t *testing.T) {
	var ring StderrRing
	if got := ring.Snapshot(); !got.Missing || got.Truncated {
		t.Fatal(got)
	}
	a := strings.Repeat("a", StartupStderrLimit-2)
	for _, text := range []string{a, "BCDE", strings.Repeat("z", StartupStderrLimit+7), "tail"} {
		if n, err := ring.Write([]byte(text)); n != len(text) || err != nil {
			t.Fatal(n, err)
		}
	}
	got := ring.Snapshot()
	if got.Text != strings.Repeat("z", StartupStderrLimit-4)+"tail" || !got.Truncated || got.Missing || got.CaptureError {
		t.Fatal("tail/overflow incorrect")
	}
	var wrapped StderrRing
	_, _ = wrapped.Write([]byte(strings.Repeat("a", StartupStderrLimit-2)))
	_, _ = wrapped.Write([]byte("BCDE"))
	if value := wrapped.Snapshot(); value.Text != strings.Repeat("a", StartupStderrLimit-4)+"BCDE" || !value.Truncated {
		t.Fatal("wrapped tail incorrect")
	}
	var exact StderrRing
	_, _ = exact.Write([]byte(strings.Repeat("x", StartupStderrLimit)))
	if exact.Snapshot().Truncated {
		t.Fatal("exact capacity marked overflow")
	}
}

func TestStartupStderrRingConcurrentWritersAndSnapshots(t *testing.T) {
	var ring StderrRing
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				if n, err := ring.Write([]byte(strings.Repeat("s", 100))); n != 100 || err != nil {
					t.Error("write")
				}
				if len(ring.Snapshot().Text) > StartupStderrLimit {
					t.Error("cap")
				}
			}
		}()
	}
	wg.Wait()
	got := ring.Snapshot()
	if got.Text != strings.Repeat("s", StartupStderrLimit) || !got.Truncated {
		t.Fatal("concurrent tail")
	}
}
