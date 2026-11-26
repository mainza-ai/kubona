"""Offline proofs for Phase 0 (ZNE A1/Z1–Z2), Phase 1 (QWC + Z2 symmetry + per-PUB shots),
and Phase 2 (Same-batch ODR CDR + live 4x4 asymmetric readout under calibrated non-unital Aer noise).
"""

from __future__ import annotations

import math
import sys
from pathlib import Path

import numpy as np
import pytest

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

from kubona.mitigation.cdr import (  # noqa: E402
    build_assignment_matrix,
    build_readout_cal_circuits,
    evaluate_cdr_batch,
    fit_cdr_odr,
    generate_cdr_training_circuits,
)
from kubona.mitigation.qwc import (  # noqa: E402
    allocate_neyman_shots,
    estimate_energy_from_qwc_counts,
    group_pauli_terms,
    measurement_circuit_for_basis,
)
from kubona.mitigation.symmetry import (  # noqa: E402
    detect_z_symmetries,
    postselect_z_counts,
    symmetry_verified_energy,
)

# H2 STO-3G at 0.735 A (exact 2-qubit parity-mapped Hamiltonian)
H2_COEFFS = {
    "II": -0.4228698512159958,
    "IZ": -0.39793742484318045,
    "ZI": -0.39793742484318045,
    "ZZ": 0.10175096627051738,
    "XX": 0.09046559989211572,
    "YY": -0.09046559989211572,
}
H2_FCI_EXACT = -1.1373060357533995


def test_qwc_grouping_drops_identity_and_computes_exact_joint_covariance():
    grouped = group_pauli_terms(H2_COEFFS)
    assert grouped["identity_offset"] == pytest.approx(H2_COEFFS["II"])
    bases = [g["basis"] for g in grouped["groups"]]
    assert bases == ["ZZ", "XX", "YY"]
    assert set(grouped["groups"][0]["terms"]) == {"IZ", "ZI", "ZZ"}

    # Exact joint covariance check: for |00> with probability 0.5 and |11> with
    # probability 0.5, <IZ> = <ZI> = 0, <ZZ> = +1, and IZ + ZI = ±2 identically.
    # Treating IZ and ZI as independent would give Var = (c_IZ^2 + c_ZI^2)/N;
    # exact joint covariance captures the +2*c_IZ*c_ZI correlation!
    n = 1000
    counts_z = {"00": 500, "11": 500}
    est = estimate_energy_from_qwc_counts(
        grouped,
        [counts_z, {"00": 500, "11": 500}, {"00": 500, "11": 500}],
    )
    # X(00) = -0.39794 - 0.39794 + 0.10175, X(11) = +0.39794 + 0.39794 + 0.10175
    # Half-difference = 2 * 0.39793742484318045 = 0.79587485 => Var_Z = 0.79587485^2 / (n - 1)
    expected_var_z = (2.0 * abs(H2_COEFFS["IZ"])) ** 2 / float(n - 1)
    assert est["groups"][0]["std_error"] == pytest.approx(math.sqrt(expected_var_z), rel=1e-6)
    # Independent approximation would be sqrt(2) times smaller (~0.0178 vs ~0.0252):
    assert est["groups"][0]["std_error"] > 1.35 * math.sqrt(2.0 * H2_COEFFS["IZ"] ** 2 / n)

    alloc = allocate_neyman_shots(grouped["groups"], 98304, min_shots=4096)
    assert sum(alloc) == 98304
    assert alloc[0] > alloc[1] and alloc[0] > alloc[2]


