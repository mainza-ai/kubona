package jobs

// R2b tests: mandate_ref threads from submit params onto the job struct
// and row ("" when unmandated); the settle hook reads it from there.

import (
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
)

func mustOpenJobsLedger(t *testing.T) (*ledger.Ledger, *Runner) {
	t.Helper()
	l, err := ledger.OpenLedger(t.TempDir() + "/r2b.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	now := time.Now().UTC()
	if _, err := l.DB().Exec(`INSERT INTO sessions
		(id, title, tenant_id, created_at, updated_at, status, spend_limit_usd, current_spend_usd)
		VALUES ('sess-r2b', 't', 'default', ?, ?, 'active', 150, 0)`, now, now); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return l, NewRunner(l, nil, 1)
}

func TestSubmitThreadsMandateRef(t *testing.T) {
	_, r := mustOpenJobsLedger(t)
	j, err := r.SubmitJob("sess-r2b", "circuit_run", map[string]any{
		"qasm": "OPENQASM 2.0;qreg q[1];", "requested_backend": "local_simulator",
		"shots": 8, "mandate_ref": "entryhash-abc123",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if j.MandateRef != "entryhash-abc123" {
		t.Fatalf("struct must carry mandate_ref, got %q", j.MandateRef)
	}
	got, err := r.GetJob(j.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MandateRef != "entryhash-abc123" {
		t.Fatalf("row must carry mandate_ref, got %q", got.MandateRef)
	}
}

func TestSubmitWithoutMandateRef(t *testing.T) {
	_, r := mustOpenJobsLedger(t)
	j, err := r.SubmitJob("sess-r2b", "circuit_run", map[string]any{
		"qasm": "OPENQASM 2.0;qreg q[1];", "requested_backend": "local_simulator",
		"shots": 8,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if j.MandateRef != "" {
		t.Fatalf("unmandated job must carry empty ref, got %q", j.MandateRef)
	}
}
