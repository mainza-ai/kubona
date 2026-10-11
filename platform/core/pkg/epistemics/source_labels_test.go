package epistemics

import (
	"testing"
)

func TestValidSourceLabel(t *testing.T) {
	valid := []string{
		"arxiv:1304.3061",
		"arxiv:1304.3061v2",
		"doi:10.1038/ncomms5213",
		"pyscf:sto-3g:rhf",
		"pyscf:sto-3g:fci",
		"bridge:circuit_run:adb9deb9-e046-4b2c-b921-dd7ec2259ac0",
		"clm-4882b85b-0001",
		"literature:pending-review",
	}
	for _, s := range valid {
		if !ValidSourceLabel(s) {
			t.Errorf("resolvable label refused: %q", s)
		}
	}
	invalid := []string{
		"",
		"JACS:2014",
		"chemrxiv:2026.01",
		"arxiv",
		"jacs",
		"nature-paper",
		"pyscf:sto-3g",
		"pyscf:sto-3g:dft",
		"arxiv:not-an-id",
		"doi:10.1038",
		"clm-too-short",
		"bridge:teleport:abc12345",
		"http://arxiv.org/abs/1304.3061",
	}
	for _, s := range invalid {
		if ValidSourceLabel(s) {
			t.Errorf("unresolvable label admitted: %q", s)
		}
	}
}

// The shipped seeds must themselves comply: invented citations entered the
// record here once (JACS:2014, chemrxiv:2026.01), and only a test keeps them
// out. arXiv:1304.3061 is the real VQE paper (Peruzzo et al., verified
// against the arXiv API 2026-10-11).
func TestSeedClaimsResolve(t *testing.T) {
	seeds := SeedClaims()
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds, got %d", len(seeds))
	}
	for _, seed := range seeds {
		for _, src := range seed.Sources {
			if !ValidSourceLabel(src) {
				t.Errorf("seed %s cites unresolvable %q", seed.ClaimID, src)
			}
		}
	}
	gate := NewEpistemicGate()
	for _, seed := range SeedClaims() {
		gate.RegisterClaim(seed)
	}
	if ok, _ := gate.CanAssert("claim-h2-ground"); !ok {
		t.Error("H2 seed (paper + computed anchor) must stay assertable")
	}
	if ok, _ := gate.CanAssert("claim-lih-dissociation"); ok {
		t.Error("LiH seed (single pending-review source) must never assert")
	}
}
