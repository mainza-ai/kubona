"""Tests for Scientific Manuscript and Automated Peer Review Engine."""

import pytest
from pathlib import Path
from kubona.science.manuscript import (
    ManuscriptGenerator, ManuscriptSignoff, ReviewEvidence, ScientificManuscript,
)


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

    evidence = ReviewEvidence(
        comparator_verdict="live_kernel_proven",
        sorry_ax_count=0,
        anchor_discrepancy_mha=abs(-1.49999 - -1.5) * 1000.0,
        provenance_receipts=["0x1234567890ABCDEF", "ledger:anchor:1"],
    )
    review = ManuscriptGenerator.automated_peer_review(manuscript, evidence)
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
    assert any("verification evidence" in c for c in review.actionable_critiques)


def test_bilingual_manuscript_and_vector_graphics_export(tmp_path):
    bilingual_ms = ManuscriptGenerator.create_bilingual_discovery_manuscript(
        title="Quantum Krylov Subspace Ground State Discovery",
        bilingual_title="Kukubona Kwamagesi Akwa-Krylov Amu-Quantum",
        target_system="Haldane Spin-1 Chain",
        authors=["Kubona Agent Collective", "Chief Scientist"],
        abstract="We observe non-variational convergence matching the Haldane spectral gap.",
        bilingual_abstract="Twabona kubusena kwakukonka mu-Haldane gap ukwakulondoloka.",
        analytical_anchor_name="Haldane_Spin_1_Chain",
        analytical_anchor_val=0.410479,
        measured_val=0.410479,
        second_language="Tonga",
        lean4_proof_hash="0xLEAN4PROOFCERT777",
        provenance_record_hash="0xMERKLEDAGHASH888",
    )

    assert bilingual_ms.second_language == "Tonga"
    assert "crystal_lattice.svg" in bilingual_ms.figures
    assert "bloch_sphere.svg" in bilingual_ms.figures
    assert "energy_convergence.svg" in bilingual_ms.figures

    latex = bilingual_ms.compile_latex()
    assert "Kukubona Kwamagesi" in latex
    assert "Tonga Summary:" in latex

    export_dir = tmp_path / "bilingual_preprint"
    bilingual_ms.export_project(export_dir)

    assert (export_dir / "figures" / "crystal_lattice.svg").exists()
    assert (export_dir / "figures" / "bloch_sphere.svg").exists()
    assert (export_dir / "figures" / "energy_convergence.svg").exists()

    # 5-Dimensional Peer Review Tribunal over live-kernel evidence
    bilingual_evidence = ReviewEvidence(
        comparator_verdict="live_kernel_proven",
        sorry_ax_count=0,
        anchor_discrepancy_mha=0.0,
        provenance_receipts=["0xLEAN4PROOFCERT777", "0xMERKLEDAGHASH888"],
    )
    review = ManuscriptGenerator.automated_peer_review(bilingual_ms, bilingual_evidence)
    assert review.overall_recommendation == "ACCEPT"
    assert review.mathematical_rigor_score >= 9.0
    assert review.novelty_score >= 8.0
    assert any("provisional" in c for c in review.actionable_critiques)
    assert review.empirical_grounding_score >= 9.0
    assert review.axiomatic_hygiene_score == 10.0
    assert review.reproducibility_score == 10.0
    assert review.mean_score() >= 9.0
    assert review.is_acceptable()



def _live_evidence(disc_mha=0.01):
    return ReviewEvidence(
        comparator_verdict="live_kernel_proven",
        sorry_ax_count=0,
        anchor_discrepancy_mha=disc_mha,
        provenance_receipts=["0xPROOF", "ledger:anchor:1"],
    )


def test_peer_review_rejects_sorry():
    from kubona.science.manuscript import ManuscriptGenerator
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title="T", target_system="H2", authors=["A"], abstract="B",
        analytical_anchor_name="H2", analytical_anchor_val=-1.137,
        measured_val=-1.137, lean4_proof_hash="0xHASH")
    ev = ReviewEvidence(comparator_verdict="live_kernel_proven", sorry_ax_count=2,
                        anchor_discrepancy_mha=0.0, provenance_receipts=["0xHASH"])
    review = ManuscriptGenerator.automated_peer_review(ms, ev)
    assert review.overall_recommendation == "REJECT"
    assert not review.is_acceptable()


def test_peer_review_static_capped_below_accept():
    from kubona.science.manuscript import ManuscriptGenerator
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title="T", target_system="H2", authors=["A"], abstract="B",
        analytical_anchor_name="H2", analytical_anchor_val=-1.137,
        measured_val=-1.137, lean4_proof_hash="0xHASH")
    ev = ReviewEvidence(comparator_verdict="static_ast_containment", sorry_ax_count=0,
                        anchor_discrepancy_mha=0.0, provenance_receipts=["0xHASH"])
    review = ManuscriptGenerator.automated_peer_review(ms, ev)
    assert review.axiomatic_hygiene_score == 9.0
    assert "not kernel-proved" in review.actionable_critiques[0]
    assert review.overall_recommendation == "REVISE"
    assert not review.is_acceptable()


def test_signoff_verifies_and_stages_deposit(tmp_path):
    import hashlib
    from nacl.signing import SigningKey
    from kubona.science.manuscript import ManuscriptGenerator, ManuscriptSignoff
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title="T", target_system="H2", authors=["A"], abstract="B",
        analytical_anchor_name="H2", analytical_anchor_val=-1.137,
        measured_val=-1.137, lean4_proof_hash="0xHASH")
    content_hash = hashlib.sha256(ms.compile_latex().encode()).hexdigest()
    sk = SigningKey.generate()
    expiry = "2030-01-01T00:00:00Z"
    payload = f"manuscript-signoff:{ms.manuscript_id}:{content_hash}:{expiry}".encode()
    sig = sk.sign(payload).signature
    signoff = ManuscriptSignoff(
        manuscript_id=ms.manuscript_id, content_hash=content_hash,
        signer_pubkey_hex=sk.verify_key.encode().hex(),
        signature_hex=sig.hex(), signed_at=expiry)
    ManuscriptGenerator.attach_signoff(ms, signoff, expiry)
    assert ms.signoff is not None

    review = ManuscriptGenerator.automated_peer_review(ms, _live_evidence())
    assert review.is_acceptable()
    out = ManuscriptGenerator.stage_zenodo_deposit(tmp_path / "deposit", ms, review,
                                                   creators=[{"name": "A"}])
    assert out.exists()
    import json
    manifest = json.loads(out.read_text())
    assert manifest["state"] == "staged-needs-human-upload"
    assert manifest["auto_publish"] is False


def test_deposit_refused_without_accept_or_signoff(tmp_path):
    from kubona.science.manuscript import ManuscriptGenerator
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title="T", target_system="H2", authors=["A"], abstract="B",
        analytical_anchor_name="H2", analytical_anchor_val=-1.137,
        measured_val=-1.5, lean4_proof_hash=None)
    review = ManuscriptGenerator.automated_peer_review(ms)  # unattested -> REVISE
    assert review.overall_recommendation == "REVISE"
    try:
        ManuscriptGenerator.stage_zenodo_deposit(tmp_path / "d", ms, review, creators=[])
        raise AssertionError("deposit without ACCEPT/sign-off must refuse")
    except ValueError:
        pass
