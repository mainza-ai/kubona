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
	sess, err := mgr.CreateSession("H2 PES Exploration", 120.0)
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
