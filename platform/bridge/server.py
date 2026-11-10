"""
Python gRPC Server implementing QuantumBridgeService (proto/kubona.proto).
Exposes circuit execution, circuit cutting, classical chemistry anchors, epistemic evaluation,
and live scientific literature grounding.
"""
from __future__ import annotations

import hashlib
import hmac
import logging
import os
import sys
from pathlib import Path
from concurrent import futures
import grpc

_DIR = Path(__file__).resolve().parent
if str(_DIR) not in sys.path:
    sys.path.insert(0, str(_DIR))
if str(_DIR / "proto") not in sys.path:
    sys.path.insert(0, str(_DIR / "proto"))

from proto import kubona_pb2, kubona_pb2_grpc
from chemistry import (
    evaluate_candidate_pipeline,
    evaluate_molecular_admet,
    solve_classical_anchor,
)
from epistemics import evaluate_claim_credibility
from executor import execute_qasm, cut_circuit_qasm
from literature import search_arxiv, search_openalex, to_records
from fidelity import predict_fidelity, probe_clifford_fidelity
from krylov import solve_quantum_krylov
from mitigation import mitigate_circuit

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("kubona-bridge")


# Workflow gateway scopes (least privilege for bridge-executed workflows).
# Everything the workflow tools legitimately need (compiler reads, submit +
# read, cost reads + session management, provider catalog, provenance,
# observatory, loop management) — and deliberately NOT execution:override
# (cancel others' jobs), control:override (kill/pause others' loops) or
# cost:approve (approve spend). The old scopes=("*") made every bridged
# workflow god-mode; a forgotten kwargs change must never do that again.
_WORKFLOW_GATEWAY_SCOPES = (
    "compiler:read",
    "execution:submit", "execution:read",
    "cost:read", "cost:manage",
    "providers:read",
    "provenance:read", "provenance:write",
    "world:read", "world:write",
    "control:read", "control:manage",
)


class _BridgeTokenAuth:
    """Authenticate the bridge-to-kubonad transport token as a per-caller operator.

    The bridge is a private compute plane. kubonad is the only intended
    caller, and it presents KUBONA_BRIDGE_TOKEN. The principal is scoped to
    the workflow tools above and labeled per caller (tenant/session) so
    audit entries attribute the bridged execution — never a bare god-mode
    ("*") principal shared by every caller.
    """

    def __init__(self, token: str, sub: str = "kubonad-bridge",
                 scopes: tuple = _WORKFLOW_GATEWAY_SCOPES, tenant: str = "default"):
        self._token = token
        self._sub = sub
        self._scopes = scopes
        self._tenant = tenant

    def verify(self, token: str):
        from kubona.core.auth import AuthError, Principal
        if not self._token or not hmac.compare_digest(token or "", self._token):
            raise AuthError("invalid or missing bridge token")
        return Principal(sub=self._sub, role="operator", scopes=tuple(self._scopes),
                         tenant=self._tenant)


def _workflow_auth(caller_label: str, tenant: str = "default"):
    """Gateway auth for one bridged workflow execution, fail-closed.

    The bridge token is required in ALL envs (no dev fallback): without it
    there is no authenticated principal to run workflow tools under.
    """
    token = os.getenv("KUBONA_BRIDGE_TOKEN", "")
    if not token:
        return None, ("bridge token is not configured: refusing "
                      "unauthenticated workflow execution")
    label = (caller_label or "").strip() or "uncategorized"
    return _BridgeTokenAuth(token, sub=f"kubonad-bridge:{label}", tenant=tenant or "default"), ""


# Must match cmd/kubonad/bridge_auth.go byte-for-byte. The shared vector in
# both test suites catches drift before it can authorize a wrong ceiling.
_SESSION_ASSERTION_METHODS = {
    "/kubona.QuantumBridgeService/QuantumSubmit",
    "/kubona.QuantumBridgeService/WorkflowRun",
}


def _session_assertion_payload(request) -> str:
    return (f"{getattr(request, 'session_id', '')}|"
            f"{float(getattr(request, 'session_budget_ceiling_usd', 0.0) or 0.0):.6f}|"
            f"{float(getattr(request, 'session_spent_usd', 0.0) or 0.0):.6f}|"
            f"{getattr(request, 'tenant_id', '') or ''}|"
            f"{'true' if getattr(request, 'session_verified', False) else 'false'}")


def _session_assertion_mac(token: str, request) -> str:
    return hmac.new(token.encode(), _session_assertion_payload(request).encode(),
                    hashlib.sha256).hexdigest()


def _verify_session_assertion(request, context, token: str) -> None:
    """Refuse a session mirror whose money/tenant fields are not Go-signed."""
    if not getattr(request, "session_verified", False):
        return
    md = dict(getattr(context, "invocation_metadata", lambda: ())() or ())
    got = md.get("x-kubona-session-assertion", "")
    want = _session_assertion_mac(token, request)
    if not got or not hmac.compare_digest(got, want):
        context.abort(grpc.StatusCode.UNAUTHENTICATED,
                      "session assertion missing or invalid: refusing self-declared money/tenant")


# Simulator-class backends are zero-cost local execution (mirrors Go
# jobs.isSimulatorBackend + Python execution._LOCAL_ONLY_PROVIDERS). Every
# other backend string names metered hardware and additionally requires a
# verified spend mandate — the bridge never relies solely on the Go edge
# for money.
_FREE_SIMULATOR_BACKENDS = frozenset({
    "", "local-simulator", "local_simulator",
    "aer", "qiskit-aer", "qiskit-aer-simulator",
    "statevector", "statevector_simulator",
    "gpu_simulator", "gpu-simulator",
})


def _is_paid_backend(provider: str) -> bool:
    return (provider or "").strip().lower() not in _FREE_SIMULATOR_BACKENDS


class _MandateRefused(ValueError):
    """Spend-mandate verification refused (fail-closed money gate)."""


def _verify_spend_mandate(mandate_ref_json: str, *, tenant_id: str,
                           session_id: str, qasm: str,
                           estimated_cost_usd: float) -> None:
    """Verify the spend mandate inside QuantumSubmit before dispatch.

    Tenant + job-hash binding, budget cover, expiry, and Ed25519 signature
    (mirrors Go verifyMandateRef / Python verify_mandate). Raises
    _MandateRefused on invalid/missing mandate — the caller gets ok=False,
    never an enqueue.
    """
    from kubona.cost.mandate import (
        SpendMandate, canonical_job_binding, verify_mandate,
    )

    if not (mandate_ref_json or "").strip():
        raise _MandateRefused(
            "a signed spend mandate is required for paid-backend dispatch "
            "(mandate_ref_json missing)")
    try:
        import json as _json
        data = _json.loads(mandate_ref_json)
        mandate = SpendMandate(
            tenant_id=data["tenant_id"],
            job_hash=data["job_hash"],
            max_budget_usd=float(data["max_budget_usd"]),
            expiry_timestamp=data["expiry_timestamp"],
            public_key_hex=data["public_key_hex"],
            signature_hex=data["signature_hex"],
        )
    except Exception as e:
        raise _MandateRefused(
            f"mandate_ref_json must be the signed mandate JSON: {e}")
    if not mandate.signature_hex or not mandate.job_hash or not mandate.public_key_hex:
        raise _MandateRefused("unsigned or unbound mandate refused")
    expected_job_hash = canonical_job_binding("circuit_run", session_id or "", qasm or "")
    ok, reason = verify_mandate(mandate, tenant_id or "",
                                expected_job_hash, float(estimated_cost_usd or 0.0))
    if not ok:
        raise _MandateRefused(f"spend mandate invalid: {reason}")


