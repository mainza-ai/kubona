package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mainza-ai/kubona/core/pkg/disk"
	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/ledger"
	"github.com/mainza-ai/kubona/core/pkg/money"
	"github.com/mainza-ai/kubona/core/pkg/qir"
	"github.com/mainza-ai/kubona/core/pkg/session"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
	pb "github.com/mainza-ai/kubona/core/proto"
)

// MaxStoredResultBytes caps one persisted job result blob (S6). Structured
// bridge/sweep payloads refuse above this (fail-closed); sandbox streams
// truncate with flags at parseHarnessOutput instead (truncating JSON would
// corrupt it, truncating logs is honest).
const MaxStoredResultBytes = 1 * 1024 * 1024

// knownJobTypes is the closed set of dispatchable job types (W0.2).
// Unknown types refuse at submit with no residue — previously they queued,
// reserved nothing, then failed in the worker after autonomy had passed
// them. The dispatch switch in executeJob covers exactly this set.
var knownJobTypes = map[string]bool{
	"circuit_run": true, "circuit_cut": true, "classical_anchor": true,
	"claim_eval": true, "krylov_solve": true, "mitigate": true,
	"pes_sweep": true,
}

// W0.1 — CAS-guard live set, bound once from the single source
// (session.LiveJobStatuses). The clause fragment and its bind args stay
// adjacent so they cannot rot apart.
var (
	liveStatusClause string
	liveStatusArgs   []any
)

func init() {
	liveStatusClause, liveStatusArgs = session.LiveJobStatusClause()
}

type Job struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	Status      pb.JobStatus    `json:"status"`
	JobType     string          `json:"job_type"` // "circuit_run", "circuit_cut", "classical_anchor", "claim_eval"
	ParamsJSON  json.RawMessage `json:"params_json"`
	ResultJSON  json.RawMessage `json:"result_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	// Spend reservation bound at admission (tenant + authorized USD).
	// Completed jobs keep it; every terminal-failed path releases it.
	ReservedTenant string  `json:"reserved_tenant,omitempty"`
	ReservedAmount float64 `json:"reserved_amount,omitempty"`
	// Compile provenance: which compiler analyzed the circuit pre-dispatch
	// and whether the metrics are fallback estimates. Empty CompilerUsed
	// means no compiler ran (runner without one) — never implied.
	CompilerUsed     string `json:"compiler_used,omitempty"`
	FallbackEstimate bool   `json:"fallback_estimate,omitempty"`
	// R2b: the spend-mandate receipt this job settles against (ledger
	// entry_hash of the mandate_verified event; "" for unmandated local
	// work). The settle hook closes the mandate's obligation from this.
	MandateRef string `json:"mandate_ref,omitempty"`
}

type Runner struct {
	ledger       *ledger.Ledger
	bridgeClient pb.QuantumBridgeServiceClient
	jobQueue     chan *Job
	workers      int
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	// aborted marks job IDs a supervisor abort settled as failed. Workers
	// consult it at execution entry and after bridge calls so an abort that
	// lands mid-flight still wins over a late completion.
	mu      sync.Mutex
	aborted map[string]bool
	// tenantMgr enforces lab-tenant autonomy and spend ceilings at admission.
	tenantMgr *tenants.Manager
	// sessionMgr enforces per-session spend limits at admission (same ledger,
	// constructed here so the Runner signature stays stable).
	sessionMgr *session.Manager
	// fidelityGate predicts whether a hardware run can produce a publishable
	// number before spending (Ring 3 second rail beside money).
	fidelityGate *fidelity.FidelityGate
	// compiler analyzes circuits pre-dispatch (see SubmitJob). Nil means
	// no analysis runs and compile columns stay empty — the honest
	// "unanalyzed" state, never a silent skip. Wired by the daemon; tests
	// set it explicitly where they need it.
	compiler *qir.Compiler
	// settledHook fires after a job reaches COMPLETED, on the worker
	// goroutine. It owns no job state — persistence already landed before it
	// runs. Failed and aborted jobs never fire it: there is no measured
	// result to fold. The hook must be fast and non-blocking (ledger writes
	// and pure folds only); a slow hook stalls the worker pool.
	settledHook func(*Job)
	// diskGuard freezes admission when set and blocking (S2 defense in
	// depth: the HTTP edge 503s first, but A2A/internal callers reach
	// SubmitJob directly). Nil means unguarded.
	diskGuard *disk.Guard
}

// SetDiskGuard attaches the S2 disk guard. Nil clears it. Safe to call
// before or after Start.
func (r *Runner) SetDiskGuard(g *disk.Guard) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.diskGuard = g
}

func (r *Runner) getDiskGuard() *disk.Guard {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.diskGuard
}

// SetCompiler registers the QIR compiler for pre-dispatch analysis.
// Nil clears it. Safe to call before or after Start.
func (r *Runner) SetCompiler(c *qir.Compiler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.compiler = c
}

func (r *Runner) getCompiler() *qir.Compiler {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.compiler
}

// SetSettledHook registers the post-completion callback (planner replan).
// Nil clears it. Safe to call before or after Start.
func (r *Runner) SetSettledHook(hook func(*Job)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settledHook = hook
}

func (r *Runner) getSettledHook() func(*Job) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settledHook
}

func NewRunner(l *ledger.Ledger, bridgeClient pb.QuantumBridgeServiceClient, workers int) *Runner {
	if workers <= 0 {
		workers = 4
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Runner{
		ledger:       l,
		bridgeClient: bridgeClient,
		jobQueue:     make(chan *Job, 1000),
		workers:      workers,
		ctx:          ctx,
		cancel:       cancel,
		aborted:      make(map[string]bool),
		tenantMgr:    tenants.NewManager(l),
		sessionMgr:   session.NewManager(l),
		fidelityGate: fidelity.NewFidelityGate(),
	}
}

func (r *Runner) Start() {
	for i := 0; i < r.workers; i++ {
		r.wg.Add(1)
		go r.workerLoop()
	}
}

func (r *Runner) Stop() {
	r.cancel()
	close(r.jobQueue)
	r.wg.Wait()
}

func (r *Runner) BridgeClient() pb.QuantumBridgeServiceClient {
	return r.bridgeClient
}

func (r *Runner) tenantStore() *tenants.Manager {
	return r.tenantMgr
}

func (r *Runner) SubmitJob(sessionID, jobType string, params any) (*Job, error) {
	return r.submitJob(sessionID, jobType, params, 0)
}

// SubmitJobWithDeclaredCost submits a job whose params schema carries no
// cost fields (A2A-mapped bridge tasks): spend is declared explicitly here
// instead. circuit_run keeps its params-embedded cost; a positive
// declaration for circuit_run is refused — one cost source per type.
func (r *Runner) SubmitJobWithDeclaredCost(sessionID, jobType string, params any, maxCostUSD float64) (*Job, error) {
	if jobType == "circuit_run" && maxCostUSD > 0 {
		return nil, fmt.Errorf("circuit_run carries cost in params: declared cost refused (one cost source)")
	}
	return r.submitJob(sessionID, jobType, params, maxCostUSD)
}

// mandateRefOf extracts the spend-mandate receipt reference from submit
// params (R2b): the admission-verified entry_hash the job settles
// against. Params without one (local/sim work) yield "" — unmandated
// jobs settle no obligation.
func mandateRefOf(data []byte) string {
	var p struct {
		MandateRef string `json:"mandate_ref"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}
	return p.MandateRef
}

