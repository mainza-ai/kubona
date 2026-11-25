"""R1 + R4 + R5 tests: deterministic 300-DPI circuit schematic figures, layout/calibration, classical settings, uv.lock hash."""

from __future__ import annotations

import sys
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import manuscript_datasheet  # noqa: E402
import manuscript_figures  # noqa: E402
import manuscript_ops  # noqa: E402


def test_r1_circuit_schematic_renders_per_family_and_keeps_qasm_out_of_campaign():
    qasm_vqe = (
        'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[4];\n'
        "ry(1.2345) q[0];\nry(0.5678) q[1];\ncx q[0],q[1];\ncx q[1],q[2];\nrz(2.3456) q[2];\nmeasure q[0] -> c[0];\n"
    )
    qasm_qaoa = (
        'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[4];\n'
        "h q[0];\nh q[1];\ncx q[0],q[1];\nrz(0.9999) q[1];\ncx q[0],q[1];\nrx(0.4444) q[0];\n"
    )
    rows = [
        {
            "job_id": "job_vqe_1",
            "kind": "vqe_hardware",
            "backend": "ibm_fez",
            "hardware": True,
            "measured": True,
            "shots": 1024,
            "seed": 7,
            "energy": -1.135,
            "fci_exact_energy": -1.137,
            "circuit_sha256": "a" * 64,
            "used_qubits": [0, 1, 2, 3],
            "molecule": "H2",
            "basis": "sto-3g",
            "active_space": "(2, 2)",
            "layers": 2,
            "calibration": {"snapshot_id": "cal_1", "median_two_qubit_error": 0.002, "median_readout_error": 0.01},
        },
        {
            "job_id": "job_qaoa_1",
            "kind": "qaoa_hardware",
            "backend": "ibm_marrakesh",
            "hardware": True,
            "measured": True,
            "shots": 2048,
            "seed": 11,
            "measured_cut": 8.5,
            "optimal_cut": 10.0,
            "approximation_ratio": 0.85,
            "circuit_sha256": "b" * 64,
        },
    ]

    figs = manuscript_figures.generate_campaign_figures(
        rows,
        circuit_samples={"job_vqe_1": qasm_vqe, "job_qaoa_1": qasm_qaoa},
    )
    fig_map = {name: (raw, cap) for name, raw, cap in figs}
    assert "fig_circuit_vqe_hardware.png" in fig_map
    assert "fig_circuit_qaoa_hardware.png" in fig_map
    for name in ("fig_circuit_vqe_hardware.png", "fig_circuit_qaoa_hardware.png"):
        raw, cap = fig_map[name]
        assert raw.startswith(b"\x89PNG\r\n\x1a\n")
        assert "circuit SHA-256 prefix" in cap

    # Compose passes circuit_samples transiently and produces both figures + R4/R5 tables
    out = manuscript_ops.compose(
        {
            "manuscript_id": "ms_r1_test",
            "title": "R1 Circuit Diagram Verification",
            "authors": ["Test Author"],
            "campaign": {"target_system": "H2", "experiments": rows},
            "evidence": {},
            "circuit_samples": {"job_vqe_1": qasm_vqe, "job_qaoa_1": qasm_qaoa},
            "build": False,
        }
    )
    assert out["ok"] is True
    assert "fig_circuit_vqe_hardware.png" in out["figure_sha256"]
    assert "fig_circuit_qaoa_hardware.png" in out["figure_sha256"]
    # Raw QASM rotation literals must NOT appear in main_tex
    assert "1.2345" not in out["main_tex"]
    assert "0.9999" not in out["main_tex"]

    # R4 layout table & R5 classical settings table present in Appendix B
    ds_tex = manuscript_datasheet.build_experiment_datasheet_section(rows)
    assert "Hardware Layout and Calibration Provenance" in ds_tex
    assert "0,1,2,3" in ds_tex
    assert "Classical Reference and Solver Settings" in ds_tex
    assert "sto-3g" in ds_tex
    assert "(2, 2)" in ds_tex

    # R5 uv_lock_sha256 recorded in tooling versions
    tooling = manuscript_ops._tooling_versions()
    assert len(tooling.get("uv_lock_sha256", "")) == 64
