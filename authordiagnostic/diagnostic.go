// Package authordiagnostic carries closed private worker failure metadata. It
// is independent of the public author-session protocol and contains no errors,
// URLs, page text, credentials, or browser state.
package authordiagnostic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const Version = "browsertools.author-diagnostic.v1"

type Class struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
}

func (c Class) Valid() bool {
	switch c.Stage {
	case "none":
		return c.Reason == "none"
	case "unknown":
		return c.Reason == "unknown"
	case "driver", "launch", "context", "page", "guard", "observation":
		return c.Reason == "failed" || c.Reason == "timeout"
	case "navigation":
		return c.Reason == "transport" || c.Reason == "timeout" || c.Reason == "missing_response" || c.Reason == "http_status" || c.Reason == "service_worker"
	case "policy":
		switch c.Reason {
		case "origin_escape", "unexpected_navigation", "post_budget", "method", "request_limit", "response_limit", "unexpected_popup", "dialog", "download", "file_chooser", "page_crash", "page_close", "context_close", "route_continue", "websocket", "response_header", "response_size":
			return true
		}
	case "lifecycle":
		return c.Reason == "cancelled" || c.Reason == "deadline"
	}
	return false
}

// Error deliberately has no raw cause. Classification never parses error text.
type Error struct{ Class }

func (e *Error) Error() string { return "author backend failed" }

func Classify(err error) Class {
	if err == nil {
		return Class{"none", "none"}
	}
	var bounded *Error
	if errors.As(err, &bounded) && bounded != nil && bounded.Class.Valid() {
		return bounded.Class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Class{"lifecycle", "deadline"}
	}
	if errors.Is(err, context.Canceled) {
		return Class{"lifecycle", "cancelled"}
	}
	return Class{"unknown", "unknown"}
}

type Record struct {
	Version string `json:"version"`
	Class   Class  `json:"class"`
}

// Reserve requires an existing private, non-symlink parent and an unused path.
// A reserved but unwritten file is incomplete evidence, never success.
func Reserve(path string) (*os.File, error) {
	bad := errors.New("private diagnostic path invalid")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, bad
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return nil, bad
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return nil, bad
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, bad
	}
	return f, nil
}

func Write(f *os.File, err error) error {
	return json.NewEncoder(f).Encode(Record{Version, Classify(err)})
}

func Read(path string) (Class, error) {
	bad := errors.New("invalid private worker diagnostic")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 1024 {
		return Class{}, bad
	}
	f, err := os.Open(path)
	if err != nil {
		return Class{}, bad
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return Class{}, bad
	}
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(data) > 1024 {
		return Class{}, bad
	}
	var r Record
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil || dec.Decode(new(any)) != io.EOF || r.Version != Version || !r.Class.Valid() {
		return Class{}, bad
	}
	// Canonical writer bytes also reject duplicate, missing and null fields.
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(bytes.TrimSpace(data), canonical) {
		return Class{}, bad
	}
	return r.Class, nil
}
