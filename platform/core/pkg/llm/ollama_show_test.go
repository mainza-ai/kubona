package llm_test

// B13 pin: Ollama capabilities come from /api/show, not the model name.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestOllamaShowCapabilities(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{{"name": "llava:latest"}, {"name": "plain:latest"}},
		})
	})
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		caps := []string{"completion"}
		if body.Name == "llava:latest" {
			caps = []string{"completion", "vision", "tools"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": caps})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderOllama]
	pc.BaseURL = srv.URL
	pc.Enabled = true
	snap.Providers[llm.ProviderOllama] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}

	models, err := llm.NewClient(s).FetchModels(context.Background(), llm.ProviderOllama)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	byID := map[string]llm.ModelInfo{}
	for _, m := range models {
		byID[m.ID] = m
	}
	llava, ok := byID["llava:latest"]
	if !ok {
		t.Fatalf("llava missing: %+v", models)
	}
	if !llava.Capabilities.Vision || !llava.Capabilities.Tools {
		t.Fatalf("declared capabilities not applied: %+v", llava.Capabilities)
	}
	plain := byID["plain:latest"]
	if plain.Capabilities.Vision || plain.Capabilities.Tools {
		t.Fatalf("plain model must claim only what /api/show says: %+v", plain.Capabilities)
	}
}
