package llm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// W2.20: provider bodies past the cap refuse instead of buffering forever.
func TestW220BoundedResponseRefuses(t *testing.T) {
	big := strings.Repeat("x", 9*1024*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, big)
	}))
	defer srv.Close()

	t.Setenv("OPENCODE_API_KEY", "test-key")
	t.Setenv("OPENCODE_MODEL", "deepseek-v4.1-flash")
	t.Setenv("KUBONA_LLM_PROVIDER", "opencode")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	// Oversize-body test runs against the cloud stub: opt out of the
	// air-gap default like an operator would.
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderOpenCode]
	pc.BaseURL = srv.URL
	snap.Providers[llm.ProviderOpenCode] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	c := llm.NewClient(s)
	_, _, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("oversize body must refuse with cap error, got %v", err)
	}
}