func (r *Runner) submitJob(sessionID, jobType string, params any, declaredCost float64) (*Job, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}
	if !knownJobTypes[jobType] {
		return nil, fmt.Errorf("unknown job type %q: dispatch refused", jobType)
	}

	// S2 defense in depth: the HTTP edge 503s first, but internal callers
	// (A2A executor, tests-as-clients) reach here directly.
	if g := r.getDiskGuard(); g != nil && g.Blocks() {
		snap := g.Snapshot()
		return nil, fmt.Errorf("storage_blocked: %s — dispatch frozen until disk space recovers", snap.Reason)
	}

	// Structural QASM gate first (cheapest check): malformed programs
	// refuse before admission, queue, ledger, or spend moves.
	// Compile analysis runs here too when a compiler is wired: the
	// summary (or its fallback estimate, honestly labeled) persists on
	// the job row, so launch inherits compilation provenance free.
	var compilerUsed string
	var fallbackEstimate bool
	if jobType == "circuit_run" {
		var qp struct {
			QASM string `json:"qasm"`
		}
		if err := json.Unmarshal(data, &qp); err == nil && qp.QASM != "" {
			if err := ValidateQASM(qp.QASM); err != nil {
				return nil, err
			}
			if c := r.getCompiler(); c != nil {
				summary, aerr := c.Analyze(qp.QASM)
				// Defensive refuse branch, kept deliberately: Analyze is
				// total today (semantic validation never errors — malformed
				// input refuses earlier at validateQASM), so this never
				// fires. It stays because a future fallible Analyze must
				// refuse here, not admit. Do not delete as "dead code".
				if aerr != nil {
					return nil, fmt.Errorf("compile analysis refused: %w", aerr)
				}
				compilerUsed, fallbackEstimate = summary.CompilerUsed, summary.FallbackEstimate
			}
		} else if qp.QASM == "" {
			return nil, fmt.Errorf("circuit_run requires qasm")
		}
	}
	// Sweep grid gate: out-of-range scans refuse with no residue, same rule.
	if jobType == "pes_sweep" {
		if _, err := ValidateSweepParams(data); err != nil {
			return nil, err
		}
	}

	// Pre-QPU admission gate: autonomy + hardware/mandate rules. Refusals
	// happen here, before any queue or ledger write, so rejected jobs leave
	// no residue and cost nothing. Success PLANS spend (bound lab +
	// authorized amount, nothing debited yet); the plan is debited inside
	// the SAME SQLite transaction as the job-row INSERT below
	// (reserveAndInsertJobTx), so a crash between ReserveSpend and INSERT
	// cannot leak a reservation. All three books (tenants, sessions, jobs)
	// live in the one ledger SQLite file (see ledger.OpenLedger initSchema),
	// so a single transaction covers them — no marker rows, no reaper.
	resTenant, resAmount, err := r.planAdmission(sessionID, jobType, data, declaredCost)
	if err != nil {
		return nil, err
	}

	j := &Job{
		ID:               uuid.New().String(),
		SessionID:        sessionID,
		Status:           pb.JobStatus_JOB_STATUS_QUEUED,
		JobType:          jobType,
		ParamsJSON:       data,
		CreatedAt:        time.Now().UTC(),
		ReservedTenant:   resTenant,
		ReservedAmount:   resAmount,
		CompilerUsed:     compilerUsed,
		FallbackEstimate: fallbackEstimate,
		MandateRef:       mandateRefOf(data),
	}

	// money.FromUSD rejects NaN/Inf/out-of-range before anything moves.
	if _, merr := money.FromUSD(resAmount); merr != nil {
		return nil, fmt.Errorf("reservation amount: %w", merr)
	}
	if err := r.reserveAndInsertJobTx(j); err != nil {
		return nil, err
	}

	r.jobQueue <- j
	return j, nil
}

