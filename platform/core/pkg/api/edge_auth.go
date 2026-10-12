package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
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
		want := os.Getenv("KUBONA_API_TOKEN")
		if want == "" {
			http.Error(w, "API token not configured: set KUBONA_API_TOKEN", http.StatusServiceUnavailable)
			return
		}
		const prefix = "Bearer "
		got := r.Header.Get("Authorization")
		if !strings.HasPrefix(got, prefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(got, prefix)), []byte(want)) != 1 {
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// corsAllowOrigin resolves the Access-Control-Allow-Origin value for a
// request. Default is an explicit allowlist (local console origins plus
// KUBONA_CORS_ORIGINS extras); "*" requires the KUBONA_CORS_ANY=1 dev
// escape hatch, never the default. Non-matching origins get no header,
// so browsers block them.
func corsAllowOrigin(r *http.Request) string {
	if os.Getenv("KUBONA_CORS_ANY") == "1" {
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
		return allow[0]
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
// The bucket map is bounded: past maxBuckets the longest-idle entry is
// evicted to make room, so spoofed-key floods cannot grow memory without
// bound (at the cost of sharing budget under extreme pressure — the
// fail-open direction for availability, with 429s still biting).
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
			// scan, only on overflow — steady state never pays it. If
			// every entry is fresh (same-instant flood), drop an
			// arbitrary one: the bound is hard either way.
			oldest, oldestAt := "", now
			for k, v := range l.buckets {
				if v.last.Before(oldestAt) {
					oldest, oldestAt = k, v.last
				}
			}
			if oldest == "" {
				for k := range l.buckets {
					oldest = k
					break
				}
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

// clientKey attributes a request for rate limiting. X-Forwarded-For is
// honored only from loopback (no trusted-proxy config exists yet); direct
// connections key on the remote IP WITHOUT the port — keying on
// RemoteAddr verbatim mints one bucket per TCP connection and the limiter
// never trips (caught live in F-4: 25 rapid curls, 25×201). Spoofing XFF
// from non-loopback buys nothing.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "127.0.0.1" || host == "::1" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return "xff:" + strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	return "addr:" + host
}
