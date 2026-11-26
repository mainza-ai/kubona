"""Tests for Upstream Publication Residuals R-A, R-B, R-C, and R-D."""

from __future__ import annotations

import sys
from pathlib import Path
from types import SimpleNamespace

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import manuscript_figures  # noqa: E402
from kubona.providers.qiskit_provider import _snapshot_from_properties  # noqa: E402


def test_r_b_snapshot_from_properties_captures_per_qubit_calibration():
    class FakeProps:
        qubits = [0, 1, 2]
        last_update_date = "2026-11-24T12:00:00Z"
        gates = [
            SimpleNamespace(name="sx", qubits=[0], parameters=[SimpleNamespace(value=0.00025)]),
            SimpleNamespace(name="sx", qubits=[1], parameters=[SimpleNamespace(value=0.00031)]),
            SimpleNamespace(name="cz", qubits=[0, 1], parameters=[SimpleNamespace(value=0.0018)]),
        ]

        def t1(self, q):
            return [2.5e-4, 2.8e-4, 2.2e-4][q]

        def t2(self, q):
            return [1.4e-4, 1.6e-4, 1.2e-4][q]

        def readout_error(self, q):
            return [0.011, 0.014, 0.009][q]

    snap = _snapshot_from_properties(FakeProps(), "ibm_fez")
    assert snap is not None
    assert snap["qubits"] == 3
    assert isinstance(snap["per_qubit"], dict)
    assert set(snap["per_qubit"].keys()) == {"0", "1", "2"}
    assert snap["per_qubit"]["0"]["t1_us"] == 250.0
    assert snap["per_qubit"]["0"]["sx_error"] == 0.00025
    assert snap["per_qubit"]["1"]["readout_error"] == 0.014


def test_r_d_multi_register_and_openqasm3_if_schematic_parsing():
    qasm = (
        'OPENQASM 3.0;\n'
        'qubit[2] data;\n'
        'qubit[2] anc;\n'
        'h data[0];\n'
        'cx data[0], anc[1];\n'
        'if (c == 1) x anc[0];\n'
        'measure anc[1] -> c[0];\n'
    )
    n_q, gates, total, two_q = manuscript_figures._parse_qasm_for_schematic(qasm)
    assert n_q == 4
    assert total == 4
    assert two_q == 1
    # cx data[0], anc[1] maps to global wires [0, 3]
    cx_gate = next(g for g in gates if g[0] == "CX")
    assert cx_gate[1] == [0, 3]
    # OQ3 conditional x anc[0] maps to global wire [2]
    x_gate = next(g for g in gates if g[0] == "X")
    assert x_gate[1] == [2]

    figs = manuscript_figures.generate_campaign_figures(
        [{"job_id": "job_oq3", "kind": "qec_memory_sim", "hardware": True, "shots": 1024, "seed": 7}],
        circuit_samples={"job_oq3": qasm},
    )
    assert any(name == "fig_circuit_qec_memory_sim.png" and raw.startswith(b"\x89PNG\r\n\x1a\n") for name, raw, _ in figs)


def test_h4_snapshot_from_target_fallback_and_h6_prefix_caption():
    from kubona.providers.qiskit_provider import _snapshot_from_target

    class FakeTarget:
        qubit_properties = [
            SimpleNamespace(t1=2.4e-4, t2=1.5e-4),
            SimpleNamespace(t1=2.6e-4, t2=1.3e-4),
        ]

        def __contains__(self, key):
            return key in ("measure", "sx")

        def __getitem__(self, key):
            if key == "measure":
                return {(0,): SimpleNamespace(error=0.012), (1,): SimpleNamespace(error=0.015)}
            if key == "sx":
                return {(0,): SimpleNamespace(error=0.0002), (1,): SimpleNamespace(error=0.0003)}
            return {}

    snap = _snapshot_from_target(FakeTarget(), "ibm_kingston")
    assert snap is not None
    assert snap["qubits"] == 2
    assert snap["per_qubit"]["0"]["t1_us"] == 240.0
    assert snap["per_qubit"]["1"]["readout_error"] == 0.015
    assert snap["per_qubit"]["0"]["sx_error"] == 0.0002

    # H6: KUBONA_PREFIX_TRUNCATED triggers explicit prefix notice in caption
    prefixed = '// KUBONA_PREFIX_TRUNCATED\nOPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    figs = manuscript_figures.generate_campaign_figures(
        [{"job_id": "job_big", "kind": "vqe_hardware", "hardware": True, "shots": 1024, "seed": 7}],
        circuit_samples={"job_big": prefixed},
    )
    assert any("rendered from 32 KiB prefix" in cap for _, _, cap in figs)


