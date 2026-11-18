package verify

// Verification report (AG-5).
//
// The report is the durable artifact signoff cites. Its status is a closed
// vocabulary: "verified" (everything checked and supported), "blocked" (at
// least one fabricated number or dead citation), "unverified" (the evidence or
// resolver was unavailable, so nothing was confirmed). A report never mixes
// "unverified" into "verified": the operator must acknowledge it.

import (
	"encoding/json"
	"time"
)

// ReportVersion names the rule set that produced the report.
const ReportVersion = "verification-v1"

// Summary is the count-and-verdict projection of a report.
type Summary struct {
	Status              string `json:"status"`
	NumbersChecked      int    `json:"numbers_checked"`
	NumbersMatched      int    `json:"numbers_matched"`
	NumbersUnmatched    int    `json:"numbers_unmatched"`
	NumbersUnverifiable int    `json:"numbers_unverifiable"`
	CitationsChecked    int    `json:"citations_checked"`
	CitationsVerified   int    `json:"citations_verified"`
	CitationsBlocking   int    `json:"citations_blocking"`
	CitationsUnverified int    `json:"citations_unverified"`
	Blocking            int    `json:"blocking"`
	Unverified          int    `json:"unverified"`
}

// Report is the full verification result for one manuscript content hash.
type Report struct {
	Version     string            `json:"version"`
	ContentHash string            `json:"content_hash"`
	CheckedAt   string            `json:"checked_at"`
	Numbers     []NumericFinding  `json:"numbers"`
	Citations   []CitationFinding `json:"citations"`
	Summary     Summary           `json:"summary"`
}

// BuildReport classifies and counts findings. The content hash binds the
// report to exactly one draft; a content change makes it stale by definition.
func BuildReport(contentHash string, numbers []NumericFinding, citations []CitationFinding, now time.Time) Report {
	rep := Report{
		Version: ReportVersion, ContentHash: contentHash,
		CheckedAt: now.UTC().Format(time.RFC3339),
		Numbers:   numbers, Citations: citations,
	}
	rep.Summary.Status = "verified"
	for _, n := range numbers {
		rep.Summary.NumbersChecked++
		switch n.Status {
		case NumberMatched:
			rep.Summary.NumbersMatched++
		case NumberUnmatched:
			rep.Summary.NumbersUnmatched++
			rep.Summary.Blocking++
		case NumberUnverifiable:
			rep.Summary.NumbersUnverifiable++
			rep.Summary.Unverified++
		}
	}
	for _, c := range citations {
		rep.Summary.CitationsChecked++
		switch {
		case c.Blocking():
			rep.Summary.CitationsBlocking++
			rep.Summary.Blocking++
		case c.Unverified():
			rep.Summary.CitationsUnverified++
			rep.Summary.Unverified++
		default:
			rep.Summary.CitationsVerified++
		}
	}
	switch {
	case rep.Summary.Blocking > 0:
		rep.Summary.Status = "blocked"
	case rep.Summary.Unverified > 0:
		rep.Summary.Status = "unverified"
	}
	return rep
}

// JSON renders the report for persistence and the API.
func (r Report) JSON() string {
	raw, _ := json.Marshal(r)
	return string(raw)
}

// BlockingDetails returns the findings that caused a "blocked" verdict, capped
// so a 409 stays readable.
func (r Report) BlockingDetails() []map[string]any {
	out := []map[string]any{}
	for _, n := range r.Numbers {
		if n.Status == NumberUnmatched {
			out = append(out, map[string]any{
				"kind": "number", "text": n.Text, "value": n.Value,
				"unit": n.Unit, "tolerance": n.Tolerance,
			})
			if len(out) >= 10 {
				return out
			}
		}
	}
	for _, c := range r.Citations {
		if c.Blocking() {
			out = append(out, map[string]any{
				"kind": "citation", "key": c.Key, "doi": c.DOI,
				"status": c.Status, "note": c.Note,
			})
			if len(out) >= 10 {
				return out
			}
		}
	}
	return out
}
