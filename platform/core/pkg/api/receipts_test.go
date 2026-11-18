package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type receiptList struct {
	Receipts []struct {
		ID        int64  `json:"id"`
		EventType string `json:"event_type"`
		Undoable  bool   `json:"undoable"`
		Reason    string `json:"reason"`
	} `json:"receipts"`
}

func TestReceiptsUndoModeAndPlanDecisions(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	// Mode flip undo.
	w := doJSON(t, srv, http.MethodPut, "/api/sessions/"+sid+"/agentic", "", map[string]any{"enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	var receipts receiptList
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var modeReceipt int64
	for _, r := range receipts.Receipts {
		if r.EventType == "agentic_mode_changed" && r.Undoable {
			modeReceipt = r.ID
		}
	}
	if modeReceipt == 0 {
		t.Fatalf("no undoable mode receipt: %+v", receipts.Receipts)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(modeReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("undo mode = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/agentic", "", nil)
	if !strings.Contains(w.Body.String(), `"agentic_enabled":true`) {
		t.Fatalf("mode not restored: %s", w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(modeReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "undo_not_reversible") {
		t.Fatalf("double undo = %d: %s", w.Code, w.Body.String())
	}

	// Plan approval undo.
	mk := func() string {
		w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
			"statement": "receipt probe", "method": "vqe", "observable": "energy",
			"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
			"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
		})
		var p struct {
			Plan struct {
				ID string `json:"id"`
			} `json:"plan"`
		}
		json.Unmarshal(w.Body.Bytes(), &p)
		return p.Plan.ID
	}
	pid := mk()
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "", map[string]any{"decision": "approve"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var approvalReceipt int64
	for _, r := range receipts.Receipts {
		if r.EventType == "plan_approval_decided" && r.Undoable {
			approvalReceipt = r.ID
		}
	}
	if approvalReceipt == 0 {
		t.Fatalf("no undoable approval receipt: %+v", receipts.Receipts)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(approvalReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("undo approval = %d: %s", w.Code, w.Body.String())
	}
	plan, _ := srv.Ledger().GetPlan(sid, pid)
	if plan.Status != "proposed" {
		t.Fatalf("plan status after undo = %s", plan.Status)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+pid+"/execute", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "plan_not_approved") {
		t.Fatalf("execute after undo = %d: %s", w.Code, w.Body.String())
	}

	// Deferral undo.
	pid2 := mk()
	until := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid2, "",
		map[string]any{"decision": "defer", "deferred_until": until})
	if w.Code != http.StatusOK {
		t.Fatalf("defer = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var deferReceipt int64
	for _, r := range receipts.Receipts {
		if r.EventType == "plan_deferred" && r.Undoable {
			deferReceipt = r.ID
		}
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(deferReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("undo defer = %d: %s", w.Code, w.Body.String())
	}
	plan2, _ := srv.Ledger().GetPlan(sid, pid2)
	if plan2.DeferredUntil != "" {
		t.Fatalf("deferral not cleared: %+v", plan2)
	}

	// An operator halt locks the mode undo.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/autonomy/halt", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("halt = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var lockedModeReceipt int64
	for _, r := range receipts.Receipts {
		if r.EventType == "agentic_mode_changed" {
			lockedModeReceipt = r.ID
		}
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(lockedModeReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("locked undo = %d: %s", w.Code, w.Body.String())
	}
}
