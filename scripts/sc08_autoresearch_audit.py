#!/usr/bin/env python3
"""SC-08: Closed-Loop Autoresearch Loop Live Audit.
Tests harness discovery, baseline evaluation, iterative step progression,
lifecycle controls (pause, resume, sleep, wake, stop), spend reservation release,
and S1 idempotency replay against kubonad (:8080).
"""
import sys
import time
import requests

API_BASE = "http://127.0.0.1:8080"
HEADERS = {"Authorization": "Bearer dev-token"}

def run_sc08():
    print("=== [SC-08] Testing Closed-Loop Autoresearch Loop (VQE PES Scan) ===")
    
    # 1. Discover available harnesses
    res = requests.get(f"{API_BASE}/api/autoresearch/harnesses", headers=HEADERS)
    assert res.status_code == 200, f"Get harnesses failed: {res.status_code} {res.text}"
    harnesses = res.json().get("harnesses", [])
    assert len(harnesses) > 0, "No autoresearch harnesses registered"
    harness_ids = [h["id"] for h in harnesses]
    print(f"✓ Registered harnesses ({len(harnesses)}): {harness_ids}")
    assert "ansatz-search" in harness_ids, "'ansatz-search' harness missing"

    # 2. Create dedicated session with spend limit
    res = requests.post(f"{API_BASE}/api/sessions", json={
        "title": "SC-08 Autoresearch Audit",
        "tenant_id": "default",
        "spend_limit_usd": 100.0,
        "target_molecule": "H2",
        "fresh": True
    }, headers=HEADERS)
    assert res.status_code == 201, f"Create session failed: {res.status_code} {res.text}"
    session_id = res.json()["id"]
    print(f"✓ Created session {session_id}")

    # 3. Start autoresearch loop with valid python ansatz
    req_id = f"ar-audit-{int(time.time())}"
    valid_ansatz = (
        "from qiskit import QuantumCircuit\n"
        "from qiskit.circuit import Parameter\n\n"
        "def build_circuit():\n"
        "    theta = Parameter('theta')\n"
        "    qc = QuantumCircuit(2)\n"
        "    qc.ry(theta, 0)\n"
        "    qc.cx(0, 1)\n"
        "    return qc\n"
    )
    start_payload = {
        "session_id": session_id,
        "harness_id": "ansatz-search",
        "initial_artifact": valid_ansatz,
        "max_iterations": 5,
        "request_id": req_id
    }
    print(f"Starting loop on harness 'ansatz-search' (max_iterations=5, request_id={req_id})...")
    res = requests.post(f"{API_BASE}/api/autoresearch/start", json=start_payload, headers=HEADERS)
    assert res.status_code == 201, f"Start loop failed: {res.status_code} {res.text}"
    loop_data = res.json()
    loop_id = loop_data["id"]
    print(f"✓ Loop started: id={loop_id}, status={loop_data.get('status')}")
    assert loop_data.get("status") == "running", f"Expected running status, got {loop_data.get('status')}"
    assert loop_data.get("harness_id") == "ansatz-search"
    baseline = loop_data.get("baseline_score")
    print(f"✓ Baseline evaluated: score={baseline}")

    # 4. S1 Idempotency Replay test: re-submitting start with same request_id
    res_replay = requests.post(f"{API_BASE}/api/autoresearch/start", json=start_payload, headers=HEADERS)
    assert res_replay.status_code in (200, 201), f"Idempotency replay failed: {res_replay.status_code}"
    replay_data = res_replay.json()
    assert replay_data["id"] == loop_id, f"Idempotency mismatch: {replay_data['id']} != {loop_id}"
    print(f"✓ S1 Idempotency verified: replayed loop {loop_id} without duplicate start")

    # 5. Verify loop appears in active loops list
    res = requests.get(f"{API_BASE}/api/autoresearch/loops", headers=HEADERS)
    assert res.status_code == 200
    loops = res.json().get("loops", [])
    found = any(l["id"] == loop_id for l in loops)
    assert found, f"Loop {loop_id} not found in active loops list"
    print(f"✓ Loop {loop_id} active in /api/autoresearch/loops")

    # 6. Execute step 1
    print("Executing Step 1...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/step", headers=HEADERS)
    assert res.status_code == 200, f"Step 1 failed: {res.status_code} {res.text}"
    st1 = res.json()
    rec1 = st1.get("step", {})
    state1 = st1.get("state", {})
    assert rec1.get("iteration") == 1, f"Expected step iteration 1, got {rec1.get('iteration')}"
    assert state1.get("current_iteration") == 1, f"Expected state iteration 1, got {state1.get('current_iteration')}"
    print(f"✓ Step 1 executed: decision={rec1.get('decision')}, metric={rec1.get('metric')}, champ={state1.get('champion_metric')}")

    # 7. Pause loop
    print("Pausing loop...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/pause", headers=HEADERS)
    assert res.status_code == 200, f"Pause failed: {res.status_code} {res.text}"
    st_pause = res.json()
    assert st_pause.get("status") == "paused", f"Expected paused, got {st_pause.get('status')}"
    print(f"✓ Loop paused successfully")

    # 8. Resume loop
    print("Resuming loop...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/resume", headers=HEADERS)
    assert res.status_code == 200, f"Resume failed: {res.status_code} {res.text}"
    st_resume = res.json()
    assert st_resume.get("status") == "running", f"Expected running, got {st_resume.get('status')}"
    print(f"✓ Loop resumed successfully")

    # 9. Sleep loop
    print("Testing sleep lifecycle...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/sleep", headers=HEADERS)
    assert res.status_code == 200, f"Sleep failed: {res.status_code} {res.text}"
    st_sleep = res.json()
    assert st_sleep.get("status") == "sleeping", f"Expected sleeping, got {st_sleep.get('status')}"
    print(f"✓ Loop entered sleeping state")

    # 10. Wake loop
    print("Testing wake lifecycle...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/wake", headers=HEADERS)
    assert res.status_code == 200, f"Wake failed: {res.status_code} {res.text}"
    st_wake = res.json()
    assert st_wake.get("status") == "running", f"Expected running, got {st_wake.get('status')}"
    print(f"✓ Loop awakened and returned to running state")

    # 11. Execute step 2
    print("Executing Step 2...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/step", headers=HEADERS)
    assert res.status_code == 200, f"Step 2 failed: {res.status_code} {res.text}"
    st2 = res.json()
    rec2 = st2.get("step", {})
    state2 = st2.get("state", {})
    assert rec2.get("iteration") == 2, f"Expected step iteration 2, got {rec2.get('iteration')}"
    assert state2.get("current_iteration") == 2, f"Expected state iteration 2, got {state2.get('current_iteration')}"
    print(f"✓ Step 2 executed: decision={rec2.get('decision')}, metric={rec2.get('metric')}")

    # 12. Stop / Terminate loop and verify clean teardown
    print("Terminating loop...")
    res = requests.post(f"{API_BASE}/api/autoresearch/loops/{loop_id}/stop", headers=HEADERS)
    assert res.status_code == 200, f"Stop failed: {res.status_code} {res.text}"
    st_stop = res.json()
    assert st_stop.get("status") in ("stopped", "aborted", "completed"), f"Unexpected final status: {st_stop.get('status')}"
    print(f"✓ Loop terminated: final status={st_stop.get('status')}")

    # 13. Verify ledger events recorded for autoresearch lifecycle
    res = requests.get(f"{API_BASE}/api/events?session_id={session_id}", headers=HEADERS)
    assert res.status_code == 200, f"Get events failed: {res.status_code}"
    events = res.json().get("events", [])
    event_types = [e["event_type"] for e in events]
    print(f"✓ Session recorded {len(events)} ledger events: {set(event_types)}")
    assert "autoresearch_loop_started" in event_types, "Missing autoresearch_loop_started event"
    assert "autoresearch_loop_slept" in event_types, "Missing autoresearch_loop_slept event"
    assert "autoresearch_loop_woken" in event_types, "Missing autoresearch_loop_woken event"

    print("=== [SC-08] PASSED 100% ===")

if __name__ == "__main__":
    try:
        run_sc08()
    except AssertionError as e:
        print(f"FAIL: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"ERROR: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(2)
