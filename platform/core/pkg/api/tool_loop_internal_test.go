package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/tools"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

// stubToolServer answers every CompleteWithTools call with one invocation
// of toolName and the given usage block. The turn then answers quietly.
func stubToolServer(t *testing.T, toolName string, usage map[string]any) (*llm.Client, func()) {
	t.Helper()
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		// First two rounds invoke the tool; the third answers quietly.
		// (A model that stops after one failure never exercises the
		// repeat-failure breaker — the stub must keep calling.)
		if calls < 3 {
			resp := map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"content": "",
						"tool_calls": []any{map[string]any{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      toolName,
								"arguments": `{}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}
			if usage != nil {
				resp["usage"] = usage
			} else {
				// W0.19 metered-usage rule: cloud stubs must report
				// usage, otherwise the tool loop refuses blind spend
				// and the breaker/observation paths never dispatch.
				resp["usage"] = map[string]any{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "weighed and answered from the observations"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"total_tokens": 10},
		})
	}))

	t.Setenv("OPENCODE_API_KEY", "test-key")
	settings := llm.DefaultSettings()
	snap := settings.GetSnapshot()
	// Tool-loop tests drive the cloud stub: opt out of the air-gap
	// default like an operator would.
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	pc.Enabled = true
	snap.Providers[llm.ProviderOpenCode] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role: "copilot", Provider: llm.ProviderOpenCode, Model: "stub",
	}
	if err := settings.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	return llm.NewClient(settings), srv.Close
}

func loopTestServer(t *testing.T, dbPath string) (*Server, *ledger.Ledger, func()) {
	t.Helper()
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)
	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return srv, l, cleanup
}

func countEvents(t *testing.T, l *ledger.Ledger, sid, typ string) int {
	t.Helper()
	events, err := l.GetRecentEvents(sid, 200)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range events {
		if e.EventType == typ {
			n++
		}
	}
	return n
}

// Repeat-failure breaker: the same tool failing twice ends the loop with
// a breaker event — never a third identical round.
func TestToolLoopBreaker(t *testing.T) {
	srv, l, cleanup := loopTestServer(t, "test_loop_breaker.db")
	defer cleanup()

	cli, closeStub := stubToolServer(t, "boom_tool", nil)
	defer closeStub()
	srv.llmClient = cli

	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name: "boom_tool", Description: "always fails",
		Parameters: map[string]any{"type": "object"},
		Capability: "test:write", ReadOnly: false,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return nil, errLoopTest("anchor unreachable")
		},
	})
	_ = reg.Allow(CopilotRole, "boom_tool")
	srv.toolRegistry = reg

	sess, err := srv.sessionMgr.CreateSession("breaker drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	out := srv.resolveWithTools(context.Background(), sess.ID,
		[]llm.ChatMessage{{Role: "user", Content: "go"}})
	_ = out

	if n := countEvents(t, l, sess.ID, "tool_called"); n != 2 {
		t.Errorf("breaker must stop after 2 identical failures, got %d dispatches", n)
	}
	if n := countEvents(t, l, sess.ID, "tool_loop_breaker"); n != 1 {
		t.Errorf("exactly one breaker event, got %d", n)
	}
	// Breaker events are not tool calls: attribution stays clean.
	for _, name := range []string{"boom_tool"} {
		if got := srv.toolNamesSince(sess.ID, 0); len(got) != 1 || got[0] != name {
			t.Errorf("attribution must list the two real dispatches once, got %v", got)
		}
	}
}

// Token guardrail: provider-reported usage past the turn cap stops the
// loop before any dispatch — recorded as a cap, never as a failure.
func TestToolLoopTokenCap(t *testing.T) {
	srv, l, cleanup := loopTestServer(t, "test_loop_tokencap.db")
	defer cleanup()

	cli, closeStub := stubToolServer(t, "boom_tool",
		map[string]any{"total_tokens": 200000})
	defer closeStub()
	srv.llmClient = cli

	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name: "boom_tool", Description: "never reached",
		Parameters: map[string]any{"type": "object"},
		Capability: "test:write", ReadOnly: false,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			t.Error("capped loop must not dispatch")
			return nil, errLoopTest("unreached")
		},
	})
	_ = reg.Allow(CopilotRole, "boom_tool")
	srv.toolRegistry = reg

	sess, err := srv.sessionMgr.CreateSession("tokencap drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	srv.resolveWithTools(context.Background(), sess.ID,
		[]llm.ChatMessage{{Role: "user", Content: "go"}})

	if n := countEvents(t, l, sess.ID, "tool_called"); n != 0 {
		t.Errorf("capped loop must dispatch nothing, got %d", n)
	}
	if n := countEvents(t, l, sess.ID, "tool_loop_capped"); n != 1 {
		t.Errorf("exactly one cap event, got %d", n)
	}
}

// Observation budget: a chatty tool cannot eat the turn; the cut is
// marked on the wire.
func TestToolObservationCapped(t *testing.T) {
	srv, _, cleanup := loopTestServer(t, "test_loop_obscap.db")
	defer cleanup()

	cli, closeStub := stubToolServer(t, "chatty_tool", nil)
	defer closeStub()
	srv.llmClient = cli

	big := strings.Repeat("datum-", 2000)
	reg := tools.NewRegistry()
	_ = reg.Register(tools.Definition{
		Name: "chatty_tool", Description: "huge output",
		Parameters: map[string]any{"type": "object"},
		Capability: "test:read", ReadOnly: true,
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"blob": big}, nil
		},
	})
	_ = reg.Allow(CopilotRole, "chatty_tool")
	srv.toolRegistry = reg

	sess, err := srv.sessionMgr.CreateSession("obscap drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	hist := srv.resolveWithTools(context.Background(), sess.ID,
		[]llm.ChatMessage{{Role: "user", Content: "go"}})

	cfg := llm.DefaultBudgetConfig()
	found := false
	for _, m := range hist {
		if m.Role != "tool" {
			continue
		}
		found = true
		if len([]rune(m.Content)) > cfg.MaxObservationChars+64 {
			t.Errorf("tool observation exceeds budget: %d runes", len([]rune(m.Content)))
		}
		if !strings.Contains(m.Content, "[observation truncated to budget]") {
			t.Error("truncation must be marked on the wire")
		}
	}
	if !found {
		t.Error("expected a tool observation in history")
	}
}

type errLoopTest string

func (e errLoopTest) Error() string { return string(e) }
