package hypotheses

// Tournament ranking and meta-review (AG-2b).
//
// The generator ranks candidates by a static heuristic. A tournament makes the
// ranking comparative: candidates are paired, a judge states which probe is
// better, and an Elo update turns those decisions into persisted scores. The
// judge is an explicit, documented heuristic — not a claim of scientific truth.
//
// Meta-review is derived from the match record (who beat whom, and why), so it
// cannot invent a reason that was not used. An LLM judge can implement the
// same Judge interface later; the persisted scores would then be labelled with
// that judge's name, keeping the provenance honest.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Judge decides which of two candidates is the better probe.
type Judge interface {
	Better(a, b Candidate) bool
	Name() string
}

// Match is one judged comparison.
type Match struct {
	Round  int    `json:"round"`
	AID    string `json:"a_id"`
	BID    string `json:"b_id"`
	Winner string `json:"winner"`
	Reason string `json:"reason"`
}

// Competitor binds a persisted hypothesis id to its candidate content.
type Competitor struct {
	ID        string    `json:"id"`
	Candidate Candidate `json:"candidate"`
}

// TournamentResult is the comparable ranking plus its audit trail.
type TournamentResult struct {
	Ranking    []Competitor `json:"ranking"`
	Matches    []Match      `json:"matches"`
	Judge      string       `json:"judge"`
	Rounds     int          `json:"rounds"`
	MetaReview string       `json:"meta_review"`
}

const (
	eloBase  = 1500.0
	eloScale = 400.0
	eloK     = 24.0
)

// RunTournament pairs competitors over the requested rounds (default 2, max 6)
// and returns a ranking whose Candidate.Score is the Elo-derived [0,1] score.
func RunTournament(in []Competitor, rounds int, judge Judge) TournamentResult {
	if judge == nil {
		judge = HeuristicJudge{}
	}
	if rounds <= 0 {
		rounds = 2
	}
	if rounds > 6 {
		rounds = 6
	}
	competitors := append([]Competitor(nil), in...)
	if len(competitors) < 2 {
		for i := range competitors {
			competitors[i].Candidate.Score = clamp(competitors[i].Candidate.Score)
		}
		return TournamentResult{
			Ranking: competitors, Judge: judge.Name(), Rounds: 0,
			MetaReview: "Tournament skipped: fewer than two candidates.",
		}
	}
	sort.SliceStable(competitors, func(i, j int) bool { return competitors[i].ID < competitors[j].ID })

	ratings := make(map[string]float64, len(competitors))
	for _, c := range competitors {
		ratings[c.ID] = eloBase + (clamp(c.Candidate.Score)-0.5)*eloScale
	}
	matches := make([]Match, 0, rounds*len(competitors)/2)
	for round := 1; round <= rounds; round++ {
		order := append([]Competitor(nil), competitors...)
		sort.SliceStable(order, func(i, j int) bool {
			if ratings[order[i].ID] == ratings[order[j].ID] {
				return order[i].ID < order[j].ID
			}
			return ratings[order[i].ID] > ratings[order[j].ID]
		})
		for i := 0; i+1 < len(order); i += 2 {
			a, b := order[i], order[i+1]
			winner := a.ID
			if judge.Better(b.Candidate, a.Candidate) {
				winner = b.ID
			}
			applyElo(ratings, a.ID, b.ID, winner)
			matches = append(matches, Match{
				Round: round, AID: a.ID, BID: b.ID, Winner: winner,
				Reason: compareReason(a, b, judge, winner),
			})
		}
	}

	for i := range competitors {
		competitors[i].Candidate.Score = round4(logistic(ratings[competitors[i].ID]))
	}
	sort.SliceStable(competitors, func(i, j int) bool {
		if competitors[i].Candidate.Score == competitors[j].Candidate.Score {
			return competitors[i].ID < competitors[j].ID
		}
		return competitors[i].Candidate.Score > competitors[j].Candidate.Score
	})
	return TournamentResult{
		Ranking: competitors, Matches: matches, Judge: judge.Name(),
		Rounds: rounds, MetaReview: metaReview(competitors, matches, judge.Name(), rounds),
	}
}

