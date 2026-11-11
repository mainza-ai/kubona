package llm_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestRoleAssignmentValidationAndPerRoleParameters(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"c1","model":"custom-local",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
		}`))
	}))
	defer srv.Close()

	s := llm.DefaultSettings()
	bad := s.GetSnapshot()
	badTemp := 2.5
	bad.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:        "copilot",
		Provider:    llm.ProviderCustom,
		Model:       "custom-local",
		Temperature: &badTemp,
	}
	if err := s.Update(&bad, nil); err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("expected temperature range error, got %v", err)
	}

	bad = s.GetSnapshot()
	badTopP := 0.0
	bad.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:     "copilot",
		Provider: llm.ProviderCustom,
		Model:    "custom-local",
		TopP:     &badTopP,
	}
	if err := s.Update(&bad, nil); err == nil || !strings.Contains(err.Error(), "top_p") {
		t.Fatalf("expected top_p range error, got %v", err)
	}

	bad = s.GetSnapshot()
	bad.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:            "copilot",
		Provider:        llm.ProviderCustom,
		Model:           "custom-local",
		ReasoningEffort: "turbo",
	}
	if err := s.Update(&bad, nil); err == nil || !strings.Contains(err.Error(), "reasoning_effort") {
		t.Fatalf("expected reasoning_effort validation error, got %v", err)
	}

	bad = s.GetSnapshot()
	bad.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:     "copilot",
		Provider: llm.ProviderCustom,
		Model:    "custom-local",
		Fallback: []llm.ModelTarget{{Provider: "ghost", Model: "x"}},
	}
	if err := s.Update(&bad, nil); err == nil || !strings.Contains(err.Error(), "fallback[0]") {
		t.Fatalf("expected fallback provider error, got %v", err)
	}

	// Valid per-role overrides propagate into the wire payload and InvocationMeta.
	snap := s.GetSnapshot()
	cust := snap.Providers[llm.ProviderCustom]
	cust.BaseURL = srv.URL + "/v1"
	cust.Enabled = true
	snap.Providers[llm.ProviderCustom] = cust

	temp := 0.7
	topP := 0.9
	seed := int64(42)
	maxTok := 321
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:            "copilot",
		Provider:        llm.ProviderCustom,
		Model:           "custom-local",
		Temperature:     &temp,
		TopP:            &topP,
		Seed:            &seed,
		MaxTokens:       &maxTok,
		ReasoningEffort: "HIGH",
	}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("valid update refused: %v", err)
	}

	c := llm.NewClient(s)
	text, resp, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "ping"}})
	if err != nil || text != "ok" {
		t.Fatalf("Complete failed: text=%q err=%v", text, err)
	}
	if got := captured["temperature"]; got != 0.7 {
		t.Errorf("wire temperature = %v, want 0.7", got)
	}
	if got := captured["top_p"]; got != 0.9 {
		t.Errorf("wire top_p = %v, want 0.9", got)
	}
	if got := captured["seed"]; got != float64(42) {
		t.Errorf("wire seed = %v, want 42", got)
	}
	if got := captured["max_tokens"]; got != float64(321) {
		t.Errorf("wire max_tokens = %v, want 321", got)
	}
	if resp == nil || resp.Meta == nil {
		t.Fatalf("expected InvocationMeta populated on response")
	}
	if resp.Meta.Provider != "custom" || resp.Meta.ReasoningEffort != "high" || resp.Meta.AttemptCount != 1 || resp.Meta.MaxTokens != 321 {
		t.Fatalf("unexpected InvocationMeta: %+v", resp.Meta)
	}
}

func TestOrderedFailoverAndHopPolicyEnforcement(t *testing.T) {
	var primaryHits atomic.Int32
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits.Add(1)
		http.Error(w, `{"error":"upstream overloaded"}`, http.StatusServiceUnavailable)
	}))
	defer primarySrv.Close()

	var fallbackHits atomic.Int32
	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"fb-1","model":"omlx-fallback",
			"choices":[{"index":0,"message":{"role":"assistant","content":"served by fallback"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}
		}`))
	}))
	defer fallbackSrv.Close()

	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = true

	cust := snap.Providers[llm.ProviderCustom]
	cust.BaseURL = primarySrv.URL + "/v1"
	cust.Enabled = true
	snap.Providers[llm.ProviderCustom] = cust

	omlx := snap.Providers[llm.ProviderOMLX]
	omlx.BaseURL = fallbackSrv.URL + "/v1"
	omlx.Enabled = true
	snap.Providers[llm.ProviderOMLX] = omlx

	anth := snap.Providers[llm.ProviderAnthropic]
	anth.BaseURL = fallbackSrv.URL + "/v1"
	anth.APIKey = "anth-key"
	anth.Enabled = true
	snap.Providers[llm.ProviderAnthropic] = anth

	// 1. Hop 1 fails (503) -> Hop 2 (anthropic) is blocked by AirGapped -> Hop 3 (omlx) succeeds!
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:     "copilot",
		Provider: llm.ProviderCustom,
		Model:    "primary-down",
		Fallback: []llm.ModelTarget{
			{Provider: llm.ProviderAnthropic, Model: "claude-sonnet-4-6"},
			{Provider: llm.ProviderOMLX, Model: "omlx-fallback"},
		},
	}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update failover settings: %v", err)
	}
	c := llm.NewClient(s)
	txt, resp, _, err := c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("expected hop-3 fallback success, got %v", err)
	}
	if txt != "served by fallback" || resp.Meta == nil || resp.Meta.Provider != "omlx" || resp.Meta.AttemptCount != 3 {
		t.Fatalf("unexpected failover meta/text: txt=%q meta=%+v", txt, resp.Meta)
	}
	if fallbackHits.Load() != 1 {
		t.Fatalf("expected anthropic hop-2 blocked before network and omlx called once, got hits=%d", fallbackHits.Load())
	}

	// 2. If only cloud fallback follows failed hop under AirGapped=true, refuse without egress.
	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:     "copilot",
		Provider: llm.ProviderCustom,
		Model:    "primary-down",
		Fallback: []llm.ModelTarget{
			{Provider: llm.ProviderAnthropic, Model: "claude-sonnet-4-6"},
		},
	}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	beforeHits := fallbackHits.Load()
	_, _, _, err = c.Complete(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "Air-Gap") {
		t.Fatalf("expected air-gap refusal on hop-2, got %v", err)
	}
	if fallbackHits.Load() != beforeHits {
		t.Fatalf("blocked hop-2 must make zero network calls")
	}
}

