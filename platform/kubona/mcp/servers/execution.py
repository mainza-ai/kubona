"""Execution MCP server — Phase 1: session-aware + mitigation.

Tools: submit_job (threads the `job_id` session abstraction), get_job_status,
get_result, run_zne_job (first mitigation), session_status, close_session.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import math
import uuid
from typing import Any

from mcp.server.mcpserver import MCPServer

from kubona.circuits import canonical_hash, parse_qasm2
from kubona.cost.governance import GovernanceError
from kubona.fidelity.accounting import FidelityAccountingEngine, PROVIDER_DEVICE_MAP
from kubona.jobs import JobRunner
from kubona.mcp.approval import (
    build_packet,
    current_gateway_approval,
)
from kubona.mitigation import run_zne
from kubona.providers import get_provider, list_providers
from kubona.providers.base import ProviderError
from kubona.schema import load_provider
from kubona.schema.provider import MODALITIES_SIMULATOR, TWO_Q_GATES
from kubona.mcp.servers._stores import GOVERNANCE as _gov, SESSIONS as _sessions

mcp = MCPServer("kubona-execution")

_runner = JobRunner()  # persistent, async job farm (production §4.3 abstraction)
_fidelity = FidelityAccountingEngine()

# Provider -> fidelity device profile loaded from platform/contracts/fidelity-devices.json.
# Simulators skip the gate (nothing to predict); every other backend must be
# catalogued or the job refuses as unpredictable — unknown devices never
# inherit the perfect simulator profile.
_FIDELITY_DEVICE_FOR_PROVIDER: dict[str, str] = dict(PROVIDER_DEVICE_MAP)
# W2.21: single shared consts from kubona.schema.provider (aliases keep the
# neighboring _LOCAL_ONLY_PROVIDERS documentation coherent — when adding a
# simulator provider, list it there AND check the modality set here).
_SIMULATOR_MODALITIES = MODALITIES_SIMULATOR
_TWO_QUBIT_GATES = TWO_Q_GATES

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

# Cancel spend policy (Phase-0, single shared const): the receipt states it
# verbatim so pre/post-vendor treatment is never ambiguous downstream.
CANCEL_SPEND_POLICY = (
    "spend policy: pre-vendor cancel refunds the full reservation "
    "(the vendor never ran); post-vendor cancel settles to reported usage "
    "when the vendor metered any (measured usage wins, difference refunded), "
    "otherwise keeps the charge "
    "(a pinned hardware id means the vendor may have run)"
)

# Tools whose gateway MRTR approval the in-function paid gate honors
# (ambient context, scope re-checked at dispatch).
_APPROVED_SUBMIT_TOOLS = frozenset({"submit_job", "submit_qubo_job"})


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


def _is_spend_prepaid(session) -> bool:
    """Go-authoritative mirrors carry `spend_prepaid` in session meta: Go
    already reserved the job against the authoritative book, so Python must
    not charge, refund, settle, or demand approval for its mirror."""
    return bool((getattr(session, "meta", {}) or {}).get("spend_prepaid"))


def _session_owned_by(session, caller: dict | None) -> bool:
    """Ownership check (Phase-1): the caller's session/tenant owns the id.

    Derived from the SESSION RECORD server-side (owner + tenant binding),
    never from request strings. Owner must equal the caller sub; a recorded
    session tenant (`meta.tenant`, bound at gateway creation) must match the
    caller tenant when present. Legacy rows without a recorded tenant fall
    back to owner-sub matching. Go-authoritative mirrors carry their tenant
    under `meta.tenant_id` on the Go rail — the Python rail does not
    reinterpret it (documented, not conflated).
    """
    if caller is None:
        return True  # direct in-process / stdio callers are transport-trusted
    if getattr(session, "owner", None) != caller.get("sub"):
        return False
    recorded = (getattr(session, "meta", None) or {}).get("tenant")
    if recorded is not None and recorded != caller.get("tenant"):
        return False
    return True


def _job_owner_session(job):
    """Session record backing a job (server-side), or None when gone."""
    try:
        return _sessions.get(job.session_id)
    except KeyError:
        return None


def _reserved_for_job(job) -> float:
    """Python-recorded reservation for a job (dispatch estimate). 0.0 means
    no Python reservation exists (free tier or Go-prepaid mirror)."""
    meta = (getattr(job, "meta", None) or {})
    reserved = meta.get("reserved_usd")
    if reserved is None:
        est = meta.get("estimate")
        reserved = est.get("estimated_usd", 0.0) if isinstance(est, dict) else 0.0
    try:
        return round(float(reserved), 6)
    except (TypeError, ValueError):
        return 0.0


def _extract_actual_usd(result) -> float | None:
    """Reported usage from a completed result. Providers carry it top-level
    (synchronous wait path) or nested under `result` (deferred poll path) —
    both shapes settle identically; anything else means usage unknown."""
    if not isinstance(result, dict):
        return None
    raw = result.get("actual_usd")
    if raw is None:
        inner = result.get("result")
        if isinstance(inner, dict):
            raw = inner.get("actual_usd")
    try:
        value = round(float(raw), 6)
    except (TypeError, ValueError):
        return None
    if not math.isfinite(value) or value < 0:
        return None
    return value


def _maybe_settle_job(job) -> dict | None:
    """Mirror the Go CAS-settle semantics on the Python rail: on EVERY terminal
    (completed/failed/cancelled) with reported usage below the reservation,
    release the difference back to the session. Measured usage always wins;
    unknown usage keeps the estimate loudly (job_settle_skipped); explicit
    zero usage refunds everything. Crash-safe (the store CAS marks first,
    so a crash under-refunds rather than double-releasing) and idempotent
    (an already-settled job is a no-op). Returns a receipt when it acted or
    when it observed a prior settle, else None (nothing reserved / Go-owned /
    pre-vendor failure the dispatch path already refunded).
    """
    if job is None or getattr(job, "status", None) not in ("completed", "failed", "cancelled"):
        return None
    actual = _extract_actual_usd(getattr(job, "result", None))
    if actual is None and getattr(job, "status", None) in ("failed", "cancelled"):
        hw = (getattr(job, "meta", None) or {}).get("hardware_job_id")
        if not hw:
            # Pre-vendor terminal (no hardware id — the vendor never ran):
            # the dispatch path already refunded the reservation, so there
            # is nothing to reconcile here. Settling would misrecord a kept
            # estimate against already-returned money.
            return None
    try:
        _, did_mark, reserved = _runner.store.cas_mark_settled(
            job.job_id, actual)
    except Exception:  # noqa: BLE001 — store failure must never break a status read
        return None
    if not did_mark:
        return {"settled": True, "already_settled": True, "refunded_usd": 0.0,
                "note": "settle replay: already settled, no money moved"}
    if reserved <= 0:
        return {"settled": True, "reserved_usd": 0.0, "actual_usd": actual,
                "refunded_usd": 0.0, "note": "no Python reservation to settle"}
    try:
        session = _sessions.get(job.session_id)
    except KeyError:
        return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
                "refunded_usd": 0.0, "note": "session gone: reservation kept, no refund"}
    if _is_spend_prepaid(session):
        # Dual-rail: Go settles its own rail from the status poll. Touching
        # the Python mirror would double-settle the same usage.
        return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
                "refunded_usd": 0.0, "note": "go-prepaid mirror: Go reconciles, Python holds no reservation"}
    if actual is None or actual >= reserved:
        _runner._record("job_settle_skipped", {
            "job_id": job.job_id, "session_id": job.session_id,
            "reserved_usd": reserved, "actual_usd": actual,
            "reason": "actual_usd_missing" if actual is None else "actual_usd_above_reserved",
        })
        return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
                "refunded_usd": 0.0, "note": "usage unknown or at/above estimate: estimate stands"}
    diff = round(reserved - actual, 6)
    if diff <= 0:
        return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
                "refunded_usd": 0.0}
    try:
        _gov.refund_spend(job.session_id, diff,
                          f"settle {job.job_id}: reserved ${reserved} -> actual ${actual}")
        _runner._record("job_settled", {
            "job_id": job.job_id, "session_id": job.session_id,
            "reserved_usd": reserved, "actual_usd": actual, "refunded_usd": diff,
        })
    except GovernanceError as e:
        # Marker is already set (no retry will double-release); the lab keeps
        # the estimate and the failure is stated, never silent.
        return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
                "refunded_usd": 0.0, "refund_failed": str(e)}
    return {"settled": True, "reserved_usd": reserved, "actual_usd": actual,
            "refunded_usd": diff}


def _paid_dispatch_authorized(*, tool: str, session, session_id: str,
                              cost_usd: float, approval_id: str | None) -> dict | None:
    """Fail-closed paid-dispatch gate (Phase-0). Returns None when the spend
    is authorized; otherwise the refusal envelope (ok False + error).

    Authorized when: the dispatch is free (zero-cost simulators proceed
    without approval); the session is a Go-prepaid mirror (Go verified the
    mandate on its rail); the enclosing gateway call just survived a full
    MRTR round for this tool covering the cost; or `approval_id` names a
    governance APPROVED approval bound to this session, covering the cost,
    with a minted mandate. Anything else refuses — approval-free spend to
    the session ceiling is gone."""
    try:
        cost = round(float(cost_usd), 6)
    except (TypeError, ValueError):
        return {"ok": False, "session_id": session_id,
                "error": f"unpriced dispatch (cost {cost_usd!r}): refusing unbound spend"}
    if not math.isfinite(cost):
        return {"ok": False, "session_id": session_id,
                "error": f"unpriced dispatch (non-finite cost {cost_usd!r}): refusing unbound spend"}
    if cost <= 0:
        return None
    if _is_spend_prepaid(session):
        return None
    ambient = current_gateway_approval()
    if isinstance(ambient, dict) and ambient.get("server") == "execution" \
            and ambient.get("tool") == tool and tool in _APPROVED_SUBMIT_TOOLS:
        try:
            covered = float(ambient.get("cost_max_usd", 0.0))
        except (TypeError, ValueError):
            covered = 0.0
        if covered >= cost:
            return None
        return {"ok": False, "session_id": session_id,
                "error": f"gateway approval covers ${covered} < dispatch cost ${cost}: refused"}
    if not approval_id or not str(approval_id).strip():
        return {"ok": False, "session_id": session_id,
                "error": "paid dispatch requires an approval: pass approval_id for a governance "
                         "APPROVED approval (cost/request_approval -> hold -> approve) covering "
                         f"${cost}, or dispatch through the approval-gated gateway: refusing "
                         "approval-free spend"}
    try:
        approvals = _gov._load_approvals()  # same-process store the gateway/tests share
    except Exception as e:  # noqa: BLE001 — unreadable approvals fail closed
        return {"ok": False, "session_id": session_id,
                "error": f"approval store unreadable: refusing paid dispatch ({e})"}
    approval = approvals.get(str(approval_id))
    if approval is None:
        return {"ok": False, "session_id": session_id,
                "error": f"unknown approval_id {str(approval_id)!r}: refusing paid dispatch"}
    if approval.state != "APPROVED":
        return {"ok": False, "session_id": session_id,
                "error": f"approval {approval.approval_id} is {approval.state}, not APPROVED "
                         "(request -> hold -> approve): refusing paid dispatch"}
    if approval.session_id != session_id:
        return {"ok": False, "session_id": session_id,
                "error": f"approval {approval.approval_id} binds session {approval.session_id!r}, "
                         "not this dispatch: refusing cross-session spend"}
    try:
        covered_amount = float(approval.amount_requested_usd)
    except (TypeError, ValueError):
        covered_amount = 0.0
    if covered_amount < cost:
        return {"ok": False, "session_id": session_id,
                "error": f"approval {approval.approval_id} covers ${covered_amount} < dispatch "
                         f"cost ${cost}: refusing under-approved spend"}
    if not approval.mandate_token or not approval.mandate_digest:
        return {"ok": False, "session_id": session_id,
                "error": f"approval {approval.approval_id} carries no minted mandate: refusing"}
    return None


def _submit_packet_builder(*, tool: str, principal, args: dict) -> dict | None:
    """Shared core for the gateway approval packet builders: returns None to
    auto-allow (free tier, dry run, a valid governance approval already
    presented, or anything the tool itself will refuse before spend), else a
    real ApprovalPacket (the gateway seals it with the shared
    `APPROVAL_SECRET_ENV` secret from `kubona.mcp.approval`).

    Ordering matters: this builder runs BEFORE the tool's routing, session,
    and fidelity gates. On any estimation/validation uncertainty it defers
    (None) so the tool's precise refusal — routing-blocked, unknown session,
    unpriced provider — is never masked by an approval ask or a builder
    raise. Enforcement lives in the tool (`_paid_dispatch_authorized`,
    checked after routing/fidelity and before money moves); the packet is
    the early human ask for dispatches that provably spend, never the lock.
    Only an unknown provider raises (the tool raises KeyError there too —
    fail-closed on both rails)."""
    a = dict(args or {})
    if a.get("estimate_only") is True:
        return None  # dry run moves no money (Go parity: mandate only for real dispatch)
    provider = a.get("provider", "local-simulator")
    try:
        capability = load_provider(provider)
    except KeyError as e:
        raise ValueError(f"unknown provider {provider!r}: refusing unbound spend") from e
    modality = getattr(capability.modality, "value", str(capability.modality))
    try:
        if tool == "submit_qubo_job" or modality == "annealing":
            n_samples = a.get("n_samples", a.get("shots", 64))
            if not isinstance(n_samples, int) or isinstance(n_samples, bool) or n_samples < 1:
                return None  # tool refuses shots precisely
            backend = a.get("backend", "local")
            if backend == "local":
                return None  # credential-free local solver: free tier
            if backend != "leap":
                return None  # tool refuses the backend precisely
            estimate = capability.cost_estimate(n_samples)
            program_hash = hashlib.sha256(
                json.dumps(a.get("qubo_matrix", ""), sort_keys=True, default=str).encode()
            ).hexdigest()[:16]
            qubits = 0
            shots = n_samples
        else:
            circuit = parse_qasm2(a.get("circuit_qasm", ""))
            shots = a.get("shots", 1024)
            if not isinstance(shots, int) or isinstance(shots, bool) or shots < 1:
                return None  # tool refuses shots precisely
            estimate = capability.cost_estimate(shots, circuit)
            program_hash = canonical_hash(circuit)
            qubits = getattr(circuit, "num_qubits", 0)
    except ValueError:
        # Unpriced/unparseable: the tool refuses precisely (preflight/
        # routing/fidelity envelope) — never mask it, never read as free.
        return None
    try:
        cost = round(float(estimate.get("estimated_usd", 0.0)), 6)
    except (TypeError, ValueError):
        return None  # unpriced: tool refuses precisely
    if cost <= 0:
        # Zero-cost simulators auto-allow; $0.00 non-simulators are refused
        # by the tool itself (reads as free tier downstream) — defer to it.
        return None
    session_id = a.get("session_id")
    remaining: float = IMPLICIT_SESSION_CEILING_USD
    if session_id:
        try:
            sess = _sessions.get(session_id)
        except KeyError:
            return None  # tool refuses unknown sessions precisely
        rem = sess.remaining()
        remaining = float(rem) if rem is not None else IMPLICIT_SESSION_CEILING_USD
        presented = a.get("approval_id")
        if presented and _paid_dispatch_authorized(
                tool=tool, session=sess, session_id=session_id,
                cost_usd=cost, approval_id=presented) is None:
            return None  # valid governance approval rides along: no MRTR theater
    sub = getattr(principal, "sub", None) or "anon"
    basis = estimate.get("cost_basis") or estimate.get("notes") or ""
    return build_packet(
        agent_id=sub, server="execution", tool=tool, backend=provider,
        program_hash=program_hash, cost_max_usd=cost,
        budget_remaining_usd=remaining, cost_basis=str(basis),
        qubits=int(qubits or 0), shots=int(shots or 0),
        evidence={"estimate": estimate, "session_id": session_id,
                  "implicit_session": session_id is None},
        args=dict(a),
    )


def submit_job_approval_builder(principal, args):
    """Gateway approval gate for execution/submit_job (Phase-0 wiring)."""
    return _submit_packet_builder(tool="submit_job", principal=principal, args=args)


def submit_qubo_job_approval_builder(principal, args):
    """Gateway approval gate for execution/submit_qubo_job (Phase-0 wiring)."""
    return _submit_packet_builder(tool="submit_qubo_job", principal=principal, args=args)


def submit_qubo_job(qubo_matrix: list[list[float]] | str, n_samples: int = 64,
                    session_id: str | None = None, budget_ceiling_usd: float | None = None,
                    seed: int | None = 7, backend: str = "local",
                    caller_jurisdictions: list[str] | None = None,
                    caller_export_class: str = "any",
                    require_review: bool = False,
                    estimate_only: bool = False,
                    idempotency_key: str | None = None,
                    idempotency_body_hash: str | None = None,
                    approval_id: str | None = None) -> dict:
    """Submit a QUBO matrix to the annealing provider (`dwave-annealing`).

    Supports credential-free local solving (`backend="local"`: exact enumeration
    for n<=16, dimod simulated annealing or labeled single-bit-flip heuristic
    for n>16) and D-Wave Leap hybrid cloud (`backend="leap"`).

    Money discipline mirrors `submit_job`: the idempotency key is claimed
    atomically BEFORE any spend (a retried dispatch replays, never
    double-charges), and paid Leap dispatches require an approval (governance
    APPROVED approval via `approval_id`, a gateway MRTR round, or a
    Go-prepaid mirror session) — free local solves proceed without one.
    """
    import numpy as np
    from kubona.providers.annealing_provider import AnnealingProvider

    provider = "dwave-annealing"
    capability = load_provider(provider)
    if not isinstance(n_samples, int) or isinstance(n_samples, bool) or n_samples < 1:
        return {"ok": False, "error": f"n_samples must be an integer >= 1, got {n_samples!r}"}
    if backend not in ("local", "leap"):
        return {"ok": False, "error": f"unknown annealing backend {backend!r}: use 'local' or 'leap'"}

    raw_q = qubo_matrix
    if isinstance(raw_q, str):
        try:
            raw_q = json.loads(raw_q)
        except Exception as e:  # noqa: BLE001
            return {"ok": False, "error": f"qubo_matrix must be valid JSON 2-D array: {e}"}
    if not isinstance(raw_q, (list, tuple)):
        return {"ok": False, "error": "qubo_matrix must be a 2-D square numeric array"}
    try:
        Q = np.asarray(raw_q, dtype=float)
        n_qubits = AnnealingProvider._validate_Q(Q)
    except (ProviderError, ValueError, TypeError) as e:
        return {"ok": False, "error": str(e)}

    if backend == "leap" and not caller_jurisdictions:
        return {"ok": False,
                "error": f"provider {provider!r} (backend='leap') is not local-only: "
                         "caller_jurisdictions must declare at least one jurisdiction"}
    if caller_jurisdictions:
        from kubona.routing import ROUTE_INELIGIBLE, route

        r = route(capability, caller_jurisdictions=caller_jurisdictions,
                  caller_export_class=caller_export_class, require_review=require_review)
        if r.verdict == ROUTE_INELIGIBLE:
            return {"ok": False, "error": f"routing blocked: {r.reason}",
                    "routing": r.to_dict()}

    if session_id is None:
        if budget_ceiling_usd is not None and budget_ceiling_usd > IMPLICIT_SESSION_CEILING_USD:
            return {"ok": False,
                    "error": f"implicit session ceiling is capped at "
                             f"${IMPLICIT_SESSION_CEILING_USD}: open a session via "
                             "cost/create_session for a larger budget"}
        session = _sessions.create(name=f"qubo-{n_qubits}q", owner="agent",
                                   budget_ceiling_usd=budget_ceiling_usd
                                   if budget_ceiling_usd is not None
                                   else IMPLICIT_SESSION_CEILING_USD)
        session_id = session.session_id
    else:
        try:
            session = _sessions.get(session_id)
        except KeyError:
            return {"ok": False, "error": f"unknown session_id {session_id!r}"}

    # Go-authoritative mirrors (spend_prepaid): Go already reserved the job —
    # Python must not charge, refund, settle, or demand approval for its copy.
    spend_prepaid = _is_spend_prepaid(session)

    if backend == "local":
        estimate = {
            "provider": provider,
            "pricing_model": "free",
            "currency": "USD",
            "estimated_usd": 0.0,
            "shots": n_samples,
            "n_qubits": n_qubits,
            "backend": "local",
            "notes": "Credential-free local QUBO solver (exact n<=16, SA/heuristic n>16)",
            "cost_basis": "local-qubo-solver (free tier)",
        }
        fidelity = {
            "admitted": True,
            "skipped": "annealing local-qubo-solver (non-gate QUBO modality)",
            "provider": provider,
            "backend": "local",
        }
        fidelity["fidelity_receipt"] = hashlib.sha256(
            json.dumps(fidelity, sort_keys=True, default=str).encode()
        ).hexdigest()[:16]
    else:
        try:
            estimate = capability.cost_estimate(n_samples)
        except ValueError as e:
            return {"ok": False, "error": f"cost estimation refused: {e}",
                    "estimate": None, "session_id": session_id}
        cost_val = estimate["estimated_usd"]
        if cost_val > 0 and session.budget_ceiling_usd is None:
            return {"ok": False, "error": "paid provider requires a session budget ceiling",
                    "estimate": estimate, "session_id": session_id}
        if cost_val > 0 and cost_val > session.remaining():
            return {"ok": False,
                    "error": f"estimate {cost_val} exceeds remaining budget {session.remaining()}",
                    "estimate": estimate, "session_id": session_id}
        fidelity = _fidelity_gate(capability, None)
        if not fidelity["admitted"]:
            return {"ok": False, "error": fidelity["rejection_reason"],
                    "fidelity": fidelity, "session_id": session_id}

    exec_plugin = get_provider(provider)
    qubo_canonical = json.dumps([[float(v) for v in row] for row in Q.tolist()], separators=(",", ":"))
    qubo_hash = hashlib.sha256(qubo_canonical.encode()).hexdigest()[:16]

    if estimate_only:
        return {"ok": True, "estimate_only": True, "session_id": session_id,
                "estimate": estimate, "fidelity": fidelity,
                "circuit_hash": qubo_hash}

    cost = estimate["estimated_usd"]

    # Paid Leap dispatches move money: fail closed without an approval.
    # (Free local solves skip — same rule as submit_job; estimate_only
    # already returned above, so this gates real spend only.)
    if not estimate_only:
        refusal = _paid_dispatch_authorized(
            tool="submit_qubo_job", session=session, session_id=session_id,
            cost_usd=cost, approval_id=approval_id)
        if refusal is not None:
            return refusal

    # Atomically claim the idempotency key BEFORE money moves (Phase-0 P0):
    # the old spend-before-create double-charged a retried dispatch. A
    # replay returns the original job with no new spend; a key reused for a
    # different body refuses — the same body-hash rule as submit_job.
    job, created = _runner.store.find_or_create(
        provider=provider, session_id=session_id,
        idempotency_key=idempotency_key or "",
        body_hash=idempotency_body_hash or "",
        circuit_hash=qubo_hash, circuit_qasm=qubo_canonical,
        shots=n_samples, seed=seed, estimate=estimate,
        reserved_usd=0.0 if spend_prepaid else cost,
    )
    job_id = job.job_id
    if not created:
        prior_body = (job.meta or {}).get("body_hash")
        if idempotency_body_hash and prior_body and prior_body != idempotency_body_hash:
            return {"ok": False, "job_id": job_id, "session_id": job.session_id,
                    "error": "idempotency_key reused with a different request body: refused"}
        return {"ok": True, "job_id": job_id, "session_id": job.session_id,
                "status": job.status, "idempotent_replay": True,
                "circuit_hash": (job.meta or {}).get("circuit_hash", qubo_hash)}

    if cost > 0 and not spend_prepaid:
        try:
            _gov.record_spend(session_id, cost)
        except GovernanceError as e:
            try:
                _runner.store.transition(job_id, "failed", error=f"spend refused: {e}")
            except Exception:  # noqa: BLE001 — best-effort ghost cleanup
                pass
            return {"ok": False, "error": f"spend refused at dispatch: {e}",
                    "session_id": session_id}

    def _run_qubo() -> dict:
        try:
            return exec_plugin.solve_qubo(Q, n_samples=n_samples, seed=seed, backend=backend)
        except Exception as e:
            # No hardware-id pin exists on this path (unlike submit_job's
            # _pin callback): a worker failure is treated as pre-vendor —
            # the reservation returns (Go parity for unpinned failures).
            # Once Leap surfaces a vendor job id, pin it here and keep the
            # charge instead (post-vendor-cancel policy).
            if cost > 0 and not spend_prepaid:
                try:
                    _gov.refund_spend(session_id, cost, f"qubo worker failure: {e}")
                    _runner._record("session_spend_refunded", {
                        "session_id": session_id, "job_id": job_id,
                        "amount_usd": cost, "reason": f"qubo worker failure: {e}",
                    })
                except GovernanceError:
                    pass
            raise

    try:
        _runner.enqueue(job_id, _run_qubo)
    except (KeyError, RuntimeError) as e:
        if cost > 0 and not spend_prepaid:
            try:
                _gov.refund_spend(session_id, cost, f"pre-vendor enqueue failure: {e}")
                _runner._record("session_spend_refunded", {
                    "session_id": session_id, "job_id": job_id,
                    "amount_usd": cost, "reason": f"pre-vendor enqueue failure: {e}",
                })
            except GovernanceError:
                pass
        try:
            _runner.store.transition(job_id, "failed", error=f"farm refused: {e}")
        except Exception:  # noqa: BLE001 — best-effort ghost cleanup
            pass
        return {"ok": False, "error": f"farm refused the job: {e}", "session_id": session_id}

    if backend == "local":
        try:
            done = _runner.wait(job_id, timeout_s=30.0)
        except TimeoutError as e:
            live = _runner.store.get(job_id)
            return {"ok": True, "job_id": job_id, "session_id": session_id,
                    "status": live.status if live else "unknown",
                    "note": str(e), "circuit_hash": qubo_hash,
                    "estimate": estimate}
    else:
        done = job

    _sessions.record_job(session_id, job_id)
    return {"ok": True, "job_id": job_id, "session_id": session_id,
            "status": done.status, "circuit_hash": qubo_hash,
            "estimate": estimate, "fidelity": fidelity}


def submit_job(circuit_qasm: str, provider: str = "local-simulator", shots: int = 1024,
               session_id: str | None = None, budget_ceiling_usd: float | None = None,
               seed: int | None = 7,
               caller_jurisdictions: list[str] | None = None,
               caller_export_class: str = "any",
               require_review: bool = False,
               wait: bool = True, wait_timeout_s: float = 30.0,
               estimate_only: bool = False,
               idempotency_key: str | None = None,
               idempotency_body_hash: str | None = None,
               approval_id: str | None = None,
               caller: dict | None = None) -> dict:
    """Submit a circuit through the cost-governance gate, threaded to a session.

    Production job/session abstraction: jobs are persisted to the async job farm;
    `wait=True` resolves synchronously (fast local path), `wait=False` returns the
    job_id immediately for long-running (real-QPU) jobs, read back via
    get_job_status / get_result, surviving restart.

    Routing enforcement (Phase 5 / §2.9): when the caller declares its
    jurisdictions/export-control class, the provider must be ELIGIBLE or the job
    is blocked BEFORE any execution — the compliance gate, not an afterthought.

    Money (Phase-0): free-tier simulator dispatches proceed without approval;
    ANY dispatch that can spend > 0 requires one — a governance APPROVED
    approval via `approval_id`, a gateway MRTR round, or a Go-prepaid mirror
    session — else the submit refuses approval-free spend.

    Ownership (Phase-1): the gateway-injected `caller` owns implicit
    sessions (owner=sub, tenant in meta); an explicit session_id the caller
    does not own refuses with the unknown-session envelope (no oracle).
    """
    capability = load_provider(provider)
    if getattr(capability.modality, "value", str(capability.modality)) == "annealing":
        stripped = (circuit_qasm or "").strip()
        if stripped.startswith("["):
            return submit_qubo_job(
                qubo_matrix=stripped,
                n_samples=shots,
                session_id=session_id,
                budget_ceiling_usd=budget_ceiling_usd,
                seed=seed,
                backend="local",
                caller_jurisdictions=caller_jurisdictions,
                caller_export_class=caller_export_class,
                require_review=require_review,
                estimate_only=estimate_only,
                idempotency_key=idempotency_key,
                idempotency_body_hash=idempotency_body_hash,
                approval_id=approval_id,
            )
        return {
            "ok": False,
            "error": (
                f"provider {provider!r} is an annealing (QUBO) modality: pass a "
                "JSON 2-D QUBO matrix (e.g. [[-1.0, 2.0], [0.0, -1.0]]) or call submit_qubo_job"
            ),
        }
    circuit = parse_qasm2(circuit_qasm)
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
        # Phase-1 ownership: the implicit session is owned by the verified
        # caller (owner=sub, tenant bound in meta). Transport-trusted direct
        # callers keep the legacy owner label.
        owner = caller["sub"] if caller is not None else "agent"
        session = _sessions.create(name=circuit.name, owner=owner,
                                   budget_ceiling_usd=budget_ceiling_usd
                                   if budget_ceiling_usd is not None
                                   else IMPLICIT_SESSION_CEILING_USD)
        if caller is not None:
            _sessions.update(session.session_id,
                             meta={**(session.meta or {}),
                                   "tenant": caller["tenant"],
                                   "created_by": caller["sub"]})
            session = _sessions.get(session.session_id)
        session_id = session.session_id
    else:
        try:
            session = _sessions.get(session_id)
        except KeyError:
            return {"ok": False, "error": f"unknown session_id {session_id!r}"}
        if not _session_owned_by(session, caller):
            # Indistinguishable from unknown (no oracle): the id is
            # caller-supplied either way, so echoing it leaks nothing new.
            return {"ok": False, "error": f"unknown session_id {session_id!r}"}

    # Go-authoritative sessions are mirrored with spend_prepaid=True: Go
    # already reserved the job, so Python must not charge/refund/settle its
    # mirror or demand approval for it.
    spend_prepaid = _is_spend_prepaid(session)

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

    # Phase-0 approval gate: anything that can spend > 0 refuses without an
    # approval (free simulators skip). Checked BEFORE the idempotency claim
    # so an approval-free retry learns nothing and books nothing.
    if not estimate_only:
        refusal = _paid_dispatch_authorized(
            tool="submit_job", session=session, session_id=session_id,
            cost_usd=pf["estimate"]["estimated_usd"], approval_id=approval_id)
        if refusal is not None:
            return refusal

    exec_plugin = get_provider(provider)  # ProviderError if data-only
    circuit_hash = canonical_hash(circuit)

    # Idempotency before spend/enqueue (2026-10-26 audit PYC-02). The old
    # lookup existed only in the bridge and ran AFTER this function had
    # created a job; a retry could therefore double-charge. A replay returns
    # the original job; a key reused for a different body refuses.
    if idempotency_key:
        prior = _runner.store.find_by_idempotency(idempotency_key)
        if prior is not None:
            prior_body = (prior.meta or {}).get("body_hash")
            if idempotency_body_hash and prior_body and prior_body != idempotency_body_hash:
                return {"ok": False, "job_id": prior.job_id, "session_id": prior.session_id,
                        "error": "idempotency_key reused with a different request body: refused"}
            return {"ok": True, "job_id": prior.job_id, "session_id": prior.session_id,
                    "status": prior.status, "idempotent_replay": True,
                    "circuit_hash": (prior.meta or {}).get("circuit_hash", circuit_hash)}

    # Dry run (estimate panel, mandate flow): full validation has passed
    # if we get here — return the validated estimate + fidelity verdict
    # with NO spend and NO enqueue. The session row already exists as the
    # estimate's audit anchor (same as a refused dispatch).
    if estimate_only:
        return {"ok": True, "estimate_only": True, "session_id": session_id,
                "estimate": pf["estimate"], "fidelity": fidelity,
                "circuit_hash": circuit_hash}

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

    # Spend is reserved at dispatch (W0.16) so multi-hour QPU work cannot
    # run unbooked. A pre-vendor refusal (no credential/SDK, routing error)
    # refunds the reservation; a failure after the vendor accepted a job
    # keeps the charge (the vendor may have run) and records the residual.
    cost = pf["estimate"]["estimated_usd"]

    # Atomically claim the idempotency key BEFORE money moves. The old
    # check-then-create raced two retries into two charges/dispatches.
    # `reserved_usd` records the Python reservation for settle/cancel; Go
    # mirrors hold no Python reservation (0.0 — Go reconciles its own rail).
    job, created = _runner.store.find_or_create(
        provider=provider, session_id=session_id,
        idempotency_key=idempotency_key or "",
        body_hash=idempotency_body_hash or "",
        circuit_hash=circuit_hash, circuit_qasm=circuit_qasm,
        shots=shots, seed=seed, estimate=pf["estimate"],
        reserved_usd=0.0 if spend_prepaid else cost,
    )
    job_id = job.job_id
    if not created:
        prior_body = (job.meta or {}).get("body_hash")
        if idempotency_body_hash and prior_body and prior_body != idempotency_body_hash:
            return {"ok": False, "job_id": job_id, "session_id": job.session_id,
                    "error": "idempotency_key reused with a different request body: refused"}
        return {"ok": True, "job_id": job_id, "session_id": job.session_id,
                "status": job.status, "idempotent_replay": True,
                "circuit_hash": (job.meta or {}).get("circuit_hash", circuit_hash)}

    # spend_prepaid: Go Core already reserved this job against the authoritative
    # session/tenant book (human/API quantum dispatch). Python must NOT charge
    # its mirror again and must not refund on failure — Go reconciles the
    # reservation from the status poll. Agent-created sessions keep the local
    # Python book (spend_prepaid=False).
    if cost > 0 and not spend_prepaid:
        try:
            _gov.record_spend(session_id, cost)
        except GovernanceError as e:
            try:
                _runner.store.transition(job_id, "failed", error=f"spend refused: {e}")
            except Exception:  # noqa: BLE001 — best-effort ghost cleanup
                pass
            return {"ok": False, "error": f"spend refused at dispatch: {e}",
                    "session_id": session_id}

    pinned: dict[str, str | None] = {"id": None}

    def _run() -> dict[str, Any]:
        # Pin the vendor job id into job meta the moment hardware accepts
        # the submission (before results arrive) so later recall knows
        # what to cancel. Simulators never call back — nothing recorded.
        def _pin(hardware_job_id: str) -> None:
            pinned["id"] = hardware_job_id
            _runner.store.record_hardware_job(job_id, hardware_job_id)
        # returns the provider result; JobRunner records transitions + provenance.
        # Hardware jobs pass wait_for_result=False so the worker thread frees
        # immediately once the vendor accepts the submission; status is then
        # polled via poll_hardware_job on get_job_status / get_result.
        import inspect
        try:
            try:
                sig = inspect.signature(exec_plugin.submit)
                if "wait_for_result" in sig.parameters:
                    return exec_plugin.submit(
                        circuit, shots, seed=seed, on_hardware_job=_pin,
                        wait_for_result=bool(wait),
                    )
            except (TypeError, ValueError):
                pass
            return exec_plugin.submit(circuit, shots, seed=seed, on_hardware_job=_pin)
        except Exception as e:
            # Pre-vendor refusal (missing credential/SDK, routing, provider
            # error) never bought QPU time: return the reservation. Once a
            # hardware job id is pinned the vendor may have run, so keep the
            # charge and record the residual honestly.
            if pinned["id"] is None and cost > 0 and not spend_prepaid:
                try:
                    _gov.refund_spend(session_id, cost, f"pre-vendor failure: {e}")
                    _runner._record("session_spend_refunded", {
                        "session_id": session_id, "job_id": job_id,
                        "amount_usd": cost, "reason": f"pre-vendor failure: {e}",
                    })
                except GovernanceError:
                    pass
            raise

    try:
        _runner.enqueue(job_id, _run)
    except (KeyError, RuntimeError) as e:
        if cost > 0 and not spend_prepaid:
            try:
                _gov.refund_spend(session_id, cost, f"pre-vendor enqueue failure: {e}")
                _runner._record("session_spend_refunded", {
                    "session_id": session_id, "job_id": job_id,
                    "amount_usd": cost, "reason": f"pre-vendor enqueue failure: {e}",
                })
            except GovernanceError:
                pass
        try:
            _runner.store.transition(job_id, "failed", error=f"farm refused: {e}")
        except Exception:  # noqa: BLE001 — best-effort ghost cleanup
            pass
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
           "estimate": pf["estimate"], "fidelity": fidelity}
    if wait_note is not None:
        out["note"] = wait_note
    return out


def _refresh_deferred_hardware_job(job):
    """If `job` is running with a pinned `hardware_job_id`, poll the vendor
    status non-blockingly and transition the store on terminal state."""
    if job is None or job.status != "running":
        return job, None
    hw_id = (getattr(job, "meta", None) or {}).get("hardware_job_id")
    if not hw_id:
        return job, None
    try:
        prov = get_provider(job.provider)
        polled = prov.poll_hardware_job(str(hw_id))
    except Exception as e:  # noqa: BLE001 — transient poll errors keep status running
        return job, str(e)
    p_status = polled.get("status")
    try:
        if p_status == "completed":
            updated = _runner.store.transition(
                job.job_id, "completed", result=polled.get("result") or polled
            )
            _runner._record("job_completed", {"job_id": job.job_id, "ok": True, "hardware_job_id": hw_id})
            # Phase-0 settle: the vendor reported usage — release
            # reserved→actual to the session (idempotent, Go-mirror-safe).
            _maybe_settle_job(updated or job)
            return updated or job, None
        if p_status == "failed":
            err = str(polled.get("error") or f"hardware job {hw_id} failed")
            usage_result = {k: polled[k] for k in ("usage_seconds", "actual_usd") if polled.get(k) is not None}
            if usage_result:
                usage_result["hardware_job_id"] = hw_id
                updated = _runner.store.transition(
                    job.job_id, "failed", error=err, result=usage_result)
            else:
                updated = _runner.store.transition(job.job_id, "failed", error=err)
            _runner._record("job_failed", {"job_id": job.job_id, "error": err, "hardware_job_id": hw_id})
            # Failure settlement: reported usage settles reserved→actual
            # the same way a completion settles (idempotent, Go-mirror-safe).
            _maybe_settle_job(updated or job)
            return updated or job, None
        if p_status == "cancelled":
            usage_result = {k: polled[k] for k in ("usage_seconds", "actual_usd") if polled.get(k) is not None}
            if usage_result:
                usage_result["hardware_job_id"] = hw_id
                updated = _runner.store.transition(
                    job.job_id, "cancelled", result=usage_result)
            else:
                updated = _runner.store.transition(job.job_id, "cancelled")
            _runner._record("job_cancelled", {"job_id": job.job_id, "hardware_job_id": hw_id})
            _maybe_settle_job(updated or job)
            return updated or job, None
    except Exception:  # noqa: BLE001 — race with concurrent transition/cancel
        return _runner.store.get(job.job_id) or job, None
    return job, None


def get_job_status(job_id: str, caller: dict | None = None) -> dict:
    """Status read comes from the PERSISTED store (survives restart).
    When a running hardware job carries a pinned `hardware_job_id`, polls
    the vendor SDK non-blockingly to advance terminal state.

    Ownership-gated before poll/settle: unowned reads return the unknown
    envelope (no oracle)."""
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    if caller is not None:
        owner_session = _job_owner_session(job)
        if owner_session is None or not _session_owned_by(owner_session, caller):
            return {"ok": False, "error": "unknown job_id"}
    job, poll_warn = _refresh_deferred_hardware_job(job)
    if job.status in ("completed", "failed", "cancelled"):
        # Terminal jobs settle here when the transition did not (synchronous
        # worker path); deferred ones settled at the poll transition above.
        # Idempotent either way; pre-vendor failures are a no-op (the
        # dispatch path already refunded — nothing to reconcile).
        _maybe_settle_job(job)
    out = {"ok": True, "job_id": job_id, "status": job.status, "provider": job.provider,
           "session_id": job.session_id, "created_at": job.created_at}
    hw_id = (getattr(job, "meta", None) or {}).get("hardware_job_id")
    if hw_id:
        out["hardware_job_id"] = hw_id
    layout = (getattr(job, "meta", None) or {}).get("layout")
    if layout:
        out["layout"] = layout
    if poll_warn:
        out["poll_warning"] = poll_warn
    return out


def get_result(job_id: str, caller: dict | None = None) -> dict:
    """Ownership-gated like get_job_status (unknown and unowned share the
    envelope; gated before poll/settle)."""
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "error": "unknown job_id"}
    if caller is not None:
        owner_session = _job_owner_session(job)
        if owner_session is None or not _session_owned_by(owner_session, caller):
            return {"ok": False, "error": "unknown job_id"}
    job, _ = _refresh_deferred_hardware_job(job)
    if job.status == "failed":
        out = {"ok": False, "job_id": job_id, "status": job.status, "error": job.error}
        settle = _maybe_settle_job(job)
        if isinstance(settle, dict) and settle.get("refunded_usd", 0.0) > 0:
            out["settlement"] = settle
        return out
    if job.status == "cancelled":
        out = {"ok": False, "job_id": job_id, "status": job.status, "error": "job cancelled"}
        settle = _maybe_settle_job(job)
        if isinstance(settle, dict) and settle.get("refunded_usd", 0.0) > 0:
            out["settlement"] = settle
        return out
    if job.status != "completed":
        return {"ok": False, "job_id": job_id, "status": job.status, "error": "not completed"}
    settle = _maybe_settle_job(job)
    out = {"ok": True, "job_id": job_id, "provider": job.provider, "session_id": job.session_id,
           "result": job.result}
    if isinstance(settle, dict) and settle.get("refunded_usd", 0.0) > 0:
        out["settlement"] = settle
    return out


def cancel_job(job_id: str, caller: dict | None = None) -> dict:
    """Cancel a queued/running job and attempt vendor recall.

    Ownership-gated before the fence, vendor recall, and refund: unowned
    ids return the not-found envelope (no oracle) with zero side effects.

    The cooperative fence is authoritative for Kubona state. If a hardware
    id was pinned at dispatch, also call the provider's cancel() — best
    effort, never implying refund. A vendor refusal does not resurrect the
    job: the fence already stopped Kubona from accepting the result.

    Money (Phase-0): pre-vendor cancel (no hardware id pinned — the vendor
    never ran) refunds the FULL Python reservation, ledger-sealed;
    post-vendor cancel settles to reported usage when the vendor metered any
    (measured usage always wins — the difference is refunded), otherwise keeps
    the charge (the vendor may have run), stated in the receipt. Go-prepaid
    mirrors never refund on this rail.
    """
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "job_id": job_id, "error": "job not found or already terminal"}
    if caller is not None:
        owner_session = _job_owner_session(job)
        if owner_session is None or not _session_owned_by(owner_session, caller):
            return {"ok": False, "job_id": job_id, "error": "job not found or already terminal"}
    hw = (getattr(job, "meta", None) or {}).get("hardware_job_id")
    reserved = _reserved_for_job(job)
    fenced = _runner.cancel(job_id)
    if not fenced:
        return {"ok": False, "job_id": job_id, "error": "job not found or already terminal"}
    out: dict = {"ok": True, "job_id": job_id, "status": "cancelled",
                 "spend_policy": CANCEL_SPEND_POLICY}
    if hw:
        out["provider_recall_attempted"] = True
        try:
            out["provider_recall"] = get_provider(job.provider).cancel(str(hw))
        except ProviderError as exc:
            out["provider_recall_error"] = str(exc)
        # Post-vendor cancel: measured usage wins when reported (settle
        # reserved→actual, refunding the difference — Go parity for
        # failure/cancel settlement); otherwise the charge stands (the vendor
        # may have run), stated in the receipt.
        fresh = _runner.store.get(job_id)
        settle = _maybe_settle_job(fresh) if fresh is not None else None
        if isinstance(settle, dict) and settle.get("refunded_usd", 0.0) > 0:
            out["settled_to_actual_usd"] = settle.get("actual_usd")
            out["refunded_usd"] = settle["refunded_usd"]
            out["settle_note"] = (
                "post-vendor cancel with reported usage: settled to actual, "
                "difference refunded (the billed part may still have run)")
            return out
        if reserved > 0:
            out["charge_kept_usd"] = reserved
            out["charge_kept_reason"] = (
                "post-vendor cancel: hardware job id was pinned, the vendor "
                "may have run — reservation kept")
        return out
    # Pre-vendor cancel: the vendor never ran — return the full reservation.
    # The fence transition above is atomic (terminal states are immutable),
    # so exactly one canceller can reach this refund: no double-release.
    if reserved <= 0:
        out["refund_usd"] = 0.0
        out["refund_note"] = "no Python reservation (free tier or Go-prepaid mirror): nothing to refund"
        return out
    try:
        session = _sessions.get(job.session_id)
    except KeyError:
        out["refund_usd"] = 0.0
        out["refund_error"] = "session gone: reservation kept, no refund issued"
        return out
    if _is_spend_prepaid(session):
        out["refund_usd"] = 0.0
        out["refund_note"] = "go-prepaid mirror: Go reconciles its own rail, Python refunds nothing"
        return out
    try:
        _gov.refund_spend(job.session_id, reserved,
                          f"pre-vendor cancel {job_id}: vendor never ran, full reservation returned")
        _runner._record("session_spend_refunded", {
            "session_id": job.session_id, "job_id": job_id,
            "amount_usd": reserved, "reason": f"pre-vendor cancel {job_id}",
        })
    except GovernanceError as e:
        out["refund_usd"] = 0.0
        out["refund_error"] = str(e)
        return out
    out["refund_usd"] = reserved
    out["refund_reason"] = f"pre-vendor cancel {job_id}: vendor never ran (no hardware id pinned)"
    return out


def request_provider_recall(job_id: str) -> dict:
    """Vendor-side recall for a Kubona job (R-33). Cooperative fence first
    (always applies); then, if the job's meta carries a hardware_job_id
    pinned at dispatch, the provider's cancel() is attempted. Recall is
    best-effort once running — always re-check status — and never implies
    refund (see the R-32 proposal). Completed jobs with a recorded hardware
    id are recallable for erroneous-billing disputes; running jobs whose
    hardware id is not yet visible report exactly that instead of pretending."""
    job = _runner.store.get(job_id)
    if job is None:
        return {"ok": False, "job_id": job_id, "error": "unknown job_id"}
    if job.status in ("completed", "failed"):
        hw = (getattr(job, "meta", None) or {}).get("hardware_job_id")
        if not hw:
            return {"ok": False, "job_id": job_id, "status": job.status,
                    "provider_recall_attempted": False,
                    "error": "terminal job has no recorded hardware id: nothing to recall"}
        try:
            rec = get_provider(job.provider).cancel(hw)
        except ProviderError as e:
            return {"ok": False, "job_id": job_id, "status": job.status,
                    "provider_recall_attempted": True, "error": str(e)}
        return {"ok": True, "job_id": job_id, "status": job.status,
                "provider_recall_attempted": True, "provider_recall": rec}
    # Queued/running: fence first, then recall if the hardware id is visible.
    fenced = _runner.cancel(job_id)
    refreshed = _runner.store.get(job_id)
    hw = ((getattr(refreshed, "meta", None) or getattr(job, "meta", None)) or {}).get("hardware_job_id")
    if not hw:
        return {"ok": fenced, "job_id": job_id, "status": "cancelled" if fenced else job.status,
                "provider_recall_attempted": False,
                "note": "fenced cooperatively; hardware id not yet visible "
                        "(submit before results) — re-check status; provider "
                        "work may still complete underneath"}
    try:
        rec = get_provider(job.provider).cancel(hw)
    except ProviderError as e:
        return {"ok": fenced, "job_id": job_id, "status": "cancelled" if fenced else job.status,
                "provider_recall_attempted": True, "error": str(e)}
    return {"ok": True, "job_id": job_id, "status": "cancelled" if fenced else job.status,
            "provider_recall_attempted": True, "provider_recall": rec}


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
    try:
        result = run_zne(circuit, shots=shots, noise_rate=noise_rate,
                         scales=tuple(parsed), seed=seed)
    except ValueError as e:
        return {"ok": False, "error": f"zne refused: {e}"}
    return {"ok": True, **result.to_dict()}


def session_status(session_id: str, jobs_limit: int = 100,
                   caller: dict | None = None) -> dict:
    """Session state with a bounded job-id page.

    The store may accumulate thousands of job ids; returning them all is an
    unbounded read. Keep the newest `jobs_limit` (1..1000), with totals and an
    explicit truncation flag so callers know to page/filter.

    Ownership-gated: unowned sessions return the unknown envelope (no oracle).
    """
    if not isinstance(jobs_limit, int) or isinstance(jobs_limit, bool):
        jobs_limit = 100
    jobs_limit = min(max(jobs_limit, 1), 1000)
    try:
        state = _sessions.get(session_id)
    except KeyError:
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}
    if not _session_owned_by(state, caller):
        return {"ok": False, "error": f"unknown session_id {session_id!r}"}
    out = {"ok": True, **{k: v for k, v in state.__dict__.items() if k not in ("meta", "jobs")}}
    total = len(state.jobs)
    out["jobs_total"] = total
    out["jobs"] = list(state.jobs[-jobs_limit:])
    out["jobs_returned"] = len(out["jobs"])
    out["jobs_truncated"] = total > jobs_limit
    return out


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
    (submit_qubo_job, "Submit a QUBO matrix to the D-Wave annealing provider (local exact/SA or Leap cloud)."),
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
