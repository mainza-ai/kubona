"""Tests for Scientific Manuscript and Automated Peer Review Engine."""

import pytest
from pathlib import Path
from kubona.science.manuscript import ManuscriptGenerator, ScientificManuscript


def test_create_and_export_discovery_manuscript(tmp_path):
    manuscript = ManuscriptGenerator.create_discovery_manuscript(
        title="Exact Haldane Topological Gap Resolution via Quantum Subspace Diagonalization",
        target_system="Spin-1 Heisenberg Chain",
        authors=["Kubona Theorist Agent", "Dr. Jane Doe"],
        abstract="We demonstrate exact agreement with the Haldane gap analytical anchor using early fault-tolerant quantum Krylov diagonalization.",
        analytical_anchor_name="Haldane_Spin_1_Chain",
        analytical_anchor_val=0.410479,
        measured_val=0.410480,
        lean4_proof_hash="0x9ABCDEF123456789",
        negative_certificates=["Barren_Plateau_Ansatz_DeepHardwareEfficient_N16"]
    )

    assert manuscript.title.startswith("Exact Haldane")
    assert manuscript.lean4_proof_hash == "0x9ABCDEF123456789"
    assert len(manuscript.sections) == 3

    latex = manuscript.compile_latex()
    assert r"\documentclass" in latex
    assert r"\maketitle" in latex
    assert "Haldane_Spin_1_Chain" in latex
    assert "Barren_Plateau_Ansatz_DeepHardwareEfficient_N16" in latex

    bibtex = manuscript.compile_bibtex()
    assert "@misc{openai_math_corpus_2026" in bibtex

    # Export project
    export_dir = tmp_path / "manuscript_export"
    manuscript.export_project(export_dir)

    assert (export_dir / "main.tex").exists()
    assert (export_dir / "references.bib").exists()
    assert (export_dir / "metadata.json").exists()
    assert (export_dir / "sections" / "introduction.tex").exists()


def test_automated_peer_review_accept():
    manuscript = ManuscriptGenerator.create_discovery_manuscript(
        title="Spontaneous Magnetization Verification",
        target_system="3D Heisenberg Ferromagnet",
        authors=["Kubona Prover"],
        abstract="Verified spontaneous magnetization matching Family 271 exact analytical bounds.",
        analytical_anchor_name="Heisenberg_Ferromagnet_3D",
        analytical_anchor_val=-1.5,
        measured_val=-1.49999,
        lean4_proof_hash="0x1234567890ABCDEF"
    )

    review = ManuscriptGenerator.automated_peer_review(manuscript)
    assert review.overall_recommendation == "ACCEPT"
    assert review.mathematical_rigor_score >= 9.0
    assert review.axiomatic_hygiene_score == 10.0
    assert review.is_acceptable()


def test_automated_peer_review_revise_when_lacking_formal_proof():
    manuscript = ManuscriptGenerator.create_discovery_manuscript(
        title="Heuristic Ground State Search",
        target_system="Toy System",
        authors=["Unverified Agent"],
        abstract="Unverified heuristic search.",
        analytical_anchor_name="ToyAnchor",
        analytical_anchor_val=0.0,
        measured_val=0.1,
        lean4_proof_hash=None  # No formal proof!
    )

    review = ManuscriptGenerator.automated_peer_review(manuscript)
    assert review.overall_recommendation == "REVISE"
    assert not review.is_acceptable()
    assert any("Lean 4" in c for c in review.actionable_critiques)
