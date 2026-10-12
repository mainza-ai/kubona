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
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
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
	if folded.Epistemics.Status != "CONTESTED" {
		t.Errorf("pass on fallback anchor must withhold collapse: %s", folded.Epistemics.Status)
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

// R2: the no-campaign notebook path with no bridge behind it must read as
// an assumed template — UNVERIFIED_FALLBACK, accuracy false, fallback
// named in the anchor and sections. Never VERIFIED_ACCURATE.
func TestNotebooksFallbackLabeled(t *testing.T) {
	srv, _, cleanup := replanHookTestServer(t, "test_notebooks_fallback.db")
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/results/notebooks", nil)
	rec := httptest.NewRecorder()
	srv.handleNotebooks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notebooks must answer 200, got %d", rec.Code)
	}
	var body struct {
		Notebooks []struct {
			ClassicalAnchor  string   `json:"classical_anchor"`
			ChemicalAccuracy bool     `json:"chemical_accuracy"`
			EpistemicVerdict string   `json:"epistemic_verdict"`
			Sections         []string `json:"sections"`
		} `json:"notebooks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notebooks) != 1 {
		t.Fatalf("expected 1 template notebook, got %d", len(body.Notebooks))
	}
	nb := body.Notebooks[0]
	if nb.EpistemicVerdict != "UNVERIFIED_FALLBACK" {
		t.Errorf("fallback verdict must be UNVERIFIED_FALLBACK, got %q", nb.EpistemicVerdict)
	}
	if nb.ChemicalAccuracy {
		t.Error("accuracy cannot pass on an assumed anchor")
	}
	if !strings.Contains(nb.ClassicalAnchor, "fallback") {
		t.Errorf("anchor must name the fallback: %q", nb.ClassicalAnchor)
	}
	if !strings.Contains(nb.Sections[1], "UNAVAILABLE") {
		t.Errorf("section 2 must disclose the missing anchor: %q", nb.Sections[1])
	}
	for _, sec := range nb.Sections {
		if strings.Contains(sec, "verification (|") {
			t.Errorf("no verification language on fallback: %q", sec)
		}
	}
}

// stubNotebookBridge answers ComputeClassicalAnchor with fixed H2 numbers
// so the notebooks template success path is testable without a live bridge.
// All other RPCs nil-panic if touched — the handler must not touch them.
type stubNotebookBridge struct {
	pb.QuantumBridgeServiceClient
}

func (s stubNotebookBridge) ComputeClassicalAnchor(ctx context.Context, in *pb.ClassicalAnchorRequest, _ ...grpc.CallOption) (*pb.ClassicalAnchorResponse, error) {
	return &pb.ClassicalAnchorResponse{
		Success: true, Molecule: in.Molecule,
		RhfEnergy: -1.11668, FciExactEnergy: -1.13727,
	}, nil
}

// Audit fix: even with a REAL anchor, the template path must never claim a
// VQE execution or a verification — verdict PROJECTION, accuracy false,
// corridor estimate labeled as such. Only job folds verify.
func TestNotebooksTemplateSuccessIsProjection(t *testing.T) {
	l, err := ledger.OpenLedger("test_notebooks_projection.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		l.Close()
		os.Remove("test_notebooks_projection.db")
		os.Remove("test_notebooks_projection.db-wal")
		os.Remove("test_notebooks_projection.db-shm")
	}()
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, stubNotebookBridge{}, 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	srv := NewServer(l, sm, jr, sup, wd)

	req := httptest.NewRequest(http.MethodGet, "/api/results/notebooks", nil)
	rec := httptest.NewRecorder()
	srv.handleNotebooks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notebooks must answer 200, got %d", rec.Code)
	}
	var body struct {
		Notebooks []struct {
			ClassicalAnchor  string   `json:"classical_anchor"`
			ChemicalAccuracy bool     `json:"chemical_accuracy"`
			EpistemicVerdict string   `json:"epistemic_verdict"`
			Sections         []string `json:"sections"`
		} `json:"notebooks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notebooks) != 1 {
		t.Fatalf("expected 1 template notebook, got %d", len(body.Notebooks))
	}
	nb := body.Notebooks[0]
	if nb.EpistemicVerdict != "PROJECTION" {
		t.Errorf("template with real anchor must be PROJECTION, got %q", nb.EpistemicVerdict)
	}
	if nb.ChemicalAccuracy {
		t.Error("template path must never pass accuracy — only folds decide")
	}
	if !strings.Contains(nb.ClassicalAnchor, "PySCF") {
		t.Errorf("real anchor must be named: %q", nb.ClassicalAnchor)
	}
	for _, sec := range nb.Sections {
		if strings.Contains(sec, "execution on Aer") || strings.Contains(sec, "collapse and chemical accuracy verification") {
			t.Errorf("no execution/verification fiction on the template path: %q", sec)
		}
	}
	if !strings.Contains(nb.Sections[2], "not measured") || !strings.Contains(nb.Sections[3], "unclaimed until measured") {
		t.Errorf("estimate must be labeled as unmeasured: %v", nb.Sections[2:4])
	}
}
