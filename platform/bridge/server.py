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
from chemistry import solve_classical_anchor
from epistemics import evaluate_claim_credibility
from executor import execute_qasm, cut_circuit_qasm
from literature import search_arxiv, search_openalex
from fidelity import predict_fidelity, probe_clifford_fidelity
from krylov import solve_quantum_krylov
from mitigation import mitigate_circuit

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("kubona-bridge")


class QuantumBridgeServicer(kubona_pb2_grpc.QuantumBridgeServiceServicer):
    def ExecuteCircuit(self, request, context):
        logger.info(f"ExecuteCircuit called for Job {request.job_id}")
        res = execute_qasm(
            qasm=request.qasm,
            shots=request.shots if request.shots > 0 else 1024,
            requested_backend=request.requested_backend,
            allow_fallback=request.allow_classical_fallback,
            caller_jurisdictions=list(request.caller_jurisdictions),
            caller_export_class=request.caller_export_class or "any",
            export_reviewed=request.export_reviewed,
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
        )

    def CutCircuit(self, request, context):
        logger.info(f"CutCircuit called with max_qubits={request.max_subcircuit_qubits}")
        res = cut_circuit_qasm(request.qasm, request.max_subcircuit_qubits)
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
        )

    def ComputeClassicalAnchor(self, request, context):
        logger.info(f"ComputeClassicalAnchor called for {request.molecule} at R={request.bond_distance}")
        try:
            res = solve_classical_anchor(request.molecule, request.bond_distance)
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

    def EvaluateWavefunction(self, request, context):
        logger.info(f"EvaluateWavefunction called for Claim {request.claim_id}")
        res = evaluate_claim_credibility(
            claim_id=request.claim_id,
            statement=request.statement,
            quantum_value=request.quantum_value,
            classical_anchor_value=request.classical_anchor_value,
            source_platforms=list(request.source_platforms),
            total_engagement=request.total_engagement,
            contradiction_signal=request.contradiction_signal,
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
        )

    def GroundLiterature(self, request, context):
        logger.info(f"GroundLiterature called for query '{request.query}'")
        limit = request.limit if request.limit > 0 else 5
        raw_results = search_arxiv(request.query, max_results=limit)
        if not raw_results:
            raw_results = search_openalex(request.query, max_results=limit)

        records = []
        for r in raw_results:
            authors_str = ", ".join(r.get("authors", [])) if isinstance(r.get("authors"), list) else str(r.get("authors", "Unknown"))
            records.append(
                kubona_pb2.LiteratureRecord(
                    id=r.get("id", ""),
                    title=r.get("title", ""),
                    authors=authors_str,
                    abstract_text=r.get("abstract", ""),
                    url=r.get("id", ""),
                    publication_year=int(r.get("year", 0) or 0),
                    citation_count=int(r.get("cited_by_count", 0) or 0),
                )
            )

        # cached=True only when every record came from the local cache —
        # a live or fallback fetch is never labeled cached.
        all_cached = bool(records) and all(bool(r.get("cached", False)) for r in raw_results)
        return kubona_pb2.LiteratureGroundingResponse(records=records, cached=all_cached)

    def ExecuteQuantumKrylov(self, request, context):
        logger.info(f"ExecuteQuantumKrylov called for {request.molecule} at R={request.bond_distance}")
        res = solve_quantum_krylov(
            molecule=request.molecule if request.molecule else "H2",
            bond_distance=request.bond_distance if request.bond_distance > 0 else 0.7414,
            subspace_dim=request.subspace_dimension if request.subspace_dimension > 0 else 4,
            time_step=request.time_step if request.time_step > 0 else 0.2,
            shots=request.shots if request.shots > 0 else 1024,
        )
        return kubona_pb2.QuantumKrylovResponse(
            success=res["success"],
            ground_state_energy=res["ground_state_energy"],
            classical_fci_energy=res["classical_fci_energy"],
            subspace_eigenvalues=res["subspace_eigenvalues"],
            subspace_dimension=res["subspace_dimension"],
            meyer_wallach_entanglement=res["meyer_wallach"],
            execution_time_ms=res["execution_time_ms"],
            error_message=res["error_message"],
        )

    def MitigateCircuit(self, request, context):
        logger.info(f"MitigateCircuit called with method={request.method}")
        scales = list(request.scale_factors) if request.scale_factors else [1.0, 1.5, 2.0]
        res = mitigate_circuit(
            qasm=request.qasm,
            method=request.method if request.method else "ZNE",
            scale_factors=scales,
            shots=request.shots if request.shots > 0 else 1024,
        )
        return kubona_pb2.MitigationResponse(
            success=res["success"],
            unmitigated_value=res["unmitigated_value"],
            mitigated_value=res["mitigated_value"],
            error_bound=res["error_bound"],
            method_used=res["method_used"],
            error_message=res["error_message"],
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
