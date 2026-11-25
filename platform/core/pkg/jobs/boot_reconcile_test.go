package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/session"
	pb "github.com/mainza-ai/kubona/core/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func openJobDB(t *testing.T) (*ledger.Ledger, func()) {
	t.Helper()
	path := t.TempDir() + "/jobs.db"
	l, err := ledger.OpenLedger(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return l, func() { l.Close(); os.Remove(path) }
}

func TestBootReconcileClassifiesDeadOwner(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-boot", "boot", 100, "default"); err != nil {
		t.Fatalf("session: %v", err)
	}
	insert := `INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, owner_epoch) VALUES (?, 'sess-boot', ?, ?, ?, CURRENT_TIMESTAMP, 'default', ?, 0)`
	for _, row := range []struct {
		id, status, kind, params string
		micro                    int64
	}{
		{"queued", "JOB_STATUS_QUEUED", "classical_anchor", "{}", 0},
		{"host", "JOB_STATUS_RUNNING", "classical_anchor", "{}", 2_000_000},
		{"vendor", "JOB_STATUS_RUNNING", "circuit_run", `{"requested_backend":"ibm_brisbane"}`, 3_000_000},
	} {
		if _, err := l.DB().Exec(insert, row.id, row.status, row.kind, row.params, row.micro); err != nil {
			t.Fatalf("seed %s: %v", row.id, err)
		}
	}
	if _, err := l.DB().Exec(`UPDATE tenants SET spent_micro_usd = 5000000 WHERE id = 'default'`); err != nil {
		t.Fatalf("seed spend: %v", err)
	}
	if _, err := l.DB().Exec(`UPDATE sessions SET current_spend_micro_usd = 5000000 WHERE id = 'sess-boot'`); err != nil {
		t.Fatalf("seed session spend: %v", err)
	}

	r := NewRunner(l, nil, 1)
	rec, err := r.ReconcileJobsOnBoot()
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if rec.Requeued != 1 || rec.Interrupted != 1 || rec.Unknown != 1 {
		t.Fatalf("counts = %+v", rec)
	}
	host, _ := r.GetJob("host")
	if host.Status != pb.JobStatus_JOB_STATUS_INTERRUPTED {
		t.Fatalf("host status %s", host.Status)
	}
	var hostReserved int64
	if err := l.DB().QueryRow(`SELECT reserved_amount_micro_usd FROM jobs WHERE id = 'host'`).Scan(&hostReserved); err != nil || hostReserved != 0 {
		t.Fatalf("host reservation must be released, got %d err %v", hostReserved, err)
	}
	vendor, _ := r.GetJob("vendor")
	if vendor.Status != pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN {
		t.Fatalf("vendor status %s", vendor.Status)
	}
	var vendorReserved int64
	if err := l.DB().QueryRow(`SELECT reserved_amount_micro_usd FROM jobs WHERE id = 'vendor'`).Scan(&vendorReserved); err != nil || vendorReserved != 3_000_000 {
		t.Fatalf("vendor reservation must stay, got %d err %v", vendorReserved, err)
	}
	queued, _ := r.GetJob("queued")
	if queued.Status != pb.JobStatus_JOB_STATUS_QUEUED {
		t.Fatalf("queued was retried into %s", queued.Status)
	}
}

