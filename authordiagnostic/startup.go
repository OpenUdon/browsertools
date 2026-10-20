package authordiagnostic

import (
	"sync"
)

const (
	StartupErrorLimit  = 16 * 1024
	StartupStderrLimit = 64 * 1024
)

// PrivateCapture is synthetic-only, potentially sensitive data. Missing means no
// bytes were captured; CaptureError means capture failed, not operation success.
type PrivateCapture struct {
	Text         string
	Truncated    bool
	Missing      bool
	CaptureError bool
}

// StartupTypes contains positive typed evidence only, never error-text guesses.
type StartupTypes struct {
	Timeout      bool
	TargetClosed bool
	Canceled     bool
	Deadline     bool
}

// StartupFailure is an opt-in private snapshot, separate from native messages
// and the unchanged v1/v2 diagnostic files. It never retains an error object.
type StartupFailure struct {
	Operation    string
	Class        Class
	Rejection    Rejection
	Types        StartupTypes
	RawError     PrivateCapture
	DriverStderr PrivateCapture
}

func (f StartupFailure) Valid() bool {
	switch f.Operation {
	case "validate", "preflight", "driver", "launch", "context", "permissions", "guard", "page", "timeout", "navigation", "navigation_response":
	default:
		return false
	}
	return f.Class.Valid() && f.Rejection.ValidFor(f.Class) && len(f.RawError.Text) <= StartupErrorLimit && len(f.DriverStderr.Text) <= StartupStderrLimit
}

// CaptureErrorText must only receive trusted backend errors whose Error method
// returns promptly. A panicking formatter is marked incomplete. No raw error is
// retained or exposed through an Unwrap method.
func CaptureErrorText(err error) (capture PrivateCapture) {
	capture.Missing = true
	if err == nil {
		return
	}
	defer func() {
		if recover() != nil {
			capture = PrivateCapture{Missing: true, CaptureError: true}
		}
	}()
	value := err.Error()
	capture.Missing = len(value) == 0
	capture.Truncated = len(value) > StartupErrorLimit
	if capture.Truncated {
		value = value[:StartupErrorLimit]
	}
	// Copy so the snapshot does not retain an oversized underlying string.
	capture.Text = string(append([]byte(nil), value...))
	return
}

// OfferStartup performs one nonblocking handoff. A nil, full, unreceived or
// closed channel drops the snapshot and cannot replace an operation's error.
// The sender never closes the caller's channel and never starts a goroutine.
// Callers must treat missing delivery as incomplete evidence.
func OfferStartup(sink chan<- StartupFailure, failure StartupFailure) (delivered bool) {
	if sink == nil {
		return false
	}
	// Only this send is recovered: closing a caller-owned sink must not panic an
	// otherwise settling operation. This does not recover operation panics.
	defer func() {
		if recover() != nil {
			delivered = false
		}
	}()
	select {
	case sink <- failure:
		return true
	default:
		return false
	}
}

// StderrRing drains all writes, retaining only the latest fixed number of bytes.
// Its zero value is ready for use; writers never invoke caller code.
type StderrRing struct {
	mu        sync.Mutex
	data      [StartupStderrLimit]byte
	used      int
	next      int
	truncated bool
}

func (r *StderrRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n > StartupStderrLimit-r.used {
		r.truncated = true
	}
	if n >= StartupStderrLimit {
		copy(r.data[:], p[n-StartupStderrLimit:])
		r.used, r.next = StartupStderrLimit, 0
		return n, nil
	}
	first := min(n, StartupStderrLimit-r.next)
	copy(r.data[r.next:], p[:first])
	copy(r.data[:], p[first:])
	r.next = (r.next + n) % StartupStderrLimit
	r.used = min(StartupStderrLimit, r.used+n)
	return n, nil
}

func (r *StderrRing) Snapshot() PrivateCapture {
	if r == nil {
		return PrivateCapture{Missing: true}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value := make([]byte, r.used)
	start := (r.next - r.used + StartupStderrLimit) % StartupStderrLimit
	first := min(r.used, StartupStderrLimit-start)
	copy(value, r.data[start:start+first])
	copy(value[first:], r.data[:r.used-first])
	return PrivateCapture{Text: string(value), Truncated: r.truncated, Missing: r.used == 0}
}
