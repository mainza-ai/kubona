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


def test_events_stream():
    """Asserts /api/events SSE returns event-stream and telemetry events."""
    r = client.get("/api/events", headers=_h())
    assert r.status_code == 200
    assert "text/event-stream" in r.headers["content-type"]
    assert "event: telemetry" in r.text
    assert "approvals" in r.text
    assert "loops" in r.text


def test_science_discovery_endpoints():
    """Validates the 5 dedicated science discovery REST endpoints."""
    # 1. Quantum Triage
    triage_payload = {
        "molecule_id": "FeMoco",
        "formula": "Fe7MoS9C",
        "num_electrons": 54,
        "is_multi_reference": True,
        "coulomb_continuum": True,
        "estimated_active_space_orbitals": 54,
    }
    r = client.post("/api/science/triage", json=triage_payload, headers=_h())
    assert r.status_code == 200
    res = r.json()
    assert res["ok"] is True
    assert res["triage"]["target"] == "FEDERATED_QUANTUM_QPU"
    assert res["triage"]["requires_qpu"] is True

    # 2. Quantum Krylov Subspace Diagonalization
    krylov_payload = {
        "subspace_dim": 3,
        "time_step": 0.1,
    }
    r = client.post("/api/science/krylov", json=krylov_payload, headers=_h())
    assert r.status_code == 200
    k_res = r.json()
    assert k_res["ok"] is True
    assert "ground_state_energy" in k_res["result"]

    # 3. Classical Shadow Tomography
    shadow_payload = {
        "num_qubits": 2,
        "num_snapshots": 150,
        "observables": ["ZZ", "XX"],
    }
    r = client.post("/api/science/shadows", json=shadow_payload, headers=_h())
    assert r.status_code == 200
    s_res = r.json()
    assert s_res["ok"] is True
    assert "ZZ" in s_res["estimates"]

    # 4. Bilingual Manuscript Synthesis & Peer Review Tribunal
    ms_payload = {
        "title": "Quantum Krylov Resolution of FeMoco Ground State",
        "target_system": "FeMoco",
        "anchor_name": "Heisenberg_Ferromagnet_3D",
        "anchor_val": -1.5,
        "measured_val": -1.499,
        "lean4_hash": "a1b2c3d4e5f67890",
    }
    r = client.post("/api/science/manuscripts", json=ms_payload, headers=_h())
    assert r.status_code == 200
    m_res = r.json()
    assert m_res["ok"] is True
    assert "latex_main" in m_res
    assert "bibtex" in m_res
    assert m_res["peer_review"]["rigor"] >= 0.0

    # 5. HippoRAG / Tiered Epistemic Memory Associative Query
    mem_payload = {
        "start_id": "ent_femoco",
        "max_hops": 2,
    }
    r = client.post("/api/science/memory/query", json=mem_payload, headers=_h())
    assert r.status_code == 200
    mem_res = r.json()
    assert mem_res["ok"] is True
    assert "recall" in mem_res
    assert "pagerank" in mem_res

    # 6. Epistemic Tribunal Swarm Deliberation & Quorum Consensus
    tribunal_payload = {
        "hypothesis_id": "hyp-qksd-femoco-01",
        "target_system": "FeMoco",
        "lean4_verified": True,
        "zero_axiom_cheating": True,
        "gradient_variance": 0.082,
    }
    r = client.post("/api/science/tribunal/deliberate", json=tribunal_payload, headers=_h())
    assert r.status_code == 200
    trib_res = r.json()
    assert trib_res["ok"] is True
    assert trib_res["quorum_percentage"] >= 75.0
    assert trib_res["verdict"] == "COMMITTED_TO_LEDGER"
    assert len(trib_res["votes"]) == 6

    # 7. QEC Hardware Resource Estimator (Neutral Atom & IBM Gross Code)
    qec_payload = {
        "num_logical_qubits": 12,
        "logical_clifford_count": 2000,
        "logical_t_count": 500,
        "physical_error_rate": 0.001,
    }
    r = client.post("/api/science/qec/estimate", json=qec_payload, headers=_h())
    assert r.status_code == 200
    qec_res = r.json()
    assert qec_res["ok"] is True
    assert qec_res["architectures"]["neutral_atom_quera"]["supported"] is True
    assert qec_res["architectures"]["bivariate_bicycle_qldpc_ibm"]["physical_qubits_total"] == 288


