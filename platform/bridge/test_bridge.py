import pytest
import numpy as np
from chemistry import solve_classical_anchor
from epistemics import evaluate_claim_credibility, compute_wavefunction_amplitudes
from executor import execute_qasm, calculate_meyer_wallach
from fidelity import build_noise_model, predict_fidelity, probe_clifford_fidelity
from cutting import cut_and_reconstruct_expectation
from literature import search_arxiv, search_openalex


def test_classical_anchor_h2():
    res = solve_classical_anchor("H2", 0.7414)
    assert res["molecule"] == "H2"
    assert res["bond_distance"] == 0.7414
    assert res["rhf_energy"] < -1.10
    assert res["fci_exact_energy"] < res["rhf_energy"]  # FCI is variational lower bound
    assert res["correlation_energy"] < 0.0
    assert res["nuclear_repulsion"] > 0.0


def test_classical_anchor_lih():
    res = solve_classical_anchor("LiH", 1.5957)
    assert res["molecule"] == "LIH"
    assert res["fci_exact_energy"] < res["rhf_energy"]
    assert len(res["orbital_energies"]) > 0


def test_epistemics_chemical_accuracy():
    # Pass within chemical accuracy (0.001 Ha = 1 mHa < 1.6 mHa)
    res = evaluate_claim_credibility(
        claim_id="claim-1",
        statement="H2 ground state energy",
        quantum_value=-1.137,
        classical_anchor_value=-1.136,
        source_platforms=["arxiv", "chemrxiv"],
        total_engagement=500,
        contradiction_signal=0.0,
    )
    assert res["passes_chemical_accuracy"] is True
    assert res["delta_mha"] == pytest.approx(1.0, 0.01)


def test_quantum_advantage_realism_rejection():
    # False advantage claim that exceeds chemical accuracy
    res = evaluate_claim_credibility(
        claim_id="claim-2",
        statement="Achieved quantum advantage over classical chemistry",
        quantum_value=-1.150,
        classical_anchor_value=-1.137,  # 13 mHa discrepancy
        source_platforms=["twitter"],
        total_engagement=20,
        contradiction_signal=0.8,
    )
    assert res["quantum_advantage_claimed"] is True
    assert res["quantum_advantage_verified"] is False
    assert "REJECTED" in res["verdict_reason"]


def test_circuit_execution_bell_state():
    bell_qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
