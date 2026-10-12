package mandates

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestHybridSpendMandateEndToEnd(t *testing.T) {
	// 1. Server generates ML-KEM-768 keypair
	serverPQC, err := GenerateRecipientKeypair()
	if err != nil {
		t.Fatalf("failed to generate server PQC keypair: %v", err)
	}

	// 2. Client generates Ed25519 identity keypair
	_, clientPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client Ed25519 keypair: %v", err)
	}

	jobHash, err := ComputeJobHash("vqe_optimization", map[string]any{"molecule": "H2", "basis": "sto-3g"})
	if err != nil {
		t.Fatalf("failed to compute job hash: %v", err)
	}

	// 3. Client signs standard SpendMandate
	expiry := time.Now().UTC().Add(10 * time.Minute)
	mandate, err := SignMandate(clientPriv, "tenant-alpha", jobHash, 50.0, expiry)
	if err != nil {
		t.Fatalf("failed to sign mandate: %v", err)
	}

	// 4. Client encapsulates mandate in ML-KEM-768 envelope
	hybrid, err := EncapsulateMandate(serverPQC.EncapsulationKey, mandate)
	if err != nil {
		t.Fatalf("failed to encapsulate mandate: %v", err)
	}

	if hybrid.RecipientFingerprint != serverPQC.Fingerprint() {
		t.Fatalf("expected fingerprint match: got %s, want %s", hybrid.RecipientFingerprint, serverPQC.Fingerprint())
	}

	// 5. Server decapsulates, decrypts, and verifies
	verifiedMandate, err := serverPQC.DecapsulateAndVerify(hybrid, "tenant-alpha", jobHash, 25.0)
	if err != nil {
		t.Fatalf("failed to decapsulate and verify: %v", err)
	}

	if verifiedMandate.TenantID != "tenant-alpha" {
		t.Fatalf("unexpected tenant: %s", verifiedMandate.TenantID)
	}
	if verifiedMandate.MaxBudgetUSD != 50.0 {
		t.Fatalf("unexpected budget: %f", verifiedMandate.MaxBudgetUSD)
	}
}

