"""Execution MCP server — Phase 1: session-aware + mitigation.

Tools: submit_job (threads the `job_id` session abstraction), get_job_status,
get_result, run_zne_job (first mitigation), session_status, close_session.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import uuid

from mcp.server.mcpserver import MCPServer

from kubona.circuits import canonical_hash, parse_qasm2
from kubona.cost.governance import CostGovernanceService
from kubona.fidelity.accounting import FidelityAccountingEngine
from kubona.jobs import JobRunner
from kubona.mitigation import run_zne
from kubona.providers import get_provider, list_providers
from kubona.schema import load_provider
from kubona.session import SessionStore

mcp = MCPServer("kubona-execution")

_sessions = SessionStore()
_gov = CostGovernanceService(_sessions)
_runner = JobRunner()  # persistent, async job farm (production §4.3 abstraction)
_fidelity = FidelityAccountingEngine()

# Provider -> fidelity device profile. Simulators skip the gate (nothing to
# predict); every other backend must be catalogued or the job refuses as
# unpredictable — unknown devices never inherit the perfect simulator profile.
_FIDELITY_DEVICE_FOR_PROVIDER = {
    "ibm-qiskit": "ibm-heron",
    "rigetti": "rigetti-aspen",
}
_SIMULATOR_MODALITIES = {"statevector_simulator", "gpu_simulator"}
_TWO_QUBIT_GATES = frozenset({"cx", "cz", "swap", "cp", "ecr", "rzz"})


def _fidelity_gate(capability, circuit) -> dict:
    """Predict-then-admit fidelity check. Returns a verdict dict with
    admitted: bool. Refusals carry the reason + receipt hash (reusable
    negative result), never a silent downgrade."""
    modality = getattr(capability.modality, "value", str(capability.modality))
    if modality in _SIMULATOR_MODALITIES:
        return {"admitted": True, "skipped": "simulator-class backend",
                "provider": capability.provider}
    device_name = _FIDELITY_DEVICE_FOR_PROVIDER.get(capability.provider)
    if device_name is None:
        verdict = {"admitted": False,
                   "rejection_reason": (
                       f"no fidelity profile for provider {capability.provider!r}: "
                       "unpredictable on this device — refused (no silent simulation)"),
                   "provider": capability.provider}
    else:
        gates = list(getattr(circuit, "gates", []) or [])
        n_2q = sum(1 for g in gates if getattr(g, "name", "") in _TWO_QUBIT_GATES)
        n_1q = len(gates) - n_2q
        v = _fidelity.evaluate_admission(
            device_name,
            n_qubits=getattr(circuit, "num_qubits", 0),
            n_single_qubit_gates=n_1q,
            n_two_qubit_gates=n_2q,
        )
        verdict = {"admitted": v.admitted and v.recommended_action == "EXECUTE_DIRECT",
                   "predicted_fidelity": v.predicted_fidelity,
                   "fidelity_floor": v.fidelity_floor,
                   "calibration_fresh": v.calibration_fresh,
                   "recommended_action": v.recommended_action,
                   "rejection_reason": v.rejection_reason,
                   "provider": capability.provider,
                   "device": device_name}
        if verdict["admitted"] is False and v.rejection_reason is None:
            verdict["rejection_reason"] = (
                f"fidelity gate recommends {v.recommended_action}: resubmit explicitly "
                "with mitigation — never applied silently")
    verdict["fidelity_receipt"] = hashlib.sha256(
        json.dumps(verdict, sort_keys=True, default=str).encode()).hexdigest()[:16]
    return verdict


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

    # Fidelity gate (Ring 3, second rail beside money): predict whether this
    # run can produce a publishable number BEFORE spending. Simulator-class
    # backends skip (no noise to predict); QPU backends need a catalogued
    # device profile, else the device is unpredictable and the job refuses.
    # Non-EXECUTE_DIRECT verdicts also refuse: remediation (ZNE/downgrade)
    # changes what was approved and must be resubmitted explicitly, never
    # applied silently. The verdict doubles as the refusal receipt.
    fidelity = _fidelity_gate(capability, circuit)
    if not fidelity["admitted"]:
        return {"ok": False, "error": fidelity["rejection_reason"],
                "fidelity": fidelity, "session_id": session_id}

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
