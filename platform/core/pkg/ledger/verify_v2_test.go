package ledger

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
)

func openVerifyLedger(t *testing.T) (*Ledger, func()) {
	t.Helper()
	path := t.TempDir() + "/ledger.db"
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	return l, func() { l.Close() }
}

func TestVerifyChainV2CommitsMetadata(t *testing.T) {
	l, cleanup := openVerifyLedger(t)
	defer cleanup()
	if err := l.AppendEvent("v2-sess", "annotation", map[string]any{"text": "hello"}); err != nil {
		t.Fatal(err)
	}
	if faults, err := l.VerifyChain("v2-sess"); err != nil || len(faults) != 0 {
		t.Fatalf("clean v2 chain must verify: %v %v", err, faults)
	}
	// Relabelling the event outside the digest was the v1 hole; v2 must fault.
	if _, err := l.DB().Exec(`UPDATE ledger_events SET tenant_id = 'evil' WHERE session_id = 'v2-sess'`); err != nil {
		t.Fatal(err)
	}
	faults, err := l.VerifyChain("v2-sess")
	if err != nil {
		t.Fatal(err)
	}
	if len(faults) == 0 {
		t.Fatal("metadata tamper must break the v2 chain")
	}
}

func TestV1ChainStillVerifies(t *testing.T) {
	l, cleanup := openVerifyLedger(t)
	defer cleanup()
	payload := `{"text":"legacy"}`
	prev := "0000000000000000000000000000000000000000000000000000000000000000"
	hash := ChainDigest(prev, "v1-sess", "annotation", payload)
	if _, err := l.DB().Exec(
		`INSERT INTO ledger_events (timestamp, session_id, tenant_id, conf_label, event_type, payload, prev_hash, entry_hash, hash_version, signature_domain) VALUES (?, ?, 'default', '', 'annotation', ?, ?, ?, 1, 'chain')`,
		time.Now().UTC().Format(time.RFC3339), "v1-sess", payload, prev, hash); err != nil {
		t.Fatal(err)
	}
	if faults, err := l.VerifyChain("v1-sess"); err != nil || len(faults) != 0 {
		t.Fatalf("legacy v1 chain must keep verifying: %v %v", err, faults)
	}
}

func TestExternalMandateSignatureDomainVerifies(t *testing.T) {
	l, cleanup := openVerifyLedger(t)
	defer cleanup()
	pub, priv, _ := ed25519.GenerateKey(nil)
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	m, err := mandates.SignMandate(priv, "lab-a", "job-1", 1.25, expiry)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"tenant_id":        m.TenantID,
		"job_hash":         m.JobHash,
		"public_key_hex":   m.PublicKeyHex,
		"signature_hex":    m.SignatureHex,
		"max_budget_usd":   m.MaxBudgetUSD,
		"expiry_timestamp": m.ExpiryTimestamp.UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(payload)
	if _, err := l.AppendExternalEvent("mandate-sess", "mandate_verified_client", payload,
		hex.EncodeToString(pub), m.SignatureHex); err != nil {
		t.Fatal(err)
	}
	_ = raw
	if faults, err := l.VerifyChain("mandate-sess"); err != nil || len(faults) != 0 {
		t.Fatalf("external mandate signature must verify: %v %v", err, faults)
	}
}

func TestOpaqueExternalProofIsNotClaimedVerified(t *testing.T) {
	l, cleanup := openVerifyLedger(t)
	defer cleanup()
	if _, err := l.AppendExternalEvent("pay-sess", "x402_payment_settled",
		map[string]any{"challenge_id": "c1"}, "00", "00"); err != nil {
		t.Fatal(err)
	}
	if faults, err := l.VerifyChain("pay-sess"); err != nil || len(faults) != 0 {
		t.Fatalf("opaque proof must be recorded without a false signature fault: %v %v", err, faults)
	}
}
