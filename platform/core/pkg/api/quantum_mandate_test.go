package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
)

func TestVerifyMandateRefBindsSessionProgramTenantAndBudget(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	qasm := "OPENQASM 2.0; qreg q[1];"
	jobHash := mandates.CanonicalJobBinding("circuit_run", "sess-1", qasm)
	m, err := mandates.SignMandate(priv, "lab-a", jobHash, 1.50, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)

	if err := verifyMandateRef(string(raw), "lab-a", "sess-1", qasm, 1.0); err != nil {
		t.Fatalf("valid mandate refused: %v", err)
	}
	// Wrong program.
	if err := verifyMandateRef(string(raw), "lab-a", "sess-1", "OPENQASM 2.0; qreg q[2];", 1.0); err == nil {
		t.Fatal("mandate for a different program must refuse")
	}
	// Wrong tenant.
	if err := verifyMandateRef(string(raw), "lab-b", "sess-1", qasm, 1.0); err == nil {
		t.Fatal("mandate for a different tenant must refuse")
	}
	// Over budget.
	if err := verifyMandateRef(string(raw), "lab-a", "sess-1", qasm, 99.0); err == nil {
		t.Fatal("mandate below the estimate must refuse")
	}
	// Tampered signature.
	m.SignatureHex = hex.EncodeToString(make([]byte, ed25519.SignatureSize))
	tampered, _ := json.Marshal(m)
	if err := verifyMandateRef(string(tampered), "lab-a", "sess-1", qasm, 1.0); err == nil {
		t.Fatal("forged signature must refuse")
	}
	// Legacy bare ref is recorded, not enforced here.
	if err := verifyMandateRef("receipt-abc", "lab-a", "sess-1", qasm, 1.0); err != nil {
		t.Fatalf("bare receipt ref must not refuse: %v", err)
	}
	_ = pub
}
