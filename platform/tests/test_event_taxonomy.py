"""R1a/C1 conformance: the Python rail validates against the same shared
taxonomy file as the Go rail (platform/contracts/event-types.json).
Drift in either direction is red."""

import json
from pathlib import Path

import pytest

from kubona.provenance import KNOWN_EVENT_TYPES, Ledger

CONTRACT = Path(__file__).resolve().parents[1] / "contracts" / "event-types.json"


def test_taxonomy_matches_contract():
    contract = json.loads(CONTRACT.read_text())
    assert contract["version"] == 1
    assert set(KNOWN_EVENT_TYPES) == set(contract["types"]), (
        "ledger registry != shared contract: "
        f"extra={sorted(set(KNOWN_EVENT_TYPES) - set(contract['types']))} "
        f"missing={sorted(set(contract['types']) - set(KNOWN_EVENT_TYPES))}"
    )


def test_every_row_has_contract():
    contract = json.loads(CONTRACT.read_text())["types"]
    for name, row in contract.items():
        assert row.get("emitters"), f"{name}: no emitters"
        assert set(row["emitters"]) <= {"go", "python"}, f"{name}: bad emitter"
        assert row.get("payload"), f"{name}: no payload contract"
        assert isinstance(row.get("required", []), list), f"{name}: required not a list"


def test_unknown_kind_refused(tmp_path):
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    with pytest.raises(ValueError, match="unknown ledger event kind"):
        ledger.append("typo_kind_xyz", {"x": 1})
    assert ledger.count() == 0  # refusal stores nothing


def test_registered_kinds_append(tmp_path):
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    for kind in ("trial", "workflow_step", "intelligence-upsert", "canary"):
        ledger.append(kind, {"probe": True})
    assert ledger.count() == 4
    ok, msg = ledger.validate_chain()
    assert ok, msg
