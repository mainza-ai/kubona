package ledger_test

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestLedgerLifecycle(t *testing.T) {
	dbPath := "test_kubona.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	sessionID := "sess-123"
	if err := l.AppendEvent(sessionID, "test_event", map[string]string{"foo": "bar"}); err != nil {
		t.Fatalf("failed to append event: %v", err)
	}

	events, err := l.GetRecentEvents(sessionID, 10)
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "test_event" {
		t.Errorf("expected test_event, got %s", events[0].EventType)
	}
}

func TestSignedLedgerEventsAndTamperDetection(t *testing.T) {
	dbPath := "test_signed_ledger.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	// Generate genuine Ed25519 keypair for test agent
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	sessionID := "sess-crypto-777"

	// 1. Append first signed event
	ev1, err := l.AppendSignedEvent(sessionID, "proposal_generated", map[string]any{
		"ansatz": "Ry-Rz-6q",
		"target": "H2_ground_state",
	}, privKey)
	if err != nil {
		t.Fatalf("failed to append signed event 1: %v", err)
	}

	if ev1.SignerPubkey == "" || ev1.Signature == "" {
		t.Fatalf("event 1 missing cryptographic signature or pubkey")
	}

	// Verify genuine signature
	valid, err := l.VerifyEventSignature(ev1)
	if err != nil || !valid {
		t.Fatalf("expected valid signature on genuine event 1: %v", err)
	}

	// 2. Append second signed event (chained to event 1)
	ev2, err := l.AppendSignedEvent(sessionID, "tribunal_verdict", map[string]any{
		"disconfirmation_passed": true,
		"admissible":             true,
	}, privKey)
	if err != nil {
		t.Fatalf("failed to append signed event 2: %v", err)
	}

	// Event 2's PrevHash must match Event 1's EntryHash
	if ev2.PrevHash != ev1.EntryHash {
		t.Fatalf("hash chain break: ev2.PrevHash (%s) != ev1.EntryHash (%s)", ev2.PrevHash, ev1.EntryHash)
	}

	valid2, err := l.VerifyEventSignature(ev2)
	if err != nil || !valid2 {
		t.Fatalf("expected valid signature on genuine event 2: %v", err)
	}

	// 3. Tampering test: modify payload of ev2
	tamperedEv2 := *ev2
	tamperedEv2.Payload = []byte(`{"disconfirmation_passed":false,"admissible":false}`)
	validTampered, _ := l.VerifyEventSignature(&tamperedEv2)
	if validTampered {
		t.Fatalf("security violation: tampered payload was accepted as valid")
	}

	// 4. Tampering test: signature forged with another key
	_, otherPrivKey, _ := ed25519.GenerateKey(nil)
	evForged, err := l.AppendSignedEvent(sessionID, "unauthorized_payout", map[string]any{"amount": 1000}, otherPrivKey)
	if err != nil {
		t.Fatalf("failed to append forged event: %v", err)
	}
	// Verify that evForged has other key's pubkey, NOT agent's pubkey
	otherPubHex := hex.EncodeToString(pubKey)
	if evForged.SignerPubkey == otherPubHex {
		t.Fatalf("expected distinct public key for unauthorized actor")
	}
}

func TestLedgerConfidentialityLabels(t *testing.T) {
	dbPath := "test_ledger_labels.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	// Default appends land in the session tenant's label.
	rec, err := l.AppendSignedEvent("sess-lab", "job_settled", map[string]string{"ok": "true"}, nil)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if rec.TenantID != "default" || rec.ConfLabel != "tenant/default" {
		t.Fatalf("default label wrong: %+v", rec)
	}

	// Explicit public label allowed.
	pub, err := l.AppendLabeledEvent("sess-lab", "announcement", map[string]string{"m": "1"}, nil, "public")
	if err != nil || pub.ConfLabel != "public" {
		t.Fatalf("public label must be writable: %+v err=%v", pub, err)
	}

	// Cross-tenant and empty labels refused.
	if _, err := l.AppendLabeledEvent("sess-lab", "x", map[string]string{}, nil, "tenant/lab-other"); err == nil {
		t.Fatalf("cross-tenant label must refuse")
	}
	if _, err := l.AppendLabeledEvent("sess-lab", "x", map[string]string{}, nil, ""); err == nil {
		t.Fatalf("empty label must refuse")
	}
	if _, err := l.AppendLabeledEvent("sess-lab", "x", map[string]string{}, nil, "deny-all"); err == nil {
		t.Fatalf("deny-all label must refuse")
	}

	// Reads carry labels; migration backfills tenant-scoped defaults.
	events, err := l.GetRecentEvents("sess-lab", 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("expected 2 events, got %d err=%v", len(events), err)
	}
	for _, e := range events {
		if e.ConfLabel == "" {
			t.Fatalf("no event may read back unlabeled: %+v", e)
		}
	}
}