// reserveAndInsertJobTx debits the admission plan (tenant + session CAS
// guards) and persists the job row in ONE SQLite transaction, then seals the
// audit events. Commit is the atomic point: a crash before it rolls back
// BOTH the counters and the row (no leaked reservation, no orphan row); a
// crash after it leaves consistent books (the post-commit events are audit
// only and never move money twice — settle/release are CAS-guarded).
//
// The CAS UPDATEs replicate the manager discipline (tenants.ReserveSpend /
// session ReserveSpend) statement-for-statement inside the tx because those
// managers execute on the bare *sql.DB handle, not a tx. The single source
// of the guard predicates stays those managers; this tx mirrors them and
// names them. Failure maps to the same refusal vocabulary.
//
// Test hook: when testCrashBeforeCommit is true, the tx rolls back after the
// INSERT instead of committing, simulating a crash between reserve and
// insert. Production never sets it.
var testCrashBeforeCommit = false

func (r *Runner) reserveAndInsertJobTx(j *Job) error {
	resMicro, merr := money.FromUSD(j.ReservedAmount)
	if merr != nil {
		return fmt.Errorf("reservation amount: %w", merr)
	}
	db := r.ledger.DB()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin admission transaction: %w", err)
	}
	// Rollback is a no-op after a successful Commit.
	defer tx.Rollback()

	if int64(resMicro) > 0 {
		res, err := tx.Exec(
			`UPDATE tenants SET spent_micro_usd = spent_micro_usd + ? WHERE id = ? AND status = 'active' AND spent_micro_usd + ? <= spend_ceiling_micro_usd AND ? <= spend_ceiling_micro_usd`,
			int64(resMicro), j.ReservedTenant, int64(resMicro), int64(resMicro))
		if err != nil {
			return fmt.Errorf("spend reservation failed: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("spend reservation failed: %w", err)
		}
		if n == 0 {
			return r.mapTenantReserveFailureTx(tx, j.ReservedTenant)
		}
		res, err = tx.Exec(
			`UPDATE sessions SET current_spend_micro_usd = current_spend_micro_usd + ? WHERE id = ? AND (spend_limit_micro_usd <= 0 OR current_spend_micro_usd + ? <= spend_limit_micro_usd)`,
			int64(resMicro), j.SessionID, int64(resMicro))
		if err != nil {
			return fmt.Errorf("session spend reservation failed: %w", err)
		}
		n, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("session spend reservation failed: %w", err)
		}
		if n == 0 {
			return r.mapSessionReserveFailureTx(tx, j.SessionID)
		}
	}
	fallbackInt := 0
	if j.FallbackEstimate {
		fallbackInt = 1
	}
	query := `INSERT INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(query, j.ID, j.SessionID, j.Status.String(), j.JobType, string(j.ParamsJSON), j.CreatedAt, j.ReservedTenant, int64(resMicro), j.CompilerUsed, fallbackInt, j.MandateRef); err != nil {
		return fmt.Errorf("failed to save job to db: %w", err)
	}
	if testCrashBeforeCommit {
		// Simulated crash after reserve+insert, before commit: the
		// deferred Rollback unwinds both. Callers observe an error and
		// the books must show zero movement (proven by test).
		return fmt.Errorf("simulated crash between reserve and insert: rolled back")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit admission transaction: %w", err)
	}
	// Post-commit audit only (money already moved atomically).
	if int64(resMicro) > 0 {
		r.appendMoneyEvent(j.ReservedTenant, "tenant_spend_reserved", map[string]any{
			"tenant_id": j.ReservedTenant, "amount_usd": j.ReservedAmount})
		r.appendMoneyEvent(j.SessionID, "session_spend_reserved", map[string]any{
			"session_id": j.SessionID, "amount_usd": j.ReservedAmount})
	}
	r.appendMoneyEvent(j.SessionID, "job_queued", j)
	return nil
}

// mapTenantReserveFailureTx names a failed in-tx tenant debit with the same
// vocabulary as tenants.ReserveSpend (unknown / suspended / ceiling).
func (r *Runner) mapTenantReserveFailureTx(tx *sql.Tx, tenantID string) error {
	var status, id string
	if err := tx.QueryRow(`SELECT id, status FROM tenants WHERE id = ?`, tenantID).Scan(&id, &status); err != nil {
		return fmt.Errorf("unknown tenant %q: dispatch refused", tenantID)
	}
	if status != "active" {
		return tenants.ErrTenantSuspended
	}
	return tenants.ErrCeilingExceeded
}

// mapSessionReserveFailureTx names a failed in-tx session debit with the
// same vocabulary as session ReserveSpend (unknown session / limit).
func (r *Runner) mapSessionReserveFailureTx(tx *sql.Tx, sessionID string) error {
	var id string
	if err := tx.QueryRow(`SELECT id FROM sessions WHERE id = ?`, sessionID).Scan(&id); err != nil {
		return fmt.Errorf("unknown session %q: dispatch refused", sessionID)
	}
	return fmt.Errorf("session %q spend limit exceeded: dispatch refused", sessionID)
}

// RecordQuantumBridgeJob persists a job row in SQLite for a job dispatched
// via the Python QuantumBridgeService (POST /api/quantum/jobs — Phase 1
// single-book unification, F-NEW-05). The job is NOT placed on Go's internal
// worker channel (the Python JobRunner executes it), so it never double-runs;
// recording it in SQLite ensures DeleteBlockers, ListJobs, CancelSessionJobs,
// and releaseReservation operate on one unified table.
func (r *Runner) RecordQuantumBridgeJob(jobID, sessionID, provider, qasm string, shots int, tenantID string, reservedUSD float64, mandateRef string) error {
	if jobID == "" || sessionID == "" {
		return nil
	}
	paramsBytes, _ := json.Marshal(map[string]any{
		"qasm":              qasm,
		"requested_backend": provider,
		"shots":             shots,
		"max_cost_usd":      reservedUSD,
		"mandate_ref":       mandateRef,
		"origin":            "quantum_bridge_rpc",
	})
	now := time.Now().UTC()
	j := &Job{
		ID:             jobID,
		SessionID:      sessionID,
		Status:         pb.JobStatus_JOB_STATUS_QUEUED,
		JobType:        "circuit_run",
		ParamsJSON:     paramsBytes,
		CreatedAt:      now,
		ReservedTenant: tenantID,
		ReservedAmount: reservedUSD,
		MandateRef:     mandateRef,
	}
	resMicro, merr := money.FromUSD(reservedUSD)
	if merr != nil {
		return fmt.Errorf("bridge job reservation: %w", merr)
	}
	query := `INSERT OR IGNORE INTO jobs (id, session_id, status, job_type, params_json, created_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', 0, ?)`
	if _, err := r.ledger.DB().Exec(query, j.ID, j.SessionID, j.Status.String(), j.JobType, string(j.ParamsJSON), j.CreatedAt, j.ReservedTenant, int64(resMicro), j.MandateRef); err != nil {
		return fmt.Errorf("failed to record quantum bridge job: %w", err)
	}
	r.appendMoneyEvent(sessionID, "job_queued", j)
	return nil
}

