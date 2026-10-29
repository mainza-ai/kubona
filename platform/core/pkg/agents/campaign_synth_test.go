package agents

import (
	"context"
	"strings"
	"testing"
)

func TestSynthesizeCampaignRefusesAnchorFallbackInProduction(t *testing.T) {
	t.Setenv("KUBONA_ENV", "production")
	t.Setenv("KUBONA_ALLOW_ANCHOR_FALLBACK", "")

	if _, err := SynthesizeCampaignState(context.Background(), "sess-prod-anchor", "H2 ground state VQE", nil, nil); err == nil {
		t.Fatal("production synthesis on a nil bridge must refuse, not fall back")
	} else if !strings.Contains(err.Error(), "refusing hardcoded fallback") {
		t.Fatalf("refusal must name the fallback, got %v", err)
	}

	// Explicit escape hatch: a throwaway illustrative run can opt in, and
	// the result must still carry the fallback label.
	t.Setenv("KUBONA_ALLOW_ANCHOR_FALLBACK", "1")
	state, err := SynthesizeCampaignState(context.Background(), "sess-prod-anchor", "H2 ground state VQE", nil, nil)
	if err != nil {
		t.Fatalf("explicit fallback opt-in must synthesize: %v", err)
	}
	if state.ClassicalAnchor.AnchorComputed {
		t.Fatal("fallback anchor must stay anchor_computed=false")
	}
	if state.ClassicalAnchor.AnchorSource != "hardcoded-fallback" {
		t.Fatalf("fallback source = %q", state.ClassicalAnchor.AnchorSource)
	}
}
