"""Intelligence entity store: upsert, tiers, corrections, snapshots.

Stub-driven throughout (no network, no accounts). Ledger assertions use a
temporary path so tests never touch the operator ledger.
"""

import pytest

from kubona.epistemics.claim_grammar import EvidenceTier
from kubona.intelligence.entities import (
    EntityStore, IntelligenceError, deterministic_id,
)
from kubona.provenance import Ledger


def make_store(tmp_path):
    return EntityStore(ledger=Ledger(path=tmp_path / "test-ledger.jsonl"))


def test_deterministic_ids_stable():
    assert deterministic_id("company", "  QunaSys  ") == deterministic_id("company", "qunasys")
    assert deterministic_id("company", "a") != deterministic_id("investor", "a")


def test_upsert_dedups_by_alias_and_merges(tmp_path):
    store = make_store(tmp_path)
    a = store.upsert("company", "QunaSys", aliases=["QunaSys Inc"],
                     provenance={"source_url": "https://example.com/q"})
    b = store.upsert("company", "QunaSys Inc")
    assert a.id == b.id and b.version == 2
    assert a.tier == EvidenceTier.T3_SAMPLED  # machine input capped at T3


def test_machine_input_above_t3_refused(tmp_path):
    store = make_store(tmp_path)
    with pytest.raises(IntelligenceError, match="named validator"):
        store.upsert("company", "X", tier=EvidenceTier.T2_CONSISTENT)


def test_alias_conflict_refuses_silent_merge(tmp_path):
    store = make_store(tmp_path)
    store.upsert("company", "Alpha Quantum", aliases=["AQ"])
    with pytest.raises(IntelligenceError, match="alias conflict"):
        store.upsert("company", "Another Qubits", aliases=["AQ"])


def test_promotion_ladder_with_sources(tmp_path):
    store = make_store(tmp_path)
    ent = store.upsert("company", "Beta Q", provenance={"source_url": "https://a.example"})
    with pytest.raises(IntelligenceError, match="primary-source"):
        store.promote(ent.id, EvidenceTier.T2_CONSISTENT, validator="analyst-1")
    store.promote(ent.id, EvidenceTier.T2_CONSISTENT, validator="analyst-1",
                  primary_source="https://a.example/filing")
    assert ent.tier == EvidenceTier.T2_CONSISTENT
    with pytest.raises(IntelligenceError, match="second independent source"):
        store.promote(ent.id, EvidenceTier.T1_VERIFIED, validator="analyst-1",
                      primary_source="https://a.example/filing")
    store.promote(ent.id, EvidenceTier.T1_VERIFIED, validator="analyst-2",
                  primary_source="https://b.example/registry")
    assert ent.tier == EvidenceTier.T1_VERIFIED and ent.supersedes


def test_no_tier_skipping(tmp_path):
    store = make_store(tmp_path)
    ent = store.upsert("company", "Gamma Q")
    with pytest.raises(IntelligenceError, match="no skipping tiers"):
        store.promote(ent.id, EvidenceTier.T1_VERIFIED, validator="analyst-1",
                      primary_source="https://x.example")


def test_correction_chains_and_retraction_preserves_id(tmp_path):
    store = make_store(tmp_path)
    ent = store.upsert("company", "Delta Q")
    store.correct(ent.id, {"name": "Delta Quantum"}, validator="analyst-1",
                  reason="legal name from filing")
    assert ent.version == 2 and ent.supersedes == f"{ent.id}@v1"
    store.retract(ent.id, validator="analyst-1", reason="dissolved 2026")
    assert ent.retracted and ent.id in store.entities


def test_link_and_snapshot(tmp_path):
    store = make_store(tmp_path)
    c = store.upsert("company", "Epsilon Q")
    i = store.upsert("investor", "Zeta Capital")
    rel = store.link("invested_in", i.id, c.id, provenance={"source_url": "https://r.example"})
    assert rel.id in store.relationships
    with pytest.raises(IntelligenceError, match="must exist"):
        store.link("invested_in", "company-deadbeef", c.id)
    snap = store.snapshot("v2026.10.11")
    assert snap["counts"]["company"] == 1 and snap["methodology"]
    assert snap["update_log"][-1]["event"] == "link"  # export frozen before self-log
    assert store._change_log[-1]["event"] == "snapshot"  # ...but the log records it
    assert (tmp_path / "test-ledger.jsonl").exists()
