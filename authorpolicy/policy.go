// Package authorpolicy defines immutable local authoring policy. It is not a
// browser protocol field and never grants access to an additional origin.
package authorpolicy

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Policy keeps one explicitly selected HTTPS origin blocked while allowing
// authoring to survive only its non-navigation GET/Script requests. Zero is strict.
type Policy struct{ origin string }

var dnsHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)

// New validates local configuration without echoing any supplied value.
func New(origin string) (Policy, error) {
	if origin == "" {
		return Policy{}, nil
	}
	u, canonical, ok := parse(origin)
	if !ok || u.Path != "" || u.RawQuery != "" || u.ForceQuery {
		return Policy{}, errors.New("blocked script policy requires an exact HTTPS origin")
	}
	return Policy{origin: canonical}, nil
}

func (p Policy) Origin() string { return p.origin }

// Matches describes a denial, never permission to fetch. The caller must check
// its prior failure, cancellation and request budget before using this result.
func (p Policy) Matches(rawURL, method string, navigation bool, resource string) bool {
	if p.origin == "" || navigation || method != "GET" || resource != "script" {
		return false
	}
	_, origin, ok := parse(rawURL)
	return ok && origin == p.origin
}

func parse(raw string) (*url.URL, string, bool) {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, " \t\r\n\\#") {
		return nil, "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return nil, "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || (net.ParseIP(host) == nil && (!dnsHost.MatchString(host) || strings.Contains(host, ".."))) {
		return nil, "", false
	}
	if net.ParseIP(host) == nil {
		if len(host) > 253 {
			return nil, "", false
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return nil, "", false
			}
		}
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return nil, "", false
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return nil, "", false
		}
	}
	if _, err := url.QueryUnescape(u.RawQuery); err != nil {
		return nil, "", false
	}
	if port == "443" {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u, "https://" + host, true
}
