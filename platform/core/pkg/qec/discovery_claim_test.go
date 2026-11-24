package qec

import "testing"

func TestDiscoveryClaimLifecycleTransitions(t *testing.T) {
	rec := &DiscoveryClaimRecord{
		ClaimID:         "dc_001",
		CodeName:        "candidate_bb_96",
		Status:          ClaimCandidate,
		EvidenceTier:    "T1_VERIFIED",
		CredibilityTier: "L4",
		ArtifactSHA256:  "deadbeef",
		Gates0To6Passed: false,
	}

	// Cannot promote to verified if gates 0-6 failed
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", ""); err == nil {
		t.Fatal("expected refusal when Gates0To6Passed is false")
	}

	// Cannot skip verified to enter claimed directly
	rec.Gates0To6Passed = true
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, "operator-alice", ""); err == nil {
		t.Fatal("expected refusal when jumping candidate -> claimed")
	}

	// Valid transition: candidate -> verified
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", ""); err != nil {
		t.Fatalf("candidate -> verified failed: %v", err)
	}

	// Cannot enter claimed without human operator sign-off (Gate 7)
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, "", ""); err == nil {
		t.Fatal("expected refusal when operator signoff is empty")
	}

	// Valid transition: verified -> claimed
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, "operator-alice", ""); err != nil {
		t.Fatalf("verified -> claimed failed: %v", err)
	}
	if rec.Status != ClaimClaimed || rec.OperatorSignoff != "operator-alice" {
		t.Fatalf("unexpected claimed state: %+v", rec)
	}

	// Retraction on re-verification failure requires reason
	if err := TransitionDiscoveryClaim(rec, ClaimRetracted, "", ""); err == nil {
		t.Fatal("expected refusal when retractReason is empty")
	}
	if err := TransitionDiscoveryClaim(rec, ClaimRetracted, "", "matched new catalog entry"); err != nil {
		t.Fatalf("claimed -> retracted failed: %v", err)
	}

	// Retracted is terminal
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", ""); err == nil {
		t.Fatal("expected refusal when transitioning out of terminal retracted state")
	}
}
