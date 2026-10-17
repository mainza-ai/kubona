package epistemics

import (
	"testing"
)

func TestEpistemicGateAssertion(t *testing.T) {
	gate := NewEpistemicGate()

	// 1. Weak unverified claim (alpha=0.5, beta=0.2, gamma=0.8) -> P_corr = 0.5^2 / norm^2 < 0.75
	weak := &ClaimWavefunction{
		ClaimID:                  "claim-weak",
		Statement:                "Hypothetical ground state shift",
		AlphaCorroborated:        0.5,
		BetaContested:            0.2,
		GammaUnverified:          0.8,
		Sources:                  []string{"arxiv:1304.3061"},
		ChemicalAccuracyDeltaMHa: 0.5,
	}
	gate.RegisterClaim(weak)

	canAssert, reason := gate.CanAssert("claim-weak")
	if canAssert {
		t.Fatalf("expected weak claim assertion to be rejected, got allowed")
	}
	if reason == "" {
		t.Fatalf("expected reason for rejection")
	}

	// 2. High confidence claim but lacking 2nd source
	singleSource := &ClaimWavefunction{
		ClaimID:                  "claim-single",
		Statement:                "High confidence single source",
		AlphaCorroborated:        0.98,
		BetaContested:            0.05,
		GammaUnverified:          0.05,
		Sources:                  []string{"doi:10.1038/ncomms5213"},
		ChemicalAccuracyDeltaMHa: 0.4,
	}
	gate.RegisterClaim(singleSource)
	canAssert, _ = gate.CanAssert("claim-single")
	if canAssert {
		t.Fatalf("expected single source claim to be rejected by source diversity check")
	}

	// 3. Fully collapsed claim: high alpha, 2 sources, delta < 1.6 mHa
	solid := &ClaimWavefunction{
		ClaimID:                  "claim-solid",
		Statement:                "H2 equilibrium ground state at 0.7414 A",
		AlphaCorroborated:        0.95,
		BetaContested:            0.05,
		GammaUnverified:          0.1,
		Sources:                  []string{"arxiv:1304.3061", "pyscf:sto-3g:fci"},
		ChemicalAccuracyDeltaMHa: 0.8,
	}
	gate.RegisterClaim(solid)
	canAssert, msg := gate.CanAssert("claim-solid")
	if !canAssert {
		t.Fatalf("expected solid claim to pass assertion gate, got rejected: %s", msg)
	}

	// Refusal ratio should be 2 / 3 = 0.666...
	if gate.RefusalRatio() < 0.5 {
		t.Fatalf("expected refusal ratio > 0.5, got %f", gate.RefusalRatio())
	}
}

func TestEpistemicCurvature(t *testing.T) {
	kappa, drift := ComputeEpistemicCurvature(0.25, -1.137, -1.136)
	if kappa <= 0.0 || kappa > 1.0 {
		t.Fatalf("invalid kappa: %f", kappa)
	}
	if drift < 0.0009 || drift > 0.0011 {
		t.Fatalf("unexpected drift: %f", drift)
	}
}

// W1.10: the gate refuses unphysical registrations and isolates stored
// claims from caller mutation.
func TestEpistemicGateDomainRefusals(t *testing.T) {
	gate := NewEpistemicGate()
	base := func() *ClaimWavefunction {
		return &ClaimWavefunction{
			ClaimID: "c1", Statement: "s",
			AlphaCorroborated: 0.9, BetaContested: 0.1, GammaUnverified: 0.1,
			Sources:                  []string{"arxiv:1304.3061", "doi:10.1038/ncomms5213"},
			ChemicalAccuracyDeltaMHa: 0.5,
		}
	}
	bad := base()
	bad.ClaimID = ""
	if err := gate.RegisterClaim(bad); err == nil {
		t.Fatalf("empty claim id must refuse")
	}
	bad = base()
	bad.AlphaCorroborated = -0.5
	if err := gate.RegisterClaim(bad); err == nil {
		t.Fatalf("negative amplitude must refuse")
	}
	bad = base()
	bad.ChemicalAccuracyDeltaMHa = -100.0
	if err := gate.RegisterClaim(bad); err == nil {
		t.Fatalf("negative delta must refuse (it used to collapse)")
	}
	// Stored claims are copies: post-registration mutation changes nothing.
	good := base()
	if err := gate.RegisterClaim(good); err != nil {
		t.Fatal(err)
	}
	good.AlphaCorroborated = 0.0
	good.ChemicalAccuracyDeltaMHa = 100.0
	if ok, _ := gate.CanAssert("c1"); !ok {
		t.Fatalf("stored claim must be isolated from caller mutation")
	}
	// Placeholder job IDs refuse.
	if ValidSourceLabel("bridge:circuit_run:--------") {
		t.Fatalf("all-dash job id must not resolve")
	}
	if !ValidSourceLabel("bridge:circuit_run:abc12345") {
		t.Fatalf("real job id must resolve")
	}
}