func TestOutcomeUnknownKeepsReservationOnTransportError(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-1", "jobs", 100, "default"); err != nil {
		t.Fatalf("session: %v", err)
	}
	params := `{"qasm":"OPENQASM 2.0;","requested_backend":"ibm_brisbane"}`
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd) VALUES ('wire', 'sess-1', 'JOB_STATUS_QUEUED', 'circuit_run', ?, CURRENT_TIMESTAMP, 'default', 2500000)`, params); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := NewRunner(l, &unavailableBridge{}, 1)
	job, err := r.GetJob("wire")
	if err != nil {
		t.Fatal(err)
	}
	r.executeJob(job)
	done, err := r.GetJob("wire")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != pb.JobStatus_JOB_STATUS_OUTCOME_UNKNOWN {
		t.Fatalf("status %s result %s", done.Status, done.ResultJSON)
	}
	var reserved int64
	if err := l.DB().QueryRow(`SELECT reserved_amount_micro_usd FROM jobs WHERE id = 'wire'`).Scan(&reserved); err != nil || reserved != 2_500_000 {
		t.Fatalf("reservation = %d err %v", reserved, err)
	}
	r.executeJob(done)
	var again int64
	var againStatus string
	if err := l.DB().QueryRow(`SELECT status, reserved_amount_micro_usd FROM jobs WHERE id = 'wire'`).Scan(&againStatus, &again); err != nil || againStatus != "JOB_STATUS_OUTCOME_UNKNOWN" || again != 2_500_000 {
		t.Fatalf("second claim status %s reserved %d err %v", againStatus, again, err)
	}
}

func TestSealOutcomeUnknownDoesNotRelease(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('sess-seal', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, dispatched_at) VALUES ('seal', 'sess-seal', 'JOB_STATUS_RUNNING', 'circuit_run', '{"requested_backend":"ibm_brisbane"}', CURRENT_TIMESTAMP, 'default', 4000000, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := NewRunner(l, nil, 1)
	now := time.Now().UTC()
	ok := r.sealOutcomeUnknown(&Job{ID: "seal", SessionID: "sess-seal", JobType: "circuit_run", ParamsJSON: json.RawMessage(`{"requested_backend":"ibm_brisbane"}`), DispatchedAt: &now}, status.Error(codes.Unavailable, "dropped"))
	if !ok {
		t.Fatal("transport after dispatch must seal outcome-unknown")
	}
	var reserved int64
	var st string
	if err := l.DB().QueryRow(`SELECT status, reserved_amount_micro_usd FROM jobs WHERE id = 'seal'`).Scan(&st, &reserved); err != nil {
		t.Fatal(err)
	}
	if st != "JOB_STATUS_OUTCOME_UNKNOWN" || reserved != 4_000_000 {
		t.Fatalf("status %s reserved %d", st, reserved)
	}
	if r.sealOutcomeUnknown(&Job{ID: "seal", JobType: "classical_anchor", DispatchedAt: &now}, status.Error(codes.Unavailable, "dropped")) {
		t.Fatal("host-only transport must not seal outcome-unknown")
	}
}

func TestLateResultDoesNotSettle(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, attempt_id, owner_epoch) VALUES ('late', 's', 'JOB_STATUS_RUNNING', 'circuit_run', '{"requested_backend":"ibm_brisbane"}', CURRENT_TIMESTAMP, 'default', 2000000, 'attempt-current', 3)`); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(l, nil, 1)
	r.SyncQuantumBridgeJobStatus("late", "completed", `{"attempt_id":"attempt-old","owner_epoch":1,"hardware_job_id":"hw","actual_usd":0}`)
	var status string
	var reserved int64
	if err := l.DB().QueryRow(`SELECT status, reserved_amount_micro_usd FROM jobs WHERE id = 'late'`).Scan(&status, &reserved); err != nil {
		t.Fatal(err)
	}
	if status != "JOB_STATUS_RUNNING" || reserved != 2_000_000 {
		t.Fatalf("late result settled the job: %s %d", status, reserved)
	}
	var n int
	if err := l.DB().QueryRow(`SELECT COUNT(*) FROM ledger_events WHERE event_type = 'late_result_quarantined'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("quarantine events %d err %v", n, err)
	}
	r.SyncQuantumBridgeJobStatus("late", "completed", `{"attempt_id":"attempt-current","owner_epoch":3,"hardware_job_id":"hw","actual_usd":0}`)
	if err := l.DB().QueryRow(`SELECT status FROM jobs WHERE id = 'late'`).Scan(&status); err != nil || status != "JOB_STATUS_COMPLETED" {
		t.Fatalf("current attempt must settle, got %s %v", status, err)
	}
}

func TestAdoptVendorPinBeforeAck(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at) VALUES ('go-1', 's', 'JOB_STATUS_RUNNING', 'hardware_experiment', '{}', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("job: %v", err)
	}
	path := t.TempDir() + "/vendor_pins.jsonl"
	if err := os.WriteFile(path, []byte("{\"vendor_job_id\":\"ibm-9\",\"go_job_id\":\"go-1\",\"session_id\":\"s\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(l, nil, 1)
	n, err := r.AdoptVendorPins(path)
	if err != nil || n != 1 {
		t.Fatalf("adopted %d err %v", n, err)
	}
	var ref string
	if err := l.DB().QueryRow(`SELECT vendor_ref FROM jobs WHERE id = 'go-1'`).Scan(&ref); err != nil || ref != "ibm-9" {
		t.Fatalf("vendor_ref %q err %v", ref, err)
	}
	if _, err := l.DB().Exec(`UPDATE jobs SET vendor_ref = 'other' WHERE id = 'go-1'`); err == nil {
		t.Fatal("pinned vendor_ref must be immutable")
	}
}

func TestJobIdentityIsImmutable(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, request_id) VALUES ('j', 's', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'req-1')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := l.DB().Exec(`UPDATE jobs SET request_id = 'req-2' WHERE id = 'j'`); err == nil {
		t.Fatal("request_id rewrite must abort")
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, request_id) VALUES ('j2', 's', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP, 'req-1')`); err == nil {
		t.Fatal("duplicate session request_id must refuse")
	}
	if _, err := l.DB().Exec(`UPDATE jobs SET status = 'JOB_STATUS_OUTCOME_UNKNOWN' WHERE id = 'j'`); err != nil {
		t.Fatalf("status change: %v", err)
	}
	if _, err := l.DB().Exec(`DELETE FROM jobs WHERE id = 'j'`); err == nil {
		t.Fatal("outcome-unknown delete must abort")
	}
}

