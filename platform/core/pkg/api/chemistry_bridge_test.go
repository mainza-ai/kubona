package api_test

// T-01a: chemistry over the bridge rail, end-to-end through the real mux
// with an in-process fake bridge (bufconn — no network, no python).
// Pins: bridge success serves with evaluated_by=bridge + ledger event;
// domain refusal keeps the local-rail {"error": ...} shape; the
// nil-bridge fallback is the pre-existing local path (covered by the
// existing chemistry tests, which need a real venv).

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type fakeChemBridge struct {
	pb.UnimplementedQuantumBridgeServiceServer
	admetJSON string
	admetErr  string
	pipeJSON  string
}

func (f *fakeChemBridge) EvaluateMolecularADMET(_ context.Context, req *pb.ChemistryADMETRequest) (*pb.ChemistryADMETResponse, error) {
	if f.admetErr != "" {
		return &pb.ChemistryADMETResponse{Success: false, ErrorMessage: f.admetErr}, nil
	}
	return &pb.ChemistryADMETResponse{Success: true, ResultJson: f.admetJSON}, nil
}

func (f *fakeChemBridge) EvaluateDiscoveryPipeline(_ context.Context, req *pb.DiscoveryPipelineRequest) (*pb.DiscoveryPipelineResponse, error) {
	return &pb.DiscoveryPipelineResponse{Success: true, ResultJson: f.pipeJSON}, nil
}

func setupChemDrill(t *testing.T, fake *fakeChemBridge) (*api.Server, *ledger.Ledger) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/chem.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { lis.Close() })
	gs := grpc.NewServer()
	pb.RegisterQuantumBridgeServiceServer(gs, fake)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	sm := session.NewManager(l)
	jr := jobs.NewRunner(l, pb.NewQuantumBridgeServiceClient(conn), 1)
	sup := supervisor.NewSupervisor(l)
	wd := watchdog.NewTaskWatchdog(l, sup)
	return api.NewServer(l, sm, jr, sup, wd), l
}

func TestChemistryADMETViaBridge(t *testing.T) {
	fake := &fakeChemBridge{admetJSON: `{"molecular_weight": 180.16, "clogp": 1.2, "qed": 0.9}`}
	srv, l := setupChemDrill(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/chemistry/admet?smiles=CCO", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bridge ADMET must 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["evaluated_by"] != "bridge" {
		t.Errorf("rail must be attributed to bridge: %+v", out["evaluated_by"])
	}
	if out["molecular_weight"] != 180.16 {
		t.Errorf("bridge payload must pass through verbatim: %v", out["molecular_weight"])
	}
	events, err := l.GetRecentEvents("system", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "molecular_admet_profiled" {
			found = true
		}
	}
	if !found {
		t.Error("bridge ADMET must seal molecular_admet_profiled like the local rail")
	}
}

func TestChemistryADMETBridgeRefusalShape(t *testing.T) {
	fake := &fakeChemBridge{admetErr: "Invalid SMILES string provided: 'zzz'"}
	srv, _ := setupChemDrill(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/chemistry/admet?smiles=zzz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	// Domain refusal keeps the local-rail shape: 200 with an error body
	// (no retry, no second attempt — the bridge already ran it).
	if w.Code != http.StatusOK {
		t.Fatalf("bridge refusal must keep the 200+error shape, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Invalid SMILES") {
		t.Errorf("refusal reason must surface: %s", w.Body.String())
	}
}

func TestDiscoveryPipelineViaBridge(t *testing.T) {
	pipe := `{"target": "COX-2", "total_evaluated": 1, "candidates": [], "top_lead": {"name": "Aspirin"}}`
	fake := &fakeChemBridge{pipeJSON: pipe}
	srv, l := setupChemDrill(t, fake)
	body, _ := json.Marshal(map[string]any{
		"target":     "COX-2",
		"candidates": []any{map[string]any{"name": "Aspirin", "smiles": "CC(=O)Oc1ccccc1C(=O)O"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/discovery/pipeline", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bridge pipeline must 200, got %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["evaluated_by"] != "bridge" {
		t.Errorf("rail must be attributed to bridge: %+v", out["evaluated_by"])
	}
	top, _ := out["top_lead"].(map[string]any)
	if top["name"] != "Aspirin" {
		t.Errorf("bridge payload must pass through verbatim: %+v", out["top_lead"])
	}
	events, err := l.GetRecentEvents("system", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "discovery_pipeline_evaluated" {
			found = true
		}
	}
	if !found {
		t.Error("bridge pipeline must seal discovery_pipeline_evaluated like the local rail")
	}
}
