package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

func TestLatestUserTurnAppearsOnce(t *testing.T) {
	const prompt = "P0-6-UNIQUE-PROMPT"
	var bodies [][]byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, raw)
		if strings.Contains(string(raw), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "answered"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
	defer upstream.Close()

	srv, _, cleanup := loopTestServer(t, "test_chat_history_once.db")
	defer cleanup()
	snap := srv.llmSettings.GetSnapshot()
	snap.AirGapped = true
	snap.ActiveProvider = llm.ProviderOMLX
	snap.ActiveModel = "stub"
	pc := snap.Providers[llm.ProviderOMLX]
	pc.BaseURL = upstream.URL + "/v1"
	pc.Enabled = true
	pc.IsLocal = true
	pc.DefaultModel = "stub"
	snap.Providers[llm.ProviderOMLX] = pc
	for role, assign := range snap.RoleAssignments {
		assign.Provider = llm.ProviderOMLX
		assign.Model = "stub"
		snap.RoleAssignments[role] = assign
	}
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Role: "copilot", Provider: llm.ProviderOMLX, Model: "stub"}
	if err := srv.llmSettings.Update(&snap, srv.ledger.DB()); err != nil {
		t.Fatal(err)
	}
	srv.llmClient = llm.NewClient(srv.llmSettings)
	srv.toolRegistry = nil

	sess, err := srv.sessionMgr.CreateSession("history once", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.sessionMgr.AddMessage(sess.ID, "user", "earlier turn", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.sessionMgr.AddMessage(sess.ID, "assistant", "earlier answer", nil); err != nil {
		t.Fatal(err)
	}

	post := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"session_id": sess.ID, "role": "user", "content": prompt})
		req := httptest.NewRequest(http.MethodPost, path+"?session_id="+sess.ID, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		return w
	}

	unary := post("/api/chat/messages")
	if unary.Code != http.StatusCreated {
		t.Fatalf("unary chat: %d %s", unary.Code, unary.Body.String())
	}
	if len(bodies) != 1 {
		t.Fatalf("unary chat made %d model calls, want 1", len(bodies))
	}
	if got := countWireTurn(t, bodies[0], "user", prompt); got != 1 {
		t.Fatalf("unary wire contains the latest user turn %d times, want 1", got)
	}
	if got := countWireTurn(t, bodies[0], "user", "earlier turn"); got != 1 {
		t.Fatalf("unary wire contains the earlier turn %d times, want 1", got)
	}

	bodies = nil
	streamSess, err := srv.sessionMgr.CreateSession("history stream", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.sessionMgr.AddMessage(streamSess.ID, "user", "earlier turn", nil); err != nil {
		t.Fatal(err)
	}
	sess = streamSess
	stream := post("/api/chat/stream")
	if stream.Code != http.StatusOK || !strings.Contains(stream.Body.String(), "event: done") {
		t.Fatalf("stream chat: %d %s", stream.Code, stream.Body.String())
	}
	if len(bodies) != 1 {
		t.Fatalf("stream made %d model calls, want 1", len(bodies))
	}
	if got := countWireTurn(t, bodies[0], "user", prompt); got != 1 {
		t.Fatalf("stream wire contains the latest user turn %d times, want 1", got)
	}

	msgs, err := srv.sessionMgr.GetMessages(streamSess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := countStored(msgs, "user", prompt); n != 1 {
		t.Fatalf("stored user rows for the prompt = %d, want 1", n)
	}

	bodies = nil
	regenRaw, _ := json.Marshal(map[string]any{"session_id": streamSess.ID})
	regenReq := httptest.NewRequest(http.MethodPost, "/api/chat/regenerate", bytes.NewReader(regenRaw))
	regenReq.Header.Set("Content-Type", "application/json")
	regenW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(regenW, regenReq)
	if regenW.Code != http.StatusCreated {
		t.Fatalf("regenerate: %d %s", regenW.Code, regenW.Body.String())
	}
	if len(bodies) != 1 || countWireTurn(t, bodies[0], "user", prompt) != 1 {
		t.Fatalf("regenerate wire duplicated the stored prompt: calls=%d", len(bodies))
	}
	msgs, err = srv.sessionMgr.GetMessages(streamSess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := countStored(msgs, "user", prompt); n != 1 {
		t.Fatalf("regenerate stored the prompt %d times, want 1", n)
	}
}

func countWireTurn(t *testing.T, body []byte, role, content string) int {
	t.Helper()
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("model request is not JSON: %v body=%s", err, body)
	}
	n := 0
	for _, m := range payload.Messages {
		if m.Role == role && m.Content == content {
			n++
		}
	}
	return n
}

func countStored(msgs []session.ChatMessage, role, content string) int {
	n := 0
	for _, m := range msgs {
		if m.Role == role && m.Content == content {
			n++
		}
	}
	return n
}
