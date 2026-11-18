package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestApprovalInboxDefersAndDecidesOnce(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	mkPlan := func(statement string) string {
		w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
			"statement": statement, "method": "vqe", "observable": "energy",
			"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
			"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("plan create = %d: %s", w.Code, w.Body.String())
		}
		var p struct {
			Plan struct {
				ID string `json:"id"`
			} `json:"plan"`
		}
		json.Unmarshal(w.Body.Bytes(), &p)
		return p.Plan.ID
	}
	first := mkPlan("approval probe one")
	second := mkPlan("approval probe two")

	w := doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/approvals", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("inbox = %d: %s", w.Code, w.Body.String())
	}
	var inbox struct {
		Pending  []any `json:"pending"`
		Deferred []any `json:"deferred"`
		Counts   struct {
			Pending  int `json:"pending"`
			Deferred int `json:"deferred"`
		} `json:"counts"`
	}
	json.Unmarshal(w.Body.Bytes(), &inbox)
	if inbox.Counts.Pending != 2 || inbox.Counts.Deferred != 0 {
		t.Fatalf("initial inbox = %+v", inbox.Counts)
	}

	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+first, "",
		map[string]any{"decision": "defer", "reason": "needs reviewer", "deferred_until": until})
	if w.Code != http.StatusOK {
		t.Fatalf("defer = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/approvals", "", nil)
	json.Unmarshal(w.Body.Bytes(), &inbox)
	if inbox.Counts.Pending != 1 || inbox.Counts.Deferred != 1 {
		t.Fatalf("deferred inbox = %+v", inbox.Counts)
	}
	plan, err := srv.Ledger().GetPlan(sid, first)
	if err != nil || plan.DeferredUntil == "" || plan.DeferReason != "needs reviewer" {
		t.Fatalf("deferral not persisted: %+v (%v)", plan, err)
	}

	// A deferred plan still cannot execute until approved.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+first+"/execute", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "plan_not_approved") {
		t.Fatalf("deferred execute = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+first, "",
		map[string]any{"decision": "approve", "reason": "reviewed"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	plan, _ = srv.Ledger().GetPlan(sid, first)
	if plan.Status != "approved" || plan.DeferredUntil != "" {
		t.Fatalf("approval did not clear deferral: %+v", plan)
	}

	// A second decision on the same plan refuses.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+first, "",
		map[string]any{"decision": "reject"})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "plan_not_pending") {
		t.Fatalf("double decision = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+second, "",
		map[string]any{"decision": "reject", "reason": "not worth the budget"})
	if w.Code != http.StatusOK {
		t.Fatalf("reject = %d: %s", w.Code, w.Body.String())
	}
	plan, _ = srv.Ledger().GetPlan(sid, second)
	if plan.Status != "rejected" {
		t.Fatalf("reject status = %s", plan.Status)
	}

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+second, "",
		map[string]any{"decision": "defer", "deferred_until": past})
	if w.Code != http.StatusConflict {
		t.Fatalf("rejected plan deferred = %d", w.Code)
	}
	third := mkPlan("approval probe three")
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+third, "",
		map[string]any{"decision": "defer", "deferred_until": past})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("past deferral = %d: %s", w.Code, w.Body.String())
	}

	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+other+"/approvals", "op-token", nil)
	if !strings.Contains(w.Body.String(), `"pending":[]`) {
		t.Fatalf("approvals leaked across sessions: %s", w.Body.String())
	}
}
