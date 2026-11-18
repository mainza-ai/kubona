package prov

import (
	"strings"
	"testing"
)

func fullBundle() Bundle {
	return Bundle{
		SessionID: "s1", ManuscriptID: "ms1", Title: "H2 results",
		ContentHash: "h1", CreatedAt: "2026-11-18T10:00:00Z",
		Authors: []string{"Ada Lovelace", "Alan Turing"},
		Jobs: []Job{{
			ID: "job1", Type: "vqe_run", Status: "JOB_STATUS_COMPLETED",
			CreatedAt: "2026-11-18T10:01:00Z", CompletedAt: "2026-11-18T10:02:00Z",
		}},
		Claims: []Claim{{
			ID: "clm-1", Statement: "H2 measured", Verdict: "verified",
			CreatedAt: "2026-11-18T10:03:00Z", JobID: "job1",
		}},
		Hypotheses: []Hypothesis{{
			ID: "hyp1", Statement: "The ansatz is under-expressive", Status: "collapsed",
			Source: "llm", ClaimID: "clm-1",
		}},
		Plans: []Plan{{
			ID: "plan1", HypothesisID: "hyp1", Status: "completed", JobID: "job1",
		}},
		Verification: &Verification{Status: "verified", CheckedAt: "2026-11-18T10:04:00Z"},
		Signoff:      &Signoff{Signer: "abcdef0123456789", SignedAt: "2026-11-18T10:05:00Z", ContentHash: "h1"},
	}
}

func TestContributorsAttributeOnlyWhatTheLedgerRecords(t *testing.T) {
	got := Contributors(fullBundle())
	byType := map[string]Contributor{}
	for _, c := range got {
		byType[c.Type] = c
		if len(c.Roles) == 0 {
			t.Fatalf("contributor without CRediT roles: %+v", c)
		}
	}
	if len(got) != 6 {
		t.Fatalf("contributors = %d: %+v", len(got), got)
	}
	author := byType["declared-author"]
	if !strings.Contains(author.Note, "personhood is not asserted") {
		t.Fatalf("author note = %q", author.Note)
	}
	if byType["model"].ID != agentLLM {
		t.Fatalf("llm attribution = %+v", byType["model"])
	}
	if !strings.Contains(byType["human-operator"].Note, "abcdef012345") {
		t.Fatalf("operator note = %q", byType["human-operator"].Note)
	}

	noLLM := fullBundle()
	noLLM.Hypotheses[0].Source = "diagnostic"
	for _, c := range Contributors(noLLM) {
		if c.Type == "model" {
			t.Fatal("an LLM was attributed without an llm-source hypothesis")
		}
	}
}

func TestBuildPROVJSONIsReferentiallyConsistent(t *testing.T) {
	doc := BuildPROVJSON(fullBundle())
	entities := doc["entity"].(map[string]any)
	activities := doc["activity"].(map[string]any)
	agents := doc["agent"].(map[string]any)
	for _, want := range []string{
		"kubona:manuscript/ms1", "kubona:campaign/s1", "kubona:job/job1",
		"kubona:claim/clm-1", "kubona:hypothesis/hyp1", "kubona:plan/plan1",
		"kubona:verification/ms1",
	} {
		if _, ok := entities[want]; !ok {
			t.Fatalf("entity %q missing", want)
		}
	}
	for _, want := range []string{"kubona:compose/ms1", "kubona:run/job1", "kubona:verify/ms1", "kubona:sign/ms1"} {
		if _, ok := activities[want]; !ok {
			t.Fatalf("activity %q missing", want)
		}
	}
	for _, want := range []string{agentCore, agentBridge, agentOperator, agentLLM} {
		if _, ok := agents[want]; !ok {
			t.Fatalf("agent %q missing", want)
		}
	}
	checkRefs(t, doc, "wasGeneratedBy", "prov:entity", entities)
	checkRefs(t, doc, "wasGeneratedBy", "prov:activity", activities)
	checkRefs(t, doc, "used", "prov:entity", entities)
	checkRefs(t, doc, "used", "prov:activity", activities)
	checkRefs(t, doc, "wasAssociatedWith", "prov:agent", agents)
	checkRefs(t, doc, "wasAttributedTo", "prov:entity", entities)
	checkRefs(t, doc, "wasDerivedFrom", "prov:generatedEntity", entities)
	checkRefs(t, doc, "wasDerivedFrom", "prov:usedEntity", entities)
}

func TestProvOGraphCarriesTheSameStory(t *testing.T) {
	nodes := ProvOGraph(fullBundle())
	if len(nodes) < 5 {
		t.Fatalf("nodes = %d", len(nodes))
	}
	foundManuscript, foundVerification := false, false
	for _, n := range nodes {
		id, _ := n["@id"].(string)
		if id == "kubona:manuscript/ms1" {
			foundManuscript = true
			if n["kubona:contentHash"] != "h1" {
				t.Fatalf("manuscript node = %+v", n)
			}
		}
		if id == "kubona:verification/ms1" {
			foundVerification = true
		}
	}
	if !foundManuscript || !foundVerification {
		t.Fatalf("nodes = %+v", nodes)
	}
	again := ProvOGraph(fullBundle())
	for i := range nodes {
		if nodes[i]["@id"] != again[i]["@id"] {
			t.Fatal("PROV-O graph ordering is not deterministic")
		}
	}
}

func checkRefs(t *testing.T, doc map[string]any, relation, field string, known map[string]any) {
	t.Helper()
	block, ok := doc[relation].(map[string]any)
	if !ok {
		t.Fatalf("relation %q missing", relation)
	}
	for id, raw := range block {
		m, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s/%s is not an object", relation, id)
		}
		ref, _ := m[field].(string)
		if ref == "" {
			t.Fatalf("%s/%s has no %s", relation, id, field)
		}
		if _, ok := known[ref]; !ok {
			t.Fatalf("%s/%s references unknown %s %q", relation, id, field, ref)
		}
	}
}
