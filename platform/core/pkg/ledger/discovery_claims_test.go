package ledger_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/qec"
)

func TestDiscoveryClaimsSchema17PersistenceAndEd25519Signoff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claims.db")
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer l.Close()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	rec := qec.DiscoveryClaimRecord{
		ClaimID:         "dc_persist_1",
		SessionID:       "sess_qec_1",
		CodeName:        "novel_bb_96",
		Status:          qec.ClaimCandidate,
		EvidenceTier:    "T1_VERIFIED",
		CredibilityTier: "L4",
		ArtifactSHA256:  "abc123def456",
		GatesDigest:     "999888777666",
		Gates0To6Passed: true,
	}
	if err := l.SaveDiscoveryClaim(rec); err != nil {
		t.Fatalf("save claim: %v", err)
	}

	// candidate -> verified persisted
	vRec, err := l.TransitionAndPersistDiscoveryClaim(rec.ClaimID, qec.ClaimVerified, "", "", "")
	if err != nil {
		t.Fatalf("transition verified: %v", err)
	}
	if vRec.Status != qec.ClaimVerified {
		t.Fatalf("status = %q, want verified", vRec.Status)
	}

	// Forged/bare sign-off must be refused and leave persisted status unchanged
	if _, err := l.TransitionAndPersistDiscoveryClaim(rec.ClaimID, qec.ClaimClaimed, "badpub", "badsig", ""); err == nil {
		t.Fatal("expected refusal for forged sign-off")
	}

	// Valid Ed25519 sign-off promotes verified -> claimed and persists
	pubHex, sigHex, err := qec.SignDiscoveryClaim(rec.ClaimID, rec.ArtifactSHA256, rec.GatesDigest, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	cRec, err := l.TransitionAndPersistDiscoveryClaim(rec.ClaimID, qec.ClaimClaimed, pubHex, sigHex, "")
	if err != nil {
		t.Fatalf("transition claimed: %v", err)
	}
	if cRec.Status != qec.ClaimClaimed || cRec.OperatorPubkeyHex != pubHex {
		t.Fatalf("unexpected claimed record: %+v", cRec)
	}

	// List by session returns the durable record
	list, err := l.ListDiscoveryClaims("sess_qec_1")
	if err != nil || len(list) != 1 || list[0].Status != qec.ClaimClaimed {
		t.Fatalf("list claims = %+v, err=%v", list, err)
	}

	// Retract persists terminal state
	rRec, err := l.TransitionAndPersistDiscoveryClaim(rec.ClaimID, qec.ClaimRetracted, "", "", "superseded by catalog")
	if err != nil || rRec.Status != qec.ClaimRetracted {
		t.Fatalf("retract failed: %+v, err=%v", rRec, err)
	}
}
