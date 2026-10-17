package epistemics

import (
	"regexp"
	"strings"
)

// Source-label vocabulary: every string cited as claim evidence must resolve
// to something checkable. Free-text citations ("JACS:2014", "chemrxiv:2026.01"
// — both shipped here once, neither resolvable) are how invented evidence
// enters the record, so construction sites validate against this grammar:
//
//	arxiv:<YYMM.NNNNN>[vN]          arXiv e-print (arxiv:1304.3061 = the VQE paper)
//	doi:10.<registrant>/<suffix>    resolved DOI
//	pyscf:<basis>:<method>          our own classical anchor (rhf|fci)
//	bridge:<job-type>:<job-id>      a measured ledger job result
//	clm-<sess8>-<seq04>             a claim record
//	literature:pending-review       explicitly unattested — never evidence,
//	                                only a marker that review is outstanding
//
// The gate enforces this at registration (RegisterClaim refuses
// unresolvable labels); the patterns below are the single source for what
// resolves. Math-only unit tests may use raw strings, but no production
// claim enters with a label that fails here.
var sourceLabelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^arxiv:\d{4}\.\d{4,5}(v\d+)?$`),
	regexp.MustCompile(`^doi:10\.\d+/\S+$`),
	regexp.MustCompile(`^pyscf:[a-z0-9-]+:(rhf|fci)$`),
	regexp.MustCompile(`^bridge:(circuit_run|circuit_cut|classical_anchor|claim_eval|krylov_solve|mitigate):[0-9a-fA-F-]{8,}$`),
	regexp.MustCompile(`^clm-[0-9a-fA-F]{8}-\d{4}$`),
	regexp.MustCompile(`^literature:pending-review$`),
}

// ValidSourceLabel reports whether a citation resolves under the vocabulary.
func ValidSourceLabel(s string) bool {
	for _, p := range sourceLabelPatterns {
		if p.MatchString(s) {
			// W1.10: the bridge pattern's [0-9a-fA-F-]{8,} accepts
			// all-dash placeholders ("--------") as job IDs. The ID
			// part (after the last colon) must contain a hex digit.
			if strings.HasPrefix(s, "bridge:") {
				id := s[strings.LastIndex(s, ":")+1:]
				if !strings.ContainsAny(id, "0123456789abcdefABCDEF") {
					return false
				}
			}
			return true
		}
	}
	return false
}

// SeedClaims returns the gate's two bootstrap claims. Amplitudes are the
// long-standing demo values; sources are resolvable labels only — the H2
// seed pairs the real VQE paper with our computed FCI anchor, and the LiH
// seed carries an explicit pending-review marker (single source, contested,
// never collapsible as seeded).
func SeedClaims() []*ClaimWavefunction {
	return []*ClaimWavefunction{
		{
			ClaimID:                  "claim-h2-ground",
			Statement:                "H2 equilibrium ground state energy is -1.13727 Ha at R=0.7414 A",
			AlphaCorroborated:        0.95,
			BetaContested:            0.05,
			GammaUnverified:          0.10,
			EvidenceStrength:         0.92,
			Sources:                  []string{"arxiv:1304.3061", "pyscf:sto-3g:fci"},
			ChemicalAccuracyDeltaMHa: 0.8,
		},
		{
			ClaimID:                  "claim-lih-dissociation",
			Statement:                "LiH singlet-triplet gap at R=2.5 A exhibits strong multi-reference character",
			AlphaCorroborated:        0.65,
			BetaContested:            0.45,
			GammaUnverified:          0.30,
			EvidenceStrength:         0.55,
			Sources:                  []string{"literature:pending-review"},
			ChemicalAccuracyDeltaMHa: 2.4, // Contested / not collapsed
		},
	}
}
