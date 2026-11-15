package api

import (
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestNotificationProjectionDerivesTerminalEventsAndState(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/notifications.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	const sid = "sess-notify"
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, 'n', 'default', ?, ?, 'active', 0, 0)`,
		sid, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "job_started", map[string]any{"id": "job-1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "job_settled", map[string]any{"id": "job-1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "loop_completed", map[string]any{"loop_id": "loop-1"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{ledger: l}
	rows, unread, err := s.notificationList(sid, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || unread != 2 {
		t.Fatalf("rows=%d unread=%d, want 2/2: %+v", len(rows), unread, rows)
	}
	for _, row := range rows {
		if row.Kind != "job" && row.Kind != "run" {
			t.Fatalf("unexpected non-terminal notification %+v", row)
		}
	}
	keys := []string{rows[0].Key}
	if err := l.MarkNotifications(sid, keys, false); err != nil {
		t.Fatal(err)
	}
	rows, unread, err = s.notificationList(sid, 100)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 1 {
		t.Fatalf("unread after read = %d, want 1", unread)
	}
	if len(rows) != 2 {
		t.Fatalf("read notification disappeared: %+v", rows)
	}
	if err := l.MarkNotifications(sid, keys, true); err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.notificationList(sid, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("resolved notification still listed: %+v", rows)
	}
}
