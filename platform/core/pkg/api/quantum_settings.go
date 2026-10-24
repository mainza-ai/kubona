package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Quantum provider settings (P2): the human-surface counterpart to the
// agent execution rail. Agents pick providers per MCP call (default
// local-simulator, governed); humans pick here. This module stores ONLY
// selection — default backend + per-harness overrides — never secrets.
// Credentials live in the environment (KUBONA_{PROVIDER}_{KEY}) or the
// OpenBao sidecar per secrets.py; the server never accepts, stores, or
// echoes them through these routes.
//
// Known provider IDs mirror platform/kubona/schema/providers/*.json
// (test_schema.py::test_initial_registry pins that set; keep the two in
// step when adding a vendor).

// QuantumProviderID is the execution-rail provider key (submit_job's
// provider param, capability JSON "provider" field).
type QuantumProviderID = string

// KnownQuantumProviders is the closed provider vocabulary. Unknown IDs
// are refused at save — a typo must never become a silent default.
var KnownQuantumProviders = []string{
	"local-simulator",
	"qiskit-aer-simulator",
	"ibm-qiskit",
	"rigetti",
	"quantinuum",
	"dwave-annealing",
	"neutral-atom-quera-pasqal",
	"xanadu-strawberry-fields",
	"ionq",
	"aws-braket",
}

// quantumBackendRow is one row of the provider table: display metadata
// plus the credential fact this daemon can measure (env presence).
// Telemetry follows the fleet contract: "measured" was read here,
// "declared" is vendor catalog until the live canary proxy lands.
type quantumBackendRow struct {
	Provider        string `json:"provider"`
	DisplayName     string `json:"display_name"`
	Modality        string `json:"modality"`
	Device          string `json:"device"`
	CredentialEnv   string `json:"credential_env"`
	CredentialState string `json:"credential_state"`
	Telemetry       string `json:"telemetry"`
	Executable      bool   `json:"executable_now"`
}

// quantumBackendTable is the single provider table for the quantum
// settings surface. Devices mirror the fleet cards where they exist;
// providers without a fleet card (simulators beyond local, unwired
// modalities, not-yet-integrated vendors) are listed with declared
// telemetry so the gap is visible, not hidden.
var quantumBackendTable = []quantumBackendRow{
	{Provider: "local-simulator", DisplayName: "Local simulator (Aer statevector)", Modality: "simulator", Device: "local", CredentialEnv: "", Telemetry: "measured"},
	{Provider: "qiskit-aer-simulator", DisplayName: "Qiskit Aer simulator", Modality: "simulator", Device: "aer_simulator", CredentialEnv: "", Telemetry: "measured"},
	{Provider: "ibm-qiskit", DisplayName: "IBM Quantum (Heron)", Modality: "superconducting", Device: "ibm_heron", CredentialEnv: "KUBONA_IBM_QISKIT_TOKEN", Telemetry: "declared"},
	{Provider: "rigetti", DisplayName: "Rigetti QCS (Aspen/Cepheus)", Modality: "superconducting", Device: "rigetti_aspen", CredentialEnv: "KUBONA_RIGETTI_QCS_TOKEN", Telemetry: "declared"},
	{Provider: "quantinuum", DisplayName: "Quantinuum H-System", Modality: "trapped_ion", Device: "quantinuum_h1", CredentialEnv: "KUBONA_QUANTINUUM_API_KEY", Telemetry: "declared"},
	{Provider: "dwave-annealing", DisplayName: "D-Wave annealing (local exact/SA)", Modality: "annealing", Device: "local/SA (Leap cloud not wired)", CredentialEnv: "KUBONA_DWAVE_ANNEALING_LEAP_TOKEN", Telemetry: "declared"},
	{Provider: "neutral-atom-quera-pasqal", DisplayName: "Neutral atom (Pulser — not wired)", Modality: "neutral_atom", Device: "unwired", CredentialEnv: "", Telemetry: "declared"},
	{Provider: "xanadu-strawberry-fields", DisplayName: "Photonic (Strawberry Fields — not wired)", Modality: "photonic", Device: "unwired", CredentialEnv: "", Telemetry: "declared"},
	{Provider: "ionq", DisplayName: "IonQ (not integrated)", Modality: "trapped_ion", Device: "unintegrated", CredentialEnv: "", Telemetry: "declared"},
	{Provider: "aws-braket", DisplayName: "AWS Braket (not integrated)", Modality: "broker", Device: "unintegrated", CredentialEnv: "", Telemetry: "declared"},
}

// QuantumSettings is the persisted human selection. Empty DefaultBackend
// normalizes to local-simulator (safe default, mirrors the MCP rail).
type QuantumSettings struct {
	DefaultBackend  string            `json:"default_backend"`
	HarnessBackends map[string]string `json:"harness_backends"`
	UpdatedAt       string            `json:"updated_at,omitempty"`
}

// DefaultQuantumSettings is the safe default: local simulation, no
// overrides. Hardware requires explicit human selection here AND
// credentials in the environment — selecting a QPU without its token
// still refuses at submit (fail-closed, unchanged).
func DefaultQuantumSettings() *QuantumSettings {
	return &QuantumSettings{
		DefaultBackend:  "local-simulator",
		HarnessBackends: map[string]string{},
	}
}

