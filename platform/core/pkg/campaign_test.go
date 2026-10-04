package core_test

import (
	"crypto/ed25519"
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/autoresearch"
	"github.com/mainza-ai/kubona/core/pkg/epistemics"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
)

func TestEndToEndScientificCampaign(t *testing.T) {
	dbPath := "test_campaign.db"
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()

	// 1. Session Setup
	sm := session.NewManager(l)
	sess, err := sm.CreateSession("VQE Ground State Discovery for Molecular Hydrogen", 100.0)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// 2. Multi-Agent Collective Engagement
	collective := agents.NewAgentCollective()
	auth, _ := collective.AuthorizeTool(agents.RoleEpistemicScribe, "evaluate_wavefunction")
	if !auth {
		t.Fatalf("Epistemic Scribe authorization failed")
	}

	// 3. Classical Anchor Baseline from PySCF STO-3G (Simulated exact value: -1.13727 Ha)
	eFCI := -1.13727

	// 4. Ring 3 Device Auditor Clifford Probe & Fidelity Accounting
	fidGate := fidelity.NewFidelityGate()
	assessment := fidGate.EvaluateAdmission("ibm_heron", 8, 4, 0.98)
	if assessment.Decision != fidelity.DecisionAdmit {
		t.Fatalf("expected ADMIT decision, got %s: %s", assessment.Decision, assessment.Reason)
	}

	// 5. AP2 Spend Mandate Signing by Human Reviewer
	_, privKey, _ := ed25519.GenerateKey(nil)
	mandate, err := mandates.SignMandate(privKey, sess.ID, "job-vqe-001", 15.0, time.Now().UTC().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to sign mandate: %v", err)
	}
	if err := mandate.Verify(sess.ID, "job-vqe-001", 15.0); err != nil {
		t.Fatalf("mandate verification failed: %v", err)
	}

	// 6. Ring 0 Supervisor Watchdog Checkpoint
	sup := supervisor.NewSupervisor(l)
	defer sup.Close()
	loop, err := sup.RegisterLoop(sess.ID, 20)
	if err != nil {
		t.Fatalf("failed to register loop: %v", err)
	}
	_, err = sup.Step(loop.ID, 0.04) // Normal step
	if err != nil {
		t.Fatalf("supervisor step failed: %v", err)
	}

	// 7. Ring 1 Epistemic Wavefunction Collapse Gating
	// Quantum measurement yields -1.1368 Ha (|delta| = 0.47 mHa <= 1.6 mHa)
	eVQE := -1.1368
	deltaMHa := (eVQE - eFCI) * 1000.0

	epGate := epistemics.NewEpistemicGate()
	claim := &epistemics.ClaimWavefunction{
		ClaimID:                  "claim-e2e-h2",
		Statement:                "H2 ground state computed via VQE within chemical accuracy",
		AlphaCorroborated:        0.96,
		BetaContested:            0.04,
		GammaUnverified:          0.05,
		Sources:                  []string{"arXiv:1304.3061", "JACS:2014"},
		ChemicalAccuracyDeltaMHa: deltaMHa,
	}
	epGate.RegisterClaim(claim)

	canAssert, msg := epGate.CanAssert("claim-e2e-h2")
	if !canAssert {
		t.Fatalf("claim failed to collapse: %s", msg)
	}

	// 8. Autoresearch Harness Registry Verification
	reg := autoresearch.NewHarnessRegistry()
	h, err := reg.Get("ansatz-search")
	if err != nil || h == nil {
		t.Fatalf("failed to load ansatz-search harness")
	}

	// 9. Immutable Ledger Provenance Verification
	events, err := l.GetRecentEvents(sess.ID, 10)
	if err != nil || len(events) == 0 {
		t.Fatalf("expected ledger events recorded for session, got %d", len(events))
	}
}
