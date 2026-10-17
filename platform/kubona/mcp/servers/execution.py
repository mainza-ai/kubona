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
from kubona.cost.governance import CostGovernanceService, GovernanceError
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

# Local-only providers (W0.16): the ONLY names allowed to skip the
# jurisdiction/export gate via jurisdiction "none". A new provider claiming
# "none" without being listed here refuses (fail-closed default) — the old
# rule trusted the string, so any misconfigured provider bypassed export
# control. When adding a simulator provider, list it here AND in the
# modality set above (two lists, one documented update).
_LOCAL_ONLY_PROVIDERS = frozenset({"local-simulator", "qiskit-aer-simulator"})

# Implicit sessions are capped here (W0.16): a caller-supplied ceiling on an
# implicitly-created session is self-granted budget (no owner binds it).
# Above the cap, open a session via cost/create_session (cost:manage-gated)
# and submit with its id. v1 heuristic, labeled.
IMPLICIT_SESSION_CEILING_USD = 50.0


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
    if not isinstance(shots, int) or isinstance(shots, bool) or shots < 1:
        return {"ok": False, "error": f"shots must be an integer >= 1, got {shots!r}"}
    # P5 §2.9: routing declarations are mandatory for anything beyond local
    # simulators. Undeclared callers must not reach for hardware through this
    # path either — the executor gate is the second rail. W0.16: "none" alone
    # no longer suffices — the provider name must be allowlisted as local.
    if capability.jurisdiction == "none" and provider not in _LOCAL_ONLY_PROVIDERS:
        return {"ok": False,
                "error": f"provider {provider!r} claims local-only jurisdiction but is not "
                         "allowlisted: refusing (declare a real jurisdiction or list it)"}
    if capability.jurisdiction != "none" and not caller_jurisdictions:
        return {"ok": False,
                "error": f"provider {provider!r} is not local-only: caller_jurisdictions "
                         "must declare at least one jurisdiction (routing cannot be "
                         "verified undeclared)"}
    if caller_jurisdictions:
        from kubona.routing import ROUTE_INELIGIBLE, route

        r = route(capability, caller_jurisdictions=caller_jurisdictions,
                  caller_export_class=caller_export_class, require_review=require_review)
        if r.verdict == ROUTE_INELIGIBLE:
            return {"ok": False, "error": f"routing blocked: {r.reason}",
                    "routing": r.to_dict()}
    if session_id is None:
        # W0.16: implicit sessions carry a capped ceiling. A caller-supplied
        # ceiling here is self-granted budget (no owner binds it) — above
        # the cap, open a session via cost/create_session and submit with
        # its id instead of silently minting a $1M lab.
        if budget_ceiling_usd is not None and budget_ceiling_usd > IMPLICIT_SESSION_CEILING_USD:
            return {"ok": False,
                    "error": f"implicit session ceiling is capped at "
                             f"${IMPLICIT_SESSION_CEILING_USD}: open a session via "
                             "cost/create_session for a larger budget"}
        session = _sessions.create(name=circuit.name, owner="agent",
                                   budget_ceiling_usd=budget_ceiling_usd
                                   if budget_ceiling_usd is not None
                                   else IMPLICIT_SESSION_CEILING_USD)
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

    # W0.16 wait discipline: wait=True resolves synchronously ONLY for the
    # fast simulator path. Multi-hour QPU jobs with wait=True used to either
    # block 30s and return a misread row or record no spend — now async-only,
    # stated in the response. wait_timeout_s must be positive finite.
    import math
    modality = getattr(capability.modality, "value", str(capability.modality))
    is_simulator = modality in _SIMULATOR_MODALITIES or provider in _LOCAL_ONLY_PROVIDERS
    if not isinstance(wait_timeout_s, (int, float)) or not math.isfinite(wait_timeout_s) \
            or wait_timeout_s <= 0:
        return {"ok": False, "error": f"wait_timeout_s must be positive finite, got {wait_timeout_s!r}"}
    wait_note = None
    if wait and not is_simulator:
        wait = False
        wait_note = (f"provider {provider!r} is not simulator-class: forced async "
                     "(wait=False); poll get_job_status, spend accounted at dispatch")

    # Spend accounts at dispatch (W0.16): the old order (record after wait)
    # left multi-hour QPU spend unrecorded on every timeout while hardware
    # ran. Failed jobs keep the accounted spend — there is no audited refund
    # primitive yet (residual, stated here, not hidden).
    cost = pf["estimate"]["estimated_usd"]
    if cost > 0:
        try:
            _gov.record_spend(session_id, cost)
        except GovernanceError as e:
            return {"ok": False, "error": f"spend refused at dispatch: {e}",
                    "session_id": session_id}

    # enqueue the job on the persistent farm; wait=True resolves synchronously for
    # the fast local-simulator/agent path, wait=False returns immediately for real
    # long-running (multi-hour QPU) jobs whose status is read back from the store.
    job = _runner.store.create(provider=provider, session_id=session_id,
                               circuit_hash=circuit_hash, circuit_qasm=circuit_qasm,
                               shots=shots, seed=seed, estimate=pf["estimate"])
    job_id = job.job_id
    try:
        _runner.enqueue(job_id, _run)
    except (KeyError, RuntimeError) as e:
        return {"ok": False, "error": f"farm refused the job: {e}", "session_id": session_id}
    if wait:
        try:
            done = _runner.wait(job_id, timeout_s=wait_timeout_s)
        except TimeoutError as e:
            # W0.11: the job is still live, not done — say so explicitly
            # instead of returning a non-terminal row the caller misreads.
            live = _runner.store.get(job_id)
            return {"ok": True, "job_id": job_id, "session_id": session_id,
                    "status": live.status if live else "unknown",
                    "note": str(e), "circuit_hash": circuit_hash,
                    "estimate": pf["estimate"]}
    else:
        done = job

    _sessions.record_job(session_id, job_id)

    out = {"ok": True, "job_id": job_id, "session_id": session_id,
           "status": done.status, "circuit_hash": circuit_hash,
           "estimate": pf["estimate"]}
    if wait_note is not None:
        out["note"] = wait_note
    return out


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
    import math
    try:
        parsed = [int(s) for s in scales.split(",")]
    except (ValueError, AttributeError):
        return {"ok": False, "error": f"scales must be comma-separated positive ints, got {scales!r}"}
    if not parsed or any(s <= 0 for s in parsed) or len(parsed) > 9 or max(parsed) > 101:
        return {"ok": False, "error": f"scales out of bounds (1-9 factors, each 1-101): refusing (unbounded folding is a DoS)"}
    if not isinstance(shots, int) or isinstance(shots, bool) or shots < 1 or shots > 100000:
        return {"ok": False, "error": f"shots must be an integer 1-100000, got {shots!r}"}
    if not isinstance(noise_rate, (int, float)) or not math.isfinite(noise_rate) \
            or not 0 < noise_rate < 1:
        return {"ok": False, "error": f"noise_rate must be in (0, 1), got {noise_rate!r}"}
    circuit = parse_qasm2(circuit_qasm)
    result = run_zne(circuit, shots=shots, noise_rate=noise_rate,
                     scales=tuple(parsed), seed=seed)
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
