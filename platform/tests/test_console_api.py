"""Console REST API tests: read/approve-only surface, auth boundary."""

import json
import os
import tempfile

import pytest
from starlette.testclient import TestClient

# isolate the API's stores BEFORE importing the app (module-level singletons).
# setdefault (first-wins): sibling suites (e.g. test_codesign_api) bind the
# same singletons when they import first — overwriting here would split the
# API's store from this file's fresh SessionStore and break the shared-store
# approval test without any logic change. Strength unchanged.
os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-api-test-"))

from kubona.console_api import app

client = TestClient(app)
TOKEN = "dev-token"


def _h():
    return {"Authorization": f"Bearer {TOKEN}"}


def test_requires_auth():
    assert client.get("/api/health").status_code == 401
    assert client.get("/api/labs").status_code == 401


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
    r = client.put("/api/settings", json={"llm_provider": "scripted", "llm_model": "none"}, headers=_h())
    assert r.status_code == 200 and r.json()["ok"] is True
    r2 = client.get("/api/settings", headers=_h())
    assert r2.json()["settings"].get("llm_provider") == "scripted"


def test_settings_put_rejects_garbage_at_the_edge():
    """Legacy PUT must validate like apply_llm_edge: unknown keys,
    unknown providers, and blank models refuse instead of persisting."""
    for bad in ({"nope": 1},
                {"llm_provider": "mystery", "llm_model": "x"},
                {"llm_provider": "scripted", "llm_model": "  "}):
        r = client.put("/api/settings", json=bad, headers=_h())
        assert r.status_code == 400 and r.json()["ok"] is False


def test_retired_shadow_routes_stay_gone():
    """K5: Go-canonical routes must not resurrect here. Sessions, loops,
    events, chat, hf, mesh, fleet, provenance and custom-agents live on
    :8080; serving them here with divergent stores was split-brain."""
    for path in ["/api/sessions", "/api/loops", "/api/events",
                 "/api/fleet", "/api/provenance", "/api/mesh/route",
                 "/api/hf/search", "/api/agents/custom"]:
        assert client.get(path, headers=_h()).status_code == 404, path
    assert client.post("/api/chat/messages", json={}, headers=_h()).status_code == 404


def test_approval_flow_over_rest(monkeypatch):
    # seed an approval through the same service the API reads
    import os
    from kubona.session import SessionStore
    from kubona.cost.governance import CostGovernanceService
    # W0.9: approval mints a real signed mandate — operator key + explicit
    # lab/program bindings travel with the request, never defaults.
    monkeypatch.setenv("KUBONA_OPERATOR_PRIVKEY_HEX", os.urandom(32).hex())

    gov = CostGovernanceService(SessionStore())
    s = gov.sessions.create("api-test", "researcher", budget_ceiling_usd=100.0)
    a = gov.request_approval(s.session_id, 42.0, "paid run")

    # The API service and this test share the same KUBONA_STATE_DIR store, so the
    # seeded approval is visible to the API; use its id directly (deterministic
    # regardless of other tests' approvals in the shared store).
    r = client.post(f"/api/approvals/{a.approval_id}/hold", headers=_h())
    assert r.status_code == 200 and r.json()["ok"] is True
    r = client.post(f"/api/approvals/{a.approval_id}/approve?tenant_id=lab-a&job_hash=job-1", headers=_h())
    assert r.json()["state"] == "APPROVED"
    r = client.post(f"/api/approvals/{a.approval_id}/reject", headers=_h())
    assert r.json()["ok"] is False  # already approved -> governance error surfaced


