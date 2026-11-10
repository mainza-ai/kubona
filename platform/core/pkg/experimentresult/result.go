// Package experimentresult is the canonical decoder for the versioned
// experiment result envelope emitted by the Python bridge and persisted by
// the Go job runner. It is deliberately dependency-free so jobs, campaign
// folding, export, and tests can share one validation rule.
package experimentresult

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

const Version = "experiment-result-v1"

// Receipt is the ledger-backed result receipt attached by the Go job runner.
// It is optional on the bridge result and required only when a caller asks
// whether a stored result is signed.
type Receipt struct {
	Algorithm       string `json:"algorithm"`
	LedgerEntryHash string `json:"ledger_entry_hash"`
	ResultSHA256    string `json:"result_sha256"`
	SignerPubKey    string `json:"signer_pubkey,omitempty"`
	Signature       string `json:"signature,omitempty"`
	Signed          bool   `json:"signed"`
	HashVersion     int    `json:"hash_version,omitempty"`
	SignatureDomain string `json:"signature_domain,omitempty"`
}

// Result is the shared envelope. Numeric pointers preserve the difference
// between "absent" and a measured zero.
type Result struct {
	ResultVersion  string   `json:"result_version"`
	Success        bool     `json:"success"`
	Kind           string   `json:"kind"`
	Molecule       string   `json:"molecule,omitempty"`
	Generic        *bool    `json:"generic,omitempty"`
	Atom           string   `json:"atom,omitempty"`
	Basis          string   `json:"basis,omitempty"`
	Charge         *int     `json:"charge,omitempty"`
	Spin           *int     `json:"spin,omitempty"`
	Model          string   `json:"model,omitempty"`
	NSites         int      `json:"n_sites,omitempty"`
	Energy         *float64 `json:"energy,omitempty"`
	EnergyExact    *float64 `json:"energy_exact,omitempty"`
	FCIExactEnergy *float64 `json:"fci_exact_energy,omitempty"`
	Uncertainty    *float64 `json:"uncertainty,omitempty"`
	DeltaMHa       *float64 `json:"delta_mha,omitempty"`
	OptimalCut     *int     `json:"optimal_cut,omitempty"`
	MeasuredCut    *float64 `json:"measured_cut,omitempty"`
	GroundEnergy   *float64 `json:"ground_energy,omitempty"`
	ExcitedEnergy  *float64 `json:"excited_energy,omitempty"`
	Gap            *float64 `json:"gap,omitempty"`
	ReactionEnergy *float64 `json:"reaction_energy,omitempty"`
	ReactionExact  *float64 `json:"reaction_energy_exact,omitempty"`
	CandidateCount *int     `json:"candidate_count,omitempty"`
	Measured       *bool    `json:"measured,omitempty"`
	Hardware       *bool    `json:"hardware,omitempty"`
	HardwareJobID  string   `json:"hardware_job_id,omitempty"`
	HardwareJobIDs []string `json:"hardware_job_ids,omitempty"`
	Claimable      *bool    `json:"claimable,omitempty"`
	Receipt        *Receipt `json:"result_receipt,omitempty"`

	raw map[string]json.RawMessage
}

// Decode validates the version, job-kind match, and the fields a citable
// result must carry. It rejects success=false: failed jobs are represented by
// the job row, not promoted into a result projection.
func Decode(jobType string, data []byte) (*Result, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("experiment result is empty")
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("experiment result is not valid JSON: %w", err)
	}
	if err := json.Unmarshal(data, &r.raw); err != nil {
		return nil, fmt.Errorf("experiment result is not an object: %w", err)
	}
	if r.ResultVersion != Version {
		return nil, fmt.Errorf("experiment result_version %q is not %s: refusing untyped result", r.ResultVersion, Version)
	}
	if !r.Success {
		return nil, fmt.Errorf("experiment result reports success=false: refusing a citable result")
	}
	kind := strings.ToLower(strings.TrimSpace(r.Kind))
	if kind == "" {
		return nil, fmt.Errorf("experiment result has no kind")
	}
	if !kindMatchesJobType(jobType, kind) {
		return nil, fmt.Errorf("experiment result kind %q does not match job type %q", r.Kind, jobType)
	}
	r.Kind = kind
	for name, value := range map[string]*float64{
		"energy": r.Energy, "energy_exact": r.EnergyExact,
		"fci_exact_energy": r.FCIExactEnergy, "uncertainty": r.Uncertainty,
		"delta_mha": r.DeltaMHa, "measured_cut": r.MeasuredCut,
		"ground_energy": r.GroundEnergy, "excited_energy": r.ExcitedEnergy,
		"gap": r.Gap, "reaction_energy": r.ReactionEnergy,
		"reaction_energy_exact": r.ReactionExact,
	} {
		if value != nil && !finite(*value) {
			return nil, fmt.Errorf("experiment result %s is not finite", name)
		}
	}
	if r.Uncertainty != nil && *r.Uncertainty < 0 {
		return nil, fmt.Errorf("experiment result uncertainty must be non-negative")
	}
	if err := validateVariant(kind, &r); err != nil {
		return nil, err
	}
	if r.Receipt != nil {
		if r.Receipt.Algorithm != "" && r.Receipt.Algorithm != "ed25519" {
			return nil, fmt.Errorf("result receipt algorithm %q is unsupported", r.Receipt.Algorithm)
		}
		if r.Receipt.LedgerEntryHash == "" || r.Receipt.ResultSHA256 == "" {
			return nil, fmt.Errorf("result receipt is missing its ledger/result hash")
		}
		if r.Receipt.Signed && r.Receipt.Signature == "" {
			return nil, fmt.Errorf("result receipt claims signed=true without a signature")
		}
	}
	return &r, nil
}

