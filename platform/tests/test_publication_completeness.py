"""Tests for Publication Completeness Phases 0–6 (Datasheet, ZNE/CI, Figures, Scholarly Frame)."""

from __future__ import annotations

import sys
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import manuscript_datasheet  # noqa: E402
import manuscript_figures  # noqa: E402
import manuscript_ops  # noqa: E402


def test_publication_completeness_datasheet_declarations_refs_and_figures():
    rows = [
        {
            "job_id": "11112222-3333-4444-5555-666677778888",
            "kind": "vqe_hardware",
            "backend": "ibm_fez",
            "hardware": True,
            "mitigated": True,
            "measured": False,
            "seed": 7,
            "shots": 1024,
            "energy": -1.131,
            "fci_exact_energy": -1.137306,
            "delta_mha": 6.306,
            "ci95_mha": 1.25,
            "usage_seconds": 2.0,
            "actual_usd": 3.20,
            "two_qubit_gate_count": 12,
            "qasm_gate_lines": 48,
            "circuit_sha256": "a" * 64,
            "counts_sha256": "b" * 64,
            "calibration": {
                "snapshot_id": "cal_8f65e57d6f49",
                "median_two_qubit_error": 0.00182,
                "median_readout_error": 0.0115,
            },
        }
    ]

    # Phase 1: Experiment Datasheet
    ds_tex = manuscript_datasheet.build_experiment_datasheet_section(rows)
    assert "supplement/datasheet.json" in ds_tex
    assert "11112222" in ds_tex
    assert "aaaaaaaaaaaa" in ds_tex
    assert "bbbbbbbbbbbb" in ds_tex

    # Phase 3: Data & Code Availability
    avail_tex = manuscript_datasheet.build_data_availability_section(rows, "ms_test_01")
    assert "github.com/mainza-ai/kubona" in avail_tex
    assert "CC-BY-4.0" in avail_tex
    assert "supplement/qasm/" in avail_tex

    # Phase 4: ZNE & Uncertainty
    zne_tex = manuscript_datasheet.build_zne_and_uncertainty_section(rows)
    assert "ZNE mitigated" in zne_tex
    assert "1.250 mHa" in zne_tex

    # Phase 5: Scholarly declarations + >= 12 canonical BibTeX references
    decl_tex = manuscript_datasheet.build_scholarly_declarations_section({}, ["Mainza AI"])
    assert "Competing Interests" in decl_tex
    assert "Funding" in decl_tex
    assert "CRediT" in decl_tex
    assert len(manuscript_datasheet.CANONICAL_METHOD_REFERENCES) >= 12

    # Phase 2: Transpile overhead & calibration overview figures render crisp PNGs
    figs = manuscript_figures.generate_campaign_figures(rows)
    fig_names = {name for name, _, _ in figs}
    assert "fig_transpile_overhead.png" in fig_names
    assert "fig_calibration_overview.png" in fig_names
    for _, raw, _ in figs:
        assert raw.startswith(b"\x89PNG\r\n\x1a\n")

    # End-to-end compose includes new sections, no template residue, and >= 12 BibTeX keys
    out = manuscript_ops.compose(
        {
            "manuscript_id": "ms_test_01",
            "title": "Hardware Replication Audit",
            "authors": ["Mainza AI"],
            "campaign": {"target_system": "H2", "experiments": rows},
            "evidence": {"resolved_receipts": ["job:11112222-3333-4444-5555-666677778888"]},
            "build": False,
        }
    )
    assert out["ok"] is True
    slugs = {s["slug"] for s in out["sections"]}
    assert "appendix_datasheet" in slugs
    assert "data_availability" in slugs
    assert "declarations" in slugs
    assert "uncertainty_zne" in slugs
    assert "caller's artifact" not in out["main_tex"]
    assert "Sun2018PySCF" in out["bibtex"]
    assert "Bravyi2024" in out["bibtex"]
