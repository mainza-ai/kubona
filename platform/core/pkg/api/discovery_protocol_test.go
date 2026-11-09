package api

import (
	"strings"
	"testing"

	pb "github.com/mainza-ai/kubona/core/proto"
)

func TestPlanDiscoverySelectsMeasuredKinds(t *testing.T) {
	cases := []struct {
		goal string
		kind string
	}{
		{"measure the H2 excited state gap", "excited_state"},
		{"transverse-field Ising spin chain", "spin_chain"},
		{"Heisenberg chain ground energy", "spin_chain"},
		{"H2 potential energy surface", "pes"},
		{"water active-space VQE", "vqe"},
		{"ground state baseline", "vqe"},
	}
	for _, tc := range cases {
		steps, reason, err := planDiscovery(tc.goal)
		if err != nil {
			t.Fatalf("%q: %v", tc.goal, err)
		}
		if len(steps) != 1 || steps[0].Kind != tc.kind {
			t.Fatalf("%q: got %+v (%s)", tc.goal, steps, reason)
		}
	}
}

func TestPlanDiscoveryRefusesUnboundedConjecture(t *testing.T) {
	_, _, err := planDiscovery("find a new conjecture")
	if err == nil || !strings.Contains(err.Error(), "explicit values") {
		t.Fatalf("conjecture plan must refuse without data, got %v", err)
	}
	if _, _, err := planDiscovery("  "); err == nil {
		t.Fatal("empty goal must refuse")
	}
}

func TestTerminalJobStatus(t *testing.T) {
	for _, status := range []pb.JobStatus{
		pb.JobStatus_JOB_STATUS_COMPLETED,
		pb.JobStatus_JOB_STATUS_FAILED,
		pb.JobStatus_JOB_STATUS_CANCELLED,
	} {
		if !terminalJobStatus(status) {
			t.Fatalf("%s must be terminal", status)
		}
	}
	for _, status := range []pb.JobStatus{
		pb.JobStatus_JOB_STATUS_QUEUED,
		pb.JobStatus_JOB_STATUS_RUNNING,
	} {
		if terminalJobStatus(status) {
			t.Fatalf("%s must not be terminal", status)
		}
	}
}
