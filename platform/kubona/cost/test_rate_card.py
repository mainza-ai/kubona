import json
from pathlib import Path

import pytest

from kubona.cost.rate_card import RateCardError, apply_rate_card, validate_rate_card


def _card(**over):
    card = {
        "model": "per_shot", "currency": "USD", "per_shot_usd": 0.01,
        "notes": "Pay-as-you-go list price", "verified": "2026-10-27",
        "source": "https://example.com/pricing",
    }
    card.update(over)
    return card


def test_validate_accepts_verified_card():
    out = validate_rate_card("ionq", _card())
    assert out["per_shot_usd"] == 0.01


@pytest.mark.parametrize("over", [
    {"model": "per_shot", "per_shot_usd": 0},
    {"verified": "27-10-2026"},
    {"verified": ""},
    {"source": "not-a-url"},
    {"notes": ""},
    {"currency": "EUR"},
    {"model": "mystery"},
])
def test_validate_refuses_bad_cards(over):
    with pytest.raises(RateCardError):
        validate_rate_card("ionq", _card(**over))


def test_apply_dry_run_does_not_write(tmp_path):
    schema = tmp_path / "ionq.json"
    schema.write_text(json.dumps({"provider": "ionq", "pricing": {"model": "free"}}))
    apply_rate_card("ionq", _card(), write=False, schema_dir=tmp_path)
    assert json.loads(schema.read_text())["pricing"]["model"] == "free"


def test_apply_write_is_atomic_and_merges(tmp_path):
    schema = tmp_path / "ionq.json"
    schema.write_text(json.dumps({"provider": "ionq", "physical_qubits": 11, "pricing": {"model": "free"}}))
    doc = apply_rate_card("ionq", _card(), write=True, schema_dir=tmp_path)
    written = json.loads(schema.read_text())
    assert written["physical_qubits"] == 11
    assert written["pricing"] == doc["pricing"]
    assert not (tmp_path / "ionq.json.tmp").exists()


def test_apply_refuses_provider_mismatch(tmp_path):
    schema = tmp_path / "ionq.json"
    schema.write_text(json.dumps({"provider": "rigetti"}))
    with pytest.raises(RateCardError):
        apply_rate_card("ionq", _card(), schema_dir=tmp_path)
