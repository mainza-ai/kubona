package agents

import (
	"context"
	"strings"
	"testing"
)

func TestBuildProposalRefusesMissingAnchorInProduction(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_ALLOW_ANCHOR_FALLBACK", "")

	if _, err := BuildCampaignProposal(context.Background(), "sess-prod-anchor", "H2 ground state VQE", "", nil); err == nil {
		t.Fatal("production proposal on a nil bridge must refuse, not fall back")
	} else if !strings.Contains(err.Error(), "refusing a campaign without a computed anchor") {
		t.Fatalf("refusal must name the missing computed anchor, got %v", err)
	}

	// Explicit escape hatch: an illustrative scaffold may exist, but it must
	// carry no borrowed energies and no result-shaped fields.
	t.Setenv("KUBONA_ALLOW_ANCHOR_FALLBACK", "1")
	state, err := BuildCampaignProposal(context.Background(), "sess-prod-anchor", "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatalf("explicit scaffold opt-in must build: %v", err)
	}
	if state.ClassicalAnchor.AnchorComputed {
		t.Fatal("missing bridge must stay anchor_computed=false")
	}
	if state.ClassicalAnchor.AnchorSource != "uncomputed" {
		t.Fatalf("anchor source = %q", state.ClassicalAnchor.AnchorSource)
	}
	if state.ClassicalAnchor.FciExactEnergy != 0 || state.ClassicalAnchor.RhfMeanFieldEnergy != 0 {
		t.Fatalf("uncomputed anchor must not borrow table energies: %+v", state.ClassicalAnchor)
	}
}
