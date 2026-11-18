package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/agents"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

func (f *fakeManuscriptBridge) GroundLiterature(_ context.Context, _ *pb.LiteratureGroundingRequest, _ ...grpc.CallOption) (*pb.LiteratureGroundingResponse, error) {
	if f.literatureErr != nil {
		return nil, f.literatureErr
	}
	if f.literature != nil {
		return &pb.LiteratureGroundingResponse{Records: f.literature}, nil
	}
	return &pb.LiteratureGroundingResponse{
		Records: []*pb.LiteratureRecord{{Id: "W123", Title: "Prior H2 work", Authors: "Doe", PublicationYear: 2024}},
	}, nil
}

func TestHypothesesGenerateRankAndShortlist(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")

	// A measured campaign with a real discrepancy.
	state := agents.CampaignState{
		SessionID: sid, TargetSystem: "H2", ResearchGoal: "H2 ground state",
		Notebook: &agents.SystematicNotebook{
			QuantumEnergy: -1.12, ClassicalEnergy: -1.137, DeltaMHa: 15.2,
		},
		Measurement: &agents.MeasurementSummary{
			Measured: true, Energy: -1.12, EnergyExact: -1.137, DeltaMHa: 15.2,
			Backend: "aer",
		},
	}
	raw, _ := json.Marshal(state)
	if err := srv.Ledger().SaveCampaignState(sid, raw); err != nil {
		t.Fatalf("campaign: %v", err)
	}

	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 12})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Hypotheses []struct {
			ID         string   `json:"id"`
			Statement  string   `json:"statement"`
			Falsifier  string   `json:"falsifier"`
			Score      float64  `json:"score"`
			Status     string   `json:"status"`
			Literature []string `json:"literature_refs"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Hypotheses) < 10 {
		t.Fatalf("hypotheses = %d, want >= 10: %s", len(out.Hypotheses), w.Body.String())
	}
	for i, h := range out.Hypotheses {
		if h.ID == "" || h.Falsifier == "" || h.Statement == "" {
			t.Fatalf("incomplete hypothesis: %+v", h)
		}
		if h.Status != "proposed" {
			t.Fatalf("initial status = %q", h.Status)
		}
		if i > 0 && out.Hypotheses[i-1].Score < h.Score {
			t.Fatalf("hypotheses not ranked by score")
		}
	}
	foundRef := false
	for _, h := range out.Hypotheses {
		if len(h.Literature) > 0 {
			foundRef = true
		}
	}
	if !foundRef {
		t.Fatal("literature refs were not attached from the bridge")
	}

	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+sid+"/hypotheses", "", nil)
	if !strings.Contains(w.Body.String(), out.Hypotheses[0].ID) {
		t.Fatalf("list missing generated hypothesis: %s", w.Body.String())
	}

	id := out.Hypotheses[0].ID
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+id, "", map[string]any{"status": "shortlisted"})
	if w.Code != http.StatusOK {
		t.Fatalf("shortlist = %d: %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, http.MethodPatch, "/api/sessions/"+sid+"/hypotheses/"+id, "", map[string]any{"status": "bogus"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d", w.Code)
	}

	other := createSession(t, srv, "other")
	w = doJSON(t, srv, http.MethodGet, "/api/sessions/"+other+"/hypotheses", "", nil)
	if strings.Contains(w.Body.String(), id) {
		t.Fatal("hypotheses leaked across sessions")
	}
}

func TestHypothesesFromEmptyCampaignStillProposeBaselines(t *testing.T) {
	bridge := &fakeManuscriptBridge{}
	srv, cleanup := setupManuscriptServer(t, bridge)
	defer cleanup()
	sid := createSession(t, srv, "default")
	w := doJSON(t, srv, http.MethodPost, "/api/sessions/"+sid+"/hypotheses", "", map[string]any{"count": 5})
	if w.Code != http.StatusCreated {
		t.Fatalf("generate = %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Hypotheses []struct {
			ExperimentKind string `json:"experiment_kind"`
		} `json:"hypotheses"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Hypotheses) < 3 || out.Hypotheses[0].ExperimentKind != "vqe" {
		t.Fatalf("empty-state hypotheses = %+v", out.Hypotheses)
	}
}
