package knowledge

// Normalisation and similarity for the cumulative lab-knowledge registry
// (AG-9). A fingerprint makes publication idempotent; Jaccard similarity makes
// "have we already tried this and failed?" answerable without a model. Both are
// lexical and documented as such.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "are": true, "was": true, "were": true,
	"into": true, "using": true, "via": true, "its": true, "their": true,
	"has": true, "have": true, "had": true, "not": true, "but": true,
	"does": true, "did": true, "can": true, "could": true, "would": true,
}

// Tokens returns the sorted content tokens of a statement.
func Tokens(s string) []string {
	seen := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(t) > 2 && !stopwords[t] {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Normalize joins the sorted content tokens with single spaces.
func Normalize(s string) string { return strings.Join(Tokens(s), " ") }

// Fingerprint is the SHA-256 of the normalised statement. Two rewordings that
// share the same token set collide by design; that is the duplicate signal.
func Fingerprint(s string) string {
	sum := sha256.Sum256([]byte(Normalize(s)))
	return hex.EncodeToString(sum[:])
}

// Similarity is the Jaccard overlap of two statements' content tokens.
func Similarity(a, b string) float64 {
	at := map[string]bool{}
	for _, t := range Tokens(a) {
		at[t] = true
	}
	bt := map[string]bool{}
	for _, t := range Tokens(b) {
		bt[t] = true
	}
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	shared := 0
	for t := range at {
		if bt[t] {
			shared++
		}
	}
	union := len(at) + len(bt) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}
