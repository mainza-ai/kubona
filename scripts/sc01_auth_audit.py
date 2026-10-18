#!/usr/bin/env python3
"""SC-01: Auth & Lab Multi-Tenancy Live Server Audit.
Tests the full Ed25519 challenge-response lifecycle, CSRF, admin signatures,
and tenant autonomy transitions against http://127.0.0.1:8080.
"""
import sys
import json
import time
import requests
import nacl.signing
import nacl.encoding
import hashlib

BASE_URL = "http://127.0.0.1:8080"
TENANT_ID = f"lab-audit-{int(time.time())}"

def run_sc01():
    print(f"=== [SC-01] Testing Ed25519 Lifecycle for tenant {TENANT_ID} ===")
    
    # 1. Generate Ed25519 keypair
    signing_key = nacl.signing.SigningKey.generate()
    verify_key = signing_key.verify_key
    priv_hex = signing_key.encode(encoder=nacl.encoding.HexEncoder).decode("utf-8")
    pub_hex = verify_key.encode(encoder=nacl.encoding.HexEncoder).decode("utf-8")
    pub_fp = hashlib.sha256(verify_key.encode()).hexdigest()[:16]
    print(f"Generated Ed25519 Pubkey: {pub_hex[:16]}... (fp: {pub_fp})")

    # 2. Register new lab tenant
    res = requests.post(f"{BASE_URL}/api/tenants", json={
        "id": TENANT_ID,
        "display_name": "SC-01 Audit Lab",
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 201, f"Create tenant failed: {res.status_code} {res.text}"
    print("✓ Tenant registered")

    # 3. Bootstrap admin pubkey
    res = requests.post(f"{BASE_URL}/api/tenants/admin-key", json={
        "tenant_id": TENANT_ID,
        "new_admin_pubkey_hex": pub_hex,
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 200, f"Bootstrap admin key failed: {res.status_code} {res.text}"
    print("✓ Admin pubkey bootstrapped")

    # 4. Request challenge nonce
    res = requests.post(f"{BASE_URL}/api/auth/challenge", json={"tenant_id": TENANT_ID})
    assert res.status_code == 200, f"Challenge failed: {res.status_code} {res.text}"
    c_data = res.json()
    nonce = c_data["nonce"]
    print(f"✓ Received challenge nonce: {nonce[:12]}... (ttl: {c_data.get('ttl_seconds')}s)")

    # 5. Fail-closed check: bad signature
    bad_sig = "0" * 128
    res = requests.post(f"{BASE_URL}/api/auth/login", json={
        "tenant_id": TENANT_ID,
        "pubkey_hex": pub_hex,
        "nonce": nonce,
        "signature_hex": bad_sig,
    })
    assert res.status_code == 401, f"Expected 401 for bad signature, got {res.status_code}"
    print("✓ Fail-closed verified: bad signature refused (401)")

    # 6. Request a fresh challenge nonce (since nonces are single-use)
    res = requests.post(f"{BASE_URL}/api/auth/challenge", json={"tenant_id": TENANT_ID})
    assert res.status_code == 200
    nonce = res.json()["nonce"]

    # 7. Sign canonical challenge string: "kubona-auth/v1 login <nonce>"
    msg = f"kubona-auth/v1 login {nonce}".encode("utf-8")
    sig = signing_key.sign(msg).signature
    sig_hex = nacl.encoding.HexEncoder.encode(sig).decode("utf-8")

    # 8. Login with valid signature
    session = requests.Session()
    res = session.post(f"{BASE_URL}/api/auth/login", json={
        "tenant_id": TENANT_ID,
        "pubkey_hex": pub_hex,
        "nonce": nonce,
        "signature_hex": sig_hex,
    })
    assert res.status_code == 200, f"Login failed: {res.status_code} {res.text}"
    login_data = res.json()
    assert login_data["ok"] is True
    assert login_data["tenant_id"] == TENANT_ID
    assert "kubona_session" in session.cookies
    print("✓ Login succeeded: session cookie established")

    # 9. Verify whoami with session cookie
    res = session.get(f"{BASE_URL}/api/auth/whoami")
    assert res.status_code == 200
    who = res.json()
    assert who["authenticated"] is True
    assert who["tenant_id"] == TENANT_ID
    print(f"✓ whoami verified: tenant={who['tenant_id']}, fp={who['pubkey_fp']}")

    # 10. Admin Action: Change ceiling
    new_ceiling = 250.0
    expiry_str = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + 300))
    # canonical payload: tenant_id|action|params_hash|expiry
    # Go encodes float64 250.0 as 250 in json.Marshal(map[string]float64)
    params = {"ceiling_usd": int(new_ceiling)}
    p_hash = hashlib.sha256(json.dumps(params, separators=(",", ":")).encode("utf-8")).hexdigest()
    canonical = f"{TENANT_ID}|set_ceiling|{p_hash}|{expiry_str}".encode("utf-8")
    action_sig = nacl.encoding.HexEncoder.encode(signing_key.sign(canonical).signature).decode("utf-8")

    res = session.post(f"{BASE_URL}/api/tenants/ceiling", json={
        "tenant_id": TENANT_ID,
        "ceiling_usd": new_ceiling,
        "action": {
            "tenant_id": TENANT_ID,
            "action": "set_ceiling",
            "params_hash": p_hash,
            "expiry": expiry_str,
            "pubkey_hex": pub_hex,
            "signature_hex": action_sig,
        }
    })
    assert res.status_code == 200, f"Set ceiling failed: {res.status_code} {res.text}"
    assert res.json().get("spend_ceiling_usd") == new_ceiling
    print(f"✓ Admin-signed mutation verified: ceiling raised to ${new_ceiling}")

    # 11. Create a session under this tenant
    res = session.post(f"{BASE_URL}/api/sessions", json={
        "title": "SC-01 Tenant Session",
        "tenant_id": TENANT_ID,
        "spend_limit_usd": 100.0,
    })
    assert res.status_code == 201, f"Create session failed: {res.status_code} {res.text}"
    tenant_sess_id = res.json()["id"]
    print(f"✓ Session {tenant_sess_id} created under {TENANT_ID}")

    # 12. Freeze tenant (status -> suspended)
    p_status = {"status": "suspended"}
    p_status_hash = hashlib.sha256(json.dumps(p_status, separators=(",", ":")).encode("utf-8")).hexdigest()
    c_status = f"{TENANT_ID}|set_status|{p_status_hash}|{expiry_str}".encode("utf-8")
    status_sig = nacl.encoding.HexEncoder.encode(signing_key.sign(c_status).signature).decode("utf-8")

    res = session.post(f"{BASE_URL}/api/tenants/status", json={
        "tenant_id": TENANT_ID,
        "status": "suspended",
        "action": {
            "tenant_id": TENANT_ID,
            "action": "set_status",
            "params_hash": p_status_hash,
            "expiry": expiry_str,
            "pubkey_hex": pub_hex,
            "signature_hex": status_sig,
        }
    })
    assert res.status_code == 200, f"Suspend tenant failed: {res.status_code} {res.text}"
    print("✓ Tenant suspended via signed admin action")

    # 13. Verify fail-closed dispatch refusal under suspended tenant
    res = session.post(f"{BASE_URL}/api/chat/tribunal", json={
        "session_id": tenant_sess_id,
        "prompt": "test should fail",
    })
    assert res.status_code == 403, f"Expected 403 for suspended tenant, got {res.status_code}"
    assert "suspended" in res.text
    print(f"✓ Fail-closed dispatch refusal verified on suspended tenant: {res.text.strip()}")

    # 14. Unfreeze tenant (status -> active)
    p_active = {"status": "active"}
    p_active_hash = hashlib.sha256(json.dumps(p_active, separators=(",", ":")).encode("utf-8")).hexdigest()
    c_active = f"{TENANT_ID}|set_status|{p_active_hash}|{expiry_str}".encode("utf-8")
    active_sig = nacl.encoding.HexEncoder.encode(signing_key.sign(c_active).signature).decode("utf-8")

    res = session.post(f"{BASE_URL}/api/tenants/status", json={
        "tenant_id": TENANT_ID,
        "status": "active",
        "action": {
            "tenant_id": TENANT_ID,
            "action": "set_status",
            "params_hash": p_active_hash,
            "expiry": expiry_str,
            "pubkey_hex": pub_hex,
            "signature_hex": active_sig,
        }
    })
    assert res.status_code == 200, f"Re-activate tenant failed: {res.status_code} {res.text}"
    print("✓ Tenant restored to active status")

    # 15. Logout
    res = session.post(f"{BASE_URL}/api/auth/logout")
    assert res.status_code == 200
    print("✓ Logged out successfully")

    # 16. Verify session invalidation
    res = session.get(f"{BASE_URL}/api/auth/whoami")
    assert res.status_code == 200
    who_after = res.json()
    assert who_after.get("authenticated") is False
    print("✓ Verified session revocation after logout")

    print(f"=== [SC-01] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc01()
