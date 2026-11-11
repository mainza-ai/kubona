package llm

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// IsLocalHost reports whether a bare hostname or IP literal refers to a local
// loopback, LAN/RFC1918, link-local, CGNAT, or container-local address.
func IsLocalHost(rawHost string) bool {
	h := strings.TrimSpace(rawHost)
	if h == "" {
		return false
	}
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}
	// Strip IPv6 zone identifier if present (e.g. fe80::1%en0).
	if i := strings.Index(h, "%"); i >= 0 {
		h = h[:i]
	}
	lower := strings.TrimSuffix(strings.ToLower(h), ".")
	if lower == "localhost" ||
		strings.HasSuffix(lower, ".localhost") ||
		strings.HasSuffix(lower, ".local") ||
		lower == "host.docker.internal" {
		return true
	}

	addr, err := netip.ParseAddr(h)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(addr)
}

// IsLocalEndpoint reports whether a URL points to a local or LAN host.
func IsLocalEndpoint(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return false
	}
	return IsLocalHost(parsed.Hostname())
}

// IsLoopbackEndpoint reports whether a URL resolves strictly to loopback.
func IsLoopbackEndpoint(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return false
	}
	h := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	addr, err := netip.ParseAddr(h)
	if err != nil {
		return false
	}
	return addr.Unmap().IsLoopback()
}

// ValidateEndpointURL enforces scheme, credential-free authority, and
// local-only plaintext HTTP policy for an LLM provider BaseURL.
func ValidateEndpointURL(provider ProviderType, rawURL string) (cleaned string, isLocal bool, err error) {
	u := strings.TrimSpace(rawURL)
	if u == "" {
		return "", isLocalProvider(provider), nil
	}
	parsed, perr := url.Parse(u)
	if perr != nil || parsed.Host == "" {
		return "", false, fmt.Errorf("provider %s has an unparseable BaseURL: refusing", provider)
	}
	if parsed.User != nil {
		return "", false, fmt.Errorf("provider %s: BaseURL must not embed userinfo credentials: refusing", provider)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", false, fmt.Errorf("provider %s: BaseURL must not include query or fragment parameters: refusing", provider)
	}
	host := parsed.Hostname()
	if _, _, splitErr := net.SplitHostPort(parsed.Host); splitErr != nil && strings.Count(parsed.Host, ":") > 0 && !strings.HasPrefix(parsed.Host, "[") {
		return "", false, fmt.Errorf("provider %s has a malformed host %q: refusing", provider, parsed.Host)
	}
	local := IsLocalHost(host)
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		parsed.Scheme = "https"
	case "http":
		if !local {
			return "", false, fmt.Errorf("provider %s: non-local http:// endpoints refuse (credentials would cross plaintext)", provider)
		}
		parsed.Scheme = "http"
	default:
		return "", false, fmt.Errorf("provider %s: scheme %q refuses (https or loopback http only)", provider, parsed.Scheme)
	}
	out := strings.TrimRight(parsed.String(), "/")
	return out, local, nil
}
