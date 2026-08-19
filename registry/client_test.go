package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientLocalSearchPullAndDefaultLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	base := loadFixtureBundle(t, "read-only")
	for _, release := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"} {
		value := rebuildRelease(t, base, release, "reviewed_synthetic_fixture")
		if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
			t.Fatal(err)
		}
	}
	client := &Client{NetworkPolicy: NetworkNever}
	report, err := client.Search(context.Background(), SearchOptions{Location: root, Query: "status", At: registryTime})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != DefaultMaxResults {
		t.Fatalf("results = %d, want %d", len(report.Results), DefaultMaxResults)
	}
	for _, result := range report.Results {
		if result.Score == 0 || result.Entry.ID != "example/status" || result.Status != "active" {
			t.Fatalf("result = %#v", result)
		}
	}
	pulled, err := client.Pull(context.Background(), PullOptions{
		Location: root, Coordinate: &Coordinate{ID: "example/status", Release: "2.0.0"}, At: registryTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pulled.Bundle.Payload.Identity.Release != "2.0.0" || len(pulled.Content) == 0 {
		t.Fatalf("pull = %#v", pulled.Entry)
	}
	byDigest, err := client.Pull(context.Background(), PullOptions{
		Location: root, Digest: pulled.Entry.Bundle.Digest.String(), At: registryTime,
	})
	if err != nil || byDigest.Entry.Release != "2.0.0" {
		t.Fatalf("digest pull = %#v, %v", byDigest.Entry, err)
	}
}

func TestNilClientUsesOfflineDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	var client *Client
	if _, err := client.Search(context.Background(), SearchOptions{Location: root, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Search(context.Background(), SearchOptions{Location: "https://example.com", At: registryTime}); err == nil || !strings.Contains(err.Error(), "forbids") {
		t.Fatalf("expected nil-client network denial, got %v", err)
	}
}

func TestClientInactiveFilteringAndPullPolicy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	coordinate := Coordinate{ID: "example/status", Release: "1.0.0"}
	if _, err := UpdateLifecycleLocal(context.Background(), root, coordinate, "revoked", registryTime.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	client := &Client{}
	report, err := client.Search(context.Background(), SearchOptions{Location: root, At: registryTime.Add(time.Hour)})
	if err != nil || len(report.Results) != 0 {
		t.Fatalf("active search = %#v, %v", report.Results, err)
	}
	report, err = client.Search(context.Background(), SearchOptions{Location: root, At: registryTime.Add(time.Hour), IncludeInactive: true})
	if err != nil || len(report.Results) != 1 || report.Results[0].Status != "revoked" {
		t.Fatalf("inactive search = %#v, %v", report.Results, err)
	}
	if _, err := client.Pull(context.Background(), PullOptions{Location: root, Coordinate: &coordinate, At: registryTime.Add(time.Hour)}); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected revoked pull rejection, got %v", err)
	}
	if _, err := client.Pull(context.Background(), PullOptions{Location: root, Coordinate: &coordinate, At: registryTime.Add(time.Hour), AllowInactive: true}); err != nil {
		t.Fatalf("historical pull: %v", err)
	}
}

func TestClientHTTPSReadAndVerify(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client(), AllowUnsafeHosts: true}
	report, err := client.Search(context.Background(), SearchOptions{Location: server.URL, Query: "read_status", At: registryTime})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || !strings.HasPrefix(report.Registry, server.URL) {
		t.Fatalf("search = %#v", report)
	}
	pulled, err := client.Pull(context.Background(), PullOptions{
		Location: server.URL, Coordinate: &Coordinate{ID: "example/status", Release: "1.0.0"}, At: registryTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pulled.Source, server.URL) {
		t.Fatalf("pull source = %q", pulled.Source)
	}
	verified, err := client.Verify(context.Background(), server.URL, registryTime)
	if err != nil || len(verified.Entries) != 1 {
		t.Fatalf("verify = %#v, %v", verified, err)
	}
}

func TestClientNetworkPolicyUnsafeHostAndHTTPSOnly(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	for _, policy := range []NetworkPolicy{NetworkNever, NetworkAsk, "invalid"} {
		client := &Client{NetworkPolicy: policy, HTTPClient: server.Client(), AllowUnsafeHosts: true}
		_, err := client.Search(context.Background(), SearchOptions{Location: server.URL, At: registryTime})
		if err == nil {
			t.Fatalf("policy %q unexpectedly allowed network", policy)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("policy rejection made %d requests", requests.Load())
	}
	client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client()}
	if _, err := client.Search(context.Background(), SearchOptions{Location: server.URL, At: registryTime}); err == nil || !errors.Is(err, ErrPolicy) {
		t.Fatalf("expected unsafe host rejection, got %v", err)
	}
	client.AllowUnsafeHosts = true
	if _, err := client.Search(context.Background(), SearchOptions{Location: strings.Replace(server.URL, "https://", "http://", 1), At: registryTime}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected HTTPS rejection, got %v", err)
	}
}

func TestUnsafeIPRejectsReservedAndEmbeddedAddresses(t *testing.T) {
	for _, test := range []struct {
		name   string
		value  string
		unsafe bool
	}{
		{name: "CGNAT lower boundary", value: "100.64.0.0", unsafe: true},
		{name: "CGNAT upper boundary", value: "100.127.255.255", unsafe: true},
		{name: "below CGNAT", value: "100.63.255.255", unsafe: false},
		{name: "above CGNAT", value: "100.128.0.0", unsafe: false},
		{name: "IPv4-mapped CGNAT", value: "::ffff:100.64.0.1", unsafe: true},
		{name: "current network", value: "0.1.2.3", unsafe: true},
		{name: "IETF assignment", value: "192.0.0.9", unsafe: true},
		{name: "documentation one", value: "192.0.2.1", unsafe: true},
		{name: "documentation two", value: "198.51.100.1", unsafe: true},
		{name: "documentation three", value: "203.0.113.1", unsafe: true},
		{name: "benchmarking", value: "198.19.255.255", unsafe: true},
		{name: "future use", value: "250.1.2.3", unsafe: true},
		{name: "public IPv4", value: "8.8.8.8", unsafe: false},
		{name: "private IPv4", value: "10.0.0.1", unsafe: true},
		{name: "private IPv6", value: "fd00::1", unsafe: true},
		{name: "IPv6 documentation", value: "2001:db8::1", unsafe: true},
		{name: "6to4 private", value: "2002:0a00:0001::", unsafe: true},
		{name: "6to4 public", value: "2002:0808:0808::", unsafe: false},
		{name: "Teredo private server", value: "2001:0000:0a00:0001:0000:0000:f7f7:f7f7", unsafe: true},
		{name: "Teredo private client", value: "2001:0000:0808:0808:0000:0000:f5ff:fffe", unsafe: true},
		{name: "Teredo public", value: "2001:0000:0808:0808:0000:0000:f7f7:f7f7", unsafe: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := net.ParseIP(test.value)
			if value == nil {
				t.Fatalf("could not parse test IP %q", test.value)
			}
			if got := unsafeIP(value); got != test.unsafe {
				t.Fatalf("unsafeIP(%q)=%v, want %v", test.value, got, test.unsafe)
			}
		})
	}
}

