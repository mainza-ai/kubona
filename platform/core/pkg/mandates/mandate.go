package mandates

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// CanonicalPayload returns the deterministic bytes over which the signature is computed.
// W0.12 rounding note: %.4f means budgets within $0.00005 share canonical
// bytes (one signature covers both spellings). This is wire-stable by
// design (console + Python sign identical bytes, pinned by vector tests),
// and SAFE at enforcement: Verify and admission compare full-precision
// floats, so a rounded signature never authorizes a rounded-up spend.
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

// Verify verifies that the mandate is cryptographically authentic, unexpired, and covers the cost.
// W0.12 order: the SIGNATURE verifies first, then bindings. Checking
// tenant/job/expiry before crypto lets an attacker probe which field
// mismatches without a valid signature (mismatch oracle) — a forged
// envelope must fail closed on crypto before learning anything else.
func (m *SpendMandate) Verify(expectedTenantID, expectedJobHash string, requestedCostUSD float64) error {
	pubKeyBytes, err := hex.DecodeString(m.PublicKeyHex)
	if err != nil {
		return fmt.Errorf("invalid public key: %w", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key: %d bytes (want %d)", len(pubKeyBytes), ed25519.PublicKeySize)
	}

	sigBytes, err := hex.DecodeString(m.SignatureHex)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature format: %d bytes (want %d)", len(sigBytes), ed25519.SignatureSize)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), m.CanonicalPayload(), sigBytes) {
		return ErrInvalidSignature
	}

	// Signature first (no mismatch oracle); only then refuse an unbound
	// mandate. A valid signature over an empty binding still has no job.
	if m.TenantID == "" || m.JobHash == "" {
		return fmt.Errorf("mandate is unbound: tenant_id and job_hash are required")
	}
	if math.IsNaN(m.MaxBudgetUSD) || math.IsInf(m.MaxBudgetUSD, 0) || m.MaxBudgetUSD < 0 {
		return fmt.Errorf("mandate budget must be finite and non-negative")
	}

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

	return nil
}

// CanonicalJobBinding binds a mandate receipt to the exact program in the
// exact session (W0.4). Format v1: hex(sha256("kubona-job-v1\n" + jobType +
// "\n" + sessionID + "\n" + qasm)). The console computes the identical
// string (mandates.ts canonicalJobBinding, pinned by the cross-language
// vector test on both sides); admission recomputes it from the submitted
// job and refuses a receipt bound to anything else. A mandate for one
// program cannot ride another — previously ANY covering mandate admitted
// ANY circuit.
func CanonicalJobBinding(jobType, sessionID, qasm string) string {
	sum := sha256.Sum256([]byte("kubona-job-v1\n" + jobType + "\n" + sessionID + "\n" + qasm))
	return hex.EncodeToString(sum[:])
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