def _job_recorded_session(job_id: str):
    """Authoritative (server-side) session/tenant for a job, from the record.

    Returns (session_id, tenant_id); tenant comes from the session store's
    meta (never from request fields). Raises KeyError for unknown jobs.
    """
    from kubona.mcp.servers import execution as _exec

    job = _exec._runner.store.get(job_id)
    if job is None:
        raise KeyError(f"unknown job_id {job_id!r}")
    session_id = job.session_id or ""
    tenant_id = ""
    if session_id:
        try:
            sess = _exec._sessions.get(session_id)
            tenant_id = str((getattr(sess, "meta", None) or {}).get("tenant_id", "") or "")
        except KeyError:
            tenant_id = ""
    return session_id, tenant_id


def _enforce_job_ownership(job_id: str, caller_session_id: str,
                            caller_tenant_id: str):
    """Enforce caller session/tenant against the job's recorded session.

    The job's session/tenant are derived server-side from the job record
    (never from the request). A caller claim that mismatches the record
    refuses fail-closed. An absent claim is the Go-edge path (kubonad
    authorizes status/cancel via JobSession + authorizeSessionRead before
    proxying) and is allowed here — the bridge check is defense-in-depth
    for direct bridge callers, never a bypass.
    Returns (job, "") when allowed, (None, reason) when refused.
    """
    from kubona.mcp.servers import execution as _exec

    job = _exec._runner.store.get(job_id)
    if job is None:
        return None, "unknown job_id"
    recorded_session, recorded_tenant = _job_recorded_session(job_id)
    caller_session_id = (caller_session_id or "").strip()
    caller_tenant_id = (caller_tenant_id or "").strip()
    if caller_session_id and caller_session_id != recorded_session:
        return None, ("caller session does not own this job: refusing "
                       "cross-session read/cancel")
    if caller_tenant_id and caller_tenant_id != (recorded_tenant or ""):
        return None, ("caller tenant does not own this job: refusing "
                       "cross-tenant read/cancel")
    return job, ""


class _AuthInterceptor(grpc.ServerInterceptor):
    """Require `authorization: Bearer <KUBONA_BRIDGE_TOKEN>` on every RPC."""

    def __init__(self, token: str):
        self._token = token

    def intercept_service(self, continuation, handler_call_details):
        md = dict(handler_call_details.invocation_metadata or ())
        got = md.get("authorization", "")
        if not self._token or not hmac.compare_digest(got, f"Bearer {self._token}"):
            def _deny(request, context):
                context.abort(grpc.StatusCode.UNAUTHENTICATED,
                              "bridge authentication required")
            return grpc.unary_unary_rpc_method_handler(_deny)
        handler = continuation(handler_call_details)
        method = getattr(handler_call_details, "method", "")
        if handler is not None and method in _SESSION_ASSERTION_METHODS:
            inner = handler.unary_unary
            def _wrapped(request, context):
                _verify_session_assertion(request, context, self._token)
                return inner(request, context)
            return grpc.unary_unary_rpc_method_handler(
                _wrapped,
                request_deserializer=handler.request_deserializer,
                response_serializer=handler.response_serializer)
        return handler


