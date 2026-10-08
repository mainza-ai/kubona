package epistemics

import (
	"testing"
	"time"
)

func TestComputationalHypothesisAndContinuumTiers(t *testing.T) {
	h := ComputationalHypothesis{
		HypothesisID:        "hyp-femoco-001",
		Lean4Statement:      "theorem femoco_ground_state (H : Matrix C) : E >= E_anchor",
		AnalyticalAnchorID:  "Family_278_Kohn_Sham_Failure",
		QIRCircuitBitcode:   "%1 = call i1 @__quantum__rt__measure()",
		SpendBudgetUSD:      120.0,
		ExpectedEntropyGain: 0.85,
		CreatedTimestamp:    time.Now(),
	}

	hash := h.ContentHash()
	if len(hash) != 64 {
		t.Fatalf("expected 64-character SHA256 hash, got %d (%s)", len(hash), hash)
	}

	// Verify continuum tiers
	if Tier1FormalDeductive.BaseConfidence() != 1.00 {
		t.Errorf("Tier 1 must have 1.00 confidence")
	}
	if Tier5GenerativeHypothesis.BaseConfidence() >= 0.70 {
		t.Errorf("Tier 5 must be exploratory (< 0.70)")
	}
}

func TestFisherStoppingRule(t *testing.T) {
	eval := NewFisherStoppingRuleEvaluator(1.5) // lambda_ap2 = 1.5

	// Case 1: high expected information gain, low cost -> don't stop
	if eval.ShouldStop(10.0, 1.0) {
		t.Errorf("expected ShouldStop=false when info gain (10) > lambda * cost (1.5)")
	}

	// Case 2: marginal cost exceeds expected information gain -> stop!
	if !eval.ShouldStop(0.5, 1.0) {
		t.Errorf("expected ShouldStop=true when info gain (0.5) <= lambda * cost (1.5)")
	}
}

func TestNegativeCertificateHash(t *testing.T) {
	cert := &NegativeCertificate{
		CertificateID:     "cert-bp-01",
		FailureType:       "BARREN_PLATEAU",
		HypothesisHash:    "deadbeefcafebabe",
		TargetSystem:      "FeMoco",
		PrunedSubspace:    "ansatz_hea_depth_12",
		DiscoveredByAgent: "adversarial_critic",
		Timestamp:         time.Now(),
	}

	h := cert.Hash()
	if len(h) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d", len(h))
	}
}

func TestEpistemicTribunalLifecycle(t *testing.T) {
	engine := NewEpistemicTribunalEngine()

	hyp := ComputationalHypothesis{
		HypothesisID:        "hyp-krylov-01",
		Lean4Statement:      "theorem krylov_mono_convergence : Monotone krylov_energies",
		AnalyticalAnchorID:  "Heisenberg_Ferromagnet_3D",
		QIRCircuitBitcode:   "declare void @qksd_evolution()",
		SpendBudgetUSD:      50.0,
		ExpectedEntropyGain: 0.92,
	}

	session := engine.StartTribunal(hyp)
	if session.CurrentPhase != PhaseProposal {
		t.Fatalf("expected PhaseProposal, got %s", session.CurrentPhase)
	}

	// Phase 2: Adversarial Challenge passes
	if err := engine.AdvanceToChallenge(session.SessionID, true); err != nil {
		t.Fatalf("challenge advance failed: %v", err)
	}

	// Phase 3: Formal verification passes with 0 sorryAx
	if err := engine.AdvanceToFormalVerification(session.SessionID, true, true); err != nil {
		t.Fatalf("formal verification advance failed: %v", err)
	}

	// Phase 4: Analytical anchor audit passes
	if err := engine.AdvanceToAnchorAudit(session.SessionID, true); err != nil {
		t.Fatalf("anchor audit advance failed: %v", err)
	}

	// Phase 5: Quorum voting
	engine.CastVote(session.SessionID, PersonaVote{
		AgentPersona: "lead_theorist",
		Vote:         VoteCorroborated,
		Confidence:   0.95,
		Rationale:    "QKSD monotonic convergence mathematically holds",
	})
	engine.CastVote(session.SessionID, PersonaVote{
		AgentPersona: "adversarial_falsification_critic",
		Vote:         VoteCorroborated,
		Confidence:   0.90,
		Rationale:    "Barren plateau gradient check passed, variance > 1e-3",
	})
	engine.CastVote(session.SessionID, PersonaVote{
		AgentPersona: "lean4_proof_engineer",
		Vote:         VoteCorroborated,
		Confidence:   1.00,
		Rationale:    "Lake comparator passed with zero axioms outside propext/Quot/Choice",
	})
	engine.CastVote(session.SessionID, PersonaVote{
		AgentPersona: "classical_anchor_solver",
		Vote:         VoteCorroborated,
		Confidence:   0.92,
		Rationale:    "Consistent with Family 271 exact analytical bounds",
	})

	concluded, err := engine.ConcludeTribunal(session.SessionID)
	if err != nil {
		t.Fatalf("failed to conclude tribunal: %v", err)
	}

	if concluded.QuorumPercentage < 75.0 {
		t.Fatalf("expected quorum >= 75%%, got %.2f%%", concluded.QuorumPercentage)
	}
	if concluded.Verdict != "COMMITTED_TO_LEDGER" {
		t.Fatalf("expected COMMITTED_TO_LEDGER, got %s", concluded.Verdict)
	}
}
