package researchgraph

import (
	"strconv"
	"strings"
	"testing"
)

func largeGraph() Graph {
	delta := 4.56
	claimable := true
	accepted := true
	unaccepted := false
	g := Graph{
		SessionID: "sess-1", Goal: "Improve the H2 ground-state reproduction",
		Target: "Hydrogen Molecule (H2)", DeltaMHa: &delta, Claimable: &claimable,
		ManuscriptCount: 2,
	}
	for i := 0; i < 12; i++ {
		g.Hypotheses = append(g.Hypotheses, Hypothesis{
			ID:        "h" + strconv.Itoa(i),
			Statement: "The candidate mechanism number " + strconv.Itoa(i) + " explains the residual with a long explanatory statement.",
			Falsifier: "A re-run with a larger budget leaves the residual above 1.6 mHa.",
			Status:    "proposed", Score: 0.9 - float64(i)*0.05,
		})
	}
	g.Hypotheses = append(g.Hypotheses, Hypothesis{
		ID: "hr", Statement: "Rejected probe.", Falsifier: "f", Status: "rejected", Score: 0.1,
	})
	g.Plans = append(g.Plans,
		Plan{ID: "p1", Statement: "Run the expressivity probe.", Status: "completed",
			Observable: "energy", AcceptanceMetric: "fci_exact_energy", Accepted: &accepted},
		Plan{ID: "p2", Statement: "Run the shot-noise probe.", Status: "rejected",
			Observable: "energy", AcceptanceMetric: "fci_exact_energy", Accepted: &unaccepted},
	)
	g.Runs = append(g.Runs,
		Run{ID: "j-old", Type: "vqe_run", Status: "JOB_STATUS_COMPLETED", CreatedAt: "2026-11-01T00:00:00Z"},
		Run{ID: "j-new", Type: "spin_chain", Status: "JOB_STATUS_FAILED", CreatedAt: "2026-11-02T00:00:00Z"},
	)
	g.Claims = append(g.Claims,
		Claim{ID: "clm-1", Statement: "H2 measured", Verdict: "verified"},
		Claim{ID: "clm-2", Statement: "H2 refuted", Verdict: "refuted"},
	)
	return g
}

func TestSummarizeRespectsBudgetAndReportsOmissions(t *testing.T) {
	text, rep := Summarize(largeGraph(), 900)
	if len(text) > 900 {
		t.Fatalf("summary = %d bytes, budget 900", len(text))
	}
	if rep.Bytes != len(text) || rep.BudgetBytes != 900 {
		t.Fatalf("report bytes = %+v", rep)
	}
	if !rep.Truncated || len(rep.Omitted) == 0 {
		t.Fatalf("budget did not report omissions: %+v", rep)
	}
	if !strings.Contains(text, "Open hypotheses") {
		t.Fatalf("priority section missing:\n%s", text)
	}
	if !containsString(rep.Included, "objective") {
		t.Fatalf("objective not included: %+v", rep)
	}
	// Tight budget must keep the objective and drop the tail sections.
	tight, tightRep := Summarize(largeGraph(), 512)
	if len(tight) > 512 {
		t.Fatalf("tight summary = %d bytes", len(tight))
	}
	if !strings.Contains(tight, "Research graph") || !containsString(tightRep.Omitted, "manuscripts") {
		t.Fatalf("tight summary lost priority: %s (%+v)", tight, tightRep)
	}
}

func TestSummarizeIsDeterministicAndCarriesNegativesAndOrder(t *testing.T) {
	a, ra := Summarize(largeGraph(), 20000)
	b, rb := Summarize(largeGraph(), 20000)
	if a != b {
		t.Fatal("summary is not deterministic")
	}
	if ra.Bytes != rb.Bytes || len(ra.Included) != len(rb.Included) {
		t.Fatalf("report differs: %+v vs %+v", ra, rb)
	}
	if !strings.Contains(a, "Negatives") || !strings.Contains(a, "Rejected probe.") ||
		!strings.Contains(a, "plan not accepted: p2") {
		t.Fatalf("negatives missing:\n%s", a)
	}
	if strings.Index(a, "j-new") > strings.Index(a, "j-old") {
		t.Fatalf("runs not newest-first:\n%s", a)
	}
	if !strings.Contains(a, "verified=1") || !strings.Contains(a, "refuted=1") {
		t.Fatalf("claim counts missing:\n%s", a)
	}
}

func TestSummarizeHandlesAnEmptyGraph(t *testing.T) {
	text, rep := Summarize(Graph{SessionID: "s"}, 4096)
	if !strings.Contains(text, "not stated") {
		t.Fatalf("empty objective not stated:\n%s", text)
	}
	if rep.Truncated || len(rep.Omitted) != 0 {
		t.Fatalf("empty graph reported omissions: %+v", rep)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
