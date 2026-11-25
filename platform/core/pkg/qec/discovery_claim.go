package qec

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// DiscoveryClaimStatus is the canonical four-state claim lifecycle enforced by
// Kubona's Go control plane (Phase 4 / P0-3).
type DiscoveryClaimStatus string

const (
	ClaimCandidate DiscoveryClaimStatus = "candidate"
	ClaimVerified  DiscoveryClaimStatus = "verified"
	ClaimClaimed   DiscoveryClaimStatus = "claimed"
	ClaimRetracted DiscoveryClaimStatus = "retracted"

	ClaimSignDomain = "kubona:discovery_claim:v1"
)

// DiscoveryClaimRecord mirrors the ledger-persisted state of a QEC or Circuit
// Codesign discovery claim (Schema 18 unified lifecycle).
type DiscoveryClaimRecord struct {
	ClaimID              string               `json:"claim_id"`
	SessionID            string               `json:"session_id,omitempty"`
	Kind                 string               `json:"kind,omitempty"` // "qec_code" (default) | "circuit"
	CodeName             string               `json:"code_name"`
	Status               DiscoveryClaimStatus `json:"status"`
	EvidenceTier         string               `json:"evidence_tier"`    // T1_VERIFIED | T3_SAMPLED
	CredibilityTier      string               `json:"credibility_tier"` // L4 | L2 | L1
	ArtifactSHA256       string               `json:"artifact_sha256"`
	GatesDigest          string               `json:"gates_digest"`
	OperatorPubkeyHex    string               `json:"operator_pubkey_hex,omitempty"`
	OperatorSignatureHex string               `json:"operator_signature_hex,omitempty"`
	RetractionReason     string               `json:"retraction_reason,omitempty"`
	GateReportJSON       string               `json:"gate_report_json,omitempty"`
	Gates0To6Passed      bool                 `json:"gates_0_to_6_passed"`
}

// CanonicalClaimSignMessage builds the exact domain-separated byte string
// signed by Ed25519 for Gate 7 (identical to Python claims.py).
func CanonicalClaimSignMessage(claimID, artifactSHA256, gatesDigest string) []byte {
	return []byte(fmt.Sprintf("%s:%s:%s:%s", ClaimSignDomain, claimID, artifactSHA256, gatesDigest))
}

// SignDiscoveryClaim signs a claim bundle with an Ed25519 private key and
// returns (pubkeyHex, signatureHex).
func SignDiscoveryClaim(claimID, artifactSHA256, gatesDigest string, priv ed25519.PrivateKey) (string, string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", "", fmt.Errorf("invalid Ed25519 private key length %d", len(priv))
	}
	pub := priv.Public().(ed25519.PublicKey)
	sig := ed25519.Sign(priv, CanonicalClaimSignMessage(claimID, artifactSHA256, gatesDigest))
	return hex.EncodeToString(pub), hex.EncodeToString(sig), nil
}

// VerifyDiscoveryClaimSignoff verifies an Ed25519 Gate-7 operator signature.
func VerifyDiscoveryClaimSignoff(claimID, artifactSHA256, gatesDigest, pubkeyHex, sigHex string) error {
	if strings.TrimSpace(claimID) == "" || strings.TrimSpace(artifactSHA256) == "" || strings.TrimSpace(gatesDigest) == "" {
		return errors.New("claim_id, artifact_sha256, and gates_digest are required for Gate-7 signature verification")
	}
	pubBytes, err := hex.DecodeString(strings.TrimSpace(pubkeyHex))
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 operator_pubkey_hex (32-byte hex required)")
	}
	sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 operator_signature_hex (64-byte hex required)")
	}
	msg := CanonicalClaimSignMessage(claimID, artifactSHA256, gatesDigest)
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), msg, sigBytes) {
		return errors.New("Ed25519 Gate-7 operator signature verification failed")
	}
	return nil
}

// TransitionDiscoveryClaim enforces the strict lifecycle:
//
//	candidate -> verified -> claimed -> retracted
//	(verified -> retracted is also allowed on re-verification failure;
//	 retracted is terminal; skipping verified to enter claimed is refused;
//	 claimed requires a valid Ed25519 signature over (claim_id, artifact_sha256, gates_digest)).
func TransitionDiscoveryClaim(
	rec *DiscoveryClaimRecord,
	next DiscoveryClaimStatus,
	operatorPubkeyHex string,
	operatorSignatureHex string,
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
		if strings.TrimSpace(rec.ArtifactSHA256) == "" || strings.TrimSpace(rec.GatesDigest) == "" {
			return errors.New("cannot promote claim to verified: frozen artifact_sha256 and gates_digest are required")
		}
		rec.Status = ClaimVerified
		return nil
	case ClaimClaimed:
		if rec.Status != ClaimVerified {
			return fmt.Errorf("cannot promote claim to claimed from %q (must pass verified first)", rec.Status)
		}
		if !rec.Gates0To6Passed || strings.TrimSpace(rec.ArtifactSHA256) == "" || strings.TrimSpace(rec.GatesDigest) == "" {
			return errors.New("cannot promote claim to claimed: verified gates, artifact_sha256, and gates_digest required")
		}
		if err := VerifyDiscoveryClaimSignoff(
			rec.ClaimID,
			rec.ArtifactSHA256,
			rec.GatesDigest,
			operatorPubkeyHex,
			operatorSignatureHex,
		); err != nil {
			return fmt.Errorf("Gate-7 Ed25519 sign-off required: %w", err)
		}
		rec.OperatorPubkeyHex = strings.TrimSpace(operatorPubkeyHex)
		rec.OperatorSignatureHex = strings.TrimSpace(operatorSignatureHex)
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
