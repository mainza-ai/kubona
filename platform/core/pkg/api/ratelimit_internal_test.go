package api

import (
	"testing"
)

// The bucket map is hard-bounded: spoofed-key floods cannot grow memory
// without bound.
func TestRateLimiterBounded(t *testing.T) {
	l := newRateLimiter(1000, 10)
	for i := 0; i < maxRateBuckets+500; i++ {
		l.allow("flood-key-" + itoaInternal(i))
	}
	l.mu.Lock()
	n := len(l.buckets)
	l.mu.Unlock()
	if n > maxRateBuckets {
		t.Errorf("bucket map exceeded bound: %d", n)
	}
	// The limiter still functions after overflow.
	if !l.allow("legit-key") {
		t.Error("limiter must serve after eviction")
	}
}

func itoaInternal(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
