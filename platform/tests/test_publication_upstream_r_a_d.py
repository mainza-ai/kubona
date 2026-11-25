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
