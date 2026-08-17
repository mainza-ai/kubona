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
from kubona.jobs import JobRunner
from kubona.mitigation import run_zne
from kubona.providers import get_provider, list_providers
from kubona.schema import load_provider
from kubona.session import SessionStore

mcp = MCPServer("kubona-execution")

_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)
_runner = JobRunner()  # persistent, async job farm (production §4.3 abstraction)


def submit_job(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
               session_id: str | None = None, budget_ceiling_usd: float | None = None,
               seed: int | None = 7,
               caller_jurisdictions: list[str] | None = None,
               caller_export_class: str = "any",
               require_review: bool = False,
               wait: bool = True, wait_timeout_s: float = 30.0) -> dict:
    """Submit a circuit through the cost-governance gate, threaded to a session.

    Production job/session abstraction: jobs are persisted to the async job farm;
    `wait=True` resolves synchronously (fast local path), `wait=False` returns the
    job_id immediately for long-running (real-QPU) jobs, read back via
    get_job_status / get_result, surviving restart.

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
    circuit_hash = canonical_hash(circuit)

    def _run() -> dict[str, Any]:
        # returns the provider result; JobRunner records transitions + provenance
        return exec_plugin.submit(circuit, shots, seed=seed)

    # enqueue the job on the persistent farm; wait=True resolves synchronously for
    # the fast local-simulator/agent path, wait=False returns immediately for real
    # long-running (multi-hour QPU) jobs whose status is read back from the store.
    job = _runner.store.create(provider=provider, session_id=session_id,
                               circuit_hash=circuit_hash, circuit_qasm=circuit_qasm,
                               shots=shots, seed=seed, estimate=pf["estimate"])
    job_id = job.job_id
    _runner.enqueue(job_id, _run)
    if wait:
        done = _runner.wait(job_id, timeout_s=wait_timeout_s)
    else:
        done = job

    _sessions.record_job(session_id, job_id)
    cost = pf["estimate"]["estimated_usd"]
    if cost > 0:
        _gov.record_spend(session_id, cost)

    return {"ok": True, "job_id": job_id, "session_id": session_id,
            "status": done.status, "circuit_hash": circuit_hash,
            "estimate": pf["estimate"]}


def get_job_status(job_id: str) -> dict:
    """Status read comes from the PERSISTED store (survives restart)."""
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    return {"ok": True, "job_id": job_id, "status": job.status, "provider": job.provider,
            "session_id": job.session_id, "created_at": job.created_at}


def get_result(job_id: str) -> dict:
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    if job.status == "failed":
        return {"ok": False, "job_id": job_id, "status": job.status, "error": job.error}
    if job.status == "cancelled":
        return {"ok": False, "job_id": job_id, "status": job.status, "error": "job cancelled"}
    if job.status != "completed":
        return {"ok": False, "job_id": job_id, "status": job.status, "error": "not completed"}
    return {"ok": True, "job_id": job_id, "provider": job.provider, "session_id": job.session_id,
            "result": job.result}


def cancel_job(job_id: str) -> dict:
    """Cancel a queued/running job (the P2 watch-and-interrupt primitive)."""
    if _runner.cancel(job_id):
        return {"ok": True, "job_id": job_id, "status": "cancelled"}
    return {"ok": False, "job_id": job_id, "error": "job not found or already terminal"}


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
    (submit_job, "Submit a circuit through the cost-governance gate, threaded to a session (async-persistent job farm)."),
    (get_job_status, "Get the persisted status of a submitted job (survives restart)."),
    (get_result, "Get the completed result of a job."),
    (cancel_job, "Cancel a queued or running job (watch-and-interrupt primitive)."),
    (run_zne_job, "Run ZNE mitigation (unitary folding + Richardson) on a circuit."),
    (session_status, "Session state: spend, ceiling, jobs, burn telemetry inputs."),
    (close_session, "Close a session."),
    (list_provider_registry, "List registered providers and their capability summaries."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
