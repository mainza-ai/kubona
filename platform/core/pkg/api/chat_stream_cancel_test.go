package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestCancelMidStreamKeepsPartialTextAndSpend(t *testing.T) {
	const partial = "partial-text"
	var mu sync.Mutex
	var bodies [][]byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, append([]byte(nil), raw...))
		mu.Unlock()
		if !strings.Contains(string(raw), `"stream":true`) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"message":       map[string]any{"role": "assistant", "content": "next answer"},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3},
			})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\""+partial+"\"}}],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":4,\"total_tokens\":15}}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	t.Setenv("KUBONA_API_TOKEN", "op-token")
	t.Setenv("GROQ_API_KEY", "gsk-test")
	srv, _, cleanup := loopTestServer(t, "test_stream_cancel.db")
	defer cleanup()
	snap := srv.llmSettings.GetSnapshot()
	snap.AirGapped = false
	snap.ActiveProvider = llm.ProviderGroq
	snap.ActiveModel = "groq-test"
	pc := snap.Providers[llm.ProviderGroq]
	pc.BaseURL = upstream.URL + "/v1"
	pc.Enabled = true
	pc.APIKey = "gsk-test"
	pc.Wire = llm.WireChatCompletions
	pc.DefaultModel = "groq-test"
	snap.Providers[llm.ProviderGroq] = pc
	for role, assign := range snap.RoleAssignments {
		assign.Provider = llm.ProviderOMLX
		if assign.Model == "" {
			assign.Model = "local"
		}
		snap.RoleAssignments[role] = assign
	}
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Role: "copilot", Provider: llm.ProviderGroq, Model: "groq-test"}
	if err := srv.llmSettings.Update(&snap, srv.ledger.DB()); err != nil {
		t.Fatal(err)
	}
	srv.llmClient = llm.NewClient(srv.llmSettings)
	var spent []llm.SpendRecord
	srv.llmClient.OnSpend = func(rec llm.SpendRecord) {
		mu.Lock()
		spent = append(spent, rec)
		mu.Unlock()
	}
	srv.toolRegistry = nil

	sess, err := srv.sessionMgr.CreateSession("cancel mid stream", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	apiSrv := httptest.NewServer(srv.Handler())
	defer apiSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"session_id": sess.ID, "role": "user", "content": "P0-5-CANCEL"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiSrv.URL+"/api/chat/stream?session_id="+sess.ID, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer op-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	saw := false
	for sc.Scan() {
		if strings.Contains(sc.Text(), partial) {
			saw = true
			cancel()
		}
	}
	if !saw {
		t.Fatalf("stream never emitted the partial token: %s", resp.Status)
	}

	var assistant, notice string
	deadline := time.Now().Add(3 * time.Second)
	for {
		msgs, err := srv.sessionMgr.GetMessages(sess.ID)
		if err != nil {
			t.Fatal(err)
		}
		assistant, notice = "", ""
		for _, m := range msgs {
			if m.Role == "assistant" && strings.Contains(m.Content, partial) {
				assistant = m.Content
			}
			if m.Role == chatRoleNotice {
				notice = m.Content
			}
		}
		if assistant != "" && notice != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stored messages missing partial or notice: assistant=%q notice=%q", assistant, notice)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(assistant, "[stopped]") {
		t.Fatalf("partial turn outcome = %q, want [stopped]", assistant)
	}
	if strings.Contains(assistant, "Notice]") {
		t.Fatal("provider notice was stored as the assistant answer")
	}
	if !strings.Contains(notice, "Notice]") {
		t.Fatalf("notice role content = %q", notice)
	}
	mu.Lock()
	gotSpend := append([]llm.SpendRecord(nil), spent...)
	mu.Unlock()
	if len(gotSpend) != 1 || gotSpend[0].PromptTokens != 11 || gotSpend[0].CompletionTokens != 4 || gotSpend[0].Unmetered || gotSpend[0].CeilingApplied {
		t.Fatalf("cancel spend = %+v, want reported 11/4", gotSpend)
	}

	nextRaw, _ := json.Marshal(map[string]any{"session_id": sess.ID, "role": "user", "content": "P0-5-NEXT"})
	nextReq, err := http.NewRequest(http.MethodPost, apiSrv.URL+"/api/chat/messages?session_id="+sess.ID, bytes.NewReader(nextRaw))
	if err != nil {
		t.Fatal(err)
	}
	nextReq.Header.Set("Authorization", "Bearer op-token")
	nextReq.Header.Set("Content-Type", "application/json")
	nextResp, err := http.DefaultClient.Do(nextReq)
	if err != nil {
		t.Fatal(err)
	}
	defer nextResp.Body.Close()
	nextBody, _ := io.ReadAll(nextResp.Body)
	if nextResp.StatusCode != http.StatusCreated {
		t.Fatalf("follow-up chat: %d %s", nextResp.StatusCode, nextBody)
	}
	mu.Lock()
	defer mu.Unlock()
	var follow []byte
	for _, body := range bodies {
		if strings.Contains(string(body), "P0-5-NEXT") {
			follow = body
		}
	}
	if follow == nil {
		t.Fatal("follow-up call did not reach the provider")
	}
	if !strings.Contains(string(follow), partial) {
		t.Fatal("follow-up history dropped the partial text")
	}
	if strings.Contains(string(follow), "Notice]") {
		t.Fatal("follow-up history replayed the provider notice")
	}
}
