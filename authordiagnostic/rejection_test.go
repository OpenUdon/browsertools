package authordiagnostic

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectionV2RoundTripPrivacyAndLegacyIsolation(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	for i, failure := range []error{nil, errors.New("TOKEN_CANARY"), &RejectionError{Rejection{"request", "script", "host_mismatch"}}, &Error{Class{"policy", "origin_escape"}}, &RejectionError{Rejection{"request", "TOKEN_CANARY", "host_mismatch"}}} {
		path := filepath.Join(dir, string(rune('a'+i)))
		f, err := Reserve(path)
		if err != nil {
			t.Fatal(err)
		}
		if WriteV2(f, failure) != nil {
			t.Fatal("write")
		}
		_ = f.Close()
		r, err := ReadV2(path)
		if err != nil || r.Class != Classify(failure) || r.Rejection != RejectionOf(failure) {
			t.Fatal(r, err)
		}
		if _, err := Read(path); err == nil {
			t.Fatal("v1 reader accepted v2")
		}
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "CANARY") {
			t.Fatal("unbounded metadata leaked")
		}
	}
	path := filepath.Join(dir, "legacy")
	f, _ := Reserve(path)
	_ = Write(f, &Error{Class{"policy", "origin_escape"}})
	_ = f.Close()
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadV2(path); err == nil {
		t.Fatal("v2 reader silently accepted v1")
	}
}

func TestRejectionV2RejectsMalformedCrossFieldAndCanary(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, "record")
	base := RecordV2{VersionV2, Class{"policy", "origin_escape"}, Rejection{"request", "document", "host_mismatch"}}
	for _, mutate := range []func(*RecordV2){
		func(r *RecordV2) { r.Rejection.Boundary = "TOKEN_CANARY" },
		func(r *RecordV2) { r.Rejection.Resource = "https://TOKEN_CANARY" },
		func(r *RecordV2) { r.Rejection.OriginRelation = "TOKEN_CANARY" },
		func(r *RecordV2) { r.Class = Class{"none", "none"} },
		func(r *RecordV2) { r.Rejection = NoRejection() },
		func(r *RecordV2) { r.Rejection.Boundary = "observation"; r.Rejection.Resource = "script" },
	} {
		r := base
		mutate(&r)
		data, _ := json.Marshal(r)
		_ = os.WriteFile(path, data, 0600)
		if _, err := ReadV2(path); err == nil {
			t.Fatal("accepted malformed cross-field metadata")
		}
	}
	data, _ := json.Marshal(base)
	for _, bad := range []string{strings.Replace(string(data), `"boundary":"request"`, `"boundary":"request","boundary":"request"`, 1), strings.TrimSuffix(string(data), "}") + `,"token":"TOKEN_CANARY"}`, strings.Replace(string(data), `"resource":"document"`, `"resource":null`, 1), string(data) + " {}", strings.Repeat("x", 2049)} {
		_ = os.WriteFile(path, []byte(bad), 0600)
		if _, err := ReadV2(path); err == nil {
			t.Fatal("accepted invalid bytes")
		}
	}
}