// JobSession returns the session a persisted job belongs to. Quantum
// status/cancel use it to enforce ownership before touching the bridge;
// missing attribution refuses instead of skipping the tenant check.
func (r *Runner) JobSession(jobID string) (string, bool) {
	var sid string
	if err := r.ledger.DB().QueryRow(`SELECT session_id FROM jobs WHERE id = ?`, jobID).Scan(&sid); err != nil {
		return "", false
	}
	return sid, sid != ""
}

// SyncQuantumBridgeJobStatus reconciles a Python QuantumBridgeService job's
// status into SQLite jobs (F-NEW-05). Every terminal state (completed,
// failed, cancelled) settles the spend reservation against reported vendor
// usage — measured usage always wins; unknown usage keeps the estimate
// loudly (settlement_skipped_unknown_usage); explicit zero usage releases
// everything. Pre-vendor terminals (no hardware id, no usage — the vendor
// never ran) release the reservation.
func (r *Runner) SyncQuantumBridgeJobStatus(jobID, bridgeStatus, resultJSON string) {
	if jobID == "" {
		return
	}
	j, err := r.GetJob(jobID)
	if err != nil || j == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(bridgeStatus)) {
	case "running":
		_, _ = r.ledger.DB().Exec(
			`UPDATE jobs SET status = ? WHERE id = ? AND status = ?`,
			pb.JobStatus_JOB_STATUS_RUNNING.String(), jobID, pb.JobStatus_JOB_STATUS_QUEUED.String(),
		)
	case "completed":
		now := time.Now().UTC()
		res, err := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status `+liveStatusClause,
			append([]any{pb.JobStatus_JOB_STATUS_COMPLETED.String(), resultJSON, now, jobID}, liveStatusArgs...)...,
		)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				j.Status = pb.JobStatus_JOB_STATUS_COMPLETED
				j.ResultJSON = json.RawMessage(resultJSON)
				j.CompletedAt = &now
				// Settle the reservation down to measured usage when the
				// provider reports it; the estimate is a ceiling, not the bill.
				// P0: actual_usd is a POINTER — a completion LACKING actual_usd
				// unmarshals to nil (unknown), never to 0. Settling an unknown
				// 0 would refund the entire estimate as if nothing ran. Settle
				// ONLY when usage is present, finite, and within
				// 0 <= actual <= reserved; otherwise the reservation stands and
				// a settlement_skipped_unknown_usage event seals why.
				// Measured usage always wins — including on failure/cancel
				// terminals below; unknown usage keeps the estimate loudly
				// (extended Q-51: the old "hardware-id failure keeps the full
				// estimate" rule now applies ONLY when no usage was reported).
				r.settleTerminalFromResult(j, resultJSON)
				r.appendMoneyEvent(j.SessionID, "job_settled", j)
			}
		}
	case "failed", "cancelled":
		now := time.Now().UTC()
		res, err := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status `+liveStatusClause,
			append([]any{pb.JobStatus_JOB_STATUS_FAILED.String(), resultJSON, now, jobID}, liveStatusArgs...)...,
		)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				j.Status = pb.JobStatus_JOB_STATUS_FAILED
				j.ResultJSON = json.RawMessage(resultJSON)
				j.CompletedAt = &now
				// Failure/cancel settlement (extended Q-51): measured vendor
				// usage settles reserved→actual exactly like a completion
				// (CAS-guarded, single-flight, idempotent). Zero-usage
				// terminals release everything. A terminal with NO usage
				// report keeps the estimate and seals a settlement_skipped
				// event — except pre-vendor failures (no hardware id pinned:
				// the vendor never ran), which release.
				r.settleTerminalFromResult(j, resultJSON)
				r.appendMoneyEvent(j.SessionID, "job_settled", j)
			}
		}
	}
}

