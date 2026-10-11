package api

import (
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
	state, err := agents.SynthesizeCampaignState(context.Background(), sess.ID, "H2 ground state VQE", nil, nil)
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
	state, err := agents.SynthesizeCampaignState(context.Background(), sess.ID, "H2 ground state VQE", nil, nil)
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
	if folded.Epistemics.Status != "COLLAPSED_VERIFIED" {
		t.Errorf("verdict not applied: %s", folded.Epistemics.Status)
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
