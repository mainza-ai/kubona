package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// W2.3: env rate overrides keep defaults when unset, honor valid values,
// and fall back on garbage (no behavior change when unset).
func TestW23RateFromEnv(t *testing.T) {
	t.Setenv("KUBONA_RATE_SIGN", "")
	if r, b := rateFromEnv("KUBONA_RATE_SIGN", 2, 5); r != 2 || b != 5 {
		t.Errorf("unset must keep default, got %v,%v", r, b)
	}
	t.Setenv("KUBONA_RATE_SIGN", "10,20")
	if r, b := rateFromEnv("KUBONA_RATE_SIGN", 2, 5); r != 10 || b != 20 {
		t.Errorf("valid override must apply, got %v,%v", r, b)
	}
	t.Setenv("KUBONA_RATE_SIGN", "garbage")
	if r, b := rateFromEnv("KUBONA_RATE_SIGN", 2, 5); r != 2 || b != 5 {
		t.Errorf("garbage must fall back to default, got %v,%v", r, b)
	}
	t.Setenv("KUBONA_RATE_SIGN", "0,-1")
	if r, b := rateFromEnv("KUBONA_RATE_SIGN", 2, 5); r != 2 || b != 5 {
		t.Errorf("non-positive must fall back, got %v,%v", r, b)
	}
}

// W2.3: XFF honored from loopback or listed proxies only.
func TestW23TrustedProxyXFF(t *testing.T) {
	mk := func(remote, xff string) string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return clientKey(r)
	}
	t.Setenv("KUBONA_TRUSTED_PROXIES", "")
	if got := mk("127.0.0.1:1234", "9.9.9.9"); got != "xff:9.9.9.9" {
		t.Errorf("loopback must honor XFF, got %q", got)
	}
	if got := mk("203.0.113.7:1234", "9.9.9.9"); got == "xff:9.9.9.9" {
		t.Errorf("unlisted peer XFF must be ignored, got %q", got)
	}
	t.Setenv("KUBONA_TRUSTED_PROXIES", "203.0.113.0/24")
	if got := mk("203.0.113.7:1234", "9.9.9.9"); got != "xff:9.9.9.9" {
		t.Errorf("listed CIDR must honor XFF, got %q", got)
	}
	t.Setenv("KUBONA_TRUSTED_PROXIES", "203.0.113.7")
	if got := mk("203.0.113.7:1234", "9.9.9.9"); got != "xff:9.9.9.9" {
		t.Errorf("listed IP must honor XFF, got %q", got)
	}
	t.Setenv("KUBONA_TRUSTED_PROXIES", "not-a-cidr,,,")
	if got := mk("203.0.113.7:1234", "9.9.9.9"); got == "xff:9.9.9.9" {
		t.Errorf("garbage allowlist must honor nothing, got %q", got)
	}
}

// W2.3: limiter eviction is fail-closed — a same-instant flood (no idle
// entry) refuses the newcomer instead of sharing a live budget, and the
// map never exceeds its bound.
func TestW23LimiterEvictionFailClosed(t *testing.T) {
	l := newRateLimiter(1000, 10)
	future := time.Now().Add(time.Hour)
	l.mu.Lock()
	for i := 0; i < maxRateBuckets; i++ {
		l.buckets["k"] = &tokenBucket{tokens: 1, last: future}
		break
	}
	// Fill to the bound with future-dated (never-idle) entries.
	for i := len(l.buckets); i < maxRateBuckets; i++ {
		l.buckets["fill-"+itoaInternal(i)] = &tokenBucket{tokens: 1, last: future}
	}
	l.mu.Unlock()
	if l.allow("newcomer") {
		t.Error("eviction with no idle entry must refuse (429), not share budget")
	}
	l.mu.Lock()
	n := len(l.buckets)
	l.mu.Unlock()
	if n > maxRateBuckets {
		t.Errorf("bound exceeded: %d", n)
	}
}

// W2.3: empty Origin gets no CORS header (no allow[0] fallback).
func TestW23CORSEmptyOriginInternal(t *testing.T) {
	t.Setenv("KUBONA_CORS_ANY", "")
	t.Setenv("KUBONA_CORS_ORIGINS", "")
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := corsAllowOrigin(r); got != "" {
		t.Errorf("empty origin must yield no header, got %q", got)
	}
}
