package jobs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/grants"
	"github.com/mainza-ai/kubona/core/pkg/mandates"
	"github.com/mainza-ai/kubona/core/pkg/tenants"
)

// simulatorBackends are zero-cost local execution targets. Admission lets
// them through on autonomy alone. Every other backend string names metered
// hardware and additionally requires a verified spend-mandate receipt.
var simulatorBackends = map[string]bool{
	"": true, "local_simulator": true, "local-simulator": true,
	"aer": true, "qiskit-aer": true, "qiskit-aer-simulator": true,
	"statevector": true, "statevector_simulator": true,
	"gpu_simulator": true, "gpu-simulator": true,
}

func isSimulatorBackend(backend string) bool {
	return simulatorBackends[strings.ToLower(strings.TrimSpace(backend))]
}

type circuitRunParams struct {
	QASM             string   `json:"qasm"`
	RequestedBackend string   `json:"requested_backend"`
	MandateRef       string   `json:"mandate_ref"`
	MaxCostUSD       float64  `json:"max_cost_usd"`
	InputLabels      []string `json:"input_labels"`
	GrantChain       []string `json:"grant_chain"`
	// P5 §2.9 routing: caller-declared jurisdictions + export-control class.
	// Presence is enforced here at admission; the semantic route() verdict
	// (intersection against the provider capability) is enforced by the
	// Python executor holding the schemas, fed through the bridge wire.
	CallerJurisdictions []string `json:"caller_jurisdictions"`
	CallerExportClass   string   `json:"caller_export_class"`
	ExportReviewed      bool     `json:"export_reviewed"`
}

type mandateReceipt struct {
	TenantID        string  `json:"tenant_id"`
	JobHash         string  `json:"job_hash"`
	MaxBudgetUSD    float64 `json:"max_budget_usd"`
	ExpiryTimestamp string  `json:"expiry_timestamp"`
}

