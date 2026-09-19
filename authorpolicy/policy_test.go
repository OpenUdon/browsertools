package authorpolicy

import (
	"strings"
	"testing"
)

func TestBlockedScriptPolicyExactOriginAndStrictDefault(t *testing.T) {
	p, err := New("https://ANALYTICS.example.test:443")
	if err != nil || p.Origin() != "https://analytics.example.test" {
		t.Fatal("canonical policy")
	}
	for _, raw := range []string{"https://analytics.example.test/a.js", "https://analytics.example.test:443/a.js?token=secret-token-canary"} {
		if !p.Matches(raw, "GET", false, "script") {
			t.Fatal("expected denial match")
		}
		if (Policy{}).Matches(raw, "GET", false, "script") {
			t.Fatal("default policy changed")
		}
	}
	for _, raw := range []string{
		"http://analytics.example.test/a.js", "https://analytics.example.test:444/a.js", "https://analytics.example.test.evil.test/a.js",
		"https://other.example.test/a.js", "https://secret-token-canary@analytics.example.test/a.js", "https://analytics.example.test:/a.js",
		"https://analytics.example.test/a.js#x", "https://analytics.example.test/a.js?x=%zz", "https://analytics.example.test/%zz",
		"https://analytics.example.test\\evil/a.js", " https://analytics.example.test/a.js", "https://analytics.example.test:0443/a.js",
	} {
		if p.Matches(raw, "GET", false, "script") {
			t.Fatal("invalid URL matched")
		}
	}
	for _, method := range []string{"HEAD", "POST", "OPTIONS", "PUT", "get", ""} {
		if p.Matches("https://analytics.example.test/a.js", method, false, "script") {
			t.Fatal("wrong method matched")
		}
	}
	for _, resource := range []string{"document", "xhr", "fetch", "stylesheet", "image", "unknown", "Script", ""} {
		if p.Matches("https://analytics.example.test/a.js", "GET", false, resource) {
			t.Fatal("wrong resource matched")
		}
	}
	if p.Matches("https://analytics.example.test/a.js", "GET", true, "script") {
		t.Fatal("navigation matched")
	}
}

func TestBlockedScriptPolicyRejectsMalformedConfigurationWithoutValues(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1", "https://example.test/", "https://example.test/path", "https://example.test?", "https://example.test?token=secret-token-canary", "https://secret-token-canary@example.test", "https://example.test#secret-token-canary", "https://*.example.test", "https://example.test:0", "https://example.test:65536", "https://example.test:", "https://example..test", "https://example.test\n"} {
		_, err := New(raw)
		if err == nil || strings.Contains(err.Error(), "secret-token-canary") {
			t.Fatal("invalid configuration or unsafe error")
		}
	}
	if p, err := New(""); err != nil || p.Origin() != "" {
		t.Fatal("zero policy")
	}
}
