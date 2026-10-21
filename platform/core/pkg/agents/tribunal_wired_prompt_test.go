package agents_test

// Audit 2026-10-21: wired tribunal turns withheld the session id, so any
// session-scoped tool call (get_campaign, list_jobs) failed with "unknown
// session" and contested the run — punishing the model for doing the right
// thing (proven live: turn 3 get_campaign("h2-ground-state-vqe") refused).
// Wired turns must name the session id verbatim for tool use.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestWiredTurnPromptsNameSessionID(t *testing.T) {
	l, sid := mustOpenTribunalLedger(t)

	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		raw, _ := json.Marshal(req)
		bodies = append(bodies, string(raw))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "stub turn content, more than twenty characters long"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
		})
	}))
	defer srv.Close()

	t.Setenv("KUBONA_LLM_PROVIDER", "strata")
	t.Setenv("STRATA_MODEL", "stub-model")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderStrata]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderStrata] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("settings: %v", err)
	}
	client := llm.NewClient(s)

	ac := agents.NewAgentCollective()
	ac.Ledger = l
	if _, err := ac.ExecuteTribunalWithSession(context.Background(), client, nil, nil, sid, "Investigate H2 ground state"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(bodies) == 0 {
		t.Fatal("expected stub LLM to receive turn calls")
	}
	for i, b := range bodies {
		if !strings.Contains(b, sid) {
			t.Errorf("turn call %d prompt must name wired session id %s verbatim", i, sid)
		}
		if !strings.Contains(b, "session_id") {
			t.Errorf("turn call %d prompt must label the id for tool use", i)
		}
	}
}
