package qec

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestDiscoveryClaimLifecycleTransitions(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	rec := &DiscoveryClaimRecord{
		ClaimID:         "dc_001",
		SessionID:       "sess_001",
		CodeName:        "candidate_bb_96",
		Status:          ClaimCandidate,
		EvidenceTier:    "T1_VERIFIED",
		CredibilityTier: "L4",
		ArtifactSHA256:  "deadbeef0123456789abcdef",
		GatesDigest:     "cafebabe0123456789abcdef",
		Gates0To6Passed: false,
	}

	pubHex, sigHex, err := SignDiscoveryClaim(rec.ClaimID, rec.ArtifactSHA256, rec.GatesDigest, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Cannot promote to verified if gates 0-6 failed
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", "", ""); err == nil {
		t.Fatal("expected refusal when Gates0To6Passed is false")
	}

	// Cannot skip verified to enter claimed directly
	rec.Gates0To6Passed = true
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, pubHex, sigHex, ""); err == nil {
		t.Fatal("expected refusal when jumping candidate -> claimed")
	}

	// Valid transition: candidate -> verified
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", "", ""); err != nil {
		t.Fatalf("candidate -> verified failed: %v", err)
	}

	// Bare string / missing signature must be refused (P0-3)
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, "operator-alice", "not-a-sig", ""); err == nil {
		t.Fatal("expected refusal for non-Ed25519 operator signoff")
	}

	// Valid Ed25519 signature promotes verified -> claimed
	if err := TransitionDiscoveryClaim(rec, ClaimClaimed, pubHex, sigHex, ""); err != nil {
		t.Fatalf("verified -> claimed failed: %v", err)
	}
	if rec.Status != ClaimClaimed || rec.OperatorPubkeyHex != pubHex {
		t.Fatalf("unexpected claimed state: %+v", rec)
	}

	// Retraction on re-verification failure requires reason
	if err := TransitionDiscoveryClaim(rec, ClaimRetracted, "", "", ""); err == nil {
		t.Fatal("expected refusal when retractReason is empty")
	}
	if err := TransitionDiscoveryClaim(rec, ClaimRetracted, "", "", "matched new catalog entry"); err != nil {
		t.Fatalf("claimed -> retracted failed: %v", err)
	}

	// Retracted is terminal
	if err := TransitionDiscoveryClaim(rec, ClaimVerified, "", "", ""); err == nil {
		t.Fatal("expected refusal when transitioning out of terminal retracted state")
	}
}