def test_r_a_qaoa_simulator_retains_counts_and_derives_ratio_interval():
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    out = experiments.run_qaoa(layers=1, shots=128, seed=7)
    assert out["success"] is True
    assert out["counts"]
    assert out["ratio_ci95"] is not None
    lo, hi = out["ratio_ci95"]
    assert 0.0 <= lo <= out["approximation_ratio"] <= hi <= 1.0
    assert out["ratio_ci_method"] == "normal_from_sampled_cuts"


def test_r_a_hardware_pes_retains_per_term_counts(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"00": 512, "11": 512} for _ in qasms],
            "backend": "fake-qpu",
            "hardware_job_id": "fake-job",
            "calibration": {"calibration_fresh": True},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    points = [
        {"bond_distance": b, "pauli_coefficients": {"XX": 0.5, "ZZ": 0.5},
         "fci_exact_energy": -0.5, "constant_energy": 0.0, "ansatz_qasm": qasm}
        for b in (0.735, 0.9)
    ]
    out = experiments.run_hardware_pes(points=points, shots=1024, seed=7)
    assert out["success"] is True
    p = out["points"][0]
    assert set(p["counts_by_term"]) == {"XX", "ZZ"}
    assert p["counts_scale"] == 1
    assert p["ci95_mha"] is not None


def test_r_d_openqasm2_multi_register_offsets():
    qasm = (
        'OPENQASM 2.0;\ninclude "qelib1.inc";\n'
        'qreg data[2];\nqreg anc[2];\n'
        'h data[0];\ncx data[0], anc[1];\n'
    )
    n_q, gates, total, two_q = manuscript_figures._parse_qasm_for_schematic(qasm)
    assert n_q == 4
    cx_gate = next(g for g in gates if g[0] == "CX")
    assert cx_gate[1] == [0, 3]
    assert total == 2 and two_q == 1


def test_h1_vqe_hardware_retains_per_term_counts(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"00": 512, "11": 512} for _ in qasms],
            "backend": "fake-qpu",
            "hardware_job_id": "fake-job",
            "calibration": {"calibration_fresh": True},
            "transpile": {"circuits": len(qasms), "max_transpiled_depth": 7},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    kw = dict(theta=[0.1, 0.2], pauli_coefficients={"XX": 0.5, "ZZ": 0.5}, layers=1,
              shots=1024, seed=7, bond_distance=0.735, fci_exact_energy=-0.5,
              ansatz_qasm=qasm, constant_energy=0.0)
    out = experiments.run_vqe_hardware(**kw)
    assert out["success"] is True
    assert set(out["counts_by_term"]) == {"XX", "ZZ"}
    assert out["counts_scale"] == 1
    assert out["transpile"]["circuits"] == 2

    zne = experiments.run_vqe_hardware(**kw, mitigation="ZNE", mitigation_scales=[1, 3, 5])
    assert zne["success"] is True
    assert set(zne["counts_by_term"]) == {"XX", "ZZ"}
    assert zne["counts_scale"] == 1
    assert zne["mitigation"]["uncertainty_method"] == "wls_linear_propagated_plus_fit_rmse"
    assert len(zne["mitigation"]["intercept_weights"]) == 3
    assert zne["counts_by_scale"] and set(zne["counts_by_scale"]) == {"1.0", "3.0", "5.0"}
    assert zne["fold_axis"] and zne["axis_valid"] is True
    assert zne["per_term_energy"] is not None


