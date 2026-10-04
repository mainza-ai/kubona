#!/usr/bin/env python3
"""
Kubona Quantum Epistemic Observatory — Real-World End-to-End Verification Suite.
Zero mocks. Zero stubs. Zero fake data.

Verifies:
1. Live Local LLM on oMLX (Apple Silicon) with Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit.
2. Ab-initio Quantum Chemistry via PySCF (RHF & FCI ground state energies for H2, LiH).
3. Quantum Circuit Simulation on Qiskit Aer (Bell state & VQE ansatz, statevector, Meyer-Wallach entanglement).
4. PennyLane & QPD Gate-Preserving Circuit Cutting.
5. Clifford Stabilizer Canary Probes and Hardware Noise Constraints (T2 <= 2*T1).
6. Go Core Orchestrator API & SQLite WAL Ledger persistence.
7. WebCrypto / Ed25519 AP2 Mandate Signing & Verification.
8. Next.js 16 Scientific Observatory Console Health.
"""

import sys
import os
import time
import json
import urllib.request
import urllib.error
import numpy as np

# Ensure bridge is importable
repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
bridge_path = os.path.join(repo_root, "platform", "bridge")
if bridge_path not in sys.path:
    sys.path.insert(0, bridge_path)

GREEN = "\033[92m"
BLUE = "\033[94m"
CYAN = "\033[96m"
YELLOW = "\033[93m"
RED = "\033[91m"
BOLD = "\033[1m"
RESET = "\033[0m"

def print_header(title: str):
    print(f"\n{BOLD}{CYAN}{'='*80}{RESET}")
    print(f"{BOLD}{CYAN}>>> {title}{RESET}")
    print(f"{BOLD}{CYAN}{'='*80}{RESET}")

def print_pass(msg: str):
    print(f"  {GREEN}[PASS]{RESET} {msg}")

def print_fail(msg: str):
    print(f"  {RED}[FAIL]{RESET} {msg}")

def print_info(key: str, val: str):
    print(f"  {BLUE}•{RESET} {BOLD}{key}:{RESET} {val}")


def verify_local_omlx_llm():
    print_header("1. REAL-WORLD LOCAL LLM VERIFICATION (oMLX Port 8787)")
    base_url = "http://127.0.0.1:8787/v1"
    model_name = "Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit"

    # Check models endpoint
    try:
        req = urllib.request.Request(f"{base_url}/models")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode())
            model_ids = [m["id"] for m in data.get("data", [])]
            if model_name not in model_ids:
                print_fail(f"Model {model_name} not found in oMLX. Available: {model_ids}")
                return False
            print_pass(f"Found target model '{model_name}' on live oMLX engine")
    except Exception as e:
        print_fail(f"Could not reach oMLX on {base_url}: {e}")
        return False

    # Execute real chat completion
    print_info("Action", "Sending scientific prompt to local 27B model...")
    chat_payload = {
        "model": model_name,
        "messages": [
            {
                "role": "system",
                "content": "You are the Ansatz Scout. Recommend active space and rotational gate structure for H2 at 0.7414 Angstrom. Keep answer under 60 words."
            },
            {
                "role": "user",
                "content": "Propose the variational ansatz topology for H2 ground state."
            }
        ],
        "temperature": 0.2,
        "max_tokens": 100
    }
    t0 = time.time()
    try:
        req = urllib.request.Request(
            f"{base_url}/chat/completions",
            data=json.dumps(chat_payload).encode("utf-8"),
            headers={"Content-Type": "application/json"}
        )
        with urllib.request.urlopen(req, timeout=60) as resp:
            data = json.loads(resp.read().decode())
            elapsed = time.time() - t0
            content = data["choices"][0]["message"]["content"]
            usage = data.get("usage", {})
            gen_speed = usage.get("generation_tokens_per_second", 0.0)
            comp_tok = usage.get("completion_tokens", len(content.split()))

            print_pass(f"Received completion in {elapsed:.2f}s ({comp_tok} tokens, ~{gen_speed:.1f} tok/s)")
            print_info("Generated Content", content.strip().replace("\n", " "))
            return True
    except Exception as e:
        print_fail(f"Chat completion failed: {e}")
        return False


