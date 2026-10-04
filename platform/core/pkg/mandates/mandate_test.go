package mandates_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
)

func TestSpendMandateVerification(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	_ = pub

	tenantID := "tenant-acme"
	jobParams := map[string]any{"molecule": "LiH", "shots": 1024}
	jobHash, err := mandates.ComputeJobHash("circuit_run", jobParams)
	if err != nil {
		t.Fatalf("failed to compute job hash: %v", err)
	}

	expiry := time.Now().UTC().Add(1 * time.Hour)
	maxBudget := 25.0

	mandate, err := mandates.SignMandate(priv, tenantID, jobHash, maxBudget, expiry)
	if err != nil {
		t.Fatalf("failed to sign mandate: %v", err)
	}

	// 1. Valid verification
	if err := mandate.Verify(tenantID, jobHash, 10.0); err != nil {
		t.Errorf("expected valid verification, got error: %v", err)
	}

	// 2. Budget exceeded check
	if err := mandate.Verify(tenantID, jobHash, 30.0); err != mandates.ErrMandateExceeded {
		t.Errorf("expected ErrMandateExceeded, got: %v", err)
	}

	// 3. Hash mismatch check
	if err := mandate.Verify(tenantID, "tampered-hash", 10.0); err != mandates.ErrJobHashMismatch {
		t.Errorf("expected ErrJobHashMismatch, got: %v", err)
	}

	// 4. Expired mandate check
	pastExpiry := time.Now().UTC().Add(-10 * time.Minute)
	expiredMandate, _ := mandates.SignMandate(priv, tenantID, jobHash, maxBudget, pastExpiry)
	if err := expiredMandate.Verify(tenantID, jobHash, 10.0); err != mandates.ErrMandateExpired {
		t.Errorf("expected ErrMandateExpired, got: %v", err)
	}
}
