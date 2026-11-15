package ledger

import "testing"

func TestChatFeedbackUpsertHydratesAndClears(t *testing.T) {
	l, err := OpenLedger(t.TempDir() + "/feedback.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	const sid = "sess-feedback"
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, 'f', 'default', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active', 0, 0)`, sid); err != nil {
		t.Fatal(err)
	}
	previous, err := l.UpsertChatFeedback(sid, "msg-1", "up", "", "local_researcher")
	if err != nil || previous != "" {
		t.Fatalf("first vote previous=%q err=%v", previous, err)
	}
	previous, err = l.UpsertChatFeedback(sid, "msg-1", "down", "wrong anchor", "lab_admin:lab")
	if err != nil || previous != "up" {
		t.Fatalf("replace previous=%q err=%v", previous, err)
	}
	rows, err := l.ListChatFeedback(sid)
	if err != nil || len(rows) != 1 || rows[0].Vote != "down" || rows[0].Reason != "wrong anchor" {
		t.Fatalf("hydrate rows=%+v err=%v", rows, err)
	}
	previous, err = l.UpsertChatFeedback(sid, "msg-1", "", "", "lab_admin:lab")
	if err != nil || previous != "down" {
		t.Fatalf("clear previous=%q err=%v", previous, err)
	}
	rows, _ = l.ListChatFeedback(sid)
	if len(rows) != 0 {
		t.Fatalf("cleared feedback still present: %+v", rows)
	}
}