// admitJob is the pre-QPU admission gate, enforced inside SubmitJob so every
// entry point (HTTP, A2A, autoresearch) passes it. Order: tenant autonomy
// first (L0/suspended/unknown refuse everything), then the hardware rule:
// simulator-class backends proceed, QPU-destined jobs must cite a verified,
// unexpired, budget-covering spend-mandate ledger receipt.
// On success it returns the spend reservation (bound tenant + authorized
// amount), already debited: SubmitJob persists it on the job row, and every
// terminal-failed path releases it. Completed jobs keep it — the authorized
// amount is the spend.
func (r *Runner) admitJob(sessionID, jobType string, paramsJSON []byte, declaredCost float64) (string, float64, error) {
	var tenantID string
	err := r.ledger.DB().QueryRow(`SELECT tenant_id FROM sessions WHERE id = ?`, sessionID).Scan(&tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", 0, fmt.Errorf("unknown session %q: dispatch refused", sessionID)
		}
		return "", 0, fmt.Errorf("session lookup failed: %w", err)
	}
	if tenantID == "" {
		tenantID = tenants.DefaultTenantID
	}
	t, err := r.tenantStore().GetTenant(tenantID)
	if err != nil {
		return "", 0, fmt.Errorf("unknown tenant %q: dispatch refused", tenantID)
	}
	if t.Status != "active" {
		return "", 0, fmt.Errorf("tenant %q is %s: dispatch refused", t.ID, t.Status)
	}
	if t.AutonomyLevel == tenants.LevelL0 {
		return "", 0, fmt.Errorf("tenant %q autonomy L0 (observe/suggest only): dispatch refused", t.ID)
	}

	if jobType != "circuit_run" {
		return r.admitNonCircuitRun(tenantID, sessionID, jobType, paramsJSON, declaredCost)
	}
	var p circuitRunParams
	if err := json.Unmarshal(paramsJSON, &p); err != nil {
		return "", 0, fmt.Errorf("invalid circuit_run params: %w", err)
	}
	// W0.7: cost is load-bearing from here on (reservations, mandate
	// cover). NaN/Inf/negative refuse for every circuit_run; zero refuses
	// for metered hardware below (a $0 reservation is unattributed spend).
	if math.IsNaN(p.MaxCostUSD) || math.IsInf(p.MaxCostUSD, 0) || p.MaxCostUSD < 0 {
		return "", 0, fmt.Errorf("invalid max_cost_usd: dispatch refused")
	}
	// Cross-lab UCAN grant path: a verified chain authorizing this tenant
	// replaces the mandate receipt and admits foreign-tenant input labels it
	// covers with read scope. Spend binds to the granting lab's ceiling.
	var grant *grants.VerifiedGrant
	if len(p.GrantChain) > 0 {
		var err error
		grant, err = r.verifyGrantChain(p.GrantChain, tenantID, grants.ScopeQPUSubmit)
		if err != nil {
			return "", 0, fmt.Errorf("grant rejected: %w", err)
		}
		if err := r.tenantStore().CheckSpend(grant.GrantingTenant, p.MaxCostUSD); err != nil {
			return "", 0, fmt.Errorf("granting lab ceiling: %w", err)
		}
		if p.MaxCostUSD > grant.MaxBudgetUSD {
			return "", 0, fmt.Errorf("job cost %.4f exceeds grant budget %.4f: dispatch refused", p.MaxCostUSD, grant.MaxBudgetUSD)
		}
	}
	// Confidential-input admission: every cited input label must already be
	// readable by the job's lab tenant, or be covered by a read grant naming
	// that foreign tenant. Anything else refuses (no silent laundering).
	for _, label := range p.InputLabels {
		if label == "public" || label == "tenant/"+tenantID {
			continue
		}
		admitted := false
		if grant != nil && hasScope(grant.Scopes, grants.ScopeRead) && strings.HasPrefix(label, "tenant/") {
			foreign := strings.TrimPrefix(label, "tenant/")
			admitted = foreign == grant.TargetTenant || foreign == grant.GrantingTenant
		}
		if !admitted {
			return "", 0, fmt.Errorf("input label %q not readable by tenant %q: dispatch refused", label, tenantID)
		}
	}
	if isSimulatorBackend(p.RequestedBackend) {
		return "", 0, nil
	}
	// Metered hardware with no declared positive cost refuses: ReserveSpend
	// treats 0 as a no-op, so admitting here would dispatch unattributed.
	if p.MaxCostUSD <= 0 {
		return "", 0, fmt.Errorf("backend %q is metered hardware: max_cost_usd must be positive (got %.4f)", p.RequestedBackend, p.MaxCostUSD)
	}
	// Routing declaration (P5 §2.9): metered hardware without declared caller
	// jurisdictions refuses here — the executor cannot verify a route it was
	// never told, and undeclared callers must not reach for hardware.
	if len(p.CallerJurisdictions) == 0 {
		return "", 0, fmt.Errorf("backend %q is metered hardware: caller_jurisdictions must declare at least one jurisdiction (routing cannot be verified undeclared)", p.RequestedBackend)
	}
	// Fidelity gate (Ring 3, second rail beside money): predict whether this
	// run can produce a publishable number BEFORE spending. Physics does not
	// care who pays, so this runs before grant/mandate checks. Only ADMIT
	// proceeds; mitigate/downgrade verdicts refuse with the recommended
	// action stated — remediation changes what was approved and must be
	// resubmitted explicitly, never applied silently.
	if err := r.admitFidelity(sessionID, p.RequestedBackend, p.QASM); err != nil {
		return "", 0, err
	}
	if grant != nil {
		// Spend binds to the granting lab's ceiling: reserve before admitting.
		if err := r.tenantStore().ReserveSpend(grant.GrantingTenant, p.MaxCostUSD); err != nil {
			return "", 0, err
		}
		if err := r.sessionMgr.ReserveSpend(sessionID, p.MaxCostUSD); err != nil {
			_ = r.tenantStore().ReleaseSpend(grant.GrantingTenant, p.MaxCostUSD)
			return "", 0, err
		}
		return grant.GrantingTenant, p.MaxCostUSD, nil
	}
	if p.MandateRef == "" {
		return "", 0, fmt.Errorf("backend %q is metered hardware: a verified spend-mandate receipt (mandate_ref) is required", p.RequestedBackend)
	}
	// W0.4: the receipt must bind THIS program in THIS session — recompute
	// the canonical binding from the submission, never trust a bare ref.
	if err := r.verifyMandateReceipt(p.MandateRef, tenantID, p.MaxCostUSD, mandates.CanonicalJobBinding("circuit_run", sessionID, p.QASM)); err != nil {
		return "", 0, err
	}
	if err := r.tenantStore().ReserveSpend(tenantID, p.MaxCostUSD); err != nil {
		return "", 0, err
	}
	if err := r.sessionMgr.ReserveSpend(sessionID, p.MaxCostUSD); err != nil {
		_ = r.tenantStore().ReleaseSpend(tenantID, p.MaxCostUSD)
		return "", 0, err
	}
	return tenantID, p.MaxCostUSD, nil
}

// jobCostEnvelope is the explicit spend declaration for job types whose
// params schemas carry no cost fields (A2A-mapped bridge tasks). Map-shaped
// params may embed it; proto-shaped params cannot, so those callers use
// SubmitJobWithDeclaredCost instead.
type jobCostEnvelope struct {
	MaxCostUSD float64 `json:"max_cost_usd"`
}

