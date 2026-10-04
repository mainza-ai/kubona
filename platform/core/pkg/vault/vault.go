package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLeaseExpired  = errors.New("credential lease has expired")
	ErrLeaseNotFound = errors.New("credential lease not found or already consumed")
)

type SecretLease struct {
	LeaseID   string    `json:"lease_id"`
	Scope     string    `json:"scope"`
	TenantID  string    `json:"tenant_id"`
	Secret    string    `json:"secret"`
	ExpiresAt time.Time `json:"expires_at"`
	Consumed  bool      `json:"consumed"`
	Revoked   bool      `json:"revoked"`
}

// VaultClient manages single-use short-lived credential leases (OpenBao sidecar pattern)
type VaultClient struct {
	mu     sync.Mutex
	leases map[string]*SecretLease
}

func NewVaultClient() *VaultClient {
	return &VaultClient{
		leases: make(map[string]*SecretLease),
	}
}

// IssueLease creates a single-use credential lease with strict TTL
func (v *VaultClient) IssueLease(scope, tenantID, secretValue string, ttl time.Duration) (*SecretLease, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	b := make([]byte, 16)
	rand.Read(b)
	leaseID := fmt.Sprintf("lease_%s_%s", scope, hex.EncodeToString(b))

	lease := &SecretLease{
		LeaseID:   leaseID,
		Scope:     scope,
		TenantID:  tenantID,
		Secret:    secretValue,
		ExpiresAt: time.Now().UTC().Add(ttl),
		Consumed:  false,
		Revoked:   false,
	}

	v.leases[leaseID] = lease
	return lease, nil
}

// ConsumeLease retrieves the credential and marks the lease as consumed (single-use)
func (v *VaultClient) ConsumeLease(leaseID string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	lease, exists := v.leases[leaseID]
	if !exists || lease.Consumed || lease.Revoked {
		return "", ErrLeaseNotFound
	}

	if time.Now().UTC().After(lease.ExpiresAt) {
		delete(v.leases, leaseID)
		return "", ErrLeaseExpired
	}

	lease.Consumed = true
	secret := lease.Secret
	delete(v.leases, leaseID)
	return secret, nil
}

// RevokeLease immediately revokes an active lease
func (v *VaultClient) RevokeLease(leaseID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	lease, exists := v.leases[leaseID]
	if !exists {
		return ErrLeaseNotFound
	}
	lease.Revoked = true
	delete(v.leases, leaseID)
	return nil
}

// ValidateLease checks if a lease is currently active and unexpired
func (v *VaultClient) ValidateLease(leaseID string) (bool, string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	lease, exists := v.leases[leaseID]
	if !exists {
		return false, "lease not found"
	}
	if lease.Revoked {
		return false, "lease revoked"
	}
	if lease.Consumed {
		return false, "lease consumed"
	}
	if time.Now().UTC().After(lease.ExpiresAt) {
		return false, "lease expired"
	}
	return true, "valid"
}
