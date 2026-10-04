package mandates

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrMandateExpired   = errors.New("spend mandate has expired")
	ErrMandateExceeded  = errors.New("requested spend exceeds authorized budget ceiling")
	ErrInvalidSignature = errors.New("cryptographic signature verification failed")
	ErrTenantMismatch   = errors.New("tenant ID mismatch in spend mandate")
	ErrJobHashMismatch  = errors.New("job payload hash mismatch in spend mandate")
)

// SpendMandate represents a cryptographic AP2 authorization envelope
type SpendMandate struct {
	TenantID        string    `json:"tenant_id"`
	JobHash         string    `json:"job_hash"`
	MaxBudgetUSD    float64   `json:"max_budget_usd"`
	ExpiryTimestamp time.Time `json:"expiry_timestamp"`
	PublicKeyHex    string    `json:"public_key_hex"`
	SignatureHex    string    `json:"signature_hex"`
}

// CanonicalPayload returns the deterministic bytes over which the signature is computed
func (m *SpendMandate) CanonicalPayload() []byte {
	return []byte(fmt.Sprintf("%s:%s:%.4f:%s",
		m.TenantID,
		m.JobHash,
		m.MaxBudgetUSD,
		m.ExpiryTimestamp.UTC().Format(time.RFC3339),
	))
}

// SignMandate creates an Ed25519 signature over the canonical mandate fields
func SignMandate(privKey ed25519.PrivateKey, tenantID, jobHash string, maxBudgetUSD float64, expiry time.Time) (*SpendMandate, error) {
	pubKey := privKey.Public().(ed25519.PublicKey)

	m := &SpendMandate{
		TenantID:        tenantID,
		JobHash:         jobHash,
		MaxBudgetUSD:    maxBudgetUSD,
		ExpiryTimestamp: expiry.UTC(),
		PublicKeyHex:    hex.EncodeToString(pubKey),
	}

	sig := ed25519.Sign(privKey, m.CanonicalPayload())
	m.SignatureHex = hex.EncodeToString(sig)
	return m, nil
}

// Verify verifies that the mandate is cryptographically authentic, unexpired, and covers the cost
func (m *SpendMandate) Verify(expectedTenantID, expectedJobHash string, requestedCostUSD float64) error {
	if expectedTenantID != "" && m.TenantID != expectedTenantID {
		return ErrTenantMismatch
	}

	if expectedJobHash != "" && m.JobHash != expectedJobHash {
		return ErrJobHashMismatch
	}

	if time.Now().UTC().After(m.ExpiryTimestamp) {
		return ErrMandateExpired
	}

	if requestedCostUSD > m.MaxBudgetUSD {
		return ErrMandateExceeded
	}

	pubKeyBytes, err := hex.DecodeString(m.PublicKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key: %w", err)
	}

	sigBytes, err := hex.DecodeString(m.SignatureHex)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), m.CanonicalPayload(), sigBytes) {
		return ErrInvalidSignature
	}

	return nil
}

// ComputeJobHash calculates the SHA-256 hash of a job specification
func ComputeJobHash(jobType string, params any) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job params: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte(jobType))
	hasher.Write([]byte(":"))
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
