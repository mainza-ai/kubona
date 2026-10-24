package api

import (
	"encoding/json"
	"testing"
	"time"
)

// Cache partition is the correctness core of the canary proxy: fresh
// entries serve without touching hardware APIs; stale/missing entries
// re-probe. TTL expiry must flip an entry from fresh to stale exactly at
// the boundary — a stuck-fresh cache would present dead telemetry as live.
func TestCanaryCachePartition(t *testing.T) {
	c := newCanaryCache()
	now := time.Now()
	c.put("fresh", json.RawMessage(`{"ok":true}`), now.Add(-time.Minute))
	c.put("stale", json.RawMessage(`{"ok":true}`), now.Add(-10*time.Minute))
	fresh, missing := c.partition([]string{"fresh", "stale", "absent"}, now)
	if len(fresh) != 1 || fresh["fresh"] == nil {
		t.Fatalf("only fresh entries served from cache, got %v / %v", fresh, missing)
	}
	if len(missing) != 2 {
		t.Fatalf("stale + absent entries must re-probe, got %v", missing)
	}
}

func TestCanarySecondsParsing(t *testing.T) {
	for _, bad := range []string{"", "0", "-5", "abc", "12x", "99999999"} {
		if _, err := parseCanarySeconds(bad); err == nil {
			t.Fatalf("parseCanarySeconds(%q) must refuse", bad)
		}
	}
	if n, err := parseCanarySeconds("90"); err != nil || n != 90 {
		t.Fatalf("parseCanarySeconds(90) = %d, %v", n, err)
	}
}