def test_c_e_snapshot_from_target_partial_calibration_fallback():
    import pytest
    from kubona.providers.qiskit_provider import _snapshot_from_target

    class PartialTarget:
        qubit_properties = [
            SimpleNamespace(t1=None, t2=None),
            SimpleNamespace(t1=None, t2=None),
        ]

        def __contains__(self, key):
            return key in ("measure", "sx")

        def __getitem__(self, key):
            if key == "measure":
                return {(0,): SimpleNamespace(error=0.011), (1,): SimpleNamespace(error=0.013)}
            if key == "sx":
                return {(0,): SimpleNamespace(error=0.00025), (1,): SimpleNamespace(error=0.00035)}
            return {}

    snap = _snapshot_from_target(PartialTarget(), "ibm_marrakesh")
    assert snap is not None
    assert "t1_us" not in snap and "t2_us" not in snap
    assert snap["readout_error"] == pytest.approx(0.012)
    assert snap["single_qubit_error"] == pytest.approx(0.0003)
    assert snap["per_qubit"]["0"]["readout_error"] == 0.011
    assert snap["per_qubit"]["1"]["sx_error"] == 0.00035

    class EmptyTarget:
        qubit_properties = [SimpleNamespace(t1=None, t2=None)]

        def __contains__(self, key):
            return False

    assert _snapshot_from_target(EmptyTarget(), "ibm_empty") is None


def test_g1_g2_g3_zne_barrier_folding_wls_and_readout_mitigation(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    from qiskit import QuantumCircuit, transpile
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    # G1: _fold_unitary inserts barriers so transpiler at opt_level=3 cannot cancel U*U^dag
    qc = QuantumCircuit(2)
    qc.ry(0.3, 0)
    qc.cx(0, 1)
    qc.rz(0.2, 1)
    qc.cx(0, 1)
    t1 = transpile(experiments._fold_unitary(qc, 1), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3)
    t3 = transpile(experiments._fold_unitary(qc, 3), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3)
    t5 = transpile(experiments._fold_unitary(qc, 5), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3)
    c1 = t1.count_ops().get("cx", 0)
    c3 = t3.count_ops().get("cx", 0)
    c5 = t5.count_ops().get("cx", 0)
    assert c1 == 2 and c3 == 6 and c5 == 10, (c1, c3, c5)

    # G1 fail-closed guard: collapsed transpile 2Q envelope refuses ZNE
    def fake_collapsed_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"00": 512, "11": 512} for _ in qasms],
            "backend": "fake-qpu",
            "hardware_job_id": "fake-job",
            "calibration": {"calibration_fresh": True, "readout_error": 0.02},
            "transpile": {"circuits": len(qasms), "min_transpiled_two_qubit_gates": 2, "max_transpiled_two_qubit_gates": 2},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_collapsed_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    refused = experiments.run_vqe_hardware(
        theta=[0.1, 0.2], pauli_coefficients={"XX": 0.5, "ZZ": 0.5}, layers=1,
        shots=1024, seed=7, bond_distance=0.735, fci_exact_energy=-0.5,
        ansatz_qasm=qasm, constant_energy=0.0, mitigation="ZNE", mitigation_scales=[1, 3, 5],
    )
    assert refused["success"] is False
    assert "collapsed" in refused["error"]

    # G2 + G3: WLS linear extrapolator + diagonal TREX readout mitigation
    cal = {"per_qubit": {"0": {"readout_error": 0.02}, "1": {"readout_error": 0.02}}}
    m_raw = experiments._hardware_counts_to_energy([{"00": 960, "01": 40}], {"IZ": 1.0}, readout_mitigation=False)
    m_mit = experiments._hardware_counts_to_energy([{"00": 960, "01": 40}], {"IZ": 1.0}, calibration=cal, readout_mitigation=True)
    assert m_mit["readout_mitigation"]["applied"] is True
    assert abs(m_mit["energy"] - 0.9583333) < 1e-3 and m_mit["energy"] > m_raw["energy"]


