package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCampaignFlightRecorderEndpoint(t *testing.T) {
	srv, cleanup := setupTestRoutesServer(t)
	defer cleanup()
	h := srv.Handler()

	// 1. Create a session
	sid := w2mkSession(t, h, "Flight Recorder Test Session")

	// 2. Mint a claim and append spend/halt events
	_, err := srv.Ledger().MintClaimRecord(sid, "disconfirmation", "Hypothesis H1 refuted by experiment", -1, -1, -1, "FALSIFIED", `{"refutation_delta": 0.42}`)
	if err != nil {
		t.Fatalf("MintClaimRecord: %v", err)
	}
	if err := srv.Ledger().AppendEvent(sid, "tool_loop_capped", map[string]any{
		"amount_usd": 150.0,
		"ceiling":    150.0,
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// 3. Query flight-recorder via /api/campaigns/{id}/flight-recorder
	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/api/campaigns/"+sid+"/flight-recorder", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("flight recorder returned %d: %s", w.Code, w.Body.String())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("flight recorder response took %v (audit gate requires <60s)", elapsed)
	}

	var resp struct {
		OK              bool    `json:"ok"`
		SessionID       string  `json:"session_id"`
		TerminalVerdict string  `json:"terminal_verdict"`
		StopReason      string  `json:"stop_reason"`
		SpentUSD        float64 `json:"spent_usd"`
		SpendLimitUSD   float64 `json:"spend_limit_usd"`
		LastClaims      []any   `json:"last_claims"`
		RecentEvents    []any   `json:"recent_events"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode flight recorder response: %v", err)
	}

	if !resp.OK || resp.SessionID != sid {
		t.Errorf("unexpected flight recorder payload: %+v", resp)
	}
	if resp.TerminalVerdict != "BUDGET_EXHAUSTED" {
		t.Errorf("want terminal verdict BUDGET_EXHAUSTED, got %q", resp.TerminalVerdict)
	}
	if len(resp.LastClaims) == 0 {
		t.Errorf("expected last claims in flight recorder payload")
	}

	// 4. Ghost session returns 404
	ghostReq := httptest.NewRequest(http.MethodGet, "/api/campaigns/sess-ghost/flight-recorder", nil)
	ghostW := httptest.NewRecorder()
	h.ServeHTTP(ghostW, ghostReq)
	if ghostW.Code != http.StatusNotFound {
		t.Errorf("ghost flight recorder must 404, got %d", ghostW.Code)
	}
}
