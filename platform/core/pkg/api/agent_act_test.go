package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// Can-agent-act: the integrated proof that the agent loop is real, run in
// CI (go-core-tests). Three legs, each against production paths with a
// stubbed model — if any leg is assumed rather than exercised, this fails:
//  1. reach: an allowed read tool dispatches, observes, and audits.
//  2. refuse: a second per-turn dispatch refuses with the bound stated.
//  3. budget: repeat failures trip the breaker instead of looping forever.
func actTestServer(t *testing.T, dbPath string, responder func(call int, sid string) map[string]any) (*Server, *ledger.Ledger, string) {
	t.Helper()
	var calls int
	var sid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(responder(calls, sid))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("OPENCODE_API_KEY", "test-key")
	settings := llm.DefaultSettings()
	snap := settings.GetSnapshot()
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOpenCode] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role: "copilot", Provider: llm.ProviderOpenCode, Model: "stub",
	}
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}

	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	server := NewServer(l, sm, jr, sup, wd)
	server.llmClient = llm.NewClient(settings)

	sess, err := sm.CreateSession("can-act drill", 150, "")
	if err != nil {
		t.Fatal(err)
	}
	sid = sess.ID
	return server, l, sess.ID
}

func toolCallResponse(id, name, args string) map[string]any {
	return map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"content": "",
				"tool_calls": []any{map[string]any{
					"id":   id,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": args,
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"total_tokens": 10},
	}
}

func textResponse(text string) map[string]any {
	return map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"total_tokens": 10},
	}
}

func countLedger(t *testing.T, l *ledger.Ledger, sid, typ string) (int, []map[string]any) {
	t.Helper()
	events, err := l.GetRecentEvents(sid, 200)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, e := range events {
		if e.EventType != typ {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(e.Payload), &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return len(out), out
}

func TestCanAgentActReach(t *testing.T) {
	srv, l, sid := actTestServer(t, "test_can_act_reach.db", func(call int, sid string) map[string]any {
		if call == 1 {
			return toolCallResponse("c1", "list_jobs", `{"session_id":"`+sid+`"}`)
		}
		return textResponse("No jobs running; the session is idle.")
	})
	hist := srv.resolveWithTools(context.Background(), sid,
		[]llm.ChatMessage{{Role: "user", Content: "job status?"}})
	if len(hist) < 4 {
		t.Fatalf("reach leg must observe the tool (base+preamble+call+obs), got %d messages", len(hist))
	}
	n, payloads := countLedger(t, l, sid, "tool_called")
	if n != 1 || payloads[0]["tool"] != "list_jobs" || payloads[0]["ok"] != true {
		t.Errorf("reach leg must audit exactly one successful dispatch, got %v", payloads)
	}
}

func TestCanAgentActRefuse(t *testing.T) {
	srv, l, sid := actTestServer(t, "test_can_act_refuse.db", func(call int, sid string) map[string]any {
		if call <= 2 {
			return toolCallResponse("c1", "submit_local_sim",
				`{"session_id":"`+sid+`","qasm":"OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];\n"}`)
		}
		return textResponse("One dispatch per turn is enough; the rest goes in the proposal.")
	})
	srv.resolveWithTools(context.Background(), sid,
		[]llm.ChatMessage{{Role: "user", Content: "launch two circuits"}})
	n, payloads := countLedger(t, l, sid, "tool_called")
	if n != 2 {
		t.Fatalf("refuse leg must dispatch twice, got %d", n)
	}
	// GetRecentEvents returns newest-first: payloads[0] is the second call.
	first, second := payloads[1], payloads[0]
	if first["ok"] != true {
		t.Errorf("first dispatch must succeed: %v", first)
	}
	if second["ok"] != false || second["tool"] != "submit_local_sim" {
		t.Errorf("second dispatch must refuse at the bound: %v", second)
	}
	if errText, _ := second["error"].(string); errText == "" {
		t.Error("refusal must carry the bound as reason, not silence")
	}
}

func TestCanAgentActBudget(t *testing.T) {
	srv, l, sid := actTestServer(t, "test_can_act_budget.db", func(call int, sid string) map[string]any {
		// Malformed QASM fails identically every round: validation refuses
		// before admission, deterministically, with no side effects.
		return toolCallResponse("c1", "submit_local_sim",
			`{"session_id":"`+sid+`","qasm":"not a circuit"}`)
	})
	srv.resolveWithTools(context.Background(), sid,
		[]llm.ChatMessage{{Role: "user", Content: "launch this"}})
	n, _ := countLedger(t, l, sid, "tool_called")
	if n != 2 {
		t.Errorf("budget leg must stop after 2 identical failures, got %d dispatches", n)
	}
	nb, _ := countLedger(t, l, sid, "tool_loop_breaker")
	if nb != 1 {
		t.Errorf("exactly one breaker event, got %d", nb)
	}
}
