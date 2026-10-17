package vault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//
// Revocation durability (W2.7): the live lease map is memory-only, so a
// restart used to wipe revocations and resurrect revoked leases. Revocations
// are therefore ALSO recorded as tombstones in an append-only revocations
// file (one lease id per line) when the client is constructed with
// NewVaultClientWithPersistence. The tombstones load at boot; ConsumeLease
// and ValidateLease consult them before the live map, so a revoked lease
// stays revoked across restarts. NewVaultClient (no path) is dev/test-only:
// revocations in it do not survive restarts — documented, not silent.
type VaultClient struct {
	mu          sync.Mutex
	leases      map[string]*SecretLease
	revoked     map[string]bool
	persistPath string
}

func NewVaultClient() *VaultClient {
	return &VaultClient{
		leases:  make(map[string]*SecretLease),
		revoked: make(map[string]bool),
	}
}

// NewVaultClientWithPersistence returns a client whose revocations survive
// restarts via an append-only file at path (loaded at boot). A missing file
// means first boot (not an error); IO failures ARE errors (fail-closed: an
// undurable revocation must not report success). W2.20: path must be
// absolute — a CWD-relative revocations file would fork durability across
// working directories (one daemon's tombstones invisible to another's).
func NewVaultClientWithPersistence(path string) (*VaultClient, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("vault persistence requires a non-empty revocations path")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("vault persistence path must be absolute (got %q): refusing CWD-relative durability", path)
	}
	v := &VaultClient{
		leases:      make(map[string]*SecretLease),
		revoked:     make(map[string]bool),
		persistPath: path,
	}
	if err := v.loadRevocations(); err != nil {
		return nil, err
	}
	return v, nil
}

// loadRevocations reads tombstones written by earlier processes. Caller must
// hold no lock (construction-time only).
func (v *VaultClient) loadRevocations() error {
	data, err := os.ReadFile(v.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("vault revocations unreadable at %s: %w", v.persistPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			v.revoked[id] = true
		}
	}
	return nil
}

// appendRevocationLocked durably records one revocation. Caller holds v.mu.
func (v *VaultClient) appendRevocationLocked(leaseID string) error {
	if err := os.MkdirAll(filepath.Dir(v.persistPath), 0o700); err != nil {
		return fmt.Errorf("vault revocations dir unwritable: %w", err)
	}
	f, err := os.OpenFile(v.persistPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("vault revocations unappendable at %s: %w", v.persistPath, err)
	}
	defer f.Close()
	if _, err := f.WriteString(leaseID + "\n"); err != nil {
		return fmt.Errorf("vault revocation write failed: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("vault revocation fsync failed: %w", err)
	}
	return nil
}

// RevokedCount reports durable tombstones (introspection/testing).
func (v *VaultClient) RevokedCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.revoked)
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

	if v.revoked[leaseID] {
		return "", ErrLeaseNotFound
	}
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

// RevokeLease immediately revokes an active lease. It is idempotent: revoking
// an already-revoked (or already-consumed) id re-records the tombstone and
// succeeds, so retry-after-restart converges. An empty id REFUSES
// (fail-closed). When persistence is configured, a revocation that cannot be
// made durable returns an error instead of a hollow success.
func (v *VaultClient) RevokeLease(leaseID string) error {
	if strings.TrimSpace(leaseID) == "" {
		return errors.New("refusing to revoke an empty lease id")
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	if lease, exists := v.leases[leaseID]; exists {
		lease.Revoked = true
		delete(v.leases, leaseID)
	}
	v.revoked[leaseID] = true
	if v.persistPath != "" {
		if err := v.appendRevocationLocked(leaseID); err != nil {
			return err
		}
	}
	return nil
}

// ValidateLease checks if a lease is currently active and unexpired
func (v *VaultClient) ValidateLease(leaseID string) (bool, string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.revoked[leaseID] {
		return false, "lease revoked"
	}
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