def test_z2_symmetry_detection_and_bonet_monroig_exact_projector():
    # Ground state of H2 STO-3G: cos(phi)|00> - sin(phi)|11>, strictly ZZ = +1
    phi = 0.1117
    state = np.array([math.cos(phi), 0.0, 0.0, -math.sin(phi)], dtype=complex)
    syms = detect_z_symmetries(H2_COEFFS, target_state=state)
    assert syms == [{"pauli": "ZZ", "eigenvalue": 1}]

    grouped = group_pauli_terms(H2_COEFFS)
    # Corrupt Z counts with 4% single-qubit bit-flip leakage into |01>/|10>
    z_counts = {"00": 9480, "11": 120, "01": 200, "10": 200}
    x_counts = {"00": 4500, "11": 4400, "01": 550, "10": 550}
    y_counts = {"00": 550, "11": 550, "01": 4500, "10": 4400}
    kept, acc = postselect_z_counts(z_counts, syms)
    assert set(kept) == {"00", "11"}
    assert acc == pytest.approx(0.96)

    raw = estimate_energy_from_qwc_counts(grouped, [z_counts, x_counts, y_counts])
    sv = symmetry_verified_energy(grouped, [z_counts, x_counts, y_counts], H2_COEFFS, syms)
    assert sv["projector_exact"] is True
    assert sv["method"] == "bonet_monroig_exact_projector"
    assert sv["survival_probability"] == pytest.approx(0.96)
    # Symmetry verification removes the bit-flip bias toward the FCI energy
    assert abs(sv["energy"] - H2_FCI_EXACT) < abs(raw["energy"] - H2_FCI_EXACT)
    assert sv["uncertainty"] > 0.0


def test_readout_assignment_matrix_inversion_and_ill_conditioned_refusal():
    # Asymmetric 2-qubit readout matrix with P(1|0)=0.01, P(0|1)=0.04
    cals = [
        {"00": 980, "01": 10, "10": 10, "11": 0},
        {"00": 40, "01": 950, "10": 0, "11": 10},
        {"00": 40, "01": 0, "10": 950, "11": 10},
        {"00": 2, "01": 38, "10": 38, "11": 922},
    ]
    res = build_assignment_matrix(cals, 2, max_condition_number=50.0)
    assert res["ok"] is True
    assert 1.0 < res["condition_number"] < 5.0
    assert res["per_qubit_asymmetry"]["0"]["p0_given_1"] > res["per_qubit_asymmetry"]["0"]["p1_given_0"]

    # Singular/collapsed readout matrix must fail closed
    bad = [{"00": 500, "01": 500}] * 4
    refused = build_assignment_matrix(bad, 2, max_condition_number=50.0)
    assert refused["ok"] is False
    assert "ill-conditioned" in refused["error"]


def test_cdr_odr_coupled_variance_and_zero_attenuation_bias():
    rng = np.random.default_rng(42)
    true_alpha, true_beta = 1.08, -0.025
    # Inverse generative model: x_noisy = (y_exact - beta) / alpha + eps
    y_exact = np.linspace(-1.13, -0.35, 12)
    sx = np.full(12, 0.0020)
    x_noisy = (y_exact - true_beta) / true_alpha + rng.normal(0.0, sx)
    target_exact = -1.1373
    target_se = 0.0008
    target_noisy = (target_exact - true_beta) / true_alpha

    fit = fit_cdr_odr(
        x_noisy.tolist(), y_exact.tolist(), sx.tolist(),
        target_noisy=target_noisy, target_uncertainty=target_se,
    )
    assert fit["fit_method"] == "cdr_eiv_deming_exact"
    assert abs(fit["mitigated"] - target_exact) * 1000.0 < 3.0
    assert fit["r_squared"] > 0.99
    assert fit["loo_cv_mha"] < 5.0
    # Coupled difference CI must be strictly tighter than naive independent sqrt(se_cdr^2 + target_se^2)
    naive_ci95 = 1.959963984540054 * math.sqrt(fit["uncertainty"] ** 2 + target_se ** 2)
    assert fit["difference_vs_input_ci95_ha"] < naive_ci95


