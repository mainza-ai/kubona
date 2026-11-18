package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAgenticModeGatesLoopsAndPlanExecution(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("agentic get = %d: %s", w.Code, w.Body.String())
	}
	var state struct {
		AgenticEnabled bool `json:"agentic_enabled"`
		AgenticLocked  bool `json:"agentic_locked"`
	}
	json.Unmarshal(w.Body.Bytes(), &state)
	if !state.AgenticEnabled || state.AgenticLocked {
		t.Fatalf("default agentic = %+v", state)
	}

	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &state)
	if state.AgenticEnabled {
		t.Fatalf("switch did not disable: %s", w.Body.String())
	}

	// Agent loop start is refused before any sandbox/harness work.
	w = doJSON(t, srv, http.MethodPost, "/api/autoresearch/start", "",
		map[string]any{"session_id": sid, "harness_id": "ansatz-search", "max_iterations": 1})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "agentic_disabled") {
		t.Fatalf("loop start while off = %d: %s", w.Code, w.Body.String())
	}

	// A plan can be proposed and approved, but not executed by the agent.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "agentic gate probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("plan create = %d: %s", w.Code, w.Body.String())
	}
	var plan struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &plan)
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "agentic_disabled") {
		t.Fatalf("plan execute while off = %d: %s", w.Code, w.Body.String())
	}

	// The manual experiment rail is not the agentic workflow and remains open.
	w = doJSON(t, srv, http.MethodPost, "/api/experiments", "", map[string]any{
		"session_id": sid, "kind": "vqe", "budget_usd": 1.0,
		"request_id": "manual-while-off",
		"spec":       map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("manual experiment while off = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("enable = %d: %s", w.Code, w.Body.String())
	}
	events, err := srv.Ledger().GetRecentEvents(sid, 20)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "agentic_mode_changed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("agentic_mode_changed event not sealed: %+v", events)
	}
}

func TestAgenticHaltLocksTheSwitchAndOperatorCanClearIt(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/halt", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("halt = %d: %s", w.Code, w.Body.String())
	}
	var halted struct {
		Policy struct {
			AgenticEnabled *bool `json:"agentic_enabled"`
			AgenticLocked  bool  `json:"agentic_locked"`
		} `json:"policy"`
	}
	json.Unmarshal(w.Body.Bytes(), &halted)
	if halted.Policy.AgenticEnabled == nil || *halted.Policy.AgenticEnabled || !halted.Policy.AgenticLocked {
		t.Fatalf("halt policy = %+v", halted.Policy)
	}

	// A session caller cannot undo an operator halt.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": true})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "agentic_locked") {
		t.Fatalf("unlock from session = %d: %s", w.Code, w.Body.String())
	}

	// A stage-only operator edit must preserve the switch and the lock.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "approve"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("stage-only put = %d: %s", w.Code, w.Body.String())
	}
	var preserved struct {
		Policy struct {
			AgenticEnabled *bool `json:"agentic_enabled"`
			AgenticLocked  bool  `json:"agentic_locked"`
		} `json:"policy"`
	}
	json.Unmarshal(w.Body.Bytes(), &preserved)
	if preserved.Policy.AgenticEnabled == nil || *preserved.Policy.AgenticEnabled || !preserved.Policy.AgenticLocked {
		t.Fatalf("stage-only edit cleared the switch/lock: %+v", preserved.Policy)
	}

	// The operator can clear the lock and re-enable.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "approve"}, "max_act_budget_usd": 1,
			"agentic_enabled": true, "agentic_locked": false})
	if w.Code != http.StatusOK {
		t.Fatalf("operator unlock = %d: %s", w.Code, w.Body.String())
	}
	var unlocked struct {
		Policy struct {
			AgenticEnabled *bool `json:"agentic_enabled"`
			AgenticLocked  bool  `json:"agentic_locked"`
		} `json:"policy"`
	}
	json.Unmarshal(w.Body.Bytes(), &unlocked)
	if unlocked.Policy.AgenticEnabled == nil || !*unlocked.Policy.AgenticEnabled || unlocked.Policy.AgenticLocked {
		t.Fatalf("operator unlock did not apply: %+v body=%s", unlocked.Policy, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("session toggle after unlock = %d: %s", w.Code, w.Body.String())
	}
}

func TestAgenticModeRequestValidation(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	w := doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": true})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST agentic = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/no-such-session/agentic", "", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown session = %d", w.Code)
	}
}