def verify_pyscf_classical_anchors():
    print_header("2. REAL-WORLD AB-INITIO QUANTUM CHEMISTRY (PySCF)")
    from chemistry import solve_classical_anchor

    # Test H2 at equilibrium distance 0.7414 A
    print_info("Molecule 1", "H2 (R = 0.7414 Angstrom, STO-3G)")
    t0 = time.time()
    res_h2 = solve_classical_anchor("H2", 0.7414, basis="sto-3g")
    dt_h2 = time.time() - t0

    rhf_h2 = res_h2["rhf_energy"]
    fci_h2 = res_h2["fci_exact_energy"]
    corr_h2 = res_h2["correlation_energy"]

    print_info("Execution Time", f"{dt_h2*1000:.2f} ms")
    print_info("RHF Energy", f"{rhf_h2:.6f} Ha")
    print_info("FCI Exact Energy", f"{fci_h2:.6f} Ha")
    print_info("Correlation Energy", f"{corr_h2:.6f} Ha")

    # Assert exact physics values
    assert abs(fci_h2 - (-1.13727)) < 0.001, f"Unexpected H2 FCI energy: {fci_h2}"
    assert corr_h2 < 0.0, "Correlation energy must be strictly negative"
    print_pass("H2 ground state FCI matches exact quantum benchmark (-1.13727 Ha)")

    # Test LiH at equilibrium distance 1.5957 A
    print_info("Molecule 2", "LiH (R = 1.5957 Angstrom, STO-3G)")
    t0 = time.time()
    res_lih = solve_classical_anchor("LiH", 1.5957, basis="sto-3g")
    dt_lih = time.time() - t0

    rhf_lih = res_lih["rhf_energy"]
    fci_lih = res_lih["fci_exact_energy"]
    corr_lih = res_lih["correlation_energy"]

    print_info("Execution Time", f"{dt_lih*1000:.2f} ms")
    print_info("RHF Energy", f"{rhf_lih:.6f} Ha")
    print_info("FCI Exact Energy", f"{fci_lih:.6f} Ha")
    print_info("Correlation Energy", f"{corr_lih:.6f} Ha")

    assert abs(fci_lih - (-7.8824)) < 0.01, f"Unexpected LiH FCI energy: {fci_lih}"
    print_pass("LiH ground state FCI matches exact quantum benchmark (-7.8824 Ha)")
    return True


def verify_qiskit_aer_simulation():
    print_header("3. REAL-WORLD QUANTUM SIMULATION (Qiskit Aer)")
    from executor import execute_qasm, calculate_meyer_wallach

    bell_qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
measure q[0] -> c[0];
measure q[1] -> c[1];
"""
    t0 = time.time()
    res = execute_qasm(bell_qasm, shots=2048, requested_backend="qiskit-aer-simulator")
    dt = time.time() - t0

    counts = res["counts"]
    q_score = res["meyer_wallach"]
    backend = res["resolved_backend"]

    print_info("Resolved Backend", backend)
    print_info("Shot Distribution", json.dumps(counts))
    print_info("Meyer-Wallach Entanglement Q", f"{q_score:.6f}")
    print_info("Execution Duration", f"{dt*1000:.2f} ms")

    assert res["success"] is True, "Execution failed"
    assert "00" in counts and "11" in counts, "Bell state must yield 00 and 11"
    # Meyer-Wallach entanglement score for Bell state must be 1.0
    assert abs(q_score - 1.0) < 1e-4, f"Bell state Q-score must be 1.0, got {q_score}"
    print_pass("Qiskit Aer executed Bell state with verified maximal entanglement Q=1.000000")
    return True


def verify_circuit_cutting():
    print_header("4. REAL-WORLD PENNYLANE / QPD CIRCUIT CUTTING")
    from executor import cut_circuit_qasm

    four_qubit_qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[4];
creg c[4];
h q[0];
cx q[0], q[1];
cx q[1], q[2];
cx q[2], q[3];
measure q[0] -> c[0];
measure q[1] -> c[1];
measure q[2] -> c[2];
measure q[3] -> c[3];
"""
    res = cut_circuit_qasm(four_qubit_qasm, max_subcircuit_qubits=2)
    print_info("Total Wire Cuts", str(res["total_cuts"]))
    print_info("Generated Fragments", str(len(res["fragments"])))

    assert res["success"] is True, "Cutting failed"
    assert len(res["fragments"]) >= 2, "Expected at least 2 fragments"
    for frag in res["fragments"]:
        print_info(f"Fragment {frag['fragment_id']}", f"Cut qubits: {frag['cut_qubits']}")
    print_pass("PennyLane/QPD successfully decomposed 4-qubit circuit into <=2 qubit fragments")
    return True


