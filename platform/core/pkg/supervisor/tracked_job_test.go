package supervisor

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func TestCompleteTrackedJobSettlesSingleJobLoop(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/tracked.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := NewSupervisor(l)
	t.Cleanup(s.Close)

	loop, err := s.RegisterLoop("sess-tracked", 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordPart("sess-tracked", LoopPart{
		LoopID: loop.ID, Helper: "quantum_job", Title: "Dispatched circuit_run",
		State: "running", JobID: "job-1",
	}); err != nil {
		t.Fatal(err)
	}
	if n := s.CompleteTrackedJob("sess-tracked", "job-1", "completed"); n != 1 {
		t.Fatalf("settled %d loops, want 1", n)
	}
	got, ok := s.GetLoop(loop.ID)
	if !ok || got.Status != LoopCompleted {
		t.Fatalf("loop = %+v, want completed", got)
	}

	// A multi-job run is not settled by one terminal job.
	multi, err := s.RegisterLoop("sess-tracked", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, jobID := range []string{"job-a", "job-b"} {
		if _, err := s.RecordPart("sess-tracked", LoopPart{
			LoopID: multi.ID, Helper: "quantum_job", Title: "Dispatched circuit_run",
			State: "running", JobID: jobID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.CompleteTrackedJob("sess-tracked", "job-a", "completed"); n != 0 {
		t.Fatalf("multi-job run settled by one job: %d", n)
	}
	got, _ = s.GetLoop(multi.ID)
	if got.Status != LoopRunning {
		t.Fatalf("multi-job loop = %q, want running", got.Status)
	}

	// A failed tracked job aborts the run, not completes it.
	failed, err := s.RegisterLoop("sess-tracked", 50)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordPart("sess-tracked", LoopPart{
		LoopID: failed.ID, Helper: "quantum_job", Title: "Dispatched circuit_run",
		State: "running", JobID: "job-f",
	}); err != nil {
		t.Fatal(err)
	}
	if n := s.CompleteTrackedJob("sess-tracked", "job-f", "failed"); n != 1 {
		t.Fatalf("failed settle count = %d", n)
	}
	got, _ = s.GetLoop(failed.ID)
	if got.Status != LoopAborted {
		t.Fatalf("failed loop = %q, want aborted", got.Status)
	}
}
