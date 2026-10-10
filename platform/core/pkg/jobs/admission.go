package jobs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mainza-ai/kubona/core/pkg/fidelity"
	"github.com/mainza-ai/kubona/core/pkg/grants"
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
}

type mandateReceipt struct {
	TenantID        string  `json:"tenant_id"`
	MaxBudgetUSD    float64 `json:"max_budget_usd"`
	ExpiryTimestamp string  `json:"expiry_timestamp"`
}

// admitJob is the pre-QPU admission gate, enforced inside SubmitJob so every
// entry point (HTTP, A2A, autoresearch) passes it. Order: tenant autonomy
// first (L0/suspended/unknown refuse everything), then the hardware rule:
// simulator-class backends proceed, QPU-destined jobs must cite a verified,
// unexpired, budget-covering spend-mandate ledger receipt.
func (r *Runner) admitJob(sessionID, jobType string, paramsJSON []byte) error {
	var tenantID string
	err := r.ledger.DB().QueryRow(`SELECT tenant_id FROM sessions WHERE id = ?`, sessionID).Scan(&tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("unknown session %q: dispatch refused", sessionID)
		}
		return fmt.Errorf("session lookup failed: %w", err)
	}
	if tenantID == "" {
		tenantID = tenants.DefaultTenantID
	}
	t, err := r.tenantStore().GetTenant(tenantID)
	if err != nil {
		return fmt.Errorf("unknown tenant %q: dispatch refused", tenantID)
	}
	if t.Status != "active" {
		return fmt.Errorf("tenant %q is %s: dispatch refused", t.ID, t.Status)
	}
	if t.AutonomyLevel == tenants.LevelL0 {
		return fmt.Errorf("tenant %q autonomy L0 (observe/suggest only): dispatch refused", t.ID)
	}

	if jobType != "circuit_run" {
		return nil
	}
	var p circuitRunParams
	if err := json.Unmarshal(paramsJSON, &p); err != nil {
		return fmt.Errorf("invalid circuit_run params: %w", err)
	}
	// Cross-lab UCAN grant path: a verified chain authorizing this tenant
	// replaces the mandate receipt and admits foreign-tenant input labels it
	// covers with read scope. Spend binds to the granting lab's ceiling.
	var grant *grants.VerifiedGrant
	if len(p.GrantChain) > 0 {
		var err error
		grant, err = r.verifyGrantChain(p.GrantChain, tenantID, grants.ScopeQPUSubmit)
		if err != nil {
			return fmt.Errorf("grant rejected: %w", err)
		}
		if err := r.tenantStore().CheckSpend(grant.GrantingTenant, p.MaxCostUSD); err != nil {
			return fmt.Errorf("granting lab ceiling: %w", err)
		}
		if p.MaxCostUSD > grant.MaxBudgetUSD {
			return fmt.Errorf("job cost %.4f exceeds grant budget %.4f: dispatch refused", p.MaxCostUSD, grant.MaxBudgetUSD)
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
			return fmt.Errorf("input label %q not readable by tenant %q: dispatch refused", label, tenantID)
		}
	}
	if isSimulatorBackend(p.RequestedBackend) {
		return nil
	}
	// Fidelity gate (Ring 3, second rail beside money): predict whether this
	// run can produce a publishable number BEFORE spending. Physics does not
	// care who pays, so this runs before grant/mandate checks. Only ADMIT
	// proceeds; mitigate/downgrade verdicts refuse with the recommended
	// action stated — remediation changes what was approved and must be
	// resubmitted explicitly, never applied silently.
	if err := r.admitFidelity(p.RequestedBackend, p.QASM); err != nil {
		return err
	}
	if grant != nil {
		return nil
	}
	if p.MandateRef == "" {
		return fmt.Errorf("backend %q is metered hardware: a verified spend-mandate receipt (mandate_ref) is required", p.RequestedBackend)
	}
	return r.verifyMandateReceipt(p.MandateRef, tenantID, p.MaxCostUSD)
}

// verifyMandateReceipt checks a cited mandate verification event: right
// tenant, budget covering the job cost, unexpired.
func (r *Runner) verifyMandateReceipt(entryHash, tenantID string, costUSD float64) error {
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
func (r *Runner) admitFidelity(backend, qasm string) error {
	device, err := fidelityDeviceForBackend(backend)
	if err != nil {
		return err
	}
	qubits, depth, twoQ := countQasmGates(qasm)
	assessment := r.fidelityGate.EvaluateAdmissionFloorOnly(device, depth, twoQ)
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
