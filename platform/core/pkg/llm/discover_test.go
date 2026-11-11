package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestDiscoverLocalWithTargetsOllamaAndOpenAICompat(t *testing.T) {
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []any{map[string]any{"name": "qwen3:8b"}},
			})
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"parameters": "num_ctx 4096\ntemperature 0.7",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollamaSrv.Close()

	lmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{
					map[string]any{"id": "qwen2.5-coder-7b", "owned_by": "lmstudio", "max_model_len": 32768},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer lmSrv.Close()

	parsePort := func(raw string) int {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			t.Fatal(err)
		}
		return p
	}

	oPort := parsePort(ollamaSrv.URL)
	lPort := parsePort(lmSrv.URL)
	closedPort := func() int {
		s := httptest.NewServer(http.NewServeMux())
		p := parsePort(s.URL)
		s.Close()
		return p
	}()

	res := llm.DiscoverLocalTargetsForTest(context.Background(), []int{oPort, lPort, closedPort})
	if len(res.Servers) != 2 {
		t.Fatalf("expected 2 reachable servers (ignoring closed port), got %d (%+v)", len(res.Servers), res.Servers)
	}
	var sawOllama, sawLM bool
	for _, s := range res.Servers {
		if s.Port == oPort {
			sawOllama = true
			if s.Provider != "ollama" || s.Wire != llm.WireOllamaNative || s.ContextWindow != 4096 || s.ContextWarning == "" {
				t.Fatalf("expected ollama metadata + <8192 num_ctx warning, got %+v", s)
			}
		}
		if s.Port == lPort {
			sawLM = true
			if s.Provider != "lmstudio" || s.Wire != llm.WireChatCompletions || s.ContextWindow != 32768 || len(s.Models) != 1 {
				t.Fatalf("expected lmstudio canonical slug + 32768 context, got %+v", s)
			}
		}
	}
	if !sawOllama || !sawLM {
		t.Fatalf("missing discovered servers: ollama=%v lm=%v", sawOllama, sawLM)
	}
}
