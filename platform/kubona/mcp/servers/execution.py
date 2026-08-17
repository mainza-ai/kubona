"""Execution MCP server — Phase 1: session-aware + mitigation.

Tools: submit_job (threads the `job_id` session abstraction), get_job_status,
get_result, run_zne_job (first mitigation), session_status, close_session.
"""

from __future__ import annotations

import asyncio
import uuid

from mcp.server.mcpserver import MCPServer

from kubona.circuits import canonical_hash, parse_qasm2
from kubona.cost.governance import CostGovernanceService
from kubona.mitigation import run_zne
from kubona.providers import get_provider, list_providers
from kubona.schema import load_provider
from kubona.session import SessionStore

mcp = MCPServer("kubona-execution")

_JOBS: dict[str, dict] = {}
_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)


def submit_job(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
               session_id: str | None = None, budget_ceiling_usd: float | None = None,
               seed: int | None = 7,
               caller_jurisdictions: list[str] | None = None,
               caller_export_class: str = "any",
               require_review: bool = False) -> dict:
    """Submit a circuit through the cost-governance gate, threaded to a session.

    Routing enforcement (Phase 5 / §2.9): when the caller declares its
    jurisdictions/export-control class, the provider must be ELIGIBLE or the job
    is blocked BEFORE any execution — the compliance gate, not an afterthought.
    """
    circuit = parse_qasm2(circuit_qasm)
    capability = load_provider(provider)
    if caller_jurisdictions:
        from kubona.routing import ROUTE_INELIGIBLE, route

        r = route(capability, caller_jurisdictions=caller_jurisdictions,
                  caller_export_class=caller_export_class, require_review=require_review)
        if r.verdict == ROUTE_INELIGIBLE:
            return {"ok": False, "error": f"routing blocked: {r.reason}",
                    "routing": r.to_dict()}
    if session_id is None:
        session = _sessions.create(name=circuit.name, owner="agent",
                                   budget_ceiling_usd=budget_ceiling_usd)
        session_id = session.session_id
    else:
        try:
            session = _sessions.get(session_id)
        except KeyError:
            return {"ok": False, "error": f"unknown session_id {session_id!r}"}

    pf = _gov.preflight(capability, circuit, shots, session)
    if not pf["ok"]:
        return {"ok": False, "error": pf["reason"], "estimate": pf["estimate"],
                "session_id": session_id}

    exec_plugin = get_provider(provider)  # ProviderError if data-only
    job_id = uuid.uuid4().hex[:16]
    _JOBS[job_id] = {"job_id": job_id, "provider": provider,
                     "circuit_hash": canonical_hash(circuit), "circuit_qasm": circuit_qasm,
                     "shots": shots, "seed": seed, "status": "queued",
                     "estimate": pf["estimate"], "session_id": session_id}
    try:
        result = exec_plugin.submit(circuit, shots, seed=seed)
        _JOBS[job_id]["status"] = "completed"
        _JOBS[job_id]["result"] = result
    except Exception as e:  # noqa: BLE001 — surfaced as job failure, not a crash
        _JOBS[job_id]["status"] = "failed"
        _JOBS[job_id]["error"] = str(e)

    _sessions.record_job(session_id, job_id)
    cost = pf["estimate"]["estimated_usd"]
    if cost > 0:
        _gov.record_spend(session_id, cost)

    return {"ok": True, "job_id": job_id, "session_id": session_id,
            "status": _JOBS[job_id]["status"], "circuit_hash": _JOBS[job_id]["circuit_hash"],
            "estimate": pf["estimate"]}


def get_job_status(job_id: str) -> dict:
    job = _JOBS.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    return {"ok": True, "job_id": job_id, "status": job["status"]}


def get_result(job_id: str) -> dict:
    job = _JOBS.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    if job["status"] != "completed":
        return {"ok": False, "job_id": job_id, "status": job["status"], "error": "not completed"}
    return {"ok": True, "job_id": job_id, "provider": job["provider"], "result": job["result"]}


def run_zne_job(circuit_qasm: str, shots: int = 8000, noise_rate: float = 0.02,
                scales: str = "1,3,5", seed: int = 7) -> dict:
    """First mitigation: run ZNE (unitary folding + Richardson) on a circuit.

    Returns raw vs. mitigated expectation of Z on qubit 0 — the eval harness's
    reference for the mitigation-strategy search (Phase 4).
    """
    circuit = parse_qasm2(circuit_qasm)
    result = run_zne(circuit, shots=shots, noise_rate=noise_rate,
                     scales=tuple(int(s) for s in scales.split(",")), seed=seed)
    return {"ok": True, **result.to_dict()}


def session_status(session_id: str) -> dict:
    try:
        return {"ok": True, **{k: v for k, v in _sessions.get(session_id).__dict__.items()
                               if k != "meta"}}
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}


def close_session(session_id: str) -> dict:
    try:
        s = _sessions.close(session_id)
        return {"ok": True, "session_id": s.session_id, "status": s.status}
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}


def list_provider_registry() -> dict:
    reg = {}
    for name, cap in list_providers().items():
        reg[name] = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model}
    return {"providers": reg}


for fn, desc in [
    (submit_job, "Submit a circuit through the cost-governance gate, threaded to a session."),
    (get_job_status, "Get the status of a submitted job."),
    (get_result, "Get the completed result of a job."),
    (run_zne_job, "Run ZNE mitigation (unitary folding + Richardson) on a circuit."),
    (session_status, "Session state: spend, ceiling, jobs, burn telemetry inputs."),
    (close_session, "Close a session."),
    (list_provider_registry, "List registered providers and their capability summaries."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
