package ledger

// B4 store tests: evidence rows are typed, linked, and the summary obeys the
// measured-evidence rule. The human acceptance gate cannot outrank evidence.

import (
	"strings"
	"testing"
)

func TestClaimEvidenceSummaryAndReviewGate(t *testing.T) {
	l := mustOpenObligationLedger(t)
	claim, err := l.MintClaimRecord("sess-b4", "claim_eval", "VQE H2 measured", 0.9, 0.05, 0.05, "verified", "{}")
	if err != nil {
		t.Fatal(err)
	}

	// Verified without evidence: readable, but incomplete.
	got, evidence, summary, err := l.ClaimProvenance("sess-b4", claim.ID)
	if err != nil || got == nil {
		t.Fatalf("provenance: %v %v", got, err)
	}
	if len(evidence) != 0 || summary.Complete || summary.Measured != 0 {
		t.Fatalf("verified claim with no evidence must be incomplete: %+v %+v", evidence, summary)
	}

	// Accepting it refuses until evidence exists.
	if _, err := l.MintClaimReview("sess-b4", claim.ID, "accepted", "looks right", "reviewer", "{}"); err == nil {
		t.Fatal("accepting a verified claim without evidence must refuse")
	}

	if err := l.AddClaimEvidence("sess-b4", claim.ID, ClaimEvidence{
		Kind: "job_result", JobID: "job-1", ResultSHA256: ResultDigest([]byte(`{"energy":-1.1}`)),
	}); err != nil {
		t.Fatalf("add evidence: %v", err)
	}
	// Idempotent: the same row twice is one row.
	if err := l.AddClaimEvidence("sess-b4", claim.ID, ClaimEvidence{
		Kind: "job_result", JobID: "job-1", ResultSHA256: ResultDigest([]byte(`{"energy":-1.1}`)),
	}); err != nil {
		t.Fatalf("repeat evidence: %v", err)
	}
	_, evidence, summary, err = l.ClaimProvenance("sess-b4", claim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || summary.Measured != 1 || !summary.Complete {
		t.Fatalf("measured evidence must complete the claim: %+v %+v", evidence, summary)
	}
	if _, err := l.MintClaimReview("sess-b4", claim.ID, "accepted", "measured", "reviewer", "{}"); err != nil {
		t.Fatalf("acceptance after evidence must pass: %v", err)
	}
}

func TestClaimEvidenceGeneralKnowledgeAndValidation(t *testing.T) {
	l := mustOpenObligationLedger(t)
	claim, err := l.MintClaimRecord("sess-b4b", "tribunal", "Consensus", -1, -1, -1, "verified", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.AddClaimEvidence("sess-b4b", claim.ID, ClaimEvidence{
		Kind: "general_knowledge", SourceRef: "textbook:Atkins",
	}); err != nil {
		t.Fatalf("general knowledge: %v", err)
	}
	_, _, summary, err := l.ClaimProvenance("sess-b4b", claim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.GeneralKnowledge || !summary.Complete || summary.Measured != 0 {
		t.Fatalf("explicit general knowledge should satisfy completeness: %+v", summary)
	}

	cases := []ClaimEvidence{
		{Kind: "wizardry", SourceRef: "x"},
		{Kind: "job_result"},                   // no job/digest
		{Kind: "job_result", JobID: "j"},       // no digest
		{Kind: "literature"},                   // no source
		{Kind: "tribunal_turn", SourceRef: ""}, // no source
	}
	for i, ev := range cases {
		if err := l.AddClaimEvidence("sess-b4b", claim.ID, ev); err == nil {
			t.Fatalf("case %d must refuse: %+v", i, ev)
		}
	}
	// Foreign session: the claim is not in that session.
	if err := l.AddClaimEvidence("sess-other", claim.ID, ClaimEvidence{Kind: "general_knowledge", SourceRef: "x"}); err == nil {
		t.Fatal("cross-session evidence must refuse")
	}
	if got, err := l.GetClaimRecord("sess-other", claim.ID); err != nil || got != nil {
		t.Fatalf("cross-session claim read must be nil, got %+v %v", got, err)
	}
}

func TestClaimProvenanceContestedIsNotGraded(t *testing.T) {
	l := mustOpenObligationLedger(t)
	claim, err := l.MintClaimRecord("sess-b4c", "qaoa_hardware", "contested measurement", -1, -1, -1, "contested", "{}")
	if err != nil {
		t.Fatal(err)
	}
	_, _, summary, err := l.ClaimProvenance("sess-b4c", claim.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Complete || summary.Measured != 0 {
		t.Fatalf("contested is not graded by the verified rule: %+v", summary)
	}
	if !strings.Contains(ResultDigest([]byte("x")), "sha256:") {
		t.Fatal("result digest must be namespaced")
	}
}
