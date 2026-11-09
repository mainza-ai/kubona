package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/api"
	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/supervisor"
	"github.com/mainza-ai/kubona/core/pkg/watchdog"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
)

type calibrationBridge struct {
	pb.QuantumBridgeServiceClient
}

func (b *calibrationBridge) RunExperiment(context.Context, *pb.ExperimentRequest, ...grpc.CallOption) (*pb.ExperimentResponse, error) {
	return &pb.ExperimentResponse{
		Ok:         true,
		ResultJson: `{"success":true,"result_version":"experiment-result-v1","kind":"calibration","snapshot":{"qubits":156,"t1_us":150,"t2_us":100,"single_qubit_error":0.0002,"two_qubit_error":0.002,"readout_error":0.01,"source":"backend-properties:ibm_test","version":"live-test","calibrated_at":"2026-11-09T00:00:00+00:00"}}`,
	}, nil
}

func TestCalibrationRefreshInstallsLiveProfile(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "cal-token")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/cal.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := api.NewServer(l, sm, jobs.NewRunner(l, &calibrationBridge{}, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	body := strings.NewReader(`{"backend":"ibm_test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/calibration/refresh", body)
	req.Header.Set("Authorization", "Bearer cal-token")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("calibration refresh: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"fidelity_version":"live-test"`) {
		t.Fatalf("live profile not installed: %s", w.Body.String())
	}
	_ = os.Remove(dir + "/cal.db")
}

func TestCalibrationRefreshRefusesMissingBackendAndDeadBridge(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "cal-token")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/cal2.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := api.NewServer(l, sm, jobs.NewRunner(l, nil, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/fleet/calibration/refresh", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer cal-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing backend must 400, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/fleet/calibration/refresh", strings.NewReader(`{"backend":"ibm_test"}`))
	req.Header.Set("Authorization", "Bearer cal-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil bridge must 503, got %d: %s", w.Code, w.Body.String())
	}
}

type badCalibrationBridge struct {
	pb.QuantumBridgeServiceClient
	ok     bool
	result string
}

func (b *badCalibrationBridge) RunExperiment(context.Context, *pb.ExperimentRequest, ...grpc.CallOption) (*pb.ExperimentResponse, error) {
	return &pb.ExperimentResponse{Ok: b.ok, ResultJson: b.result, ErrorMessage: "vendor refused"}, nil
}

func TestCalibrationRefreshRefusesUpstreamAndInvalidSnapshot(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "cal-token")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/cal3.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)

	for name, bridge := range map[string]*badCalibrationBridge{
		"not-ok":     {ok: false},
		"not-json":   {ok: true, result: "not-json"},
		"unphysical": {ok: true, result: `{"success":true,"snapshot":{"qubits":156,"t1_us":100,"t2_us":300,"readout_error":0.01,"source":"x","version":"v","calibrated_at":"2026-11-09T00:00:00Z"}}`},
	} {
		srv := api.NewServer(l, sm, jobs.NewRunner(l, bridge, 1), sup, watchdog.NewTaskWatchdog(l, sup))
		req := httptest.NewRequest(http.MethodPost, "/api/fleet/calibration/refresh", strings.NewReader(`{"backend":"ibm_test"}`))
		req.Header.Set("Authorization", "Bearer cal-token")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if name == "not-ok" || name == "not-json" || name == "unphysical" {
			if w.Code != http.StatusBadGateway {
				t.Fatalf("%s must 502, got %d: %s", name, w.Code, w.Body.String())
			}
		}
	}
}

func TestScheduledCalibrationRefreshUpdatesFleetProfile(t *testing.T) {
	t.Setenv("KUBONA_API_TOKEN", "cal-token")
	dir := t.TempDir()
	l, err := ledger.OpenLedger(dir + "/cal4.db")
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	sup := supervisor.NewSupervisor(l)
	srv := api.NewServer(l, sm, jobs.NewRunner(l, &calibrationBridge{}, 1), sup, watchdog.NewTaskWatchdog(l, sup))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.StartCalibrationRefresh(ctx, 5*time.Millisecond, "ibm_test")
	deadline := time.Now().Add(2 * time.Second)
	for {
		req := httptest.NewRequest(http.MethodGet, "/api/fleet", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), `"fidelity_version":"live-test"`) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scheduled refresh did not install live profile: %d %s", w.Code, w.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
