package api_test

// S6 — event taxonomy conformance. Threat: untyped event strings drift —
// a typo'd type silently forks the audit trail. Every event_type emitted
// by exercised flows must be registered in ledger.KnownEventTypes; adding
// a type means adding its row + payload contract there.

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestEventTypesRegistered(t *testing.T) {
	srv, l, cleanup := setupIdemServer(t)
	defer cleanup()
	t.Setenv("KUBONA_API_TOKEN", "op-token")
	t.Setenv("KUBONA_LEASE_HMAC_KEY", testLeaseKey)
	h := srv.Handler()
	sid := idemSession(t, h, "taxonomy")

	// Broad flow sweep: chat, streamless tribunal seed path via lease +
	// jobs + launch + loop lifecycle + leases + canvas + export/import.
	if code, _, _ := idemPost(t, h, "/api/chat/messages", map[string]any{"session_id": sid, "content": "taxonomy drill"}); code != 201 {
		t.Fatalf("chat: %d", code)
	}
	qasm := map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator", "shots": 8}
	if code, _, _ := idemPost(t, h, "/api/jobs", qasm); code != 201 {
		t.Fatalf("jobs: %d", code)
	}
	if code, _, _ := idemPost(t, h, "/api/chat/launch", map[string]any{"session_id": sid, "qasm": idemQASM, "backend": "local_simulator"}); code != 201 {
		t.Fatalf("launch: %d", code)
	}
	if code, _, _ := idemPost(t, h, "/api/autoresearch/start", map[string]any{"session_id": sid, "harness_id": "ansatz-search", "max_iterations": 1}); code != 201 {
		t.Fatalf("autoresearch: %d", code)
	}
	if code, _ := leasePost(t, h, "/api/vault/leases", map[string]any{"scope": "provider:test", "session_id": sid}); code != 200 {
		t.Fatalf("lease: %d", code)
	}
	if code, _ := migPost(t, h, "/api/sessions/export", map[string]any{"session_id": sid}); code != 200 {
		t.Fatalf("export: %d", code)
	}
	if code, _, _ := idemPost(t, h, "/api/canvas/layout", map[string]any{"session_id": sid, "layout": "{}"}); code != 200 && code != 201 && code != 404 {
		t.Fatalf("canvas: %d", code)
	}

	rows, err := l.DB().Query(`SELECT DISTINCT event_type FROM ledger_events`)
	if err != nil {
		t.Fatalf("distinct: %v", err)
	}
	defer rows.Close()
	var unregistered []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if !ledger.IsKnownEventType(et) {
			unregistered = append(unregistered, et)
		}
	}
	if len(unregistered) > 0 {
		t.Fatalf("unregistered event types emitted: %v (add rows to KnownEventTypes with payload contracts)", unregistered)
	}
	if len(ledger.KnownEventTypes) < 50 {
		t.Fatalf("registry looks gutted: %d rows", len(ledger.KnownEventTypes))
	}
}