"""
    res = execute_qasm(bell_qasm, shots=500)
    assert res["success"] is True
    assert "counts" in res
    assert res["meyer_wallach"] > 0.9  # Maximally entangled Bell state


def test_fidelity_accounting_and_noise_constraint():
    model = build_noise_model("ibm_heron")
    assert model is not None

    fid_short = predict_fidelity(circuit_depth=5, two_qubit_gates=2, device_name="ibm_heron")
    fid_deep = predict_fidelity(circuit_depth=50, two_qubit_gates=40, device_name="ibm_heron")
    assert fid_short > fid_deep
    assert fid_short > 0.9

    cliff_fid = probe_clifford_fidelity("h q[0]; cx q[0], q[1];", shots=64)
    assert 0.1 <= cliff_fid <= 1.0


def test_pennylane_circuit_cutting():
    res = cut_and_reconstruct_expectation(n_qubits=2, rotations=[0.54, 0.32], cut_wire=0)
    assert res["reconstruction_error"] < 1e-5
    assert res["method"] == "PennyLane QPD Wire Cut"


def test_literature_grounding():
    results = search_arxiv("quantum eigensolver", max_results=2)
    assert len(results) > 0
    assert "title" in results[0]
    assert results[0]["provenance_verified"] is True

    oa_results = search_openalex("quantum computing", max_results=2)
    assert len(oa_results) > 0
    assert "title" in oa_results[0]


def test_ansatz_genome_search_and_prechecks():
    from ansatz_search import (
        AnsatzGenome,
        synthesize_qasm,
        evaluate_meyer_wallach_expressibility,
        probe_barren_plateau_variance,
        estimate_mps_bond_dimension,
    )

    genome = AnsatzGenome(
        family="HardwareEfficient",
        num_qubits=4,
        depth=3,
        entangler_topology="linear",
        initializer="zero",
    )

    # 1. Synthesize QASM
    qasm = synthesize_qasm(genome)
    assert "OPENQASM 2.0;" in qasm
    assert "qreg q[4];" in qasm
    assert "cx q[0], q[1];" in qasm

    # 2. Expressibility Q
    q = evaluate_meyer_wallach_expressibility(genome)
    assert 0.0 < q <= 1.0

    # 3. Barren plateau probe
    bp = probe_barren_plateau_variance(genome)
    assert bp["admissible"] is True
    assert bp["barren_plateau_detected"] is False

    # Deep circuit on 24 qubits should detect barren plateau
    deep_genome = AnsatzGenome(
        family="HardwareEfficient",
        num_qubits=24,
        depth=50,
        entangler_topology="all-to-all",
        initializer="random",
    )
    bp_deep = probe_barren_plateau_variance(deep_genome)
    assert bp_deep["barren_plateau_detected"] is True
    assert bp_deep["admissible"] is False

    # 4. MPS bond dimension
    mps = estimate_mps_bond_dimension(genome)
    assert mps["estimated_bond_dimension"] <= 64
    assert mps["classical_mps_sufficient"] is True


def test_molecular_admet_profiling():
    from chemistry import evaluate_molecular_admet, evaluate_candidate_pipeline

    # Aspirin
    aspirin = evaluate_molecular_admet("CC(=O)Oc1ccccc1C(=O)O")
    assert aspirin["molecular_weight"] == pytest.approx(180.16, 0.1)
    assert aspirin["clogp"] == pytest.approx(1.31, 0.1)
    assert aspirin["lipinski"]["pass"] is True
    assert aspirin["lipinski"]["violations"] == 0
    assert aspirin["admet"]["solubility_class"] in ("Soluble", "Moderately Soluble")
    assert aspirin["admet"]["caco2_permeability"] == "High"
    assert len(aspirin["pains_alerts"]) == 0
    assert aspirin["sa_score"] < 3.5

    # 12-Agent Candidate Pipeline
    candidates = [
        {"name": "Aspirin", "smiles": "CC(=O)Oc1ccccc1C(=O)O"},
        {"name": "Paracetamol", "smiles": "CC(=O)Nc1ccc(O)cc1"},
        {"name": "Ibuprofen", "smiles": "CC(C)Cc1ccc(cc1)C(C)C(=O)O"},
    ]
    res = evaluate_candidate_pipeline("COX-2", candidates)
    assert res["target"] == "COX-2"
    assert res["total_evaluated"] == 3
    assert res["top_lead"] is not None
    assert res["top_lead"]["composite_score"] > 0.0


def test_simulation_preflight_memory_clamp():
    from executor import check_circuit_memory_bounds, execute_qasm
    from qiskit import QuantumCircuit

    # 1. Bounds calculation
    qc_small = QuantumCircuit(4)
    bounds_small = check_circuit_memory_bounds(qc_small)
    assert bounds_small["num_qubits"] == 4
    assert bounds_small["required_mb"] < 1.0
    assert bounds_small["exceeds_statevector_limit"] is False
    assert bounds_small["exceeds_aer_limit"] is False

    qc_32 = QuantumCircuit(32)
    bounds_32 = check_circuit_memory_bounds(qc_32)
    assert bounds_32["num_qubits"] == 32
    assert bounds_32["required_mb"] > 60000.0  # ~64 GB
    assert bounds_32["exceeds_statevector_limit"] is True
    assert bounds_32["exceeds_aer_limit"] is True

    # 2. Strict fail-closed execution without fallback on 32-qubit circuit
    qasm_32 = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[32];\nh q[0];\ncx q[0], q[1];\n"
    res_strict = execute_qasm(qasm_32, requested_backend="aer_simulator", allow_fallback=False)
    assert res_strict["success"] is False
    assert "PREFLIGHT_RAM_EXCEEDED" in res_strict["error_message"]
    assert "exceeding safe host simulation limit" in res_strict["error_message"]

    # 3. Honest auto-partition degradation when allow_fallback=True
    res_partition = execute_qasm(qasm_32, requested_backend="aer_simulator", allow_fallback=True)
    assert res_partition["success"] is True
    assert res_partition["degradation_level"] == 4
    assert "circuit-partition" in res_partition["resolved_backend"]
    assert "Pre-flight RAM limit exceeded" in res_partition["degradation_reason"]
    assert len(res_partition["partition_fragments"]) > 0