def test_g4_tfim_4site_critical_hva_default_converges():
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    from lattice_experiment import run_spin_chain  # noqa: E402

    out = run_spin_chain(model="tfim", n_sites=4, coupling=1.0, field=1.0, periodic=True, layers=2, shots=2048, seed=7)
    assert out["success"] is True
    assert abs(out["energy"] - out["energy_exact"]) / abs(out["energy_exact"]) < 0.03


def test_g5_canonical_citations_auto_seeded_for_campaign():
    sys.path.insert(0, BRIDGE_DIR)
    import manuscript_ops  # noqa: E402

    campaign = {
        "experiments": [
            {"job_id": "j1", "kind": "vqe", "energy": -1.137, "measured": True},
            {"job_id": "j2", "kind": "qaoa", "approximation_ratio": 0.85, "measured": True},
            {"job_id": "j3", "kind": "spin_chain", "energy": -5.22, "measured": True},
        ]
    }
    resp = manuscript_ops.run({"title": "Cross-Domain Test", "campaign": campaign, "evidence": {}})
    keys = {c["key"] for c in resp["citations"]}
    assert {"OMalley2016", "Kandala2017", "Kandala2019", "Zhou2020QAOA", "Harrigan2021", "Pfeuty1970"}.issubset(keys)


def test_p6_1_4_local_fractional_folding_and_exponential_zne():
    import pytest
    pytest.importorskip("qiskit")
    from qiskit import QuantumCircuit, transpile
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    # P6-4: local fractional 2Q folding survives opt_level=3 on a 4-CX circuit
    qc = QuantumCircuit(4)
    for i in range(4):
        qc.ry(0.2 + 0.1 * i, i)
    qc.cx(0, 1)
    qc.cx(1, 2)
    qc.cx(2, 3)
    qc.cx(3, 0)
    c1 = transpile(experiments._fold_circuit(qc, 1.0), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3).count_ops().get("cx", 0)
    c15 = transpile(experiments._fold_circuit(qc, 1.5), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3).count_ops().get("cx", 0)
    c20 = transpile(experiments._fold_circuit(qc, 2.0), basis_gates=["rz", "sx", "x", "cx"], optimization_level=3).count_ops().get("cx", 0)
    # All-gate folding is the canonical R2 axis; the exact transpiled CX
    # count depends on the basis decomposition, but it must increase strictly.
    assert 0 < c1 < c15 < c20, (c1, c15, c20)

    # R2: a 3-point exponential without an exact asymptote is underdetermined
    # (dof=0); it must fall back to WLS linear rather than explode pcov.
    fallback = experiments._extrapolate_zne(
        [1.0, 2.0, 3.0],
        [-1.118758, -1.108218, -1.102873],
        [0.0018, 0.0020, 0.0020],
        extrapolator="exponential",
    )
    assert fallback["extrapolator_used"] == "linear"

    # With the exact depolarizing asymptote c_II, the 2-parameter fit is
    # determined (dof=1) and gives a tight, finite interval.
    fixed = experiments._extrapolate_zne(
        [1.0, 2.0, 3.0],
        [-1.118758, -1.108218, -1.102873],
        [0.0018, 0.0020, 0.0020],
        extrapolator="exponential",
        asymptote=-0.4228698512159958,
    )
    assert fixed["extrapolator_used"] == "exponential_fixed_asymptote"
    assert fixed["chi2_reduced"] >= 0.0
    assert fixed["uncertainty"] < 0.008