func applyElo(ratings map[string]float64, a, b, winner string) {
	ra, rb := ratings[a], ratings[b]
	ea := 1 / (1 + math.Pow(10, (rb-ra)/eloScale))
	sa := 0.0
	if winner == a {
		sa = 1
	}
	ratings[a] = ra + eloK*(sa-ea)
	ratings[b] = rb + eloK*((1-sa)-(1-ea))
}

func logistic(rating float64) float64 {
	return 1 / (1 + math.Pow(10, (eloBase-rating)/eloScale))
}

func compareReason(a, b Competitor, judge Judge, winner string) string {
	sa, sb := heuristicQuality(a.Candidate), heuristicQuality(b.Candidate)
	switch {
	case winner == a.ID && sa > sb:
		return fmt.Sprintf("%s wins: stronger falsifier/grounding (%.1f vs %.1f)", judge.Name(), sa, sb)
	case winner == b.ID && sb > sa:
		return fmt.Sprintf("%s wins: stronger falsifier/grounding (%.1f vs %.1f)", judge.Name(), sb, sa)
	}
	return fmt.Sprintf("%s wins on the deterministic tiebreak", judge.Name())
}

func metaReview(ranking []Competitor, matches []Match, judge string, rounds int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tournament over %d hypotheses (%d matches, %d rounds, judge=%s).\n",
		len(ranking), len(matches), rounds, judge)
	top := ranking
	if len(top) > 3 {
		top = top[:3]
	}
	b.WriteString("Top probes:\n")
	for i, c := range top {
		fmt.Fprintf(&b, "  %d. [%.2f] %s\n", i+1, c.Candidate.Score, truncate(c.Candidate.Statement, 160))
	}
	weak := ranking
	if len(weak) > 3 {
		weak = weak[len(weak)-3:]
	}
	if len(weak) > 0 && len(matches) > 0 {
		b.WriteString("Lowest-ranked probes and the match that exposed them:\n")
		for _, c := range weak {
			reason := ""
			for i := len(matches) - 1; i >= 0; i-- {
				if matches[i].Winner != c.ID && (matches[i].AID == c.ID || matches[i].BID == c.ID) {
					reason = matches[i].Reason
					break
				}
			}
			if reason == "" {
				reason = "no loss recorded in this tournament"
			}
			fmt.Fprintf(&b, "  - %s: %s\n", truncate(c.Candidate.Statement, 120), reason)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// HeuristicJudge v1 scores each probe on falsifier checkability, executable
// grounding, rationale specificity, and claim-language avoidance. Ties break
// deterministically so the same input always produces the same ranking.
type HeuristicJudge struct{}

func (HeuristicJudge) Name() string { return "heuristic_v1" }

func (HeuristicJudge) Better(a, b Candidate) bool {
	sa, sb := heuristicQuality(a), heuristicQuality(b)
	if sa != sb {
		return sa > sb
	}
	if len(a.Falsifier) != len(b.Falsifier) {
		return len(a.Falsifier) > len(b.Falsifier)
	}
	return a.Statement < b.Statement
}

var checkableTerms = []string{
	"mha", "%", "within", "above", "below", "match", "matches", "unchanged",
	"increase", "decrease", "shrink", "does not", "refus", "error", "tolerance",
	"interval", "seed", "calibration", "anchor", "shots",
}

var claimWords = []string{
	"we proved", "we discovered", "demonstrates that", "confirms that",
	"proves that", "establishes that", "is the first",
}

var vagueWords = []string{"maybe", "possibly", "interesting", "somehow", "various"}

func heuristicQuality(c Candidate) float64 {
	score := 0.0
	falsifier := strings.ToLower(c.Falsifier)
	if strings.ContainsAny(falsifier, "0123456789") || containsAny(falsifier, checkableTerms) {
		score += 2
	}
	if NormalizeKindExecutable(c.ExperimentKind) {
		score++
	}
	if strings.TrimSpace(c.ExperimentHint) != "" {
		score++
	}
	rationale := strings.ToLower(c.Rationale)
	if containsAny(rationale, []string{"measured", "anchor", "delta", "shots", "calibration", "claimable"}) {
		score++
	}
	statement := strings.ToLower(c.Statement)
	if containsAny(statement, claimWords) {
		score -= 2
	}
	if containsAny(statement, vagueWords) {
		score--
	}
	if c.NoveltyStatus == "measured" {
		score += 0.5
	}
	if len(c.Literature) > 0 {
		score += 0.5
	}
	return score
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
