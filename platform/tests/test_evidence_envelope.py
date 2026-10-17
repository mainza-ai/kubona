"""R1 conformance: the Python rail reproduces every checked-in evidence
vector byte-exactly (canonical JSON + evidence_id). The Go rail asserts
the same file — agreement across the file IS the cross-language proof."""

import json
from pathlib import Path

import pytest

from kubona.provenance.envelope import build, canonical_bytes, verify

VECTORS = Path(__file__).resolve().parents[1] / "contracts" / "evidence-vectors.json"


def _load():
    doc = json.loads(VECTORS.read_text())
    assert doc["version"] == 1
    return doc["fixtures"]


def test_vectors_reproduce():
    for fx in _load():
        env = build(
            fx["inputs"]["normalized_result"],
            fx["inputs"]["analysis_profile"],
            fx["inputs"]["tenant_id"],
            fx["inputs"]["raw_result"],
            fx["inputs"]["mandate_ref"],
        )
        assert env["evidence_id"] == fx["evidence_id"], fx["name"]
        body = {k: v for k, v in env.items() if k != "evidence_id"}
        assert canonical_bytes(body).decode("utf-8") == fx["canonical_json"], fx["name"]
        verify(env)  # sealed envelopes verify


def test_missing_profile_refused():
    with pytest.raises(ValueError, match="analysis_profile"):
        build({"x": 1}, {}, "lab")
    with pytest.raises(ValueError, match="method"):
        build({"x": 1}, {"backend": "b"}, "lab")


def test_missing_bindings_refused():
    with pytest.raises(ValueError, match="normalized_result"):
        build({}, {"method": "m"}, "lab")
    with pytest.raises(ValueError, match="tenant_id"):
        build({"x": 1}, {"method": "m"}, "")


def test_nonfinite_refused():
    with pytest.raises(ValueError, match="non-finite"):
        build({"x": float("nan")}, {"method": "m"}, "lab")
    with pytest.raises(ValueError, match="non-finite"):
        build({"x": float("inf")}, {"method": "m"}, "lab")


def test_tamper_refused():
    (fx,) = [f for f in _load() if f["name"] == "vqe-minimal"]
    env = build(
        fx["inputs"]["normalized_result"],
        fx["inputs"]["analysis_profile"],
        fx["inputs"]["tenant_id"],
    )
    mutated = dict(env)
    mutated["normalized_result"] = dict(env["normalized_result"])
    mutated["normalized_result"]["theta"] = 3.7
    with pytest.raises(ValueError, match="does not match content"):
        verify(mutated)
    wrong_id = dict(env)
    wrong_id["evidence_id"] = "ev_" + "0" * 64
    with pytest.raises(ValueError, match="does not match content"):
        verify(wrong_id)