class QuantumBridgeServicer(kubona_pb2_grpc.QuantumBridgeServiceServicer):
    # Failure contract: handlers whose response carries success/error_message
    # return a structured failure (success=False + cause) on unexpected
    # exceptions — never an unstructured gRPC UNKNOWN, and never a fake
    # success. EvaluateWavefunction and GroundLiterature have NO success
    # field in their proto messages, so raising (UNKNOWN, handled
    # fail-closed by Go callers) is their honest failure mode; wrapping
    # GroundLiterature to return empty records would read an outage as
    # "no literature found". Each wrapped handler keeps its own envelope.
    def ExecuteCircuit(self, request, context):
        logger.info(f"ExecuteCircuit called for Job {request.job_id}")
        try:
            # W1.4: executor requires an explicit seed (no unseeded sampling).
            # The proto carries no seed field, so derive deterministically
            # from job_id (same job -> same seed, reproducible; empty -> 7).
            import hashlib as _hl
            _jid = request.job_id or ""
            _seed = int(_hl.sha256(_jid.encode()).hexdigest()[:8], 16) % (2 ** 31) if _jid else 7
            from executor import route_backend
            backend = request.requested_backend or "aer_simulator"
            if route_backend(backend).startswith("hardware:"):
                return kubona_pb2.CircuitExecutionResponse(
                    job_id=request.job_id,
                    success=False,
                    error_message=(
                        "hardware dispatch is not served by ExecuteCircuit: use the "
                        "governed QuantumSubmit rail (cost/session/idempotency apply)"
                    ),
                )
            res = execute_qasm(
                qasm=request.qasm,
                shots=request.shots if request.shots > 0 else 1024,
                requested_backend=backend,
                allow_fallback=request.allow_classical_fallback,
                caller_jurisdictions=list(request.caller_jurisdictions),
                caller_export_class=request.caller_export_class or "any",
                export_reviewed=request.export_reviewed,
                seed=_seed,
            )
        except Exception as e:
            logger.exception("ExecuteCircuit failed for Job %s", request.job_id)
            return kubona_pb2.CircuitExecutionResponse(
                job_id=request.job_id,
                success=False,
                error_message=f"circuit execution failed: {e}",
            )
        return kubona_pb2.CircuitExecutionResponse(
            job_id=request.job_id,
            success=res["success"],
            resolved_backend=res["resolved_backend"],
            degradation_level=res["degradation_level"],
            degradation_reason=res["degradation_reason"],
            counts=res["counts"],
            statevector_real=res["statevector_real"],
            statevector_imag=res["statevector_imag"],
            meyer_wallach_entanglement=res["meyer_wallach"],
            execution_time_ms=res["execution_time_ms"],
            error_message=res["error_message"],
            hardware_job_id=res.get("hardware_job_id") or "",
        )

    def CutCircuit(self, request, context):
        logger.info(f"CutCircuit called with max_qubits={request.max_subcircuit_qubits}")
        try:
            res = cut_circuit_qasm(request.qasm, request.max_subcircuit_qubits or 2)
        except Exception as e:
            logger.exception("CutCircuit failed")
            return kubona_pb2.CircuitCutResponse(
                success=False,
                error_message=f"circuit cutting failed: {e}",
            )
        fragments = [
            kubona_pb2.SubcircuitFragment(
                fragment_id=f["fragment_id"],
                qasm=f["qasm"],
                cut_qubits=f["cut_qubits"],
            )
            for f in res.get("fragments", [])
        ]
        return kubona_pb2.CircuitCutResponse(
            success=res["success"],
            fragments=fragments,
            total_cuts=res["total_cuts"],
            error_message=res["error_message"],
            sampling_overhead_kappa=float(res.get("sampling_overhead_kappa", 1.0)),
        )

    def ComputeClassicalAnchor(self, request, context):
        logger.info(f"ComputeClassicalAnchor called for {request.molecule} at R={request.bond_distance}")
        try:
            # W1.8: basis_set travels the wire now (was silently dropped).
            basis = (getattr(request, "basis_set", "") or "").strip() or "sto-3g"
            res = solve_classical_anchor(request.molecule, request.bond_distance, basis=basis)
            return kubona_pb2.ClassicalAnchorResponse(
                molecule=res["molecule"],
                bond_distance=res["bond_distance"],
                rhf_energy=res["rhf_energy"],
                fci_exact_energy=res["fci_exact_energy"],
                nuclear_repulsion=res["nuclear_repulsion"],
                orbital_energies=res["orbital_energies"],
                success=True,
                error_message="",
            )
        except Exception as e:
            return kubona_pb2.ClassicalAnchorResponse(
                molecule=request.molecule,
                bond_distance=request.bond_distance,
                success=False,
                error_message=str(e),
            )

    def EvaluateMolecularADMET(self, request, context):
        # T-01a: chemistry over the typed rail. The science payload rides
        # as JSON (evolving Dict[str, Any]) — the proto carries transport
        # framing only. Structured failure per the servicer contract.
        logger.info("EvaluateMolecularADMET called")
        try:
            import json as _json

            res = evaluate_molecular_admet(request.smiles)
            return kubona_pb2.ChemistryADMETResponse(
                success=True,
                result_json=_json.dumps(res),
                error_message="",
            )
        except Exception as e:
            logger.exception("EvaluateMolecularADMET failed")
            return kubona_pb2.ChemistryADMETResponse(
                success=False,
                result_json="",
                error_message=str(e),
            )

    def EvaluateDiscoveryPipeline(self, request, context):
        # T-01a: same framing as ADMET; the pipeline ranks caller-supplied
        # candidates (name + smiles each) — no chemistry is invented here.
        logger.info(f"EvaluateDiscoveryPipeline called for {request.target}")
        try:
            import json as _json

            cands = [{"name": c.name, "smiles": c.smiles} for c in request.candidates]
            res = evaluate_candidate_pipeline(request.target, cands)
            return kubona_pb2.DiscoveryPipelineResponse(
                success=True,
                result_json=_json.dumps(res),
                error_message="",
            )
        except Exception as e:
            logger.exception("EvaluateDiscoveryPipeline failed")
            return kubona_pb2.DiscoveryPipelineResponse(
                success=False,
                result_json="",
                error_message=str(e),
            )

    def EvaluateCodesign(self, request, context):
        # T-01b: closed-vocabulary dispatch into codesign_ops — the
        # operation selects a function, never caller-supplied code.
        # Structured failure per the servicer failure contract.
        logger.info(f"EvaluateCodesign called for operation {request.operation}")
        try:
            import json as _json

            from codesign_ops import run_op

            body = _json.loads(request.request_json or "{}")
            res = run_op(request.operation, body)
            if not res.get("ok"):
                return kubona_pb2.CodesignResponse(
                    success=False,
                    result_json="",
                    error_message=str(res.get("error", "evaluation refused")),
                )
            return kubona_pb2.CodesignResponse(
                success=True,
                result_json=_json.dumps(res),
                error_message="",
            )
        except Exception as e:
            logger.exception("EvaluateCodesign failed")
            return kubona_pb2.CodesignResponse(
                success=False,
                result_json="",
                error_message=str(e),
            )

    # -- Human quantum dispatch (R-34) + provider cancel (R-33) --
    # Thin transport over the MCP execution rail: the SAME submit_job the
    # agents use (jurisdiction, preflight, fidelity, spend-at-dispatch,
    # async-only QPU). No parallel governance exists here by construction.
    def QuantumSubmit(self, request, context):
        import hashlib as _hl
        import json as _json

        from kubona.mcp.servers import execution as _exec

        try:
            if not request.idempotency_key:
                return kubona_pb2.QuantumSubmitResponse(
                    ok=False, error_message="idempotency_key is required: "
                    "retries without a key risk double hardware spend", job_id="")
            # Restart-resilient idempotency: the key lives in job meta
            # (persisted store), so a retry after process restart replays
            # the original job_id instead of double-submitting.
            body_hash = _hl.sha256(
                (request.qasm + "\x00" + request.provider + "\x00" + str(request.shots)).encode()
            ).hexdigest()
            prior = _exec._runner.store.find_by_idempotency(request.idempotency_key)
            if prior is not None:
                if prior.meta.get("body_hash") == body_hash:
                    return kubona_pb2.QuantumSubmitResponse(
                        ok=True, error_message="", job_id=prior.job_id,
                        estimate_json=_json.dumps(prior.meta.get("estimate", {})),
                        fidelity_json="", queued=prior.status == "queued",
                        session_id=prior.session_id,
                        idempotent_replay=True)
                return kubona_pb2.QuantumSubmitResponse(
                    ok=False, error_message="idempotency_key reuse with a different body: "
                    "replay the original request or mint a new key", job_id="")

            # Cross-rail session authority (F-NEW-01 / F-NEW-05): when Go Core
            # has verified the session in SQLite, mirror its authoritative
            # budget ceiling and spent_usd into Python's SessionStore before
            # submit_job runs preflight.
            if request.session_id and getattr(request, "session_verified", False):
                from kubona.mcp.servers import cost as _cost
                raw_ceiling = getattr(request, "session_budget_ceiling_usd", 0.0)
                ceiling_usd = float(raw_ceiling) if raw_ceiling and raw_ceiling > 0 else _exec.IMPLICIT_SESSION_CEILING_USD
                spent_usd = max(0.0, float(getattr(request, "session_spent_usd", 0.0) or 0.0))
                _exec._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                    spend_prepaid=True,
                )
                from kubona.mcp.servers import cost as _cost
                _cost._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                    spend_prepaid=True,
                )

            caller_jurisdictions = list(getattr(request, "caller_jurisdictions", None) or [])
            caller_export_class = getattr(request, "caller_export_class", "") or "any"
            require_review = bool(getattr(request, "require_review", False))

            # Spend-mandate gate (money): for any paid backend the bridge
            # verifies the mandate itself before dispatch — it never relies
            # solely on the Go edge. Free simulators proceed without one;
            # dry-run is the pre-mandate estimate panel and skips the gate.
            effective_session_id = request.session_id or None
            if not request.dry_run and _is_paid_backend(request.provider):
                seed = int(_hl.sha256(request.idempotency_key.encode()).hexdigest()[:8], 16) % (2 ** 31)
                pre = _exec.submit_job(
                    circuit_qasm=request.qasm, provider=request.provider or "local-simulator",
                    shots=request.shots if request.shots > 0 else 1024,
                    session_id=request.session_id or None,
                    caller_jurisdictions=caller_jurisdictions,
                    caller_export_class=caller_export_class,
                    require_review=require_review,
                    wait=False, seed=seed,
                    estimate_only=True,
                    idempotency_key=None,
                    idempotency_body_hash=None,
                )
                if not pre.get("ok"):
                    return kubona_pb2.QuantumSubmitResponse(
                        ok=False, error_message=str(pre.get("error", "submit refused")),
                        job_id="")
                effective_session_id = pre.get("session_id") or request.session_id or ""
                try:
                    est_cost = float((pre.get("estimate") or {}).get("estimated_usd", 0.0) or 0.0)
                except (TypeError, ValueError):
                    est_cost = 0.0
                effective_tenant = (getattr(request, "tenant_id", "") or "").strip()
                if not effective_tenant and effective_session_id:
                    try:
                        sess = _exec._sessions.get(effective_session_id)
                        effective_tenant = str((getattr(sess, "meta", None) or {}).get("tenant_id", "") or "")
                    except KeyError:
                        effective_tenant = ""
                try:
                    _verify_spend_mandate(
                        getattr(request, "mandate_ref_json", "") or "",
                        tenant_id=effective_tenant,
                        session_id=effective_session_id or "",
                        qasm=request.qasm,
                        estimated_cost_usd=est_cost,
                    )
                except _MandateRefused as e:
                    return kubona_pb2.QuantumSubmitResponse(
                        ok=False, error_message=f"spend mandate refused: {e}", job_id="")

            # wait is forced async for QPU inside submit_job; the bridge
            # never blocks on hardware. Seed derives from the idempotency
            # key: same key -> same seed -> reproducible retry.
            seed = int(_hl.sha256(request.idempotency_key.encode()).hexdigest()[:8], 16) % (2 ** 31)
            out = _exec.submit_job(
                circuit_qasm=request.qasm, provider=request.provider or "local-simulator",
                shots=request.shots if request.shots > 0 else 1024,
                session_id=effective_session_id,
                caller_jurisdictions=caller_jurisdictions,
                caller_export_class=caller_export_class,
                require_review=require_review,
                wait=False, seed=seed,
                estimate_only=bool(request.dry_run),
                idempotency_key=request.idempotency_key,
                idempotency_body_hash=body_hash,
            )
            if not out.get("ok"):
                return kubona_pb2.QuantumSubmitResponse(
                    ok=False, error_message=str(out.get("error", "submit refused")),
                    job_id=out.get("job_id", ""))
            if request.dry_run:
                return kubona_pb2.QuantumSubmitResponse(
                    ok=True, error_message="", job_id="",
                    estimate_json=_json.dumps(out.get("estimate", {})),
                    fidelity_json=_json.dumps(out.get("fidelity", {})),
                    queued=False, session_id=out.get("session_id", ""))
            # Tenant attribution (cross-book audit join): advisory merge
            # into session meta — never fails the dispatch.
            if request.tenant_id and out.get("session_id"):
                try:
                    sess = _exec._sessions.get(out["session_id"])
                    meta = dict(getattr(sess, "meta", None) or {})
                    meta.setdefault("tenant_id", request.tenant_id)
                    meta.setdefault("origin", "quantum-bridge-rpc")
                    _exec._sessions.update(out["session_id"], meta=meta)
                except Exception:
                    logger.warning("tenant attribution merge failed for session %s",
                                   out.get("session_id"))
            return kubona_pb2.QuantumSubmitResponse(
                ok=True, error_message="", job_id=out.get("job_id", ""),
                estimate_json=_json.dumps(out.get("estimate", {})),
                fidelity_json=_json.dumps(out.get("fidelity", {})),
                queued=True, session_id=out.get("session_id", ""))
        except Exception as e:
            logger.exception("QuantumSubmit failed")
            return kubona_pb2.QuantumSubmitResponse(
                ok=False, error_message=f"quantum submit failed: {e}", job_id="")

    def QuantumJobStatus(self, request, context):
        import json as _json

        from kubona.mcp.servers import execution as _exec

        try:
            # Ownership: the caller's session/tenant (when presented) must
            # match the job's recorded session — derived server-side from
            # the job record, never from request fields.
            _, refusal = _enforce_job_ownership(
                request.job_id,
                getattr(request, "caller_session_id", "") or "",
                getattr(request, "caller_tenant_id", "") or "")
            if refusal:
                return kubona_pb2.QuantumJobStatusResponse(
                    ok=False, error_message=refusal, job_id=request.job_id,
                    status="", result_json="", estimate_json="")
            st = _exec.get_job_status(request.job_id)
            if not st.get("ok"):
                return kubona_pb2.QuantumJobStatusResponse(
                    ok=False, error_message=str(st.get("error", "unknown job")), job_id=request.job_id,
                    status="", result_json="", estimate_json="")
            res = _exec.get_result(request.job_id)
            payload: dict = {}
            hw = st.get("hardware_job_id")
            if hw:
                payload["hardware_job_id"] = str(hw)
            if st.get("layout"):
                payload["layout"] = st.get("layout")
            for key in ("usage_seconds", "actual_usd"):
                if st.get(key) is not None:
                    payload[key] = st.get(key)
            if res.get("ok"):
                payload["result"] = res.get("result", {})
            elif res.get("error"):
                payload["error"] = str(res.get("error"))
            return kubona_pb2.QuantumJobStatusResponse(
                ok=True, error_message="", job_id=request.job_id,
                status=st.get("status", ""),
                result_json=_json.dumps(payload) if payload else "",
                estimate_json="",
                session_id=str(st.get("session_id", "")))
        except Exception as e:
            logger.exception("QuantumJobStatus failed")
            return kubona_pb2.QuantumJobStatusResponse(
                ok=False, error_message=f"job status failed: {e}", job_id=request.job_id,
                status="", result_json="", estimate_json="")

    def QuantumCancel(self, request, context):
        from kubona.mcp.servers import execution as _exec

        try:
            if not request.job_id:
                return kubona_pb2.QuantumCancelResponse(
                    ok=False, error_message="job_id is required", job_id="",
                    provider_recall_attempted=False, provider_recall_status="")
            # Ownership before the fence: a mismatched caller must not
            # cancel (or fence) another session's job.
            _, refusal = _enforce_job_ownership(
                request.job_id,
                getattr(request, "caller_session_id", "") or "",
                getattr(request, "caller_tenant_id", "") or "")
            if refusal:
                return kubona_pb2.QuantumCancelResponse(
                    ok=False, error_message=refusal, job_id=request.job_id,
                    provider_recall_attempted=False, provider_recall_status="")
            rec = _exec.request_provider_recall(request.job_id)
            return kubona_pb2.QuantumCancelResponse(
                ok=bool(rec.get("ok")), error_message=str(rec.get("error") or rec.get("note", "")),
                job_id=request.job_id,
                provider_recall_attempted=bool(rec.get("provider_recall_attempted")),
                provider_recall_status=str(rec.get("provider_recall", rec.get("status", ""))))
        except Exception as e:
            logger.exception("QuantumCancel failed")
            return kubona_pb2.QuantumCancelResponse(
                ok=False, error_message=f"quantum cancel failed: {e}", job_id=request.job_id,
                provider_recall_attempted=False, provider_recall_status="")

    def RunCanary(self, request, context):
        import concurrent.futures as _fut
        import json as _json

        from kubona.canary import canary as _canary

        try:
            providers = [p for p in request.providers if p] or None
            max_seconds = request.max_seconds if request.max_seconds > 0 else 60
            with _fut.ThreadPoolExecutor(max_workers=1) as pool:
                fut = pool.submit(_canary.run_all, None, providers)
                try:
                    results = fut.result(timeout=max_seconds)
                except _fut.TimeoutError:
                    return kubona_pb2.CanaryRunResponse(
                        ok=False, error_message=f"canary exceeded {max_seconds}s: "
                        "treat as unmeasured, fall back to declared catalog", results_json="")
            return kubona_pb2.CanaryRunResponse(
                ok=True, error_message="", results_json=_json.dumps(results, default=str))
        except Exception as e:
            logger.exception("RunCanary failed")
            return kubona_pb2.CanaryRunResponse(
                ok=False, error_message=f"canary failed: {e}", results_json="")

    def EvaluateWavefunction(self, request, context):
        logger.info(f"EvaluateWavefunction called for Claim {request.claim_id}")
        try:
            res = evaluate_claim_credibility(
                claim_id=request.claim_id,
                statement=request.statement,
                quantum_value=request.quantum_value,
                classical_anchor_value=request.classical_anchor_value,
                source_platforms=list(request.source_platforms),
                total_engagement=request.total_engagement,
                contradiction_signal=request.contradiction_signal,
            )
        except Exception as e:
            logger.exception("EvaluateWavefunction failed for Claim %s", request.claim_id)
            return kubona_pb2.ClaimEvaluationResponse(
                claim_id=request.claim_id,
                success=False,
                error_message=f"claim evaluation failed: {e}",
            )
        return kubona_pb2.ClaimEvaluationResponse(
            claim_id=res["claim_id"],
            corroborated_amplitude=res["corroborated_amplitude"],
            contested_amplitude=res["contested_amplitude"],
            unverified_amplitude=res["unverified_amplitude"],
            passes_chemical_accuracy=res["passes_chemical_accuracy"],
            delta_mha=res["delta_mha"],
            quantum_advantage_claimed=res["quantum_advantage_claimed"],
            quantum_advantage_verified=res["quantum_advantage_verified"],
            verdict_reason=res["verdict_reason"],
            success=True,
            error_message="",
        )

    def GroundLiterature(self, request, context):
        logger.info(f"GroundLiterature called for query '{request.query}'")
        limit = request.limit if request.limit > 0 else 5
        raw_results = search_arxiv(request.query, max_results=limit)
        if not raw_results:
            raw_results = search_openalex(request.query, max_results=limit)

        records = [
            kubona_pb2.LiteratureRecord(
                id=rec["id"],
                title=rec["title"],
                authors=rec["authors"],
                abstract_text=rec["abstract"],
                url=rec["url"],
                publication_year=rec["year"],
                citation_count=rec["cited_by_count"],
            )
            for rec in to_records(raw_results)
        ]

        # cached=True only when every record came from the local cache —
        # a live or fallback fetch is never labeled cached.
        all_cached = bool(records) and all(bool(r.get("cached", False)) for r in raw_results)
        return kubona_pb2.LiteratureGroundingResponse(records=records, cached=all_cached)

    def ExecuteQuantumKrylov(self, request, context):
        logger.info(f"ExecuteQuantumKrylov called for {request.molecule} at R={request.bond_distance}")
        # W1.8: the solver is classical emulation — a metered-hardware
        # backend here would label emulation as hardware compute
        # (quantum-washing). Hardware dispatch goes through circuit_run.
        backend = (getattr(request, "requested_backend", "") or "").strip().lower()
        if backend and backend not in ("local_simulator", "local-simulator", "aer",
                                       "qiskit-aer", "statevector", "statevector_simulator",
                                       "gpu_simulator", "simulator"):
            return kubona_pb2.QuantumKrylovResponse(
                success=False,
                error_message=(f"backend {request.requested_backend!r} is not a simulator: "
                               "krylov runs as classical emulation, hardware dispatch "
                               "goes through circuit_run"),
            )
        try:
            res = solve_quantum_krylov(
                molecule=request.molecule if request.molecule else "H2",
                bond_distance=request.bond_distance if request.bond_distance > 0 else 0.7414,
                subspace_dim=request.subspace_dimension if request.subspace_dimension > 0 else 4,
                time_step=request.time_step if request.time_step > 0 else 0.2,
                shots=request.shots if request.shots > 0 else 1024,
            )
        except Exception as e:
            logger.exception("ExecuteQuantumKrylov failed for %s", request.molecule)
            return kubona_pb2.QuantumKrylovResponse(
                success=False,
                error_message=f"krylov solve failed: {e}",
            )
        # J5: solver attribution rides the response so downstream folds
        # can tell a real numerical solve from the analytical corridor
        # schedule (solve_quantum_krylov tags res["solver"]).
        # W1.2: refusal/heuristic paths carry None for uncomputed numbers
        # (proto doubles cannot be null) — map None to 0.0; the solver +
        # success/error_message carry the honest signal.
        return kubona_pb2.QuantumKrylovResponse(
            solver=res.get("solver", ""),
            success=res["success"],
            ground_state_energy=res.get("ground_state_energy") or 0.0,
            classical_fci_energy=res.get("classical_fci_energy") or 0.0,
            subspace_eigenvalues=res.get("subspace_eigenvalues") or [],
            subspace_dimension=res.get("subspace_dimension") or 0,
            meyer_wallach_entanglement=res.get("meyer_wallach") or 0.0,
            execution_time_ms=res["execution_time_ms"],
            error_message=res["error_message"],
        )

    def MitigateCircuit(self, request, context):
        logger.info(f"MitigateCircuit called with method={request.method}")
        scales = list(request.scale_factors) if request.scale_factors else [1.0, 1.5, 2.0]
        try:
            res = mitigate_circuit(
                qasm=request.qasm,
                method=request.method if request.method else "ZNE",
                scale_factors=scales,
                shots=request.shots if request.shots > 0 else 1024,
            )
        except Exception as e:
            logger.exception("MitigateCircuit failed with method=%s", request.method)
            return kubona_pb2.MitigationResponse(
                success=False,
                method_used=request.method if request.method else "ZNE",
                error_message=f"error mitigation failed: {e}",
            )
        return kubona_pb2.MitigationResponse(
            success=res["success"],
            unmitigated_value=res["unmitigated_value"],
            mitigated_value=res["mitigated_value"],
            error_bound=res["error_bound"] if res.get("error_bound") is not None else 0.0,
            method_used=res["method_used"],
            error_message=res["error_message"],
            sampling_overhead_kappa=float(res.get("sampling_overhead_kappa", 0.0)),
        )

    def RunExperiment(self, request, context):
        """Execute a bounded real experiment (VQE/PES) in this compute plane.

        The spec is a versioned JSON object; the closed dispatcher refuses
        unknown kinds and unsupported molecules instead of borrowing numbers.
        Provenance is written by the Go job/settlement rail that calls us.
        """
        import json as _json

        try:
            spec = _json.loads(request.spec_json or "{}")
        except Exception as e:
            return kubona_pb2.ExperimentResponse(
                ok=False, error_message=f"invalid experiment spec JSON: {e}", result_json="")
        if not isinstance(spec, dict):
            return kubona_pb2.ExperimentResponse(
                ok=False, error_message="experiment spec must be a JSON object", result_json="")
        kind = str(spec.get("kind") or "").strip().lower()
        try:
            if kind == "vqe":
                from experiments import run_vqe
                result = run_vqe(
                    molecule=str(spec.get("molecule") or "H2"),
                    bond_distance=float(spec.get("bond_distance", 0.7414)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 65536)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 120)),
                    full_space=bool(spec.get("full_space", False)),
                    full_space_pool=str(spec.get("full_space_pool") or "ceo"),
                    atom=spec.get("atom") or None,
                    basis=str(spec.get("basis") or "sto-3g"),
                    charge=int(spec.get("charge", 0)),
                    spin=int(spec.get("spin", 0)),
                    ncas=spec.get("ncas"),
                    nelecas=spec.get("nelecas"),
                )
            elif kind == "geometry_optimization":
                from experiments import run_geometry_optimization
                result = run_geometry_optimization(
                    molecule=str(spec.get("molecule") or "H2"),
                    initial_bond_distance=float(spec.get("initial_bond_distance", spec.get("bond_distance", 0.7414))),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 8192)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                )
            elif kind == "vqe_repeats":
                from experiments import run_repeated_vqe
                result = run_repeated_vqe(
                    molecule=str(spec.get("molecule") or "H2"),
                    bond_distance=float(spec.get("bond_distance", 0.7414)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 8192)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                    repeats=int(spec.get("repeats", 3)),
                )
            elif kind == "pes":
                from experiments import run_pes
                result = run_pes(
                    molecule=str(spec.get("molecule") or "H2"),
                    r_min=float(spec.get("r_min", 0.5)),
                    r_max=float(spec.get("r_max", 2.0)),
                    steps=int(spec.get("steps", 5)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 16384)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                    full_space=bool(spec.get("full_space", False)),
                    full_space_pool=str(spec.get("full_space_pool") or "ceo"),
                )
            elif kind == "calibration":
                from kubona.providers.qiskit_provider import QiskitProvider
                from kubona.schema import load_provider
                backend = str(spec.get("backend") or "").strip()
                if not backend:
                    return kubona_pb2.ExperimentResponse(
                        ok=False, error_message="calibration refresh requires a backend", result_json="")
                provider = QiskitProvider(load_provider("ibm-qiskit"), backend_name=backend)
                snapshot = provider.calibration_snapshot()
                if snapshot is None:
                    return kubona_pb2.ExperimentResponse(
                        ok=False, error_message="provider returned no calibration snapshot", result_json="")
                result = {
                    "success": True,
                    "kind": "calibration",
                    "result_version": "experiment-result-v1",
                    "backend": backend,
                    "provider": "ibm-qiskit",
                    "snapshot": snapshot,
                    "queue": provider.queue_snapshot(),
                }
            elif kind == "krylov":
                from krylov import solve_quantum_krylov
                result = solve_quantum_krylov(
                    molecule=str(spec.get("molecule") or "H2"),
                    bond_distance=float(spec.get("bond_distance", 0.7414)),
                    subspace_dim=int(spec.get("subspace_dimension", 4)),
                    time_step=float(spec.get("time_step", 0.2)),
                    shots=int(spec.get("shots", 1024)),
                )
            elif kind == "mitigate":
                from mitigation import mitigate_circuit
                result = mitigate_circuit(
                    qasm=str(spec.get("qasm") or ""),
                    method=str(spec.get("method") or "ZNE"),
                    scale_factors=spec.get("scale_factors") or [1.0, 1.5, 2.0],
                    shots=int(spec.get("shots", 1024)),
                )
            elif kind == "qaoa":
                from experiments import run_qaoa
                result = run_qaoa(
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 4096)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                    graph_seed=int(spec.get("graph_seed", 3)),
                )
            elif kind == "excited_state":
                from experiments import run_excited_state
                result = run_excited_state(
                    molecule=str(spec.get("molecule") or "H2"),
                    bond_distance=float(spec.get("bond_distance", 0.7414)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 16384)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                    penalty=float(spec.get("penalty", 10.0)),
                )
            elif kind == "reaction":
                from experiments import run_reaction_energy
                result = run_reaction_energy(
                    molecule=str(spec.get("molecule") or "H2"),
                    reactant_distance=float(spec.get("reactant_distance", 0.7414)),
                    product_distance=float(spec.get("product_distance", 10.0)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 8192)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 80)),
                )
            elif kind == "spin_chain":
                from lattice_experiment import run_spin_chain
                result = run_spin_chain(
                    model=str(spec.get("model") or "tfim"),
                    n_sites=int(spec.get("n_sites", 4)),
                    coupling=float(spec.get("coupling", 1.0)),
                    field=float(spec.get("field", 1.0)),
                    anisotropy=float(spec.get("anisotropy", 1.0)),
                    periodic=bool(spec.get("periodic", True)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 8192)),
                    seed=int(spec.get("seed", 7)),
                    max_iterations=int(spec.get("max_iterations", 160)),
                    target_error=float(spec.get("target_error", 1e-3)),
                    state_mode=str(spec.get("state_mode") or "vqe"),
                )
            elif kind == "spin_chain_hardware":
                from lattice_experiment import run_spin_chain_hardware
                result = run_spin_chain_hardware(
                    qasm=str(spec.get("qasm") or ""),
                    pauli_coefficients=spec.get("pauli_coefficients") or {},
                    n_sites=int(spec.get("n_sites", 0)),
                    energy_exact=float(spec.get("energy_exact", 0.0)),
                    model=str(spec.get("model") or "tfim"),
                    coupling=float(spec.get("coupling", 1.0)),
                    field=float(spec.get("field", 0.0)),
                    anisotropy=float(spec.get("anisotropy", 1.0)),
                    periodic=bool(spec.get("periodic", True)),
                    layers=int(spec.get("layers", 2)),
                    shots=int(spec.get("shots", 4096)),
                    seed=int(spec.get("seed", 7)),
                    target_error=float(spec.get("target_error", 1e-3)),
                    backend=spec.get("requested_backend"),
                    state_mode=str(spec.get("state_mode") or "vqe"),
                    source_job_id=str(spec.get("source_job_id") or ""),
                )
            elif kind == "lattice_dynamics":
                from lattice_experiment import run_lattice_dynamics
                result = run_lattice_dynamics(
                    rows=int(spec.get("rows", 2)),
                    cols=int(spec.get("cols", 2)),
                    coupling=float(spec.get("coupling", 1.0)),
                    initial_field=float(spec.get("initial_field", 1.0)),
                    quench_field=float(spec.get("quench_field", 0.2)),
                    times=spec.get("times"),
                    periodic=bool(spec.get("periodic", True)),
                )
            elif kind == "conjecture":
                from kubona.science.conjecture import discover_sequence_laws
                holdout = spec.get("holdout")
                result = discover_sequence_laws(
                    spec.get("values") or [],
                    holdout=None if holdout is None else int(holdout),
                    max_order=int(spec.get("max_order", 6)),
                    max_degree=int(spec.get("max_degree", 6)),
                    literature_search=bool(spec.get("literature_search", False)),
                )
            elif kind == "hardware_pes":
                from experiments import run_hardware_pes
                result = run_hardware_pes(
                    points=spec.get("points") or [],
                    molecule=str(spec.get("molecule") or "H2"),
                    shots=int(spec.get("shots", 4096)),
                    seed=int(spec.get("seed", 7)),
                    backend=spec.get("requested_backend"),
                    mitigation=spec.get("mitigation"),
                    mitigation_scales=spec.get("mitigation_scales"),
                    source_job_id=str(spec.get("source_job_id") or ""),
                )
            elif kind == "vqe_hardware":
                from experiments import run_vqe_hardware, run_vqe_hardware_loop
                if bool(spec.get("loop")):
                    result = run_vqe_hardware_loop(
                        theta=spec.get("theta"),
                        pauli_coefficients=spec.get("pauli_coefficients"),
                        layers=int(spec.get("layers", 2)),
                        shots=int(spec.get("shots", 4096)),
                        seed=int(spec.get("seed", 7)),
                        bond_distance=float(spec.get("bond_distance", 0.7414)),
                        fci_exact_energy=spec.get("fci_exact_energy"),
                        backend=spec.get("requested_backend"),
                        molecule=str(spec.get("molecule") or "H2"),
                        ansatz_qpy=spec.get("ansatz_qpy"),
                        constant_energy=float(spec.get("constant_energy", 0.0)),
                        max_iterations=int(spec.get("max_iterations", 8)),
                    )
                else:
                    result = run_vqe_hardware(
                        theta=spec.get("theta"),
                        pauli_coefficients=spec.get("pauli_coefficients"),
                        layers=int(spec.get("layers", 2)),
                        shots=int(spec.get("shots", 4096)),
                        seed=int(spec.get("seed", 7)),
                        bond_distance=float(spec.get("bond_distance", 0.7414)),
                        fci_exact_energy=spec.get("fci_exact_energy"),
                        backend=spec.get("requested_backend"),
                        molecule=str(spec.get("molecule") or "H2"),
                        ansatz_qasm=spec.get("qasm") or spec.get("ansatz_qasm"),
                        constant_energy=float(spec.get("constant_energy", 0.0)),
                        mitigation=spec.get("mitigation"),
                        mitigation_scales=spec.get("mitigation_scales"),
                    )
            elif kind == "qaoa_hardware":
                from experiments import run_qaoa_hardware, run_qaoa_hardware_loop
                graph = spec.get("graph") or {
                    "nodes": spec.get("nodes", 0),
                    "edges": spec.get("edges", []),
                }
                if bool(spec.get("loop")):
                    result = run_qaoa_hardware_loop(
                        nodes=int(graph.get("nodes", 0)),
                        edges=graph.get("edges") or [],
                        optimal_cut=int(spec.get("optimal_cut", 0)),
                        layers=int(spec.get("layers", 2)),
                        shots=int(spec.get("shots", 4096)),
                        seed=int(spec.get("seed", 7)),
                        max_iterations=int(spec.get("max_iterations", 8)),
                        backend=spec.get("requested_backend"),
                    )
                else:
                    result = run_qaoa_hardware(
                        qasm=spec.get("qasm") or "",
                        optimal_cut=int(spec.get("optimal_cut", 0)),
                        nodes=int(graph.get("nodes", 0)),
                        edges=graph.get("edges") or [],
                        shots=int(spec.get("shots", 4096)),
                        seed=int(spec.get("seed", 7)),
                        energy_exact=float(spec.get("energy_exact", 0.0)),
                        backend=spec.get("requested_backend"),
                    )
            else:
                return kubona_pb2.ExperimentResponse(
                    ok=False,
                    error_message=f"unknown experiment kind {kind!r}: use vqe, geometry_optimization, vqe_repeats, pes, qaoa, excited_state, reaction, spin_chain, lattice_dynamics, conjecture, spin_chain_hardware, vqe_hardware, or qaoa_hardware",
                    result_json="")
        except Exception as e:
            logger.exception("RunExperiment failed")
            return kubona_pb2.ExperimentResponse(
                ok=False, error_message=f"experiment failed: {e}", result_json="")
        return kubona_pb2.ExperimentResponse(
            ok=bool(result.get("success")),
            error_message=str(result.get("error") or result.get("error_message") or ""),
            result_json=_json.dumps(result),
        )

    def RunWorkflowStep(self, request, context):
        """Execute exactly one non-job workflow step for the Go orchestrator.

        The Go worker owns step sequencing, per-step budgets, retry policy, and
        compensation; this RPC never runs a whole workflow. Job steps are
        submitted through the Go jobs rail and do not use this handler.
        """
        import json as _json
        import uuid as _uuid

        from kubona.mcp.servers import execution as _exec
        from kubona.mcp.servers._mount import build_gateway
        from kubona.provenance import Ledger
        from kubona.workflows.engine import WorkflowEngine, parse_workflow

        try:
            label = (f"step:{getattr(request, 'tenant_id', '') or 'no-tenant'}:"
                     f"{getattr(request, 'session_id', '') or 'no-session'}")
            auth, refusal = _workflow_auth(label, tenant=getattr(request, "tenant_id", "") or "default")
            if auth is None:
                return kubona_pb2.WorkflowStepResponse(ok=False, error_message=refusal)
            if request.session_id and getattr(request, "session_verified", False):
                from kubona.mcp.servers import cost as _cost
                raw_ceiling = getattr(request, "session_budget_ceiling_usd", 0.0)
                ceiling_usd = float(raw_ceiling) if raw_ceiling and raw_ceiling > 0 else _exec.IMPLICIT_SESSION_CEILING_USD
                spent_usd = max(0.0, float(getattr(request, "session_spent_usd", 0.0) or 0.0))
                owner = f"kubonad-bridge:{label}"
                for store in (_exec._sessions, _cost._sessions):
                    store.sync_external_session(
                        session_id=request.session_id,
                        owner=owner,
                        budget_ceiling_usd=ceiling_usd,
                        spent_usd=spent_usd,
                        tenant_id=getattr(request, "tenant_id", "") or "",
                        spend_prepaid=True,
                    )
            raw_step = _json.loads(request.step_json or "{}")
            if not isinstance(raw_step, dict) or not raw_step.get("id") or not raw_step.get("tool"):
                return kubona_pb2.WorkflowStepResponse(ok=False, error_message="step needs id and tool")
            step_id = str(raw_step["id"])
            wf = {
                "name": "step:" + step_id,
                "version": 1,
                "workflow_id": request.workflow_id or _uuid.uuid4().hex[:16],
                "steps": [raw_step],
            }
            parsed = parse_workflow(wf)
            inputs = _json.loads(request.inputs_json) if request.inputs_json else {}
            bridge_token = os.getenv("KUBONA_BRIDGE_TOKEN", "")
            token = request.token or bridge_token
            gw = build_gateway(require_auth=True, auth=auth)
            res = WorkflowEngine(gateway=gw, ledger=Ledger()).run(
                parsed, token=token, session_id=request.session_id or None,
                inputs=inputs, dry_run=False, force=True,
            )
            if not res.get("ok"):
                return kubona_pb2.WorkflowStepResponse(
                    ok=False, step_id=step_id,
                    error_message=str(res.get("error", "workflow step failed")),
                    step_records=list(res.get("step_records", [])),
                    spent_usd=float(res.get("spent_usd", 0.0)),
                )
            last = res.get("last_results") or {}
            result = last.get(step_id, {})
            return kubona_pb2.WorkflowStepResponse(
                ok=True, step_id=step_id,
                result_json=_json.dumps(result),
                spent_usd=float(res.get("spent_usd", 0.0)),
                step_records=list(res.get("step_records", [])),
            )
        except Exception as e:
            logger.exception("RunWorkflowStep failed")
            return kubona_pb2.WorkflowStepResponse(ok=False, error_message=f"workflow step failed: {e}")

    def WorkflowPlan(self, request, context):
        import json as _json

        from kubona.mcp.servers._mount import build_gateway
        from kubona.provenance import Ledger
        from kubona.workflows.engine import WorkflowEngine, parse_workflow

        logger.info("WorkflowPlan called for session %s", request.session_id)
        try:
            # Authenticated planning: no unauthenticated compute, even dry-run.
            # The plan never touches tools, but the transport bearer plus a
            # configured bridge token are still required to reach it.
            auth, refusal = _workflow_auth(
                f"plan:{getattr(request, 'session_id', '') or 'no-session'}")
            if auth is None:
                return kubona_pb2.WorkflowPlanResponse(
                    ok=False,
                    error_message=refusal,
                    workflow_id="",
                    planned_steps_json="",
                    unresolved_references=[],
                    compensations_planned_json="",
                )
            raw_wf = _json.loads(request.workflow_json)
            wf = parse_workflow(raw_wf)
            inputs = _json.loads(request.inputs_json) if request.inputs_json else {}
            gw = build_gateway(require_auth=True, auth=auth)
            ledger = Ledger()
            engine = WorkflowEngine(gateway=gw, ledger=ledger)
            res = engine.run(wf, token=os.getenv("KUBONA_BRIDGE_TOKEN", ""),
                             session_id=request.session_id or None,
                             inputs=inputs, dry_run=True, force=True)
            return kubona_pb2.WorkflowPlanResponse(
                ok=bool(res.get("ok", False)),
                error_message="",
                workflow_id=str(res.get("workflow_id", wf.workflow_id)),
                planned_steps_json=_json.dumps(res.get("steps", [])),
                unresolved_references=list(res.get("unresolved", [])),
                compensations_planned_json=_json.dumps(res.get("compensations_planned", [])),
            )
        except Exception as e:
            logger.exception("WorkflowPlan failed")
            return kubona_pb2.WorkflowPlanResponse(
                ok=False,
                error_message=f"workflow plan failed: {e}",
                workflow_id="",
                planned_steps_json="",
                unresolved_references=[],
                compensations_planned_json="",
            )

    def WorkflowRun(self, request, context):
        import json as _json

        from kubona.mcp.servers import execution as _exec
        from kubona.mcp.servers._mount import build_gateway
        from kubona.provenance import Ledger
        from kubona.workflows.engine import WorkflowEngine, parse_workflow

        logger.info("WorkflowRun called for session %s", request.session_id)
        try:
            label = (f"run:{getattr(request, 'tenant_id', '') or 'no-tenant'}:"
                     f"{getattr(request, 'session_id', '') or 'no-session'}")
            # Authenticate before mirroring any Go session state.
            auth, refusal = _workflow_auth(label, tenant=getattr(request, "tenant_id", "") or "default")
            if auth is None:
                return kubona_pb2.WorkflowRunResponse(
                    ok=False,
                    error_message=refusal,
                    workflow_id="",
                )
            if request.session_id and getattr(request, "session_verified", False):
                from kubona.mcp.servers import cost as _cost
                raw_ceiling = getattr(request, "session_budget_ceiling_usd", 0.0)
                ceiling_usd = float(raw_ceiling) if raw_ceiling and raw_ceiling > 0 else _exec.IMPLICIT_SESSION_CEILING_USD
                spent_usd = max(0.0, float(getattr(request, "session_spent_usd", 0.0) or 0.0))
                # The workflow gateway authenticates as kubonad-bridge:<label>;
                # the mirrored session must be owned by that principal or
                # submit_job refuses it as an unknown session. Mark it
                # Go-prepaid so the Python rail never double-charges it.
                owner = f"kubonad-bridge:{label}"
                for store in (_exec._sessions, _cost._sessions):
                    store.sync_external_session(
                        session_id=request.session_id,
                        owner=owner,
                        budget_ceiling_usd=ceiling_usd,
                        spent_usd=spent_usd,
                        tenant_id=getattr(request, "tenant_id", "") or "",
                        spend_prepaid=True,
                    )

            raw_wf = _json.loads(request.workflow_json)
            wf = parse_workflow(raw_wf)
            inputs = _json.loads(request.inputs_json) if request.inputs_json else {}
            bridge_token = os.getenv("KUBONA_BRIDGE_TOKEN", "")
            token = request.token or bridge_token
            gw = build_gateway(require_auth=True, auth=auth)
            ledger = Ledger()
            engine = WorkflowEngine(gateway=gw, ledger=ledger)
            res = engine.run(wf, token=token, session_id=request.session_id or None,
                             inputs=inputs, dry_run=False, force=bool(request.force))
            if not res.get("ok"):
                return kubona_pb2.WorkflowRunResponse(
                    ok=False,
                    error_message=str(res.get("error", "workflow execution failed")),
                    workflow_id=str(res.get("workflow_id", wf.workflow_id)),
                    steps_completed=0,
                    step_records=list(res.get("step_records", [])),
                    last_results_json=_json.dumps(res.get("last_results", {})),
                    spent_usd=float(res.get("spent_usd", 0.0)),
                    compensations_json=_json.dumps(res.get("compensations", [])),
                )
            return kubona_pb2.WorkflowRunResponse(
                ok=True,
                error_message="",
                workflow_id=str(res.get("workflow_id", wf.workflow_id)),
                steps_completed=int(res.get("steps", 0)),
                step_records=list(res.get("step_records", [])),
                last_results_json=_json.dumps(res.get("last_results", {})),
                spent_usd=float(res.get("spent_usd", 0.0)),
                compensations_json=_json.dumps(res.get("compensations", [])),
            )
        except Exception as e:
            logger.exception("WorkflowRun failed")
            return kubona_pb2.WorkflowRunResponse(
                ok=False,
                error_message=f"workflow run failed: {e}",
                workflow_id="",
                steps_completed=0,
                step_records=[],
                last_results_json="",
                spent_usd=0.0,
                compensations_json="",
            )


