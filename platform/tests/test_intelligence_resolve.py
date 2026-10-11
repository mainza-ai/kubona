"""ER pipeline: duplicates cluster, near-misses refuse, merges chain."""

import pytest

from kubona.intelligence.entities import EntityStore
from kubona.intelligence.resolve import apply_merges, propose_merges


def make_store():
    return EntityStore(ledger=None)


def test_true_duplicates_cluster():
    store = make_store()
    a = store.upsert("company", "QunaSys Incorporated")
    b = store.upsert("company", "QunaSys Inc.")
    c = store.upsert("company", "Qunasys")
    props = propose_merges(store)
    assert len(props) == 1
    assert sorted(props[0]["members"]) == sorted([a.id, b.id, c.id])


def test_chain_merge_refused():
    # A~B merges; C links B above threshold but scores below floor vs A:
    # verified merge must keep C out (the Portland/Seattle chaining failure).
    store = make_store()
    a = store.upsert("company", "Apex Boro Cinder Drift Ember")
    b = store.upsert("company", "Apex Boro Cinder Drift")
    c = store.upsert("company", "Apex Boro Cinder Drift Ferro Grove")
    props = propose_merges(store, threshold=0.6, floor=0.6)
    assert len(props) == 1
    assert sorted(props[0]["members"]) == sorted([a.id, b.id])
    assert all(c.id not in p["members"] for p in props)


def test_sparse_records_need_exact_hit():
    store = make_store()
    store.upsert("company", "AQ")
    store.upsert("company", "AQX")
    assert propose_merges(store) == []


def test_distinct_entities_stay_apart():
    store = make_store()
    store.upsert("company", "Superconducting Systems Corp")
    store.upsert("company", "Trapped Ion Dynamics")
    assert propose_merges(store) == []


def test_apply_merges_chains_and_preserves_ids():
    store = make_store()
    a = store.upsert("company", "QunaSys Incorporated")
    b = store.upsert("company", "Qunasys")
    props = propose_merges(store)
    with pytest.raises(Exception, match="validator"):
        apply_merges(store, props, validator="", reason="test")
    canon = apply_merges(store, props, validator="analyst-1", reason="filing check")
    assert canon == [a.id] or canon == [b.id]
    loser = b.id if canon == [a.id] else a.id
    assert store.entities[loser].retracted
    assert store.entities[canon[0]].version >= 2