func TestHybridSpendMandateTampering(t *testing.T) {
	serverPQC, _ := GenerateRecipientKeypair()
	_, clientPriv, _ := ed25519.GenerateKey(nil)
	jobHash, _ := ComputeJobHash("test_job", map[string]any{"shots": 1000})
	mandate, _ := SignMandate(clientPriv, "tenant-beta", jobHash, 10.0, time.Now().UTC().Add(5*time.Minute))

	hybrid, err := EncapsulateMandate(serverPQC.EncapsulationKey, mandate)
	if err != nil {
		t.Fatalf("failed to encapsulate: %v", err)
	}

	// Case 1: Tamper with ML-KEM ciphertext
	tamperedCiphertextBytes, _ := hex.DecodeString(hybrid.EncapsulatedKeyHex)
	tamperedCiphertextBytes[10] ^= 0xFF
	tamperedHybrid := *hybrid
	tamperedHybrid.EncapsulatedKeyHex = hex.EncodeToString(tamperedCiphertextBytes)

	_, err = serverPQC.DecapsulateAndVerify(&tamperedHybrid, "tenant-beta", jobHash, 5.0)
	if err == nil {
		t.Fatalf("expected error on tampered ML-KEM ciphertext")
	}

	// Case 2: Tamper with AES-GCM encrypted payload
	tamperedPayloadBytes, _ := hex.DecodeString(hybrid.EncryptedPayloadHex)
	tamperedPayloadBytes[len(tamperedPayloadBytes)-1] ^= 0xAA
	tamperedHybrid2 := *hybrid
	tamperedHybrid2.EncryptedPayloadHex = hex.EncodeToString(tamperedPayloadBytes)

	_, err = serverPQC.DecapsulateAndVerify(&tamperedHybrid2, "tenant-beta", jobHash, 5.0)
	if err == nil {
		t.Fatalf("expected error on tampered encrypted payload")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestHybridSpendMandateRecipientMismatch(t *testing.T) {
	serverA, _ := GenerateRecipientKeypair()
	serverB, _ := GenerateRecipientKeypair()
	_, clientPriv, _ := ed25519.GenerateKey(nil)

	mandate, _ := SignMandate(clientPriv, "tenant-gamma", "hash123", 10.0, time.Now().UTC().Add(5*time.Minute))
	hybrid, _ := EncapsulateMandate(serverA.EncapsulationKey, mandate)

	// Server B tries to decapsulate mandate intended for Server A
	_, err := serverB.DecapsulateAndVerify(hybrid, "tenant-gamma", "hash123", 5.0)
	if !errors.Is(err, ErrRecipientMismatch) {
		t.Fatalf("expected ErrRecipientMismatch, got: %v", err)
	}
}

// KEM parameter contract (E-1e): FIPS 203 ML-KEM-768 dimensions are pinned
// here so an algorithm swap (or a confused import) fails loudly instead of
// silently weakening the envelope. Note on NIST KAT vectors: they bind a
// fixed seed to fixed outputs, but Go's stdlib API is seedless by design
// (GenerateKey768/Encapsulate draw internally) — there is no seam to feed
// vectors through. The stdlib implementation itself is the validated
// primitive; these tests pin OUR envelope logic (sizes, parsing,
// tamper/wrong-key behavior) on top of it.
func TestMLKEM768ParameterSizes(t *testing.T) {
	kp, err := GenerateRecipientKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if n := len(kp.EncapsulationKey.Bytes()); n != 1184 {
		t.Errorf("ML-KEM-768 public key must be 1184 bytes, got %d", n)
	}
	if _, err := ParseEncapsulationKey(make([]byte, 1183)); err == nil {
		t.Error("short public key must refuse parsing")
	}
	if _, err := ParseEncapsulationKey(make([]byte, 1185)); err == nil {
		t.Error("long public key must refuse parsing")
	}
	if _, err := ParseEncapsulationKey(kp.EncapsulationKey.Bytes()); err != nil {
		t.Errorf("valid public key must parse: %v", err)
	}
}

func TestMLKEMCiphertextAndNonceShape(t *testing.T) {
	kp, err := GenerateRecipientKeypair()
	if err != nil {
		t.Fatal(err)
	}
	_, clientPriv, _ := ed25519.GenerateKey(nil)
	jobHash, _ := ComputeJobHash("job", map[string]any{"a": 1})
	mandate, _ := SignMandate(clientPriv, "t", jobHash, 5.0, time.Now().UTC().Add(time.Minute))

	first, err := EncapsulateMandate(kp.EncapsulationKey, mandate)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := hex.DecodeString(first.EncapsulatedKeyHex)
	if err != nil || len(ct) != 1088 {
		t.Errorf("ML-KEM-768 ciphertext must be 1088 bytes, got %d (%v)", len(ct), err)
	}
	nonce, err := hex.DecodeString(first.NonceHex)
	if err != nil || len(nonce) != 12 {
		t.Errorf("AES-GCM nonce must be 12 bytes, got %d (%v)", len(nonce), err)
	}
	// Fresh randomness per encapsulation: identical mandates seal differently.
	second, err := EncapsulateMandate(kp.EncapsulationKey, mandate)
	if err != nil {
		t.Fatal(err)
	}
	if first.EncapsulatedKeyHex == second.EncapsulatedKeyHex || first.NonceHex == second.NonceHex {
		t.Error("encapsulation randomness must differ per envelope")
	}
	// Fingerprint binds the recipient deterministically and uniquely.
	if kp.Fingerprint() == "" || len(kp.Fingerprint()) != 64 {
		t.Errorf("fingerprint must be 64-hex SHA-256, got %q", kp.Fingerprint())
	}
	other, _ := GenerateRecipientKeypair()
	if other.Fingerprint() == kp.Fingerprint() {
		t.Error("distinct keypairs must fingerprint distinctly")
	}
}

func TestMLKEMWrongKeyDecapsulationFails(t *testing.T) {
	a, _ := GenerateRecipientKeypair()
	b, _ := GenerateRecipientKeypair()
	_, clientPriv, _ := ed25519.GenerateKey(nil)
	jobHash, _ := ComputeJobHash("job", map[string]any{"a": 1})
	mandate, _ := SignMandate(clientPriv, "t", jobHash, 5.0, time.Now().UTC().Add(time.Minute))

	hybrid, err := EncapsulateMandate(a.EncapsulationKey, mandate)
	if err != nil {
		t.Fatal(err)
	}
	// Wrong recipient, right tenant/job/budget: the fingerprint gate fires
	// before any decapsulation attempt.
	if _, err := b.DecapsulateAndVerify(hybrid, "t", jobHash, 5.0); !errors.Is(err, ErrRecipientMismatch) {
		t.Fatalf("expected ErrRecipientMismatch, got %v", err)
	}
	// Bypass the gate (same keypair object, corrupted ciphertext): the KEM
	// itself must not yield a usable secret — GCM open fails closed.
	corrupt := *hybrid
	raw, _ := hex.DecodeString(hybrid.EncapsulatedKeyHex)
	raw[0] ^= 0x01
	corrupt.EncapsulatedKeyHex = hex.EncodeToString(raw)
	if _, err := a.DecapsulateAndVerify(&corrupt, "t", jobHash, 5.0); err == nil {
		t.Fatal("corrupted KEM ciphertext must never verify")
	}
}
