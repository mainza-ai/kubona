package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestToolApprovalRoutesDecideOnceAndStaySessionScoped(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	preview := `{"molecule":"H2"}`
	req, created, err := srv.Ledger().RequestToolApproval(sid, "run_vqe", "seeded-digest", preview, false)
	if err != nil || !created {
		t.Fatalf("seed tool approval: %v created=%v", err, created)
	}

	w := doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/tool-approvals", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", w.Code, w.Body.String())
	}
	var list struct {
		Pending []struct {
			ID     string `json:"id"`
			Tool   string `json:"tool"`
			Status string `json:"status"`
			Args   string `json:"args"`
		} `json:"pending"`
		Counts struct {
			Pending int `json:"pending"`
		} `json:"counts"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if list.Counts.Pending != 1 || len(list.Pending) != 1 ||
		list.Pending[0].Tool != "run_vqe" || !strings.Contains(list.Pending[0].Args, "H2") {
		t.Fatalf("pending list = %+v", list)
	}

	// Approval is a compare-and-set; an unknown id is a 404, a second
	// decision is a 409 with its own code.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/tool-approvals/nope", "",
		map[string]any{"decision": "approve"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown approval = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/tool-approvals/"+req.ID, "",
		map[string]any{"decision": "maybe"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad decision = %d", w.Code)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/tool-approvals/"+req.ID, "",
		map[string]any{"decision": "approve", "reason": "exact call reviewed"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	var decided struct {
		Approval struct {
			Status    string `json:"status"`
			Reason    string `json:"reason"`
			ExpiresAt string `json:"expires_at"`
		} `json:"approval"`
	}
	json.Unmarshal(w.Body.Bytes(), &decided)
	if decided.Approval.Status != "approved" || decided.Approval.Reason != "exact call reviewed" {
		t.Fatalf("approval = %+v", decided.Approval)
	}
	ttl, err := time.Parse(time.RFC3339, decided.Approval.ExpiresAt)
	if err != nil || !ttl.After(time.Now().UTC()) {
		t.Fatalf("grant has no future expiry: %+v (%v)", decided.Approval.ExpiresAt, err)
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/tool-approvals/"+req.ID, "",
		map[string]any{"decision": "deny"})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "tool_approval_not_pending") {
		t.Fatalf("second decision = %d: %s", w.Code, w.Body.String())
	}

	// A tool-approval id from another session is not addressable.
	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+other+"/tool-approvals/"+req.ID, "op-token",
		map[string]any{"decision": "approve"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-session approval = %d: %s", w.Code, w.Body.String())
	}

	// The approval event is sealed for the audit trail.
	events, _ := srv.Ledger().GetRecentEvents(sid, 100)
	found := false
	for _, e := range events {
		if e.EventType == "tool_approval_decided" && strings.Contains(string(e.Payload), req.ID) {
			found = true
		}
	}
	if !found {
		t.Fatal("tool_approval_decided event missing")
	}
}
