package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	pb "github.com/mainza-ai/kubona/core/proto"
)

// Live canary proxy (fleet honesty follow-through). The Python canary
// reads live backend properties where credentials exist and skips the
// rest fast — never a measurement, never a zero. Go caches per provider
// (TTL) so the console can refresh without hammering hardware APIs; on
// bridge failure or timeout the endpoint fails soft (ok:false) and the
// console keeps showing declared catalog values.

const canaryCacheTTL = 5 * time.Minute
const canaryDefaultTimeoutS = 60
const canaryMaxTimeoutS = 120

type canaryEntry struct {
	at      time.Time
	results json.RawMessage
}

type canaryCache struct {
	mu      sync.Mutex
	entries map[string]canaryEntry
}

func newCanaryCache() *canaryCache {
	return &canaryCache{entries: map[string]canaryEntry{}}
}

// get returns cached results younger than the TTL, split into fresh
// (served as-is) and stale (re-probed) provider sets.
func (c *canaryCache) partition(providers []string, now time.Time) (fresh map[string]json.RawMessage, stale []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fresh = map[string]json.RawMessage{}
	for _, p := range providers {
		if e, ok := c.entries[p]; ok && now.Sub(e.at) < canaryCacheTTL {
			fresh[p] = e.results
		} else {
			stale = append(stale, p)
		}
	}
	return fresh, stale
}

func (c *canaryCache) put(provider string, results json.RawMessage, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[provider] = canaryEntry{at: now, results: results}
}

// GET /api/quantum/canary?providers=a,b&max_seconds=60 — public read like
// fleet (credential presence is already public there; results carry no
// secrets). Unknown providers 400; timeout clamped to [5, 120]s.
func (s *Server) handleQuantumCanary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteAPIError(w, "method_not_allowed", "GET required for canary results", nil)
		return
	}
	var providers []string
	if raw := strings.TrimSpace(r.URL.Query().Get("providers")); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				if !knownQuantumProvider(p) {
					WriteAPIError(w, "bad_request", "unknown provider "+p+": use a known provider id", nil)
					return
				}
				providers = append(providers, p)
			}
		}
	}
	if len(providers) == 0 {
		providers = append([]string{}, KnownQuantumProviders...)
	}
	maxSeconds := canaryDefaultTimeoutS
	if raw := strings.TrimSpace(r.URL.Query().Get("max_seconds")); raw != "" {
		if n, err := parseCanarySeconds(raw); err == nil {
			maxSeconds = n
		}
	}
	if maxSeconds < 5 {
		maxSeconds = 5
	}
	if maxSeconds > canaryMaxTimeoutS {
		maxSeconds = canaryMaxTimeoutS
	}
	now := time.Now()
	fresh, stale := s.quantumCanaryCache.partition(providers, now)
	merged := map[string]json.RawMessage{}
	for p, res := range fresh {
		merged[p] = res
	}
	cached := len(stale) == 0
	if len(stale) > 0 {
		bridge := s.quantumBridge(w, "quantum_canary")
		if bridge == nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(maxSeconds)*time.Second)
		defer cancel()
		resp, err := bridge.RunCanary(ctx, &pb.CanaryRunRequest{Providers: stale, MaxSeconds: int32(maxSeconds)})
		if err != nil {
			writeBridgeUnavailable(w, "quantum_canary", "RunCanary RPC failed: "+err.Error())
			return
		}
		if !resp.Ok {
			writeBridgeUnavailable(w, "quantum_canary", resp.ErrorMessage)
			return
		}
		for p, res := range indexCanaryResults(resp.ResultsJson, stale) {
			s.quantumCanaryCache.put(p, res, now)
			merged[p] = res
		}
		cached = false
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "results": merged, "cached": cached,
		"cache_ttl_seconds": int(canaryCacheTTL.Seconds()),
	})
}

// indexCanaryResults indexes RunCanary's JSON output by provider id (F-NEW-06).
// Python's canary.run_all returns a JSON array [{"provider":"...", ...}, ...];
// map-shaped responses are also accepted. Each entry is stored under its own
// provider key rather than duplicating the full array under every stale key.
func indexCanaryResults(raw string, stale []string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	var perProvider map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &perProvider); err == nil && len(perProvider) > 0 {
		for p, res := range perProvider {
			out[p] = res
		}
		return out
	}
	var list []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		for _, elem := range list {
			var header struct {
				Provider string `json:"provider"`
			}
			if herr := json.Unmarshal(elem, &header); herr == nil && header.Provider != "" {
				out[header.Provider] = elem
			}
		}
		if len(out) > 0 {
			for _, p := range stale {
				if _, ok := out[p]; !ok {
					out[p] = json.RawMessage(`{"ok":false,"error":"provider omitted from canary response"}`)
				}
			}
			return out
		}
	}
	for _, p := range stale {
		out[p] = json.RawMessage(raw)
	}
	return out
}

// parseCanarySeconds parses a positive integer timeout, clamped by the
// caller to [5, canaryMaxTimeoutS].
func parseCanarySeconds(s string) (int, error) {
	if s == "" {
		return 0, errNotPositiveInt
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotPositiveInt
		}
		n = n*10 + int(c-'0')
		if n > 1000000 {
			return 0, errNotPositiveInt
		}
	}
	if n <= 0 {
		return 0, errNotPositiveInt
	}
	return n, nil
}

var errNotPositiveInt = canaryParseError("not a positive integer")

type canaryParseError string

func (e canaryParseError) Error() string { return string(e) }