// terminalUsage is every shape a vendor result reports spend-relevant usage
// in. Providers carry actual_usd top-level (synchronous wait path) or nested
// under result (deferred poll path) — both settle identically. usage_seconds
// is the raw meter: an explicit 0 means the vendor reports nothing ran
// (release everything), while a missing reading means unknown (keep loudly).
// hardware_job_id marks post-vendor terminals (the vendor may have run).
type terminalUsage struct {
	ActualUSD     *float64 `json:"actual_usd"`
	UsageSeconds  *float64 `json:"usage_seconds"`
	HardwareJobID string   `json:"hardware_job_id"`
	Result        struct {
		ActualUSD    *float64 `json:"actual_usd"`
		UsageSeconds *float64 `json:"usage_seconds"`
	} `json:"result"`
}

// extractTerminalUsage parses a terminal result payload. ok=false means the
// payload is unreadable (unknown usage, never zero).
func extractTerminalUsage(resultJSON string) (actual *float64, usageSeconds *float64, hardwareJobID string, ok bool) {
	var u terminalUsage
	if err := json.Unmarshal([]byte(resultJSON), &u); err != nil {
		return nil, nil, "", false
	}
	actual = u.ActualUSD
	if actual == nil {
		actual = u.Result.ActualUSD
	}
	usageSeconds = u.UsageSeconds
	if usageSeconds == nil {
		usageSeconds = u.Result.UsageSeconds
	}
	return actual, usageSeconds, u.HardwareJobID, true
}

// settleTerminalFromResult settles one terminal job (any terminal status)
// from its reported vendor usage. It is the single settlement funnel for
// completed/failed/cancelled bridge terminals — one vocabulary, one CAS
// discipline:
//
//   - measured actual_usd (finite, >= 0, <= reserved): settleReservation
//     down to actual (actual == 0 settles to zero = full release);
//   - actual_usd above the reservation or non-finite/negative: keep the
//     estimate + seal settlement_skipped with the cause;
//   - no actual_usd but explicit usage_seconds == 0: the vendor reports
//     nothing ran — release everything;
//   - no usage at all: pre-vendor terminals (no hardware id — the vendor
//     never ran) release; post-vendor terminals keep the estimate and seal
//     settlement_skipped (actual_usd_missing), because the hardware may
//     have run despite reporting nothing.
//
// No reservation (reserved <= 0) is a no-op. settleReservation and
// releaseReservation are CAS-guarded, so concurrent terminal racers settle
// exactly once.
func (r *Runner) settleTerminalFromResult(j *Job, resultJSON string) {
	if j == nil {
		return
	}
	actual, usageSeconds, hardwareJobID, ok := extractTerminalUsage(resultJSON)
	if j.ReservedAmount <= 0 && !ok {
		return
	}
	if !ok {
		if hardwareJobID != "" {
			r.emitSettlementSkipped(j, "result_unreadable")
			return
		}
		r.releaseReservation(j.ID)
		return
	}
	if actual != nil {
		a := *actual
		if math.IsNaN(a) || math.IsInf(a, 0) || a < 0 {
			r.emitSettlementSkipped(j, "actual_usd_not_finite_nonnegative")
			return
		}
		if j.ReservedAmount > 0 && a > j.ReservedAmount {
			r.emitSettlementSkipped(j, "actual_usd_above_reserved")
			return
		}
		// 0 <= actual <= reserved (or unreserved): settle down to measured.
		r.settleReservation(j, a)
		return
	}
	if usageSeconds != nil && *usageSeconds == 0 {
		r.releaseReservation(j.ID)
		return
	}
	if hardwareJobID != "" {
		r.emitSettlementSkipped(j, "actual_usd_missing")
		return
	}
	r.releaseReservation(j.ID)
}

func (r *Runner) workerLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		case j, ok := <-r.jobQueue:
			if !ok {
				return
			}
			r.executeJob(j)
		}
	}
}