def test_events_stream_retired_to_go():
    """K5: event streaming lives on Go :8080 (/api/events + /stream,
    tested there). This surface keeps no SSE duplicate."""
    assert client.get("/api/events", headers=_h()).status_code == 404


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
    # Fully attested evidence must commit; the refs below stand in for ledger
    # record hashes issued by the comparator, anchor solver, jobs runner,
    # QIR compiler and mandate verifier.
    tribunal_payload = {
        "hypothesis_id": "hyp-qksd-femoco-01",
        "target_system": "FeMoco",
        "lean4_verified": True,
        "zero_axiom_cheating": True,
        "verification_mode": "live_kernel_proven",
        "comparator_ledger_ref": "ledger:comparator:abc123",
        "anchor_ledger_ref": "ledger:anchor:def456",
        "krylov_ledger_ref": "ledger:krylov:ghi789",
        "qir_ledger_ref": "ledger:qir:jkl012",
        "mandate_ledger_ref": "ledger:mandate:mno345",
        "gradient_variance": 0.082,
    }
    r = client.post("/api/science/tribunal/deliberate", json=tribunal_payload, headers=_h())
    assert r.status_code == 200
    trib_res = r.json()
    assert trib_res["ok"] is True
    assert trib_res["quorum_percentage"] >= 75.0
    assert trib_res["verdict"] == "QUORUM_MET"
    assert trib_res["ledger_committed"] is False
    assert len(trib_res["votes"]) == 6
    prover = next(v for v in trib_res["votes"] if v["persona_id"] == "prover-01")
    assert prover["verification_mode"] == "live_kernel_proven"
    assert "machine-checked" in prover["rationale"]

    # 6b. Unattested evidence must falsify, never silently commit: the same
    # hypothesis with no refs must come back below quorum and rejected.
    unattested_payload = {
        "hypothesis_id": "hyp-qksd-femoco-01",
        "target_system": "FeMoco",
        "gradient_variance": 0.082,
    }
    r = client.post("/api/science/tribunal/deliberate", json=unattested_payload, headers=_h())
    assert r.status_code == 200
    unatt = r.json()
    assert unatt["ok"] is True
    assert unatt["verdict"] == "FALSIFIED_OR_REJECTED"
    assert unatt["quorum_percentage"] < 75.0
    falsified = [v for v in unatt["votes"] if v["verdict"] == "falsified"]
    assert len(falsified) >= 5, unatt["votes"]

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




def test_cors_origin_allowlist():
    """F-1: CORS echoes allowlisted origins only; foreign origins get no header."""
    from kubona import console_api
    # helper unit level: default, extras, ANY escape
    assert console_api._cors_allow_origins()[:2] == ["http://localhost:3000", "http://127.0.0.1:3000"]
    os.environ["KUBONA_CORS_ORIGINS"] = "https://console.example.com"
    try:
        assert "https://console.example.com" in console_api._cors_allow_origins()
    finally:
        del os.environ["KUBONA_CORS_ORIGINS"]
    os.environ["KUBONA_CORS_ANY"] = "1"
    try:
        assert console_api._cors_allow_origins() == ["*"]
    finally:
        del os.environ["KUBONA_CORS_ANY"]
    # live middleware level: allowlisted origin echoed, foreign origin blocked
    h = dict(_h(), Origin="http://localhost:3000")
    r = client.get("/api/health", headers=h)
    assert r.headers.get("access-control-allow-origin") == "http://localhost:3000"
    h = dict(_h(), Origin="https://evil.example.com")
    r = client.get("/api/health", headers=h)
    assert "access-control-allow-origin" not in r.headers


def test_cors_preflight_restricts_methods_and_headers():
    """F-1 audit: preflight must not bless arbitrary methods/headers."""
    origin = {"Origin": "http://localhost:3000", "Access-Control-Request-Method": "PATCH",
              "Access-Control-Request-Headers": "X-Custom"}
    r = client.options("/api/health", headers=dict(_h(), **origin))
    allow_methods = r.headers.get("access-control-allow-methods", "")
    assert "PATCH" not in allow_methods and "POST" in allow_methods
    allow_headers = r.headers.get("access-control-allow-headers", "")
    assert "X-Custom" not in allow_headers and "Authorization" in allow_headers


