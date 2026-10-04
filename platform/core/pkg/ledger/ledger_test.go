package ledger_test

import (
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
