package capture

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/OpenUdon/browsertools/authorsession"
)

func TestNetworkGuardCoreBoundsAndFirstViolation(t *testing.T) {
	t.Run("requests", func(t *testing.T) {
		limitErr := errors.New("request limit")
		core := newNetworkGuardCore(2, 10)
		if !core.beginRequest(limitErr) || !core.beginRequest(limitErr) || core.beginRequest(limitErr) {
			t.Fatal("request boundary was not enforced")
		}
		if core.requests != 3 || !errors.Is(core.result(), limitErr) {
			t.Fatalf("requests=%d err=%v", core.requests, core.result())
		}
	})

	t.Run("declared response", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			length int64
			fails  bool
		}{
			{name: "below", length: 9},
			{name: "boundary", length: 10},
			{name: "negative", length: -1, fails: true},
			{name: "above", length: 11, fails: true},
		} {
			t.Run(test.name, func(t *testing.T) {
				sizeErr := errors.New("response size")
				core := newNetworkGuardCore(1, 10)
				core.observeResponseContentLength(test.length, sizeErr)
				if failed := errors.Is(core.result(), sizeErr); failed != test.fails {
					t.Fatalf("length=%d err=%v", test.length, core.result())
				}
			})
		}
	})

	t.Run("cumulative and overflow", func(t *testing.T) {
		sizeErr := errors.New("response size")
		core := newNetworkGuardCore(1, 10)
		if !core.observeFinishedResponse(4, sizeErr) || !core.observeFinishedResponse(6, sizeErr) {
			t.Fatal("response bytes at the exact boundary were rejected")
		}
		if core.observeFinishedResponse(1, sizeErr) || core.responseBytes != 10 || !errors.Is(core.result(), sizeErr) {
			t.Fatalf("bytes=%d err=%v", core.responseBytes, core.result())
		}

		core = newNetworkGuardCore(1, math.MaxInt64)
		if !core.observeFinishedResponse(math.MaxInt64, sizeErr) || core.observeFinishedResponse(1, sizeErr) {
			t.Fatal("MaxInt64 accumulation did not fail closed before overflow")
		}
		if core.responseBytes != math.MaxInt64 || !errors.Is(core.result(), sizeErr) {
			t.Fatalf("overflow bytes=%d err=%v", core.responseBytes, core.result())
		}

		core = newNetworkGuardCore(1, 10)
		if core.observeFinishedResponse(-1, sizeErr) || !errors.Is(core.result(), sizeErr) {
			t.Fatalf("negative response size err=%v", core.result())
		}
	})

	t.Run("sticky first violation", func(t *testing.T) {
		firstErr := errors.New("first")
		core := newNetworkGuardCore(1, 1)
		core.violate(firstErr)
		core.observeResponseContentLength(2, errors.New("declared"))
		core.observeFinishedResponse(2, errors.New("finished"))
		_ = core.beginRequest(errors.New("request"))
		if !errors.Is(core.result(), firstErr) {
			t.Fatalf("first violation was replaced: %v", core.result())
		}
	})
}

type guardParityHarness struct {
	allow         func() bool
	observeLength func(int64)
	observeBytes  func(int64)
	poison        func()
	result        func() error
	responseCode  string
	poisonCode    string
}

func networkGuardParityHarnesses() map[string]func() guardParityHarness {
	return map[string]func() guardParityHarness{
		"live": func() guardParityHarness {
			request := validLiveRequest()
			request.MaxRequests, request.MaxResponseBytes = 1, 10
			guard := newNetworkGuard(request)
			return guardParityHarness{
				allow: func() bool {
					return guard.allowRequest(requestFacts{URL: request.URL, Method: "GET", ResourceType: "document"})
				},
				observeLength: guard.observeResponseContentLength,
				observeBytes:  guard.observeFinishedResponse,
				poison: func() {
					guard.allowRequest(requestFacts{URL: "https://evil.test", Method: "GET", ResourceType: "document"})
				},
				result:       func() error { _, err := guard.result(); return err },
				responseCode: "response_size", poisonCode: "origin",
			}
		},
		"authentication": func() guardParityHarness {
			request := validAuthBrowserRequest()
			request.MaxRequests, request.MaxResponseBytes = 1, 10
			guard := newAuthNetworkGuard(request)
			return guardParityHarness{
				allow: func() bool {
					return guard.allowRequest(requestFacts{URL: "https://login.example.test", Method: "GET", ResourceType: "document"})
				},
				observeLength: guard.observeResponseContentLength,
				observeBytes:  guard.observeFinishedResponse,
				poison: func() {
					guard.allowRequest(requestFacts{URL: "https://evil.test", Method: "GET", ResourceType: "document"})
				},
				result: guard.result, responseCode: "response_size", poisonCode: "origin",
			}
		},
		"author": func() guardParityHarness {
			guard := newAuthorNetworkGuard(authorsession.BrowserRequest{
				ApprovedOrigins: []string{"https://example.test"}, MaxRequests: 1, MaxResponseBytes: 10,
			})
			return guardParityHarness{
				allow:        func() bool { return guard.allow("https://example.test", "GET") },
				observeBytes: guard.observeBytes,
				poison:       func() { guard.allow("https://evil.test", "GET") },
				result:       guard.result, responseCode: "response_limit", poisonCode: "origin_escape",
			}
		},
	}
}

