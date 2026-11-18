package hypotheses

import (
	"context"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

type fakeCompleter struct {
	reply string
	err   error
	last  []llm.ChatMessage
	calls int
}

func (f *fakeCompleter) Complete(_ context.Context, _ string, messages []llm.ChatMessage) (string, error) {
	f.calls++
	f.last = messages
	return f.reply, f.err
}

func validProposalJSON() string {
	return "```json\n" + `{"hypotheses":[
	  {"statement":"Shot noise, not ansatz expressivity, explains the residual.",
	   "rationale":"The measured point carries a finite-shot interval.",
	   "falsifier":"Quadrupling shots leaves the residual above 1.6 mHa.",
	   "experiment_kind":"vqe","experiment_hint":"re-run with 4x shots"},
	  {"statement":"The optimizer converges to a local minimum.",
	   "rationale":"The measured energy is above the anchor.",
	   "falsifier":"Three seeded restarts do not lower the energy by 0.1 mHa.",
	   "experiment_kind":"vqe_repeats","experiment_hint":"run 3 seeds"}
	]}` + "\n```"
}

func TestProposeValidatesAndLabelsLLMCandidates(t *testing.T) {
	fake := &fakeCompleter{reply: validProposalJSON()}
	out, err := Propose(context.Background(), fake, ProposeOptions{
		Goal: "Improve H2", Target: "H2", Measured: true, DeltaMHa: 4.5,
		Literature: []PriorArt{{ID: "W1", Title: "H2 ground state"}}, Count: 12,
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("candidates = %d", len(out))
	}
	for _, c := range out {
		if c.Source != "llm" {
			t.Fatalf("source = %q", c.Source)
		}
		if c.Score != NeutralLLMScore {
			t.Fatalf("score = %v, want neutral", c.Score)
		}
		if c.Falsifier == "" || c.ExperimentHint == "" {
			t.Fatalf("incomplete candidate: %+v", c)
		}
	}
	user := fake.last[len(fake.last)-1].Content
	if !strings.Contains(user, "4.500 mHa") || !strings.Contains(user, "H2 ground state") {
		t.Fatalf("prompt lost the measured facts: %s", user)
	}
	system := fake.last[0].Content
	if !strings.Contains(system, "Never assert novelty") ||
		!strings.Contains(system, "A hypothesis is a probe, never a claim") {
		t.Fatalf("system prompt lost the contract: %s", system)
	}
}

func TestProposeRejectsStructuralErrorsWholesale(t *testing.T) {
	cases := map[string]string{
		"no json":       "I think the answer is yes.",
		"empty":         `{"hypotheses":[]}`,
		"missing fals":  `{"hypotheses":[{"statement":"s","experiment_kind":"vqe"}]}`,
		"unknown kind":  `{"hypotheses":[{"statement":"s","falsifier":"f","experiment_kind":"telepathy"}]}`,
		"unknown field": `{"hypotheses":[{"statement":"s","falsifier":"f","experiment_kind":"vqe","confidence":0.9}]}`,
		"not an object": `[]`,
	}
	for name, reply := range cases {
		fake := &fakeCompleter{reply: reply}
		if _, err := Propose(context.Background(), fake, ProposeOptions{Count: 3}); err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
	}
}

func TestProposeNormalizesKindAliasesAndCapsCount(t *testing.T) {
	reply := `{"hypotheses":[
	  {"statement":"a","falsifier":"f1","experiment_kind":"mitigation"},
	  {"statement":"b","falsifier":"f2","experiment_kind":"classical_anchor"},
	  {"statement":"c","falsifier":"f3","experiment_kind":"vqe"}
	]}`
	out, err := Propose(context.Background(), &fakeCompleter{reply: reply}, ProposeOptions{Count: 2})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("count cap not applied: %d", len(out))
	}
	if out[0].ExperimentKind != "mitigate" || out[1].ExperimentKind != "anchor" {
		t.Fatalf("aliases not normalized: %+v", out)
	}
}

func TestMergeReservesSlotsForModelProposals(t *testing.T) {
	primary := []Candidate{
		{Statement: "Diagnostic one.", Falsifier: "f", Score: 0.9, Source: "diagnostic"},
		{Statement: "Diagnostic two.", Falsifier: "f", Score: 0.8, Source: "diagnostic"},
		{Statement: "Diagnostic three.", Falsifier: "f", Score: 0.7, Source: "diagnostic"},
	}
	secondary := []Candidate{
		{Statement: "Model one.", Falsifier: "f", Score: 0.5, Source: "llm"},
		{Statement: "Model two.", Falsifier: "f", Score: 0.5, Source: "llm"},
	}
	out := Merge(primary, secondary, 3)
	if len(out) != 3 {
		t.Fatalf("merged = %d", len(out))
	}
	llmRows := 0
	for _, c := range out {
		if c.Source == "llm" {
			llmRows++
		}
	}
	if llmRows < 1 {
		t.Fatalf("no model proposal survived the cap: %+v", out)
	}
}

func TestMergeDedupesAndKeepsHigherScoringVersion(t *testing.T) {
	diagnostic := []Candidate{
		{Statement: "The optimizer is trapped in a local minimum for H2.",
			Falsifier: "restarts unchanged", Score: 0.7, Source: "diagnostic"},
		{Statement: "Shot noise explains the H2 discrepancy.",
			Falsifier: "4x shots unchanged", Score: 0.6, Source: "diagnostic"},
	}
	llmOut := []Candidate{
		{Statement: "The optimizer is trapped in a local minimum for H2.",
			Falsifier: "three seeded restarts do not lower the energy", Score: 0.5, Source: "llm"},
		{Statement: "A larger active space would move the residual.",
			Falsifier: "next tier unchanged", Score: 0.5, Source: "llm"},
	}
	out := Merge(diagnostic, llmOut, 10)
	if len(out) != 3 {
		t.Fatalf("merged = %d: %+v", len(out), out)
	}
	if out[0].Score != 0.7 || out[len(out)-1].Source != "llm" {
		t.Fatalf("order/identity lost: %+v", out)
	}
	seen := map[string]int{}
	for _, c := range out {
		seen[c.Statement]++
	}
	for s, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate survived: %q x%d", s, n)
		}
	}
}