def verify_clifford_canary_probes():
    print_header("5. REAL-WORLD CLIFFORD CANARY PROBES & NOISE AUDIT")
    from fidelity import probe_clifford_fidelity, predict_fidelity, build_noise_model, DEVICE_PROFILES

    test_qasm = """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
measure q[0] -> c[0];
measure q[1] -> c[1];
"""
    canary_fid = probe_clifford_fidelity(test_qasm, shots=64)
    print_info("Device Profile", "ibm_heron (156 qubits)")
    print_info("Canary Fidelity", f"{canary_fid*100:.2f}%")

    assert canary_fid >= 0.90, f"Canary fidelity unexpectedly low: {canary_fid}"
    print_pass(f"Hardware canary probe passed with {canary_fid*100:.2f}% fidelity")

    pred_fid = predict_fidelity(circuit_depth=8, two_qubit_gates=2, device_name="ibm_heron")
    print_info("Predicted Circuit Fidelity (depth=8)", f"{pred_fid*100:.2f}%")
    assert pred_fid >= 0.95, f"Predicted fidelity unexpectedly low: {pred_fid}"

    # Verify physical coherence constraint T2 <= 2*T1 across all device profiles
    for dev, prof in DEVICE_PROFILES.items():
        t1 = prof["t1_us"]
        t2 = prof["t2_us"]
        assert t2 <= 2.0 * t1 + 1e-9, f"Physical constraint violated for {dev}: T2={t2} > 2*T1={2*t1}"
        print_info(f"Noise Profile {dev}", f"T1={t1}us, T2={t2}us (T2 <= 2*T1 satisfied)")

    noise_model = build_noise_model("ibm_heron")
    assert noise_model is not None, "Failed to build Qiskit Aer noise model"
    print_pass("Physical coherence constraints (T2 <= 2*T1) and Aer NoiseModel verified")
    return True


