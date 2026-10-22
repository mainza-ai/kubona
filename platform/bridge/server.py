"""
Python gRPC Server implementing QuantumBridgeService (proto/kubona.proto).
Exposes circuit execution, circuit cutting, classical chemistry anchors, epistemic evaluation,
and live scientific literature grounding.
"""
from __future__ import annotations

import logging
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
            res = execute_qasm(
                qasm=request.qasm,
                shots=request.shots if request.shots > 0 else 1024,
                requested_backend=request.requested_backend,
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
            res = cut_circuit_qasm(request.qasm, request.max_subcircuit_qubits)
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


def serve(port: int = 50051):
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    kubona_pb2_grpc.add_QuantumBridgeServiceServicer_to_server(QuantumBridgeServicer(), server)
    server.add_insecure_port(f"[::]:{port}")
    logger.info(f"Starting Python Quantum Bridge server on port {port}...")
    server.start()
    return server


if __name__ == "__main__":
    server = serve()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
