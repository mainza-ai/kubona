package session_test

import (
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

func TestSessionLifecycleAndDelete(t *testing.T) {
	dbPath := "test_session_lifecycle.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	mgr := session.NewManager(l)

	// Create a session
	sess, err := mgr.CreateSession("H2 PES Exploration", 120.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if sess.ID == "" {
		t.Fatalf("expected non-empty session ID")
	}

	// Add a message
	_, err = mgr.AddMessage(sess.ID, "user", "Run VQE with UCCSD ansatz", nil)
	if err != nil {
		t.Fatalf("failed to add message: %v", err)
	}

	// Fetch messages
	msgs, err := mgr.GetMessages(sess.ID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Verify session can be retrieved
	fetched, err := mgr.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	if fetched == nil || fetched.Title != "H2 PES Exploration" {
		t.Fatalf("unexpected fetched session: %+v", fetched)
	}

	// Delete session
	if err := mgr.DeleteSession(sess.ID); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Verify session is gone
	deleted, err := mgr.GetSession(sess.ID)
	if err == nil || deleted != nil {
		t.Fatalf("expected error getting deleted session, got session=%+v, err=%v", deleted, err)
	}

	// Verify messages are deleted
	msgsAfter, err := mgr.GetMessages(sess.ID)
	if err != nil {
		t.Fatalf("failed to get messages after deletion: %v", err)
	}
	if len(msgsAfter) != 0 {
		t.Fatalf("expected 0 messages after deletion, got %d", len(msgsAfter))
	}
}

func TestSessionTenantBinding(t *testing.T) {
	dbPath := "test_session_tenant.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	mgr := session.NewManager(l)

	// Unknown tenant must refuse: sessions belong to registered lab tenants.
	if _, err := mgr.CreateSession("Rogue", 10.0, "no-such-lab"); err == nil {
		t.Fatalf("expected refusal for unknown tenant")
	}

	// Empty tenant normalizes to the seeded default lab.
	sess, err := mgr.CreateSession("Default lab work", 10.0, "")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if sess.TenantID != "default" {
		t.Fatalf("expected default tenant, got %q", sess.TenantID)
	}
	fetched, err := mgr.GetSession(sess.ID)
	if err != nil || fetched.TenantID != "default" {
		t.Fatalf("tenant not round-tripped: %+v err=%v", fetched, err)
	}
}

func TestSessionReserveBindsTighterThanTenant(t *testing.T) {
	dbPath := "test_session_spend.db"
	defer func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	mgr := session.NewManager(l)

	sess, err := mgr.CreateSession("capped", 120.0, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.ReserveSpend(sess.ID, 100); err != nil {
		t.Fatalf("first $100 on a $120 session must pass: %v", err)
	}
	if err := mgr.ReserveSpend(sess.ID, 100); err == nil {
		t.Fatal("second $100 on a $120 session must refuse")
	}
	if err := mgr.ReserveSpend(sess.ID, 20); err != nil {
		t.Fatalf("$20 within remaining budget must pass: %v", err)
	}
	if err := mgr.ReleaseSpend(sess.ID, 120); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentSpendUSD != 0 {
		t.Fatalf("spend must release to 0, got %v", got.CurrentSpendUSD)
	}
}

func TestSessionUncappedLimitUnaffected(t *testing.T) {
	dbPath := "test_session_nolimit.db"
	defer func() {
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	mgr := session.NewManager(l)

	// Limit 0 = uncapped (legacy rows; CreateSession normalizes to 100):
	// pre-enforcement sessions keep working exactly.
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES ('legacy-nolimit','t','default',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'active',0,0)`); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ReserveSpend("legacy-nolimit", 1000000); err != nil {
		t.Fatalf("uncapped session must admit any amount: %v", err)
	}
	if err := mgr.ReserveSpend("no-such-session", 10); err == nil {
		t.Fatal("unknown session must refuse, not pass open")
	}
}