def test_phase0_stratified_folding_and_duplicate_2q_scale_refusal(monkeypatch):
    pytest.importorskip("qiskit")
    import experiments

    # H2 layers=2 ansatz has 8 1Q gates + 2 CX = 10 gates.
    qc = experiments._ansatz(np.array([0.2, -0.1, 0.3, 0.4, -0.2, 0.1, -0.3, 0.2]), layers=2)

    def _counts(circ):
        n1 = sum(1 for inst in circ.data if len(inst.qubits) < 2 and inst.operation.name not in ("barrier", "measure", "reset"))
        n2 = sum(1 for inst in circ.data if len(inst.qubits) >= 2 and inst.operation.name not in ("barrier", "measure", "reset"))
        return n1, n2

    # At integer scales [1, 2, 3, 4], 1Q and 2Q scale identically!
    assert _counts(experiments._fold_local_all(qc, 1.0)) == (8, 2)
    assert _counts(experiments._fold_local_all(qc, 2.0)) == (16, 4)
    assert _counts(experiments._fold_local_all(qc, 3.0)) == (24, 6)
    assert _counts(experiments._fold_local_all(qc, 4.0)) == (32, 8)
    # At s=1.5 on 2-CX H2, stratified folding does NOT jump 2Q to 4 (fixing A1!)
    assert _counts(experiments._fold_local_all(qc, 1.5)) == (12, 2)

    # And run_vqe_hardware refuses [1.0, 1.5, 2.0, 3.0] on 2-CX H2 before dispatch!
    def _must_not_dispatch(*args, **kwargs):
        raise AssertionError("must refuse duplicate-2Q ZNE axis before dispatch")

    monkeypatch.setattr(experiments, "_hardware_batch", _must_not_dispatch)
    refused = experiments.run_vqe_hardware(
        theta=[0.2, -0.1, 0.3, 0.4, -0.2, 0.1, -0.3, 0.2],
        pauli_coefficients=H2_COEFFS,
        layers=2, shots=1024, seed=7, bond_distance=0.735,
        fci_exact_energy=H2_FCI_EXACT, mitigation="ZNE",
        mitigation_scales=[1.0, 1.5, 2.0, 3.0],
    )
    assert refused["success"] is False
    assert "2Q gates" in refused["error"]


def test_phase0_zne_receipt_separates_raw_and_trex_baselines(monkeypatch):
    pytest.importorskip("qiskit")
    import experiments

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"00": 950, "01": 50} for _ in qasms],
            "backend": "ibm_fez",
            "hardware_job_id": "hw-z2-test",
            "calibration": {
                "calibration_fresh": True,
                "per_qubit": {"142": {"readout_error": 0.02}, "143": {"readout_error": 0.02}},
            },
            "layout": {"physical_qubits": [142, 143]},
            "transpile": {"circuits": len(qasms), "min_transpiled_two_qubit_gates": 2, "max_transpiled_two_qubit_gates": 6},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    res = experiments.run_vqe_hardware(
        theta=[0.1] * 8, pauli_coefficients=H2_COEFFS, layers=2,
        shots=1000, seed=7, bond_distance=0.735, fci_exact_energy=H2_FCI_EXACT,
        mitigation="ZNE", mitigation_scales=[1, 2, 3], readout_mitigation=True,
    )
    assert res["success"] is True
    mit = res["mitigation"]
    assert "raw_unmitigated_energy" in mit
    assert "readout_mitigated_baseline_energy" in mit
    assert mit["raw_unmitigated_energy"] != pytest.approx(mit["readout_mitigated_baseline_energy"])
    assert "difference_vs_raw_ha" in mit and "difference_vs_baseline_ha" in mit


