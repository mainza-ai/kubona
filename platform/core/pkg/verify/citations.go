package verify

// Citation verification (AG-5).
//
// A citation is verified only against a resolved external record: the DOI must
// resolve to the same work, or (when no DOI is supplied) the title must match a
// resolved record closely. A DOI that resolves to a different work is a dead
// citation even if the title looks right, because the reader would land on the
// wrong paper.
//
// Resolution goes through a Resolver (the bridge's literature search in
// production). A resolver failure is "unverified", never "verified": the
// manuscript cannot claim evidence the system did not obtain.

import (
	"context"
	"fmt"
	"strings"
)

// Citation is one reference bound to the manuscript.
type Citation struct {
	Key   string
	Title string
	DOI   string
}

// ResolvedWork is one external record returned by a resolver.
type ResolvedWork struct {
	ID    string
	Title string
}

// Resolver looks up candidate works for a citation query.
type Resolver interface {
	Resolve(ctx context.Context, query string) ([]ResolvedWork, error)
}

const (
	CitationVerifiedDOI     = "verified_doi"
	CitationVerifiedTitle   = "verified_title"
	CitationDOIMismatch     = "doi_mismatch"
	CitationUnmatched       = "unmatched"
	CitationMissingIdentify = "missing_identifier"
	CitationUnverified      = "unverified"
)

// CitationFinding is the outcome for one citation.
type CitationFinding struct {
	Key          string  `json:"key"`
	DOI          string  `json:"doi,omitempty"`
	Title        string  `json:"title,omitempty"`
	Status       string  `json:"status"`
	MatchedID    string  `json:"matched_id,omitempty"`
	MatchedTitle string  `json:"matched_title,omitempty"`
	Similarity   float64 `json:"similarity,omitempty"`
	Error        string  `json:"error,omitempty"`
	Note         string  `json:"note,omitempty"`
}

// Blocking reports whether the status must stop publication without an
// operator override. An unavailable resolver does not block by itself: it is
// acknowledged, not silently accepted.
func (f CitationFinding) Blocking() bool {
	switch f.Status {
	case CitationDOIMismatch, CitationUnmatched, CitationMissingIdentify:
		return true
	}
	return false
}

// Unverified reports whether the status needs an explicit acknowledgment.
func (f CitationFinding) Unverified() bool { return f.Status == CitationUnverified }

// VerifyCitations resolves and classifies every citation.
func VerifyCitations(ctx context.Context, citations []Citation, resolver Resolver) []CitationFinding {
	out := make([]CitationFinding, 0, len(citations))
	for _, c := range citations {
		f := CitationFinding{Key: c.Key, DOI: c.DOI, Title: c.Title}
		title := strings.TrimSpace(c.Title)
		doi := NormalizeDOI(c.DOI)
		if title == "" && doi == "" {
			f.Status = CitationMissingIdentify
			f.Note = "citation has neither a DOI nor a title"
			out = append(out, f)
			continue
		}
		if resolver == nil {
			f.Status = CitationUnverified
			f.Error = "no citation resolver is configured"
			out = append(out, f)
			continue
		}
		// DOI-first lookup: when a DOI is supplied, resolve by DOI first so
		// exact identifiers aren't drowned out by generic title search hits;
		// fall back to title lookup if the DOI query returns no hits.
		query := title
		if doi != "" {
			query = c.DOI
		}
		works, err := resolver.Resolve(ctx, query)
		if err != nil {
			f.Status = CitationUnverified
			f.Error = fmt.Sprintf("resolver failed: %v", err)
			out = append(out, f)
			continue
		}
		if len(works) == 0 && doi != "" && title != "" {
			if fallbackWorks, ferr := resolver.Resolve(ctx, title); ferr == nil {
				works = fallbackWorks
			}
		}
		if doi != "" {
			for _, w := range works {
				if NormalizeDOI(w.ID) == doi {
					sim := titleSimilarity(title, w.Title)
					if title != "" && sim < 0.40 {
						f.Status = CitationDOIMismatch
						f.MatchedID, f.MatchedTitle, f.Similarity = w.ID, w.Title, sim
						f.Note = "DOI resolved, but its title does not match the cited title"
					} else {
						f.Status = CitationVerifiedDOI
						f.MatchedID, f.MatchedTitle, f.Similarity = w.ID, w.Title, sim
					}
					break
				}
			}
			if f.Status == "" {
				f.Status = CitationDOIMismatch
				if best, sim := bestTitle(works, title); sim >= 0.85 {
					f.MatchedID, f.MatchedTitle, f.Similarity = best.ID, best.Title, sim
					f.Note = "a record with this title exists, but not at the supplied DOI"
				} else {
					f.Note = "no resolved record carries this DOI"
				}
			}
			out = append(out, f)
			continue
		}
		best, sim := bestTitle(works, title)
		switch {
		case sim >= 0.85:
			f.Status = CitationVerifiedTitle
			f.MatchedID, f.MatchedTitle, f.Similarity = best.ID, best.Title, sim
		default:
			f.Status = CitationUnmatched
			f.Similarity = sim
			f.Note = "no resolved title matched closely enough"
		}
		out = append(out, f)
	}
	return out
}

// NormalizeDOI strips common resolver prefixes and lower-cases the identifier.
func NormalizeDOI(doi string) string {
	d := strings.ToLower(strings.TrimSpace(doi))
	for _, prefix := range []string{
		"https://doi.org/", "http://doi.org/", "https://dx.doi.org/",
		"http://dx.doi.org/", "doi:", "doi ",
	} {
		d = strings.TrimPrefix(d, prefix)
	}
	return strings.TrimSpace(d)
}

func bestTitle(works []ResolvedWork, title string) (ResolvedWork, float64) {
	var best ResolvedWork
	bestSim := 0.0
	for _, w := range works {
		if sim := titleSimilarity(title, w.Title); sim > bestSim {
			best, bestSim = w, sim
		}
	}
	return best, bestSim
}

// titleSimilarity is containment of the citation title's content tokens in the
// resolved title; it is lexical, not semantic, and the report says so.
func titleSimilarity(a, b string) float64 {
	at := contentTokens(a)
	bt := contentTokens(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	shared := 0
	for t := range at {
		if bt[t] {
			shared++
		}
	}
	containment := float64(shared) / float64(len(at))
	recall := float64(shared) / float64(len(bt))
	// Reject degenerate 1-token spoofing against a long resolved title while
	// preserving legitimate shortened titles (e.g. missing subtitle).
	if len(at) == 1 && len(bt) > 2 {
		return recall
	}
	if recall < 0.40 {
		return 2.0 * float64(shared) / float64(len(at)+len(bt))
	}
	return containment
}

var citationStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true,
	"that": true, "this": true, "are": true, "was": true, "were": true,
	"into": true, "using": true, "via": true, "its": true, "their": true,
}

func contentTokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(t) > 2 && !citationStopwords[t] {
			out[t] = true
		}
	}
	return out
}