func knownQuantumProvider(id string) bool {
	for _, p := range KnownQuantumProviders {
		if p == id {
			return true
		}
	}
	return false
}

// ValidateQuantumSettings rejects unknown providers and empty harness
// names. Backend executability (credentials, wiring) is NOT validated
// here — selection and executability are separate facts, and the submit
// rail re-checks both. Conflating them would turn "plan a Quantinuum run
// before the token arrives" into an error.
func ValidateQuantumSettings(s *QuantumSettings) error {
	if s.DefaultBackend == "" {
		s.DefaultBackend = "local-simulator"
	}
	if !knownQuantumProvider(s.DefaultBackend) {
		return fmt.Errorf("unknown default_backend %q: use a known provider id", s.DefaultBackend)
	}
	if s.HarnessBackends == nil {
		s.HarnessBackends = map[string]string{}
	}
	for harness, backend := range s.HarnessBackends {
		if strings.TrimSpace(harness) == "" {
			return fmt.Errorf("harness_backends has an empty harness name")
		}
		if !knownQuantumProvider(backend) {
			return fmt.Errorf("unknown backend %q for harness %q", backend, harness)
		}
	}
	return nil
}

// initQuantumSettingsTable creates the persistence table (mirrors
// llm_settings: single active row, operator-owned).
func initQuantumSettingsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS quantum_settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// loadQuantumSettings reads persisted settings or the safe default.
// Corrupt rows refuse (fail-closed) rather than silently defaulting.
func loadQuantumSettings(db *sql.DB) (*QuantumSettings, error) {
	if err := initQuantumSettingsTable(db); err != nil {
		return nil, err
	}
	var raw string
	err := db.QueryRow("SELECT value_json FROM quantum_settings WHERE key = 'active'").Scan(&raw)
	if err == sql.ErrNoRows {
		return DefaultQuantumSettings(), nil
	}
	if err != nil {
		return nil, err
	}
	var s QuantumSettings
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, err
	}
	if err := ValidateQuantumSettings(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveQuantumSettings(db *sql.DB, s *QuantumSettings) error {
	if err := ValidateQuantumSettings(s); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO quantum_settings(key, value_json, updated_at) VALUES('active', ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=CURRENT_TIMESTAMP`,
		string(raw))
	return err
}

// mustLoadQuantumSettings loads persisted settings or the safe default.
// Boot never fails on this: a corrupt row is fail-closed at read time by
// loadQuantumSettings, and here we fall back to defaults so the daemon
// stays up (the operator re-saves through the PUT route).
func mustLoadQuantumSettings(db *sql.DB) *QuantumSettings {
	s, err := loadQuantumSettings(db)
	if err != nil {
		return DefaultQuantumSettings()
	}
	return s
}

// credential facts. Executable-now = simulator (credential-free) or a
// wired provider whose token is present. Unwired/unintegrated providers
// are listed as not-executable so the gap is visible in the UI.
func quantumBackends() []quantumBackendRow {
	out := make([]quantumBackendRow, 0, len(quantumBackendTable))
	for _, row := range quantumBackendTable {
		r := row
		switch r.Provider {
		case "local-simulator", "qiskit-aer-simulator":
			r.CredentialState = "ready"
			r.Executable = true
		case "ibm-qiskit", "rigetti", "quantinuum":
			if strings.TrimSpace(os.Getenv(r.CredentialEnv)) != "" {
				r.CredentialState = "ready"
				r.Executable = true
			} else {
				r.CredentialState = "credentialless"
			}
		default:
			// dwave-annealing solves locally (executable without a key
			// for local exact/SA; Leap cloud unwired); unwired and
			// unintegrated providers never execute.
			if r.Provider == "dwave-annealing" {
				r.CredentialState = "ready"
				r.Executable = true
			} else {
				r.CredentialState = "unavailable"
			}
		}
		out = append(out, r)
	}
	return out
}

func (s *Server) handleQuantumSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.quantumSettings)
	case http.MethodPut, http.MethodPost:
		// Writes get edge auth (same rule as LLM settings): a settings
		// writer could redirect human runs at an expensive backend.
		switch _, _, code := s.checkAuth(r); code {
		case http.StatusOK:
		case http.StatusServiceUnavailable:
			http.Error(w, "API token not configured: set KUBONA_API_TOKEN", http.StatusServiceUnavailable)
			return
		case http.StatusForbidden:
			WriteAPIError(w, "forbidden", "cross-site session use refused: retry same-origin", nil)
			return
		default:
			http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
			return
		}
		var req QuantumSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteAPIError(w, "bad_request", "malformed JSON request", nil)
			return
		}
		if err := saveQuantumSettings(s.ledger.DB(), &req); err != nil {
			WriteAPIError(w, "bad_request", err.Error(), nil)
			return
		}
		updated, err := loadQuantumSettings(s.ledger.DB())
		if err != nil {
			WriteAPIError(w, "internal", "saved but could not re-read settings", nil)
			return
		}
		s.quantumSettings = updated
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "settings": updated})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleQuantumBackends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backends": quantumBackends()})
}