def test_phase2_non_unital_aer_replay_cdr_passes_chemical_accuracy_across_three_seeds():
    """Offline proof on calibrated IBM Heron non-unital noise (T1/T2 + asymmetric readout + coherent 2Q)."""
    pytest.importorskip("qiskit")
    pytest.importorskip("qiskit_aer")
    from qiskit import QuantumCircuit, transpile
    from qiskit_aer import AerSimulator
    from qiskit_aer.noise import (
        NoiseModel,
        ReadoutError,
        coherent_unitary_error,
        depolarizing_error,
        thermal_relaxation_error,
    )

    # Optimal H2 STO-3G 0.735 A ground-state circuit with layers=2 (8 1Q + 2 CX, same structure as hardware).
    # Layer 1 leaves |00> invariant (modulo phase), Layer 2 prepares exact ground state cos(phi)|00> - sin(phi)|11>.
    phi = 0.111728
    qc = QuantumCircuit(2)
    qc.ry(0.0, 0)
    qc.ry(0.0, 1)
    qc.rz(0.15, 0)
    qc.rz(-0.15, 1)
    qc.cx(0, 1)
    qc.ry(-2.0 * phi, 0)
    qc.ry(0.0, 1)
    qc.rz(0.0, 0)
    qc.rz(0.0, 1)
    qc.cx(0, 1)

    # Verify target ansatz reaches H2 FCI exact energy and lives in ZZ=+1 sector
    from qiskit.quantum_info import Statevector
    st = np.asarray(Statevector.from_instruction(qc).data, dtype=complex)
    syms = detect_z_symmetries(H2_COEFFS, target_state=st)
    assert syms == [{"pauli": "ZZ", "eigenvalue": 1}]

    # Calibrated non-unital Heron-like noise model:
    # - T1 = 180 us, T2 = 120 us (non-unital amplitude damping!)
    # - 1Q gate time 40 ns, 2Q gate time 80 ns + 0.35% 2Q depolarizing + coherent ZZ over-rotation (0.04 rad)
    # - Asymmetric readout: P(1|0)=0.008, P(0|1)=0.032 (strong T1 measurement asymmetry!)
    noise = NoiseModel()
    t1, t2 = 180e-6, 120e-6
    err_1q = thermal_relaxation_error(t1, t2, 40e-9).compose(depolarizing_error(0.0004, 1))
    # Coherent ZZ over-rotation on CX: exp(-i * 0.02 * ZZ)
    eps_coh = 0.02
    u_coh = np.diag([
        np.exp(-1j * eps_coh), np.exp(1j * eps_coh),
        np.exp(1j * eps_coh), np.exp(-1j * eps_coh),
    ])
    err_2q = (
        thermal_relaxation_error(t1, t2, 80e-9)
        .tensor(thermal_relaxation_error(t1, t2, 80e-9))
        .compose(depolarizing_error(0.0035, 2))
        .compose(coherent_unitary_error(u_coh))
    )
    noise.add_all_qubit_quantum_error(err_1q, ["rx", "ry", "rz", "h", "sdg", "x", "sx"])
    noise.add_all_qubit_quantum_error(err_2q, ["cx"])
    ro_err = ReadoutError([[0.992, 0.008], [0.032, 0.968]])
    noise.add_all_qubit_readout_error(ro_err)

    grouped = group_pauli_terms(H2_COEFFS)
    cal_circs = [c for _, c in build_readout_cal_circuits(2)]

    # Run offline proof across 3 independent random seeds!
    for seed in (7, 19, 31):
        sim = AerSimulator(noise_model=noise, seed_simulator=seed)
        train_pkg = generate_cdr_training_circuits(
            qc, H2_COEFFS, n_circuits=12, seed=seed, symmetries=syms, min_distinct_energies=8,
        )
        assert train_pkg["distinct_energies"] >= 8
        assert train_pkg["sector_aligned"] is True
        assert train_pkg["target_in_training_domain"] is True

        def _run_circs(circ_list, shots_list):
            out_counts = []
            for c, s in zip(circ_list, shots_list):
                tc = transpile(c, basis_gates=["rx", "ry", "rz", "h", "sdg", "x", "cx"], optimization_level=0)
                cnt = sim.run(tc, shots=int(s)).result().get_counts()
                out_counts.append({str(k): int(v) for k, v in cnt.items()})
            return out_counts

        target_circs = [measurement_circuit_for_basis(qc, g["basis"]) for g in grouped["groups"]]
        target_shots = [65536, 16384, 16384]
        target_counts = _run_circs(target_circs, target_shots)

        # Neyman shot allocation on training circuits (exact same 12,288 shots/circuit, zero extra QPU cost!)
        train_shots = allocate_neyman_shots(grouped["groups"], 12288, min_shots=1024)
        train_counts_nested = []
        for tqc in train_pkg["circuits"]:
            t_meas = [measurement_circuit_for_basis(tqc, g["basis"]) for g in grouped["groups"]]
            train_counts_nested.append(_run_circs(t_meas, train_shots))

        cal_counts = _run_circs(cal_circs, [8192] * 4)

        ev = evaluate_cdr_batch(
            grouped, H2_COEFFS,
            target_counts, train_counts_nested, train_pkg["exact_energies"],
            fci_exact_energy=H2_FCI_EXACT,
            readout_cal_counts=cal_counts,
            symmetries=syms,
            train_sector_aligned=train_pkg["sector_aligned"],
        )
        assert ev["ok"] is True
        cdr = ev["cdr"]
        raw_delta_mha = ev["ablation_layers"]["raw_unmitigated"]["delta_mha"]
        cdr_delta_mha = ev["ablation_layers"]["cdr_mitigated"]["delta_mha"]
        # Raw non-unital error is large (> 15 mHa); CDR + Live 4x4 RO + Z2 Symmetry hits chemical accuracy!
        assert raw_delta_mha > 10.0, raw_delta_mha
        assert cdr_delta_mha <= 1.6, f"seed={seed}: cdr_delta_mha={cdr_delta_mha:.3f}"
        assert cdr["model_adequate"] is True, f"seed={seed}: {cdr}"
        assert cdr["chi2_reduced"] <= 1.5, f"seed={seed}: chi2={cdr['chi2_reduced']}"
        assert cdr["difference_significant"] is True, f"seed={seed}: diff={cdr['difference_ha']*1000:.3f} ± {cdr['difference_ci95_mha']:.3f}"


