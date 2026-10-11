"""Taxonomy v0: controlled vocabularies, suggestion discipline, tagging."""

import pytest

from kubona.intelligence.entities import EntityStore, IntelligenceError
from kubona.intelligence.taxonomy import (
    DOMAINS, SEGMENTS, STAGES, TECHNOLOGIES, UNCLASSIFIED,
    suggest, tag_entity, validate_tags,
)
from kubona.provenance import Ledger


def make_store(tmp_path):
    return EntityStore(ledger=Ledger(path=tmp_path / "tax-ledger.jsonl"))


def test_technology_axis_reuses_modality_vocabulary():
    assert "superconducting" in TECHNOLOGIES and "annealing" in TECHNOLOGIES
    assert set(DOMAINS) == {"computing", "networking", "sensing"}


def test_validate_tags_refuses_unknown():
    good = {"technology": "trapped_ion", "domain": "computing",
            "segment": "hardware", "stage": "series-a"}
    assert validate_tags(good) == good
    for bad in ({"technology": "quantum-ish"}, {"domain": "teleportation"},
                {"segment": "vibes"}, {"stage": "moon"}):
        with pytest.raises(IntelligenceError, match="unknown"):
            validate_tags(bad)


def test_suggest_returns_value_or_unclassified_never_guess():
    out = suggest("We build superconducting transmon processors with high coherence")
    assert out["technology"] == "superconducting"
    vague = suggest("We do quantum stuff for the future")
    assert vague["technology"] == UNCLASSIFIED
    tie = suggest("Our trapped-ion and neutral-atom Rydberg systems lead")
    assert tie["technology"] == UNCLASSIFIED  # tie refuses, analyst decides


def test_tag_entity_names_validator_and_logs(tmp_path):
    store = make_store(tmp_path)
    ent = store.upsert("company", "Eta Q")
    with pytest.raises(IntelligenceError, match="named validator"):
        tag_entity(store, ent.id, {"technology": "photonic"}, validator="")
    got = tag_entity(store, ent.id,
                     {"technology": "photonic", "domain": "computing",
                      "segment": "hardware"},
                     validator="analyst-1")
    assert got["technology"] == "photonic"
    assert ent.provenance["taxonomy_v0"] == got
    assert store._change_log[-1]["event"] == "tag"
    assert (tmp_path / "tax-ledger.jsonl").exists()
