package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestScienceMetricsReport(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	// Cost evidence: a settled job with a measured actual cost, cited by a
	// verified claim.
	if _, err := srv.Ledger().DB().Exec(`INSERT INTO jobs
		(id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_amount_micro_usd)
		VALUES ('cost-job', ?, 'JOB_STATUS_COMPLETED', 'vqe_run', '{}', '{"actual_usd":0.25}',
		?, ?, 500000)`, sid, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("job row: %v", err)
	}
	if _, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "H2 measured", 0.9, 0.05, 0.05,
		"verified", `{"job_id":"cost-job"}`); err != nil {
		t.Fatalf("claim: %v", err)
	}

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("hypotheses = %d: %s", w.Code, w.Body.String())
	}

	// One approved plan (its created/approved events feed the latency metric).
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "metric probe", "method": "vqe", "observable": "energy",
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
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}

	// One signed manuscript.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts", "", map[string]any{})
	if w.Code != http.StatusCreated {
		t.Fatalf("manuscript = %d: %s", w.Code, w.Body.String())
	}
	var ms struct {
		Manuscript struct {
			ID          string `json:"id"`
			ContentHash string `json:"content_hash"`
		} `json:"manuscript"`
	}
	json.Unmarshal(w.Body.Bytes(), &ms)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/manuscripts/"+ms.Manuscript.ID+"/signoff", "op-token",
		map[string]any{"content_hash": ms.Manuscript.ContentHash})
	if w.Code != http.StatusOK {
		t.Fatalf("signoff = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodGet, "/api/ops/science", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("science metrics = %d: %s", w.Code, w.Body.String())
	}
	var report struct {
		Stages []struct {
			Stage   string `json:"stage"`
			Outcome string `json:"outcome"`
			Count   int64  `json:"count"`
		} `json:"stages"`
		Cost struct {
			VerifiedClaims  int64    `json:"verified_claims"`
			ClaimsWithJob   int64    `json:"claims_with_job"`
			TotalCostUSD    float64  `json:"total_cost_usd"`
			CostPerClaimUSD *float64 `json:"cost_per_claim_usd"`
			Basis           string   `json:"basis"`
		} `json:"cost"`
		Human struct {
			PlanApprovals      int64    `json:"plan_approvals"`
			PlanRejections     int64    `json:"plan_rejections"`
			PendingApprovals   int64    `json:"pending_approvals"`
			DeferredApprovals  int64    `json:"deferred_approvals"`
			Signoffs           int64    `json:"signoffs"`
			Total              int64    `json:"total"`
			HumanMinutes       *float64 `json:"human_minutes"`
			HumanMinutesStatus string   `json:"human_minutes_status"`
			ApprovalLatencyP50 *float64 `json:"approval_latency_p50_minutes"`
			ApprovalLatencyP90 *float64 `json:"approval_latency_p90_minutes"`
			PlanRejectionRate  *float64 `json:"plan_rejection_rate"`
		} `json:"human"`
		Queue struct {
			JobsByStatus map[string]int64 `json:"jobs_by_status"`
			LiveJobs     int64            `json:"live_jobs"`
		} `json:"queue"`
		Disclosure string `json:"disclosure"`
	}
	json.Unmarshal(w.Body.Bytes(), &report)
	stageCount := func(stage, outcome string) int64 {
		for _, s := range report.Stages {
			if s.Stage == stage && s.Outcome == outcome {
				return s.Count
			}
		}
		return 0
	}
	if stageCount("hypothesize", "open") < 5 || stageCount("plan", "pending") < 1 ||
		stageCount("execute", "success") < 1 || stageCount("signoff", "success") < 1 {
		t.Fatalf("stage taxonomy = %+v", report.Stages)
	}
	if report.Cost.VerifiedClaims != 1 || report.Cost.ClaimsWithJob != 1 ||
		report.Cost.TotalCostUSD != 0.25 || report.Cost.CostPerClaimUSD == nil ||
		*report.Cost.CostPerClaimUSD != 0.25 {
		t.Fatalf("cost metric = %+v", report.Cost)
	}
	if !strings.Contains(report.Cost.Basis, "actual_usd") {
		t.Fatalf("cost basis = %q", report.Cost.Basis)
	}
	if report.Human.PlanApprovals < 1 || report.Human.Signoffs < 1 || report.Human.Total < 2 {
		t.Fatalf("human metrics = %+v", report.Human)
	}
	if report.Human.HumanMinutes != nil || !strings.Contains(report.Human.HumanMinutesStatus, "unmeasured") {
		t.Fatalf("human minutes must stay unmeasured: %+v", report.Human)
	}
	if report.Human.PendingApprovals != 0 || report.Human.DeferredApprovals != 0 {
		t.Fatalf("queue metrics = %+v", report.Human)
	}
	if report.Human.PlanRejectionRate == nil || *report.Human.PlanRejectionRate != 0 {
		t.Fatalf("rejection rate = %+v", report.Human.PlanRejectionRate)
	}
	if report.Human.ApprovalLatencyP50 != nil && *report.Human.ApprovalLatencyP50 < 0 {
		t.Fatalf("negative latency: %+v", report.Human)
	}
	if report.Queue.JobsByStatus["JOB_STATUS_COMPLETED"] < 1 {
		t.Fatalf("queue health = %+v", report.Queue)
	}
	if report.Disclosure == "" {
		t.Fatal("missing disclosure")
	}
}

func TestScienceMetricsRequireBearerAndRenderPrometheus(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	w := doJSON(t, srv, http.MethodGet, "/api/ops/science", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous science metrics = %d", w.Code)
	}
	_ = sid

	w = doJSON(t, srv, http.MethodGet, "/metrics", "op-token", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics = %d: %s", w.Code, w.Body.String())
	}
	for _, want := range []string{
		"# HELP kubona_science_stage", "kubona_verified_claims",
		"kubona_human_interventions{", "kubona_live_jobs", "kubona_active_loops",
		"kubona_pending_approvals", "kubona_deferred_approvals",
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("metrics missing %q; body:\n%s", want, w.Body.String())
		}
	}
}