// ReceiptFromJSON extracts a receipt without requiring a full valid result.
// Used by campaign fold after the job boundary already validated the result.
func ReceiptFromJSON(data []byte) *Receipt {
	var envelope struct {
		Receipt *Receipt `json:"result_receipt"`
	}
	if len(data) == 0 || json.Unmarshal(data, &envelope) != nil {
		return nil
	}
	return envelope.Receipt
}

func kindMatchesJobType(jobType, kind string) bool {
	expected := map[string][]string{
		"vqe_run":             {"vqe"},
		"vqe_pes":             {"pes"},
		"qaoa_run":            {"qaoa"},
		"excited_state":       {"excited_state"},
		"reaction":            {"reaction"},
		"spin_chain":          {"spin_chain"},
		"conjecture_run":      {"conjecture"},
		"hardware_experiment": {"vqe_hardware", "vqe_hardware_loop"},
		"qaoa_hardware":       {"qaoa_hardware", "qaoa_hardware_loop"},
		"spin_chain_hardware": {"spin_chain_hardware"},
		"krylov_json":         {"krylov"},
		"mitigate_json":       {"mitigate"},
	}
	kinds, ok := expected[jobType]
	if !ok {
		// Non-experiment jobs are handled by their own RPC shapes.
		return true
	}
	for _, candidate := range kinds {
		if kind == candidate {
			return true
		}
	}
	return false
}

func validateVariant(kind string, r *Result) error {
	require := func(name string, value *float64) error {
		if value == nil {
			return fmt.Errorf("experiment result kind %q requires finite %s", kind, name)
		}
		return nil
	}
	switch kind {
	case "vqe":
		if err := require("energy", r.Energy); err != nil {
			return err
		}
		if err := require("fci_exact_energy", r.FCIExactEnergy); err != nil {
			return err
		}
	case "pes":
		var points []map[string]json.RawMessage
		raw, ok := r.raw["points"]
		if !ok || json.Unmarshal(raw, &points) != nil || len(points) == 0 {
			return fmt.Errorf("experiment result kind %q requires non-empty points", kind)
		}
		for i, point := range points {
			for _, key := range []string{"energy", "fci_exact_energy"} {
				value, ok := point[key]
				if !ok {
					return fmt.Errorf("PES point %d is missing %s", i, key)
				}
				var number float64
				if json.Unmarshal(value, &number) != nil || !finite(number) {
					return fmt.Errorf("PES point %d %s is not finite", i, key)
				}
			}
		}
	case "qaoa":
		if err := require("energy", r.Energy); err != nil {
			return err
		}
		if r.OptimalCut == nil || *r.OptimalCut <= 0 {
			return fmt.Errorf("experiment result kind %q requires positive optimal_cut", kind)
		}
	case "excited_state":
		for name, value := range map[string]*float64{
			"ground_energy": r.GroundEnergy, "excited_energy": r.ExcitedEnergy, "gap": r.Gap,
		} {
			if err := require(name, value); err != nil {
				return err
			}
		}
	case "reaction":
		if err := require("reaction_energy", r.ReactionEnergy); err != nil {
			return err
		}
		if err := require("reaction_energy_exact", r.ReactionExact); err != nil {
			return err
		}
	case "spin_chain":
		if err := require("energy", r.Energy); err != nil {
			return err
		}
		if err := require("energy_exact", r.EnergyExact); err != nil {
			return err
		}
	case "spin_chain_hardware":
		if err := require("energy", r.Energy); err != nil {
			return err
		}
		if err := require("energy_exact", r.EnergyExact); err != nil {
			return err
		}
		if r.Hardware == nil || !*r.Hardware {
			return fmt.Errorf("experiment result kind %q must be hardware=true", kind)
		}
		if strings.TrimSpace(r.HardwareJobID) == "" {
			return fmt.Errorf("experiment result kind %q requires hardware_job_id", kind)
		}
	case "qaoa_hardware":
		if r.MeasuredCut == nil && r.Energy == nil {
			return fmt.Errorf("experiment result kind %q requires measured_cut/energy", kind)
		}
		if r.OptimalCut == nil || *r.OptimalCut <= 0 {
			return fmt.Errorf("experiment result kind %q requires positive optimal_cut", kind)
		}
		if r.Hardware == nil || !*r.Hardware {
			return fmt.Errorf("experiment result kind %q must be hardware=true", kind)
		}
		if strings.TrimSpace(r.HardwareJobID) == "" {
			return fmt.Errorf("experiment result kind %q requires hardware_job_id", kind)
		}
	case "vqe_hardware", "vqe_hardware_loop":
		if err := require("energy", r.Energy); err != nil {
			return err
		}
		if err := require("fci_exact_energy", r.FCIExactEnergy); err != nil {
			return err
		}
		if r.Hardware == nil || !*r.Hardware {
			return fmt.Errorf("experiment result kind %q must be hardware=true", kind)
		}
		if strings.TrimSpace(r.HardwareJobID) == "" && len(r.HardwareJobIDs) == 0 {
			return fmt.Errorf("experiment result kind %q requires a vendor hardware job id", kind)
		}
	case "conjecture":
		if r.CandidateCount == nil || *r.CandidateCount < 0 {
			return fmt.Errorf("experiment result kind %q requires candidate_count", kind)
		}
	case "krylov":
		// The typed Krylov RPC owns its own required fields; the versioned
		// experiment JSON path only needs the kind envelope.
	case "mitigate":
		// Mitigation receipts are validated by the mitigation runner.
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
