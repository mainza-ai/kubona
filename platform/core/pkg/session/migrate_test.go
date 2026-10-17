package session

// S5/I4 — DeleteSession fail-closed cascade. Threat: deleting live work
// or money trails silently (G6: previously orphaned 5 of 8 tables).

import (
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenCascade(t *testing.T) (*Manager, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/cascade.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return NewManager(l), l
}

func seedCascadeSession(t *testing.T, m *Manager, l *ledger.Ledger, sid string) {
	t.Helper()
	s := &Session{ID: sid, Title: "cascade", TenantID: "default", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Status: "active", SpendLimitUSD: 100}
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		s.ID, s.Title, s.TenantID, s.CreatedAt, s.UpdatedAt, s.Status, s.SpendLimitUSD); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := m.AddMessage(sid, "user", "hello", nil); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	if err := l.AppendEvent(sid, "seed_event", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := l.SaveLoopState(ledger.LoopPersist{ID: "loop-" + sid, SessionID: sid, HarnessID: "ansatz-search", Status: "running", MaxSteps: 5, LastActivity: time.Now().UTC()}); err != nil {
		t.Fatalf("seed loop: %v", err)
	}
}

func TestDeleteSessionRefusesLiveWork(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-live")
	if err := m.DeleteSession("sess-live"); err == nil {
		t.Fatalf("delete with a running loop must refuse")
	} else if !strings.Contains(strings.ToLower(err.Error()), "live loops") {
		t.Fatalf("refusal must name live loops, got %v", err)
	}
	// Non-terminal queued job with a reservation blocks too.
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount) VALUES ('j1', 'sess-live', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'default', 5.0)`); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := m.DeleteSession("sess-live"); err == nil {
		t.Fatalf("delete with reserved job must refuse")
	} else if !strings.Contains(strings.ToLower(err.Error()), "non-terminal jobs") {
		t.Fatalf("refusal must name non-terminal jobs, got %v", err)
	}
}

func TestDeleteSessionCascadesAll(t *testing.T) {
	m, l := mustOpenCascade(t)
	seedCascadeSession(t, m, l, "sess-gone")
	// Settle: abort the loop, complete the books (no jobs seeded here).
	if _, err := l.DB().Exec(`UPDATE loops SET status = 'aborted' WHERE session_id = 'sess-gone'`); err != nil {
		t.Fatalf("settle loop: %v", err)
	}
	if err := m.DeleteSession("sess-gone"); err != nil {
		t.Fatalf("settled delete must succeed: %v", err)
	}
	tables := map[string]string{
		"chat_messages": "session_id", "campaign_states": "session_id",
		"campaign_versions": "session_id", "claim_records": "session_id",
		"canvas_layouts": "session_id", "jobs": "session_id", "loops": "session_id",
		"idempotency_keys": "session_id",
		"sessions":         "id",
	}
	for table, col := range tables {
		var n int
		if err := l.DB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+col+` = ?`, "sess-gone").Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("table %s must be empty after cascade, got %d rows", table, n)
		}
	}
	// Exactly one event survives by design: the session_deleted receipt,
	// which must outlive the rows it attests.
	var evs int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE session_id = ?`, "sess-gone").Scan(&evs); err != nil || evs != 1 {
		t.Fatalf("exactly the deletion receipt must survive: %v %d", err, evs)
	}
	var etype string
	if err := l.DB().QueryRow(`SELECT event_type FROM ledger_events WHERE session_id = ?`, "sess-gone").Scan(&etype); err != nil || etype != "session_deleted" {
		t.Fatalf("survivor must be session_deleted, got %v %q", err, etype)
	}
	if err := m.DeleteSession("sess-ghost"); err == nil {
		t.Fatalf("ghost delete must refuse")
	}
}
