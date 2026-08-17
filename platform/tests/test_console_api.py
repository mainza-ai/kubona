"""Console REST API tests: read/approve-only surface, auth boundary."""

import json
import os
import tempfile

import pytest
from starlette.testclient import TestClient

# isolate the API's stores BEFORE importing the app (module-level singletons)
os.environ["KUBONA_STATE_DIR"] = tempfile.mkdtemp(prefix="kubona-api-test-")

from kubona.console_api import app

client = TestClient(app)
TOKEN = "dev-token"


def _h():
    return {"Authorization": f"Bearer {TOKEN}"}


def test_requires_auth():
    assert client.get("/api/health").status_code == 401
    assert client.get("/api/sessions").status_code == 401


def test_health_and_providers():
    assert client.get("/api/health", headers=_h()).json()["ok"] is True
    provs = client.get("/api/providers", headers=_h()).json()["providers"]
    assert "local-simulator" in provs and "qiskit-aer-simulator" in provs


def test_read_only_surface_no_submission_endpoints():
    """The two hard rules: the console API must never submit jobs."""
    for path in ["/api/submit", "/api/jobs", "/api/execution", "/api/compiler"]:
        assert client.get(path, headers=_h()).status_code == 404
        assert client.post(path, headers=_h()).status_code == 404


def test_models_and_settings_endpoints():
    # auto-detect scans ports; with nothing up it returns an honest empty scan
    r = client.get("/api/models", headers=_h())
    assert r.status_code == 200 and "scan" in r.json()
    # settings GET (user choices + separate live host snapshot) + PUT round-trip
    r = client.get("/api/settings", headers=_h())
    assert r.status_code == 200 and "settings" in r.json()
    r = client.put("/api/settings", json={"llm_provider": "scripted", "llm_model": "none",
                                          "llm_temperature": 0.1}, headers=_h())
    assert r.status_code == 200 and r.json()["ok"] is True
    r2 = client.get("/api/settings", headers=_h())
    assert r2.json()["settings"].get("llm_provider") == "scripted"


def test_sessions_and_burn_rate():
    r = client.get("/api/sessions", headers=_h())
    assert r.json()["ok"] is True


def test_approval_flow_over_rest():
    # seed an approval through the same service the API reads
    from kubona.session import SessionStore
    from kubona.cost.governance import CostGovernanceService

    gov = CostGovernanceService(SessionStore())
    s = gov.sessions.create("api-test", "researcher", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 42.0, "paid run")

    # The API service and this test share the same KUBONA_STATE_DIR store, so the
    # seeded approval is visible to the API; use its id directly (deterministic
    # regardless of other tests' approvals in the shared store).
    r = client.post(f"/api/approvals/{a.approval_id}/hold", headers=_h())
    assert r.status_code == 200 and r.json()["ok"] is True
    r = client.post(f"/api/approvals/{a.approval_id}/approve", headers=_h())
    assert r.json()["state"] == "APPROVED"
    r = client.post(f"/api/approvals/{a.approval_id}/reject", headers=_h())
    assert r.json()["ok"] is False  # already approved -> governance error surfaced