// admitNonCircuitRun rails bridge-compute jobs (W0.2): tenant autonomy
// already passed above; the caller must declare an explicit positive
// max_cost_usd, reserved here against tenant + session exactly like
// circuit_run. No mandate receipt and no fidelity floor: those gate QPU
// dispatch, and these types never touch hardware — the exemption is
// recorded here, not silent, and the reservation is the receipt.
func (r *Runner) admitNonCircuitRun(tenantID, sessionID, jobType string, paramsJSON []byte, declaredCost float64) (string, float64, error) {
	maxCost := declaredCost
	if maxCost <= 0 {
		var c jobCostEnvelope
		if err := json.Unmarshal(paramsJSON, &c); err == nil {
			maxCost = c.MaxCostUSD
		}
	}
	if math.IsNaN(maxCost) || math.IsInf(maxCost, 0) || maxCost <= 0 {
		return "", 0, fmt.Errorf("%s declares no positive max_cost_usd: dispatch refused", jobType)
	}
	if err := r.tenantStore().ReserveSpend(tenantID, maxCost); err != nil {
		return "", 0, err
	}
	if err := r.sessionMgr.ReserveSpend(sessionID, maxCost); err != nil {
		_ = r.tenantStore().ReleaseSpend(tenantID, maxCost)
		return "", 0, err
	}
	return tenantID, maxCost, nil
}

// verifyMandateReceipt checks a cited mandate verification event: right
// tenant, budget covering the job cost, unexpired, AND bound to the exact
// submitted program (W0.4: expectedJobHash is the canonical binding
// recomputed from the submission — a receipt for another program refuses
// even when tenant and budget cover).
func (r *Runner) verifyMandateReceipt(entryHash, tenantID string, costUSD float64, expectedJobHash string) error {
	var eventType, payloadStr string
	err := r.ledger.DB().QueryRow(
		`SELECT event_type, payload FROM ledger_events WHERE entry_hash = ? AND event_type IN ('mandate_verified_client', 'mandate_verified_pqc_mlkem768')`,
		entryHash).Scan(&eventType, &payloadStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("mandate receipt %q not found: QPU dispatch refused", entryHash)
		}
		return fmt.Errorf("mandate receipt lookup failed: %w", err)
	}
	var rec mandateReceipt
	if err := json.Unmarshal([]byte(payloadStr), &rec); err != nil {
		return fmt.Errorf("mandate receipt unreadable: %w", err)
	}
	if rec.TenantID != tenantID {
		return fmt.Errorf("mandate receipt belongs to tenant %q: dispatch refused", rec.TenantID)
	}
	if rec.JobHash != expectedJobHash {
		return fmt.Errorf("mandate receipt binds a different program (job_hash mismatch): dispatch refused")
	}
	if costUSD > rec.MaxBudgetUSD {
		return fmt.Errorf("job cost %.4f exceeds mandate budget %.4f: dispatch refused", costUSD, rec.MaxBudgetUSD)
	}
	if rec.ExpiryTimestamp != "" {
		expiry, err := time.Parse(time.RFC3339, rec.ExpiryTimestamp)
		if err != nil || !time.Now().UTC().Before(expiry) {
			return fmt.Errorf("mandate receipt expired: dispatch refused")
		}
	}
	return nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// fidelityDeviceForBackend maps a requested backend to a calibrated fidelity
// profile. Unknown hardware refuses as unpredictable — it must never inherit
// a simulator profile (which would admit everything at fidelity ~1.0).
func fidelityDeviceForBackend(backend string) (string, error) {
	low := strings.ToLower(strings.TrimSpace(backend))
	switch {
	case strings.HasPrefix(low, "ibm"):
		return "ibm_heron", nil
	case strings.HasPrefix(low, "rigetti"):
		return "rigetti_aspen", nil
	case strings.HasPrefix(low, "quantinuum"):
		return "quantinuum_h1", nil
	default:
		return "", fmt.Errorf("no fidelity profile for backend %q: unpredictable on this device — refused (register a calibrated profile first)", backend)
	}
}

var (
	qasmQregRe = regexp.MustCompile(`qreg\s+\w+\[(\d+)\]`)
	qasmTwoQRe = regexp.MustCompile(`^(cx|cz|swap|cp|ecr|rzz)\b`)
	qasmSkipRe = regexp.MustCompile(`^(OPENQASM|include|qreg|creg|measure|barrier|reset|opaque|//|$)`)
)

