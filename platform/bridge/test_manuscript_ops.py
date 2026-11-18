"""Manuscript authoring/build contract (R0/R3).

These tests pin the honesty rules that make a manuscript a result rather
than a template: no invented numbers, no decorative figures, unresolved
receipts earn no credit, and the PDF build is byte-reproducible.
"""

from __future__ import annotations

import base64
import shutil

import pytest

import manuscript_ops

PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)


def _campaign(measured: bool = True) -> dict:
    rows = [
        {
            "job_id": "job-vqe-1",
            "kind": "vqe",
            "energy": -1.1372 if measured else None,
            "energy_exact": -1.1370,
            "delta": 0.2 if measured else None,
            "backend": "aer_simulator",
            "measured": measured,
            "claimable": True,
        },
        {
            "job_id": "job-spin-1",
            "kind": "spin_chain",
            "energy": -3.009,
            "energy_exact": -3.00902,
            "delta": 0.02,
            "backend": "exact",
            "measured": True,
            "claimable": False,
        },
    ]
    return {
        "session_id": "sess-1",
        "research_goal": "Reproduce the H2 ground-state energy",
        "target_system": "H2",
        "experiments": rows if measured else [],
        "measurement": ({"measured": True, "energy": -1.1372, "fci_exact_energy": -1.1370,
                         "job_id": "job-vqe-1", "backend": "aer_simulator"}
                        if measured else {}),
        "notebook": ({"quantum_energy": -1.1372, "classical_energy": -1.1370, "delta_mha": 0.2}
                     if measured else None),
    }


def _req(**over) -> dict:
    req = {
        "manuscript_id": "ms_test",
        "title": "Measured H2 results",
        "authors": ["Operator"],
        "target_system": "H2",
        "campaign": _campaign(),
        "evidence": {"resolved_receipts": ["job-vqe-1"], "unresolved_receipts": []},
    }
    req.update(over)
    return req


def test_draft_uses_only_measured_rows_and_makes_no_verification_claim():
    resp = manuscript_ops.run(_req())
    assert resp["ok"], resp
    assert "2 measured result(s)" in resp["abstract"]
    assert "chemical accuracy" in resp["abstract"]
    results = next(s for s in resp["sections"] if s["slug"] == "results")
    assert "job-vqe-1" in results["latex"]
    assert "-1.137200" in results["latex"]
    assert "job-spin-1" in results["latex"]
    assert "job-not-measured" not in results["latex"]
    provenance = next(s for s in resp["sections"] if s["slug"] == "provenance")
    assert "resolved against ledger records" in provenance["latex"]
    assert "unverified" in provenance["latex"].lower()
    assert "verified under the Lean 4 comparator" not in provenance["latex"]
    assert resp["review"]["recommendation"] == "REVISE"
    assert resp["review"]["acceptable"] is False


def test_primary_measurement_is_folded_once():
    """A real VQE fold puts the point in measurement/notebook, not experiments.

    The first live build printed "no measured result" for a completed VQE run;
    this pins the promotion of the primary point into the results table.
    """
    campaign = {
        "target_system": "H2",
        "research_goal": "H2 ground state",
        "experiments": [],
        "measurement": {
            "measured": True, "energy": -1.1221, "fci_exact_energy": -1.1373,
            "job_id": "job-primary", "backend": "qiskit-aer-simulator",
            "claimable": False,
        },
        "notebook": {"quantum_energy": -1.1221, "classical_energy": -1.1373, "delta_mha": 15.19},
    }
    resp = manuscript_ops.run(_req(campaign=campaign))
    assert resp["ok"], resp
    assert "1 measured result(s)" in resp["abstract"]
    results = next(s for s in resp["sections"] if s["slug"] == "results")
    assert "job-primary" in results["latex"]
    assert "-1.122100" in results["latex"]


def test_unresolved_receipts_are_named_and_earn_no_credit():
    resp = manuscript_ops.run(_req(evidence={
        "resolved_receipts": [],
        "unresolved_receipts": ["job-forged"],
    }))
    assert resp["ok"], resp
    provenance = next(s for s in resp["sections"] if s["slug"] == "provenance")
    assert "did NOT resolve" in provenance["latex"]
    assert "job-forged" in provenance["latex"]
    assert resp["review"]["reproducibility"] <= 7.0


def test_no_measured_rows_produces_no_numbers_and_no_table():
    resp = manuscript_ops.run(_req(campaign=_campaign(measured=False)))
    assert resp["ok"], resp
    assert "no measured result" in resp["abstract"].lower()
    results = next(s for s in resp["sections"] if s["slug"] == "results")
    assert "intentionally empty" in results["latex"]
    assert "tabular" not in results["latex"]


def test_figures_must_be_includable_images_and_are_declared():
    resp = manuscript_ops.run(_req(figures=[{"name": "convergence.png",
                                             "content_b64": base64.b64encode(PNG).decode()}]))
    assert resp["ok"], resp
    results = next(s for s in resp["sections"] if s["slug"] == "results")
    assert r"\includegraphics[width=0.9\linewidth]{figures/convergence.png}" in results["latex"]
    assert "convergence.png" in resp["figure_sha256"]


@pytest.mark.parametrize("name", ["convergence.svg", "../escape.png", "a/b.png", "x.exe"])
def test_figures_refuse_unsafe_or_unincludable_names(name):
    resp = manuscript_ops.run(_req(figures=[{"name": name,
                                             "content_b64": base64.b64encode(PNG).decode()}]))
    assert resp["ok"] is False
    assert resp["error_code"] == "invalid_request"


def test_figure_payload_must_match_its_extension_magic():
    resp = manuscript_ops.run(_req(figures=[{"name": "plot.png",
                                             "content_b64": base64.b64encode(b"not an image").decode()}]))
    assert resp["ok"] is False
    assert resp["error_code"] == "invalid_request"


def test_citations_become_bibtex_and_a_cite():
    resp = manuscript_ops.run(_req(citations=[{
        "key": "10.1234/example", "title": "Prior work", "authors": "Doe, J.",
        "year": 2024, "doi": "10.1234/example",
    }]))
    assert resp["ok"], resp
    intro = next(s for s in resp["sections"] if s["slug"] == "introduction")
    assert r"\cite{10_1234_example}" in intro["latex"]
    assert "@misc{10_1234_example" in resp["bibtex"]
    assert "doi = {10.1234/example}" in resp["bibtex"]


requires_tex = pytest.mark.skipif(shutil.which("pdflatex") is None,
                                  reason="pdfTeX not installed in this environment")


@requires_tex
def test_build_is_byte_reproducible_and_ships_the_source():
    first = manuscript_ops.run(_req(build=True, source_date_epoch=1700000000))
    second = manuscript_ops.run(_req(build=True, source_date_epoch=1700000000))
    assert first["ok"], first
    assert second["ok"], second
    assert first["build"]["build_hash"] == second["build"]["build_hash"]
    names = {a["name"] for a in first["build"]["artifacts"]}
    assert {"main.tex", "main.pdf", "references.bib", "sections/results.tex"} <= names
    pdf = next(a for a in first["build"]["artifacts"] if a["name"] == "main.pdf")
    raw = base64.b64decode(pdf["content_b64"])
    assert raw.startswith(b"%PDF-")
    assert len(raw) > 1024


@requires_tex
def test_build_without_measured_rows_still_compiles_and_says_so():
    resp = manuscript_ops.run(_req(campaign=_campaign(measured=False), build=True,
                                   source_date_epoch=1700000000))
    assert resp["ok"], resp
    assert resp["build"]["build_hash"]
