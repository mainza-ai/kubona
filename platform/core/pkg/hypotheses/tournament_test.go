package hypotheses

import (
	"strings"
	"testing"
)

func competitor(id, statement, falsifier, kind string) Competitor {
	return Competitor{ID: id, Candidate: Candidate{
		Statement: statement, Falsifier: falsifier, ExperimentKind: kind,
		ExperimentHint: "run it", Score: 0.5, Source: "diagnostic",
	}}
}

func TestRunTournamentRanksCheckableProbesAndRecordsMatches(t *testing.T) {
	in := []Competitor{
		competitor("h1", "Maybe the ansatz is interesting.", "it might change", "vqe"),
		competitor("h2", "The ansatz is under-expressive.", "Increasing layers leaves the error above 1.6 mHa.", "vqe"),
		competitor("h3", "We proved the optimizer is trapped.", "The optimizer is trapped.", "vqe"),
		competitor("h4", "Shot noise explains the residual.", "Quadrupling shots shrinks the interval by less than 20%.", "vqe"),
	}
	got := RunTournament(in, 3, nil)
	if len(got.Ranking) != 4 {
		t.Fatalf("ranking = %d", len(got.Ranking))
	}
	if len(got.Matches) == 0 {
		t.Fatal("no matches recorded")
	}
	if got.Judge != "heuristic_v1" || got.Rounds != 3 {
		t.Fatalf("metadata = %+v", got)
	}
	topTwo := map[string]bool{got.Ranking[0].ID: true, got.Ranking[1].ID: true}
	if !topTwo["h2"] || !topTwo["h4"] {
		t.Fatalf("top two = %v, want the two checkable probes h2/h4: %+v", topTwo, got.Ranking)
	}
	if got.Ranking[len(got.Ranking)-1].ID != "h3" {
		t.Fatalf("bottom = %q, want h3 (claim language + restated falsifier)",
			got.Ranking[len(got.Ranking)-1].ID)
	}
	for _, c := range got.Ranking {
		if c.Candidate.Score < 0 || c.Candidate.Score > 1 {
			t.Fatalf("score out of range: %+v", c)
		}
	}
	for _, m := range got.Matches {
		if m.Winner != m.AID && m.Winner != m.BID {
			t.Fatalf("winner is not a participant: %+v", m)
		}
		if m.Reason == "" {
			t.Fatalf("match without a reason: %+v", m)
		}
	}
	if !strings.Contains(got.MetaReview, "judge=heuristic_v1") ||
		!strings.Contains(got.MetaReview, "Top probes") ||
		!strings.Contains(got.MetaReview, "Lowest-ranked") {
		t.Fatalf("meta review incomplete: %s", got.MetaReview)
	}
}

func TestRunTournamentIsDeterministicAndSymmetric(t *testing.T) {
	in := []Competitor{
		competitor("a", "Probe A.", "A residual above 2 mHa after four shots.", "vqe"),
		competitor("b", "Probe B.", "A residual above 2 mHa after four shots.", "vqe"),
	}
	first := RunTournament(in, 2, nil)
	second := RunTournament(in, 2, nil)
	if first.Ranking[0].ID != second.Ranking[0].ID {
		t.Fatalf("non-deterministic ranking: %v vs %v", first.Ranking, second.Ranking)
	}
	if first.Ranking[0].Candidate.Score != second.Ranking[0].Candidate.Score {
		t.Fatalf("non-deterministic score: %v vs %v",
			first.Ranking[0].Candidate.Score, second.Ranking[0].Candidate.Score)
	}
	// Equal-quality probes split the matches: neither can dominate.
	for _, c := range first.Ranking {
		if c.Candidate.Score < 0.35 || c.Candidate.Score > 0.65 {
			t.Fatalf("symmetric probes drifted: %+v", first.Ranking)
		}
	}
}

func TestRunTournamentHandlesASingleCandidate(t *testing.T) {
	got := RunTournament([]Competitor{competitor("only", "One probe.", "One check at 1 mHa.", "vqe")}, 2, nil)
	if len(got.Ranking) != 1 || got.Matches != nil {
		t.Fatalf("single-candidate tournament = %+v", got)
	}
	if !strings.Contains(got.MetaReview, "fewer than two") {
		t.Fatalf("meta review = %q", got.MetaReview)
	}
}
