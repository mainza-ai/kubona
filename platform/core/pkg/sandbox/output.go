package sandbox

// S6 — bounded execution-output capture.
//
// Threat: agent-controlled stdout/stderr is unbounded. Persisting it
// verbatim bloats job rows and ledger events served by the API (and can
// carry echoed secrets into stored records). Capture keeps a bounded head
// plus the original size, so truncation is visible, never silent.
//
// The single cap lives here (production bar: drift-proof shared values).

import (
	"unicode/utf8"
)

// MaxStoredOutputBytes caps one captured stream (Cloudroom keeps 16 KiB
// tails; same bound, head instead of tail — head preserves the command
// echo and early errors operators actually read).
const MaxStoredOutputBytes = 16 * 1024

// BoundOutput truncates s to the cap at a UTF-8 boundary, reporting
// whether truncation happened and the original byte length.
func BoundOutput(s string) (out string, truncated bool, original int) {
	original = len(s)
	if original <= MaxStoredOutputBytes {
		return s, false, original
	}
	cut := MaxStoredOutputBytes
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}
	return s[:cut], true, original
}