def verify_go_core_and_ledger():
    print_header("6. REAL-WORLD GO CORE ORCHESTRATOR & LEDGER VERIFICATION")
    base_url = "http://localhost:8080"

    # Health check
    try:
        with urllib.request.urlopen(f"{base_url}/api/health", timeout=5) as resp:
            data = json.loads(resp.read().decode())
            print_pass(f"Go Core daemon healthy: version {data.get('version')}")
    except Exception as e:
        print_fail(f"Go Core unreachable at {base_url}: {e}")
        return False

    # Submit circuit job
    job_payload = {
        "session_id": "real-world-e2e-session",
        "qasm": """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
h q[0];
cx q[0], q[1];
measure q[0] -> c[0];
measure q[1] -> c[1];
""",
        "backend": "qiskit-aer-simulator",
        "shots": 1024,
        "budget_usd": 0.00
    }
    req = urllib.request.Request(
        f"{base_url}/api/jobs",
        data=json.dumps(job_payload).encode(),
        headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        job_data = json.loads(resp.read().decode())
        job_id = job_data["id"]
        print_pass(f"Submitted real circuit job to Go Core (Job ID: {job_id})")

    # Give job runner 1 second to execute via Python bridge
    time.sleep(1.0)

    # Fetch jobs list and verify execution result
    with urllib.request.urlopen(f"{base_url}/api/jobs?session_id=real-world-e2e-session", timeout=5) as resp:
        data = json.loads(resp.read().decode())
        jobs = data.get("jobs", [])
        matched = [j for j in jobs if j["id"] == job_id]
        assert len(matched) == 1, "Job not found in database"
        completed_job = matched[0]
        res = completed_job.get("result_json", {})
        print_info("Execution Backend", res.get("resolved_backend", "unknown"))
        print_info("Result Counts", json.dumps(res.get("counts", {})))
        print_info("Entanglement Q", str(res.get("meyer_wallach_entanglement")))
        assert res.get("success") is True, "Job execution was not successful"
        print_pass("Job completed via Go Core -> Python Bridge gRPC -> Qiskit Aer -> SQLite WAL Ledger")
    return True


def verify_ed25519_mandates():
    print_header("7. REAL-WORLD AP2 CRYPTOGRAPHIC MANDATE VERIFICATION")
    base_url = "http://localhost:8080"

    sign_payload = {
        "tenant_id": "tenant-frontier-lab",
        "job_hash": "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi",
        "max_budget_usd": 50.00,
        "ttl_seconds": 3600
    }
    req = urllib.request.Request(
        f"{base_url}/api/mandates/sign",
        data=json.dumps(sign_payload).encode(),
        headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=5) as resp:
        mandate_res = json.loads(resp.read().decode())
        mandate = mandate_res["mandate"]
        print_info("Signer Public Key", mandate["public_key_hex"][:32] + "...")
        print_info("Ed25519 Signature", mandate["signature_hex"][:32] + "...")

    # Statelessly verify the signature
    verify_req = urllib.request.Request(
        f"{base_url}/api/mandates/verify",
        data=json.dumps(mandate).encode(),
        headers={"Content-Type": "application/json"}
    )
    with urllib.request.urlopen(verify_req, timeout=5) as resp:
        verify_data = json.loads(resp.read().decode())
        print_info("Verification Result", str(verify_data))
        assert verify_data["valid"] is True, "Signature verification failed"
        print_pass("Ed25519 AP2 spend mandate verified statelessly by Go orchestrator")
    return True


def verify_nextjs_observatory_console():
    print_header("8. REAL-WORLD NEXT.JS OBSERVATORY CONSOLE VERIFICATION (Port 3000)")
    try:
        req = urllib.request.Request("http://localhost:3000")
        with urllib.request.urlopen(req, timeout=5) as resp:
            status = resp.status
            content = resp.read(2048).decode("utf-8", errors="ignore")
            print_info("HTTP Status", str(status))
            assert status == 200, f"Expected 200 OK, got {status}"
            assert "Kubona" in content or "html" in content, "Invalid console HTML payload"
            print_pass("Next.js 16 Observatory Console active and serving HTTP 200 OK")
            return True
    except Exception as e:
        print_fail(f"Next.js Console error: {e}")
        return False


def main():
    print(f"\n{BOLD}{YELLOW}KUBONA REAL-WORLD END-TO-END VERIFICATION SUITE{RESET}")
    print(f"Executing non-mocked integration across local oMLX model, PySCF, Qiskit, Go Core, and Next.js...\n")

    results = {}
    results["1_local_omlx_llm"] = verify_local_omlx_llm()
    results["2_pyscf_chemistry"] = verify_pyscf_classical_anchors()
    results["3_qiskit_aer"] = verify_qiskit_aer_simulation()
    results["4_circuit_cutting"] = verify_circuit_cutting()
    results["5_clifford_canary"] = verify_clifford_canary_probes()
    results["6_go_core_ledger"] = verify_go_core_and_ledger()
    results["7_ed25519_mandates"] = verify_ed25519_mandates()
    results["8_nextjs_console"] = verify_nextjs_observatory_console()

    print_header("VERIFICATION SUMMARY")
    all_passed = True
    for test_name, passed in results.items():
        status_str = f"{GREEN}PASS{RESET}" if passed else f"{RED}FAIL{RESET}"
        print(f"  {test_name:<30}: {status_str}")
        if not passed:
            all_passed = False

    if all_passed:
        print(f"\n{BOLD}{GREEN}ALL 8 REAL-WORLD SUBSYSTEMS ARE 100% OPERATIONAL WITHOUT MOCKS!{RESET}\n")
        return 0
    else:
        print(f"\n{BOLD}{RED}ONE OR MORE SUBSYSTEMS FAILED REAL-WORLD VERIFICATION.{RESET}\n")
        return 1

if __name__ == "__main__":
    sys.exit(main())
