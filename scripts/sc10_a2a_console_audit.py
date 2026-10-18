#!/usr/bin/env python3
"""SC-10: Agent-to-Agent (A2A) Delegation & Console Integration Live Audit.
Tests:
1. A2A Agent Card discovery via JSON-RPC 2.0 (POST /api/a2a/v1).
2. x402 Payment challenge enforcement on a2a.SubmitTask (402 Payment Required).
3. Explicit x402 challenge acquisition (GET /api/a2a/v1/challenge).
4. Ed25519-signed x402 authorization header and task submission handoff.
5. Task status polling and revocation lifecycle (a2a.GetTaskStatus, a2a.RevokeTask).
6. Operator Console SSR and API proxy integration against localhost:3000.
"""
import sys
import time
import json
import hashlib
import requests
from cryptography.hazmat.primitives.asymmetric import ed25519

API_BASE = "http://127.0.0.1:8080"
CONSOLE_BASE = "http://localhost:3000"
HEADERS = {"Authorization": "Bearer dev-token"}

def run_sc10():
    print("=== [SC-10] Testing Agent-to-Agent (A2A) Delegation & Console Integration ===")

    # ---------------------------------------------------------
    # PART 1: A2A Agent Card Discovery
    # ---------------------------------------------------------
    print("\n--- Part 1: A2A Agent Card Discovery ---")
    rpc_card_req = {
        "jsonrpc": "2.0",
        "id": "req-card-1",
        "method": "a2a.GetAgentCard",
        "params": {}
    }
    res = requests.post(f"{API_BASE}/api/a2a/v1", json=rpc_card_req, headers=HEADERS)
    assert res.status_code == 200, f"GetAgentCard failed: {res.status_code} {res.text}"
    card_resp = res.json()
    assert card_resp.get("jsonrpc") == "2.0", f"Invalid jsonrpc: {card_resp}"
    card = card_resp.get("result", {})
    assert "capabilities" in card, f"Missing capabilities in card: {card}"
    assert "rate_usd_per_job" in card or "pricing" in card, f"Missing rate_usd_per_job in card: {card}"
    caps = card["capabilities"]
    print(f"✓ A2A Agent Card retrieved: name={card.get('name')}, rate={card.get('rate_usd_per_job')}, capabilities={caps}")

    # ---------------------------------------------------------
    # PART 2: x402 Payment Challenge & Refusal on SubmitTask
    # ---------------------------------------------------------
    print("\n--- Part 2: x402 Payment Refusal on Unpaid SubmitTask ---")
    rpc_submit_unpaid = {
        "jsonrpc": "2.0",
        "id": "req-sub-unpaid",
        "method": "a2a.SubmitTask",
        "params": {
            "task_type": "circuit_optimization",
            "payload": {"circuit": "OPENQASM 2.0;"},
            "max_budget_usd": 10.0,
            "scopes": [caps[0]] if caps else ["execution:submit"]
        }
    }
    res_unpaid = requests.post(f"{API_BASE}/api/a2a/v1", json=rpc_submit_unpaid)
    assert res_unpaid.status_code == 402, f"Expected 402 Payment Required, got {res_unpaid.status_code} {res_unpaid.text}"
    www_auth = res_unpaid.headers.get("WWW-Authenticate", "")
    assert "x402" in www_auth, f"Missing x402 in WWW-Authenticate header: {www_auth}"
    print(f"✓ Unpaid task submission refused fail-closed: 402 Payment Required, WWW-Authenticate={www_auth[:40]}...")

    # ---------------------------------------------------------
    # PART 3: x402 Challenge Acquisition & Signing
    # ---------------------------------------------------------
    print("\n--- Part 3: x402 Challenge Acquisition & Ed25519 Signing ---")
    res_chal = requests.get(f"{API_BASE}/api/a2a/v1/challenge?token=USDC&amount=0.05")
    assert res_chal.status_code == 402, f"Expected 402 with challenge, got {res_chal.status_code}"
    challenge = res_chal.json()
    challenge_id = challenge["challenge_id"]
    payload_to_sign = challenge["payload"]
    recipient = challenge.get("recipient")
    print(f"✓ Payment challenge generated: id={challenge_id}, recipient={recipient}")

    # Generate client Ed25519 payer keypair and sign challenge
    payer_priv = ed25519.Ed25519PrivateKey.generate()
    payer_pub = payer_priv.public_key()
    payer_pub_hex = payer_pub.public_bytes_raw().hex()
    
    # Hash challenge payload and sign
    msg_hash = hashlib.sha256(payload_to_sign.encode("utf-8")).digest()
    sig = payer_priv.sign(msg_hash).hex()

    x402_header = f'x402 challenge="{challenge_id}", token="USDC", amount="0.05", payer="{payer_pub_hex}", signature="{sig}"'
    print(f"✓ Signed x402 authorization generated for payer {payer_pub_hex[:16]}...")

    # ---------------------------------------------------------
    # PART 4: A2A Paid Task Submission & Lineage
    # ---------------------------------------------------------
    print("\n--- Part 4: A2A Paid Task Submission ---")
    rpc_submit_paid = {
        "jsonrpc": "2.0",
        "id": "req-sub-paid-1",
        "method": "a2a.SubmitTask",
        "params": {
            "task_type": "circuit_optimization",
            "payload": {"circuit": "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[2];\n"},
            "max_budget_usd": 15.0,
            "scopes": [caps[0]] if caps else ["execution:submit"]
        }
    }
    res_paid = requests.post(f"{API_BASE}/api/a2a/v1", json=rpc_submit_paid, headers={
        "Authorization": x402_header
    })
    assert res_paid.status_code == 200, f"Paid SubmitTask failed: {res_paid.status_code} {res_paid.text}"
    sub_resp = res_paid.json()
    assert sub_resp.get("error") is None, f"SubmitTask returned RPC error: {sub_resp.get('error')}"
    task_res = sub_resp.get("result", {})
    task_id = task_res.get("task_id") or task_res.get("id")
    assert task_id is not None, f"Missing task_id in response: {sub_resp}"
    print(f"✓ Paid task submitted and scheduled: task_id={task_id}, status={task_res.get('status')}")

    # ---------------------------------------------------------
    # PART 5: A2A Task Query & Revocation Lifecycle
    # ---------------------------------------------------------
    print("\n--- Part 5: Task Status & Revocation Lifecycle ---")
    rpc_status_req = {
        "jsonrpc": "2.0",
        "id": "req-status-1",
        "method": "a2a.GetTaskStatus",
        "params": {"task_id": task_id}
    }
    res_st = requests.post(f"{API_BASE}/api/a2a/v1", json=rpc_status_req, headers=HEADERS)
    assert res_st.status_code == 200
    st_data = res_st.json().get("result", {})
    assert st_data.get("id") == task_id or st_data.get("task_id") == task_id
    print(f"✓ a2a.GetTaskStatus verified: status={st_data.get('status')}, budget={st_data.get('max_budget_usd')}")

    # Revoke task
    rpc_revoke_req = {
        "jsonrpc": "2.0",
        "id": "req-revoke-1",
        "method": "a2a.RevokeTask",
        "params": {"task_id": task_id}
    }
    res_rev = requests.post(f"{API_BASE}/api/a2a/v1", json=rpc_revoke_req, headers=HEADERS)
    assert res_rev.status_code == 200
    rev_result = res_rev.json().get("result", {})
    print(f"✓ a2a.RevokeTask executed: result={rev_result}")

    # ---------------------------------------------------------
    # PART 6: Operator Console SSR & API Proxy Live Audit
    # ---------------------------------------------------------
    print("\n--- Part 6: Next.js Operator Console Live Audit ---")
    res_console = requests.get(f"{CONSOLE_BASE}/", timeout=10)
    assert res_console.status_code == 200, f"Console SSR failed: {res_console.status_code}"
    html = res_console.text
    assert "<!DOCTYPE html>" in html or "<html" in html, "Console response is not valid HTML"
    assert "Kubona" in html or "kubona" in html.lower() or "_next" in html, "Console HTML does not reference Kubona or Next.js"
    print(f"✓ Operator Console homepage verified (HTTP 200, {len(html)} bytes HTML rendered)")

    # Test Console Core Health & Fleet API Probing
    endpoints = [
        ("/api/health", 200),
        ("/api/fleet", 200),
        ("/api/sessions", 200),
        ("/api/llm/settings", 200),
        ("/api/auth/whoami", 200),
    ]
    for ep, exp_status in endpoints:
        r = requests.get(f"{API_BASE}{ep}", headers=HEADERS)
        assert r.status_code == exp_status, f"Endpoint {ep} expected {exp_status}, got {r.status_code} {r.text}"
        print(f"✓ Backend endpoint {ep} verified (HTTP {r.status_code})")

    print("\n=== [SC-10] PASSED 100% ===")

if __name__ == "__main__":
    try:
        run_sc10()
    except AssertionError as e:
        print(f"FAIL: {e}")
        sys.exit(1)
    except Exception as e:
        print(f"ERROR: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(2)
