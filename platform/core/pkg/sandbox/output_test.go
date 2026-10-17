package sandbox

// S6 — bounded capture. Threat: unbounded agent-controlled streams
// persisted verbatim into rows served by the API.

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundOutputPassthrough(t *testing.T) {
	for _, s := range []string{"", "x", strings.Repeat("y", MaxStoredOutputBytes)} {
		out, trunc, n := BoundOutput(s)
		if out != s || trunc || n != len(s) {
			t.Fatalf("passthrough broken for len %d", len(s))
		}
	}
}

func TestBoundOutputTruncatesWithAccounting(t *testing.T) {
	big := strings.Repeat("z", MaxStoredOutputBytes+1000)
	out, trunc, n := BoundOutput(big)
	if !trunc {
		t.Fatalf("must flag truncation")
	}
	if n != len(big) {
		t.Fatalf("must report original bytes, got %d", n)
	}
	if len(out) != MaxStoredOutputBytes {
		t.Fatalf("must cap at %d, got %d", MaxStoredOutputBytes, len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("truncation must land on a UTF-8 boundary")
	}
}

func TestBoundOutputMultibyteBoundary(t *testing.T) {
	// Fill to cap-1 then add a 3-byte rune: naive cut would split it.
	s := strings.Repeat("a", MaxStoredOutputBytes-1) + "€" + strings.Repeat("b", 100)
	out, trunc, _ := BoundOutput(s)
	if !trunc || !utf8.ValidString(out) {
		t.Fatalf("multibyte boundary unsafe: trunc=%v valid=%v len=%d", trunc, utf8.ValidString(out), len(out))
	}
	if len(out) > MaxStoredOutputBytes {
		t.Fatalf("over cap: %d", len(out))
	}
}
