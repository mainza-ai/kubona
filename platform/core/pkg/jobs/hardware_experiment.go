package jobs

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/mainza-ai/kubona/core/pkg/mandates"
)

// HardwareExperimentParams is the parent experiment submission. The parent
// owns one bounded mandate, one vendor batch, one usage reading, and one
// settlement. Child measurement circuits are prepared by the bridge inside
// that parent execution; they are not independently admitted or settled.
type HardwareExperimentParams struct {
	Molecule            string             `json:"molecule"`
	BondDistance        float64            `json:"bond_distance"`
	Layers              int                `json:"layers"`
	Shots               int                `json:"shots"`
	Seed                int                `json:"seed"`
	Theta               []float64          `json:"theta"`
	PauliCoefficients   map[string]float64 `json:"pauli_coefficients"`
	FCIExactEnergy      float64            `json:"fci_exact_energy"`
	Provider            string             `json:"provider"`
	RequestedBackend    string             `json:"requested_backend"`
	FidelityQASM        string             `json:"fidelity_qasm"`
	MaxCostUSD          float64            `json:"max_cost_usd"`
	MandateRef          string             `json:"mandate_ref"`
	CallerJurisdictions []string           `json:"caller_jurisdictions"`
	CallerExportClass   string             `json:"caller_export_class"`
	ExportReviewed      bool               `json:"export_reviewed"`
}

// CanonicalHardwareExperimentBinding is the byte-stable parent mandate binding.
// It hashes the normalized experiment spec, not an individual child circuit:
// one parent approval covers the whole batch and its aggregate budget.
func CanonicalHardwareExperimentBinding(sessionID string, p HardwareExperimentParams) string {
	spec := struct {
		Molecule          string             `json:"molecule"`
		BondDistance      float64            `json:"bond_distance"`
		Layers            int                `json:"layers"`
		Shots             int                `json:"shots"`
		Seed              int                `json:"seed"`
		Theta             []float64          `json:"theta"`
		PauliCoefficients map[string]float64 `json:"pauli_coefficients"`
		FCIExactEnergy    float64            `json:"fci_exact_energy"`
		Provider          string             `json:"provider"`
		RequestedBackend  string             `json:"requested_backend"`
	}{
		Molecule: p.Molecule, BondDistance: p.BondDistance,
		Layers: p.Layers, Shots: p.Shots, Seed: p.Seed,
		Theta: p.Theta, PauliCoefficients: p.PauliCoefficients,
		FCIExactEnergy: p.FCIExactEnergy, Provider: p.Provider,
		RequestedBackend: p.RequestedBackend,
	}
	b, _ := json.Marshal(spec)
	return mandates.CanonicalJobBinding("hardware_experiment", sessionID, string(b))
}

// planHardwareExperiment is the money-free validation twin for the hardware
// parent. It requires a real hardware provider/backend, a verified parent
// mandate, declared routing, positive cost, and a fidelity-admitted ansatz.
func (r *Runner) planHardwareExperiment(tenantID, sessionID string, paramsJSON []byte) (string, float64, error) {
	var p HardwareExperimentParams
	if err := json.Unmarshal(paramsJSON, &p); err != nil {
		return "", 0, fmt.Errorf("invalid hardware_experiment params: %w", err)
	}
	if p.Provider == "" {
		p.Provider = "ibm-qiskit"
	}
	if p.Provider != "ibm-qiskit" {
		return "", 0, fmt.Errorf("hardware_experiment provider %q is unsupported (only ibm-qiskit)", p.Provider)
	}
	if p.Molecule != "H2" {
		return "", 0, fmt.Errorf("hardware_experiment currently supports H2 only; refusing %q", p.Molecule)
	}
	if p.RequestedBackend == "" {
		return "", 0, fmt.Errorf("hardware_experiment requires requested_backend")
	}
	if isSimulatorBackend(p.RequestedBackend) {
		return "", 0, fmt.Errorf("hardware_experiment requires a hardware backend; %q is simulator-class", p.RequestedBackend)
	}
	if math.IsNaN(p.MaxCostUSD) || math.IsInf(p.MaxCostUSD, 0) || p.MaxCostUSD <= 0 {
		return "", 0, fmt.Errorf("hardware_experiment requires a positive max_cost_usd")
	}
	if len(p.CallerJurisdictions) == 0 {
		return "", 0, fmt.Errorf("hardware_experiment requires caller_jurisdictions")
	}
	if len(p.Theta) == 0 || len(p.PauliCoefficients) == 0 {
		return "", 0, fmt.Errorf("hardware_experiment requires local theta and pauli_coefficients")
	}
	if p.FidelityQASM == "" {
		return "", 0, fmt.Errorf("hardware_experiment requires fidelity_qasm from the local ansatz")
	}
	if err := r.admitFidelity(sessionID, p.Provider, p.FidelityQASM); err != nil {
		return "", 0, err
	}
	if p.MandateRef == "" {
		return "", 0, fmt.Errorf("hardware_experiment requires a verified spend-mandate receipt (mandate_ref)")
	}
	if err := r.verifyMandateReceipt(p.MandateRef, tenantID, p.MaxCostUSD,
		CanonicalHardwareExperimentBinding(sessionID, p)); err != nil {
		return "", 0, err
	}
	if err := r.tenantStore().CheckSpend(tenantID, p.MaxCostUSD); err != nil {
		return "", 0, err
	}
	if err := r.checkSessionSpend(sessionID, p.MaxCostUSD); err != nil {
		return "", 0, err
	}
	return tenantID, p.MaxCostUSD, nil
}
