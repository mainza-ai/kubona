package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/llm"
)

func TestVerifyProviderUsesFreeMetadataOnly(t *testing.T) {
	var postCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			postCount.Add(1)
			http.Error(w, "verify must not call POST inference", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("x-api-key") == "anth-ok" && r.Header.Get("anthropic-version") != "" {
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-4-6"}]}`))
				return
			}
			if r.Header.Get("Authorization") == "Bearer or-ok" {
				_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-5","context_length":128000}]}`))
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
		case "/v1/key":
			_, _ = w.Write([]byte(`{"data":{"label":"ok"}}`))
		case "/v1beta/models":
			if r.Header.Get("x-goog-api-key") != "gem-ok" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"status":"PERMISSION_DENIED","message":"API_KEY_INVALID"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-2.5-pro","inputTokenLimit":1048576}]}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:8b"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "anth-ok")
	t.Setenv("OPENROUTER_API_KEY", "or-ok")
	t.Setenv("GEMINI_API_KEY", "gem-ok")
	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false

	anth := snap.Providers[llm.ProviderAnthropic]
	anth.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderAnthropic] = anth

	orCfg := snap.Providers[llm.ProviderOpenRouter]
	orCfg.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderOpenRouter] = orCfg

	gem := snap.Providers[llm.ProviderGoogle]
	gem.BaseURL = srv.URL + "/v1beta"
	snap.Providers[llm.ProviderGoogle] = gem

	oll := snap.Providers[llm.ProviderOllama]
	oll.BaseURL = srv.URL + "/v1"
	snap.Providers[llm.ProviderOllama] = oll

	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	c := llm.NewClient(s)
	for _, pt := range []llm.ProviderType{llm.ProviderAnthropic, llm.ProviderOpenRouter, llm.ProviderGoogle, llm.ProviderOllama} {
		res, err := c.VerifyProvider(context.Background(), pt, llm.VerifyRequest{CheckGeneration: 7})
		if err != nil {
			t.Fatalf("verify %s err: %v", pt, err)
		}
		if !res.OK || res.KeyStatus != llm.KeyStatusValid || len(res.Models) == 0 || res.CheckGeneration != 7 {
			t.Fatalf("verify %s failed: %+v", pt, res)
		}
	}
	if postCount.Load() != 0 {
		t.Fatalf("VerifyProvider sent %d non-GET requests", postCount.Load())
	}

	// Transient wrong key does not overwrite persisted valid key.
	badRes, err := c.VerifyProvider(context.Background(), llm.ProviderAnthropic, llm.VerifyRequest{
		APIKey:          "wrong-transient",
		CheckGeneration: 9,
	})
	if err != nil {
		t.Fatalf("transient verify err: %v", err)
	}
	if badRes.OK || badRes.Fault != llm.FaultWrongKey || badRes.CheckGeneration != 9 {
		t.Fatalf("expected wrong_key classification, got %+v", badRes)
	}
	if persisted := s.GetPublicSnapshot().Providers[llm.ProviderAnthropic].KeyStatus; persisted != llm.KeyStatusValid {
		t.Fatalf("transient verify must not clobber stored valid key status, got %q", persisted)
	}
}

func TestVerifyProviderRespectsAirGapAndCustomLocalRule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "custom-local"}}})
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-blocked")
	s := llm.DefaultSettings() // AirGapped: true
	snap := s.GetSnapshot()
	cust := snap.Providers[llm.ProviderCustom]
	cust.BaseURL = srv.URL + "/v1"
	cust.Enabled = true
	snap.Providers[llm.ProviderCustom] = cust
	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update custom: %v", err)
	}

	c := llm.NewClient(s)
	cloudRes, err := c.VerifyProvider(context.Background(), llm.ProviderOpenAI, llm.VerifyRequest{})
	if err != nil {
		t.Fatalf("verify airgap: %v", err)
	}
	if cloudRes.OK || cloudRes.Fault != llm.FaultAirGapBlocked {
		t.Fatalf("expected air_gap_blocked on cloud verify when air-gapped, got %+v", cloudRes)
	}

	// Local Custom slot succeeds even while AirGapped == true.
	custRes, err := c.VerifyProvider(context.Background(), llm.ProviderCustom, llm.VerifyRequest{})
	if err != nil {
		t.Fatalf("verify custom local: %v", err)
	}
	if !custRes.OK || len(custRes.Models) != 1 || custRes.Models[0].ID != "custom-local" {
		t.Fatalf("expected local custom verify to succeed under air-gap, got %+v", custRes)
	}
}