func TestNetworkGuardAdapterAccountingParity(t *testing.T) {
	for name, factory := range networkGuardParityHarnesses() {
		t.Run(name+"/requests", func(t *testing.T) {
			guard := factory()
			if !guard.allow() || guard.allow() || policyCode(guard.result()) != "request_limit" {
				t.Fatalf("request result=%v", guard.result())
			}
		})
		t.Run(name+"/cumulative responses", func(t *testing.T) {
			guard := factory()
			guard.observeBytes(4)
			guard.observeBytes(6)
			if err := guard.result(); err != nil {
				t.Fatalf("exact byte boundary failed: %v", err)
			}
			guard.observeBytes(1)
			if code := policyCode(guard.result()); code != guard.responseCode {
				t.Fatalf("response code=%q err=%v", code, guard.result())
			}
		})
		t.Run(name+"/sticky violation", func(t *testing.T) {
			guard := factory()
			guard.poison()
			guard.observeBytes(11)
			if code := policyCode(guard.result()); code != guard.poisonCode {
				t.Fatalf("poison code=%q err=%v", code, guard.result())
			}
		})
	}

	for name, factory := range networkGuardParityHarnesses() {
		guard := factory()
		if guard.observeLength == nil {
			continue
		}
		t.Run(name+"/declared response", func(t *testing.T) {
			guard.observeLength(10)
			if err := guard.result(); err != nil {
				t.Fatalf("exact declared boundary failed: %v", err)
			}
			guard.observeLength(11)
			if code := policyCode(guard.result()); code != guard.responseCode {
				t.Fatalf("declared code=%q err=%v", code, guard.result())
			}
		})
	}
}

func TestNetworkGuardAdaptersRetainDistinctPolicies(t *testing.T) {
	liveRequest := validLiveRequest()
	liveRequest.MaxRequests, liveRequest.MaxResponseBytes = 8, 10
	live := newNetworkGuard(liveRequest)
	if !live.allowRequest(requestFacts{URL: liveRequest.URL, Method: "HEAD", ResourceType: "document"}) {
		t.Fatal("live capture blocked HEAD")
	}
	live = newNetworkGuard(liveRequest)
	if live.allowRequest(requestFacts{URL: liveRequest.URL, Method: "OPTIONS", ResourceType: "fetch"}) || policyCode(func() error { _, err := live.result(); return err }()) != "method" {
		t.Fatal("live capture widened beyond GET/HEAD")
	}
	live = newNetworkGuard(liveRequest)
	if live.allowRequest(requestFacts{URL: liveRequest.URL, Method: "GET", ResourceType: "image"}) {
		t.Fatal("live capture widened its resource allowlist")
	}
	if _, err := live.result(); err != nil {
		t.Fatalf("non-essential live resource poisoned the guard: %v", err)
	}

	auth := newAuthNetworkGuard(validAuthBrowserRequest())
	for _, resource := range []string{"image", "font"} {
		if !auth.allowRequest(requestFacts{URL: "https://login.example.test", Method: "OPTIONS", ResourceType: resource}) {
			t.Fatalf("authentication blocked OPTIONS %s: %v", resource, auth.result())
		}
	}
	auth = newAuthNetworkGuard(validAuthBrowserRequest())
	if auth.allowRequest(requestFacts{URL: "https://login.example.test", Method: "GET", ResourceType: "media"}) || auth.result() != nil {
		t.Fatalf("authentication resource fallback changed: %v", auth.result())
	}

	author := newAuthorNetworkGuard(authorsession.BrowserRequest{
		ApprovedOrigins: []string{"https://example.test"}, MaxRequests: 4, MaxResponseBytes: 10,
	})
	if !author.allow("https://example.test", "OPTIONS") {
		t.Fatalf("author session blocked OPTIONS: %v", author.result())
	}
	if author.allow("https://example.test", "POST") || !strings.Contains(author.result().Error(), "post_budget") {
		t.Fatalf("author session widened POST authority: %v", author.result())
	}
}