def test_p6_2_spin_chain_hardware_zne_and_counts_by_term(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402
    from lattice_experiment import run_spin_chain_hardware  # noqa: E402

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"0000": shots} for _ in qasms],
            "backend": "ibm_fez",
            "hardware_job_id": "fake-tfim",
            "calibration": {"calibration_fresh": True, "per_qubit": {"0": {"readout_error": 0.01}, "1": {"readout_error": 0.01}, "2": {"readout_error": 0.01}, "3": {"readout_error": 0.01}}},
            "layout": {"physical_qubits": [0, 1, 2, 3]},
            "transpile": {"circuits": len(qasms), "min_transpiled_two_qubit_gates": 8, "max_transpiled_two_qubit_gates": 16},
            "error_suppression": {"dynamical_decoupling": True, "gate_twirling": True, "measure_twirling": True},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[4];\nh q[0];\ncx q[0],q[1];\n'
    out = run_spin_chain_hardware(
        qasm=qasm,
        pauli_coefficients={"ZZII": -1.0, "XIIII"[:4]: -1.0},
        n_sites=4,
        energy_exact=-5.226252,
        mitigation="ZNE",
        mitigation_scales=[1.0, 2.0],
        extrapolator="linear",
    )
    assert out["success"] is True
    assert out["mitigated"] is True
    assert out["mitigation"]["method"] == "ZNE"
    assert "counts_by_term" in out and "Z_mag" in out["counts_by_term"]
    assert out["counts_scale"] == 1
    assert out["readout_mitigation"]["applied"] is True
    assert out["error_suppression"]["dynamical_decoupling"] is True


def test_p6_3_hardware_pes_readout_and_error_suppression_parity(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"00": 512, "11": 512} for _ in qasms],
            "backend": "ibm_fez",
            "hardware_job_id": "fake-pes",
            "calibration": {"calibration_fresh": True, "per_qubit": {"22": {"readout_error": 0.01}, "23": {"readout_error": 0.01}}},
            "layout": {"physical_qubits": [22, 23]},
            "error_suppression": {"dynamical_decoupling": True, "gate_twirling": True, "measure_twirling": True},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    points = [
        {"bond_distance": b, "pauli_coefficients": {"XX": 0.5, "ZZ": 0.5},
         "fci_exact_energy": -0.5, "constant_energy": 0.0, "ansatz_qasm": qasm}
        for b in (0.75, 1.50)
    ]
    out = experiments.run_hardware_pes(points=points, shots=1024, seed=7)
    assert out["success"] is True
    assert out["readout_mitigation"]["applied"] is True
    assert out["error_suppression"]["dynamical_decoupling"] is True


def test_r1_global_layout_selector_avoids_high_readout_trap():
    from kubona.providers.qiskit_provider import QiskitProvider
    from kubona.schema import load_provider

    # Synthetic 10-qubit graph: edge (0,1) is the single best edge on the chip,
    # but sits inside a high-readout trap (q2=0.08, q3=0.06). Cluster {5,6,7,8}
    # has uniformly low readout (0.008..0.012) and good 2Q edges.
    ro = [0.004, 0.004, 0.080, 0.060, 0.070, 0.009, 0.010, 0.011, 0.012, 0.050]
    t2 = [150e-6] * 10
    coupling = [(0, 1), (1, 2), (2, 3), (3, 4), (5, 6), (6, 7), (7, 8), (8, 5), (8, 9)]

    class TrapProps:
        def readout_error(self, q):
            return ro[q]

        def t2(self, q):
            return t2[q]

        def gate_error(self, gate, qubits):
            return 0.002 if set(qubits) == {0, 1} else 0.004

    class TrapBackend:
        name = "ibm_trap_mock"
        num_qubits = 10
        coupling_map = coupling

        def properties(self):
            return TrapProps()

    prov = QiskitProvider(load_provider("ibm-qiskit"))
    chosen = prov._calibrated_initial_layout(SimpleNamespace(num_qubits=4), TrapBackend())
    assert chosen is not None and len(chosen) == 4
    assert set(chosen) == {5, 6, 7, 8}, f"R1 violated: trapped in {chosen}"
    lq = prov._layout_quality(chosen, TrapBackend())
    assert lq is not None and lq["suspicious"] is False and lq["max_readout_error"] <= 0.012


