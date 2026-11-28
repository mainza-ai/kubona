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
	"has": true, "have": true, "had": true, "but": true,
	"does": true, "did": true, "can": true, "could": true, "would": true,
}

// polarityNegators preserves negation polarity (M0 / Doc 2 Gap #1) so
// "X was effective" and "X was not effective" (or "no improvement") never
// collide onto the same SHA-256 fingerprint.
var polarityNegators = map[string]bool{
	"not": true, "no": true, "never": true, "none": true, "without": true,
	"cannot": true, "cant": true, "dont": true, "doesnt": true, "didnt": true,
	"wont": true, "failed": true, "fails": true, "failing": true, "worse": true,
	"ineffective": true, "unable": true, "refuted": true, "false": true,
}

func rawWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
}

// HasNegation reports whether s contains an explicit negation or negative
// outcome polarity token.
func HasNegation(s string) bool {
	for _, t := range rawWords(s) {
		if polarityNegators[t] {
			return true
		}
	}
	return false
}

// Tokens returns the sorted content tokens of a statement, preserving negation
// markers such as "not" and "no".
func Tokens(s string) []string {
	seen := map[string]bool{}
	for _, t := range rawWords(s) {
		if polarityNegators[t] || (len(t) > 2 && !stopwords[t]) {
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

// Fingerprint is the polarity-aware SHA-256 of the normalised statement.
// Affirmations and negations of the same predicate always produce distinct hashes.
func Fingerprint(s string) string {
	pol := "pos:"
	if HasNegation(s) {
		pol = "neg:"
	}
	sum := sha256.Sum256([]byte(pol + Normalize(s)))
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