func TestCompleteStreamFailoverAndPartialTokenGuard(t *testing.T) {
	// 1. Pre-token failure fails over to streaming hop 2.
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `down`, http.StatusBadGateway)
	}))
	defer s1.Close()

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello \"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"world\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer s2.Close()

	// 2. Mid-stream error after emitting tokens must NOT invoke hop 2.
	var hop2Calls atomic.Int32
	midErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"error\":{\"message\":\"stream crashed mid-flight\"}}\n\n"))
	}))
	defer midErr.Close()

	hop2Spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hop2Calls.Add(1)
		http.Error(w, "should not run", http.StatusInternalServerError)
	}))
	defer hop2Spy.Close()

	st := llm.DefaultSettings()
	snap := st.GetSnapshot()
	cust := snap.Providers[llm.ProviderCustom]
	cust.BaseURL = s1.URL + "/v1"
	cust.Enabled = true
	snap.Providers[llm.ProviderCustom] = cust

	omlx := snap.Providers[llm.ProviderOMLX]
	omlx.BaseURL = s2.URL + "/v1"
	omlx.Enabled = true
	snap.Providers[llm.ProviderOMLX] = omlx

	snap.RoleAssignments["copilot"] = llm.RoleModelAssignment{
		Role:     "copilot",
		Provider: llm.ProviderCustom,
		Model:    "down",
		Fallback: []llm.ModelTarget{{Provider: llm.ProviderOMLX, Model: "stream-ok"}},
	}
	if err := st.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	c := llm.NewClient(st)
	var toks []string
	res, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) {
		toks = append(toks, s)
	})
	if err != nil {
		t.Fatalf("expected stream failover success, got %v", err)
	}
	if strings.Join(toks, "") != "hello world" || res.Meta == nil || res.Meta.Provider != "omlx" || res.Meta.AttemptCount != 2 {
		t.Fatalf("unexpected stream failover result: toks=%v res=%+v", toks, res)
	}

	// Verify mid-stream crash after token emission never retries hop 2.
	cust.BaseURL = midErr.URL + "/v1"
	omlx.BaseURL = hop2Spy.URL + "/v1"
	snap.Providers[llm.ProviderCustom] = cust
	snap.Providers[llm.ProviderOMLX] = omlx
	if err := st.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	var partial []string
	partRes, err := c.CompleteStream(context.Background(), "copilot", []llm.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) {
		partial = append(partial, s)
	})
	if err == nil || strings.Join(partial, "") != "partial" {
		t.Fatalf("expected partial error after emitting 'partial', got partial=%v err=%v", partial, err)
	}
	if hop2Calls.Load() != 0 {
		t.Fatalf("mid-stream error after token emission must NEVER failover to hop 2, got %d calls", hop2Calls.Load())
	}
	if partRes == nil || partRes.Text != "partial" {
		t.Fatalf("expected partial StreamResult preserved, got %+v", partRes)
	}
}

func TestLoopIterationBoundUsesMaxAcrossFallbackChainAndRefusesUnpricedFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false

	omlx := snap.Providers[llm.ProviderOMLX]
	omlx.Enabled = true
	snap.Providers[llm.ProviderOMLX] = omlx

	oa := snap.Providers[llm.ProviderOpenAI]
	oa.APIKey = "sk-test"
	oa.Enabled = true
	snap.Providers[llm.ProviderOpenAI] = oa

	// Local primary + metered cloud fallback -> bound must cover cloud fallback ceiling!
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{
		Role:     "coder",
		Provider: llm.ProviderOMLX,
		Model:    "local-coder",
		Fallback: []llm.ModelTarget{{Provider: llm.ProviderOpenAI, Model: "gpt-5.4"}},
	}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	bound, _, err := llm.LoopIterationBound(s, []string{"coder"})
	if err != nil {
		t.Fatalf("LoopIterationBound failed: %v", err)
	}
	want := 0.015 * float64(llm.DefaultBudgetConfig().MaxTurnTokens) / 1000.0
	if math.Abs(bound-want) > 1e-12 {
		t.Fatalf("bound = %v, want %v (from metered fallback)", bound, want)
	}

	// Unpriced non-local fallback fails closed.
	snap.RoleAssignments["coder"] = llm.RoleModelAssignment{
		Role:     "coder",
		Provider: llm.ProviderOMLX,
		Model:    "local-coder",
		Fallback: []llm.ModelTarget{{Provider: llm.ProviderOpenAI, Model: "unpriced-frontier-9000"}},
	}
	if err := s.Update(&snap, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := llm.LoopIterationBound(s, []string{"coder"}); err == nil {
		t.Fatalf("expected fail-closed error on unpriced fallback model")
	}
}
