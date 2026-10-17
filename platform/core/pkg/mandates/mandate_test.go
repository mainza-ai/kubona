package mandates_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
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

// TestCrossLanguageMandateVector proves the browser signer (console
// src/lib/mandates.ts, @noble/ed25519) and this Go verifier agree byte for
// byte: the public key and signature below were produced by noble from the
// seed over the canonical payload string, and must verify with crypto/ed25519.
func TestCrossLanguageMandateVector(t *testing.T) {
	seed, err := hex.DecodeString("0001020304050607080900010203040506070809000102030405060708090001")
	if err != nil {
		t.Fatalf("decode seed: %v", err)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	if hex.EncodeToString(pub) != "c99aff667c7578056bd7d4590df92bf2ae108a6a7f33dd52384a83bf325f0269" {
		t.Fatalf("key derivation mismatch: noble pubkey != Go pubkey: %x", pub)
	}

	m := &mandates.SpendMandate{
		TenantID:        "tenant-vector",
		JobHash:         "jobhash-vector",
		MaxBudgetUSD:    25.5,
		ExpiryTimestamp: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		PublicKeyHex:    "c99aff667c7578056bd7d4590df92bf2ae108a6a7f33dd52384a83bf325f0269",
		SignatureHex:    "ea6a9a67b22c82be13f918e22824d1fd7d8f4f6e70a51f14f19112782dbc9e6e2075ba41c3c141955f24278e1cb6b8e0f643fdd0ddc0bc818412e7ed5d97570e",
	}
	if string(m.CanonicalPayload()) != "tenant-vector:jobhash-vector:25.5000:2030-01-01T00:00:00Z" {
		t.Fatalf("canonical payload mismatch: %q", m.CanonicalPayload())
	}
	if err := m.Verify("tenant-vector", "jobhash-vector", 25.5); err != nil {
		t.Fatalf("noble-produced mandate must verify in Go: %v", err)
	}
	if err := m.Verify("tenant-vector", "jobhash-vector", 25.5001); err != mandates.ErrMandateExceeded {
		t.Fatalf("over-budget must refuse with ErrMandateExceeded, got %v", err)
	}
}

// TestCanonicalJobBindingVector pins the v1 job binding both sides must
// compute identically (console mandates.ts canonicalJobBinding asserts the
// same hex): hex(sha256("kubona-job-v1\ncircuit_run\nsess-vec\n" + QASM)).
func TestCanonicalJobBindingVector(t *testing.T) {
	const qasm = "OPENQASM 2.0;\nqreg q[1];\n"
	if got := mandates.CanonicalJobBinding("circuit_run", "sess-vec", qasm); got != "67cc6d01173411aecf553a038cc9873a15779416dafd58a30e3c15a98c63bbfb" {
		t.Fatalf("binding vector mismatch: %s", got)
	}
	// Sensitivity: session, program, or type change must rebind.
	base := mandates.CanonicalJobBinding("circuit_run", "sess-vec", qasm)
	for _, other := range []string{
		mandates.CanonicalJobBinding("circuit_run", "sess-other", qasm),
		mandates.CanonicalJobBinding("circuit_run", "sess-vec", qasm+"h q[0];\n"),
		mandates.CanonicalJobBinding("mitigate", "sess-vec", qasm),
	} {
		if other == base {
			t.Fatalf("binding must change with session/program/type")
		}
	}
}
