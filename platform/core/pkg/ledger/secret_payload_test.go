package ledger

import (
	"strings"
	"testing"
)

func TestAppendEventRedactsCredentialShapes(t *testing.T) {
	l, err := OpenLedger(t.TempDir() + "/secret-payload.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	const sid = "sess-secret-payload"
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, created_at, updated_at, status, spend_limit_micro_usd, current_spend_micro_usd) VALUES (?, 's', 'default', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active', 0, 0)`, sid); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendEvent(sid, "tool_called", map[string]any{
		"tool": "leak", "note": "token sk-abcdefghijklmnopqrstuvwxyz123456",
	}); err != nil {
		t.Fatal(err)
	}
	events, err := l.GetRecentEvents(sid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	body := string(events[0].Payload)
	if strings.Contains(body, "sk-abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("credential leaked into ledger: %s", body)
	}
	if !strings.Contains(body, "redacted:openai_key") {
		t.Fatalf("expected visible redaction marker: %s", body)
	}
}