type unavailableBridge struct{ fakeBridge }

func (unavailableBridge) ExecuteCircuit(context.Context, *pb.CircuitExecutionRequest, ...grpc.CallOption) (*pb.CircuitExecutionResponse, error) {
	return nil, status.Error(codes.Unavailable, "upstream dropped")
}

func TestGetJobReadsIdentityColumns(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, owner_epoch, attempt_id, dispatched_at, vendor_ref, request_id) VALUES ('j', 's', 'JOB_STATUS_OUTCOME_UNKNOWN', 'circuit_run', '{}', CURRENT_TIMESTAMP, 4, 'attempt-4', CURRENT_TIMESTAMP, 'ibm-9', 'req-live')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := NewRunner(l, nil, 1).GetJob("j")
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerEpoch != 4 || got.AttemptID != "attempt-4" || got.VendorRef != "ibm-9" || got.RequestID != "req-live" || got.DispatchedAt == nil {
		t.Fatalf("identity not loaded: epoch=%d attempt=%q vendor=%q request=%q dispatched=%v", got.OwnerEpoch, got.AttemptID, got.VendorRef, got.RequestID, got.DispatchedAt)
	}
	listed, err := NewRunner(l, nil, 1).ListJobs("s", 10)
	if err != nil || len(listed) != 1 || listed[0].RequestID != "req-live" || listed[0].VendorRef != "ibm-9" {
		t.Fatalf("list identity: %+v err %v", listed, err)
	}
}

func TestBootReconcileOverflowFeedsQueue(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	if _, err := l.DB().Exec(`INSERT INTO sessions (id, title, created_at, updated_at, status) VALUES ('s', 's', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active')`); err != nil {
		t.Fatalf("session: %v", err)
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("jq-%d", i)
		if _, err := l.DB().Exec(`INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at) VALUES (?, 's', 'JOB_STATUS_QUEUED', 'circuit_run', '{}', CURRENT_TIMESTAMP)`, id); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	r := NewRunner(l, nil, 1)
	r.jobQueue = make(chan *Job, 2)
	rec, err := r.ReconcileJobsOnBoot()
	if err != nil {
		t.Fatalf("ReconcileJobsOnBoot overflow must not fail: %v", err)
	}
	if rec.Requeued != 5 {
		t.Fatalf("expected 5 requeued, got %d", rec.Requeued)
	}
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		select {
		case j := <-r.jobQueue:
			seen[j.ID] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out draining overflow job %d", i)
		}
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 unique drained jobs, got %d", len(seen))
	}
	r.cancel()
	r.wg.Wait()
}

func TestListAllSessionJobsPagesBeyondThe200RowCap(t *testing.T) {
	l, cleanup := openJobDB(t)
	defer cleanup()
	sm := session.NewManager(l)
	if _, err := sm.EnsureSessionWithID("sess-page", "page", 100, "default"); err != nil {
		t.Fatalf("session: %v", err)
	}
	insert := `INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, owner_epoch) VALUES (?, 'sess-page', 'JOB_STATUS_COMPLETED', 'vqe', '{}', CURRENT_TIMESTAMP, 'default', 0, 0)`
	for i := 0; i < 205; i++ {
		id := fmt.Sprintf("page-%03d", i)
		if _, err := l.DB().Exec(insert, id); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	got, err := NewRunner(l, nil, 1).ListAllSessionJobs("sess-page")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(got) != 205 {
		t.Fatalf("expected 205 jobs across pages, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, j := range got {
		if seen[j.ID] {
			t.Fatalf("duplicate job %s across pages", j.ID)
		}
		seen[j.ID] = true
	}
}
