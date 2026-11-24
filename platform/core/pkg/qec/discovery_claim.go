package qec

import (
	"errors"
	"fmt"
	"strings"
)

// DiscoveryClaimStatus is the canonical four-state claim lifecycle enforced by
// Kubona's Go control plane (Phase 4 / G3).
type DiscoveryClaimStatus string

const (
	ClaimCandidate DiscoveryClaimStatus = "candidate"
	ClaimVerified  DiscoveryClaimStatus = "verified"
	ClaimClaimed   DiscoveryClaimStatus = "claimed"
	ClaimRetracted DiscoveryClaimStatus = "retracted"
)

// DiscoveryClaimRecord mirrors the ledger-persisted state of a QEC discovery claim.
type DiscoveryClaimRecord struct {
	ClaimID           string               `json:"claim_id"`
	CodeName          string               `json:"code_name"`
	Status            DiscoveryClaimStatus `json:"status"`
	EvidenceTier      string               `json:"evidence_tier"`     // T1_VERIFIED | T3_SAMPLED
	CredibilityTier   string               `json:"credibility_tier"`  // L4 | L2 | L1
	ArtifactSHA256    string               `json:"artifact_sha256"`
	OperatorSignoff   string               `json:"operator_signoff,omitempty"`
	RetractionReason  string               `json:"retraction_reason,omitempty"`
	Gates0To6Passed   bool                 `json:"gates_0_to_6_passed"`
}

// TransitionDiscoveryClaim enforces the strict lifecycle:
//   candidate -> verified -> claimed -> retracted
//   (verified -> retracted is also allowed on re-verification failure;
//    retracted is terminal; skipping verified to enter claimed is refused).
func TransitionDiscoveryClaim(
	rec *DiscoveryClaimRecord,
	next DiscoveryClaimStatus,
	operatorSignoff string,
	retractReason string,
) error {
	if rec == nil {
		return errors.New("discovery claim record is nil")
	}
	if rec.Status == ClaimRetracted {
		return errors.New("discovery claim is already retracted (terminal state)")
	}
	switch next {
	case ClaimVerified:
		if rec.Status != ClaimCandidate {
			return fmt.Errorf("cannot transition claim from %q to %q", rec.Status, next)
		}
		if !rec.Gates0To6Passed {
			return errors.New("cannot promote claim to verified: deterministic gates 0-6 did not all pass")
		}
		if strings.TrimSpace(rec.ArtifactSHA256) == "" {
			return errors.New("cannot promote claim to verified: frozen artifact_sha256 is required")
		}
		rec.Status = ClaimVerified
		return nil
	case ClaimClaimed:
		if rec.Status != ClaimVerified {
			return fmt.Errorf("cannot promote claim to claimed from %q (must pass verified first)", rec.Status)
		}
		if !rec.Gates0To6Passed || strings.TrimSpace(rec.ArtifactSHA256) == "" {
			return errors.New("cannot promote claim to claimed: verified gates and frozen artifact_sha256 required")
		}
		if strings.TrimSpace(operatorSignoff) == "" {
			return errors.New("cannot promote claim to claimed: explicit human operator sign-off is required (Gate 7)")
		}
		rec.OperatorSignoff = strings.TrimSpace(operatorSignoff)
		rec.Status = ClaimClaimed
		return nil
	case ClaimRetracted:
		if rec.Status != ClaimVerified && rec.Status != ClaimClaimed {
			return fmt.Errorf("cannot retract claim from %q (only verified or claimed records may be retracted)", rec.Status)
		}
		if strings.TrimSpace(retractReason) == "" {
			return errors.New("retraction requires a non-empty reason")
		}
		rec.RetractionReason = strings.TrimSpace(retractReason)
		rec.Status = ClaimRetracted
		return nil
	default:
		return fmt.Errorf("unsupported target discovery claim status %q", next)
	}
}
