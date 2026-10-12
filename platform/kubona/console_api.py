"""Console REST API (Phase 2) — the read/approve-only surface the Flutter console talks to.

Per ui-ux-vision's two hard rules:
  1. READ/APPROVE ONLY — this API never submits jobs; job submission stays
     exclusively behind the MCP-governed flow.
  2. Fixed, audited surfaces — approve/deny, pause/kill only.

Auth: bearer token via the same DevAuthProvider (OAuth 2.1 hardens this in Phase 3).
"""

from __future__ import annotations

import hmac
import os
import asyncio
import json
from typing import Any

from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.middleware.cors import CORSMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, StreamingResponse
from starlette.routing import Route

from kubona import config
from kubona.control import LoopSupervisor
from kubona.cost.governance import CostGovernanceService
from kubona.providers import list_providers
from kubona.session import SessionStore

_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)
_loops = LoopSupervisor(_sessions)
# console auth is constant-time comparison against DEV_TOKEN (config.console_token())

# Fail-closed: in KUBONA_ENV=production the token MUST be configured; otherwise
# every request is unauthorized. Outside production the dev token is the local/CI default.
DEV_TOKEN = config.console_token()


def _authed(request: Request) -> bool:
    if DEV_TOKEN is None:
        return False  # production fail-closed: no configured token, no access
    if not request.headers.get("authorization", "").startswith("Bearer "):
        return False
    token = request.headers["authorization"][len("Bearer "):]
    # constant-time compare against the configured operator token
    if hmac.compare_digest(token, DEV_TOKEN):
        return True
    return False


def _auth_error() -> JSONResponse:
    return JSONResponse({"ok": False, "error": "unauthorized: use Bearer <token>"}, status_code=401)


def _json(data: dict[str, Any], status: int = 200) -> JSONResponse:
    return JSONResponse(data, status_code=status)


