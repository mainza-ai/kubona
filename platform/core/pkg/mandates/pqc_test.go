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
