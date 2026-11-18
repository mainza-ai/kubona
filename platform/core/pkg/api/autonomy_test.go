package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type autonomyResponse struct {
	Policy struct {
		Stages map[string]string `json:"stages"`
		MaxAct float64           `json:"max_act_budget_usd"`
	} `json:"policy"`
	Source        string `json:"source"`
	CancelledJobs int    `json:"cancelled_jobs"`
}

func TestAutonomyPolicyControlsPlanExecution(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get autonomy = %d: %s", w.Code, w.Body.String())
	}
	var got autonomyResponse
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Policy.Stages["execute"] != "approve" || got.Source != "default" || got.Policy.MaxAct != 1 {
		t.Fatalf("default policy = %+v", got)
	}

	// Operator sets act mode.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "act"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("put autonomy = %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got.Policy.Stages["execute"] != "act" || got.Source != "session" {
		t.Fatalf("updated policy = %+v", got)
	}

	// Low risk inside the cap: execute without prior approval.
	lowRisk := map[string]any{
		"statement": "low-risk simulator probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"budget_usd": 1.0,
		"spec":       map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", lowRisk)
	if w.Code != http.StatusCreated {
		t.Fatalf("create low = %d: %s", w.Code, w.Body.String())
	}
	var plan struct {
		Plan struct {
			ID   string `json:"id"`
			Risk string `json:"risk"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &plan)
	if plan.Plan.Risk != "low" {
		t.Fatalf("registration risk = %q", plan.Plan.Risk)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("act execute = %d: %s", w.Code, w.Body.String())
	}
	var run struct {
		AutoApproved bool `json:"auto_approved"`
		Risk         struct {
			Level string `json:"level"`
		} `json:"risk"`
	}
	json.Unmarshal(w.Body.Bytes(), &run)
	if !run.AutoApproved || run.Risk.Level != "low" {
		t.Fatalf("act run = %+v", run)
	}

	// Hardware is high risk: act cannot bypass approval.
	highRisk := map[string]any{
		"statement": "hardware probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"budget_usd": 1.0,
		"spec":       map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1, "hardware": true},
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", highRisk)
	json.Unmarshal(w.Body.Bytes(), &plan)
	if plan.Plan.Risk != "high" {
		t.Fatalf("hardware registration risk = %q", plan.Plan.Risk)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "risk_approval_required") {
		t.Fatalf("high-risk act execute = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("approved high-risk execute = %d: %s", w.Code, w.Body.String())
	}

	// Over budget is high risk even on a simulator.
	overBudget := map[string]any{
		"statement": "over budget probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"budget_usd": 5.0,
		"spec":       map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", overBudget)
	json.Unmarshal(w.Body.Bytes(), &plan)
	if plan.Plan.Risk != "high" {
		t.Fatalf("over-budget registration risk = %q", plan.Plan.Risk)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("over-budget execute = %d: %s", w.Code, w.Body.String())
	}

	// Suggest blocks even an approved plan.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "suggest"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("suggest policy = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve for suggest = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "autonomy_suggest_only") {
		t.Fatalf("suggest execute = %d: %s", w.Code, w.Body.String())
	}
}

func TestAutonomyValidationAndHalt(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	w := doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "",
		map[string]any{"stages": map[string]any{"execute": "act"}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated put = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "automatic"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"teleport": "act"}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown stage = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "act", "collapse": "approve"}, "max_act_budget_usd": 2.5})
	if w.Code != http.StatusOK {
		t.Fatalf("set policy = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/halt", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("halt = %d: %s", w.Code, w.Body.String())
	}
	var halted autonomyResponse
	json.Unmarshal(w.Body.Bytes(), &halted)
	for stage, mode := range halted.Policy.Stages {
		if mode != "suggest" {
			t.Fatalf("halt left %s=%s", stage, mode)
		}
	}
	if halted.Policy.MaxAct != 2.5 {
		t.Fatalf("halt lost the act cap: %+v", halted.Policy)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy", "", nil)
	json.Unmarshal(w.Body.Bytes(), &halted)
	if halted.Source != "session" || halted.Policy.Stages["execute"] != "suggest" {
		t.Fatalf("halt not persisted: %+v", halted)
	}
}
