"""Execution MCP server (Phase 0 stub).

Tools: submit_job / get_job_status / get_result. Routes to the provider trait
(currently the local simulator). Job state is in-memory for the skeleton; the
provenance server persists the record.
"""

from __future__ import annotations

import asyncio
import uuid

from mcp.server.mcpserver import MCPServer

from kubona.circuits import canonical_hash, parse_qasm2
from kubona.cost import preflight
from kubona.providers import get_provider, list_providers
from kubona.schema import load_provider

mcp = MCPServer("kubona-execution")

_JOBS: dict[str, dict] = {}


def submit_job(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
               budget_ceiling_usd: float | None = None, seed: int | None = 7) -> dict:
    """Submit a circuit to a provider through the cost-governance pre-flight."""
    circuit = parse_qasm2(circuit_qasm)
    capability = load_provider(provider)
    pf = preflight(capability, circuit, shots, budget_ceiling_usd)
    if not pf["preflight_ok"]:
        return {"ok": False, "error": pf["reason"], "estimate": pf["estimate"]}

    exec_plugin = get_provider(provider)  # raises ProviderError if data-only
    job_id = uuid.uuid4().hex[:16]
    _JOBS[job_id] = {
        "job_id": job_id,
        "provider": provider,
        "circuit_hash": canonical_hash(circuit),
        "circuit_qasm": circuit_qasm,
        "shots": shots,
        "seed": seed,
        "status": "queued",
        "estimate": pf["estimate"],
        "preflight": pf,
    }
    try:
        result = exec_plugin.submit(circuit, shots, seed=seed)
        _JOBS[job_id]["status"] = "completed"
        _JOBS[job_id]["result"] = result
    except Exception as e:
        _JOBS[job_id]["status"] = "failed"
        _JOBS[job_id]["error"] = str(e)
    return {"ok": True, "job_id": job_id, "status": _JOBS[job_id]["status"],
            "circuit_hash": _JOBS[job_id]["circuit_hash"], "estimate": pf["estimate"]}


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


def list_provider_registry() -> dict:
    reg = {}
    for name, cap in list_providers().items():
        reg[name] = {"modality": cap.modality.value, "status": cap.status, "pricing": cap.pricing.model}
    return {"providers": reg}


for fn, desc in [
    (submit_job, "Submit a circuit to a provider through the cost-governance pre-flight."),
    (get_job_status, "Get the status of a submitted job."),
    (get_result, "Get the completed result of a job."),
    (list_provider_registry, "List registered providers and their capability summaries."),
]:
    mcp.add_tool(fn, name=fn.__name__, description=desc)

if __name__ == "__main__":
    asyncio.run(mcp.run_stdio_async())