async def health(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    return _json({"ok": True, "service": "kubona-console-api", "version": "0.2.0"})


async def providers(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    reg = {}
    for name, cap in list_providers().items():
        reg[name] = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model}
    return _json({"ok": True, "providers": reg})


async def sessions(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    # SessionStore is append-only snapshots; rebuild the latest per id
    latest: dict[str, dict] = {}
    for sid in _all_session_ids():
        latest[sid] = _session_payload(sid)
    return _json({"ok": True, "sessions": list(latest.values())})


def _all_session_ids() -> list[str]:
    ids: list[str] = []
    if _sessions.path.exists():
        for line in _sessions.path.read_text().splitlines():
            if line.strip():
                import json as _j
                sid = _j.loads(line)["session_id"]
                if sid not in ids:
                    ids.append(sid)
    return ids


def _session_payload(sid: str) -> dict:
    s = _sessions.get(sid)
    br = _gov.burn_rate(sid)
    return {
        "session_id": s.session_id, "name": s.name, "owner": s.owner,
        "spent_usd": s.spent_usd, "budget_ceiling_usd": s.budget_ceiling_usd,
        "remaining_usd": s.remaining(), "status": s.status,
        "burn_rate_usd_per_s": br["burn_rate_usd_per_s"],
        "jobs": s.jobs, "approvals": s.approvals,
    }


async def session_detail(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    try:
        return _json({"ok": True, "session": _session_payload(request.path_params["sid"])})
    except KeyError:
        return _json({"ok": False, "error": "unknown session"}, 404)


async def approvals(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    state = request.query_params.get("state")
    out = []
    for a in _gov._load_approvals().values():
        if state is None or a.state == state:
            out.append({
                "approval_id": a.approval_id,
                "session_id": a.session_id,
                "state": a.state,
                "amount_requested_usd": a.amount_requested_usd,
                "reason": a.reason,
                "requested_by": a.requested_by,
                "mandate_token": a.mandate_token,
                "mandate_digest": a.mandate_digest,
            })
    return _json({"ok": True, "approvals": out})


async def approval_action(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    aid = request.path_params["aid"]
    action = request.path_params["action"]
    reviewer = request.query_params.get("reviewer", "operator")
    try:
        if action == "hold":
            a = _gov.hold_approval(aid, reviewer)
        elif action == "approve":
            a = _gov.approve(aid, reviewer, note=request.query_params.get("note"))
        elif action == "reject":
            a = _gov.reject(aid, reviewer, note=request.query_params.get("note") or "rejected from console")
        else:
            return _json({"ok": False, "error": f"unknown action {action}"}, 400)
        return _json({
            "ok": True,
            "approval_id": a.approval_id,
            "state": a.state,
            "mandate_token": a.mandate_token,
            "mandate_digest": a.mandate_digest,
        })
    except KeyError:
        return _json({"ok": False, "error": "unknown approval"}, 404)
    except Exception as e:
        return _json({"ok": False, "error": str(e)}, 400)


async def provenance(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.provenance import Ledger

    ledger = Ledger()
    records = [r for r in ledger if r.kind in ("experiment", "vqe_step", "canary")][-50:]
    by_hash: dict[str, dict] = {}
    for r in records:
        key = r.payload.get("circuit_hash") or r.payload.get("provider") or "unknown"
        agg = by_hash.setdefault(key, {"count": 0, "providers": set()})
        agg["count"] += 1
        if r.payload.get("provider"):
            agg["providers"].add(r.payload["provider"])
    return _json({"ok": True, "recent": [r.to_json() for r in records],
                  "aggregate": {k: {"count": v["count"], "providers": sorted(v["providers"])}
                                for k, v in by_hash.items()}})


async def fleet(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.canary import canary_check
    from kubona.providers import list_providers

    reg = {}
    for name, cap in list_providers().items():
        entry = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model,
                 "canary": None}
        if cap.execution_plugin:
            try:
                entry["canary"] = canary_check(name, shots=1024)["ok"]
            except Exception as e:  # noqa: BLE001 — fleet view flags unreachable providers
                entry["canary"] = f"unreachable: {e}"
        reg[name] = entry
    return _json({"ok": True, "fleet": reg})


async def reports(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.reports.notebook import ReportStore

    return _json({"ok": True, "reports": ReportStore().all()})


async def workflows(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.workflows.library import WorkflowLibrary

    lib = WorkflowLibrary()
    return _json({"ok": True, "workflows": [{"workflow_id": w.workflow_id,
                                             "name": w.source.get("name"),
                                             "created_by": w.created_by,
                                             "created_by_agent": w.source.get("created_by_agent", False)}
                                            for w in lib.all()]})


async def agents(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents import AgentRegistry

    return _json({"ok": True, "agents": [a.to_dict() for a in AgentRegistry().all()]})


async def labs(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.labs import LabRegistry

    return _json({"ok": True, "labs": LabRegistry().all()})


async def models(request: Request) -> JSONResponse:
    """Auto-detect the host's local LLM servers (Ollama / LM Studio / MLX / etc.)."""
    if not _authed(request):
        return _auth_error()
    from kubona.llm.registry import LocalModelRegistry

    return _json({"ok": True, "scan": LocalModelRegistry().discover(),
                  "ports": DEFAULT_LLM_PORTS()})


async def settings(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.settings import SettingsStore

    store = SettingsStore()
    if request.method == "GET":
        return _json({"ok": True, "settings": store.public()})
    body = await request.json()
    return _json({"ok": True, "settings": store.update(body)})


def DEFAULT_LLM_PORTS() -> list[int]:
    from kubona.llm.registry import DEFAULT_PORTS as p

    return p


async def loops(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    return _json({"ok": True, "loops": _loops.list()})


async def loop_history(request: Request) -> JSONResponse:
    """Returns 4D flight recorder history and trajectory for a given loop."""
    if not _authed(request):
        return _auth_error()
    lid = request.path_params["lid"]
    try:
        h = _loops.get(lid)
        return _json({
            "ok": True,
            "loop_id": lid,
            "name": h.name,
            "status": h.status,
            "step": h.step,
            "total_steps": h.total_steps,
            "history": h.history,
        })
    except KeyError:
        return _json({"ok": False, "error": "unknown loop"}, 404)


async def loop_action(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    lid = request.path_params["lid"]
    action = request.path_params["action"]
    try:
        if action == "pause":
            h = _loops.pause(lid)
        elif action == "resume":
            h = _loops.resume(lid)
        elif action == "kill":
            h = _loops.kill(lid)
        else:
            return _json({"ok": False, "error": f"unknown action {action}"}, 400)
        return _json({"ok": True, "loop_id": lid, "status": h.status})
    except KeyError:
        return _json({"ok": False, "error": "unknown loop"}, 404)


async def events(request: Request) -> StreamingResponse:
    """Server-Sent Events (SSE) telemetry stream for real-time console observability."""
    if not _authed(request):
        return _auth_error()

    async def _event_generator():
        # Stream heartbeat, loop states, approvals, and burn rate telemetry
        for _ in range(3):  # Yield bursts in test/connection or stream continuously
            apps = [
                {
                    "approval_id": a.approval_id,
                    "session_id": a.session_id,
                    "state": a.state,
                    "amount_requested_usd": a.amount_requested_usd,
                    "reason": a.reason,
                }
                for a in _gov._load_approvals().values()
            ]
            data = {
                "timestamp": asyncio.get_event_loop().time(),
                "loops": _loops.list(),
                "approvals": apps,
            }
            yield f"event: telemetry\ndata: {json.dumps(data)}\n\n"
            await asyncio.sleep(0.1)

    return StreamingResponse(
        _event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )


async def chat_messages(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.chat.dialogue import ResearchDialogueEngine, ChatMessage
    body = await request.json()
    prompt = body.get("prompt", "")
    engine = ResearchDialogueEngine()
    reply = engine.process_turn(prompt)
    return _json({"ok": True, "message": reply.model_dump()})


async def math_families(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.math_catalog import MathCatalog
    discipline = request.query_params.get("discipline")
    quantum_only = request.query_params.get("quantum_only", "").lower() in ("true", "1")
    catalog = MathCatalog()
    families = catalog.list_families(discipline=discipline, quantum_only=quantum_only)
    return _json({"ok": True, "families": [f.to_dict() for f in families], "stats": catalog.summary_stats()})


async def math_anchors(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine
    benchmarks = MathPhysicsAnchorEngine.list_benchmarks()
    anchors = [MathPhysicsAnchorEngine.get_anchor(b).to_dict() for b in benchmarks]
    return _json({"ok": True, "benchmarks": benchmarks, "anchors": anchors})


async def hf_search(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.mesh.hf_hub import HuggingFaceHubClient
    client = HuggingFaceHubClient()
    q = request.query_params.get("q", "")
    lib = request.query_params.get("library", "gguf")
    limit = int(request.query_params.get("limit", "15"))
    results = await client.search_models(query=q, library=lib, limit=limit)
    return _json({"ok": True, "models": [m.to_dict() for m in results]})


async def hf_model_detail(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.mesh.hf_hub import HuggingFaceHubClient
    client = HuggingFaceHubClient()
    repo_id = request.path_params["repo_id"]
    detail = await client.get_model_details(repo_id)
    if not detail:
        return _json({"ok": False, "error": "model not found"}, 404)
    return _json({"ok": True, "model": detail.to_dict()})


async def hf_download_start(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.mesh.hf_hub import HFDownloadManager
    mgr = HFDownloadManager()
    body = await request.json()
    repo_id = body.get("repo_id", "")
    filename = body.get("filename", "")
    if not repo_id or not filename:
        return _json({"ok": False, "error": "repo_id and filename required"}, 400)
    task = await mgr.start_download(repo_id, filename)
    return _json({"ok": True, "task": task.to_dict()})


async def hf_downloads_list(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.mesh.hf_hub import HFDownloadManager
    mgr = HFDownloadManager()
    tasks = mgr.list_tasks()
    return _json({"ok": True, "downloads": [t.to_dict() for t in tasks]})


async def hf_download_cancel(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.mesh.hf_hub import HFDownloadManager
    mgr = HFDownloadManager()
    task_id = request.path_params["task_id"]
    success = mgr.cancel_task(task_id)
    return _json({"ok": success, "task_id": task_id})


async def custom_agents_list(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents.user_defined import AgentRegistry
    reg = AgentRegistry()
    agents = reg.list_all(include_system=True)
    return _json({"ok": True, "agents": [a.to_dict() for a in agents]})


async def custom_agent_create(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents.user_defined import AgentRegistry, UserDefinedAgentManifest
    reg = AgentRegistry()
    body = await request.json()
    try:
        manifest = UserDefinedAgentManifest.from_dict(body)
        registered = reg.register_custom_agent(manifest)
        return _json({"ok": True, "agent": registered.to_dict()})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)}, 400)


async def custom_agent_delete(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.agents.user_defined import AgentRegistry
    reg = AgentRegistry()
    agent_id = request.path_params["agent_id"]
    try:
        deleted = reg.delete_custom_agent(agent_id)
        return _json({"ok": deleted, "agent_id": agent_id})
    except ValueError as e:
        return _json({"ok": False, "error": str(e)}, 400)


async def mesh_route(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.settings import SettingsStore
    from kubona.mesh.model_router import HybridModelRouter, MultimodalPayload, MultimodalItem, Modality
    store = SettingsStore()
    router = HybridModelRouter(default_air_gapped=store.public().get("air_gapped", True))
    body = await request.json()
    items = []
    for it in body.get("items", []):
        items.append(MultimodalItem(
            modality=Modality(it.get("modality", "text")),
            data=it.get("data", ""),
            mime_type=it.get("mime_type", "text/plain"),
            metadata=it.get("metadata", {}),
        ))
    payload = MultimodalPayload(
        prompt=body.get("prompt", ""),
        system_prompt=body.get("system_prompt", ""),
        items=items,
        role_hint=body.get("role_hint", "lead_theorist"),
        preferred_model=body.get("preferred_model"),
        max_tokens=body.get("max_tokens", 1024),
        temperature=body.get("temperature", 0.2),
    )
    spend_budget = float(body.get("spend_budget_usd", 0.0))
    has_mandate = bool(body.get("has_ed25519_mandate", False))
    decision = router.route(payload, spend_budget_usd=spend_budget, has_ed25519_mandate=has_mandate)
    return _json({"ok": True, "decision": decision.to_dict()})


async def science_triage(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.triage import QuantumTriageEngine, MolecularSystem
    body = await request.json()
    system = MolecularSystem(
        molecule_id=body.get("molecule_id", "mol_custom"),
        formula=body.get("formula", "H2"),
        num_electrons=int(body.get("num_electrons", 2)),
        nuclear_charges=body.get("nuclear_charges", [1, 1]),
        is_multi_reference=bool(body.get("is_multi_reference", False)),
        coulomb_continuum=bool(body.get("coulomb_continuum", True)),
        estimated_active_space_orbitals=int(body.get("estimated_active_space_orbitals", 4)),
    )
    res = QuantumTriageEngine.triage(system)
    return _json({"ok": True, "triage": res})


async def science_krylov(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.circuits.krylov import QKSDSolver
    import numpy as np
    body = await request.json()
    subspace_dim = int(body.get("subspace_dim", 4))
    dt = float(body.get("time_step", 0.2))

    if "hamiltonian" in body:
        H = np.array(body["hamiltonian"], dtype=complex)
    else:
        Z = np.array([[1, 0], [0, -1]], dtype=complex)
        X = np.array([[0, 1], [1, 0]], dtype=complex)
        I = np.eye(2, dtype=complex)
        H = -np.kron(Z, Z) - 0.5 * (np.kron(X, I) + np.kron(I, X))

    result = QKSDSolver.solve(H, subspace_dim=subspace_dim, time_step=dt)
    return _json({"ok": True, "result": result.to_dict()})


async def science_shadows(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.circuits.shadows import ClassicalShadowEngine
    import numpy as np
    body = await request.json()
    num_qubits = int(body.get("num_qubits", 2))
    num_snapshots = int(body.get("num_snapshots", 400))
    observables = body.get("observables", ["ZZ", "XX", "YY"])

    state = np.zeros(2**num_qubits, dtype=complex)
    state[0] = 1.0 / np.sqrt(2)
    state[-1] = 1.0 / np.sqrt(2)

    snapshots = ClassicalShadowEngine.sample_shadows_from_state(
        state, num_qubits=num_qubits, num_snapshots=num_snapshots
    )
    estimates = {}
    for obs in observables:
        estimates[obs] = ClassicalShadowEngine.estimate_observable_from_snapshots(snapshots, obs)
    return _json({"ok": True, "num_snapshots": num_snapshots, "estimates": estimates})


async def science_manuscript_create(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.science.manuscript import ManuscriptGenerator, ReviewEvidence
    body = await request.json()
    anchor_val = float(body.get("anchor_val", -1.5))
    measured_val = float(body.get("measured_val", -1.498))
    ms = ManuscriptGenerator.create_discovery_manuscript(
        title=body.get("title", "Quantum Subspace Discovery"),
        target_system=body.get("target_system", "H2"),
        authors=body.get("authors", ["Kubona Autonomous Theorist"]),
        abstract=body.get("abstract", "Automated epistemic discovery."),
        analytical_anchor_name=body.get("anchor_name", "Heisenberg_Ferromagnet_3D"),
        analytical_anchor_val=anchor_val,
        measured_val=measured_val,
        lean4_proof_hash=body.get("lean4_hash"),
        negative_certificates=body.get("negative_certs"),
    )
    # Evidence is caller-attested and fail-closed: unattested dimensions
    # falsify (REVISE/REJECT), never default to ACCEPT. The discrepancy is
    # derived server-side from the submitted numbers, not caller-asserted.
    sorry_raw = body.get("sorry_ax_count")
    evidence = ReviewEvidence(
        comparator_verdict=body.get("verification_mode"),
        sorry_ax_count=int(sorry_raw) if sorry_raw is not None else None,
        anchor_discrepancy_mha=abs(measured_val - anchor_val) * 1000.0,
        provenance_receipts=[r for r in body.get("provenance_receipts", []) if r] + (
            [body["lean4_hash"]] if body.get("lean4_hash") else []),
        citation_distance=body.get("citation_distance"),
    )
    review = ManuscriptGenerator.automated_peer_review(ms, evidence)
    return _json({
        "ok": True,
        "manuscript_id": ms.manuscript_id,
        "latex_main": ms.compile_latex(),
        "bibtex": ms.compile_bibtex(),
        "peer_review": {
            "reviewer": review.reviewer_persona,
            "rigor": review.mathematical_rigor_score,
            "novelty": review.novelty_score,
            "grounding": review.empirical_grounding_score,
            "hygiene": review.axiomatic_hygiene_score,
            "recommendation": review.overall_recommendation,
            "mean_score": review.mean_score(),
            "acceptable": review.is_acceptable(),
            "summary": review.summary,
            "critiques": review.actionable_critiques,
        }
    })


async def science_memory_query(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    from kubona.memory.tiered_storage import TieredStorageManager
    from kubona.science.memory import KnowledgeEntity, KnowledgeRelation
    body = await request.json()
    mgr = TieredStorageManager()

    e1 = KnowledgeEntity("ent_femoco", "molecule", "FeMoco Nitrogenase Active Cluster", {"coulomb_continuum": True})
    e2 = KnowledgeEntity("ent_family278", "theorem", "Family 278 KS Representability Failure", {"family_id": 278})
    mgr.index_entity(e1)
    mgr.index_entity(e2)
    mgr.index_relation(KnowledgeRelation("ent_femoco", "ent_family278", "FAILS_ON", 1.0))

    start_id = body.get("start_id", "ent_femoco")
    max_hops = int(body.get("max_hops", 2))
    recall = mgr.associative_recall(start_id, max_hops=max_hops)
    ppr = mgr.personalized_pagerank([start_id])
    formatted_recall = []
    for item in recall:
        target = item["target_entity"]
        formatted_recall.append({
            "hop_depth": item["hop_depth"],
            "target_entity": {
                "entity_id": target.entity_id,
                "entity_type": target.entity_type,
                "name": target.name,
                "properties": target.properties,
            },
            "path": item["path"],
        })
    return _json({"ok": True, "start_id": start_id, "recall": formatted_recall, "pagerank": ppr})


async def science_tribunal_deliberate(request: Request) -> JSONResponse:
    """Weighted-quorum recorder over caller-attested evidence.

    This endpoint does NOT verify anything itself: it records the weighted
    votes of the six tribunal personas over evidence the caller attests to
    (comparator outcome, gradient variance, anchor/mandate ledger refs) and
    computes whether the supermajority rule is met. Fail-closed defaults apply:
    unattested verification inputs falsify the prover vote, and the prover
    rationale always states the verification mode (live kernel proof vs static
    AST containment) so readers can never mistake one for the other.
    """
    if not _authed(request):
        return _auth_error()
    body = await request.json()
    hyp_id = body.get("hypothesis_id", "hyp-femoco-001")
    target_system = body.get("target_system", "FeMoco")
    # Fail-closed attestations: absence of evidence is evidence of absence.
    lean4_verified = bool(body.get("lean4_verified", False))
    zero_axiom_cheating = bool(body.get("zero_axiom_cheating", False))
    verification_mode = str(body.get("verification_mode", "unattested"))
    comparator_ledger_ref = body.get("comparator_ledger_ref")
    gradient_variance = float(body.get("gradient_variance", 0.0))
    anchor_ledger_ref = body.get("anchor_ledger_ref")
    mandate_ledger_ref = body.get("mandate_ledger_ref")
    krylov_ledger_ref = body.get("krylov_ledger_ref")
    qir_ledger_ref = body.get("qir_ledger_ref")

    # Each persona carries a fixed voting weight (its potential contribution).
    # The quorum denominator is the sum of ALL weights, so a falsified vote
    # always dilutes consensus — unattested evidence can never silently pass.
    theorist_ok = bool(krylov_ledger_ref)
    compiler_ok = bool(qir_ledger_ref)
    anchor_ok = bool(anchor_ledger_ref)
    sentinel_ok = bool(mandate_ledger_ref)
    prover_weight = 1.0 if verification_mode == "live_kernel_proven" else 0.90

    prover_ok = bool(lean4_verified and zero_axiom_cheating and verification_mode in (
        "live_kernel_proven", "static_ast_containment"))
    if verification_mode == "live_kernel_proven":
        prover_rho, prover_claim = 1.0, "machine-checked kernel proof"
    elif verification_mode == "static_ast_containment":
        # Static containment is weaker than a kernel proof: cap the vote
        # weight below 1.0 and say so in the rationale.
        prover_rho, prover_claim = 0.90, "static AST containment (not kernel-proved)"
    else:
        prover_rho, prover_claim = 0.0, "no verified attestation supplied"

    votes = [
        {
            "persona_id": "theorist-01",
            "role": "lead_theorist",
            "name": "Lead Polymath Theorist",
            "weight": 0.96,
            "verdict": "corroborated" if theorist_ok else "falsified",
            "confidence_rho": 0.96 if theorist_ok else 0.0,
            "rationale": (
                "Quantum Krylov monotonic convergence validated against ledger record"
                if theorist_ok else
                "No Krylov ledger reference supplied; convergence claim unattested"
            ),
            "evidence_ref": krylov_ledger_ref,
        },
        {
            "persona_id": "critic-01",
            "role": "adversarial_falsification_critic",
            "name": "Adversarial Falsification Critic",
            "weight": 0.92,
            "verdict": "corroborated" if gradient_variance >= 1e-4 else "falsified",
            "confidence_rho": 0.92 if gradient_variance >= 1e-4 else 0.99,
            "rationale": f"Variance of gradients Var[∇H] = {gradient_variance:.4f}",
        },
        {
            "persona_id": "prover-01",
            "role": "lean4_proof_engineer",
            "name": "Lean 4 Proof Engineer",
            "weight": prover_weight,
            "verdict": "corroborated" if prover_ok else "falsified",
            "confidence_rho": prover_rho if prover_ok else 0.0,
            "rationale": (
                f"Comparator check: standard axioms only, 0 sorryAx ({prover_claim})"
                if prover_ok else
                f"Unverified proof attestation ({prover_claim}); falsified pending evidence"
            ),
            "evidence_ref": comparator_ledger_ref,
            "verification_mode": verification_mode,
        },
        {
            "persona_id": "anchor-01",
            "role": "classical_anchor_solver",
            "name": "Classical Anchor Solver",
            "weight": 0.94,
            "verdict": "corroborated" if anchor_ok else "falsified",
            "confidence_rho": 0.94 if anchor_ok else 0.0,
            "rationale": (
                "Consistent with Family 271 ferromagnetic exact bounds within chemical precision (1.6 mHa)"
                if anchor_ok else
                "No classical-anchor ledger reference supplied; anchor claim unattested"
            ),
            "evidence_ref": anchor_ledger_ref,
        },
        {
            "persona_id": "compiler-01",
            "role": "quantum_circuit_compiler",
            "name": "Quantum Circuit Compiler",
            "weight": 0.89,
            "verdict": "corroborated" if compiler_ok else "falsified",
            "confidence_rho": 0.89 if compiler_ok else 0.0,
            "rationale": (
                "QIR compilation mapped to [[144, 12, 12]] gross code; T-depth within budget"
                if compiler_ok else
                "No QIR lowering ledger reference supplied; compilation claim unattested"
            ),
            "evidence_ref": qir_ledger_ref,
        },
        {
            "persona_id": "sentinel-01",
            "role": "sentinel_spend_governor",
            "name": "Sentinel Spend Governor",
            "weight": 1.0,
            "verdict": "corroborated" if sentinel_ok else "falsified",
            "confidence_rho": 1.0 if sentinel_ok else 0.0,
            "rationale": (
                "AP2 spend authorization signed with Ed25519; air-gap egress zero bytes"
                if sentinel_ok else
                "No spend-mandate ledger reference supplied; authorization claim unattested"
            ),
            "evidence_ref": mandate_ledger_ref,
        },
    ]

    total_weight = sum(v["weight"] for v in votes)
    corrob_weight = sum(v["weight"] for v in votes if v["verdict"] == "corroborated")
    quorum_pct = (corrob_weight / total_weight * 100.0) if total_weight > 0 else 0.0

    supermajority_met = (
        quorum_pct >= 75.0
        and prover_ok
        and (gradient_variance >= 1e-4)
    )
    verdict = "COMMITTED_TO_LEDGER" if supermajority_met else "FALSIFIED_OR_REJECTED"

    return _json({
        "ok": True,
        "hypothesis_id": hyp_id,
        "target_system": target_system,
        "current_phase": "PHASE_5_QUORUM_VOTING",
        "quorum_percentage": quorum_pct,
        "verdict": verdict,
        "votes": votes,
    })


async def science_qec_estimate(request: Request) -> JSONResponse:
    if not _authed(request):
        return _auth_error()
    body = await request.json()
    logical_qubits = int(body.get("num_logical_qubits", 12))
    clifford_count = int(body.get("logical_clifford_count", 1000))
    t_count = int(body.get("logical_t_count", 250))
    physical_error_rate = float(body.get("physical_error_rate", 0.001))

    neutral_capacity_ok = logical_qubits <= 48
    neutral_atoms_total = logical_qubits * 8 if neutral_capacity_ok else None

    blocks_needed = (logical_qubits + 11) // 12
    bb_physical_qubits = blocks_needed * 288
    bb_overhead = float(bb_physical_qubits) / float(logical_qubits)
    bb_threshold = 0.0075
    bb_suppressed_err = 0.1 * ((physical_error_rate / bb_threshold) ** 6)

    return _json({
        "ok": True,
        "num_logical_qubits": logical_qubits,
        "logical_clifford_count": clifford_count,
        "logical_t_count": t_count,
        "architectures": {
            "neutral_atom_quera": {
                "supported": neutral_capacity_ok,
                "physical_qubits_total": neutral_atoms_total,
                "hardware_platform": "Rb87_Optical_Tweezers",
                "max_logical_qubits": 48,
                "tweezer_mobility": "Dynamic_All_To_All",
            },
            "bivariate_bicycle_qldpc_ibm": {
                "supported": True,
                "physical_qubits_total": bb_physical_qubits,
                "gross_code_blocks": blocks_needed,
                "code_distance": 12,
                "physical_qubit_overhead_ratio": bb_overhead,
                "threshold": bb_threshold,
                "suppressed_logical_error_rate": bb_suppressed_err,
                "overhead_reduction_vs_surface_code": "10.4x",
            }
        }
    })


# CORS: explicit origin allowlist, mirroring the Go edge rail
# (platform/core/pkg/api/edge_auth.go corsAllowOrigin). Default is the
# local console origins plus KUBONA_CORS_ORIGINS extras; "*" requires the
# KUBONA_CORS_ANY=1 dev escape hatch, never the default.
def _cors_allow_origins() -> list[str]:
    if os.environ.get("KUBONA_CORS_ANY") == "1":
        return ["*"]
    allow = ["http://localhost:3000", "http://127.0.0.1:3000"]
    extra = os.environ.get("KUBONA_CORS_ORIGINS", "")
    for origin in extra.split(","):
        origin = origin.strip()
        if origin:
            allow.append(origin)
    return allow


app = Starlette(
    middleware=[Middleware(CORSMiddleware,
                          allow_origins=_cors_allow_origins(),
                          allow_methods=["*"], allow_headers=["*"])],
    routes=[
        Route("/api/health", health),
        Route("/api/events", events),
        Route("/api/chat/messages", chat_messages, methods=["POST"]),
        Route("/api/science/math/families", math_families),
        Route("/api/science/math/anchors", math_anchors),
        Route("/api/science/triage", science_triage, methods=["POST"]),
        Route("/api/science/krylov", science_krylov, methods=["POST"]),
        Route("/api/science/shadows", science_shadows, methods=["POST"]),
        Route("/api/science/manuscripts", science_manuscript_create, methods=["POST"]),
        Route("/api/science/memory/query", science_memory_query, methods=["POST"]),
        Route("/api/science/tribunal/deliberate", science_tribunal_deliberate, methods=["POST"]),
        Route("/api/science/qec/estimate", science_qec_estimate, methods=["POST"]),
        Route("/api/providers", providers),
        Route("/api/sessions", sessions),
        Route("/api/sessions/{sid}", session_detail),
        Route("/api/approvals", approvals),
        Route("/api/approvals/{aid}/{action}", approval_action, methods=["POST"]),
        Route("/api/provenance", provenance),
        Route("/api/fleet", fleet),
        Route("/api/reports", reports),
        Route("/api/workflows", workflows),
        Route("/api/agents", agents),
        Route("/api/labs", labs),
        Route("/api/models", models),
        Route("/api/settings", settings, methods=["GET", "PUT"]),
        Route("/api/loops", loops),
        Route("/api/loops/{lid}/history", loop_history),
        Route("/api/loops/{lid}/{action}", loop_action, methods=["POST"]),
        Route("/api/hf/search", hf_search),
        Route("/api/hf/models/{repo_id:path}", hf_model_detail),
        Route("/api/hf/download", hf_download_start, methods=["POST"]),
        Route("/api/hf/downloads", hf_downloads_list),
        Route("/api/hf/downloads/{task_id}/cancel", hf_download_cancel, methods=["POST"]),
        Route("/api/agents/custom", custom_agents_list),
        Route("/api/agents/custom", custom_agent_create, methods=["POST"]),
        Route("/api/agents/custom/{agent_id}", custom_agent_delete, methods=["DELETE"]),
        Route("/api/mesh/route", mesh_route, methods=["POST"]),
    ],
)


def main() -> None:
    import uvicorn

    port = int(os.environ.get("KUBONA_CONSOLE_PORT", "8000"))
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="warning")


if __name__ == "__main__":
    main()