func (r *Runner) executeJob(j *Job) {
	if r.isAborted(j.ID) {
		r.failJob(j, "aborted by supervisor before execution")
		return
	}

	// Update status to running
	j.Status = pb.JobStatus_JOB_STATUS_RUNNING
	r.ledger.DB().Exec(`UPDATE jobs SET status = ? WHERE id = ?`, j.Status.String(), j.ID)
	r.ledger.AppendEvent(j.SessionID, "job_started", j)

	var resultData []byte
	var execErr error

	if r.bridgeClient == nil {
		execErr = fmt.Errorf("bridge client unavailable: no %s computation performed", j.JobType)
	} else {
		switch j.JobType {
		case "circuit_run":
			var req pb.CircuitExecutionRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			req.JobId = j.ID
			req.SessionId = j.SessionID
			resp, err := r.bridgeClient.ExecuteCircuit(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				// W1.8: bridge-reported failure settles failed, never
				// completed with a zero payload (every RPC type below).
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "circuit_cut":
			var req pb.CircuitCutRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			resp, err := r.bridgeClient.CutCircuit(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "classical_anchor":
			var req pb.ClassicalAnchorRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			resp, err := r.bridgeClient.ComputeClassicalAnchor(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "claim_eval":
			var req pb.ClaimEvaluationRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			resp, err := r.bridgeClient.EvaluateWavefunction(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "krylov_solve":
			var req pb.QuantumKrylovRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			resp, err := r.bridgeClient.ExecuteQuantumKrylov(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "mitigate":
			var req pb.MitigationRequest
			if err := json.Unmarshal(j.ParamsJSON, &req); err != nil {
				execErr = err
				break
			}
			resp, err := r.bridgeClient.MitigateCircuit(r.ctx, &req)
			if err != nil {
				execErr = err
			} else if !resp.Success {
				execErr = fmt.Errorf("bridge reports failure: %s", resp.ErrorMessage)
			} else {
				resultData, _ = json.Marshal(resp)
			}

		case "pes_sweep":
			var sp PesSweepParams
			if err := json.Unmarshal(j.ParamsJSON, &sp); err != nil {
				execErr = err
				break
			}
			if _, err := ValidateSweepParams(j.ParamsJSON); err != nil {
				execErr = err
				break
			}
			resultData, execErr = r.executePesSweep(&sp)

		default:
			execErr = fmt.Errorf("unknown job type: %s", j.JobType)
		}
	}

	now := time.Now().UTC()
	j.CompletedAt = &now
	if execErr != nil {
		r.failJob(j, execErr.Error())
		return
	}
	// S6: bound persisted results. Bridge/sweep payloads are structured,
	// so oversize blobs refuse (fail-closed, auditable) rather than
	// truncate into unparseable JSON. Sandbox streams are bounded
	// separately at parseHarnessOutput (truncation with flags).
	if len(resultData) > MaxStoredResultBytes {
		r.failJob(j, fmt.Sprintf("result_too_large: %d bytes exceeds %d cap — refusing to persist an unbounded blob", len(resultData), MaxStoredResultBytes))
		return
	}
	// An abort that landed mid-flight wins over the late completion, but the
	// computed result is preserved for provenance instead of being dropped.
	// Settlement funnels through settleTerminalFromResult like every other
	// terminal: measured usage wins, zero usage releases, unknown pre-vendor
	// usage releases, unknown post-vendor usage keeps loudly.
	if r.isAborted(j.ID) {
		j.Status = pb.JobStatus_JOB_STATUS_FAILED
		if len(resultData) > 0 {
			j.ResultJSON = resultData
		} else {
			errRes, _ := json.Marshal(map[string]string{"error": "aborted by supervisor during execution"})
			j.ResultJSON = errRes
		}
		r.settleTerminalFromResult(j, string(j.ResultJSON))
	} else {
		j.Status = pb.JobStatus_JOB_STATUS_COMPLETED
		j.ResultJSON = resultData
	}

	r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ?`,
		j.Status.String(), string(j.ResultJSON), j.CompletedAt, j.ID,
	)
	r.ledger.AppendEvent(j.SessionID, "job_settled", j)
	if j.Status == pb.JobStatus_JOB_STATUS_COMPLETED {
		if hook := r.getSettledHook(); hook != nil {
			hook(j)
		}
	}
}

// releaseReservation returns a job's bound spend to its lab. Idempotent:
// it zeroes the row's reservation first, so double-release (cancel racing
// worker settlement) can never mint spend back twice. Completed jobs never
// call this — kept reservations are the spend.
func (r *Runner) releaseReservation(jobID string) {
	var tenant, sessionID string
	var amountMicro int64
	err := r.ledger.DB().QueryRow(
		`SELECT reserved_tenant, reserved_amount_micro_usd, session_id FROM jobs WHERE id = ?`, jobID).Scan(&tenant, &amountMicro, &sessionID)
	if err != nil || tenant == "" || amountMicro <= 0 {
		return
	}
	amount := money.MicroUSD(amountMicro).USD()
	res, err := r.ledger.DB().Exec(
		`UPDATE jobs SET reserved_amount_micro_usd = 0 WHERE id = ? AND reserved_amount_micro_usd > 0`, jobID)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return // already released
	}
	_ = r.tenantStore().ReleaseSpend(tenant, amount)
	_ = r.sessionMgr.ReleaseSpend(sessionID, amount)
}

// appendMoneyEvent seals a money/provenance event; on ledger failure it
// writes the event to the durable provenance spool. Both failing is logged
// loudly, but the money movement itself is already atomic in SQLite, so it
// does not turn a committed admission/settlement into a rollback.
func (r *Runner) appendMoneyEvent(sessionID, eventType string, payload any) {
	if _, err := r.ledger.AppendEventWithSpool(sessionID, eventType, payload); err != nil {
		slog.Error("[jobs] money event lost (ledger and spool failed)", "event", eventType, "session_id", sessionID, "error", err)
	}
}

// emitSettlementSkipped seals WHY a terminal job kept its full estimate
// reservation (P0 backfill: unknown usage never refunds). The reservation
// stands — this event is the audit trail, keyed by the shared taxonomy
// const (never a copied literal). The reason vocabulary is shared across
// all terminal states (completed/failed/cancelled): result_unreadable,
// actual_usd_missing, actual_usd_not_finite_nonnegative,
// actual_usd_above_reserved.
func (r *Runner) emitSettlementSkipped(j *Job, reason string) {
	if j == nil {
		return
	}
	r.appendMoneyEvent(j.SessionID, ledger.SettlementSkippedUnknownUsageEvent, map[string]any{
		"job_id":              j.ID,
		"reserved_amount_usd": j.ReservedAmount,
		"reserved_tenant":     j.ReservedTenant,
		"reason":              reason,
	})
}

// settleReservation releases the difference between the estimate reservation
// and measured usage. It is a no-op when usage is unknown or >= the reserve;
// a CAS prevents a double release racing a cancel/settle.
func (r *Runner) settleReservation(j *Job, actualUSD float64) {
	if j == nil || actualUSD < 0 || math.IsNaN(actualUSD) || math.IsInf(actualUSD, 0) {
		return
	}
	var tenant, sessionID string
	var reservedMicro int64
	if err := r.ledger.DB().QueryRow(
		`SELECT reserved_tenant, reserved_amount_micro_usd, session_id FROM jobs WHERE id = ?`, j.ID).
		Scan(&tenant, &reservedMicro, &sessionID); err != nil || reservedMicro <= 0 {
		return
	}
	actualMicro, err := money.FromUSD(actualUSD)
	if err != nil || int64(actualMicro) >= reservedMicro {
		return
	}
	res, err := r.ledger.DB().Exec(
		`UPDATE jobs SET reserved_amount_micro_usd = ? WHERE id = ? AND reserved_amount_micro_usd = ?`,
		int64(actualMicro), j.ID, reservedMicro)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return
	}
	j.ReservedAmount = money.MicroUSD(actualMicro).USD()
	refund := money.MicroUSD(reservedMicro - int64(actualMicro)).USD()
	if tenant != "" {
		_ = r.tenantStore().ReleaseSpend(tenant, refund)
	}
	if sessionID != "" {
		_ = r.sessionMgr.ReleaseSpend(sessionID, refund)
	}
}

// failJob records a failed job with a reason string. Settlement funnels
// through settleTerminalFromResult like every other terminal: a bare error
// receipt carries no usage and no hardware id, so pre-vendor failures
// release (the vendor never ran); a result that reports measured usage
// settles reserved→actual even on this path.
func (r *Runner) failJob(j *Job, reason string) {
	now := time.Now().UTC()
	j.CompletedAt = &now
	j.Status = pb.JobStatus_JOB_STATUS_FAILED
	errRes, _ := json.Marshal(map[string]string{"error": reason})
	j.ResultJSON = errRes
	r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ?`,
		j.Status.String(), string(j.ResultJSON), j.CompletedAt, j.ID,
	)
	r.ledger.AppendEvent(j.SessionID, "job_settled", j)
	r.settleTerminalFromResult(j, string(j.ResultJSON))
}

// settleCancelledJob settles one job cancelled via the supervisor paths
// (CancelJob/CancelSessionJobs/CancelTenantJobs) from its PRE-CANCEL state:
// prev carries the snapshot read before the terminal UPDATE overwrote
// result_json, so vendor usage or a pinned hardware id is still visible.
// Measured usage settles; zero usage releases; unknown pre-vendor usage
// releases (the vendor never ran); unknown post-vendor usage keeps the
// estimate loudly. CAS-guarded — a worker that settles first wins and this
// is a no-op.
func (r *Runner) settleCancelledJob(prev *Job) {
	if prev == nil {
		return
	}
	r.settleTerminalFromResult(prev, string(prev.ResultJSON))
}

func (r *Runner) isAborted(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.aborted[id]
}

// CancelTenantJobs aborts every unsettled job belonging to any session of a
// lab tenant (revocation propagation for tenant autonomy revocation). It
// shares the abort-registry semantics of CancelSessionJobs.
func (r *Runner) CancelTenantJobs(tenantID, reason string) (int, error) {
	rows, err := r.ledger.DB().Query(
		`SELECT jobs.id FROM jobs JOIN sessions ON jobs.session_id = sessions.id WHERE sessions.tenant_id = ? AND jobs.status `+liveStatusClause,
		append([]any{tenantID}, liveStatusArgs...)...,
	)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()

	now := time.Now().UTC()
	errRes, _ := json.Marshal(map[string]string{"error": reason})
	for _, id := range ids {
		prev, _ := r.GetJob(id)
		r.mu.Lock()
		r.aborted[id] = true
		r.mu.Unlock()
		res, _ := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status `+liveStatusClause,
			append([]any{pb.JobStatus_JOB_STATUS_FAILED.String(), string(errRes), now, id}, liveStatusArgs...)...,
		)
		if n, _ := res.RowsAffected(); n > 0 {
			r.settleCancelledJob(prev) // terminal-failed here: settle from pre-cancel state
		}
	}
	r.ledger.AppendEvent(tenantID, "tenant_jobs_aborted", map[string]any{
		"tenant_id":     tenantID,
		"aborted_jobs":  ids,
		"aborted_count": len(ids),
		"reason":        reason,
	})
	return len(ids), nil
}

// CancelSessionJobs aborts every unsettled job for a session (Ring-0 reflex
// intercept). It marks the rows failed with the given reason, registers the
// IDs so in-flight workers settle failed rather than completed, and seals a
// supervisor_abort receipt in the ledger. It returns the aborted count.
func (r *Runner) CancelSessionJobs(sessionID, reason string) (int, error) {
	rows, err := r.ledger.DB().Query(
		`SELECT id FROM jobs WHERE session_id = ? AND status `+liveStatusClause,
		append([]any{sessionID}, liveStatusArgs...)...,
	)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()

	now := time.Now().UTC()
	errRes, _ := json.Marshal(map[string]string{"error": reason})
	for _, id := range ids {
		prev, _ := r.GetJob(id)
		r.mu.Lock()
		r.aborted[id] = true
		r.mu.Unlock()
		res, _ := r.ledger.DB().Exec(
			`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status `+liveStatusClause,
			append([]any{pb.JobStatus_JOB_STATUS_FAILED.String(), string(errRes), now, id}, liveStatusArgs...)...,
		)
		if n, _ := res.RowsAffected(); n > 0 {
			r.settleCancelledJob(prev) // terminal-failed here: settle from pre-cancel state
		}
	}
	r.ledger.AppendEvent(sessionID, "supervisor_abort", map[string]any{
		"session_id":    sessionID,
		"aborted_jobs":  ids,
		"aborted_count": len(ids),
		"reason":        reason,
	})
	return len(ids), nil
}

// CancelJob cancels one unsettled job (S7): QUEUED jobs never execute;
// RUNNING jobs are fenced via the abort registry so a late completion
// cannot overwrite the cancellation. Terminal jobs replay success
// (already-terminal is a stable answer, not an error); unknown ids
// refuse. Spend settles from the pre-cancel state exactly once via the CAS
// guard (measured usage wins; unknown post-vendor usage keeps loudly).
func (r *Runner) CancelJob(id, reason string) (transitioned bool, err error) {
	if reason == "" {
		reason = "cancelled by operator"
	}
	j, err := r.GetJob(id)
	if err != nil {
		return false, fmt.Errorf("job not found: %s", id)
	}
	switch j.Status {
	case pb.JobStatus_JOB_STATUS_COMPLETED, pb.JobStatus_JOB_STATUS_FAILED:
		return false, nil
	}
	r.mu.Lock()
	r.aborted[id] = true
	r.mu.Unlock()
	now := time.Now().UTC()
	errRes, _ := json.Marshal(map[string]string{"error": reason, "cancelled": "true"})
	res, err := r.ledger.DB().Exec(
		`UPDATE jobs SET status = ?, result_json = ?, completed_at = ? WHERE id = ? AND status `+liveStatusClause,
		append([]any{pb.JobStatus_JOB_STATUS_FAILED.String(), string(errRes), now, id}, liveStatusArgs...)...,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		// Lost the race with the worker (it settled first): report the
		// terminal state, not a phantom cancellation.
		return false, nil
	}
	r.settleCancelledJob(j)
	// The receipt is load-bearing (audit of who cancelled what): a
	// cancellation that cannot be recorded fails loudly so the client
	// retries into the already-terminal replay — never a silent loss.
	if err := r.ledger.AppendEvent(j.SessionID, "job_cancelled", map[string]any{
		"job_id": id, "reason": reason,
	}); err != nil {
		return true, fmt.Errorf("job cancelled but receipt failed: %w (retry: already-terminal replay)", err)
	}
	return true, nil
}

func (r *Runner) GetJob(id string) (*Job, error) {
	query := `SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref FROM jobs WHERE id = ?`
	row := r.ledger.DB().QueryRow(query, id)

	var j Job
	var statusStr, paramsStr string
	var resultStr sql.NullString
	var compAt sql.NullTime
	var resTenant sql.NullString
	var resAmount sql.NullInt64
	var compilerUsed sql.NullString
	var fallbackInt sql.NullInt64
	var mandateRef sql.NullString

	if err := row.Scan(&j.ID, &j.SessionID, &statusStr, &j.JobType, &paramsStr, &resultStr, &j.CreatedAt, &compAt, &resTenant, &resAmount, &compilerUsed, &fallbackInt, &mandateRef); err != nil {
		return nil, err
	}
	j.Status = pb.JobStatus(pb.JobStatus_value[statusStr])
	j.ParamsJSON = json.RawMessage(paramsStr)
	if resultStr.Valid {
		j.ResultJSON = json.RawMessage(resultStr.String)
	}
	if compAt.Valid {
		t := compAt.Time
		j.CompletedAt = &t
	}
	if resTenant.Valid {
		j.ReservedTenant = resTenant.String
	}
	if resAmount.Valid {
		if resAmount.Valid {
			j.ReservedAmount = money.MicroUSD(resAmount.Int64).USD()
		}
	}
	if compilerUsed.Valid {
		j.CompilerUsed = compilerUsed.String
	}
	j.FallbackEstimate = fallbackInt.Valid && fallbackInt.Int64 != 0
	if mandateRef.Valid {
		j.MandateRef = mandateRef.String
	}
	return &j, nil
}

func (r *Runner) ListJobs(sessionID string, limit int) ([]*Job, error) {
	// W2.2: tenant scoping — an empty session would enumerate every lab's
	// jobs (cross-tenant read). Refuse instead of listing all tenants;
	// callers must name the session they may read.
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required: refusing unscoped job list (would cross tenants)")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if sessionID != "" {
		rows, err = r.ledger.DB().Query(
			`SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref FROM jobs WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`,
			sessionID, limit,
		)
	} else {
		rows, err = r.ledger.DB().Query(
			`SELECT id, session_id, status, job_type, params_json, result_json, created_at, completed_at, reserved_tenant, reserved_amount_micro_usd, compiler_used, fallback_estimate, mandate_ref FROM jobs ORDER BY created_at DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobsList []*Job
	for rows.Next() {
		var j Job
		var statusStr, paramsStr string
		var resultStr sql.NullString
		var compAt sql.NullTime
		var resTenant sql.NullString
		var resAmount sql.NullInt64
		var compilerUsed sql.NullString
		var fallbackInt sql.NullInt64
		var mandateRef sql.NullString
		if err := rows.Scan(&j.ID, &j.SessionID, &statusStr, &j.JobType, &paramsStr, &resultStr, &j.CreatedAt, &compAt, &resTenant, &resAmount, &compilerUsed, &fallbackInt, &mandateRef); err != nil {
			return nil, err
		}
		j.Status = pb.JobStatus(pb.JobStatus_value[statusStr])
		j.ParamsJSON = json.RawMessage(paramsStr)
		if resultStr.Valid {
			j.ResultJSON = json.RawMessage(resultStr.String)
		}
		if compAt.Valid {
			t := compAt.Time
			j.CompletedAt = &t
		}
		if resTenant.Valid {
			j.ReservedTenant = resTenant.String
		}
		if resAmount.Valid {
			if resAmount.Valid {
				j.ReservedAmount = money.MicroUSD(resAmount.Int64).USD()
			}
		}
		if compilerUsed.Valid {
			j.CompilerUsed = compilerUsed.String
		}
		j.FallbackEstimate = fallbackInt.Valid && fallbackInt.Int64 != 0
		if mandateRef.Valid {
			j.MandateRef = mandateRef.String
		}
		jobsList = append(jobsList, &j)
	}
	return jobsList, nil
}
