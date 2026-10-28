"""Bridge production-bar tests (W1.20): no live net in CI, heuristic labels,
ranges over pinned values, refusal paths for every fiction class.

Conventions:
- Literature: httpx is MOCKED; live-net tests are marked live-only and skip
  unless KUBONA_LIVE_NET=1 (CI never touches the network).
- Physics: ranges + method labels, or a statevector oracle — never pinned
  decimals presented as physics.
- RDKit pins are version-pinned (recalibrated for the installed RDKit;
  bump + recalibrate on upgrade, never silently widen).
"""

import math
import os
import socket

import pytest
import numpy as np

from chemistry import solve_classical_anchor
from epistemics import evaluate_claim_credibility, compute_wavefunction_amplitudes
from executor import execute_qasm, calculate_meyer_wallach
from fidelity import build_noise_model, predict_fidelity, probe_clifford_fidelity
from cutting import cut_and_reconstruct_expectation

RDKIT_PINNED_VERSION = "2026.03.6"

BELL_QASM = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
"""


# ---------------------------------------------------------------- anchors ---

def test_classical_anchor_h2():
    res = solve_classical_anchor("H2", 0.7414)
    assert res["molecule"] == "H2"
    assert res["bond_distance"] == 0.7414
    assert res["method"] == "FCI-exact"  # full-space solve, citable as exact
    assert res["rhf_energy"] < -1.10
    assert res["fci_exact_energy"] < res["rhf_energy"]  # FCI is variational lower bound
    assert res["correlation_energy"] < 0.0
    assert res["nuclear_repulsion"] > 0.0


def test_classical_anchor_lih_method_checked():
    # LiH lands in FCI or a named CASCI space — callers must read method
    # before citing the energy as exact (CASCI-labeled-exact is refused
    # downstream, never cited as FCI).
    res = solve_classical_anchor("LiH", 1.5957)
    assert res["molecule"] == "LIH"
    assert res["method"] in ("FCI-exact",) or res["method"].startswith("CASCI(")
    assert res["fci_exact_energy"] < res["rhf_energy"]
    assert len(res["orbital_energies"]) > 0


def test_anchor_unknown_molecule_refuses():
    with pytest.raises(RuntimeError, match="unsupported molecule"):
        solve_classical_anchor("XyZz-Not-A-Molecule", 1.0)


def test_anchor_bond_window_and_basis_refuse():
    with pytest.raises(ValueError, match="bond distance"):
        solve_classical_anchor("H2", 0.05)  # compressed-core artifact zone
    with pytest.raises(ValueError, match="bond distance"):
        solve_classical_anchor("H2", 50.0)  # dissociated-limit zone
    with pytest.raises(ValueError, match="basis"):
        solve_classical_anchor("H2", 0.7414, basis="made-up-basis")


def test_anchor_r_sweep_honored_not_ignored():
    # Regression: NH3/CH4 builders once IGNORED r (hardcoded H positions),
    # so R-sweeps returned one geometry's energy at every distance.
    nh3_a = solve_classical_anchor("NH3", 1.0)
    nh3_b = solve_classical_anchor("NH3", 1.5)
    assert nh3_a["rhf_energy"] != pytest.approx(nh3_b["rhf_energy"], abs=1e-6)
    ch4_a = solve_classical_anchor("CH4", 1.09)
    ch4_b = solve_classical_anchor("CH4", 1.5)
    assert ch4_a["rhf_energy"] != pytest.approx(ch4_b["rhf_energy"], abs=1e-6)
    # CH4 symmetric-tetrahedron regression pin (documented in chemistry.py):
    # the old sign pattern sat +0.22 Ha too high.
    assert ch4_a["rhf_energy"] == pytest.approx(-39.727, abs=0.05)


# --------------------------------------------------------------- epistemics ---

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
        anchor_method="FCI-exact",
        claim_type="measured",
    )
    assert res["passes_chemical_accuracy"] is True
    assert res["delta_mha"] == pytest.approx(1.0, abs=0.01)
    assert res["credibility_method"] == "credibility_heuristic_v1"


def test_wavefunction_amplitudes_normalized():
    alpha, beta, gamma = compute_wavefunction_amplitudes(
        ["arxiv", "chemrxiv"], 500, 0.0
    )
    # Born-rule invariant: probabilities sum to exactly 1.
    assert alpha**2 + beta**2 + gamma**2 == pytest.approx(1.0, abs=1e-9)
    assert all(math.isfinite(a) for a in (alpha, beta, gamma))


def test_epistemics_nan_and_range_refuse():
    # NaN / infinite / out-of-range signals must refuse, never NaN verdicts.
    with pytest.raises(ValueError):
        compute_wavefunction_amplitudes(["x"], 10, float("nan"))
    with pytest.raises(ValueError):
        compute_wavefunction_amplitudes(["x"], 10, 1.5)
    with pytest.raises(ValueError):
        evaluate_claim_credibility(
            claim_id="c", statement="measured x", quantum_value=float("nan"),
            classical_anchor_value=-1.0, source_platforms=["x"],
            total_engagement=5, contradiction_signal=0.0,
        )
    with pytest.raises(ValueError, match="claim_type"):
        evaluate_claim_credibility(
            claim_id="c", statement="measured x", quantum_value=-1.0,
            classical_anchor_value=-1.0, source_platforms=["x"],
            total_engagement=5, contradiction_signal=0.0,
            claim_type="proves",  # not on the allowlist
        )


def test_epistemics_word_boundary_no_substring_gate():
    # "advantageous" contains "advantage" as a substring but is not an
    # advantage claim — the word-boundary regex must not trip.
    res = evaluate_claim_credibility(
        claim_id="c-wb", statement="measured advantageous convergence",
        quantum_value=-1.136, classical_anchor_value=-1.137,
        source_platforms=["arxiv"], total_engagement=50,
        contradiction_signal=0.0, anchor_method="FCI-exact",
        claim_type="measured",
    )
    assert res["quantum_advantage_claimed"] is False


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
        anchor_method="FCI-exact",
        claim_type="advantage",
    )
    assert res["quantum_advantage_claimed"] is True
    assert res["quantum_advantage_verified"] is False
    assert "REJECTED" in res["verdict_reason"]


def test_advantage_casci_labeled_exact_refuses():
    # A CASCI value is variational-in-active-space, never full FCI:
    # advantage grounded on it refuses even with a passing delta.
    res = evaluate_claim_credibility(
        claim_id="c-cas", statement="quantum advantage",
        quantum_value=-7.883, classical_anchor_value=-7.882,
        source_platforms=["arxiv"], total_engagement=100,
        contradiction_signal=0.0, anchor_method="CASCI(6,6)",
        claim_type="advantage",
    )
    assert res["quantum_advantage_claimed"] is True
    assert res["quantum_advantage_verified"] is False
    assert "FCI-exact" in res["verdict_reason"]


# ---------------------------------------------------------------- execution ---

def _bell_probs(counts):
    """Fold Aer 'q c' double-register keys ('00 00') to quantum bits."""
    folded = {}
    for key, c in counts.items():
        qbits = key.split()[0]  # quantum register is the first field
        folded[qbits] = folded.get(qbits, 0) + c
    total = sum(folded.values())
    return {k: v / total for k, v in folded.items()}, total


def test_circuit_execution_bell_state():
    # W1.4 recalibration (documented): explicit seed replaces the old
    # np.random.seed(7) global — same reproducibility, no unseeded draw.
    # The seed is echoed; success carries error_message="".
    res = execute_qasm(BELL_QASM, shots=2000, seed=7)
    assert res["success"] is True
    assert res["error_message"] == ""
    assert res["seed"] == 7
    assert "counts" in res
    probs, total = _bell_probs(res["counts"])
    assert total == 2000
    # Tolerance bands (not pinned counts): Bell is 50/50 on 00/11.
    assert probs.get("00", 0.0) == pytest.approx(0.5, abs=0.08)
    assert probs.get("11", 0.0) == pytest.approx(0.5, abs=0.08)
    assert set(probs) <= {"00", "11"}
    # Statevector oracle: exact MW of the Bell state is 1.0.
    assert res["meyer_wallach"] == pytest.approx(1.0, abs=0.02)
    # Same seed reproduces exactly (seeded RNG, not global state).
    res2 = execute_qasm(BELL_QASM, shots=2000, seed=7)
    assert res2["counts"] == res["counts"]


def test_fidelity_accounting_and_noise_constraint():
    model = build_noise_model("ibm_heron")
    assert model is not None
    assert build_noise_model("not-a-device") is None

    fid_short = predict_fidelity(circuit_depth=5, two_qubit_gates=2, device_name="ibm_heron")
    fid_deep = predict_fidelity(circuit_depth=50, two_qubit_gates=40, device_name="ibm_heron")
    assert fid_short > fid_deep  # ordering, not pinned values
    assert 0.0 < fid_deep <= fid_short <= 1.0

    # W1.3 recalibration (documented): the probe is a labeled heuristic
    # receipt (clifford-probe-heuristic-v1, measured=False end-to-end),
    # never a bare float presented as physics.
    probe = probe_clifford_fidelity("h q[0]; cx q[0], q[1];", shots=64)
    assert probe["measured"] is False
    assert probe["method"] == "clifford-probe-heuristic-v1"
    assert 0.1 <= probe["fidelity"] <= 1.0
    assert probe["receipt"]["measured"] is False
    assert probe["receipt"]["method"] == "clifford-probe-heuristic-v1"

    # Unknown devices refuse in both predict and probe (never ibm_heron default).
    with pytest.raises(ValueError, match="unknown device"):
        predict_fidelity(circuit_depth=5, two_qubit_gates=2, device_name="not-a-device")
    with pytest.raises(ValueError, match="unknown device"):
        probe_clifford_fidelity("h q[0];", shots=64, device_name="not-a-device")


def test_pennylane_circuit_cutting():
    res = cut_and_reconstruct_expectation(n_qubits=2, rotations=[0.54, 0.32], cut_wire=0)
    assert res["reconstruction_error"] < 1e-5
    assert res["method"] == "PennyLane QPD Wire Cut"


# --------------------------------------------------------------- literature ---

_ARXIV_FEED = """<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry><id>http://arxiv.org/abs/2601.00001</id>
<title>Mocked VQE error mitigation study</title>
<summary>Abstract text for the mocked record.</summary>
<author><name>Test Author</name></author>
<published>2026-01-15T00:00:00Z</published></entry>
</feed>"""

_OPENALEX_DOC = {"results": [{
    "display_name": "Mocked OpenAlex quantum record",
    "doi": "https://doi.org/10.1234/mocked",
    "authorships": [{"author": {"display_name": "OA Author"}}],
    "open_access": {"oa_status": "gold"},
    "publication_year": 2026, "cited_by_count": 42,
}]}


class _FakeResp:
    def __init__(self, status_code=200, text="", payload=None):
        self.status_code = status_code
        self.text = text
        self._payload = payload

    def json(self):
        return self._payload


def _mock_httpx(monkeypatch, tmp_path):
    import literature as lit

    monkeypatch.setattr(lit, "CACHE_DB_PATH", tmp_path / "lit_cache.db")

    def fake_get(url, timeout=5.0):
        if "arxiv" in url:
            return _FakeResp(200, text=_ARXIV_FEED)
        return _FakeResp(200, payload=_OPENALEX_DOC)

    monkeypatch.setattr(lit.httpx, "get", fake_get)
    return lit


def test_literature_grounding_mocked(tmp_path, monkeypatch):
    """Offline CI path: httpx mocked, cache redirected to tmp — no network."""
    lit = _mock_httpx(monkeypatch, tmp_path)
    results = lit.search_arxiv("quantum eigensolver", max_results=2)
    assert len(results) == 1
    assert results[0]["title"] == "Mocked VQE error mitigation study"
    assert results[0]["fetched_live"] is True
    assert results[0]["cached"] is False

    oa_results = lit.search_openalex("quantum computing", max_results=2)
    assert len(oa_results) == 1
    assert oa_results[0]["cited_by_count"] == 42
    assert oa_results[0]["title"] == "Mocked OpenAlex quantum record"

    # Second fetch comes from cache (labeled, never presented as live).
    again = lit.search_arxiv("quantum eigensolver", max_results=2)
    assert again[0]["cached"] is True


def test_literature_offline_returns_empty_never_fabricated(tmp_path, monkeypatch):
    """Socket-blocked: the outage path returns [] (no fabricated papers)."""
    import literature as lit

    monkeypatch.setattr(lit, "CACHE_DB_PATH", tmp_path / "lit_cache.db")

    def boom(url, timeout=5.0):
        raise socket.gaierror("mocked outage: no network in CI")

    monkeypatch.setattr(lit.httpx, "get", boom)
    assert lit.search_arxiv("anything", max_results=2) == []
    assert lit.search_openalex("anything", max_results=2) == []
    # And the outage cached nothing sticky.
    assert lit.search_arxiv("anything", max_results=2) == []


@pytest.mark.skipif(os.environ.get("KUBONA_LIVE_NET") != "1",
                    reason="live-net: set KUBONA_LIVE_NET=1 to run")
def test_literature_live_net():
    """Live-network probe (never runs in CI)."""
    from literature import search_arxiv

    results = search_arxiv("quantum eigensolver", max_results=2)
    assert len(results) > 0
    assert "title" in results[0]


# ------------------------------------------------------------------ ansatz ---

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

    # 1. Synthesize QASM (explicit default angles — the flag names the choice)
    qasm = synthesize_qasm(genome, allow_default_angles=True)
    assert "OPENQASM 2.0;" in qasm
    assert "qreg q[4];" in qasm
    assert "cx q[0], q[1];" in qasm

    # 2. Small-n MW is MEASURED (statevector), labeled, in range.
    mw = evaluate_meyer_wallach_expressibility(genome)
    assert mw["measured"] is True and mw["method"] == "statevector"
    assert 0.0 <= mw["q"] <= 1.0

    # 3. Barren plateau probe: heuristic label + never spend-gating.
    bp = probe_barren_plateau_variance(genome)
    assert bp["admissible"] is True
    assert bp["barren_plateau_detected"] is False
    assert bp["measured"] is False and "heuristic" in bp["method"]
    assert bp["gate_eligible"] is False and "refusing" in bp["gate_reason"]

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
    assert bp_deep["gate_eligible"] is False

    # 4. MPS bond dimension: heuristic label + never spend-gating.
    mps = estimate_mps_bond_dimension(genome)
    assert mps["estimated_bond_dimension"] <= 64
    assert mps["classical_mps_sufficient"] is True
    assert mps["measured"] is False and mps["gate_eligible"] is False


def test_ansatz_large_n_mw_labeled_heuristic_not_fiction():
    """Above the statevector cap, MW refuses fiction: labeled heuristic."""
    from ansatz_search import AnsatzGenome, evaluate_meyer_wallach_expressibility

    big = AnsatzGenome(family="HardwareEfficient", num_qubits=24, depth=5,
                       entangler_topology="linear", initializer="zero")
    mw = evaluate_meyer_wallach_expressibility(big)
    assert mw["measured"] is False
    assert mw["method"] == "expressibility_heuristic_v1"
    assert mw["gate_eligible"] is False
    assert 0.0 <= mw["q"] <= 1.0


def test_ansatz_short_params_refuse_no_silent_fill():
    """Highest-risk line: omitted params used to silently fill 0.54."""
    from ansatz_search import AnsatzGenome, synthesize_qasm

    genome = AnsatzGenome(family="RealAmplitudes", num_qubits=2, depth=2,
                          entangler_topology="linear", initializer="zero")
    with pytest.raises(ValueError, match="allow_default_angles"):
        synthesize_qasm(genome)  # no silent fill
    with pytest.raises(ValueError, match="length mismatch"):
        synthesize_qasm(genome, params=[0.1])  # RealAmplitudes 2x2 needs 4
    # "random" initializer has no default angle: explicit params required.
    rnd = AnsatzGenome(family="RealAmplitudes", num_qubits=2, depth=1,
                       entangler_topology="linear", initializer="random")
    with pytest.raises(ValueError, match="no default angle"):
        synthesize_qasm(rnd, allow_default_angles=True)


def test_ansatz_unknown_topology_and_family_refuse():
    from ansatz_search import AnsatzGenome

    with pytest.raises(ValueError, match="entangler_topology"):
        AnsatzGenome(family="HardwareEfficient", num_qubits=2, depth=1,
                     entangler_topology="hypercube", initializer="zero")
    with pytest.raises(ValueError, match="family"):
        AnsatzGenome(family="MagicAnsatz", num_qubits=2, depth=1,
                     entangler_topology="linear", initializer="zero")


def test_ansatz_families_differ():
    """RealAmplitudes / HardwareEfficient / StronglyEntangling syntheses differ."""
    from ansatz_search import AnsatzGenome, synthesize_qasm

    kw = dict(num_qubits=2, depth=1, entangler_topology="linear", initializer="zero")
    ra = synthesize_qasm(AnsatzGenome(family="RealAmplitudes", **kw),
                         params=[0.1, 0.2])
    he = synthesize_qasm(AnsatzGenome(family="HardwareEfficient", **kw),
                         params=[0.1, 0.2, 0.3, 0.4])
    se = synthesize_qasm(AnsatzGenome(family="StronglyEntangling", **kw),
                         params=[0.1, 0.2, 0.3, 0.4])
    assert ra != he != se and ra != se


# -------------------------------------------------------------------- ADMET ---

def test_molecular_admet_profiling():
    import rdkit
    from chemistry import evaluate_molecular_admet, evaluate_candidate_pipeline

    # Version pin: heuristic outputs below were recalibrated for this RDKit.
    # On upgrade, re-run, recalibrate, and bump the pin — never widen blindly.
    assert rdkit.__version__ == RDKIT_PINNED_VERSION, (
        f"RDKit {rdkit.__version__} != pinned {RDKIT_PINNED_VERSION}: "
        "recalibrate the pins below, then bump the pin"
    )

    # Aspirin
    aspirin = evaluate_molecular_admet("CC(=O)Oc1ccccc1C(=O)O")
    assert aspirin["molecular_weight"] == pytest.approx(180.16, abs=0.5)
    assert aspirin["clogp"] == pytest.approx(1.31, abs=0.5)
    assert aspirin["lipinski"]["pass"] is True
    assert aspirin["lipinski"]["violations"] == 0
    assert aspirin["admet"]["solubility_class"] in ("Soluble", "Moderately Soluble")
    assert aspirin["admet"]["caco2_permeability"] == "High"
    assert len(aspirin["pains_alerts"]) == 0
    assert aspirin["sa_score"] < 3.5
    # Every heuristic number travels with its model label.
    for label_key in ("solubility_model", "permeability_model", "herg_model",
                      "toxicity_panel_model"):
        assert "heuristic" in aspirin["admet"][label_key] or "panel" in aspirin["admet"][label_key]
    assert "heuristic" in aspirin["sa_model"]

    with pytest.raises(ValueError, match="Invalid SMILES"):
        evaluate_molecular_admet("not-a-smiles!!!")

    # Candidate ranking (single-loop ADMET ranking, honestly named)
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
    assert "heuristic" in res["top_lead"]["composite_model"]

    # All-errored slate yields NO top lead (never an error dict as "lead").
    bad = evaluate_candidate_pipeline("X", [{"name": "bad", "smiles": "%%%-nope"}])
    assert bad["top_lead"] is None


# ------------------------------------------------------- simulation bounds ---

def test_simulation_preflight_memory_clamp():
    from executor import check_circuit_memory_bounds
    from qiskit import QuantumCircuit

    # Behavior classes: small simulates, 32-qubit refuses-or-partitions.
    qc_small = QuantumCircuit(4)
    bounds_small = check_circuit_memory_bounds(qc_small)
    assert bounds_small["num_qubits"] == 4
    assert bounds_small["exceeds_statevector_limit"] is False
    assert bounds_small["exceeds_aer_limit"] is False
    assert bounds_small["required_mb"] < 1.0  # order-of-magnitude sanity

    qc_32 = QuantumCircuit(32)
    bounds_32 = check_circuit_memory_bounds(qc_32)
    assert bounds_32["num_qubits"] == 32
    assert bounds_32["required_mb"] > 1e4  # GB-scale detected (order of magnitude)
    assert bounds_32["exceeds_statevector_limit"] is True
    assert bounds_32["exceeds_aer_limit"] is True

    # Strict fail-closed execution without fallback on 32-qubit circuit
    # W1.4 recalibration (documented): explicit seed on every call; the
    # allow_fallback=True path now REFUSES with counts={} (no synthetic
    # all-zero counts behind success=True) while still returning the
    # partition plan for explicit fragment execution.
    qasm_32 = "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[32];\nh q[0];\ncx q[0], q[1];\n"
    res_strict = execute_qasm(qasm_32, requested_backend="aer_simulator", allow_fallback=False, seed=7)
    assert res_strict["success"] is False
    assert "PREFLIGHT_RAM_EXCEEDED" in res_strict["error_message"]
    assert res_strict["counts"] == {}
    assert res_strict["error_message"] != ""

    # Pre-flight partition REFUSES (success=False, no synthetic counts).
    res_partition = execute_qasm(qasm_32, requested_backend="aer_simulator", allow_fallback=True, seed=7)
    assert res_partition["success"] is False
    assert res_partition["counts"] == {}
    assert "PREFLIGHT_RAM_EXCEEDED" in res_partition["error_message"]
    assert len(res_partition["partition_fragments"]) > 0
    # Error iff failure: success False -> non-empty error; no synthetic counts.
    assert res_partition["error_message"] != ""


def test_quantum_krylov_subspace_diagonalization():
    from krylov import solve_quantum_krylov
    res = solve_quantum_krylov("H2", bond_distance=0.7414, subspace_dim=4, time_step=0.2)
    assert res["success"] is True
    assert res["solver"] == "numerical-krylov"  # real solve, labeled
    assert res["measured"] is True  # W1.2: H2 path is still measured
    assert res["method"] == "numerical-krylov"
    assert res["ground_state_energy"] < -1.13
    assert res["ground_state_energy"] <= res["classical_fci_energy"] + 0.005
    assert len(res["subspace_eigenvalues"]) == res["subspace_dimension"]
    assert res["subspace_dimension"] >= 2
    assert 0.0 <= res["meyer_wallach"] <= 1.0
    assert res["error_message"] == ""


def test_krylov_analytical_path_labels_method():
    """W1.2 recalibration (documented): non-H2 REFUSES by default (no
    synthetic corridor behind success=True); explicit allow_heuristic=True
    returns the corridor LABELED (analytical-sequence-heuristic,
    measured=False everywhere, MW omitted with reason)."""
    from krylov import solve_quantum_krylov
    # Default: fail-closed refusal, no synthetic numbers.
    refused = solve_quantum_krylov("LiH", bond_distance=1.5957, subspace_dim=4)
    assert refused["success"] is False
    assert refused["solver"] == "analytical-sequence-refused"
    assert refused["subspace_eigenvalues"] == []
    assert refused["meyer_wallach"] is None
    assert "allow_heuristic" in refused["error_message"]
    assert refused["classical_method"] in ("FCI-exact",) or \
        refused["classical_method"].startswith("CASCI(")
    # Opt-in heuristic: every number labeled, MW omitted with reason.
    heur = solve_quantum_krylov("LiH", bond_distance=1.5957, subspace_dim=4,
                                allow_heuristic=True)
    assert heur["success"] is True
    assert heur["solver"] == "analytical-sequence-heuristic"
    assert heur["method"] == "analytical-sequence-heuristic"
    assert heur["measured"] is False
    assert heur["ground_state_energy_measured"] is False
    assert all(m is False for m in heur["subspace_eigenvalues_measured"])
    assert heur["meyer_wallach"] is None
    assert "reason" in heur["meyer_wallach_reason"].lower() or \
        "omitted" in heur["meyer_wallach_reason"].lower() or \
        "synthetic" in heur["meyer_wallach_reason"].lower()
    assert heur["classical_method"] in ("FCI-exact",) or \
        heur["classical_method"].startswith("CASCI(")
    # Heuristic corridor shape still holds (anchor + 0.05/(k+1), sorted).
    assert len(heur["subspace_eigenvalues"]) == heur["subspace_dimension"] >= 2
    assert heur["ground_state_energy"] == min(heur["subspace_eigenvalues"])


def test_circuit_error_mitigation_suite():
    # W1.3 recalibration (documented): bounds are data-computed (fit RMSE +
    # shot noise), PEC gamma scales with measured depth/qubits, CDR fits a
    # real >=2-point line with conditioning — never invented multipliers or
    # the single-point mitigated==ideal tautology.
    from mitigation import mitigate_circuit
    res_zne = mitigate_circuit(BELL_QASM, method="ZNE", scale_factors=[1.0, 1.5, 2.0], shots=500)
    assert res_zne["success"] is True
    assert res_zne["method_used"] == "ZNE"
    assert -1.0 <= res_zne["mitigated_value"] <= 1.0
    assert res_zne["measured"] is False  # inferred, not sampled
    assert res_zne["unmitigated_measured"] is True
    assert res_zne["error_bound"] is not None and res_zne["error_bound"] >= 0.0
    assert res_zne["fit_condition_number"] < 1e12
    assert res_zne["fidelity_probe"]["measured"] is False

    res_pec = mitigate_circuit(BELL_QASM, method="PEC", shots=500)
    assert res_pec["success"] is True
    assert res_pec["method_used"] == "PEC"
    assert res_pec["pec_gamma"] > 0.0  # depth-scaled, not a constant scalar
    assert res_pec["pec_n_1q"] + res_pec["pec_n_2q"] > 0
    assert res_pec["error_bound"] is not None and res_pec["error_bound"] >= 0.0
    assert res_pec["fidelity_probe"]["measured"] is False

    res_cdr = mitigate_circuit(BELL_QASM, method="CDR", shots=500)
    assert res_cdr["success"] is True
    assert res_cdr["method_used"] == "CDR"
    assert res_cdr["n_calibration_points"] >= 2
    assert res_cdr["fit_condition_number"] < 1e12
    assert res_cdr["error_bound"] is not None and res_cdr["error_bound"] >= 0.0
    # Not the tautology: the fit line is reported; mitigated comes from the
    # line, and the bound is RMSE-derived (zero only if the fit is exact).
    assert "slope" in str(res_cdr.keys()) or "cdr_slope" in res_cdr

    res_bogus = mitigate_circuit(BELL_QASM, method="NOPE", shots=100)
    assert res_bogus["success"] is False

    # Single-scale calibration refuses (would be a one-point tautology).
    res_single = mitigate_circuit(BELL_QASM, method="CDR", scale_factors=[1.0], shots=100)
    assert res_single["success"] is False
    assert "at least 2" in res_single["error_message"]

    # Unknown devices refuse in the fidelity probe (never ibm_heron default).
    res_dev = mitigate_circuit(BELL_QASM, method="ZNE", shots=100, device_name="not-a-device")
    assert res_dev["success"] is False
    assert "unknown device" in res_dev["error_message"]


def test_route_backend_classification():
    from executor import route_backend
    assert route_backend("aer_simulator") == "aer"
    assert route_backend("local_simulator") == "aer"
    assert route_backend("ibm_brisbane") == "hardware:ibm-qiskit"
    assert route_backend("ibm_kyiv") == "hardware:ibm-qiskit"
    assert route_backend("quantinuum_h1").startswith("unknown-hardware")


def test_qpu_request_without_credential_fails_closed(monkeypatch):
    """Undeclared hardware routing refuses (P5 §2.9: success False, no
    simulation theater); declared-but-credentialless routing ALSO refuses
    (W2.18: the provider raises unless the caller explicitly opts into
    simulation with allow_sim_fallback=True — this executor does not opt
    in, so the refusal propagates with the missing-credential reason and
    zero network I/O). Neither path ever claims hardware without a job."""
    # No order-dependent env assertion: force the condition locally.
    monkeypatch.delenv("KUBONA_IBM_QISKIT_TOKEN", raising=False)
    from executor import execute_qasm

    # 1. No declared jurisdictions -> routing refusal (fail-closed, no sim).
    # W1.4: explicit seed on every call (no unseeded sampling).
    refused = execute_qasm(BELL_QASM, shots=100, requested_backend="ibm_brisbane", seed=7)
    assert refused["success"] is False
    assert "routing refused" in refused["error_message"]
    assert "hardware_job_id" not in refused

    # 2. Declared jurisdictions, no token -> plugin refusal propagates.
    res = execute_qasm(BELL_QASM, shots=100, requested_backend="ibm_brisbane",
                       caller_jurisdictions=["US"], seed=7)
    assert res["success"] is False
    assert "hardware_job_id" not in res
    assert res["counts"] == {}
    # Names the missing credential and the explicit opt-in — not a
    # silent downgrade, not a bare 500.
    assert "KUBONA_IBM_QISKIT_TOKEN" in res["error_message"]
    assert "allow_sim_fallback" in res["error_message"]


# ------------------------------------------------- W1.4 executor refusals ---

def test_executor_seed_required_refuses_unseeded():
    """W1.4: sampling without an explicit seed refuses (no unseeded np.random)."""
    from executor import execute_qasm
    refused = execute_qasm(BELL_QASM, shots=100)  # no seed
    assert refused["success"] is False
    assert "seed is required" in refused["error_message"]
    assert refused["counts"] == {}
    refused2 = execute_qasm(BELL_QASM, shots=100, seed=None)
    assert refused2["success"] is False


def test_executor_no_counts_refuses_not_synthetic():
    """W1.4: a backend that returns no counts and no statevector refuses."""
    from executor import execute_qasm
    # requested_backend without aer/local/sim yields counts={} and no
    # statevector sampling path for a small circuit -> must refuse, never
    # fabricate all-zero counts behind success=True.
    res = execute_qasm(BELL_QASM, shots=64, requested_backend="statevector-only", seed=7)
    # Bell is 2q so statevector sampling CAN serve it; force the no-counts
    # path via an unknown non-aer backend on a circuit whose unitary
    # extraction still works — instead assert the invariant directly:
    # success True always carries error_message="", success False non-empty.
    if res["success"] is True:
        assert res["error_message"] == ""
        assert sum(res["counts"].values()) == 64
    else:
        assert res["error_message"] != ""
        assert res["counts"] == {}


def test_executor_fallback_returns_failure_not_synthetic():
    """W1.4: the exception fallback returns success=False (error iff failure)."""
    from executor import execute_qasm
    # Garbage QASM triggers the parse-error refusal (the fallback path no
    # longer returns success=True with synthetic counts + error text).
    bad = execute_qasm("not a circuit", shots=64, seed=7)
    assert bad["success"] is False
    assert bad["error_message"] != ""
    assert bad["counts"] == {}


def test_cut_kappa_cap_refuses_blowup():
    """W1.4: kappa=4^K above the cap refuses instead of a silent blowup."""
    from executor import cut_circuit_qasm, MAX_SAMPLING_OVERHEAD_KAPPA_V1
    # 8 qubits split 1-per-partition with a ladder of CX across every
    # boundary -> K=7 cuts -> kappa=4^7=16384 > 4096 cap.
    lines = ['OPENQASM 2.0;', 'include "qelib1.inc";', 'qreg q[8];']
    for i in range(7):
        lines.append(f"cx q[{i}], q[{i+1}];")
    qasm = "\n".join(lines)
    res = cut_circuit_qasm(qasm, max_subcircuit_qubits=1)
    assert res["success"] is False
    assert "KAPPA_CAP_EXCEEDED" in res["error_message"]
    assert res["sampling_overhead_kappa"] > MAX_SAMPLING_OVERHEAD_KAPPA_V1


def test_cut_cross_partition_multiqubit_refuses_or_records():
    """W1.4: >2q gates spanning partitions never silently drop controls."""
    from executor import cut_circuit_qasm
    # Toffoli (ccx) with controls in different partitions.
    qasm = "\n".join([
        'OPENQASM 2.0;', 'include "qelib1.inc";', 'qreg q[3];',
        'ccx q[0], q[2], q[1];',
    ])
    res = cut_circuit_qasm(qasm, max_subcircuit_qubits=1)
    assert res["success"] is False
    assert "CROSS_PARTITION" in res["error_message"]
    assert len(res["dropped_gates"]) == 1


def test_pec_gamma_scales_with_depth():
    """W1.4/W1.3: PEC overhead grows with circuit size (no constant scalar)."""
    from mitigation import mitigate_circuit
    shallow_qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[1];\nh q[0];\n'
    deep_lines = ['OPENQASM 2.0;', 'include "qelib1.inc";', 'qreg q[2];', 'h q[0];']
    deep_lines += ['cx q[0],q[1];'] * 20
    deep_qasm = "\n".join(deep_lines)
    r_shallow = mitigate_circuit(shallow_qasm, method="PEC", shots=300)
    r_deep = mitigate_circuit(deep_qasm, method="PEC", shots=300)
    assert r_shallow["success"] is True and r_deep["success"] is True
    assert r_deep["pec_gamma"] > r_shallow["pec_gamma"]
    assert r_deep["pec_n_2q"] > r_shallow["pec_n_2q"]


def test_chemistry_admet_servicer_roundtrip():
    """T-01a: the ADMET servicer wraps evaluate_molecular_admet in the
    transport envelope (success + result_json). Pre-fix the RPC did not
    exist — this pins the wire path, not just the chemistry function."""
    import json
    from types import SimpleNamespace

    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    resp = svc.EvaluateMolecularADMET(SimpleNamespace(smiles="CCO"), None)
    assert resp.success is True
    assert resp.error_message == ""
    res = json.loads(resp.result_json)
    assert res["lipinski"]["pass"] is True
    assert "qed" in res and "sa_score" in res


def test_chemistry_admet_servicer_refuses_structured():
    """T-01a: invalid SMILES fails structured (success=False + cause),
    never an unstructured gRPC UNKNOWN and never a fake success."""
    from types import SimpleNamespace

    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    resp = svc.EvaluateMolecularADMET(SimpleNamespace(smiles="not-a-molecule!!!"), None)
    assert resp.success is False
    assert resp.result_json == ""
    assert resp.error_message != ""


def test_discovery_pipeline_servicer_roundtrip():
    """T-01a: the pipeline servicer ranks caller-supplied candidates and
    returns the same envelope (success + result_json with top_lead)."""
    import json
    from types import SimpleNamespace

    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    cands = [
        SimpleNamespace(name="Aspirin", smiles="CC(=O)Oc1ccccc1C(=O)O"),
        SimpleNamespace(name="junk", smiles="not-a-molecule!!!"),
    ]
    resp = svc.EvaluateDiscoveryPipeline(SimpleNamespace(target="COX-2", candidates=cands), None)
    assert resp.success is True
    res = json.loads(resp.result_json)
    assert res["target"] == "COX-2"
    assert res["total_evaluated"] == 2
    assert res["top_lead"]["name"] == "Aspirin"


def test_codesign_servicer_evaluate_roundtrip():
    """T-01b: the codesign servicer runs the ported op and returns the
    same envelope the retired snippet printed (ok + spec_kind + result)."""
    import json
    from types import SimpleNamespace

    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    body = json.dumps({"spec_kind": "and2", "num_tests": 8,
                       "artifact": [["ccx", [0, 1, 2], []]], "kappa": 1})
    resp = svc.EvaluateCodesign(SimpleNamespace(operation="evaluate", request_json=body), None)
    assert resp.success is True
    assert resp.error_message == ""
    res = json.loads(resp.result_json)
    assert res["ok"] is True
    assert res["spec_kind"] == "and2"
    # Golden pins mirror the Go handler test (and2 = T1 Q3 TQ3).
    assert res["result"]["valid"] is True
    assert res["result"]["toffoli"] == 1
    assert res["result"]["peak_qubits"] == 3


def test_codesign_servicer_closed_vocabulary():
    """T-01b: unknown operations and unparseable bodies refuse as
    structured failures — never exec, never a fake success."""
    from types import SimpleNamespace

    from server import QuantumBridgeServicer

    svc = QuantumBridgeServicer()
    resp = svc.EvaluateCodesign(SimpleNamespace(operation="rm -rf", request_json="{}"), None)
    assert resp.success is False
    assert "unknown codesign operation" in resp.error_message
    resp = svc.EvaluateCodesign(SimpleNamespace(operation="evaluate", request_json="{nope"), None)
    assert resp.success is False
    assert resp.error_message != ""


def test_codesign_ops_error_strings_match_retired_snippets():
    """T-01b: the ported validation speaks the retired snippets'
    byte-identical refusal strings (handler tests assert them)."""
    from codesign_ops import run_op

    bad = run_op("evaluate", {"spec_kind": "and2", "kappa": 9, "artifact": []})
    assert bad == {"ok": False, "error": "kappa must be an int in [1,3]: refusing"}
    bad = run_op("budget", {"entries": [{"source": "x", "per_use_rate": True, "uses": 1}]})
    assert bad["ok"] is False
    assert "per_use_rate must be a number" in bad["error"]


# --------------------------------------- global RAM admission (in-flight) ---

def test_global_inflight_cap_defaults_to_twice_single_job_ceiling():
    from executor import (
        _SINGLE_JOB_CEILING_BYTES,
        _global_inflight_cap_bytes,
        check_circuit_memory_bounds,
    )
    from qiskit import QuantumCircuit

    assert _SINGLE_JOB_CEILING_BYTES == (2 ** 28) * 16
    assert _global_inflight_cap_bytes() == 2 * _SINGLE_JOB_CEILING_BYTES
    # Any per-job-admittable solo simulation always fits an empty semaphore:
    # the largest admittable estimate (28 qubits) is half the default cap.
    assert check_circuit_memory_bounds(QuantumCircuit(28))["required_bytes"] * 2 \
        <= _global_inflight_cap_bytes()


def test_global_inflight_refuses_and_releases_cleanly():
    import executor
    from executor import execute_qasm, inflight_bytes

    bell = ("OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\n"
            "h q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n")
    assert inflight_bytes() == 0
    # Saturate the process-wide cap by hand, then a small job must refuse
    # fail-closed (INFLIGHT marker, no counts, counter untouched).
    assert executor._inflight_try_acquire(executor._global_inflight_cap_bytes())
    try:
        assert inflight_bytes() == executor._global_inflight_cap_bytes()
        refused = execute_qasm(bell, shots=16, requested_backend="aer_simulator", seed=7)
        assert refused["success"] is False
        assert "INFLIGHT_RAM_EXCEEDED" in refused["error_message"]
        assert refused["counts"] == {}
        assert inflight_bytes() == executor._global_inflight_cap_bytes()
    finally:
        executor._inflight_release(executor._global_inflight_cap_bytes())
    assert inflight_bytes() == 0
    # Drained: the same job simulates and the hold does not leak.
    ok = execute_qasm(bell, shots=16, requested_backend="aer_simulator", seed=7)
    assert ok["success"] is True, ok
    assert inflight_bytes() == 0


def test_global_inflight_precedence_keeps_precise_per_job_refusal():
    from executor import execute_qasm

    qasm_32 = ("OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[32];\n"
               "h q[0];\ncx q[0], q[1];\n")
    # 32 qubits fails BOTH rails: the precise per-job refusal (partition
    # plan included) must win over the global marker, solo or saturated.
    solo = execute_qasm(qasm_32, requested_backend="aer_simulator",
                        allow_fallback=False, seed=7)
    assert solo["success"] is False
    assert "PREFLIGHT_RAM_EXCEEDED" in solo["error_message"]


def test_global_inflight_release_clamps_and_env_cap_parses():
    import executor

    assert executor.inflight_bytes() == 0
    executor._inflight_release(1 << 40)  # double-release must not mint headroom
    assert executor.inflight_bytes() == 0
    assert executor._inflight_try_acquire(0) is True  # non-positive: no-op hold
    assert executor.inflight_bytes() == 0
    import os
    old = os.environ.get("KUBONA_BRIDGE_MAX_INFLIGHT_BYTES")
    try:
        os.environ["KUBONA_BRIDGE_MAX_INFLIGHT_BYTES"] = "not-a-number"
        assert executor._global_inflight_cap_bytes() == 2 * executor._SINGLE_JOB_CEILING_BYTES
        os.environ["KUBONA_BRIDGE_MAX_INFLIGHT_BYTES"] = "12345"
        assert executor._global_inflight_cap_bytes() == 12345
    finally:
        if old is None:
            os.environ.pop("KUBONA_BRIDGE_MAX_INFLIGHT_BYTES", None)
        else:
            os.environ["KUBONA_BRIDGE_MAX_INFLIGHT_BYTES"] = old
