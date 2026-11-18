package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/llm"
)

// fakeHypothesisLLM is the proposer seam: the API path is exercised without a
// provider, and the fake captures nothing but the fact that it was called.
type fakeHypothesisLLM struct {
	reply string
	err   error
	calls int
}

func (f *fakeHypothesisLLM) Complete(_ context.Context, _ string, _ []llm.ChatMessage) (string, error) {
	f.calls++
	return f.reply, f.err
}

func TestHypothesesUseLLMProposalsAndMeasureNovelty(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	fake := &fakeHypothesisLLM{reply: `{"hypotheses":[
	  {"statement":"The optimizer converges to a local minimum for H2.",
	   "rationale":"The measured point is above the anchor.",
	   "falsifier":"Three seeded restarts do not lower the energy by 0.1 mHa.",
	   "experiment_kind":"vqe_repeats","experiment_hint":"run three seeds"},
	  {"statement":"A larger active space would move the residual.",
	   "rationale":"The model may be under-specified.",
	   "falsifier":"The next active-space tier leaves the residual above 1.6 mHa.",
	   "experiment_kind":"vqe","experiment_hint":"next active-space tier"}
	]}`}
	srv.SetHypothesisCompleter(fake)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "",
		map[string]any{"count": 12, "use_llm": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Hypotheses []struct {
			ID              string   `json:"id"`
			Source          string   `json:"source"`
			Falsifier       string   `json:"falsifier"`
			NoveltyStatus   string   `json:"novelty_status"`
			NoveltyMethod   string   `json:"novelty_method"`
			NoveltyDistance *float64 `json:"novelty_distance"`
		} `json:"hypotheses"`
		LLM struct {
			Requested bool   `json:"requested"`
			Used      bool   `json:"used"`
			Count     int    `json:"count"`
			Error     string `json:"error"`
		} `json:"llm"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if fake.calls != 1 || !out.LLM.Requested || !out.LLM.Used || out.LLM.Count != 2 {
		t.Fatalf("LLM path not taken: calls=%d meta=%+v", fake.calls, out.LLM)
	}
	if out.LLM.Error != "" {
		t.Fatalf("unexpected LLM error: %s", out.LLM.Error)
	}
	llmRows := 0
	for _, h := range out.Hypotheses {
		if h.Falsifier == "" {
			t.Fatalf("candidate without falsifier: %+v", h)
		}
		if h.Source == "llm" {
			llmRows++
		}
		switch h.NoveltyStatus {
		case "measured":
			if h.NoveltyMethod == "" || h.NoveltyDistance == nil {
				t.Fatalf("measured novelty without method/distance: %+v", h)
			}
			if *h.NoveltyDistance < 0 || *h.NoveltyDistance > 1 {
				t.Fatalf("distance out of range: %+v", h)
			}
		case "unmeasured":
			if h.NoveltyMethod != "" || h.NoveltyDistance != nil {
				t.Fatalf("unmeasured novelty carries a value: %+v", h)
			}
		default:
			t.Fatalf("unknown novelty status: %+v", h)
		}
	}
	if llmRows != 2 {
		t.Fatalf("LLM candidates merged = %d, want 2: %s", llmRows, w.Body.String())
	}
}

func TestHypothesesLLMFailureFallsBackToDiagnosticsHonestly(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)
	srv.SetHypothesisCompleter(&fakeHypothesisLLM{reply: `not json at all`})

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "",
		map[string]any{"count": 12, "use_llm": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Hypotheses []struct {
			Source string `json:"source"`
		} `json:"hypotheses"`
		LLM struct {
			Used  bool   `json:"used"`
			Error string `json:"error"`
		} `json:"llm"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.LLM.Used || out.LLM.Error == "" {
		t.Fatalf("LLM failure was not surfaced: %+v", out.LLM)
	}
	if len(out.Hypotheses) < 3 {
		t.Fatalf("diagnostics did not survive the LLM failure: %d", len(out.Hypotheses))
	}
	for _, h := range out.Hypotheses {
		if h.Source == "llm" {
			t.Fatal("a rejected LLM reply leaked a candidate")
		}
	}
}

func TestHypothesisTournamentRanksAndPersistsScores(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "",
		map[string]any{"count": 12})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/tournament", "",
		map[string]any{"rounds": 3})
	if w.Code != http.StatusOK {
		t.Fatalf("tournament = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Judge   string `json:"judge"`
		Rounds  int    `json:"rounds"`
		Matches int    `json:"matches"`
		Ranking []struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
		} `json:"ranking"`
		MetaReview string `json:"meta_review"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Judge != "heuristic_v1" || out.Rounds != 3 || out.Matches == 0 {
		t.Fatalf("tournament metadata = %+v", out)
	}
	if len(out.Ranking) < 5 || !strings.Contains(out.MetaReview, "Top probes") {
		t.Fatalf("ranking/meta-review = %d/%q", len(out.Ranking), out.MetaReview)
	}
	for i, r := range out.Ranking {
		if r.Score < 0 || r.Score > 1 {
			t.Fatalf("score out of range: %+v", r)
		}
		if i > 0 && out.Ranking[i-1].Score < r.Score {
			t.Fatalf("ranking not sorted: %+v", out.Ranking)
		}
	}

	// Scores are persisted, not response-only: the list is score-ordered.
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/hypotheses", "", nil)
	var listed struct {
		Hypotheses []struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &listed)
	if len(listed.Hypotheses) == 0 || listed.Hypotheses[0].ID != out.Ranking[0].ID {
		t.Fatalf("persisted order does not match the tournament: %+v vs %+v", listed.Hypotheses, out.Ranking)
	}
}

func TestHypothesisCollapseRequiresEvidence(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var gen struct {
		Hypotheses []struct {
			ID string `json:"id"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &gen)
	id := gen.Hypotheses[0].ID

	// No evidence: refuse.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+id+"/collapse", "", map[string]any{})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "hypothesis_not_evidenced") {
		t.Fatalf("collapse without evidence = %d: %s", w.Code, w.Body.String())
	}
	// Status PATCH cannot fake a collapse.
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+id, "", map[string]any{"status": "collapsed"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PATCH collapsed = %d: %s", w.Code, w.Body.String())
	}

	// A completed, accepted plan is evidence.
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+id, "", map[string]any{"status": "shortlisted"})
	if w.Code != http.StatusOK {
		t.Fatalf("shortlist = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"hypothesis_id": id, "method": "vqe",
		"observable": "energy", "acceptance_metric": "energy_exact", "tolerance": 0.05,
		"stop_rule": "one run", "budget_usd": 1.0,
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
	// Not executed yet: still not evidence.
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+id+"/collapse", "",
		map[string]any{"plan_id": plan.Plan.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("unexecuted plan collapsed = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+plan.Plan.ID+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("execute = %d: %s", w.Code, w.Body.String())
	}
	var executed struct {
		Accepted bool `json:"accepted"`
	}
	json.Unmarshal(w.Body.Bytes(), &executed)
	if !executed.Accepted {
		t.Fatalf("plan was not accepted; collapse evidence must not be invented: %s", w.Body.String())
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+id+"/collapse", "",
		map[string]any{"plan_id": plan.Plan.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("collapse = %d: %s", w.Code, w.Body.String())
	}
	var collapsed struct {
		Status  string `json:"status"`
		ClaimID string `json:"claim_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &collapsed)
	if collapsed.Status != "collapsed" || collapsed.ClaimID == "" {
		t.Fatalf("collapse result = %+v", collapsed)
	}
	rec, err := srv.Ledger().GetClaimRecord(sid, collapsed.ClaimID)
	if err != nil || rec == nil || rec.Verdict != "verified" {
		t.Fatalf("claim record = %+v (%v)", rec, err)
	}
	if !strings.Contains(rec.Evidence, plan.Plan.ID) {
		t.Fatalf("claim evidence lost the plan: %s", rec.Evidence)
	}
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/hypotheses", "", nil)
	if !strings.Contains(w.Body.String(), `"claim_id":"`+collapsed.ClaimID+`"`) {
		t.Fatalf("collapsed hypothesis lost its claim link: %s", w.Body.String())
	}
}

func TestHypothesisCollapseLinksOnlyVerifiedClaims(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedMeasuredH2(t, srv, sid)

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	var gen struct {
		Hypotheses []struct {
			ID string `json:"id"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &gen)

	verified, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "H2 measured", 0.9, 0.05, 0.05, "verified", `{"job_id":"j1"}`)
	if err != nil {
		t.Fatalf("mint verified: %v", err)
	}
	refuted, err := srv.Ledger().MintClaimRecord(sid, "claim_eval", "H2 refuted", 0.1, 0.8, 0.1, "refuted", `{"job_id":"j2"}`)
	if err != nil {
		t.Fatalf("mint refuted: %v", err)
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+gen.Hypotheses[0].ID+"/collapse", "",
		map[string]any{"claim_id": verified.ID})
	if w.Code != http.StatusOK {
		t.Fatalf("verified claim collapse = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+gen.Hypotheses[1].ID+"/collapse", "",
		map[string]any{"claim_id": refuted.ID})
	if w.Code != http.StatusConflict {
		t.Fatalf("refuted claim collapse = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses/"+gen.Hypotheses[1].ID+"/collapse", "",
		map[string]any{"claim_id": "clm-missing-0001"})
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown claim collapse = %d: %s", w.Code, w.Body.String())
	}
}

func seedMeasuredH2(t *testing.T, srv *api.Server, sessionID string) {
	t.Helper()
	state := agents.CampaignState{
		SessionID: sessionID, TargetSystem: "H2", ResearchGoal: "H2 ground state",
		Notebook: &agents.SystematicNotebook{
			QuantumEnergy: -1.12, ClassicalEnergy: -1.137, DeltaMHa: 15.2,
		},
		Measurement: &agents.MeasurementSummary{
			Measured: true, Energy: -1.12, EnergyExact: -1.137, DeltaMHa: 15.2,
			Backend: "aer", Ansatz: "hardware-efficient layers=2", Shots: 4096, Seed: 11,
		},
	}
	raw, _ := json.Marshal(state)
	if err := srv.Ledger().SaveCampaignState(sessionID, raw); err != nil {
		t.Fatalf("campaign: %v", err)
	}
}