func TestLoopbackOptInCannotPermitPrivateOrReservedHosts(t *testing.T) {
	for _, deprecated := range []bool{false, true} {
		client := &Client{AllowLoopbackHosts: !deprecated, AllowUnsafeHosts: deprecated}
		if err := client.rejectHost(context.Background(), "127.0.0.1"); err != nil {
			t.Fatalf("deprecated=%v loopback: %v", deprecated, err)
		}
		for _, host := range []string{"10.0.0.1", "192.0.2.1", "198.18.0.1"} {
			if err := client.rejectHost(context.Background(), host); err == nil {
				t.Fatalf("deprecated=%v unexpectedly allowed %s", deprecated, host)
			}
		}
	}
}

func TestClientBoundsCancellationTimeoutAndRedirect(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	if _, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime}); err != nil {
		t.Fatal(err)
	}
	t.Run("size", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(strings.Repeat("x", 65)))
		}))
		defer server.Close()
		client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client(), AllowUnsafeHosts: true, MaxBytes: 64}
		if _, err := client.Search(context.Background(), SearchOptions{Location: server.URL, At: registryTime}); err == nil || !errors.Is(err, ErrLimit) {
			t.Fatalf("expected size rejection, got %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer server.Close()
		client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client(), AllowUnsafeHosts: true, Timeout: 10 * time.Millisecond}
		if _, err := client.Search(context.Background(), SearchOptions{Location: server.URL, At: registryTime}); err == nil {
			t.Fatal("expected timeout")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client := &Client{}
		if _, err := client.Search(ctx, SearchOptions{Location: root, At: registryTime}); err == nil {
			t.Fatal("expected cancellation")
		}
	})
	t.Run("unsafe redirect scheme", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "http://example.com/index.json", http.StatusFound)
		}))
		defer server.Close()
		client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client(), AllowUnsafeHosts: true}
		if _, err := client.Search(context.Background(), SearchOptions{Location: server.URL, At: registryTime}); err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Fatalf("expected redirect rejection, got %v", err)
		}
	})
}

func TestClientRejectsRemoteTamperAndBadSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "registry")
	value := loadFixtureBundle(t, "read-only")
	report, err := PublishLocal(context.Background(), PublishOptions{Root: root, Bundle: value, At: registryTime})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(report.BlobPath, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	client := &Client{NetworkPolicy: NetworkAllow, HTTPClient: server.Client(), AllowUnsafeHosts: true}
	coordinate := Coordinate{ID: "example/status", Release: "1.0.0"}
	if _, err := client.Pull(context.Background(), PullOptions{Location: server.URL, Coordinate: &coordinate, At: registryTime}); err == nil || !errors.Is(err, ErrIntegrity) {
		t.Fatalf("expected remote tamper rejection, got %v", err)
	}
	if _, err := client.Pull(context.Background(), PullOptions{Location: server.URL, At: registryTime}); err == nil {
		t.Fatal("expected missing selector rejection")
	}
	if _, err := client.Pull(context.Background(), PullOptions{Location: server.URL, Coordinate: &coordinate, Digest: fmt.Sprintf("sha256:%064d", 0), At: registryTime}); err == nil {
		t.Fatal("expected duplicate selector rejection")
	}
}
