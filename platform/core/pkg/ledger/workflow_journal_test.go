package ledger

import (
	"path/filepath"
	"testing"
)

func TestWorkflowJournalCreateStepFinishAndRecover(t *testing.T) {
	l, err := OpenLedger(filepath.Join(t.TempDir(), "wf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, tenant_id, status, spend_limit_micro_usd) VALUES ('sess-wf','workflow drill','default','active',50000000)`); err != nil {
		t.Fatal(err)
	}
	if err := l.CreateWorkflowRun("wf-1", "sess-wf", `{"name":"drill","steps":[]}`, 5); err != nil {
		t.Fatal(err)
	}
	if err := l.UpsertWorkflowStep(WorkflowStep{RunID: "wf-1", StepID: "s1", Ordinal: 1,
		Tool: "execution/submit_job", Status: "running", JobID: "job-1"}); err != nil {
		t.Fatal(err)
	}
	if err := l.UpsertWorkflowStep(WorkflowStep{RunID: "wf-1", StepID: "s1", Ordinal: 1,
		Tool: "execution/submit_job", Status: "completed", JobID: "job-1", ResultJSON: `{"ok":true}`}); err != nil {
		t.Fatal(err)
	}
	if err := l.FinishWorkflowRun("wf-1", "completed", 1.25, ""); err != nil {
		t.Fatal(err)
	}
	runs, err := l.ListWorkflowRuns("sess-wf", 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "completed" || runs[0].SpentUSD != 1.25 {
		t.Fatalf("run journal wrong: %+v err=%v", runs, err)
	}
	steps, err := l.ListWorkflowSteps("wf-1")
	if err != nil || len(steps) != 1 || steps[0].Status != "completed" || steps[0].JobID != "job-1" {
		t.Fatalf("step journal wrong: %+v err=%v", steps, err)
	}

	// A second run left running is surfaced as repair_required on boot.
	if err := l.CreateWorkflowRun("wf-2", "sess-wf", `{"name":"crash","steps":[]}`, 0); err != nil {
		t.Fatal(err)
	}
	n, err := l.RecoverInterruptedWorkflowRuns()
	if err != nil || n != 1 {
		t.Fatalf("recover: n=%d err=%v", n, err)
	}
	runs, _ = l.ListWorkflowRuns("sess-wf", 10)
	if runs[0].ID != "wf-2" || runs[0].Status != "repair_required" {
		t.Fatalf("interrupted run not surfaced: %+v", runs[0])
	}
}
