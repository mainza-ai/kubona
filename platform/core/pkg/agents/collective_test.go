package agents

import (
	"testing"
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
