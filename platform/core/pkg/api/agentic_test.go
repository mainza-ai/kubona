package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
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

	// Tool permissions are operator-owned and preserved by the session switch.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "approve"}, "max_act_budget_usd": 1,
			"tool_permissions": map[string]any{"run_vqe": "block"}})
	if w.Code != http.StatusOK {
		t.Fatalf("tool policy = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("agentic toggle = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy", "", nil)
	if !strings.Contains(w.Body.String(), `"run_vqe":"block"`) {
		t.Fatalf("tool permission not preserved: %s", w.Body.String())
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

func TestNotifyModeExecutesAndBatchesDigest(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "notify"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("notify policy = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "notify probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("plan = %d: %s", w.Code, w.Body.String())
	}
	var plan struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &plan)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("notify execute = %d: %s", w.Code, w.Body.String())
	}
	var run struct {
		Notify       bool `json:"notify"`
		AutoApproved bool `json:"auto_approved"`
	}
	json.Unmarshal(w.Body.Bytes(), &run)
	if !run.Notify || !run.AutoApproved {
		t.Fatalf("notify response = %+v", run)
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic/digest?hours=24", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("digest = %d: %s", w.Code, w.Body.String())
	}
	var digest struct {
		Entries []struct {
			PlanID string `json:"plan_id"`
			Notify bool   `json:"notify"`
		} `json:"entries"`
		Counts struct {
			Total  int `json:"total"`
			Notify int `json:"notify"`
		} `json:"counts"`
	}
	json.Unmarshal(w.Body.Bytes(), &digest)
	if digest.Counts.Total < 1 || digest.Counts.Notify < 1 || len(digest.Entries) < 1 || !digest.Entries[0].Notify {
		t.Fatalf("digest = %+v", digest)
	}

	// High risk still needs explicit approval in notify mode.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "notify hardware probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1, "hardware": true},
	})
	json.Unmarshal(w.Body.Bytes(), &plan)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "risk_approval_required") {
		t.Fatalf("notify high risk = %d: %s", w.Code, w.Body.String())
	}
}

func TestAutonomyHistoryRollbackAndStrictRead(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	// Policy A (act), then policy B (approve).
	w := doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "act"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("policy A = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "approve"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("policy B = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy/history", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("history = %d: %s", w.Code, w.Body.String())
	}
	var hist struct {
		Entries []struct {
			ID        int64           `json:"id"`
			EventType string          `json:"event_type"`
			Payload   json.RawMessage `json:"payload"`
		} `json:"entries"`
	}
	json.Unmarshal(w.Body.Bytes(), &hist)
	var actEvent int64
	for _, e := range hist.Entries {
		if e.EventType != "autonomy_policy_updated" {
			continue
		}
		var p struct {
			Stages map[string]string `json:"stages"`
		}
		if json.Unmarshal(e.Payload, &p) == nil && p.Stages["execute"] == "act" {
			actEvent = e.ID
		}
	}
	if actEvent == 0 {
		t.Fatalf("act policy event not in history: %+v", hist.Entries)
	}

	// Session callers cannot roll back; operators can.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/rollback", "",
		map[string]any{"event_id": actEvent})
	if w.Code != http.StatusForbidden {
		t.Fatalf("session rollback = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/rollback", "op-token",
		map[string]any{"event_id": actEvent})
	if w.Code != http.StatusOK {
		t.Fatalf("rollback = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy", "", nil)
	var restored struct {
		Policy struct {
			Stages map[string]string `json:"stages"`
		} `json:"policy"`
	}
	json.Unmarshal(w.Body.Bytes(), &restored)
	if restored.Policy.Stages["execute"] != "act" {
		t.Fatalf("rollback did not restore act: %+v", restored.Policy)
	}

	// A mode-change event is not a policy snapshot.
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("disable = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/autonomy/history", "", nil)
	json.Unmarshal(w.Body.Bytes(), &hist)
	var modeEvent int64
	for _, e := range hist.Entries {
		if e.EventType == "agentic_mode_changed" {
			modeEvent = e.ID
			break
		}
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/rollback", "op-token",
		map[string]any{"event_id": modeEvent})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("mode event rollback = %d: %s", w.Code, w.Body.String())
	}

	// Strict read: a corrupted stored policy refuses instead of enabling.
	if _, err := srv.Ledger().DB().Exec(`INSERT INTO session_autonomy (session_id, policy_json, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(session_id) DO UPDATE SET policy_json = excluded.policy_json`,
		sid, "{not-json", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("corrupt policy: %v", err)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic", "", nil)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "autonomy_policy_unreadable") {
		t.Fatalf("corrupt policy read = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/autonomy", "op-token",
		map[string]any{"stages": map[string]any{"execute": "approve"}, "max_act_budget_usd": 1})
	if w.Code != http.StatusOK {
		t.Fatalf("repair = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("repaired read = %d: %s", w.Code, w.Body.String())
	}
}

func TestTenantAgenticDefaultInheritedByNewSessions(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	_ = createSession(t, srv, "other") // registers the tenant row

	w := doJSON(t, srv, http.MethodPut, "/api/tenants/agentic-default", "",
		map[string]any{"tenant_id": "other", "enabled": false})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated default = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPut, "/api/tenants/agentic-default", "op-token",
		map[string]any{"tenant_id": "other", "enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("set default = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/tenants/agentic-default?tenant_id=other", "op-token", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("get default = %d: %s", w.Code, w.Body.String())
	}

	sid := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic", "op-token", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"agentic_enabled":false`) ||
		!strings.Contains(w.Body.String(), `"source":"tenant-default"`) {
		t.Fatalf("inherit default = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "op-token", map[string]any{"enabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("session override = %d: %s", w.Code, w.Body.String())
	}
	sid2 := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid2+"/agentic", "op-token", nil)
	if !strings.Contains(w.Body.String(), `"agentic_enabled":false`) {
		t.Fatalf("second session did not inherit: %s", w.Body.String())
	}

	// An unset tenant keeps the safe default (on) for compatibility.
	def := createSession(t, srv, "default")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+def+"/agentic", "op-token", nil)
	if !strings.Contains(w.Body.String(), `"agentic_enabled":true`) {
		t.Fatalf("unset tenant default = %s", w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPut, "/api/tenants/agentic-default", "op-token",
		map[string]any{"tenant_id": "other", "enabled": true})
	if w.Code != http.StatusOK {
		t.Fatalf("clear default = %d", w.Code)
	}
}
