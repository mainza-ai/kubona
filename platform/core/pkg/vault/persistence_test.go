package vault

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// W2.7: a revoked lease must stay revoked across restarts (revocations are
// durable tombstones, not memory-only).
func TestRevocationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "revoked.leases")

	before, err := NewVaultClientWithPersistence(path)
	if err != nil {
		t.Fatalf("failed to construct persistent client: %v", err)
	}
	lease, err := before.IssueLease("ibm_quantum", "tenant-alpha", "s3cret", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to issue lease: %v", err)
	}
	if err := before.RevokeLease(lease.LeaseID); err != nil {
		t.Fatalf("failed to revoke lease: %v", err)
	}

	// "Restart": a fresh client over the same revocations file.
	after, err := NewVaultClientWithPersistence(path)
	if err != nil {
		t.Fatalf("failed to reconstruct client: %v", err)
	}
	if valid, reason := after.ValidateLease(lease.LeaseID); valid {
		t.Fatalf("revoked lease resurrected after restart (reason=%q)", reason)
	}
	if _, err := after.ConsumeLease(lease.LeaseID); err == nil {
		t.Fatalf("consumed a lease revoked before restart")
	}
	// Idempotent re-revoke converges instead of erroring.
	if err := after.RevokeLease(lease.LeaseID); err != nil {
		t.Fatalf("re-revoke after restart failed: %v", err)
	}
	if after.RevokedCount() != 1 {
		t.Fatalf("expected 1 durable tombstone, got %d", after.RevokedCount())
	}
}

func TestRevokeEmptyLeaseRefused(t *testing.T) {
	vc := NewVaultClient()
	if err := vc.RevokeLease(""); err == nil {
		t.Fatalf("VaultClient.RevokeLease(\"\") succeeded: refusing empty is fail-closed")
	}
	if err := vc.RevokeLease("   "); err == nil {
		t.Fatalf("VaultClient.RevokeLease(whitespace) succeeded: refusing empty is fail-closed")
	}

	bao, err := NewOpenBaoClient(OpenBaoConfig{Addr: "http://127.0.0.1:1", Token: "x"})
	if err != nil {
		t.Fatalf("failed to construct bao client: %v", err)
	}
	if err := bao.RevokeLease(context.Background(), ""); err == nil {
		t.Fatalf("OpenBaoClient.RevokeLease(\"\") succeeded: refusing empty is fail-closed")
	}
}
