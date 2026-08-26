package registrationurl

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestParseRetainedStructuralQuery(t *testing.T) {
	facts, err := Parse("https://www.example.test/register?action=startnew", true, testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if facts.URL != "https://www.example.test/register?action=startnew" || facts.Origin != "https://www.example.test" || facts.Path != "/register" {
		t.Fatalf("facts = %#v", facts)
	}
	if strings.Contains(facts.Origin, "?") || strings.Contains(facts.Path, "?") {
		t.Fatalf("safe facts exposed query: %#v", facts)
	}
}

func TestParseRejectsUnsafeOrNoncanonicalQueriesWithoutEcho(t *testing.T) {
	tests := []string{
		"https://www.example.test/register?",
		"https://www.example.test/register?action",
		"https://www.example.test/register?=startnew",
		"https://www.example.test/register?action=startnew&action=again",
		"https://www.example.test/register?token=private",
		"https://www.example.test/register?action=sk-proj-abcdefghijklmnopqrstuvwxyz",
		"https://www.example.test/register?action=secret",
		"https://www.example.test/register?action=%7B%7Bflow%7D%7D",
		"https://www.example.test/%7B%7Bflow%7D%7D?action=startnew",
		"https://www.example.test/register?%D1%82oken=value",
		"https://www.example.test/register?b=2&a=1",
		"https://www.example.test/register?action=start%20new",
		"https://www.example.test/register?action=%ZZ",
		"https://user@example.test/register?action=startnew",
		"https://www.example.test/register?action=startnew#private",
	}
	for _, raw := range tests {
		t.Run(strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			_, err := Parse(raw, true, testOrigin)
			if err == nil {
				t.Fatalf("unsafe URL accepted")
			}
			for _, secret := range []string{"private", "token", "sk-proj", "flow"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error disclosed rejected material: %v", err)
				}
			}
		})
	}
}

func TestParsePreservesV1QueryRejection(t *testing.T) {
	if _, err := Parse("https://www.example.test/register?action=startnew", false, testOrigin); err == nil {
		t.Fatal("v1 unexpectedly accepted a query")
	}
}

func testOrigin(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "www.example.test" {
		return "", errors.New("invalid origin")
	}
	return raw, nil
}
