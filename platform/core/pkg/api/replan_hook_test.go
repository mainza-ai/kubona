package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
)

func replanHookTestServer(t *testing.T, dbPath string) (*Server, *ledger.Ledger, func()) {
	t.Helper()
	l, err := ledger.OpenLedger(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, nil, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)
	cleanup := func() {
		l.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + ".lock")
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return srv, l, cleanup
}

// A settled job with no campaign behind the session replans nothing —
// and, critically, writes no version.
func TestReplanHookSkipsCampaignless(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_replan_nocamp.db")
	defer cleanup()

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-nocamp", SessionID: "ghost-session", JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":true,"meyer_wallach_entanglement":0.9}`),
	})

	versions, err := srv.ledger.GetCampaignVersions("ghost-session", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Errorf("campaignless session must gain no versions, got %d", len(versions))
	}
}

// A settled measurement folds into the campaign and lands as a labeled
// version — the plan visibly learns the run happened.
func TestReplanHookFoldsMeasurement(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_replan_fold.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("replan drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-foldme123", SessionID: sess.ID, JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":true,"resolved_backend":"local_simulator","meyer_wallach_entanglement":0.71,"counts":{"00":100}}`),
	})

	versions, err := l.GetCampaignVersions(sess.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected session-create + replan versions, got %d", len(versions))
	}
	if !strings.HasPrefix(versions[1].Trigger, "replan:circuit_run:") {
		t.Errorf("replan version mislabeled: %q", versions[1].Trigger)
	}
	var folded agents.CampaignState
	if err := json.Unmarshal([]byte(versions[1].StateJSON), &folded); err != nil {
		t.Fatal(err)
	}
	if folded.QuantumFactors.MeyerWallachQ != 0.71 {
		t.Errorf("measured MW not in saved version: %v", folded.QuantumFactors.MeyerWallachQ)
	}

	// A fold with nothing honest to change saves nothing.
	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-noop", SessionID: sess.ID, JobType: "circuit_run",
		ResultJSON: json.RawMessage(`{"success":false}`),
	})
	versions, _ = l.GetCampaignVersions(sess.ID, 10)
	if len(versions) != 2 {
		t.Errorf("no-change fold must not write a version, got %d", len(versions))
	}
}

// A settled claim_eval mints a citable record and the replan version cites
// it — verdict, evidence, and plan change all linked by the record ID.
func TestReplanHookMintsClaimRecord(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_replan_claim.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("claim drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-eval123", SessionID: sess.ID, JobType: "claim_eval",
		ResultJSON: json.RawMessage(`{"claim_id":"claim-h2-ground","corroborated_amplitude":0.97,"contested_amplitude":0.02,"unverified_amplitude":0.05,"passes_chemical_accuracy":true,"delta_mha":0.4,"verdict_reason":"within 0.4 mHa"}`),
	})

	records, err := l.ListClaimRecords(sess.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 claim record, got %d", len(records))
	}
	rec := records[0]
	if rec.TurnKind != "claim_eval" || rec.Verdict != "verified" || rec.Alpha != 0.97 {
		t.Errorf("record wrong: %+v", rec)
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(rec.Evidence), &ev); err != nil {
		t.Fatal(err)
	}
	if ev["job_id"] != "job-eval123" || ev["external_claim_id"] != "claim-h2-ground" {
		t.Errorf("evidence must link job and external claim: %v", ev)
	}

	versions, _ := l.GetCampaignVersions(sess.ID, 10)
	if len(versions) != 2 {
		t.Fatalf("expected session-create + replan, got %d", len(versions))
	}
	var folded agents.CampaignState
	if err := json.Unmarshal([]byte(versions[1].StateJSON), &folded); err != nil {
		t.Fatal(err)
	}
	last := folded.Notebook.Sections[len(folded.Notebook.Sections)-1]
	if !strings.Contains(last, "["+rec.ID+"]") {
		t.Errorf("version must cite the minted record %q, got %q", rec.ID, last)
	}
	if folded.Epistemics.Status != "CONTESTED" {
		t.Errorf("pass on an uncomputed anchor must withhold collapse: %s", folded.Epistemics.Status)
	}
	if !strings.Contains(folded.Epistemics.Statement, "UNCOMPUTED") {
		t.Errorf("withholding must name the cause: %q", folded.Epistemics.Statement)
	}
}

