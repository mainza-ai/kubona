package session_test

import (
	"crypto/ed25519"
	"encoding/json"
	"math"
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
	if err := mgr.DeleteSession(sess.ID, testDeleteSigner()); err != nil {
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
	// W0.7: over-release refuses without mutating; invalid amounts refuse;
	// ghost sessions refuse.
	if err := mgr.ReleaseSpend(sess.ID, 1); err == nil {
		t.Fatal("over-release must refuse")
	}
	if got, _ := mgr.GetSession(sess.ID); got.CurrentSpendUSD != 0 {
		t.Fatalf("refused release must leave books at 0, got %v", got.CurrentSpendUSD)
	}
	for _, amt := range []float64{-5.0, math.NaN(), math.Inf(1)} {
		if err := mgr.ReserveSpend(sess.ID, amt); err == nil {
			t.Fatalf("ReserveSpend(%v) must refuse", amt)
		}
		if err := mgr.ReleaseSpend(sess.ID, amt); err == nil {
			t.Fatalf("ReleaseSpend(%v) must refuse", amt)
		}
	}
	if err := mgr.ReleaseSpend("ghost-session", 10); err == nil {
		t.Fatal("ghost release must refuse")
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
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES ('legacy-nolimit','t','default',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'active',0,0)`); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ReserveSpend("legacy-nolimit", 1000000); err != nil {
		t.Fatalf("uncapped session must admit any amount: %v", err)
	}
	if err := mgr.ReserveSpend("no-such-session", 10); err == nil {
		t.Fatal("unknown session must refuse, not pass open")
	}
}

func TestClearMessagesAndRegenerate(t *testing.T) {
	dbPath := "test_session_chatops.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	mgr := session.NewManager(l)
	sess, err := mgr.CreateSession("ChatOps", 50.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if _, err := mgr.AddMessage(sess.ID, "user", "first question", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddMessage(sess.ID, "assistant", "first answer", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddMessage(sess.ID, "user", "second question", nil); err != nil {
		t.Fatal(err)
	}
	stale, err := mgr.AddMessage(sess.ID, "assistant", "stale answer", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Regenerate drops the trailing assistant reply, keeps the user prompt.
	prompt, err := mgr.DropTrailingAssistant(sess.ID)
	if err != nil {
		t.Fatalf("DropTrailingAssistant failed: %v", err)
	}
	if prompt != "second question" {
		t.Errorf("expected surviving prompt 'second question', got %q", prompt)
	}
	msgs, _ := mgr.GetMessages(sess.ID)
	for _, m := range msgs {
		if m.ID == stale.ID {
			t.Error("stale assistant reply survives DropTrailingAssistant")
		}
	}

	// Usage persistence round-trips through GetMessages.
	usage := json.RawMessage(`{"completion_tokens": 42}`)
	if err := mgr.SetMessageUsage(msgs[1].ID, 1234, &usage); err != nil {
		t.Fatalf("SetMessageUsage failed: %v", err)
	}
	msgs, _ = mgr.GetMessages(sess.ID)
	if msgs[1].DurationMs == nil || *msgs[1].DurationMs != 1234 {
		t.Error("duration_ms did not persist")
	}
	if msgs[1].UsageJSON == nil {
		t.Error("usage_json did not persist")
	}

	// Clear wipes the transcript but keeps the session.
	n, err := mgr.ClearMessages(sess.ID)
	if err != nil {
		t.Fatalf("ClearMessages failed: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 deleted messages, got %d", n)
	}
	msgs, _ = mgr.GetMessages(sess.ID)
	if len(msgs) != 0 {
		t.Errorf("expected empty transcript, got %d messages", len(msgs))
	}
	if _, err := mgr.GetSession(sess.ID); err != nil {
		t.Errorf("session must survive ClearMessages: %v", err)
	}
	if _, err := mgr.DropTrailingAssistant(sess.ID); err == nil {
		t.Error("regenerate on empty transcript must refuse")
	}
}

func TestRewriteUserMessageBranches(t *testing.T) {
	dbPath := "test_session_rewrite.db"
	defer os.Remove(dbPath)

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatalf("failed to open ledger: %v", err)
	}
	defer l.Close()

	mgr := session.NewManager(l)
	sess, err := mgr.CreateSession("Rewrite", 50.0, "default")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	u1, _ := mgr.AddMessage(sess.ID, "user", "original question", nil)
	mgr.AddMessage(sess.ID, "assistant", "original answer", nil)

	rewritten, err := mgr.RewriteUserMessage(sess.ID, u1.ID, "edited question", "")
	if err != nil {
		t.Fatalf("RewriteUserMessage failed: %v", err)
	}
	if rewritten.Content != "edited question" {
		t.Errorf("expected edited content, got %q", rewritten.Content)
	}
	msgs, _ := mgr.GetMessages(sess.ID)
	if len(msgs) != 1 || msgs[0].ID != u1.ID {
		t.Errorf("expected truncated branch (1 message), got %d", len(msgs))
	}

	a2, _ := mgr.AddMessage(sess.ID, "assistant", "new answer", nil)
	if _, err := mgr.RewriteUserMessage(sess.ID, a2.ID, "sneaky", ""); err == nil {
		t.Error("rewriting an assistant message must refuse")
	}
	if _, err := mgr.RewriteUserMessage(sess.ID, u1.ID, "", ""); err == nil {
		t.Error("empty rewrite must refuse")
	}
	if _, err := mgr.RewriteUserMessage(sess.ID, "missing-id", "x", ""); err == nil {
		t.Error("unknown message id must refuse")
	}
	// S7: compare-and-swap guard — stale expected content refuses.
	if _, err := mgr.RewriteUserMessage(sess.ID, u1.ID, "race", "stale guess"); err != session.ErrContentMismatch {
		t.Errorf("stale CAS must return ErrContentMismatch, got %v", err)
	}
	if _, err := mgr.RewriteUserMessage(sess.ID, u1.ID, "second edit", "edited question"); err != nil {
		t.Errorf("matching CAS must proceed: %v", err)
	}
}

func testDeleteSigner() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestSessionSpendMicroArithmeticIsExact(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/micro.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	mgr := session.NewManager(l)
	sess, err := mgr.CreateSession("micro-drill", 1000.0, "default")
	if err != nil {
		t.Fatal(err)
	}
	// 10k x $0.000001 is exactly $0.01 in micro-USD; float accumulation
	// would drift (the old REAL path did).
	for i := 0; i < 10_000; i++ {
		if err := mgr.ReserveSpend(sess.ID, 0.000001); err != nil {
			t.Fatalf("reserve %d: %v", i, err)
		}
	}
	got, err := mgr.GetSession(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentSpendUSD != 0.01 {
		t.Fatalf("micro reserve drifted: got %v want 0.01", got.CurrentSpendUSD)
	}
	if err := mgr.ReleaseSpend(sess.ID, 0.01); err != nil {
		t.Fatal(err)
	}
	got, _ = mgr.GetSession(sess.ID)
	if got.CurrentSpendUSD != 0 {
		t.Fatalf("micro release left dust: %v", got.CurrentSpendUSD)
	}
}
