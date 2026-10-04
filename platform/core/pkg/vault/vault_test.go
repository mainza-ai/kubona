package vault

import (
	"testing"
	"time"
)

func TestVaultLeaseLifecycle(t *testing.T) {
	client := NewVaultClient()

	// Issue lease
	lease, err := client.IssueLease("ibm_quantum", "tenant-alpha", "super-secret-api-token", 10*time.Second)
	if err != nil {
		t.Fatalf("failed to issue lease: %v", err)
	}

	if lease.Scope != "ibm_quantum" || lease.TenantID != "tenant-alpha" {
		t.Fatalf("unexpected lease metadata: %+v", lease)
	}

	// Validate before consumption
	valid, reason := client.ValidateLease(lease.LeaseID)
	if !valid {
		t.Fatalf("expected lease to be valid, got false: %s", reason)
	}

	// Consume lease
	secret, err := client.ConsumeLease(lease.LeaseID)
	if err != nil {
		t.Fatalf("failed to consume lease: %v", err)
	}
	if secret != "super-secret-api-token" {
		t.Fatalf("unexpected secret returned: %s", secret)
	}

	// Re-consuming same lease must fail (single-use invariant)
	_, err = client.ConsumeLease(lease.LeaseID)
	if err == nil {
		t.Fatalf("expected error re-consuming single-use lease, got nil")
	}

	// Issue another lease and revoke it
	lease2, _ := client.IssueLease("rigetti", "tenant-beta", "rigetti-secret", 5*time.Second)
	if err := client.RevokeLease(lease2.LeaseID); err != nil {
		t.Fatalf("failed to revoke lease: %v", err)
	}

	valid, _ = client.ValidateLease(lease2.LeaseID)
	if valid {
		t.Fatalf("expected revoked lease to be invalid")
	}
}
