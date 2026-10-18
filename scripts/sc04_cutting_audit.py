#!/usr/bin/env python3
"""SC-04: Circuit Cutting (QPD) & Wire Recombination Live Audit.
Tests circuit cutting via gRPC bridge (CutCircuit) and PennyLane QPD
exact expectation value reconstruction.
"""
import sys
import grpc
import numpy as np

sys.path.insert(0, "platform/bridge/proto")
sys.path.insert(0, "platform/bridge")
import kubona_pb2, kubona_pb2_grpc
from cutting import cut_and_reconstruct_expectation

GRPC_TARGET = "localhost:50051"

def run_sc04():
    print("=== [SC-04] Testing Circuit Cutting (QPD) & Wire Recombination ===")
    
    channel = grpc.insecure_channel(GRPC_TARGET)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    # 1. 4-Qubit Entangled Circuit cut into max 2-qubit fragments
    qasm_4q = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[4];
creg c[4];
h q[0];
h q[2];
cx q[0],q[1];
cx q[1],q[2];
cx q[2],q[3];
measure q[0] -> c[0];
measure q[1] -> c[1];
measure q[2] -> c[2];
measure q[3] -> c[3];
"""
    req = kubona_pb2.CircuitCutRequest(
        qasm=qasm_4q,
        max_subcircuit_qubits=2
    )
    resp = stub.CutCircuit(req)
    assert resp.success, f"CutCircuit failed: {resp.error_message}"
    assert len(resp.fragments) >= 2, f"Expected >=2 fragments, got {len(resp.fragments)}"
    assert resp.total_cuts >= 1, f"Expected cuts >= 1, got {resp.total_cuts}"
    assert resp.sampling_overhead_kappa >= 1.0, f"Overhead kappa must be >= 1.0, got {resp.sampling_overhead_kappa}"

    print(f"✓ gRPC CutCircuit partitioned 4q circuit into {len(resp.fragments)} fragments: "
          f"cuts={resp.total_cuts}, sampling_overhead_kappa={resp.sampling_overhead_kappa:.2f}")

    # Verify that each fragment executes cleanly on AerSimulator
    for f in resp.fragments:
        assert f.qasm, f"Fragment {f.fragment_id} has empty QASM"
        exec_req = kubona_pb2.CircuitExecutionRequest(
            qasm=f.qasm,
            requested_backend="qiskit-aer-simulator",
            shots=512
        )
        exec_resp = stub.ExecuteCircuit(exec_req)
        assert exec_resp.success, f"Fragment {f.fragment_id} execution failed: {exec_resp.error_message}"
        print(f"  ✓ Subcircuit Fragment {f.fragment_id} ({len(exec_resp.counts)} bitstrings) verified on AerSimulator")

    # 2. Test PennyLane QPD expectation value reconstruction
    print("\n--- Testing PennyLane QPD Wire Cut Reconstruction ---")
    for nq, wire in [(3, 1), (4, 1), (4, 2)]:
        rotations = [0.3 * (i + 1) for i in range(nq)]
        res = cut_and_reconstruct_expectation(
            n_qubits=nq,
            rotations=rotations,
            cut_wire=wire
        )
        uncut = res["uncut_expectation"]
        cut_reconstructed = res["cut_reconstructed_expectation"]
        err = res["reconstruction_error"]
        assert err < 1e-4, f"Reconstruction error too high ({err}): uncut={uncut}, cut={cut_reconstructed}"
        print(f"✓ PennyLane QPD {nq}Q cut at wire {wire}: uncut={uncut:.6f}, "
              f"reconstructed={cut_reconstructed:.6f}, err={err:.2e}, overhead={res['overhead_scale']}x")

    # 3. Fail-closed test: excessive cuts exceeding kappa cap
    # Construct a circuit with many cross-partition CNOTs exceeding MAX_SAMPLING_OVERHEAD_KAPPA_V1
    deep_entangled_qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[6];
creg c[6];
"""
    for _ in range(8):
        deep_entangled_qasm += "cx q[0],q[3];\ncx q[1],q[4];\ncx q[2],q[5];\n"
    deep_entangled_qasm += "measure q[0] -> c[0];\n"

    excessive_req = kubona_pb2.CircuitCutRequest(
        qasm=deep_entangled_qasm,
        max_subcircuit_qubits=2
    )
    excessive_resp = stub.CutCircuit(excessive_req)
    assert not excessive_resp.success, (
        f"Expected failure for excessive cuts, got success={excessive_resp.success}, "
        f"kappa={excessive_resp.sampling_overhead_kappa}"
    )
    print(f"✓ Fail-closed verified on excessive cuts exceeding kappa cap: {excessive_resp.error_message.strip()[:65]}")

    print("=== [SC-04] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc04()
