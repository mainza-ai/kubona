package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestWireAnthropicMessagesUnaryToolAndStream(t *testing.T) {
	var seenHeaders []http.Header
	var seenBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		seenHeaders = append(seenHeaders, r.Header.Clone())
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		seenBodies = append(seenBodies, body)

		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"cache_read_input_tokens\":40,\"cache_creation_input_tokens\":5}}}\n\n" +
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"deliberating \"}}\n\n" +
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig_anth_1\"}}\n\n" +
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n" +
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Anthropic\"}}\n\n" +
					"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":12}}\n\n" +
					"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_1",
			"model":       "claude-sonnet-4-6",
			"stop_reason": "tool_use",
			"content": []any{
				map[string]any{"type": "thinking", "thinking": "checked geometry", "signature": "sig_123"},
				map[string]any{"type": "tool_use", "id": "tu_1", "name": "get_anchor", "input": map[string]any{"molecule": "LiH"}},
			},
			"usage": map[string]any{
				"input_tokens":                100,
				"output_tokens":               25,
				"cache_read_input_tokens":     80,
				"cache_creation_input_tokens": 20,
			},
		})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("ANTHROPIC_MODEL", "claude-sonnet-4-6")
	t.Setenv("KUBONA_LLM_PROVIDER", "anthropic")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderAnthropic]
	pc.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderAnthropic] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	var spent []llm.SpendRecord
	c.OnSpend = func(rec llm.SpendRecord) { spent = append(spent, rec) }

	longSys := strings.Repeat("You are an exact quantum scientist. ", 12)
	_, calls, resp, _, err := c.CompleteWithTools(context.Background(), "copilot", []llm.ChatMessage{
		{Role: "system", Content: longSys},
		{Role: "user", Content: "Anchor for LiH?"},
	}, []map[string]any{{
		"type": "function",
		"function": map[string]any{
			"name":        "get_anchor",
			"description": "Fetch classical baseline",
			"parameters":  map[string]any{"type": "object"},
		},
	}})
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "tu_1" || calls[0].Name != "get_anchor" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
	if resp.Usage.PromptTokens != 200 || resp.Usage.CachedInputTokens != 80 || resp.Usage.CacheWriteTokens != 20 {
		t.Fatalf("unexpected Anthropic cached usage aggregation: %+v", resp.Usage)
	}
	if resp.Choices[0].Message.ReasoningSignature != "sig_123" {
		t.Fatalf("expected thinking signature preserved, got %q", resp.Choices[0].Message.ReasoningSignature)
	}
	if seenHeaders[0].Get("x-api-key") != "test-anthropic-key" || seenHeaders[0].Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("missing Anthropic auth/version headers: %+v", seenHeaders[0])
	}
	sysBlocks, ok := seenBodies[0]["system"].([]any)
	if !ok || len(sysBlocks) == 0 {
		t.Fatalf("expected system block array with cache_control, got %#v", seenBodies[0]["system"])
	}
	firstSys := sysBlocks[0].(map[string]any)
	if firstSys["cache_control"] == nil {
		t.Fatalf("expected cache_control breakpoint on system block, got %#v", firstSys)
	}

	var tokens strings.Builder
	sres, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{
		{Role: "user", Content: "Say hi"},
	}, func(tok string) { tokens.WriteString(tok) })
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if sres.Text != "Hello Anthropic" || tokens.String() != "Hello Anthropic" || sres.Finish != "stop" {
		t.Fatalf("unexpected stream result: text=%q tokens=%q finish=%q", sres.Text, tokens.String(), sres.Finish)
	}
	if len(spent) != 2 || spent[1].PromptTokens != 55 || spent[1].CompletionTokens != 12 {
		t.Fatalf("unexpected stream spend records: %+v", spent)
	}
}

func TestWireOpenAIResponsesUnaryAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["instructions"] == nil {
			t.Fatalf("expected instructions field in OpenAI Responses request, got %#v", body)
		}
		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"OpenAI \"}\n\n" +
					"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Responses\"}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5-mini\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OpenAI Responses\"}]}],\"usage\":{\"input_tokens\":30,\"output_tokens\":9,\"total_tokens\":39}}}\n\n",
			))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "resp_1",
			"model": "gpt-5-mini",
			"output": []any{
				map[string]any{"type": "reasoning", "encrypted_content": "enc_r_1", "summary": []any{map[string]any{"type": "summary_text", "text": "plan"}}},
				map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "Unary OK"}}},
			},
			"usage": map[string]any{
				"input_tokens":         50,
				"output_tokens":        15,
				"total_tokens":         65,
				"input_tokens_details": map[string]any{"cached_tokens": 20},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-oa-test")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")
	t.Setenv("KUBONA_LLM_PROVIDER", "openai")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderOpenAI]
	pc.BaseURL = srv.URL + "/v1"
	pc.Wire = llm.WireOpenAIResponses
	snap.Providers[llm.ProviderOpenAI] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	txt, resp, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{
		{Role: "system", Content: "Be brief."},
		{Role: "user", Content: "ping"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if txt != "Unary OK" || resp.Choices[0].Message.EncryptedReasoning != "enc_r_1" || resp.Usage.CachedInputTokens != 20 {
		t.Fatalf("unexpected Responses unary outcome: txt=%q msg=%+v usage=%+v", txt, resp.Choices[0].Message, resp.Usage)
	}

	sres, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{
		{Role: "system", Content: "Be brief."},
		{Role: "user", Content: "stream"},
	}, nil)
	if err != nil {
		t.Fatalf("CompleteStream: %v", err)
	}
	if sres.Text != "OpenAI Responses" {
		t.Fatalf("unexpected Responses stream text: %q", sres.Text)
	}
}