// countQasmGates estimates (qubits, serial depth, two-qubit count) from
// OpenQASM 2.0 source. Serial depth overestimates parallel circuits, which is
// the safe direction for a fidelity floor (predicts lower, refuses earlier).
func countQasmGates(qasm string) (qubits, depth, twoQ int) {
	for _, line := range strings.Split(qasm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || qasmSkipRe.MatchString(line) {
			if m := qasmQregRe.FindStringSubmatch(line); len(m) > 1 {
				var n int
				fmt.Sscanf(m[1], "%d", &n)
				if n > qubits {
					qubits = n
				}
			}
			continue
		}
		if !strings.HasSuffix(line, ";") {
			continue
		}
		depth++
		if qasmTwoQRe.MatchString(line) {
			twoQ++
		}
	}
	return qubits, depth, twoQ
}

// admitFidelity enforces the fidelity floor for metered-hardware jobs.
// Every assessment — admit AND refuse — is sealed into the ledger first:
// refused assessments are the audit trail of denied hardware spend, and the
// series is the raw material for drift curves. A ledger that cannot record
// refuses admission (no unattested spend).
func (r *Runner) admitFidelity(sessionID, backend, qasm string) error {
	device, derr := fidelityDeviceForBackend(backend)
	if derr != nil {
		// No profile, no prediction — but the denial itself is sealed:
		// unattested refusals are indistinguishable from never-asked.
		unassessed := map[string]any{
			"device_name":        "",
			"predicted_fidelity": 0.0,
			"measured_clifford":  0.0,
			"measured":           false,
			"decision":           string(fidelity.DecisionRefuse),
			"reason":             derr.Error(),
			"floor_threshold":    r.fidelityGate.FloorThreshold(),
			"drift_tolerance":    r.fidelityGate.DriftTolerance(),
			"profile_source":     "none",
			"backend":            backend,
		}
		if aerr := r.ledger.AppendEvent(sessionID, "fidelity_assessment", unassessed); aerr != nil {
			return fmt.Errorf("fidelity assessment could not be sealed: %w (admission refused)", aerr)
		}
		return derr
	}
	qubits, depth, twoQ := countQasmGates(qasm)
	assessment := r.fidelityGate.EvaluateAdmissionFloorOnly(device, depth, twoQ)
	record := map[string]any{
		"device_name":        assessment.DeviceName,
		"predicted_fidelity": assessment.PredictedFidelity,
		"measured_clifford":  assessment.MeasuredClifford,
		"measured":           assessment.MeasuredClifford > 0,
		"decision":           string(assessment.Decision),
		"reason":             assessment.Reason,
		"qubits":             qubits,
		"depth":              depth,
		"two_qubit_gates":    twoQ,
		"floor_threshold":    r.fidelityGate.FloorThreshold(),
		"drift_tolerance":    r.fidelityGate.DriftTolerance(),
		"profile_source":     "builtin-bootstrap",
		"backend":            backend,
	}
	if err := r.ledger.AppendEvent(sessionID, "fidelity_assessment", record); err != nil {
		return fmt.Errorf("fidelity assessment could not be sealed: %w (admission refused)", err)
	}
	if assessment.Decision != fidelity.DecisionAdmit {
		return fmt.Errorf("fidelity gate: %s (predicted %.3f on %s, %d qubits, depth %d, %d 2q gates; %s)",
			assessment.Reason, assessment.PredictedFidelity, device, qubits, depth, twoQ,
			assessment.RecommendedAction)
	}
	return nil
}

// verifyGrantChain decodes, verifies, and binds a presented UCAN grant chain
// to the job's tenant: the chain must target this tenant and carry the
// required scope. Trust roots come from registered lab admin keys; revocation
// from the ledger registry.
func (r *Runner) verifyGrantChain(chainB64 []string, jobTenant, requiredScope string) (*grants.VerifiedGrant, error) {
	chain, err := grants.DecodeChainB64(chainB64)
	if err != nil {
		return nil, err
	}
	all, err := r.tenantStore().ListTenants()
	if err != nil {
		return nil, fmt.Errorf("tenant lookup failed: %w", err)
	}
	admins := make([]grants.AdminDID, 0, len(all))
	for _, t := range all {
		admins = append(admins, grants.AdminDID{TenantID: t.ID, AdminPubkeyHex: t.AdminPubkeyHex, Status: t.Status})
	}
	roots := grants.TrustRootsForTenants(admins)
	vg, err := grants.VerifyChain(chain, grants.VerifyOptions{
		TrustedRoots:  roots,
		Revoked:       grants.RevokedCIDs(r.ledger.DB()),
		RequiredScope: requiredScope,
	})
	if err != nil {
		return nil, err
	}
	if vg.TargetTenant != jobTenant {
		return nil, fmt.Errorf("grant targets tenant %q, job runs as %q: refused", vg.TargetTenant, jobTenant)
	}
	return vg, nil
}
