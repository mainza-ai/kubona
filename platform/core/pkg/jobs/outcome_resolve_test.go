package jobs

import (
	"testing"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
)

func seedLab(t *testing.T, l *ledger.Ledger) {
	t.Helper()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-1", "resolve", 100, "default"); err != nil {
		t.Fatal(err)
	}
}

func seedUnknown(t *testing.T, r *Runner, id string, micro int64, vendor string) {
	t.Helper()
	if _, err := r.ledger.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, vendor_ref) VALUES (?, 'sess-1', 'JOB_STATUS_OUTCOME_UNKNOWN', 'circuit_run', '{"requested_backend":"ibm_brisbane"}', CURRENT_TIMESTAMP, 'default', ?, ?)`, id, micro, vendor); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOutcomeUnknown(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	seedLab(t, l)
	if _, err := l.DB().Exec(`UPDATE tenants SET spent_micro_usd = 6000000 WHERE id = 'default'`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE sessions SET current_spend_micro_usd = 6000000 WHERE id = 'sess-1'`); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(l, nil, 1)
	seedUnknown(t, r, "never", 2_000_000, "")
	seedUnknown(t, r, "keep", 2_000_000, "")
	seedUnknown(t, r, "meter", 2_000_000, "ibm-1")
	if err := r.ResolveOutcomeUnknown("never", "vendor_never_received", "vendor lookup returned no job", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := r.ResolveOutcomeUnknown("never", "vendor_never_received", "again", "", nil); err == nil {
		t.Fatal("second resolution must refuse")
	}
	if err := r.ResolveOutcomeUnknown("keep", "written_off", "operator wrote off the estimate", "", nil); err != nil {
		t.Fatal(err)
	}
	one := 1.0
	if err := r.ResolveOutcomeUnknown("meter", "vendor_confirmed", "", "ibm-1", &one); err != nil {
		t.Fatal(err)
	}
	assertJob := func(id, status string, reserved int64) {
		t.Helper()
		var gotStatus string
		var gotReserved int64
		if err := l.DB().QueryRow(`SELECT status, reserved_amount_micro_usd FROM jobs WHERE id = ?`, id).Scan(&gotStatus, &gotReserved); err != nil {
			t.Fatal(err)
		}
		if gotStatus != status || gotReserved != reserved {
			t.Fatalf("%s: status %s reserved %d", id, gotStatus, gotReserved)
		}
	}
	assertJob("never", "JOB_STATUS_FAILED", 0)
	assertJob("keep", "JOB_STATUS_FAILED", 2_000_000)
	assertJob("meter", "JOB_STATUS_COMPLETED", 1_000_000)
	var spent int64
	if err := l.DB().QueryRow(`SELECT spent_micro_usd FROM tenants WHERE id = 'default'`).Scan(&spent); err != nil {
		t.Fatal(err)
	}
	if spent != 3_000_000 {
		t.Fatalf("tenant spent %d, want 3000000", spent)
	}
	seedUnknown(t, r, "pinned", 1_000_000, "ibm-kept")
	if err := r.ResolveOutcomeUnknown("pinned", "vendor_never_received", "should refuse", "", nil); err == nil {
		t.Fatal("pinned vendor id must refuse never-received")
	}
}

func TestKillPointsDoNotDoubleChargeOrRefund(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	seedLab(t, l)
	r := NewRunner(l, nil, 1)
	if _, err := l.DB().Exec(`UPDATE tenants SET spent_micro_usd = 5000000 WHERE id = 'default'`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE sessions SET current_spend_micro_usd = 5000000 WHERE id = 'sess-1'`); err != nil {
		t.Fatal(err)
	}
	spent := func() int64 {
		t.Helper()
		var n int64
		if err := l.DB().QueryRow(`SELECT spent_micro_usd FROM tenants WHERE id = 'default'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := spent()

	testCrashBeforeCommit = true
	_, err := r.SubmitJob("sess-1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE", "max_cost_usd": 1.0})
	testCrashBeforeCommit = false
	if err == nil || spent() != before {
		t.Fatalf("submit crash must leave no charge: err %v spent %d", err, spent())
	}

	if _, err := r.SubmitJobWithRequest("sess-1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE", "max_cost_usd": 1.0}, "req-kill"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SubmitJobWithRequest("sess-1", "mitigate", map[string]any{"qasm": "OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\n", "method": "ZNE", "max_cost_usd": 1.0}, "req-kill"); err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM jobs WHERE request_id = 'req-kill'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("claim retry jobs %d err %v", jobs, err)
	}
	if spent() != before+1_000_000 {
		t.Fatalf("one reservation, spent %d", spent())
	}

	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, owner_epoch) VALUES ('disp', 'sess-1', 'JOB_STATUS_RUNNING', 'circuit_run', '{"requested_backend":"ibm_brisbane"}', CURRENT_TIMESTAMP, 'default', 2000000, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`UPDATE tenants SET spent_micro_usd = spent_micro_usd + 2000000 WHERE id = 'default'`); err != nil {
		t.Fatal(err)
	}
	next := NewRunner(l, nil, 1)
	rec, err := next.ReconcileJobsOnBoot()
	if err != nil || rec.Unknown != 1 {
		t.Fatalf("dispatch kill: %+v %v", rec, err)
	}
	var status string
	var reserved int64
	if err := l.DB().QueryRow(`SELECT status, reserved_amount_micro_usd FROM jobs WHERE id = 'disp'`).Scan(&status, &reserved); err != nil {
		t.Fatal(err)
	}
	if status != "JOB_STATUS_OUTCOME_UNKNOWN" || reserved != 2_000_000 {
		t.Fatalf("dispatch kill refunded or stranded: %s %d", status, reserved)
	}
	r.releaseReservation("disp")
	r.releaseReservation("disp")
	if err := l.DB().QueryRow(`SELECT reserved_amount_micro_usd FROM jobs WHERE id = 'disp'`).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatalf("settle left %d", reserved)
	}
	if spent() != before+1_000_000 {
		t.Fatalf("double release moved money: spent %d", spent())
	}
}
