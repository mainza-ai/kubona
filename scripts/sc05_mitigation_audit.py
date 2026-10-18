#!/usr/bin/env python3
"""SC-05: Zero-Noise Extrapolation (ZNE) & Error Mitigation Live Audit.
Tests ZNE, PEC, and CDR error mitigation techniques against Python Bridge
gRPC (:50051) and kubonad orchestrator (:8080).
"""
import sys
import grpc
import requests

sys.path.insert(0, "platform/bridge/proto")
import kubona_pb2, kubona_pb2_grpc

GRPC_TARGET = "localhost:50051"
API_BASE = "http://127.0.0.1:8080"

def run_sc05():
    print("=== [SC-05] Testing Quantum Error Mitigation (ZNE, PEC, CDR) ===")
    
    channel = grpc.insecure_channel(GRPC_TARGET)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0],q[1];
rz(0.785398) q[1];
cx q[0],q[1];
h q[0];
measure q[0] -> c[0];
measure q[1] -> c[1];
"""

    # 1. Test ZNE via gRPC
    zne_req = kubona_pb2.MitigationRequest(
        qasm=qasm,
        method="ZNE",
        scale_factors=[1.0, 1.5, 2.0],
        shots=1024
    )
    zne_resp = stub.MitigateCircuit(zne_req)
    assert zne_resp.success, f"ZNE failed: {zne_resp.error_message}"
    assert zne_resp.method_used == "ZNE"
    assert zne_resp.error_bound >= 0.0, f"Error bound must be non-negative: {zne_resp.error_bound}"
    print(f"✓ gRPC ZNE Mitigation: unmitigated={zne_resp.unmitigated_value:.4f}, "
          f"mitigated={zne_resp.mitigated_value:.4f}, error_bound={zne_resp.error_bound:.4f}")

    # 2. Test PEC via gRPC
    pec_req = kubona_pb2.MitigationRequest(
        qasm=qasm,
        method="PEC",
        scale_factors=[],
        shots=1024
    )
    pec_resp = stub.MitigateCircuit(pec_req)
    assert pec_resp.success, f"PEC failed: {pec_resp.error_message}"
    assert pec_resp.method_used == "PEC"
    assert pec_resp.sampling_overhead_kappa >= 0.0, f"PEC gamma must be >= 0.0, got {pec_resp.sampling_overhead_kappa}"
    print(f"✓ gRPC PEC Mitigation: unmitigated={pec_resp.unmitigated_value:.4f}, "
          f"mitigated={pec_resp.mitigated_value:.4f}, gamma_overhead={pec_resp.sampling_overhead_kappa:.4f}")

    # 3. Test CDR via gRPC
    cdr_req = kubona_pb2.MitigationRequest(
        qasm=qasm,
        method="CDR",
        scale_factors=[],
        shots=1024
    )
    cdr_resp = stub.MitigateCircuit(cdr_req)
    assert cdr_resp.success, f"CDR failed: {cdr_resp.error_message}"
    assert cdr_resp.method_used == "CDR"
    print(f"✓ gRPC CDR Mitigation: unmitigated={cdr_resp.unmitigated_value:.4f}, "
          f"mitigated={cdr_resp.mitigated_value:.4f}")

    # 4. Test via kubonad orchestrator: POST /api/mitigation/apply
    print("\n--- Testing via kubonad orchestrator :8080 ---")
    res = requests.post(f"{API_BASE}/api/mitigation/apply", json={
        "qasm": qasm,
        "method": "ZNE",
        "scale_factors": [1.0, 2.0, 3.0],
        "shots": 1024
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 200, f"Orchestrator mitigation failed: {res.status_code} {res.text}"
    orch_data = res.json()
    assert orch_data["success"] is True, f"Orchestrator returned unsuccessful result: {orch_data}"
    assert orch_data["method_used"] == "ZNE"
    print(f"✓ Orchestrator ZNE verified: unmitigated={orch_data['unmitigated_value']:.4f}, "
          f"mitigated={orch_data['mitigated_value']:.4f}")

    # 5. Fail-closed check: invalid scale factors (collinear or negative)
    bad_zne_req = kubona_pb2.MitigationRequest(
        qasm=qasm,
        method="ZNE",
        scale_factors=[-1.0, -1.0],
        shots=1024
    )
    bad_zne_resp = stub.MitigateCircuit(bad_zne_req)
    assert not bad_zne_resp.success, f"Expected fail-closed for negative scale factors, got {bad_zne_resp.success}"
    print(f"✓ Fail-closed verified on invalid scale factors: {bad_zne_resp.error_message.strip()[:65]}")

    print("=== [SC-05] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc05()
