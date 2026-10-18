#!/usr/bin/env python3
"""SC-06: Quantum Krylov Subspace Diagonalization Live Audit.
Tests real-time evolution Krylov subspace solver, generalized eigenvalue problem,
variational bound enforcement, and simulator-only gating against Python Bridge (:50051)
and kubonad (:8080).
"""
import sys
import grpc
import requests

sys.path.insert(0, "platform/bridge/proto")
import kubona_pb2, kubona_pb2_grpc

GRPC_TARGET = "localhost:50051"
API_BASE = "http://127.0.0.1:8080"

def run_sc06():
    print("=== [SC-06] Testing Quantum Krylov Subspace Diagonalization ===")
    
    channel = grpc.insecure_channel(GRPC_TARGET)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    # 1. H2 Equilibrium (R=0.7414 A) with Krylov subspace
    req = kubona_pb2.QuantumKrylovRequest(
        molecule="H2",
        bond_distance=0.7414,
        subspace_dimension=4,
        time_step=0.2,
        shots=1024,
        requested_backend="local_simulator"
    )
    resp = stub.ExecuteQuantumKrylov(req)
    assert resp.success, f"Krylov solve failed: {resp.error_message}"
    assert resp.subspace_dimension in (2, 4), f"Expected effective subspace dimension 2 or 4, got {resp.subspace_dimension}"
    assert len(resp.subspace_eigenvalues) == resp.subspace_dimension
    assert resp.solver == "numerical-krylov"
    
    # Eigenvalues must be monotonically increasing
    for i in range(len(resp.subspace_eigenvalues) - 1):
        assert resp.subspace_eigenvalues[i] <= resp.subspace_eigenvalues[i+1] + 1e-6, "Eigenvalues not sorted"
        
    # Variational Bound: E_krylov >= E_FCI (within numerical precision)
    assert resp.ground_state_energy >= resp.classical_fci_energy - 1e-4, (
        f"Variational violation: Krylov ({resp.ground_state_energy}) < FCI ({resp.classical_fci_energy})"
    )
    
    print(f"✓ gRPC Krylov H2 (R=0.7414 A): E_0={resp.ground_state_energy:.6f} Ha, "
          f"FCI={resp.classical_fci_energy:.6f} Ha, ΔE={(resp.ground_state_energy - resp.classical_fci_energy)*1000:.3f} mHa, "
          f"dim={resp.subspace_dimension}, solver={resp.solver}, time={resp.execution_time_ms:.1f}ms")

    # 2. LiH: Production Bar W1.2 fail-closed verification (refuses synthetic corridor)
    lih_req = kubona_pb2.QuantumKrylovRequest(
        molecule="LiH",
        bond_distance=1.595,
        subspace_dimension=4,
        time_step=0.2,
        shots=1024,
        requested_backend="local_simulator"
    )
    lih_resp = stub.ExecuteQuantumKrylov(lih_req)
    assert not lih_resp.success, f"Expected non-H2 Krylov to refuse without heuristic flag, got {lih_resp.success}"
    assert "refusing analytical-sequence for non-H2" in lih_resp.error_message
    print(f"✓ Fail-closed verified on non-H2 Krylov (W1.2 anti-fiction): {lih_resp.error_message.strip()[:65]}")

    # 3. Test through kubonad orchestrator: POST /api/krylov/solve
    print("\n--- Testing via kubonad orchestrator :8080 ---")
    res = requests.post(f"{API_BASE}/api/krylov/solve", json={
        "molecule": "H2",
        "bond_distance": 0.7414,
        "subspace_dimension": 4,
        "time_step": 0.2,
        "shots": 1024
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 200, f"Orchestrator Krylov failed: {res.status_code} {res.text}"
    orch_data = res.json()
    assert orch_data["success"] is True, f"Krylov solve returned error: {orch_data}"
    assert orch_data["subspace_dimension"] in (2, 4)
    print(f"✓ Orchestrator Krylov endpoint verified: E_0={orch_data['ground_state_energy']:.6f} Ha, "
          f"eigenvalues={[round(x, 4) for x in orch_data['subspace_eigenvalues']]}")

    # 4. Fail-closed test: hardware backend must refuse (quantum-washing protection W1.8)
    hw_req = kubona_pb2.QuantumKrylovRequest(
        molecule="H2",
        bond_distance=0.7414,
        requested_backend="ibm_sherbrooke"
    )
    hw_resp = stub.ExecuteQuantumKrylov(hw_req)
    assert not hw_resp.success, f"Expected refusal for hardware backend on Krylov, got success={hw_resp.success}"
    assert "not a simulator" in hw_resp.error_message
    print(f"✓ Fail-closed verified: hardware backend on Krylov refused cleanly: {hw_resp.error_message.strip()[:65]}")

    print("=== [SC-06] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc06()
