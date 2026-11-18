package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

func (f *fakeManuscriptBridge) RunExperiment(_ context.Context, _ *pb.ExperimentRequest, _ ...grpc.CallOption) (*pb.ExperimentResponse, error) {
	return &pb.ExperimentResponse{
		Ok:         true,
		ResultJson: `{"result_version":"experiment-result-v1","success":true,"kind":"spin_chain","model":"tfim","n_sites":8,"energy":-10.2516,"energy_exact":-10.2516}`,
	}, nil
}

func TestPlanRequiresAcceptanceAndApprovalThenExecutes(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)

	// Missing acceptance metric refuses.
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "s", "method": "spin_chain", "observable": "energy",
		"spec": map[string]any{"kind": "spin_chain"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing acceptance = %d", w.Code)
	}

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"statement": "The TFIM measured energy reproduces its exact anchor.",
		"method":    "spin_chain", "observable": "energy",
		"acceptance_metric": "energy_exact", "tolerance": 0.05,
		"stop_rule": "one run; reject if outside tolerance", "budget_usd": 1.0,
		"spec": map[string]any{"kind": "spin_chain", "model": "tfim", "n_sites": 8,
			"coupling": 1.0, "field": 1.0, "periodic": true, "layers": 2,
			"target_error": 0.005, "shots": 2048},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create plan = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	pid := created.Plan.ID

	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+pid+"/execute", "", nil)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "plan_not_approved") {
		t.Fatalf("unapproved execute = %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/plans/"+pid, "", map[string]any{"status": "approved"})
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans/"+pid+"/execute", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("execute = %d: %s", w.Code, w.Body.String())
	}
	var executed struct {
		Accepted bool `json:"accepted"`
		Plan     struct {
			Status string `json:"status"`
			JobID  string `json:"job_id"`
		} `json:"plan"`
	}
	json.Unmarshal(w.Body.Bytes(), &executed)
	if !executed.Accepted || executed.Plan.Status != "completed" || executed.Plan.JobID == "" {
		t.Fatalf("execution result = %+v body=%s", executed, w.Body.String())
	}
}

func TestResearchGraphAssemblesSessionState(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	seedCampaign(t, srv, sid)

	// One hypothesis and one plan.
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("hypotheses = %d: %s", w.Code, w.Body.String())
	}
	var hy struct {
		Hypotheses []struct {
			ID string `json:"id"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &hy)
	w = doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/plans", "", map[string]any{
		"hypothesis_id": hy.Hypotheses[0].ID, "method": "spin_chain",
		"observable": "energy", "acceptance_metric": "energy_exact", "tolerance": 0.05,
		"stop_rule": "one run", "spec": map[string]any{"kind": "spin_chain", "model": "tfim", "n_sites": 4},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("plan = %d: %s", w.Code, w.Body.String())
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/research-graph", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("graph = %d: %s", w.Code, w.Body.String())
	}
	var graph struct {
		Hypotheses []any `json:"hypotheses"`
		Plans      []any `json:"plans"`
		Counts     struct {
			Hypotheses int `json:"hypotheses"`
			Plans      int `json:"plans"`
		} `json:"counts"`
	}
	json.Unmarshal(w.Body.Bytes(), &graph)
	if graph.Counts.Hypotheses == 0 || graph.Counts.Plans != 1 || len(graph.Hypotheses) == 0 || len(graph.Plans) != 1 {
		t.Fatalf("graph counts = %+v", graph.Counts)
	}
}
