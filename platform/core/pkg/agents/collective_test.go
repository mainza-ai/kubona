package agents

import (
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/memory"
)

func TestAgentCollectiveManifests(t *testing.T) {
	ac := NewAgentCollective()

	// Verify all 5 personas exist
	roles := ac.ListRoles()
	if len(roles) != 5 {
		t.Fatalf("expected 5 personas, got %d", len(roles))
	}

	// Epistemic scribe can evaluate wavefunctions but cannot sign mandates
	auth, msg := ac.AuthorizeTool(RoleEpistemicScribe, "evaluate_wavefunction")
	if !auth {
		t.Fatalf("expected epistemic-scribe to be authorized for evaluate_wavefunction, got: %s", msg)
	}

	auth, _ = ac.AuthorizeTool(RoleEpistemicScribe, "sign_spend_mandate")
	if auth {
		t.Fatalf("epistemic-scribe must NOT be authorized for sign_spend_mandate")
	}

	// Human reviewer holds sign_spend_mandate
	auth, _ = ac.AuthorizeTool(RoleHumanReviewer, "sign_spend_mandate")
	if !auth {
		t.Fatalf("expected human-reviewer to be authorized for sign_spend_mandate")
	}

	// Disconfirmation officer can compute classical anchors
	auth, _ = ac.AuthorizeTool(RoleDisconfirmationOfficer, "compute_classical_anchor")
	if !auth {
		t.Fatalf("expected disconfirmation-officer to be authorized for compute_classical_anchor")
	}
}

func TestExecuteTribunal(t *testing.T) {
	ac := NewAgentCollective()
	res, err := ac.ExecuteTribunal(t.Context(), nil, nil, "Investigate H2 ground state")
	if err != nil {
		t.Fatalf("ExecuteTribunal failed: %v", err)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(res.Turns))
	}
	if !res.ConsensusPassed {
		t.Fatalf("expected consensus to pass")
	}
}

func TestExecuteTribunalWithMemory(t *testing.T) {
	ac := NewAgentCollective()
	store := memory.NewHindsightStore()

	// Seed relevant past experience into memory
	store.Retain(memory.BankExperiences, "Past trial for LiH observed shallow 2-qubit ansatz converges within 1.2 mHa without barren plateaus", 0.95)

	res, err := ac.ExecuteTribunalWithMemory(t.Context(), nil, nil, store, "Investigate LiH molecule ground state")
	if err != nil {
		t.Fatalf("ExecuteTribunalWithMemory failed: %v", err)
	}
	if len(res.Turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(res.Turns))
	}
	if !res.ConsensusPassed {
		t.Fatalf("expected consensus to pass")
	}

	// Verify that the tribunal consensus was retained into Hindsight memory
	recalled := store.SemanticRecall(memory.BankExperiences, "LiH consensus", 200, 0.3)
	if len(recalled) == 0 {
		t.Fatalf("expected consensus to be retained in Hindsight memory")
	}
	foundConsensus := false
	for _, r := range recalled {
		if strings.Contains(r.Item.Content, "Tribunal consensus for LiH") {
			foundConsensus = true
			break
		}
	}
	if !foundConsensus {
		t.Fatalf("expected to find consensus memory record in store")
	}
}
