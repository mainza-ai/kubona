"""R5 conformance, Python rail: KubonaError codes are a subset of the
single-source CSV (Go asserts full identity); construction refuses
unknown codes; the taxonomy refusal is typed and non-retryable."""

import csv
from pathlib import Path

import pytest

from kubona.errors import ERROR_CATALOG, KubonaError
from kubona.provenance import Ledger

CONTRACT = Path(__file__).resolve().parents[1] / "contracts" / "error-catalog.csv"


def test_codes_match_contract():
    rows = list(csv.DictReader(CONTRACT.read_text().splitlines()))
    assert len(rows) == 22, f"catalog grew/shrank: {len(rows)} rows"
    assert set(ERROR_CATALOG) == {r["code"] for r in rows}
    for code in ERROR_CATALOG:
        e = KubonaError(code, "probe")
        assert isinstance(e, ValueError)  # existing handlers keep working
        d = e.to_dict()
        assert d["code"] == code
        assert set(d) == {"code", "category", "message", "retryable", "remediation", "details"}


def test_unknown_code_refuses():
    with pytest.raises(ValueError, match="uncatalogued"):
        KubonaError("no_such_code", "x")


def test_taxonomy_refusal_is_typed(tmp_path):
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    with pytest.raises(KubonaError) as exc:
        ledger.append("typo_kind_xyz", {"x": 1})
    assert exc.value.code == "bad_request"
    assert exc.value.retryable is False
    assert exc.value.details == {"kind": "typo_kind_xyz"}
    assert ledger.count() == 0
