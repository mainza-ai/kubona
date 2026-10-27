"""
Python gRPC Server implementing QuantumBridgeService (proto/kubona.proto).
Exposes circuit execution, circuit cutting, classical chemistry anchors, epistemic evaluation,
and live scientific literature grounding.
"""
from __future__ import annotations

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


class _BridgeTokenAuth:
    """Authenticate the bridge-to-kubonad transport token as an operator.

    The bridge is a private compute plane. kubonad is the only intended
    caller, and it presents KUBONA_BRIDGE_TOKEN. This is intentionally
    narrow: no user tokens, no anonymous principal.
    """

    def __init__(self, token: str):
        self._token = token

    def verify(self, token: str):
        from kubona.core.auth import AuthError, Principal
        if not self._token or not hmac.compare_digest(token or "", self._token):
            raise AuthError("invalid or missing bridge token")
        return Principal(sub="kubonad-bridge", role="operator", scopes=("*",))


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
        return continuation(handler_call_details)


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
                raw_ceiling = getattr(request, "session_budget_ceiling_usd", 0.0)
                ceiling_usd = float(raw_ceiling) if raw_ceiling and raw_ceiling > 0 else _exec.IMPLICIT_SESSION_CEILING_USD
                spent_usd = max(0.0, float(getattr(request, "session_spent_usd", 0.0) or 0.0))
                _exec._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                )
                from kubona.mcp.servers import cost as _cost
                _cost._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                )

            caller_jurisdictions = list(getattr(request, "caller_jurisdictions", None) or [])
            caller_export_class = getattr(request, "caller_export_class", "") or "any"
            require_review = bool(getattr(request, "require_review", False))

            # wait is forced async for QPU inside submit_job; the bridge
            # never blocks on hardware. Seed derives from the idempotency
            # key: same key -> same seed -> reproducible retry.
            seed = int(_hl.sha256(request.idempotency_key.encode()).hexdigest()[:8], 16) % (2 ** 31)
            out = _exec.submit_job(
                circuit_qasm=request.qasm, provider=request.provider or "local-simulator",
                shots=request.shots if request.shots > 0 else 1024,
                session_id=request.session_id or None,
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
            st = _exec.get_job_status(request.job_id)
            if not st.get("ok"):
                return kubona_pb2.QuantumJobStatusResponse(
                    ok=False, error_message=str(st.get("error", "unknown job")), job_id=request.job_id,
                    status="", result_json="", estimate_json="")
            res = _exec.get_result(request.job_id)
            return kubona_pb2.QuantumJobStatusResponse(
                ok=True, error_message="", job_id=request.job_id,
                status=st.get("status", ""),
                result_json=_json.dumps(res.get("result", {})) if res.get("ok") else "",
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

    def WorkflowPlan(self, request, context):
        import json as _json

        from kubona.mcp.servers._mount import build_gateway
        from kubona.provenance import Ledger
        from kubona.workflows.engine import WorkflowEngine, parse_workflow

        logger.info("WorkflowPlan called for session %s", request.session_id)
        try:
            raw_wf = _json.loads(request.workflow_json)
            wf = parse_workflow(raw_wf)
            inputs = _json.loads(request.inputs_json) if request.inputs_json else {}
            gw = build_gateway(require_auth=False)
            ledger = Ledger()
            engine = WorkflowEngine(gateway=gw, ledger=ledger)
            res = engine.run(wf, token="dev-plan-token", session_id=request.session_id or None,
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
            if request.session_id and getattr(request, "session_verified", False):
                raw_ceiling = getattr(request, "session_budget_ceiling_usd", 0.0)
                ceiling_usd = float(raw_ceiling) if raw_ceiling and raw_ceiling > 0 else _exec.IMPLICIT_SESSION_CEILING_USD
                spent_usd = max(0.0, float(getattr(request, "session_spent_usd", 0.0) or 0.0))
                _exec._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                )
                from kubona.mcp.servers import cost as _cost
                _cost._sessions.sync_external_session(
                    session_id=request.session_id,
                    budget_ceiling_usd=ceiling_usd,
                    spent_usd=spent_usd,
                    tenant_id=getattr(request, "tenant_id", "") or "",
                )

            raw_wf = _json.loads(request.workflow_json)
            wf = parse_workflow(raw_wf)
            inputs = _json.loads(request.inputs_json) if request.inputs_json else {}
            bridge_token = os.getenv("KUBONA_BRIDGE_TOKEN", "")
            if bridge_token:
                token = request.token or bridge_token
                gw = build_gateway(require_auth=True, auth=_BridgeTokenAuth(bridge_token))
            else:
                if os.getenv("KUBONA_ENV") == "production":
                    return kubona_pb2.WorkflowRunResponse(
                        ok=False,
                        error_message=("bridge token is not configured: refusing "
                                       "unauthenticated workflow execution"),
                        workflow_id="",
                    )
                token = request.token or "dev-token"
                gw = build_gateway(require_auth=False)
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
        logger.info(f"Binding Python Quantum Bridge to unix://{sock} (TCP disabled)")
        port = 0

    if port > 0:
        server.add_insecure_port(f"{bind}:{port}")
        logger.info(f"Starting Python Quantum Bridge on {bind}:{port} "
                    f"(token={'set' if token else 'NOT SET: loopback/dev only'})")

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
