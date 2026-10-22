package api_test

// T-01b: codesign over the bridge rail, end-to-end through the real mux
// with an in-process fake bridge (bufconn). The fake returns canned
// envelopes; the real-op equivalence is pinned by the unchanged handler
// suite (TestCodesign*Proxy runs the fallback shim against the ported
// codesign_ops module) plus the compose live run.

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

type fakeCodesignBridge struct {
	pb.UnimplementedQuantumBridgeServiceServer
	resultJSON string
	errMsg     string
	gotOp      string
}

func (f *fakeCodesignBridge) EvaluateCodesign(_ context.Context, req *pb.CodesignRequest) (*pb.CodesignResponse, error) {
	f.gotOp = req.Operation
	if f.errMsg != "" {
		return &pb.CodesignResponse{Success: false, ErrorMessage: f.errMsg}, nil
	}
	return &pb.CodesignResponse{Success: true, ResultJson: f.resultJSON}, nil
}

func setupCodesignDrill(t *testing.T, fake *fakeCodesignBridge) *api.Server {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/codesign.db")
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
	return api.NewServer(l, sm, jr, sup, wd)
}

func TestCodesignEvaluateViaBridge(t *testing.T) {
	fake := &fakeCodesignBridge{resultJSON: `{"ok": true, "spec_kind": "adder", "result": {"valid": true}}`}
	srv := setupCodesignDrill(t, fake)
	body, _ := json.Marshal(map[string]any{"spec_kind": "adder", "num_tests": 8, "artifact": []any{}})
	req := httptest.NewRequest(http.MethodPost, "/api/codesign/evaluate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bridge codesign must 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.gotOp != "evaluate" {
		t.Errorf("operation must travel the wire, got %q", fake.gotOp)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["spec_kind"] != "adder" {
		t.Errorf("bridge payload must pass through verbatim: %+v", out)
	}
}

func TestCodesignBridgeRefusalMapsTo400(t *testing.T) {
	fake := &fakeCodesignBridge{errMsg: "kappa must be an int in [1,3]: refusing"}
	srv := setupCodesignDrill(t, fake)
	body, _ := json.Marshal(map[string]any{"spec_kind": "adder", "kappa": 9})
	req := httptest.NewRequest(http.MethodPost, "/api/codesign/evaluate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	// Domain refusal keeps the retired-snippet status mapping: 400 naming
	// the refusal, never a 502/503.
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bridge refusal must 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kappa must be an int") {
		t.Errorf("refusal reason must surface verbatim: %s", w.Body.String())
	}
}

func TestCodesignPacksViaBridgeGet(t *testing.T) {
	fake := &fakeCodesignBridge{resultJSON: `{"ok": true, "order": ["a"], "packs": []}`}
	srv := setupCodesignDrill(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api/codesign/packs", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bridge packs GET must 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.gotOp != "packs" {
		t.Errorf("operation must travel the wire, got %q", fake.gotOp)
	}
}