def test_science_endpoints_refuse_unbounded_inputs():
    cases = [
        ("/api/science/krylov", {"subspace_dim": 10000}),
        ("/api/science/krylov", {"hamiltonian": [[1, 0], [0, 1]], "subspace_dim": 4, "time_step": 1e9}),
        ("/api/science/shadows", {"num_qubits": 40, "num_snapshots": 400}),
        ("/api/science/shadows", {"num_qubits": 2, "num_snapshots": 10**9}),
        ("/api/science/qec/estimate", {"num_logical_qubits": 0}),
        ("/api/science/qec/estimate", {"physical_error_rate": 2.0}),
        ("/api/science/memory/query", {"max_hops": 999}),
        ("/api/science/triage", {"num_electrons": -1}),
    ]
    for path, body in cases:
        r = client.post(path, json=body, headers=_h())
        assert r.status_code == 400, (path, body, r.status_code, r.text)
        assert r.json()["ok"] is False


def test_science_malformed_numbers_are_typed_400s():
    """Malformed numbers must be typed 400s naming the bad field — never 500s
    (bare int()/float() conversions used to raise ValueError into a 500)."""
    cases = [
        ("/api/science/triage", {"num_electrons": "many"}, "num_electrons"),
        ("/api/science/triage", {"num_electrons": True}, "num_electrons"),
        ("/api/science/triage", {"num_electrons": 2.5}, "num_electrons"),
        ("/api/science/triage", {"nuclear_charges": [1, "a"]}, "nuclear_charges[1]"),
        ("/api/science/krylov", {"subspace_dim": "four"}, "subspace_dim"),
        ("/api/science/krylov", {"subspace_dim": 2.5}, "subspace_dim"),
        ("/api/science/krylov", {"time_step": "soon"}, "time_step"),
        ("/api/science/krylov", {"time_step": True}, "time_step"),
        ("/api/science/krylov", {"hamiltonian": [["a", "b"], ["c", "d"]]}, "hamiltonian"),
        ("/api/science/shadows", {"num_qubits": "two"}, "num_qubits"),
        ("/api/science/shadows", {"num_snapshots": 1.5}, "num_snapshots"),
        ("/api/science/shadows", {"seed": -1}, "seed"),
        ("/api/science/manuscripts", {"anchor_val": "low"}, "anchor_val"),
        ("/api/science/manuscripts", {"measured_val": None}, "measured_val"),
        ("/api/science/manuscripts", {"sorry_ax_count": "none"}, "sorry_ax_count"),
        ("/api/science/memory/query", {"max_hops": "far"}, "max_hops"),
        ("/api/science/memory/query", {"max_hops": True}, "max_hops"),
        ("/api/science/tribunal/deliberate", {"gradient_variance": "high"}, "gradient_variance"),
        ("/api/science/qec/estimate", {"num_logical_qubits": "twelve"}, "num_logical_qubits"),
        ("/api/science/qec/estimate", {"logical_t_count": 1.5}, "logical_t_count"),
        ("/api/science/qec/estimate", {"physical_error_rate": "tiny"}, "physical_error_rate"),
    ]
    for path, body, field in cases:
        r = client.post(path, json=body, headers=_h())
        assert r.status_code == 400, (path, body, r.status_code, r.text)
        assert r.json()["ok"] is False
        assert field in r.json()["error"], (path, body, r.json())


def test_console_token_rotation_without_restart(monkeypatch):
    """Per-request token read: rotating KUBONA_CONSOLE_TOKEN takes effect on
    the next request in BOTH directions (new works, old stops) — no reimport,
    no restart. The old import-time freeze pinned the token for the process."""
    monkeypatch.setenv("KUBONA_CONSOLE_TOKEN", "tok-A")
    assert client.get("/api/health",
                      headers={"Authorization": "Bearer tok-A"}).status_code == 200
    assert client.get("/api/health",
                      headers={"Authorization": "Bearer wrong"}).status_code == 401
    monkeypatch.setenv("KUBONA_CONSOLE_TOKEN", "tok-B")  # rotate
    assert client.get("/api/health",
                      headers={"Authorization": "Bearer tok-A"}).status_code == 401
    assert client.get("/api/health",
                      headers={"Authorization": "Bearer tok-B"}).status_code == 200
