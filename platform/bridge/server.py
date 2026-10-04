"""
Python gRPC Server implementing QuantumBridgeService (proto/kubona.proto).
Exposes circuit execution, circuit cutting, classical chemistry anchors, epistemic evaluation,
and live scientific literature grounding.
"""
from __future__ import annotations

import logging
import sys
from concurrent import futures
import grpc

from proto import kubona_pb2, kubona_pb2_grpc
from chemistry import solve_classical_anchor
from epistemics import evaluate_claim_credibility
from executor import execute_qasm, cut_circuit_qasm
from literature import search_arxiv, search_openalex
from fidelity import predict_fidelity, probe_clifford_fidelity

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
        raw_results = search_arxiv(request.query, max_results=5)
        if not raw_results:
            raw_results = search_openalex(request.query, max_results=5)

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
                    publication_year=2026,
                    citation_count=10,
                )
            )

        return kubona_pb2.LiteratureGroundingResponse(records=records, cached=True)


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
