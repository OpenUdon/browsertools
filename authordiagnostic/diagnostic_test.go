package authordiagnostic

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateDiagnosticClassesAndCanary(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want Class
	}{
		{nil, Class{"none", "none"}}, {errors.New("TOKEN_CANARY https://secret.invalid/?cookie=SECRET"), Class{"unknown", "unknown"}},
		{context.Canceled, Class{"lifecycle", "cancelled"}}, {context.DeadlineExceeded, Class{"lifecycle", "deadline"}},
		{&Error{Class{"launch", "failed"}}, Class{"launch", "failed"}},
		{&Error{Class{"navigation", "timeout"}}, Class{"navigation", "timeout"}},
		{&Error{Class{"policy", "origin_escape"}}, Class{"policy", "origin_escape"}},
		{&Error{Class{"launch", "TOKEN_CANARY"}}, Class{"unknown", "unknown"}},
	} {
		dir := t.TempDir()
		_ = os.Chmod(dir, 0700)
		path := filepath.Join(dir, "record.json")
		f, err := Reserve(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = Write(f, tc.err); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		got, err := Read(path)
		if err != nil || got != tc.want {
			t.Fatalf("class = %v, %v; want %v", got, err, tc.want)
		}
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "CANARY") || strings.Contains(string(data), "secret.invalid") {
			t.Fatal("private error leaked")
		}
		if _, err := Reserve(path); err == nil {
			t.Fatal("overwrote consumed diagnostic")
		}
	}
}

func TestPrivateDiagnosticRejectsMalformedAndUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	path := filepath.Join(dir, "record.json")
	for _, data := range []string{
		``, `{}`, `null`, `{"version":"browsertools.author-diagnostic.v1","class":{"stage":"none","reason":"CANARY"}}`,
		`{"version":"browsertools.author-diagnostic.v1","class":{"stage":"none","stage":"none","reason":"none"}}`,
		`{"version":"browsertools.author-diagnostic.v1","class":{"stage":"none","reason":"none"},"token":"CANARY"}`,
		`{"version":"browsertools.author-diagnostic.v1","class":{"stage":"none","reason":"none"}} {}`,
		strings.Repeat("x", 1025),
	} {
		_ = os.WriteFile(path, []byte(data), 0600)
		if _, err := Read(path); err == nil {
			t.Fatal("accepted malformed diagnostic")
		}
	}
	_ = os.Remove(path)
	_ = os.Symlink(filepath.Join(dir, "missing"), path)
	if _, err := Read(path); err == nil {
		t.Fatal("read symlink")
	}
	if _, err := Reserve(path); err == nil {
		t.Fatal("reserved symlink")
	}
	_ = os.Remove(path)
	_ = os.Chmod(dir, 0755)
	if _, err := Reserve(path); err == nil {
		t.Fatal("accepted public parent")
	}
}
