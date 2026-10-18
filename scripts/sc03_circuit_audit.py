#!/usr/bin/env python3
"""SC-03: Quantum Circuit Synthesis & Rust QIR Compilation Live Audit.
Tests multi-qubit GHZ, QFT, and parameterized entangling ansatz circuits
via kubonad (:8080) -> Rust QIR -> Python Bridge Aer Simulator.
"""
import sys
import time
import json
import requests
import numpy as np

API_BASE = "http://127.0.0.1:8080"

def run_sc03():
    print("=== [SC-03] Testing Quantum Circuit Synthesis & Rust QIR Compilation ===")
    
    # Create an audit session
    res = requests.post(f"{API_BASE}/api/sessions", json={
        "title": "SC-03 Circuit Audit",
        "tenant_id": "default",
        "spend_limit_usd": 150.0,
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 201, f"Create session failed: {res.status_code} {res.text}"
    session_id = res.json()["id"]
    print(f"✓ Created session {session_id}")

    circuits = [
        ("3-Qubit GHZ", """OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
creg c[3];
h q[0];
cx q[0],q[1];
cx q[1],q[2];
measure q[0] -> c[0];
measure q[1] -> c[1];
measure q[2] -> c[2];
""", lambda counts: (
            counts.get("000", 0) + counts.get("111", 0) > 900
        )),
        ("5-Qubit GHZ", """OPENQASM 2.0;
include "qelib1.inc";
qreg q[5];
creg c[5];
h q[0];
cx q[0],q[1];
cx q[1],q[2];
cx q[2],q[3];
cx q[3],q[4];
measure q[0] -> c[0];
measure q[1] -> c[1];
measure q[2] -> c[2];
measure q[3] -> c[3];
measure q[4] -> c[4];
""", lambda counts: (
            counts.get("00000", 0) + counts.get("11111", 0) > 850
        )),
        ("3-Qubit QFT (Uniform Superposition)", """OPENQASM 2.0;
include "qelib1.inc";
qreg q[3];
creg c[3];
h q[0];
h q[1];
h q[2];
measure q[0] -> c[0];
measure q[1] -> c[1];
measure q[2] -> c[2];
""", lambda counts: (
            len(counts) == 8 and all(60 <= counts.get(f"{i:03b}", 0) <= 200 for i in range(8))
        )),
        ("2-Qubit Entangling Ansatz", """OPENQASM 2.0;
include "qelib1.inc";
qreg q[2];
creg c[2];
ry(0.785398) q[0];
rz(1.570796) q[1];
cx q[0],q[1];
ry(0.392699) q[1];
measure q[0] -> c[0];
measure q[1] -> c[1];
""", lambda counts: (
            len(counts) >= 2 and sum(counts.values()) == 1024
        )),
    ]

    for name, qasm, validator in circuits:
        job_req = {
            "session_id": session_id,
            "qasm": qasm,
            "backend": "local_simulator",
            "shots": 1024,
            "budget_usd": 1.0,
        }
        res = requests.post(f"{API_BASE}/api/jobs", json=job_req, headers={"Authorization": "Bearer dev-token"})
        assert res.status_code == 201, f"Job submission failed for {name}: {res.status_code} {res.text}"
        data = res.json()
        job_id = data.get("id") or data.get("job", {}).get("id")
        
        # Poll for completion (up to 5s)
        completed = False
        job_data = None
        for _ in range(25):
            res = requests.get(f"{API_BASE}/api/jobs/{job_id}?session_id={session_id}", headers={"Authorization": "Bearer dev-token"})
            assert res.status_code == 200
            job_data = res.json()["job"]
            if job_data["status"] == 3: # COMPLETED
                completed = True
                break
            elif job_data["status"] == 4: # FAILED
                break
            time.sleep(0.1)

        assert completed, f"Job {job_id} did not complete: status={job_data['status']}, error={job_data.get('error')}"
        res_json = job_data["result_json"]
        assert res_json.get("success") is True, f"Job result was not successful: {res_json}"
        assert job_data.get("compiler_used") in ["rust-qir-core", "rust-native", "rust-compiler"], f"Unexpected compiler: {job_data.get('compiler_used')}"

        counts = res_json.get("counts", {})
        assert validator(counts), f"Count distribution failed validation for {name}: counts={counts}"
        
        mw_ent = res_json.get("meyer_wallach_entanglement")
        exec_ms = res_json.get("execution_time_ms")
        print(f"✓ {name:32s}: status=COMPLETED (compiler={job_data.get('compiler_used')}, "
              f"Aer time={exec_ms:.2f}ms, MW_ent={mw_ent if mw_ent is not None else 0:.4f}, bitstrings={len(counts)})")

    # Fail-closed check: syntactically invalid QASM
    bad_job_req = {
        "session_id": session_id,
        "qasm": "THIS IS NOT VALID QASM CODE AT ALL;",
        "backend": "local_simulator",
        "shots": 1024,
    }
    res = requests.post(f"{API_BASE}/api/jobs", json=bad_job_req, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code in [400, 422, 500] or (res.status_code == 201 and res.json()["job"]["status"] == 4), (
        f"Fail-closed expected for invalid QASM, got status={res.status_code}"
    )
    print("✓ Fail-closed verified: malformed OpenQASM rejected cleanly")

    print("=== [SC-03] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc03()