def test_r2_fixed_asymptote_exponential_zne_avoids_dof0_blowup():
    import pytest
    pytest.importorskip("scipy")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    # 3 scales [1, 2, 3] with exact depolarizing asymptote c_II = -0.42287 -> dof = 1
    ext = experiments._extrapolate_zne(
        [1.0, 2.0, 3.0],
        [-1.1227, -1.1060, -1.0910],
        [0.0020, 0.0020, 0.0020],
        extrapolator="exponential",
        asymptote=-0.42287,
    )
    assert ext["extrapolator_used"] == "exponential_fixed_asymptote"
    assert ext["exp_params"]["asymptote_fixed"] is True
    assert ext["uncertainty"] < 0.006  # ~±11 mHa 95% CI, not ±38 mHa blowup!

    # Without asymptote on only 3 scales, falls back cleanly to WLS linear (never dof=0 blowup)
    ext_fb = experiments._extrapolate_zne(
        [1.0, 2.0, 3.0],
        [-1.1227, -1.1060, -1.0910],
        [0.0020, 0.0020, 0.0020],
        extrapolator="exponential",
    )
    assert ext_fb["extrapolator_used"] == "linear"


def test_r3_qaoa_hardware_emits_error_suppression_and_layout_quality(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    def fake_batch(qasms, shots, seed, backend=None):
        return {
            "counts": [{"01": shots // 2, "10": shots // 2}],
            "backend": "ibm_fez",
            "hardware_job_id": "fake-qaoa",
            "layout": {"physical_qubits": [5, 6]},
            "layout_quality": {"physical_qubits": [5, 6], "max_readout_error": 0.01, "suspicious": False},
            "error_suppression": {"dynamical_decoupling": True, "gate_twirling": True, "measure_twirling": True},
        }

    monkeypatch.setattr(experiments, "_hardware_batch", fake_batch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    out = experiments.run_qaoa_hardware(qasm=qasm, optimal_cut=1, nodes=2, edges=[[0, 1]], shots=1024, seed=7)
    assert out["success"] is True
    assert out["error_suppression"]["dynamical_decoupling"] is True
    assert out["layout_quality"]["suspicious"] is False
    assert out["readout_mitigation"]["method"] == "none"
    assert out["readout_mitigation"]["applied"] is False


def test_c_a_qaoa_ratio_interval_matches_sampled_cut_convention():
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    counts = {"000": 10, "011": 20, "110": 30, "101": 40}
    edges = [(0, 1), (1, 2)]
    mean_cut = experiments._counts_cut(counts, edges)
    ci, se = experiments._ratio_ci_from_counts(counts, edges, 2, mean_cut)
    assert ci is not None and se is not None and se > 0
    ratio = mean_cut / 2.0
    assert ci[0] <= ratio <= ci[1]
    # Symmetric distribution around the mean, clamped to [0,1].
    assert abs(((ci[0] + ci[1]) / 2.0) - ratio) < 1e-6


def test_g2_trex_uses_physical_layout_qubits_not_logical_indices():
    import pytest
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402

    assert experiments._layout_physical_qubits({"initial": {"0": 22, "1": 23}}) == [22, 23]
    assert experiments._layout_physical_qubits({"physical_qubits": [22, 23]}) == [22, 23]
    cal = {"per_qubit": {
        "0": {"readout_error": 0.01},
        "22": {"readout_error": 0.10},
    }}
    # Logical qubit 0 ran on physical 22; TREX must use 22's error (0.10).
    assert experiments._readout_mitigation_factor("Z", cal, [22, 23]) == pytest.approx(0.80)
    assert experiments._readout_mitigation_factor("Z", cal, None) == pytest.approx(0.98)


def test_g1_barrier_protected_folding_survives_opt_level_3():
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402
    from qiskit import QuantumCircuit, transpile

    def twoq(circ):
        return sum(1 for inst in circ.data if len(inst.qubits) == 2 and inst.operation.name != "barrier")

    base = QuantumCircuit(2)
    base.h(0)
    base.cx(0, 1)
    base.cx(1, 0)
    counts = {}
    for scale in (1, 3, 5):
        folded = experiments._fold_unitary(base, scale)
        transpiled = transpile(folded, basis_gates=["rz", "sx", "x", "cx"], optimization_level=3)
        counts[scale] = twoq(transpiled)
    assert counts[1] < counts[3] < counts[5], counts
    assert counts[3] == 3 * counts[1]
    assert counts[5] == 5 * counts[1]


def test_g4_tfim_ansatz_qasm_is_qasm2_loadable_for_hardware():
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    from lattice_experiment import run_spin_chain
    from qiskit import qasm2

    res = run_spin_chain(n_sites=4, model="tfim", layers=2, shots=256,
                         max_iterations=20, target_error=1e-2)
    assert res["success"] is True
    assert "rzz" not in res["ansatz_qasm"] and "sx" not in res["ansatz_qasm"]
    qasm2.loads(res["ansatz_qasm"])


def test_r2_all_gate_zne_axis_is_monotonic():
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402
    from qiskit import QuantumCircuit

    # H2-shaped circuit: 8 single-qubit gates + 2 CX = 10 gates. The R2 audit
    # found the old dispatcher mixed global and local folding; the canonical
    # axis must be strictly increasing in TOTAL gates for every scale.
    qc = QuantumCircuit(2)
    for _ in range(4):
        qc.h(0)
        qc.x(1)
    qc.cx(0, 1)
    qc.cx(1, 0)

    def total(circuit):
        return sum(
            1 for inst in circuit.data
            if str(inst.operation.name) not in ("barrier", "measure", "reset")
        )

    counts = {scale: total(experiments._fold_circuit(qc, scale)) for scale in (1.0, 1.5, 2.0, 3.0)}
    assert counts == {1.0: 10, 1.5: 14, 2.0: 20, 3.0: 30}, counts
    assert counts[1.0] < counts[1.5] < counts[2.0] < counts[3.0]


def test_p6_local_fractional_folding_and_actual_scale_guard(monkeypatch):
    import pytest
    pytest.importorskip("qiskit")
    sys.path.insert(0, BRIDGE_DIR)
    import experiments  # noqa: E402
    from qiskit import QuantumCircuit, transpile

    def cx_count(circ):
        return sum(1 for inst in circ.data if len(inst.qubits) >= 2 and inst.operation.name != "barrier")

    base = QuantumCircuit(4)
    base.h(0)
    for a, b in ((0, 1), (1, 2), (2, 3), (3, 0)):
        base.cx(a, b)
    counts = {}
    for scale in (1.0, 1.5, 2.0):
        folded = experiments._fold_circuit(base, scale)
        counts[scale] = cx_count(transpile(folded, basis_gates=["rz", "sx", "x", "cx"], optimization_level=3))
    assert counts == {1.0: 4, 1.5: 6, 2.0: 8}
    total = {scale: sum(1 for inst in experiments._fold_circuit(base, scale).data
                        if str(inst.operation.name) not in ("barrier", "measure", "reset"))
             for scale in (1.0, 1.5, 2.0)}
    assert total[1.0] < total[1.5] < total[2.0]

    # A 2-gate H2 circuit cannot represent two near-1x scales distinctly;
    # the runner must refuse instead of fitting duplicate total-gate ratios.
    def _must_not_dispatch(*args, **kwargs):
        raise AssertionError("hardware batch must not be dispatched on collapsed scales")

    monkeypatch.setattr(experiments, "_hardware_batch", _must_not_dispatch)
    qasm = 'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\nh q[0];\ncx q[0],q[1];\n'
    out = experiments.run_vqe_hardware(
        theta=[0.1, 0.2], pauli_coefficients={"XX": 0.5, "ZZ": 0.5}, layers=1,
        shots=1024, seed=7, bond_distance=0.735, fci_exact_energy=-0.5,
        ansatz_qasm=qasm, constant_energy=0.0, mitigation="ZNE",
        mitigation_scales=[1.01, 1.02],
    )
    assert out["success"] is False
    assert "not strictly increasing" in out["error"]
