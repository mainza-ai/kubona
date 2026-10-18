#!/usr/bin/env python3
"""SC-07: Multi-Agent Epistemic Tribunal & Dispute Resolution Live Audit.
Tests 4-role multi-agent deliberation, obligation ledger case tracking,
and S1 idempotency replay against kubonad (:8080).
"""
import sys
import time
import requests

API_BASE = "http://127.0.0.1:8080"

def run_sc07():
    print("=== [SC-07] Testing Multi-Agent Epistemic Tribunal & Dispute Resolution ===")
    
    # 1. Create a dedicated session for tribunal audit
    res = requests.post(f"{API_BASE}/api/sessions", json={
        "title": "SC-07 Epistemic Tribunal Audit",
        "tenant_id": "default",
        "spend_limit_usd": 150.0,
        "target_molecule": "LiH",
        "fresh": True
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 201, f"Create session failed: {res.status_code} {res.text}"
    session_id = res.json()["id"]
    print(f"✓ Created session {session_id}")

    # 2. Convene Tribunal with idempotency request_id
    req_id = f"trib-audit-{int(time.time())}"
    prompt = "Investigate H2 ground state at R = 0.7414 A in STO-3G basis"
    
    print(f"Submitting tribunal prompt: '{prompt}' (request_id={req_id})...")
    res = requests.post(f"{API_BASE}/api/chat/tribunal", json={
        "session_id": session_id,
        "prompt": prompt,
        "request_id": req_id
    }, headers={"Authorization": "Bearer dev-token"}, timeout=600)
    
    assert res.status_code == 200, f"Tribunal failed: {res.status_code} {res.text}"
    trib_data = res.json()
    assert "tribunal" in trib_data, f"Missing tribunal key in response: {trib_data.keys()}"
    t_res = trib_data["tribunal"]
    
    turns = t_res.get("turns", [])
    assert len(turns) == 4, f"Expected exactly 4 tribunal turns, got {len(turns)}"
    
    expected_roles = ["ansatz-scout", "disconfirmation-officer", "device-auditor", "epistemic-scribe"]
    for i, role in enumerate(expected_roles):
        assert turns[i]["role"] == role, f"Turn {i+1} expected role {role}, got {turns[i]['role']}"
        assert len(turns[i]["content"]) > 20, f"Turn {i+1} has suspiciously short content"
        print(f"✓ Turn {i+1} [{turns[i]['name']}]: verdict={turns[i].get('verdict')}, content_len={len(turns[i]['content'])} chars")

    proposal = t_res.get("proposal_json")
    assert proposal is not None, "Missing proposal_json from tribunal consensus"
    assert "qasm" in proposal, f"Missing qasm in proposal: {proposal}"
    assert "classical_anchor" in proposal or "target" in proposal
    print(f"✓ Cryptographic proposal synthesized: target={proposal.get('target')}, tier={proposal.get('recommended_tier')}")

    obligation_id = t_res.get("obligation_id")
    print(f"✓ Obligation case registered: {obligation_id}")

    # 3. Test S1 Idempotency Replay on Slow Rail
    print("\n--- Testing S1 Idempotency Replay (Re-submitting same request_id) ---")
    t0 = time.time()
    res_replay = requests.post(f"{API_BASE}/api/chat/tribunal", json={
        "session_id": session_id,
        "prompt": prompt,
        "request_id": req_id
    }, headers={"Authorization": "Bearer dev-token"}, timeout=10)
    elapsed = time.time() - t0
    
    assert res_replay.status_code == 200, f"Replay failed: {res_replay.status_code}"
    # Replay should be nearly instantaneous (< 0.5s) because it serves cached consensus
    assert elapsed < 1.0, f"Replay took {elapsed:.2f}s, expected < 1.0s (should not re-execute LLM calls)"
    replay_data = res_replay.json()
    assert replay_data["tribunal"]["obligation_id"] == obligation_id
    print(f"✓ S1 Idempotency Replay verified: served cached consensus in {elapsed*1000:.1f}ms without LLM churn")

    print("=== [SC-07] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc07()
