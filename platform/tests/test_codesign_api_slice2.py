"""Slice-2 API: truth-table specs, kappa sweep, budget ledger."""

import os
import tempfile

os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-codesign-s2-"))

from starlette.testclient import TestClient

from kubona.console_api import app

client = TestClient(app)
TOKEN = "dev-token"


def _h():
    return {"Authorization": f"Bearer {TOKEN}"}


def _and_table():
    return {
        "num_qubits": 3, "inputs": [0, 1], "outputs": [2],
        "table": {
            "00": {"out": {"2": 0}}, "01": {"out": {"2": 0}},
            "10": {"out": {"2": 0}}, "11": {"out": {"2": 1}},
        },
    }


def test_table_spec_evaluate_with_sweep():
    r = client.post("/api/codesign/evaluate",
                    json={"spec_table": _and_table(),
                          "artifact": [["ccx", [0, 1, 2], []]],
                          "num_tests": 8, "kappa": 1}, headers=_h())
    assert r.status_code == 200
    body = r.json()
    assert body["ok"] is True and body["result"]["valid"] is True
    assert body["result"]["kappa_sweep"] == {"1": 3, "2": 9, "3": 27}


def test_table_partial_refused():
    bad = _and_table()
    bad["table"] = {"00": {"out": {"2": 0}}}
    r = client.post("/api/codesign/evaluate",
                    json={"spec_table": bad, "artifact": []}, headers=_h())
    assert r.status_code == 400 and r.json()["ok"] is False


def test_named_xor2():
    # inputs preserved, output 2 ^= a ^ b (two CNOTs, T=0)
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "xor2",
                          "artifact": [["cx", [0, 2], []], ["cx", [1, 2], []]],
                          "num_tests": 8}, headers=_h())
    assert r.status_code == 200 and r.json()["result"]["valid"] is True
    assert r.json()["result"]["toffoli"] == 0


def test_budget_union_bound():
    r = client.post("/api/codesign/budget",
                    json={"entries": [
                        {"source": "carry", "per_use_rate": 0.5e-6, "uses": 40},
                        {"source": "history", "per_use_rate": 2.4e-6, "uses": 2},
                    ]}, headers=_h())
    assert r.status_code == 200
    body = r.json()
    assert body["ok"] is True
    assert abs(body["union_bound"] - (0.5e-6 * 40 + 2.4e-6 * 2)) < 1e-15
    assert body["rows"][-1]["source"] == "union_bound"


def test_budget_refuses_nan_and_oversize():
    r = client.post("/api/codesign/budget",
                    json={"entries": [{"source": "x", "per_use_rate": "nan", "uses": 1}]},
                    headers=_h())
    assert r.status_code == 400
    r = client.post("/api/codesign/budget",
                    json={"entries": [{"source": "x", "per_use_rate": 0.1, "uses": 1}] * 33},
                    headers=_h())
    assert r.status_code == 400


def test_no_submission_on_new_routes():
    assert client.post("/api/codesign/budget/submit", headers=_h()).status_code == 404
