package llm_test

// B3 client pins: the guard runs before the wire request, its ceiling is
// applied, and its release is called on success and refusal.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

type fakeGuard struct {
	ceiling  int
	err      error
	reserves atomic.Int32
	releases atomic.Int32
	lastPlan atomic.Int32
}

func (g *fakeGuard) Reserve(_ llm.ProviderType, _ string, _ []llm.ChatMessage, planned int) (int, func(), error) {
	g.reserves.Add(1)
	g.lastPlan.Store(int32(planned))
	if g.err != nil {
		return 0, nil, g.err
	}
	return g.ceiling, func() { g.releases.Add(1) }, nil
}

func TestSpendGuardBoundsUnaryCall(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"content": "hi"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7},
		})
	}))
	defer srv.Close()

	guard := &fakeGuard{ceiling: 77}
	client := llm.NewClient(toolTestSettings(t, srv.URL))
	client.SetSpendGuard(guard)
	if _, _, _, err := client.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if guard.reserves.Load() != 1 {
		t.Fatalf("guard must reserve once, got %d", guard.reserves.Load())
	}
	if guard.releases.Load() != 1 {
		t.Fatalf("guard must release after accounting, got %d", guard.releases.Load())
	}
	if gotBody["max_tokens"] != float64(77) {
		t.Fatalf("ceiling must reach the wire, got %v", gotBody["max_tokens"])
	}
}

func TestSpendGuardRefusalStopsTheCall(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	guard := &fakeGuard{err: errors.New("budget exhausted")}
	client := llm.NewClient(toolTestSettings(t, srv.URL))
	client.SetSpendGuard(guard)
	if _, _, _, err := client.Complete(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("guard refusal must fail the call")
	}
	if requests.Load() != 0 {
		t.Fatalf("nothing may reach the provider after a refusal, got %d requests", requests.Load())
	}
}

func TestSpendGuardBoundsStream(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	guard := &fakeGuard{ceiling: 33}
	client := llm.NewClient(toolTestSettings(t, srv.URL))
	client.SetSpendGuard(guard)
	if _, err := client.CompleteStream(context.Background(), "copilot",
		[]llm.ChatMessage{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if gotBody["max_tokens"] != float64(33) {
		t.Fatalf("stream ceiling must reach the wire, got %v", gotBody["max_tokens"])
	}
	if guard.releases.Load() != 1 {
		t.Fatalf("stream must release once, got %d", guard.releases.Load())
	}
}