def serve(port: int = 50051, socket_path: str | None = None,
          bind: str | None = None, token: str | None = None):
    bind = bind if bind is not None else os.getenv("KUBONA_BRIDGE_BIND", "127.0.0.1")
    token = token if token is not None else os.getenv("KUBONA_BRIDGE_TOKEN", "")
    sock = socket_path or os.getenv("KUBONA_BRIDGE_SOCKET")

    if not token and not sock and bind not in ("127.0.0.1", "localhost", "::1"):
        raise SystemExit(
            "refusing non-loopback bridge without KUBONA_BRIDGE_TOKEN: "
            "set the token (compose does) or bind 127.0.0.1"
        )

    # P0 confinement: in production a TCP bridge is refused outright — the
    # only production transport is UDS-exclusive (both ends already support
    # it: the socket branch below and kubonad's KUBONA_BRIDGE_SOCKET dial).
    # Loopback TCP + token remains allowed for dev/CI.
    if os.getenv("KUBONA_ENV") == "production" and not sock and port > 0:
        raise SystemExit(
            "refusing TCP bridge in KUBONA_ENV=production: use UDS-exclusive "
            "mode (KUBONA_BRIDGE_SOCKET) — loopback+token is dev/CI only"
        )

    interceptors = [_AuthInterceptor(token)] if token else []
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8),
                         interceptors=interceptors)
    kubona_pb2_grpc.add_QuantumBridgeServiceServicer_to_server(QuantumBridgeServicer(), server)

    if sock:
        if sock.startswith("unix://"):
            sock = sock[7:]
        sock_dir = os.path.dirname(os.path.abspath(sock))
        if sock_dir:
            os.makedirs(sock_dir, exist_ok=True, mode=0o700)
        if os.path.exists(sock):
            try:
                os.unlink(sock)
            except OSError:
                pass
        # UDS mode is exclusive: do not also open a TCP port (the old
        # behaviour left an unauthenticated TCP door beside the 0600 socket).
        old_umask = os.umask(0o077)
        try:
            server.add_insecure_port(f"unix://{sock}")
        finally:
            os.umask(old_umask)
        logger.info("Bridge transport mode: uds-exclusive (unix://%s, TCP disabled, token %s)",
                    sock, 'set' if token else 'NOT SET')
        port = 0

    if port > 0:
        server.add_insecure_port(f"{bind}:{port}")
        logger.info("Bridge transport mode: loopback-tcp (%s:%d, token %s)",
                    bind, port, 'set' if token else 'NOT SET: loopback/dev only')

    server.start()

    if sock and os.path.exists(sock):
        try:
            os.chmod(sock, 0o600)
        except OSError:
            pass

    return server

if __name__ == "__main__":
    server = serve()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
