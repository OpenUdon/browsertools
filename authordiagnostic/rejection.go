package authordiagnostic

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
)

const VersionV2 = "browsertools.author-diagnostic.v2"

// Rejection contains only closed classifications. Resource describes the CDP
// resource type, not frame identity or proof of where a request originated.
type Rejection struct {
	Boundary       string `json:"boundary"`
	Resource       string `json:"resource"`
	OriginRelation string `json:"origin_relation"`
}

func NoRejection() Rejection      { return Rejection{"none", "none", "none"} }
func UnknownRejection() Rejection { return Rejection{"unknown", "unknown", "unknown"} }

func (r Rejection) ValidFor(c Class) bool {
	if !c.Valid() {
		return false
	}
	if c != (Class{"policy", "origin_escape"}) {
		return r == NoRejection()
	}
	if r == UnknownRejection() {
		return true
	}
	if r.Boundary != "request" && r.Boundary != "observation" {
		return false
	}
	if r.Boundary == "observation" && r.Resource != "document" {
		return false
	}
	switch r.Resource {
	case "document", "stylesheet", "image", "media", "font", "script", "xhr", "fetch", "event_source", "manifest", "text_track", "ping", "prefetch", "other", "unknown":
	default:
		return false
	}
	switch r.OriginRelation {
	case "unknown", "invalid_url", "userinfo", "missing_host", "local_scheme", "unsupported_scheme", "scheme_mismatch", "port_mismatch", "scheme_and_port_mismatch", "host_mismatch":
		return true
	}
	return false
}

// RejectionError intentionally cannot carry the URL or original error text.
type RejectionError struct{ Rejection }

func (*RejectionError) Error() string { return "author origin rejected" }
func (*RejectionError) Unwrap() error { return &Error{Class{"policy", "origin_escape"}} }

func RejectionOf(err error) Rejection {
	c := Classify(err)
	if c != (Class{"policy", "origin_escape"}) {
		return NoRejection()
	}
	var detail *RejectionError
	if errors.As(err, &detail) && detail != nil && detail.Rejection.ValidFor(c) {
		return detail.Rejection
	}
	return UnknownRejection()
}

type RecordV2 struct {
	Version   string    `json:"version"`
	Class     Class     `json:"class"`
	Rejection Rejection `json:"rejection"`
}

func WriteV2(f *os.File, err error) error {
	return json.NewEncoder(f).Encode(RecordV2{VersionV2, Classify(err), RejectionOf(err)})
}

// ReadV2 is separate from the unchanged, strict v1 reader.
func ReadV2(path string) (RecordV2, error) {
	bad := errors.New("invalid private worker diagnostic")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 2048 {
		return RecordV2{}, bad
	}
	f, err := os.Open(path)
	if err != nil {
		return RecordV2{}, bad
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return RecordV2{}, bad
	}
	data, err := io.ReadAll(io.LimitReader(f, 2049))
	if err != nil || len(data) > 2048 {
		return RecordV2{}, bad
	}
	var r RecordV2
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&r) != nil || dec.Decode(new(any)) != io.EOF || r.Version != VersionV2 || !r.Rejection.ValidFor(r.Class) {
		return RecordV2{}, bad
	}
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(bytes.TrimSpace(data), canonical) {
		return RecordV2{}, bad
	}
	return r, nil
}
