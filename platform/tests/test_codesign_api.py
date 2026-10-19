"""Codesign console_api routes: auth, fail-closed specs, real scoring."""

import os
import tempfile

os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-codesign-api-"))

from starlette.testclient import TestClient

from kubona.console_api import app

client = TestClient(app)
TOKEN = "dev-token"


def _h():
    return {"Authorization": f"Bearer {TOKEN}"}


def _and_artifact():
    # valid AND2: ccx 0,1 -> 2
    return [["ccx", [0, 1, 2], []]]


def test_requires_auth():
    assert client.post("/api/codesign/evaluate", json={}).status_code == 401
    assert client.post("/api/codesign/frontier", json={}).status_code == 401


def test_unknown_spec_refused():
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "shor", "artifact": []}, headers=_h())
    assert r.status_code == 400 and r.json()["ok"] is False


def test_evaluate_valid_and2():
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "and2", "artifact": _and_artifact(),
                          "num_tests": 8, "kappa": 1}, headers=_h())
    assert r.status_code == 200
    body = r.json()
    assert body["ok"] is True
    res = body["result"]
    assert res["valid"] is True
    assert res["toffoli"] == 1 and res["peak_qubits"] == 3
    assert res["tq"] == 3


def test_evaluate_invalid_scores_worst_not_valid():
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "and2", "artifact": [["cx", [0, 2], []]],
                          "num_tests": 8}, headers=_h())
    assert r.status_code == 200 and r.json()["result"]["valid"] is False


def test_frontier_best_is_min_tq():
    arts = [_and_artifact(), [["cx", [0, 2], []]], [["ccx", [0, 1, 2], []]]]
    r = client.post("/api/codesign/frontier",
                    json={"spec_kind": "and2", "artifacts": arts, "kappa": 1},
                    headers=_h())
    assert r.status_code == 200
    body = r.json()
    assert body["ok"] is True and body["frontier"]["best"]["tq"] == 3
    assert len(body["details"]) == 3


def test_frontier_refuses_oversize():
    r = client.post("/api/codesign/frontier",
                    json={"spec_kind": "and2",
                          "artifacts": [_and_artifact()] * 65}, headers=_h())
    assert r.status_code == 400


def test_no_submission_endpoint_added():
    # read-only rail: scoring only, never job submission
    for path in ["/api/codesign/submit", "/api/codesign/launch"]:
        assert client.post(path, headers=_h()).status_code == 404
