package ledger_test

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"os"
	"strings"
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

func TestMigrationAddsSpendColumns(t *testing.T) {
	// Simulate a pre-spend-tracking database: old tenants/jobs shapes.
	dbPath := "test_ledger_migrate.db"
	os.Remove(dbPath)
	defer func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE tenants (id TEXT PRIMARY KEY, display_name TEXT, admin_pubkey_hex TEXT NOT NULL DEFAULT '', autonomy_level TEXT NOT NULL DEFAULT 'L1', spend_ceiling_usd REAL NOT NULL DEFAULT 150.0, status TEXT NOT NULL DEFAULT 'active', created_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tenants (id, spend_ceiling_usd, status) VALUES ('old-lab', 150.0, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE jobs (id TEXT PRIMARY KEY, session_id TEXT, status TEXT, job_type TEXT, params_json TEXT, result_json TEXT, created_at DATETIME, completed_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("open must migrate, not fail: %v", err)
	}
	defer l.Close()
	var spent float64
	if err := l.DB().QueryRow(`SELECT spent_usd FROM tenants WHERE id = 'old-lab'`).Scan(&spent); err != nil {
		t.Fatalf("migrated spent_usd must read: %v", err)
	}
	if spent != 0 {
		t.Fatalf("pre-tracking spend backfills 0, got %v", spent)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount) VALUES ('j1','s','x','circuit_run','{}',CURRENT_TIMESTAMP,'old-lab',25.0)`); err != nil {
		t.Fatalf("migrated jobs columns must accept reservations: %v", err)
	}
}

func TestCampaignVersions(t *testing.T) {
	dbPath := "test_campaign_versions.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	sid := "sess-versions"
	for i, trigger := range []string{"session-create", "chat:first", "chat:second"} {
		if err := l.SaveCampaignStateWithTrigger(sid, []byte(`{"v":`+itoaTest(i)+`}`), trigger); err != nil {
			t.Fatal(err)
		}
	}
	// Live row holds the latest.
	raw, err := l.GetCampaignState(sid)
	if err != nil || string(raw) != `{"v":2}` {
		t.Errorf("live row must hold latest, got %s / %v", raw, err)
	}
	versions, err := l.GetCampaignVersions(sid, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	for i, v := range versions {
		if v.Version <= 0 {
			t.Errorf("version %d has no rowid version", i)
		}
	}
	if versions[0].Trigger != "session-create" || versions[2].Trigger != "chat:second" {
		t.Errorf("triggers out of order: %+v", versions)
	}
	// Cap enforcement: push 60 more, oldest must fall off at 50.
	for i := 0; i < 60; i++ {
		if err := l.SaveCampaignStateWithTrigger(sid, []byte(`{"v":"x"}`), "flood"); err != nil {
			t.Fatal(err)
		}
	}
	versions, _ = l.GetCampaignVersions(sid, 100)
	if len(versions) != 50 {
		t.Errorf("expected capped history of 50, got %d", len(versions))
	}
	if versions[0].Trigger != "flood" {
		t.Errorf("oldest rows must fall off first, oldest trigger is %q", versions[0].Trigger)
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestMintClaimRecordRoundTrip(t *testing.T) {
	dbPath := "test_claim_records.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	r1, err := l.MintClaimRecord("sess-claims", "claim_eval", "Evaluated claim: within 0.4 mHa",
		0.97, 0.02, 0.05, "verified", `{"job_id":"j1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.ID) < 13 || r1.ID[:4] != "clm-" {
		t.Errorf("record ID must be stable and prefixed, got %q", r1.ID)
	}
	r2, err := l.MintClaimRecord("sess-claims", "tribunal", "Tribunal consensus: go",
		-1, -1, -1, "verified", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if r1.ID == r2.ID {
		t.Error("per-turn IDs must differ")
	}

	list, err := l.ListClaimRecords("sess-claims", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != r2.ID || list[1].ID != r1.ID {
		t.Errorf("newest-first ordering broken: %+v", list)
	}
	if list[1].Alpha != 0.97 || list[0].Alpha != -1 {
		t.Error("amplitudes must round-trip, -1 sentinel preserved")
	}
	if empty, _ := l.ListClaimRecords("ghost", 10); len(empty) != 0 {
		t.Error("unknown session must list nothing")
	}
	if _, err := l.MintClaimRecord("", "tribunal", "x", -1, -1, -1, "verified", ""); err == nil {
		t.Error("empty session must refuse a mint")
	}
	if _, err := l.MintClaimRecord("s", "tribunal", "  ", -1, -1, -1, "verified", ""); err == nil {
		t.Error("empty statement must refuse a mint")
	}
}

func TestVerifyCampaignHistory(t *testing.T) {
	dbPath := "test_verify_campaign.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	// No campaign verifies cleanly.
	if err := l.VerifyCampaignHistory("ghost"); err != nil {
		t.Errorf("empty history must verify: %v", err)
	}

	sid := "sess-verify"
	if err := l.SaveCampaignStateWithTrigger(sid, []byte(`{"v":1}`), "session-create"); err != nil {
		t.Fatal(err)
	}
	if err := l.SaveCampaignStateWithTrigger(sid, []byte(`{"v":2}`), "chat:x"); err != nil {
		t.Fatal(err)
	}
	if err := l.VerifyCampaignHistory(sid); err != nil {
		t.Errorf("consistent history must verify: %v", err)
	}

	// Simulate a bypass writer: live row moved without a version.
	if _, err := l.DB().Exec(`UPDATE campaign_states SET state_json = '{"v":999}' WHERE session_id = ?`, sid); err != nil {
		t.Fatal(err)
	}
	if err := l.VerifyCampaignHistory(sid); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Errorf("bypass write must fail loudly, got %v", err)
	}
}

func TestSingleWriterGuard(t *testing.T) {
	dbPath := "test_single_writer.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + ".lock")
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

	l1, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	// Second read-write open on the same state refuses (no split brain).
	if l2, err := ledger.OpenLedger(dbPath); err == nil {
		l2.Close()
		l1.Close()
		t.Fatal("second writer must be refused")
	} else if !strings.Contains(err.Error(), "another daemon") {
		l1.Close()
		t.Fatalf("refusal must name the cause, got: %v", err)
	}
	// Read-only handles never take the lock.
	ro, err := ledger.OpenLedgerReadOnly(dbPath)
	if err != nil {
		l1.Close()
		t.Fatalf("read-only open beside a writer: %v", err)
	}
	ro.Close()
	// After close the state is openable again (lock releases, no stale).
	l1.Close()
	l3, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
	l3.Close()
}

// R10: Close is idempotent-safe — a second Close must not panic or
// corrupt lock state (database/sql Close is nil-returning when already
// closed), and the state must reopen cleanly afterwards.
func TestLedgerDoubleClose(t *testing.T) {
	dbPath := "test_double_close.db"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + ".lock")
	defer os.Remove(dbPath + "-wal")
	defer os.Remove(dbPath + "-shm")

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Must not panic; sql.DB.Close on a closed DB returns nil.
	_ = l.Close()
	l2, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("reopen after double close: %v", err)
	}
	l2.Close()
}
