package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
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

func TestApprovalEditsRevalidateAndUndoRestores(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "original protocol", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("plan create = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	pid := created.Plan.ID

	// Invalid edits refuse and leave the plan untouched.
	for name, edits := range map[string]any{
		"blank statement": map[string]any{"statement": "  "},
		"negative tol":    map[string]any{"tolerance": -1.0},
		"zero budget":     map[string]any{"budget_usd": 0},
		"non-object spec": map[string]any{"spec": []int{1, 2}},
	} {
		w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
			map[string]any{"decision": "approve", "edits": edits})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: edits = %d: %s", name, w.Code, w.Body.String())
		}
	}
	plan, _ := srv.Ledger().GetPlan(sid, pid)
	if plan.Statement != "original protocol" || plan.Status != "proposed" {
		t.Fatalf("invalid edits mutated the plan: %+v", plan)
	}

	// Edits on a rejection are refused (feedback belongs there).
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
		map[string]any{"decision": "reject", "edits": map[string]any{"tolerance": 0.01}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("reject + edits = %d: %s", w.Code, w.Body.String())
	}

	// Valid edits apply, are reclassified, and are sealed with the previous
	// values used by undo.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
		map[string]any{"decision": "approve", "reason": "tightened",
			"edits": map[string]any{"statement": "reviewed protocol", "tolerance": 0.02}})
	if w.Code != http.StatusOK {
		t.Fatalf("edited approve = %d: %s", w.Code, w.Body.String())
	}
	plan, _ = srv.Ledger().GetPlan(sid, pid)
	if plan.Statement != "reviewed protocol" || plan.Tolerance != 0.02 || plan.Status != "approved" {
		t.Fatalf("edits not applied: %+v", plan)
	}
	if plan.ReviewedAt == "" {
		t.Fatalf("review timestamp not persisted: %+v", plan)
	}
	events, _ := srv.Ledger().GetRecentEvents(sid, 100)
	var decided map[string]any
	for _, e := range events {
		if e.EventType != "plan_approval_decided" {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(e.Payload, &body)
		if body["plan_id"] == pid {
			decided = body
		}
	}
	if decided == nil {
		t.Fatal("approval decision event missing")
	}
	editedFields, _ := decided["edited_fields"].([]any)
	if len(editedFields) < 2 {
		t.Fatalf("edited fields not sealed: %+v", decided)
	}
	prev, _ := decided["previous"].(map[string]any)
	if prev["statement"] != "original protocol" {
		t.Fatalf("previous statement not sealed: %+v", prev)
	}

	// Undo restores the pre-decision protocol and clears the review state.
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	var receipts receiptList
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var approvalReceipt int64
	for _, r := range receipts.Receipts {
		if r.EventType == "plan_approval_decided" && r.Undoable {
			approvalReceipt = r.ID
		}
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(approvalReceipt, 10)+"/undo", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("undo edited approval = %d: %s", w.Code, w.Body.String())
	}
	plan, _ = srv.Ledger().GetPlan(sid, pid)
	if plan.Status != "proposed" || plan.Statement != "original protocol" ||
		plan.Tolerance != 0.05 || plan.ReviewedAt != "" {
		t.Fatalf("undo did not restore the review snapshot: %+v", plan)
	}
}

func TestApprovalRejectionCarriesDurableFeedback(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "feedback probe", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	pid := created.Plan.ID
	feedback := "Tighten the tolerance to 5 mHa and pre-register the shots."
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
		map[string]any{"decision": "reject", "feedback": feedback})
	if w.Code != http.StatusOK {
		t.Fatalf("reject = %d: %s", w.Code, w.Body.String())
	}
	plan, _ := srv.Ledger().GetPlan(sid, pid)
	if plan.Status != "rejected" || plan.ReviewFeedback != feedback || plan.ReviewedAt == "" {
		t.Fatalf("feedback not durable: %+v", plan)
	}
	if len(feedback) > 2000 {
		t.Fatal("test feedback must fit the limit")
	}

	// The rejection is visible on the plan list the agent and UI read.
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/plans", "", nil)
	if !strings.Contains(w.Body.String(), "Tighten the tolerance") {
		t.Fatalf("plan list omits rejection feedback: %s", w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
		map[string]any{"decision": "reject", "feedback": strings.Repeat("x", 2001)})
	if w.Code != http.StatusConflict {
		t.Fatalf("oversize feedback on a decided plan = %d", w.Code)
	}
}

func TestPlanRiskCategoriesPersistAndReviewReclassifies(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "study pathogen transmissibility", "method": "vqe", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05, "stop_rule": "one run",
		"spec": map[string]any{"kind": "vqe", "molecule": "H2", "bond_distance": 0.7414, "layers": 1},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("plan create = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Plan struct {
			ID             string   `json:"id"`
			Risk           string   `json:"risk"`
			RiskReasons    []string `json:"risk_reasons"`
			RiskCategories []string `json:"risk_categories"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created.Plan.Risk != "high" || len(created.Plan.RiskCategories) != 1 || created.Plan.RiskCategories[0] != "bio" {
		t.Fatalf("created risk = %+v", created.Plan)
	}
	if !strings.Contains(strings.Join(created.Plan.RiskReasons, " | "), "bio: pathogen") {
		t.Fatalf("categorical reason missing: %v", created.Plan.RiskReasons)
	}
	pid := created.Plan.ID

	// A reviewer edit that removes the hazard must clear the category too.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/approvals/"+pid, "",
		map[string]any{"decision": "approve", "edits": map[string]any{"statement": "simulate the H2 ground state"}})
	if w.Code != http.StatusOK {
		t.Fatalf("edited approve = %d: %s", w.Code, w.Body.String())
	}
	plan, _ := srv.Ledger().GetPlan(sid, pid)
	if plan.Risk != "low" || len(plan.RiskCategories) != 0 {
		t.Fatalf("edit did not reclassify: %+v", plan)
	}
	events, _ := srv.Ledger().GetRecentEvents(sid, 100)
	var previous map[string]any
	for _, e := range events {
		if e.EventType != "plan_approval_decided" {
			continue
		}
		var body map[string]any
		_ = json.Unmarshal(e.Payload, &body)
		if body["plan_id"] == pid {
			previous, _ = body["previous"].(map[string]any)
		}
	}
	if previous == nil {
		t.Fatal("approval event missing previous snapshot")
	}
	cats, _ := previous["risk_categories"].([]any)
	if len(cats) != 1 || cats[0] != "bio" {
		t.Fatalf("previous categories not sealed: %+v", previous["risk_categories"])
	}

	// Undo restores the classified plan, not just its status.
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/receipts", "", nil)
	var receipts receiptList
	json.Unmarshal(w.Body.Bytes(), &receipts)
	var rid int64
	for _, r := range receipts.Receipts {
		if r.EventType == "plan_approval_decided" && r.Undoable {
			rid = r.ID
		}
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/receipts/"+strconv.FormatInt(rid, 10)+"/undo", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("undo = %d: %s", w.Code, w.Body.String())
	}
	plan, _ = srv.Ledger().GetPlan(sid, pid)
	if plan.Risk != "high" || len(plan.RiskCategories) != 1 || plan.RiskCategories[0] != "bio" {
		t.Fatalf("undo did not restore categories: %+v", plan)
	}
}
