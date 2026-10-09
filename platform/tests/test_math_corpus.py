"""Tests for OpenAI Math Repository integration, catalog, and Comparator verification."""

import pytest
from pathlib import Path
from kubona.science.math_catalog import MathCatalog, ResultFamily
from kubona.eval.comparator_eval import (
    ComparatorChallengeRegistry,
    ComparatorChallengeSpec,
    STANDARD_PERMITTED_AXIOMS,
)
from kubona.provenance import Ledger


def test_math_catalog_initialization():
    catalog = MathCatalog()
    catalog.initialize()
    stats = catalog.summary_stats()
    assert stats["total_families"] > 0
    assert stats["disciplines_count"] > 0
    assert stats["quantum_relevant_count"] >= 15
    assert stats["reasoning_traces_count"] >= 10


def test_math_catalog_family_retrieval_and_search():
    catalog = MathCatalog()
    # Test retrieving quantum-relevant family 271 (Bloch law / Heisenberg)
    fam271 = catalog.get_family(271)
    assert fam271 is not None
    assert fam271.family_id == 271
    assert "ferromagnetic" in fam271.title.lower() or "bloch" in fam271.title.lower()
    assert fam271.is_quantum_relevant
    assert fam271.has_reasoning_trace

    # Test search
    results = catalog.search("Heisenberg")
    assert any(f.family_id == 271 for f in results)

    # Test filtering by discipline
    phys_families = catalog.list_families(discipline="Mathematical physics")
    assert len(phys_families) > 0


def test_comparator_challenge_registry():
    registry = ComparatorChallengeRegistry()
    specs = registry.load_all()
    assert len(specs) > 0

    # Test challenge retrieval
    challenge = registry.get_challenge("QuasiRiemannHypothesis")
    assert challenge is not None
    assert "propext" in challenge.permitted_axioms
    assert "Quot.sound" in challenge.permitted_axioms
    assert "Classical.choice" in challenge.permitted_axioms


def test_comparator_axiom_verification(tmp_path):
    registry = ComparatorChallengeRegistry()
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))

    # Valid proof: only permitted axioms used
    res_valid = registry.verify_axioms(
        "QuasiRiemannHypothesis",
        ["propext", "Classical.choice"],
        ledger=ledger,
    )
    assert res_valid.ok
    assert len(res_valid.unpermitted_axioms) == 0
    assert res_valid.ledger_ref is not None

    # Invalid proof: unpermitted axiom / cheat axiom used
    res_invalid = registry.verify_axioms(
        "QuasiRiemannHypothesis",
        ["propext", "sorryAx", "arbitrary_magic_axiom"],
        ledger=ledger,
    )
    assert not res_invalid.ok
    assert "sorryAx" in res_invalid.unpermitted_axioms
    assert "arbitrary_magic_axiom" in res_invalid.unpermitted_axioms
    assert "Forbidden axioms detected" in (res_invalid.error_message or "")


def test_comparator_live_or_fallback(tmp_path):
    registry = ComparatorChallengeRegistry()
    ledger = Ledger(path=str(tmp_path / "ledger_live.jsonl"))

    # When lake is absent (standard host), verify_lean4_live falls back gracefully to AST containment
    res = registry.verify_lean4_live("QuasiRiemannHypothesis", ledger=ledger)
    assert res.ok
    assert res.verification_mode in ("live_kernel_proven", "static_ast_containment")
    if res.verification_mode == "static_ast_containment":
        assert res.kernel_execution_skipped is True
    else:
        assert res.kernel_execution_skipped is False
    assert res.ledger_ref is not None