def test_phase3_run_vqe_hardware_cdr_single_batch_with_per_pub_shots(monkeypatch):
    """Bridge integration test: run_vqe_hardware(mitigation='CDR') submits one batch with QWC + cal + per-PUB shots."""
    pytest.importorskip("qiskit")
    pytest.importorskip("qiskit_aer")
    from qiskit import qasm2, transpile
    from qiskit_aer import AerSimulator
    from qiskit_aer.noise import NoiseModel, ReadoutError, depolarizing_error, thermal_relaxation_error
    import experiments

    noise = NoiseModel()
    t1, t2 = 180e-6, 120e-6
    noise.add_all_qubit_quantum_error(
        thermal_relaxation_error(t1, t2, 40e-9).compose(depolarizing_error(0.0004, 1)),
        ["rx", "ry", "rz", "h", "sdg", "x", "u3"],
    )
    noise.add_all_qubit_quantum_error(
        thermal_relaxation_error(t1, t2, 80e-9)
        .tensor(thermal_relaxation_error(t1, t2, 80e-9))
        .compose(depolarizing_error(0.0035, 2)),
        ["cx"],
    )
    noise.add_all_qubit_readout_error(ReadoutError([[0.992, 0.008], [0.032, 0.968]]))
    sim = AerSimulator(noise_model=noise, seed_simulator=7)

    calls = []

    def fake_batch(qasm_list, shots, seed, backend=None, shots_per_circuit=None):
        calls.append((len(qasm_list), shots, shots_per_circuit, backend))
        out_counts = []
        for idx, qasm in enumerate(qasm_list):
            s = shots_per_circuit[idx] if shots_per_circuit else shots
            qc_i = transpile(qasm2.loads(qasm), basis_gates=["rx", "ry", "rz", "h", "sdg", "x", "cx"], optimization_level=0)
            cnt = sim.run(qc_i, shots=int(s)).result().get_counts()
            out_counts.append({str(k): int(v) for k, v in cnt.items()})
        return {
            "success": True,
            "counts": out_counts,
            "backend": backend or "ibm_fez",
            "hardware_job_id": "hw-cdr-1",
            "layout": {"physical_qubits": [142, 143]},
            "layout_quality": {"physical_qubits": [142, 143], "max_readout_error": 0.015, "suspicious": False},
            "calibration": {"calibration_fresh": True},
            "error_suppression": {"dynamical_decoupling": True, "gate_twirling": True, "measure_twirling": True},
            "usage_seconds": 12.0,
            "actual_usd": 0.0,
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    phi = 0.111728
    theta = [0.0, 0.0, 0.15, -0.15, -2.0 * phi, 0.0, 0.0, 0.0]
    res = experiments.run_vqe_hardware(
        theta=theta,
        pauli_coefficients=H2_COEFFS,
        layers=2,
        shots=4096,
        seed=7,
        bond_distance=0.735,
        fci_exact_energy=H2_FCI_EXACT,
        backend="ibm_fez",
        mitigation="CDR",
        cdr_training_circuits=12,
        live_readout_cal=True,
        symmetry_verification=True,
        target_shots=[65536, 16384],
    )
    assert res["success"] is True, res
    # Single governed hardware batch: 3 target QWC + 12*3 training QWC + 4 readout cal = 43 circuits!
    assert len(calls) == 1
    assert calls[0][0] == 43
    assert calls[0][2] is not None and calls[0][2][:3] == [65536, 16384, 16384]
    assert res["measured"] is False and res["mitigated"] is True and res["claimable"] is False
    assert res["mitigation"]["method"] == "CDR"
    assert res["mitigation"]["model_adequate"] is True
    assert res["mitigation"]["difference_significant"] is True
    assert res["delta_mha"] <= 1.6, res["delta_mha"]
    assert set(res["ablation_layers"]) == {"raw_unmitigated", "live_readout_mitigated", "symmetry_verified", "cdr_mitigated"}
    assert len(res["cdr_training_counts"]) == 12
    assert set(res["readout_cal_counts"]) == {"00", "01", "10", "11"}
    assert set(res["counts_by_term"]) == {"IZ", "ZI", "ZZ", "XX", "YY"}


def test_phase4_datasheet_renders_cdr_mode_ablation_and_citations():
    import manuscript_datasheet

    rows = [
        {
            "job_id": "job_cdr_1",
            "kind": "vqe_hardware",
            "backend": "ibm_fez",
            "hardware": True,
            "mitigated": True,
            "mitigation_method": "CDR",
            "energy": -1.1368,
            "fci_exact_energy": H2_FCI_EXACT,
            "delta_mha": 0.506,
            "ci95_mha": 1.85,
            "ablation_layers": {
                "raw_unmitigated": {"energy": -1.119, "delta_mha": 18.3, "ci95_mha": 2.4},
                "live_readout_mitigated": {"energy": -1.131, "delta_mha": 6.3, "ci95_mha": 3.1},
                "symmetry_verified": {"energy": -1.133, "delta_mha": 4.3, "ci95_mha": 2.6},
                "cdr_mitigated": {"energy": -1.1368, "delta_mha": 0.506, "ci95_mha": 1.85},
            },
        }
    ]
    cites = {c["key"] for c in manuscript_datasheet.canonical_citations_for_campaign(rows)}
    assert {"Czarnik2021CDR", "BonetMonroig2018"}.issubset(cites)

    tex = manuscript_datasheet.build_zne_and_uncertainty_section(rows)
    assert "CDR mitigated" in tex
    assert "Same-Batch Error-Mitigation Ablation" in tex
    assert "ODR CDR mitigated" in tex


def test_phase1_qiskit_provider_per_pub_shots_dispatches_sampler_v2_tuples(monkeypatch):
    """Verify QiskitProvider.submit_batch realizes heterogeneous shots via GCD uniform PUB replication."""
    pytest.importorskip("qiskit")
    import types
    from kubona.providers.qiskit_provider import QiskitProvider
    from kubona.schema import load_provider

    captured = {}

    class FakePubData:
        def __init__(self, shots):
            self._shots = shots

        def get_counts(self):
            return {"00": self._shots}

    class FakeResultItem:
        def __init__(self, shots):
            self.data = types.SimpleNamespace(meas=FakePubData(shots))

    class FakeJob:
        def __init__(self, shot_list):
            self._shots = shot_list

        def job_id(self):
            return "fake-pub-job-1"

        def result(self):
            return [FakeResultItem(s) for s in self._shots]

    class FakeSampler:
        def __init__(self, mode=None, backend=None):
            self.options = None

        def run(self, pubs, shots=None):
            captured["pubs"] = pubs
            captured["shots_kwarg"] = shots
            assert shots is not None, "IBM executor_sampler requires uniform shots across all PUBs"
            return FakeJob([int(shots)] * len(pubs))

    class FakeBackend:
        name = "ibm_fake_heron"

    class FakeService:
        def backend(self, name):
            return FakeBackend()

    fake_mod = types.ModuleType("qiskit_ibm_runtime")
    fake_mod.SamplerV2 = FakeSampler
    fake_exec_mod = types.ModuleType("qiskit_ibm_runtime.executor_sampler")
    fake_exec_mod.Sampler = FakeSampler
    monkeypatch.setitem(sys.modules, "qiskit_ibm_runtime", fake_mod)
    monkeypatch.setitem(sys.modules, "qiskit_ibm_runtime.executor_sampler", fake_exec_mod)
    monkeypatch.setenv("KUBONA_IBM_QISKIT_TOKEN", "dummy-test-token")

    prov = QiskitProvider(load_provider("ibm-qiskit"), backend_name="ibm_fake_heron")
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0],q[1];\nmeasure q -> c;\n'
    out = prov.submit_batch(
        [qasm, qasm, qasm],
        shots=4096,
        seed=7,
        service_factory=lambda token, **kw: FakeService(),
        shots_per_circuit=[65536, 16384, 4096],
    )
    assert out["success"] is True
    assert out["shots_per_circuit"] == [65536, 16384, 4096]
    assert out["pub_base_shots"] == 4096
    assert out["expanded_pubs"] == 21
    assert captured["shots_kwarg"] == 4096
    assert len(captured["pubs"]) == 21
    assert [sum(c.values()) for c in out["counts"]] == [65536, 16384, 4096]



