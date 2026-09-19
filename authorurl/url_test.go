package authorurl

import (
	"strings"
	"testing"
)

func TestReviewedNavigationRetainsOnlyBoundedStructuralQueries(t *testing.T) {
	for _, raw := range []string{"https://example.test/campaign?action=topics", "http://127.0.0.1:3210/login?next=dashboard", "https://example.test/", "https://example.test/?lang=en&page=2"} {
		value, origin, err := Normalize(raw)
		if err != nil || value != raw || strings.Contains(origin, "?") {
			t.Fatalf("reviewed URL did not survive: %q %q %v", value, origin, err)
		}
	}
	if value, origin, err := Normalize(" https://EXAMPLE.test:443?view=home "); err != nil || value != "https://example.test/?view=home" || origin != "https://example.test" {
		t.Fatalf("canonical origin: %q %q %v", value, origin, err)
	}
	for _, suffix := range []string{
		"?token=TOKEN_CANARY", "?code=TOKEN_CANARY", "?state=TOKEN_CANARY", "?password=TOKEN_CANARY", "?private=TOKEN_CANARY", "?email=fixture%40example.test",
		"?action=topics&action=other", "?action=%ZZ", "?action=%74opics", "?z=last&a=first", "?action", "?", "#", "#private",
		"?action=${TOKEN_CANARY}", "?action=%7B%7BTOKEN_CANARY%7D%7D", "?action=first%0Asecond", "?action=" + strings.Repeat("x", 257),
		"?action=fixture%40example.test", "?action=one;other=two",
	} {
		value, origin, err := Normalize("https://example.test/campaign" + suffix)
		if err == nil || value != "" || origin != "" || strings.Contains(err.Error(), "TOKEN_CANARY") {
			t.Fatalf("unsafe query accepted or disclosed: %q", suffix)
		}
	}
	for _, raw := range []string{"http://example.test/campaign?action=topics", "https://user:pass@example.test/", "https://example.test/../private?action=topics", "https://example.test/" + strings.Repeat("a", 4096)} {
		if _, _, err := Normalize(raw); err == nil {
			t.Fatal("unsafe navigation accepted")
		}
	}
}
