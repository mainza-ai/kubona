package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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
	if !strings.Contains(w.Body.String(), `"version":"live-test"`) {
		t.Fatalf("live profile not installed: %s", w.Body.String())
	}
	_ = os.Remove(dir + "/cal.db")
}