// The claims endpoint resolves every cited ID: records listed newest
// first, unknown sessions resolve empty.
func TestClaimsEndpointResolves(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_claims_endpoint.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("claims endpoint drill", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.MintClaimRecord(sess.ID, "tribunal", "Tribunal consensus: go", -1, -1, -1, "verified", `{}`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sess.ID+"/claims?limit=10", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out struct {
		Claims []ledger.ClaimRecord `json:"claims"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Claims) != 1 || out.Claims[0].TurnKind != "tribunal" || out.Claims[0].Alpha != -1 {
		t.Errorf("claims endpoint wrong: %+v", out.Claims)
	}

	missReq := httptest.NewRequest(http.MethodGet, "/api/sessions/ghost/claims", nil)
	missW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missW, missReq)
	if missW.Code != http.StatusOK {
		t.Fatalf("unknown session must resolve empty with 200, got %d", missW.Code)
	}
}

// The evaluation record exists independent of any campaign: a verdict on
// a campaignless session still mints, it just folds nowhere.
func TestClaimEvalMintsWithoutCampaign(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_replan_claim_nocamp.db")
	defer cleanup()

	srv.replanCampaignOnSettled(&jobs.Job{
		ID: "job-evalnc", SessionID: "bare-session", JobType: "claim_eval",
		ResultJSON: json.RawMessage(`{"claim_id":"c","corroborated_amplitude":0.5,"contested_amplitude":0.3,"unverified_amplitude":0.2,"passes_chemical_accuracy":false,"verdict_reason":"off"}`),
	})

	records, err := l.ListClaimRecords("bare-session", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Verdict != "contested" {
		t.Errorf("evaluation must record despite no campaign: %+v", records)
	}
	versions, _ := l.GetCampaignVersions("bare-session", 10)
	if len(versions) != 0 {
		t.Errorf("no campaign means no versions, got %d", len(versions))
	}
}

// A proposal has no notebook. The endpoint must not manufacture one from
// H2 defaults: an empty list is the honest answer until a job folds.
func TestNotebooksEmptyWithoutMeasurement(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_notebooks_empty.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("no notebook yet", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/results/notebooks?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	srv.handleNotebooks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notebooks must answer 200, got %d", rec.Code)
	}
	var body struct {
		Notebooks []any `json:"notebooks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notebooks) != 0 {
		t.Fatalf("proposal must not yield a notebook, got %d", len(body.Notebooks))
	}
	for _, forbidden := range []string{"H2 Molecular", "1.13727", "1.11668", "0.00047"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Errorf("empty notebook response must not contain %q: %s", forbidden, rec.Body.String())
		}
	}
}

// A folded job creates the notebook; the endpoint serves exactly that
// measured notebook, not a template.
func TestNotebooksServesFoldedMeasurement(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_notebooks_folded.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("folded notebook", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	if err := l.SaveCampaignStateWithTrigger(sess.ID, stateBytes, "session-create"); err != nil {
		t.Fatal(err)
	}
	anchored, anchorChanged, _ := agents.ReplanWithJobResult(stateBytes, "job-anchor1", "classical_anchor",
		[]byte(`{"success":true,"molecule":"H2","bond_distance":0.7414,"rhf_energy":-1.11668,"fci_exact_energy":-1.13727}`), "")
	if !anchorChanged {
		t.Fatal("computed anchor must fold")
	}
	updated, changed, _ := agents.ReplanWithJobResult(anchored, "job-measured1", "circuit_run",
		[]byte(`{"success":true,"resolved_backend":"local_simulator","meyer_wallach_entanglement":0.71,"counts":{"00":100}}`), "")
	if !changed {
		t.Fatal("measured circuit must fold")
	}
	if err := l.SaveCampaignStateWithTrigger(sess.ID, updated, "replan:circuit_run:job-meas"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/results/notebooks?session_id="+sess.ID, nil)
	rec := httptest.NewRecorder()
	srv.handleNotebooks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notebooks must answer 200, got %d", rec.Code)
	}
	var body struct {
		Notebooks []struct {
			Title          string   `json:"title"`
			ProvenanceHash string   `json:"provenance_hash"`
			Sections       []string `json:"sections"`
		} `json:"notebooks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notebooks) != 1 {
		t.Fatalf("expected 1 folded notebook, got %d", len(body.Notebooks))
	}
	nb := body.Notebooks[0]
	if nb.ProvenanceHash == "" {
		t.Error("folded notebook must carry provenance")
	}
	joined := strings.Join(nb.Sections, "\n")
	if !strings.Contains(joined, "classical_anchor") || !strings.Contains(joined, "circuit_run") {
		t.Errorf("folded notebook must record the jobs: %+v", nb.Sections)
	}
	if strings.Contains(rec.Body.String(), "0.00047") {
		t.Error("folded notebook must not contain the old template VQE corridor")
	}
}

// K4: the global cloud-LLM dollar cap refuses new metered entries once
// spent; track-only mode (cap unset) never refuses. Local deployments
// never trip it (checked via active provider, not per call).
func TestLLMBudgetCapRefusesWhenSpent(t *testing.T) {
	// Metered-provider budget test: pin the active provider explicitly
	// (the code default is local/unmetered Strata).
	t.Setenv("KUBONA_LLM_PROVIDER", "opencode")
	t.Setenv("OPENCODE_API_KEY", "test-key")
	t.Setenv("OPENCODE_MODEL", "deepseek-v4.1-flash")
	t.Setenv("KUBONA_API_TOKEN", "budget-test-token")
	srv, l, cleanup := replanHookTestServer(t, "test_llm_budget.db")
	defer cleanup()

	seedSpend := func(usd float64) {
		t.Helper()
		if err := l.AppendEvent("global", "llm_spend_estimated", map[string]any{
			"provider": "opencode", "model": "deepseek-v4.1-flash",
			"prompt_tokens": 1000, "completion_tokens": 500,
			"estimated_usd": usd, "heuristic": true, "usage_reported": true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Track-only (no cap): allowed regardless of recorded spend.
	seedSpend(999.0)
	t.Setenv("KUBONA_LLM_BUDGET_USD", "")
	if err := srv.checkLLMBudget(); err != nil {
		t.Errorf("track-only must never refuse, got %v", err)
	}

	// Cap above spend: allowed.
	t.Setenv("KUBONA_LLM_BUDGET_USD", "1000")
	if err := srv.checkLLMBudget(); err != nil {
		t.Errorf("under-cap must allow, got %v", err)
	}

	// Cap below spend: refused with the cause named.
	t.Setenv("KUBONA_LLM_BUDGET_USD", "10")
	if err := srv.checkLLMBudget(); err == nil {
		t.Error("over-cap must refuse")
	} else if !strings.Contains(err.Error(), "KUBONA_LLM_BUDGET_USD") {
		t.Errorf("refusal must name the control, got %v", err)
	}

	// HTTP entry refuses fast (429) without touching any LLM.
	req := httptest.NewRequest(http.MethodPost, "/api/chat/tribunal",
		bytes.NewReader([]byte(`{"prompt":"H2","session_id":"s"}`)))
	req.Header.Set("Authorization", "Bearer budget-test-token")
	w := httptest.NewRecorder()
	srv.handleChatTribunal(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("capped tribunal must 429 before any LLM call, got %d", w.Code)
	}
	if got := srv.llmSpendTotal(); got != 999.0 {
		t.Errorf("spend total must sum recorded estimates, got %v", got)
	}
}

// A fresh campaign is a proposal, not a result. The stored state must carry
// no trajectory, notebook, epistemics, VQE energy, canary fidelity, or cut
// overhead before any real job/claim folds.
func TestFreshCampaignIsProposalOnly(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_fresh_proposal.db")
	defer cleanup()

	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions",
		strings.NewReader(`{"title":"proposal drill","target_molecule":"H2"}`))
	createRec := httptest.NewRecorder()
	srv.handleSessions(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", createRec.Code, createRec.Body.String())
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &sess); err != nil || sess.ID == "" {
		t.Fatalf("create decode: %v %s", err, createRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sess.ID+"/campaign", nil)
	getRec := httptest.NewRecorder()
	srv.handleSessionByID(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("campaign: %d %s", getRec.Code, getRec.Body.String())
	}
	var cs agents.CampaignState
	if err := json.Unmarshal(getRec.Body.Bytes(), &cs); err != nil {
		t.Fatal(err)
	}
	if cs.Notebook != nil || cs.Epistemics != nil {
		t.Fatalf("proposal carried measurement objects: %+v", cs)
	}
	if len(cs.Trajectory) != 0 || len(cs.Branches) != 0 {
		t.Fatalf("proposal carried trajectory/branches: %+v", cs)
	}
	if cs.ClassicalAnchor.VqeExpectationEnergy != 0 || cs.ClassicalAnchor.ChemicalAccuracyPassed {
		t.Fatalf("proposal carried an energy/verdict: %+v", cs.ClassicalAnchor)
	}
	if cs.QuantumFactors.CliffordFidelity != 0 {
		t.Fatalf("proposal carried canary fidelity: %+v", cs.QuantumFactors)
	}
	if len(cs.QuantumFactors.WireCuts) != 0 {
		t.Fatalf("proposal carried cut overhead: %+v", cs.QuantumFactors.WireCuts)
	}
	for _, forbidden := range []string{"vqe_expectation_energy", "clifford_fidelity", "chemical_accuracy_passed", "\"notebook\":{", "\"epistemics\":{"} {
		if strings.Contains(getRec.Body.String(), forbidden) {
			t.Errorf("proposal response must not contain %q: %s", forbidden, getRec.Body.String())
		}
	}

	epiReq := httptest.NewRequest(http.MethodGet, "/api/epistemics?session_id="+sess.ID, nil)
	epiRec := httptest.NewRecorder()
	srv.handleEpistemics(epiRec, epiReq)
	if epiRec.Code != http.StatusOK {
		t.Fatalf("epistemics: %d %s", epiRec.Code, epiRec.Body.String())
	}
	var epi struct {
		Active []any `json:"active_wavefunctions"`
	}
	if err := json.Unmarshal(epiRec.Body.Bytes(), &epi); err != nil {
		t.Fatal(err)
	}
	if len(epi.Active) != 0 {
		t.Fatalf("proposal must not expose an active claim: %s", epiRec.Body.String())
	}
	for _, forbidden := range []string{"0.95", "1.13727", "0.7414"} {
		if strings.Contains(epiRec.Body.String(), forbidden) {
			t.Errorf("empty epistemics must not contain fallback %q: %s", forbidden, epiRec.Body.String())
		}
	}
}

// An explicit synthesize request may refresh a proposal, but it must never
// overwrite a campaign that already carries measured evidence.
func TestCampaignSynthesizeRefusesToOverwriteMeasurement(t *testing.T) {
	srv, l, cleanup := replanHookTestServer(t, "test_synth_overwrite.db")
	defer cleanup()

	sess, err := srv.sessionMgr.CreateSession("measured campaign", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	state, err := agents.BuildCampaignProposal(context.Background(), sess.ID, "H2 ground state VQE", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.Marshal(state)
	updated, changed, _ := agents.ReplanWithJobResult(stateBytes, "job-measured", "circuit_run",
		[]byte(`{"success":true,"resolved_backend":"local_simulator","meyer_wallach_entanglement":0.71,"counts":{"00":100}}`), "")
	if !changed {
		t.Fatal("measurement must fold")
	}
	if err := l.SaveCampaignStateWithTrigger(sess.ID, updated, "replan:circuit_run:job-meas"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/campaign/synthesize",
		strings.NewReader(`{"prompt":"a different target"}`))
	rec := httptest.NewRecorder()
	srv.handleSessionByID(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("measured campaign must refuse overwrite, got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "measured evidence") {
		t.Errorf("refusal must name measured evidence: %s", rec.Body.String())
	}
}