func TestWireGeminiHeaderAuthToolSignatureAndStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "key=") {
			t.Fatalf("API key must NEVER travel in query string, got %s", r.URL.String())
		}
		if r.Header.Get("x-goog-api-key") != "gem-secret" {
			t.Fatalf("expected x-goog-api-key header, got %q", r.Header.Get("x-goog-api-key"))
		}
		if strings.Contains(r.URL.Path, ":streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Gem \",\"thought\":true},{\"text\":\"Result\"}]}}]}\n\n" +
					"data: {\"candidates\":[{\"finishReason\":\"STOP\",\"content\":{\"parts\":[]}}],\"usageMetadata\":{\"promptTokenCount\":22,\"candidatesTokenCount\":7,\"totalTokenCount\":29}}\n\n",
			))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"modelVersion": "gemini-2.5-pro",
			"candidates": []any{map[string]any{
				"finishReason": "STOP",
				"content": map[string]any{
					"parts": []any{
						map[string]any{
							"thoughtSignature": "tsig_abc",
							"functionCall":     map[string]any{"id": "gc_1", "name": "lookup_molecule", "args": map[string]any{"formula": "H2O"}},
						},
					},
				},
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 40, "candidatesTokenCount": 12, "totalTokenCount": 52, "cachedContentTokenCount": 10},
		})
	}))
	defer srv.Close()

	t.Setenv("GEMINI_API_KEY", "gem-secret")
	t.Setenv("GEMINI_MODEL", "gemini-2.5-pro")
	t.Setenv("KUBONA_LLM_PROVIDER", "google")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderGoogle]
	pc.BaseURL = srv.URL + "/v1beta"
	snap.Providers[llm.ProviderGoogle] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	_, calls, resp, _, err := c.CompleteWithTools(context.Background(), "copilot", []llm.ChatMessage{
		{Role: "user", Content: "Lookup H2O"},
	}, []map[string]any{{"name": "lookup_molecule", "parameters": map[string]any{"type": "object"}}})
	if err != nil {
		t.Fatalf("Gemini CompleteWithTools: %v", err)
	}
	if len(calls) != 1 || calls[0].ThoughtSignature != "tsig_abc" || resp.Usage.CachedInputTokens != 10 {
		t.Fatalf("expected Gemini tool call with thoughtSignature and usage, got calls=%+v usage=%+v", calls, resp.Usage)
	}

	sres, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("Gemini CompleteStream: %v", err)
	}
	if sres.Text != "Result" || sres.Finish != "stop" {
		t.Fatalf("unexpected Gemini stream: %+v", sres)
	}
}

func TestWireOllamaNativeChatAndStreamingMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte("{\"model\":\"qwen3:8b\",\"message\":{\"content\":\"Oll \"},\"done\":false}\n"))
			_, _ = w.Write([]byte("{\"model\":\"qwen3:8b\",\"message\":{\"content\":\"OK\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":10,\"eval_count\":20,\"eval_duration\":1000000000}\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":             "qwen3:8b",
			"done_reason":       "stop",
			"message":           map[string]any{"role": "assistant", "content": "Local OK", "thinking": "brief thought"},
			"prompt_eval_count": 15,
			"eval_count":        30,
			"eval_duration":     500000000,
		})
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_MODEL", "qwen3:8b")
	t.Setenv("KUBONA_LLM_PROVIDER", "ollama")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	pc := snap.Providers[llm.ProviderOllama]
	pc.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderOllama] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	txt, resp, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Ollama Complete: %v", err)
	}
	if txt != "Local OK" || resp.Usage.GenTokPerSec < 59.0 || resp.Choices[0].Message.ReasoningContent != "brief thought" {
		t.Fatalf("unexpected Ollama response: txt=%q resp=%+v", txt, resp)
	}
	sres, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "stream"}}, nil)
	if err != nil {
		t.Fatalf("Ollama CompleteStream: %v", err)
	}
	if sres.Text != "Oll OK" {
		t.Fatalf("unexpected Ollama stream text: %q", sres.Text)
	}
}

func TestTransientRetryAnd401InvalidatesStoredStanding(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("retry-after-ms", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		if n == 2 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Recovered after retry"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":5,"total_tokens":10}}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"bad key"}}`))
	}))
	defer srv.Close()

	t.Setenv("NVIDIA_API_KEY", "nv-test")
	t.Setenv("NVIDIA_MODEL", "nvidia/m1")
	t.Setenv("KUBONA_LLM_PROVIDER", "nvidia")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false
	pc := snap.Providers[llm.ProviderNVIDIA]
	pc.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderNVIDIA] = pc
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	txt, _, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil || txt != "Recovered after retry" {
		t.Fatalf("expected 429 retry recovery, got txt=%q err=%v", txt, err)
	}
	if st := s.GetPublicSnapshot().Providers[llm.ProviderNVIDIA].KeyStatus; st != llm.KeyStatusValid {
		t.Fatalf("expected valid key status after success, got %q", st)
	}

	// Next call hits 401 -> must NOT retry and must immediately mark KeyStatusInvalid.
	_, _, _, err = c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected 401 failure")
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 401 not to retry (3 attempts total), got %d", attempts.Load())
	}
	pub := s.GetPublicSnapshot().Providers[llm.ProviderNVIDIA]
	if pub.KeyStatus != llm.KeyStatusInvalid || pub.LastFault != llm.FaultWrongKey {
		t.Fatalf("expected 401 to invalidate standing with wrong_key, got %+v", pub)
	}
}
