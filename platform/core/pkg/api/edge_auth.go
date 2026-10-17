package api

import (
	"crypto/subtle"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Edge auth (B0-2): the trust boundary in front of spend-authority
// endpoints. Three independent rails, all fail-closed:
//
//  1. Bearer token (requireBearer): Authorization: Bearer <token> checked
//     against KUBONA_API_TOKEN with constant-time compare. The token is
//     operator-configured on both sides; when unset the endpoint refuses
//     with 503 instead of silently serving. NOTE: the console's
//     NEXT_PUBLIC_* token is client-visible and is NOT accepted here —
//     public values are not secrets.
//  2. Tenant-admin envelope: spend mandates bind a tenant admin's Ed25519
//     signature over the exact parameters (same bindAdminAction envelope
//     as the tenant mutating routes). Stolen bearer tokens alone cannot
//     mint spend for a tenant whose admin key they lack.
//  3. Rate limits: per-IP token buckets on sign/verify so stolen
//     credentials cannot be sprayed at machine pace.

// requireBearer gates one handler on the operator API token.
func (s *Server) requireBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch bearerGate(r) {
		case http.StatusOK:
			next(w, r)
		case http.StatusServiceUnavailable:
			WriteAPIError(w, "server_misconfigured", "API token not configured: set KUBONA_API_TOKEN", nil)
		default:
			WriteAPIError(w, "unauthorized", "missing or invalid bearer token", nil)
		}
	}
}

// bearerGate checks the operator token without writing a response, for
// handlers that mix public reads with gated writes on one route (custom
// agents: list public, create/delete gated). Returns StatusOK when the
// token checks out, ServiceUnavailable when unconfigured, Unauthorized
// on mismatch — the same codes requireBearer emits.
func bearerGate(r *http.Request) int {
	want := os.Getenv("KUBONA_API_TOKEN")
	if want == "" {
		return http.StatusServiceUnavailable
	}
	const prefix = "Bearer "
	got := r.Header.Get("Authorization")
	if !strings.HasPrefix(got, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(got, prefix)), []byte(want)) != 1 {
		return http.StatusUnauthorized
	}
	return http.StatusOK
}

// corsAllowOrigin resolves the Access-Control-Allow-Origin value for a
// request. Default is an explicit allowlist (local console origins plus
// KUBONA_CORS_ORIGINS extras); "*" requires the KUBONA_CORS_ANY=1 dev
// escape hatch (explicit "1" only, never "true"/empty), never the default.
// Non-matching origins get no header, so browsers block them. Threat model:
// reads stay public behind limiters (CORS is a browser-only control, not
// auth); writes get edge auth regardless of origin. An empty Origin carries
// no allowlist proof, so it gets no header either (no allow[0] fallback —
// that minted a trusted origin for non-browser clients).
func corsAllowOrigin(r *http.Request) string {
	if os.Getenv("KUBONA_CORS_ANY") == "1" {
		log.Printf("[Kubona Core] WARNING: KUBONA_CORS_ANY=1 — CORS wide open (*); dev only, never production")
		return "*"
	}
	allow := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if extra := os.Getenv("KUBONA_CORS_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allow = append(allow, o)
			}
		}
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return ""
	}
	for _, o := range allow {
		if subtle.ConstantTimeCompare([]byte(origin), []byte(o)) == 1 {
			return origin
		}
	}
	return ""
}

// rateLimiter is a per-key token bucket. v1 constants are deliberately
// tight on sign (human-paced approvals) and looser on verify (called per
// submit by admission paths); tune with production traffic, not theory.
// Defaults are documented here and overridable via environment (no behavior
// change when unset):
//
//	KUBONA_RATE_SIGN   "2,5"    (2 tokens/sec, burst 5)
//	KUBONA_RATE_VERIFY "20,50"  (20/sec, burst 50)
//	KUBONA_RATE_TENANT "5,15"
//	KUBONA_RATE_GRANT  "5,15"
//	KUBONA_RATE_LEDGER "5,15"   (ledger append / abort kill-switch)
//
// Format is "rate,burst" (float,int); malformed values fall back to the
// compiled default.
// The bucket map is bounded: past maxBuckets the longest-idle entry is
// evicted to make room, so spoofed-key floods cannot grow memory without
// bound. Eviction is fail-closed: when no idle entry exists to evict
// (same-instant flood), the new key is refused (429) instead of deleting an
// arbitrary live budget and sharing it (the old fail-open direction).
func rateFromEnv(key string, defRate float64, defBurst int) (float64, int) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defRate, defBurst
	}
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return defRate, defBurst
	}
	rate, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	burst, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || rate <= 0 || burst <= 0 {
		return defRate, defBurst
	}
	return rate, burst
}

type rateLimiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   int
	buckets map[string]*tokenBucket
}

// maxRateBuckets caps per-limiter memory.
const maxRateBuckets = 10000

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(rate float64, burst int) *rateLimiter {
	return &rateLimiter{rate: rate, burst: burst, buckets: map[string]*tokenBucket{}}
}

// allow reports whether key may proceed, consuming one token.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= maxRateBuckets {
			// Evict the longest-idle entry to bound memory. One linear
			// scan, only on overflow — steady state never pays it.
			// Fail-closed: when every entry is fresh (same-instant
			// flood, no idle candidate), refuse the newcomer (429)
			// instead of deleting an arbitrary live budget and
			// sharing it.
			oldest, oldestAt := "", now
			for k, v := range l.buckets {
				if v.last.Before(oldestAt) {
					oldest, oldestAt = k, v.last
				}
			}
			if oldest == "" {
				return false
			}
			delete(l.buckets, oldest)
		}
		b = &tokenBucket{tokens: float64(l.burst), last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// trustedProxy reports whether the direct peer is allowed to assert
// X-Forwarded-For. Loopback is always trusted (tests + local reverse
// proxies); anything else must appear in the KUBONA_TRUSTED_PROXIES
// allowlist (comma-separated IPs or CIDRs, e.g.
// "10.0.0.0/8,192.168.1.10"). Unlisted peers have their XFF ignored and
// share one bucket on their direct IP (stated, fail-closed: spoofing XFF
// from an untrusted peer buys nothing).
func trustedProxy(host string) bool {
	if host == "127.0.0.1" || host == "::1" {
		return true
	}
	raw := strings.TrimSpace(os.Getenv("KUBONA_TRUSTED_PROXIES"))
	if raw == "" {
		return false
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			_, cidr, err := net.ParseCIDR(part)
			if err != nil || ip == nil {
				continue
			}
			if cidr.Contains(ip) {
				return true
			}
			continue
		}
		if part == host {
			return true
		}
		if pip := net.ParseIP(part); pip != nil && ip != nil && pip.Equal(ip) {
			return true
		}
	}
	return false
}

// clientKey attributes a request for rate limiting. X-Forwarded-For is
// honored only from loopback or KUBONA_TRUSTED_PROXIES peers (no allowlist
// = loopback only); direct connections key on the remote IP WITHOUT the
// port — keying on RemoteAddr verbatim mints one bucket per TCP connection
// and the limiter never trips (caught live in F-4: 25 rapid curls, 25×201).
// Spoofing XFF from non-trusted peers buys nothing (shared direct-IP bucket).
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trustedProxy(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return "xff:" + strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	return "addr:" + host
}
