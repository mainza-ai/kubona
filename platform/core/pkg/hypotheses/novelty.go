package hypotheses

// Measured novelty (AG-2b).
//
// Novelty is a measurement, never an assertion. This file implements one
// explicit, reproducible measure: lexical containment of a hypothesis
// statement against the retrieved prior-art records. If no prior art was
// retrieved, the caller must leave novelty "unmeasured" — an empty corpus is
// not evidence of novelty.
//
// The measure is deliberately lexical, not semantic: it is cheap, deterministic,
// and auditable. It answers "does a retrieved title/abstract already contain
// this statement's concepts", not "is this idea new to science". The method
// name is persisted with the distance so nobody later reads it as a semantic
// novelty score.

import (
	"fmt"
	"strings"
)

// NoveltyMethodLexicalContainmentV1 names the measure persisted alongside the
// distance. A different measure must use a different name.
const NoveltyMethodLexicalContainmentV1 = "lexical_containment_v1"

// structuralDomains cannot be certified by lexical token overlap (Phase 0 B6).
// Novelty for codes, circuits, schedules, and decoders requires structural
// canonicalization + catalog comparison.
var structuralDomains = map[string]bool{
	"qec":          true,
	"qec_code":     true,
	"code_novelty": true,
	"circuit":      true,
	"schedule":     true,
	"decoder":      true,
}

// IsStructuralDomain reports whether novelty in the given domain requires
// structural canonicalization rather than lexical containment.
func IsStructuralDomain(domain string) bool {
	return structuralDomains[strings.ToLower(strings.TrimSpace(domain))]
}

// MeasureNoveltyForDomain refuses lexical novelty certification for structural
// domains (B6) and delegates text/literature hypotheses to MeasureNovelty.
func MeasureNoveltyForDomain(domain, statement string, refs []PriorArt) (distance float64, method string, ok bool, err error) {
	if IsStructuralDomain(domain) {
		return 0, "", false, fmt.Errorf("lexical novelty cannot certify structural domain %q: canonical form and catalog comparison required", strings.TrimSpace(domain))
	}
	d, m, measured := MeasureNovelty(statement, refs)
	return d, m, measured, nil
}

// PriorArt is one retrieved record the statement is compared against.
type PriorArt struct {
	ID       string
	Title    string
	Abstract string
}

// MeasureNovelty returns the distance in [0,1] from the closest retrieved
// record and the method that produced it. A distance of 0 means every content
// token of the statement appears in one record; 1 means none do. It reports
// ok=false when no record carries text, because a distance against nothing is
// not a measurement.
func MeasureNovelty(statement string, refs []PriorArt) (distance float64, method string, ok bool) {
	s := contentTokens(statement)
	if len(s) == 0 {
		return 0, "", false
	}
	best := 0.0
	found := false
	for _, ref := range refs {
		text := strings.TrimSpace(ref.Title + " " + ref.Abstract)
		if text == "" {
			continue
		}
		r := contentTokens(text)
		if len(r) == 0 {
			continue
		}
		shared := 0
		for tok := range s {
			if r[tok] {
				shared++
			}
		}
		containment := float64(shared) / float64(len(s))
		if !found || containment > best {
			best = containment
			found = true
		}
	}
	if !found {
		return 0, "", false
	}
	return round4(1 - best), NoveltyMethodLexicalContainmentV1, true
}

var noveltyStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "with": true,
	"from": true, "this": true, "these": true, "those": true, "are": true,
	"was": true, "were": true, "not": true, "but": true, "have": true,
	"has": true, "had": true, "its": true, "their": true, "our": true,
	"can": true, "could": true, "would": true, "should": true, "may": true,
	"into": true, "than": true, "then": true, "when": true, "where": true,
	"which": true, "while": true, "does": true, "did": true, "been": true,
	"being": true, "each": true, "more": true, "most": true, "such": true,
}

// contentTokens lower-cases, strips punctuation, drops stopwords and short
// tokens. It is shared with the dedupe similarity so both use one definition
// of "the same words".
func contentTokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(t) > 2 && !noveltyStopwords[t] {
			out[t] = true
		}
	}
	return out
}

func round4(v float64) float64 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return float64(int64(v*10000+0.5)) / 10000
}