func TestFetchModelsEnrichesCapabilitiesAndOpenRouterPricing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			if r.Header.Get("Authorization") == "Bearer or-live" {
				_, _ = w.Write([]byte(`{
					"data": [
						{
							"id": "openrouter/priced-vision-reasoner",
							"context_length": 65536,
							"top_provider": {"context_length": 65536, "max_completion_tokens": 8192},
							"architecture": {"input_modalities": ["text", "image"]},
							"supported_parameters": ["tools", "reasoning", "structured_outputs"],
							"pricing": {"prompt": "0.000002", "completion": "0.000006"}
						},
						{
							"id": "openrouter/unpriced-chat",
							"context_length": 32768
						}
					]
				}`))
				return
			}
			// OpenAI / Custom / LM Studio shape
			_, _ = w.Write([]byte(`{
				"data": [
					{"id": "gpt-5.4"},
					{"id": "local-qwen-32b", "max_model_len": 32768}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := llm.DefaultSettings()
	snap := s.GetSnapshot()
	snap.AirGapped = false

	// 1. OpenRouter live pricing + capabilities + unpriced fallback.
	or := snap.Providers[llm.ProviderOpenRouter]
	or.BaseURL = srv.URL + "/v1"
	or.APIKey = "or-live"
	or.Enabled = true
	snap.Providers[llm.ProviderOpenRouter] = or

	// 2. OpenAI rate-card vendor enrichment.
	oa := snap.Providers[llm.ProviderOpenAI]
	oa.BaseURL = srv.URL + "/v1"
	oa.APIKey = "sk-oa"
	oa.Enabled = true
	snap.Providers[llm.ProviderOpenAI] = oa

	// 3. Local LM Studio free-price tagging even if model ID matches rate card.
	lms := snap.Providers[llm.ProviderLMStudio]
	lms.BaseURL = srv.URL + "/v1"
	lms.Enabled = true
	snap.Providers[llm.ProviderLMStudio] = lms

	if err := s.Update(&snap, nil); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	c := llm.NewClient(s)

	orModels, err := c.FetchModels(context.Background(), llm.ProviderOpenRouter)
	if err != nil {
		t.Fatalf("FetchModels openrouter: %v", err)
	}
	if len(orModels) != 2 {
		t.Fatalf("expected 2 openrouter models, got %d", len(orModels))
	}
	m0 := orModels[0]
	if !m0.Priced || m0.PriceSource != "openrouter_live" || m0.ContextWindow != 65536 || m0.MaxOutputTokens != 8192 {
		t.Fatalf("unexpected openrouter live model metadata: %+v", m0)
	}
	if !m0.Capabilities.Vision || !m0.Capabilities.Tools || !m0.Capabilities.Reasoning || !m0.Capabilities.StructuredOutput || !m0.Capabilities.Streaming {
		t.Fatalf("expected all capabilities parsed from OpenRouter metadata, got %+v", m0.Capabilities)
	}
	m1 := orModels[1]
	if m1.Priced || m1.PriceSource != "unpriced" || m1.ContextWindow != 32768 {
		t.Fatalf("expected unpriced OpenRouter model, got %+v", m1)
	}

	oaModels, err := c.FetchModels(context.Background(), llm.ProviderOpenAI)
	if err != nil {
		t.Fatalf("FetchModels openai: %v", err)
	}
	if !oaModels[0].Priced || oaModels[0].PriceSource != "vendor" || oaModels[0].ContextWindow != 400000 || !oaModels[0].Capabilities.Tools || !oaModels[0].Capabilities.Vision {
		t.Fatalf("expected vendor rate card enrichment for gpt-5.4, got %+v", oaModels[0])
	}

	lmsModels, err := c.FetchModels(context.Background(), llm.ProviderLMStudio)
	if err != nil {
		t.Fatalf("FetchModels lmstudio: %v", err)
	}
	if lmsModels[0].Priced || lmsModels[0].PriceSource != "local_free" || lmsModels[1].ContextWindow != 32768 || lmsModels[1].PriceSource != "local_free" {
		t.Fatalf("expected local_free models for lmstudio, got %+v", lmsModels)
	}
}
