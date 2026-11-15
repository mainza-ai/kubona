package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestStreamFailureAfterTokensRecordsSpend(t *testing.T) {
	const partial = "partial-text"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"" + partial + "\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"upstream broke\"}}\n\n"))
	}))
	defer upstream.Close()

	t.Run("usage missing and no ceiling is unmetered", func(t *testing.T) {
		c := cloudStreamClient(t, upstream.URL, 0)
		var spent []llm.SpendRecord
		c.OnSpend = func(rec llm.SpendRecord) { spent = append(spent, rec) }
		res, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "go"}}, nil)
		if err == nil {
			t.Fatal("provider error after tokens must surface")
		}
		if res == nil || res.Text != partial {
			t.Fatalf("partial text = %#v, want %q", res, partial)
		}
		if len(spent) != 1 || !spent[0].Unmetered || spent[0].Provider != string(llm.ProviderGroq) {
			t.Fatalf("spend = %+v, want one unmetered groq record", spent)
		}
	})

	t.Run("usage missing with a ceiling bills the ceiling", func(t *testing.T) {
		c := cloudStreamClient(t, upstream.URL, 80)
		var spent []llm.SpendRecord
		c.OnSpend = func(rec llm.SpendRecord) { spent = append(spent, rec) }
		res, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "go"}}, nil)
		if err == nil || res == nil || res.Text != partial {
			t.Fatalf("result text=%v err=%v", res, err)
		}
		if len(spent) != 1 || !spent[0].CeilingApplied || spent[0].CeilingTokens != 80 {
			t.Fatalf("spend = %+v, want ceiling of 80", spent)
		}
	})
}

func cloudStreamClient(t *testing.T, base string, maxTokens int) *llm.Client {
	t.Helper()
	t.Setenv("GROQ_API_KEY", "gsk-test")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false
	snap.MaxTokens = maxTokens
	snap.ActiveProvider = llm.ProviderGroq
	snap.ActiveModel = "groq-test"
	pc := snap.Providers[llm.ProviderGroq]
	pc.BaseURL = base + "/v1"
	pc.Enabled = true
	pc.APIKey = "gsk-test"
	pc.Wire = llm.WireChatCompletions
	pc.DefaultModel = "groq-test"
	snap.Providers[llm.ProviderGroq] = pc
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{Role: "copilot", Provider: llm.ProviderGroq, Model: "groq-test"}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	return llm.NewClient(s)
}
