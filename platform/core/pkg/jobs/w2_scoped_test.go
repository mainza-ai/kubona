package jobs_test

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/jobs"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

// W2.2: ListJobs with an empty session refuses instead of enumerating
// every tenant's jobs.
func TestW22ListJobsEmptyRefuses(t *testing.T) {
	l, err := ledger.OpenLedger(t.TempDir() + "/w22jobs.db")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	sm := session.NewManager(l)
	s, err := sm.CreateSession("w22 jobs", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	r := jobs.NewRunner(l, nil, 1)
	if _, err := r.ListJobs("", 50); err == nil {
		t.Error("empty ListJobs must refuse")
	}
	if _, err := r.ListJobs(s.ID, 50); err != nil {
		t.Errorf("scoped ListJobs must succeed, got %v", err)
	}
}
