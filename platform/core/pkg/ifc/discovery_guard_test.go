package ifc

import (
	"errors"
	"testing"
)

func TestDiscoveryHeldOutIsolationIFC(t *testing.T) {
	publicFrontier := TaggedDiscoveryItem{Name: "frontier_top5", Label: PublicDiscoveryLabel()}
	heldOutHeron := TaggedDiscoveryItem{Name: "heron_r2_cal_heldout", Label: HeldOutDiscoveryLabel()}
	heldOutSeeds := TaggedDiscoveryItem{Name: "sinter_heldout_seeds", Label: HeldOutDiscoveryLabel()}

	// Public frontier items may flow into the proposer LLM context.
	if err := GuardProposerContext([]TaggedDiscoveryItem{publicFrontier}); err != nil {
		t.Fatalf("public frontier should flow to proposer: %v", err)
	}

	// Held-out noise or seeds MUST be blocked from flowing into the proposer LLM context.
	err := GuardProposerContext([]TaggedDiscoveryItem{publicFrontier, heldOutHeron})
	if !errors.Is(err, ErrHeldOutLeakage) {
		t.Fatalf("expected ErrHeldOutLeakage for held-out noise in proposer context, got: %v", err)
	}
	err = GuardProposerContext([]TaggedDiscoveryItem{heldOutSeeds})
	if !errors.Is(err, ErrHeldOutLeakage) {
		t.Fatalf("expected ErrHeldOutLeakage for held-out seeds in proposer context, got: %v", err)
	}

	// Deterministic verifier is permitted to read BOTH public and held-out items.
	if err := GuardVerifierContext([]TaggedDiscoveryItem{publicFrontier, heldOutHeron, heldOutSeeds}); err != nil {
		t.Fatalf("verifier should be permitted to read public + held-out items: %v", err)
	}
}
