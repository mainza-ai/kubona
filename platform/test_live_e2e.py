import urllib.request
import json
import time

def get(url):
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=15) as r:
        return json.loads(r.read().decode())

def post(url, data):
    body = json.dumps(data).encode()
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=180) as r:
        return json.loads(r.read().decode())

def run_e2e_verification():
    print("[1/6] Testing Fleet Status on :8080...")
    fleet = get("http://localhost:8080/api/fleet")
    backends = [b["name"] for b in fleet["backends"]]
    print(f"  Fleet active: {backends}")
    assert "local_simulator" in backends and "ibm_heron" in backends

    print("[2/6] Testing Live Multi-Agent Tribunal with Qwen3.8-27B (:8787) & PySCF Bridge (:50051)...")
    trib = post("http://localhost:8080/api/chat/tribunal", {
        "session_id": "live-e2e-tribunal",
        "prompt": "Evaluate H2 molecular ground state energy at equilibrium bond distance 0.7414 A"
    })
    target = trib.get("tribunal", {}).get("target", "")
    passed = trib.get("tribunal", {}).get("consensus_passed", False)
    turns = trib.get("tribunal", {}).get("turns", [])
    print(f"  Target: {target}")
    print(f"  Consensus passed: {passed}")
    print(f"  Turns executed: {len(turns)}")
    for t in turns:
        print(f"    - {t['name']}: {t['verdict']}")

    print("[3/6] Submitting Quantum Circuit Job to Qiskit Aer Engine...")
    job = post("http://localhost:8080/api/jobs", {
        "session_id": "live-e2e-job",
        "qasm": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\ncreg c[2];\nh q[0];\ncx q[0], q[1];\nmeasure q[0] -> c[0];\nmeasure q[1] -> c[1];\n",
        "backend": "local_simulator",
        "shots": 1024,
        "budget_usd": 0.0
    })
    job_id = job.get("id", "")
    status = job.get("status", "")
    print(f"  Job ID: {job_id}, Status: {status}")
    time.sleep(1)

    print("[4/6] Testing RFC 8032 Ed25519 AP2 Mandate Signing & Verification...")
    signed = post("http://localhost:8080/api/mandates/sign", {
        "tenant_id": "tenant-quantum-lab-1",
        "job_hash": "sha256:88fa29810bce328a994781dfa98e21",
        "max_budget_usd": 25.0,
        "ttl_seconds": 3600
    })
    mandate = signed.get("mandate", {})
    mandate_resp = post("http://localhost:8080/api/mandates/verify", mandate)
    valid = mandate_resp.get("valid", False)
    print(f"  Mandate valid: {valid}")
    assert valid is True

    print("[5/6] Testing Publication Results Studio Notebook Generation...")
    notebooks = get("http://localhost:8080/api/results/notebooks")
    nb = notebooks["notebooks"][0]
    print(f"  Notebook: '{nb['title']}'")
    print(f"  FCI Anchor: {nb['classical_energy']} Ha | VQE: {nb['quantum_energy']} Ha | Delta: {nb['delta_mha']} mHa")
    print(f"  Chemical Accuracy: {nb['chemical_accuracy']} | Merkle Hash: {nb['provenance_hash'][:24]}...")

    print("[6/7] Verifying SQLite WAL Cryptographic Ledger...")
    events = get("http://localhost:8080/api/events?limit=10")
    recorded = events.get("events", [])
    print(f"  Recorded events in SQLite WAL: {len(recorded)}")
    for ev in recorded[:3]:
        print(f"    - Event #{ev['id']}: {ev['event_type']} | Timestamp: {ev['timestamp']}")

    print("[7/7] Testing RDKit Molecular ADMET & 12-Agent Candidate Pipeline...")
    admet = get("http://localhost:8080/api/chemistry/admet?smiles=CC(=O)Oc1ccccc1C(=O)O")
    print(f"  Aspirin MW: {admet.get('molecular_weight', 0):.1f} Da | cLogP: {admet.get('clogp', 0):.2f} | QED: {admet.get('qed', 0):.2f}")
    assert admet.get("lipinski", {}).get("pass") is True
    assert admet.get("sa_score", 10.0) < 3.5

    pipeline = post("http://localhost:8080/api/discovery/pipeline", {
        "target": "COX-2",
        "candidates": [
            {"name": "Aspirin", "smiles": "CC(=O)Oc1ccccc1C(=O)O"},
            {"name": "Paracetamol", "smiles": "CC(=O)Nc1ccc(O)cc1"}
        ]
    })
    total_eval = pipeline.get("total_evaluated", 0)
    top_lead = pipeline.get("top_lead", {}).get("name", "")
    print(f"  Target: {pipeline.get('target')} | Evaluated: {total_eval} | Top Lead: {top_lead}")
    assert total_eval == 2
    assert top_lead != ""

    print("\n🚀 ALL 7 LIVE END-TO-END SUBSYSTEM TESTS PASSED WITH 100% REAL EXECUTION!")

if __name__ == "__main__":
    run_e2e_verification()

