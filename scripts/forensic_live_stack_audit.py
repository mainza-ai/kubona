#!/usr/bin/env python3
"""
Kubona Live Stack Forensic Audit: Cryptographic, Physical & Real-Time Observatory Verification
=============================================================================================
Proves zero-surface causal integrity across:
Browser / Client Action -> Network Protocol -> Algorithmic Engine -> Database Mutation -> Cryptographic Ledger

Target Live Architecture:
- Go 1.24+ Core Orchestrator API Gateway: http://localhost:8080 (kubonad)
- Python 3.12+ Quantum Bridge gRPC Service: localhost:50051 (server.py)
- Next.js 16 / TypeScript Cognitive Observatory: http://localhost:3000
- Underlying Persistence: SQLite3 WAL database (platform/core/kubona.db)
"""
from __future__ import annotations

import os
import sys
import time
import json
import sqlite3
import hashlib
import signal
import subprocess
from datetime import datetime, timezone, timedelta
from typing import Dict, Any, List

import pytest
import requests
import grpc
from cryptography.hazmat.primitives.asymmetric import ed25519
from playwright.sync_api import sync_playwright

# Ensure bridge proto stubs are on sys.path
REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
BRIDGE_DIR = os.path.join(REPO_ROOT, "platform", "bridge")
PROTO_DIR = os.path.join(BRIDGE_DIR, "proto")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)
if PROTO_DIR not in sys.path:
    sys.path.insert(0, PROTO_DIR)

from proto import kubona_pb2, kubona_pb2_grpc

CORE_API_BASE = "http://localhost:8080"
OBSERVATORY_BASE = "http://localhost:3000"
GRPC_BRIDGE_HOST = "localhost:50051"
DB_PATH = os.path.join(REPO_ROOT, "platform", "core", "kubona.db")

AUDIT_LOGS: List[Dict[str, Any]] = []

def log_audit_entry(test_id: str, title: str, details: Dict[str, Any]):
    entry = {
        "test_id": test_id,
        "title": title,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        **details,
    }
    AUDIT_LOGS.append(entry)
    print(f"\n[AUDIT PASS] {test_id}: {title}")
    for k, v in details.items():
        print(f"    • {k}: {v}")


# ============================================================================
# PHASE 1: CRYPTOGRAPHIC MANDATE & MERKLE DAG STATE TRACEABILITY
# ============================================================================

def test_1_1_ap2_spend_mandate_valid_flow_and_ledger_commit():
    """
    Test 1.1: Cryptographic AP2 Spend Mandate — Valid Flow & Ledger Commit
    """
    t0 = time.time()
    # 1. Generate authentic Ed25519 keypair
    priv_key = ed25519.Ed25519PrivateKey.generate()
    pub_key = priv_key.public_key()
    pub_bytes = pub_key.public_bytes_raw()
    pub_hex = pub_bytes.hex()
    assert len(pub_hex) == 64, "Public key must be 32 bytes (64 hex characters)"

    tenant_id = f"tenant-forensic-{int(time.time()*1000)}"
    job_hash = f"sha256:{hashlib.sha256(b'forensic-vqe-circuit-01').hexdigest()[:28]}"
    max_budget_usd = 18.5000
    expiry_dt = (datetime.now(timezone.utc) + timedelta(hours=1))
    expiry_str = expiry_dt.strftime("%Y-%m-%dT%H:%M:%SZ")

    # Canonical payload: fmt.Sprintf("%s:%s:%.4f:%s", TenantID, JobHash, MaxBudgetUSD, ExpiryTimestamp)
    canonical_payload = f"{tenant_id}:{job_hash}:{max_budget_usd:.4f}:{expiry_str}"
    sig_bytes = priv_key.sign(canonical_payload.encode("utf-8"))
    sig_hex = sig_bytes.hex()
    assert len(sig_hex) == 128, "Ed25519 signature must be 64 bytes (128 hex characters)"

    mandate_payload = {
        "tenant_id": tenant_id,
        "job_hash": job_hash,
        "max_budget_usd": max_budget_usd,
        "expiry_timestamp": expiry_str,
        "public_key_hex": pub_hex,
        "signature_hex": sig_hex,
    }

    # Dispatch to Go Core API Gateway
    http_t0 = time.time()
    resp = requests.post(f"{CORE_API_BASE}/api/mandates/verify", json=mandate_payload, timeout=10)
    http_latency_ms = (time.time() - http_t0) * 1000.0

    assert resp.status_code == 200, f"Expected 200 OK, got {resp.status_code}: {resp.text}"
    body = resp.json()
    assert body.get("valid") is True, f"Mandate expected valid, got {body}"

    # Direct SQLite Database Inspection
    conn = sqlite3.connect(DB_PATH)
    cur = conn.cursor()
    cur.execute(
        """SELECT id, session_id, event_type, payload, prev_hash, entry_hash, record_hash, signer_pubkey_hex, signature_hex
           FROM ledger_events
           WHERE event_type = 'mandate_verified_client' AND session_id = ?
           ORDER BY id DESC LIMIT 1""",
        (tenant_id,),
    )
    row = cur.fetchone()
    assert row is not None, f"Expected new row in ledger_events for tenant {tenant_id}"
    (
        row_id,
        stored_session,
        stored_event,
        stored_payload_raw,
        prev_h,
        entry_h,
        rec_h,
        signer_pub,
        stored_sig,
    ) = row

    stored_payload = json.loads(stored_payload_raw)
    assert stored_payload.get("tenant_id") == tenant_id
    assert stored_payload.get("job_hash") == job_hash
    assert stored_payload.get("public_key_hex") == pub_hex
    assert stored_payload.get("signature_hex") == sig_hex

    # Bit-for-bit cryptographic hash chain recomputation
    chained_data = f"{prev_h}|{stored_session}|{stored_event}|{stored_payload_raw}".encode("utf-8")
    recomputed_digest = hashlib.sha256(chained_data).hexdigest()
    assert recomputed_digest == entry_h, f"Entry hash mismatch: recomputed {recomputed_digest} vs db {entry_h}"
    assert entry_h == rec_h, f"record_hash must match entry_hash bit-for-bit: {rec_h} vs {entry_h}"

    conn.close()

    log_audit_entry(
        "TEST 1.1",
        "Cryptographic AP2 Spend Mandate & Merkle DAG Commit",
        {
            "http_status": resp.status_code,
            "http_latency_ms": round(http_latency_ms, 2),
            "tenant_id": tenant_id,
            "job_hash": job_hash,
            "public_key_hex": pub_hex,
            "signature_hex": sig_hex[:32] + "...",
            "database_row_id": row_id,
            "database_entry_hash": entry_h,
            "recomputed_sha256": recomputed_digest,
            "hash_match_bit_for_bit": True,
        },
    )


def test_1_2_signature_forgery_and_tamper_rejection():
    """
    Test 1.2: Signature Forgery & Tamper Rejection (Fail-Closed Enforcement)
    """
    priv_key = ed25519.Ed25519PrivateKey.generate()
    pub_hex = priv_key.public_key().public_bytes_raw().hex()

    tenant_id = f"tenant-tamper-{int(time.time()*1000)}"
    job_hash = "sha256:authentic_job_hash_12345"
    max_budget_usd = 20.0000
    expiry_str = (datetime.now(timezone.utc) + timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")

    canonical_payload = f"{tenant_id}:{job_hash}:{max_budget_usd:.4f}:{expiry_str}"
    sig_hex = priv_key.sign(canonical_payload.encode("utf-8")).hex()

    # Tamper action: Mutate job_hash by 1 character while keeping signature unchanged
    tampered_job_hash = "sha256:authentic_job_hash_1234X"
    tampered_mandate = {
        "tenant_id": tenant_id,
        "job_hash": tampered_job_hash,
        "max_budget_usd": max_budget_usd,
        "expiry_timestamp": expiry_str,
        "public_key_hex": pub_hex,
        "signature_hex": sig_hex,
    }

    # Pre-condition: count rows in ledger_events
    conn = sqlite3.connect(DB_PATH)
    cur = conn.cursor()
    cur.execute("SELECT count(*) FROM ledger_events")
    n0 = cur.fetchone()[0]

    resp = requests.post(f"{CORE_API_BASE}/api/mandates/verify", json=tampered_mandate, timeout=10)
    cur.execute("SELECT count(*) FROM ledger_events")
    n_post = cur.fetchone()[0]
    conn.close()

    assert n_post == n0, f"Fail-closed violation: {n_post - n0} new row(s) committed on forged mandate!"
    assert resp.status_code in (200, 400)
    body = resp.json()
    assert body.get("valid") is False
    assert "cryptographic signature verification failed" in body.get("error", "")

    log_audit_entry(
        "TEST 1.2",
        "Signature Forgery & Tamper Rejection (Fail-Closed)",
        {
            "http_status": resp.status_code,
            "error_message": body.get("error"),
            "ledger_rows_before": n0,
            "ledger_rows_after": n_post,
            "new_rows_committed": 0,
            "tampered_job_hash": tampered_job_hash,
        },
    )


def test_1_3_ap2_spend_ceiling_clamping_and_rejection():
    """
    Test 1.3: AP2 Spend Ceiling Clamping & Rejection
    """
    # 1. Setup: Create a test session with spend_limit_usd = 10.00
    r_sess = requests.post(
        f"{CORE_API_BASE}/api/sessions",
        json={"title": "Forensic Budget Session", "spend_limit_usd": 10.00},
        timeout=10,
    )
    assert r_sess.status_code == 201, f"Failed to create test session: {r_sess.text}"
    sess = r_sess.json()
    session_id = sess["id"]
    spend_limit = sess["spend_limit_usd"]
    assert spend_limit == 10.00

    # 2. Action: Issue a signed mandate requesting max_budget_usd = 25.00
    priv_key = ed25519.Ed25519PrivateKey.generate()
    pub_hex = priv_key.public_key().public_bytes_raw().hex()
    job_hash = "sha256:exceeded_budget_job_999"
    requested_budget = 25.0000
    expiry_str = (datetime.now(timezone.utc) + timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")

    canonical_payload = f"{session_id}:{job_hash}:{requested_budget:.4f}:{expiry_str}"
    sig_hex = priv_key.sign(canonical_payload.encode("utf-8")).hex()

    mandate = {
        "tenant_id": session_id,
        "job_hash": job_hash,
        "max_budget_usd": requested_budget,
        "expiry_timestamp": expiry_str,
        "public_key_hex": pub_hex,
        "signature_hex": sig_hex,
    }

    r_verify = requests.post(f"{CORE_API_BASE}/api/mandates/verify", json=mandate, timeout=10)
    body = r_verify.json()
    assert body.get("valid") is False
    assert "requested spend exceeds authorized budget ceiling" in body.get("error", "")

    # Query session balance in kubona.db
    conn = sqlite3.connect(DB_PATH)
    cur = conn.cursor()
    cur.execute("SELECT spend_limit_usd, current_spend_usd FROM sessions WHERE id = ?", (session_id,))
    row = cur.fetchone()
    conn.close()
    assert row is not None
    db_limit, db_current = row
    assert db_current == 0.00, f"Current spend should not increase on rejected mandate, got {db_current}"

    log_audit_entry(
        "TEST 1.3",
        "AP2 Spend Ceiling Clamping & Rejection",
        {
            "session_id": session_id,
            "session_spend_limit_usd": db_limit,
            "requested_budget_usd": requested_budget,
            "rejection_error": body.get("error"),
            "current_spend_usd_post_rejection": db_current,
            "clamped_successfully": True,
        },
    )


# ============================================================================
# PHASE 2: PHYSICAL & MATHEMATICAL COMPUTING VERIFICATION
# ============================================================================

def test_2_1_electronic_structure_chemistry_rhf_fci():
    """
    Test 2.1: Electronic Structure Chemistry — Hartree-Fock & FCI Convergence
    """
    channel = grpc.insecure_channel(GRPC_BRIDGE_HOST)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    t0 = time.time()
    req = kubona_pb2.ClassicalAnchorRequest(
        molecule="H2",
        bond_distance=0.7414,
        basis_set="sto-3g",
    )
    resp = stub.ComputeClassicalAnchor(req, timeout=15)
    grpc_latency_ms = (time.time() - t0) * 1000.0

    assert resp.success is True, f"Classical anchor failed: {resp.error_message}"

    e_rhf = resp.rhf_energy
    e_fci = resp.fci_exact_energy
    e_nuc = resp.nuclear_repulsion
    delta_corr = e_fci - e_rhf

    # Mathematical & Physical Assertions
    assert -1.1175 <= e_rhf <= -1.1160, f"E_RHF={e_rhf} out of range [-1.1175, -1.1160]"
    assert -1.1380 <= e_fci <= -1.1365, f"E_FCI={e_fci} out of range [-1.1380, -1.1365]"
    assert delta_corr < 0.0, f"Correlation energy must be strictly negative, got {delta_corr}"
    assert abs(delta_corr - (-0.0206)) < 0.001, f"Correlation energy ~ -0.0206 Ha, got {delta_corr}"
    # Nuclear repulsion: 1 / (0.7414 * 1.889726) ~ 0.71375 Ha
    assert abs(e_nuc - 0.7137) < 0.0015, f"Nuclear repulsion V_NN={e_nuc} out of range 0.7137 ± 0.001"

    log_audit_entry(
        "TEST 2.1",
        "Electronic Structure PySCF RHF & FCI Convergence",
        {
            "grpc_roundtrip_ms": round(grpc_latency_ms, 2),
            "molecule": resp.molecule,
            "bond_distance_angstrom": resp.bond_distance,
            "e_rhf_hartree": round(e_rhf, 7),
            "e_fci_hartree": round(e_fci, 7),
            "delta_correlation_hartree": round(delta_corr, 7),
            "nuclear_repulsion_hartree": round(e_nuc, 7),
            "orbital_energies": [round(x, 4) for x in resp.orbital_energies],
        },
    )


def test_2_2_quantum_circuit_execution_and_entanglement_invariant():
    """
    Test 2.2: Quantum Circuit Execution & Entanglement Invariant
    """
    channel = grpc.insecure_channel(GRPC_BRIDGE_HOST)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    bell_qasm = (
        'OPENQASM 2.0;\n'
        'include "qelib1.inc";\n'
        'qreg q[2];\n'
        'creg c[2];\n'
        'h q[0];\n'
        'cx q[0], q[1];\n'
        'measure q[0] -> c[0];\n'
        'measure q[1] -> c[1];\n'
    )

    t0 = time.time()
    req = kubona_pb2.CircuitExecutionRequest(
        job_id=f"job-bell-{int(time.time()*1000)}",
        qasm=bell_qasm,
        shots=1024,
        requested_backend="aer_simulator",
        allow_classical_fallback=True,
    )
    resp = stub.ExecuteCircuit(req, timeout=15)
    grpc_latency_ms = (time.time() - t0) * 1000.0

    assert resp.success is True, f"ExecuteCircuit failed: {resp.error_message}"
    assert resp.execution_time_ms > 0.0, "Reported execution time must be > 0.0 ms"

    counts = dict(resp.counts)
    total_shots = sum(counts.values())
    assert total_shots == 1024, f"Expected 1024 shots, got {total_shots}"

    p00 = counts.get("00", 0) / float(total_shots)
    p11 = counts.get("11", 0) / float(total_shots)
    p01 = counts.get("01", 0) / float(total_shots)
    p10 = counts.get("10", 0) / float(total_shots)

    # |Phi+> parity invariant: P(00) + P(11) >= 0.999, P(01) == 0, P(10) == 0
    assert (p00 + p11) >= 0.999, f"Bell parity violated: P(00)+P(11)={p00+p11}"
    assert p01 == 0.0, f"Unphysical cross-talk: P(01)={p01}"
    assert p10 == 0.0, f"Unphysical cross-talk: P(10)={p10}"

    # Meyer-Wallach Entanglement Measure Q >= 0.99
    mw = resp.meyer_wallach_entanglement
    assert mw >= 0.99, f"Meyer-Wallach entanglement Q={mw} < 0.99"

    log_audit_entry(
        "TEST 2.2",
        "Qiskit Aer Bell State Execution & Meyer-Wallach Entanglement",
        {
            "grpc_roundtrip_ms": round(grpc_latency_ms, 2),
            "backend": resp.resolved_backend,
            "shots": total_shots,
            "counts": counts,
            "p_00_plus_p_11": round(p00 + p11, 4),
            "p_01": p01,
            "p_10": p10,
            "meyer_wallach_Q": round(mw, 6),
            "engine_execution_time_ms": round(resp.execution_time_ms, 2),
        },
    )


def test_2_3_circuit_wire_cutting_subcircuit_recombination():
    """
    Test 2.3: Circuit Wire Cutting & Subcircuit Recombination
    """
    channel = grpc.insecure_channel(GRPC_BRIDGE_HOST)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    # 8-qubit entangled GHZ circuit
    ghz_8_qasm = (
        'OPENQASM 2.0;\n'
        'include "qelib1.inc";\n'
        'qreg q[8];\n'
        'creg c[8];\n'
        'h q[0];\n'
        'cx q[0], q[1];\n'
        'cx q[1], q[2];\n'
        'cx q[2], q[3];\n'
        'cx q[3], q[4];\n'
        'cx q[4], q[5];\n'
        'cx q[5], q[6];\n'
        'cx q[6], q[7];\n'
    )

    t0 = time.time()
    req = kubona_pb2.CircuitCutRequest(
        qasm=ghz_8_qasm,
        max_subcircuit_qubits=4,
    )
    resp = stub.CutCircuit(req, timeout=15)
    grpc_latency_ms = (time.time() - t0) * 1000.0

    assert resp.success is True, f"CutCircuit failed: {resp.error_message}"
    assert len(resp.fragments) >= 2, f"Expected >= 2 fragments, got {len(resp.fragments)}"
    assert resp.total_cuts >= 1, f"Expected total_cuts >= 1, got {resp.total_cuts}"

    fragment_details = []
    from qiskit.qasm2 import loads
    for f in resp.fragments:
        qc = loads(f.qasm)
        assert qc.num_qubits <= 4, f"Fragment {f.fragment_id} exceeded max qubits: {qc.num_qubits} > 4"
        fragment_details.append({
            "id": f.fragment_id,
            "qubits": qc.num_qubits,
            "cut_qubits": list(f.cut_qubits),
        })

    log_audit_entry(
        "TEST 2.3",
        "PennyLane QPD Circuit Wire Cutting (8q -> <=4q fragments)",
        {
            "grpc_roundtrip_ms": round(grpc_latency_ms, 2),
            "original_qubits": 8,
            "max_subcircuit_qubits": 4,
            "total_cuts": resp.total_cuts,
            "fragment_count": len(resp.fragments),
            "fragments": fragment_details,
        },
    )


# ============================================================================
# PHASE 3: MULTI-AGENT SWARM & EPISTEMIC EVALUATION STATE
# ============================================================================

def test_3_1_epistemic_wavefunction_claim_verification():
    """
    Test 3.1: Epistemic Wavefunction Claim Verification
    """
    channel = grpc.insecure_channel(GRPC_BRIDGE_HOST)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    e_quantum = -1.1370
    e_classical = -1.1373
    delta_mha = abs(e_quantum - e_classical) * 1000.0

    t0 = time.time()
    req = kubona_pb2.ClaimEvaluationRequest(
        claim_id=f"claim-h2-{int(time.time()*1000)}",
        statement="Ground state energy of H2 at equilibrium.",
        quantum_value=e_quantum,
        classical_anchor_value=e_classical,
        source_platforms=["pyscf", "qiskit", "arxiv", "chemrxiv"],
        total_engagement=5000,
        contradiction_signal=0.0,
    )
    resp = stub.EvaluateWavefunction(req, timeout=10)
    grpc_latency_ms = (time.time() - t0) * 1000.0

    assert abs(delta_mha - 0.3) < 1e-4, f"Expected 0.3 mHa, got {delta_mha}"
    assert delta_mha <= 1.6, f"Expected <= 1.6 mHa (Chemical Accuracy), got {delta_mha}"
    assert resp.passes_chemical_accuracy is True
    assert 0.65 <= resp.corroborated_amplitude <= 1.0, (
        f"Corroborated amplitude {resp.corroborated_amplitude} out of bound [0.65, 1.0]"
    )

    log_audit_entry(
        "TEST 3.1",
        "Epistemic Wavefunction Credibility Collapse",
        {
            "grpc_roundtrip_ms": round(grpc_latency_ms, 2),
            "claim_id": resp.claim_id,
            "e_quantum_ha": e_quantum,
            "e_classical_ha": e_classical,
            "delta_mha": round(resp.delta_mha, 4),
            "passes_chemical_accuracy": resp.passes_chemical_accuracy,
            "corroborated_amplitude_rho": round(resp.corroborated_amplitude, 6),
            "contested_amplitude": round(resp.contested_amplitude, 6),
            "unverified_amplitude": round(resp.unverified_amplitude, 6),
            "verdict": resp.verdict_reason,
        },
    )


def test_3_2_multi_agent_tribunal_deliberation_state():
    """
    Test 3.2: Multi-Agent Tribunal Deliberation State
    """
    t0 = time.time()
    resp = requests.post(
        f"{CORE_API_BASE}/api/chat/tribunal",
        json={"target": "H2", "prompt": "Evaluate VQE ansatz for hydrogen dissociation"},
        timeout=35,
    )
    latency_ms = (time.time() - t0) * 1000.0

    assert resp.status_code == 200, f"Tribunal failed: {resp.status_code}: {resp.text}"
    data = resp.json()
    trib = data.get("tribunal", {})
    turns = trib.get("turns", [])
    assert len(turns) == 4, f"Expected 4 specialist turns, got {len(turns)}"

    turn_roles = [t.get("role") for t in turns]
    assert "ansatz-scout" in turn_roles
    assert "disconfirmation-officer" in turn_roles
    assert "device-auditor" in turn_roles
    assert "epistemic-scribe" in turn_roles

    assert isinstance(trib.get("consensus_passed"), bool)
    assert trib.get("consensus_passed") is True

    prop = trib.get("proposal_json", {})
    assert prop is not None
    assert isinstance(prop.get("qasm"), str) and len(prop["qasm"]) > 0

    log_audit_entry(
        "TEST 3.2",
        "Live 4-Specialist Collective Tribunal Debate",
        {
            "http_status": resp.status_code,
            "roundtrip_latency_ms": round(latency_ms, 2),
            "target": trib.get("target"),
            "molecule": trib.get("molecule"),
            "consensus_passed": trib.get("consensus_passed"),
            "turns_executed": [
                {"name": t["name"], "role": t["role"], "verdict": t["verdict"]}
                for t in turns
            ],
            "proposal_has_valid_qasm": True,
            "qasm_snippet": prop["qasm"].splitlines()[0],
        },
    )


# ============================================================================
# PHASE 4: FULL BROWSER-TO-DATABASE TRACEABILITY (PLAYWRIGHT)
# ============================================================================

def test_4_1_live_cockpit_spend_approval_and_database_reconciliation():
    """
    Test 4.1: Live Cockpit Spend Approval & Database Reconciliation
    """
    captured_requests = []
    captured_responses = []

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        def on_req(req):
            if "/api/mandates/verify" in req.url:
                captured_requests.append(req.post_data_json)

        def on_res(res):
            if "/api/mandates/verify" in res.url:
                try:
                    captured_responses.append(res.json())
                except Exception:
                    pass

        page.on("request", on_req)
        page.on("response", on_res)

        page.goto(OBSERVATORY_BASE, wait_until="networkidle")

        # Switch to Sentinel Guardrails
        tab_btn = page.locator("#tab-surface4")
        tab_btn.click()
        page.wait_for_selector("text=Approve Simulation Run", timeout=6000)

        # Step 1 Pre-condition: Record N0 immediately before browser click action
        conn = sqlite3.connect(DB_PATH)
        cur = conn.cursor()
        cur.execute("SELECT count(*) FROM ledger_events")
        n0 = cur.fetchone()[0]

        # Step 2 Browser Action: Click Approve Simulation Run
        btn = page.get_by_role("button", name="Approve Simulation Run")
        btn.click()

        # Wait for dispatch and ledger recording
        page.wait_for_timeout(2000)
        browser.close()

    assert len(captured_requests) >= 1, "Expected browser to dispatch /api/mandates/verify"
    wire_mandate = captured_requests[0]
    sig_hex = wire_mandate.get("signature_hex", "")
    job_hash = wire_mandate.get("job_hash", "")
    assert len(sig_hex) == 128, f"Signature hex must be 64 bytes (128 characters), got {len(sig_hex)}"

    # Post-condition Database Verification
    cur.execute("SELECT count(*) FROM ledger_events")
    n_post = cur.fetchone()[0]
    assert n_post == n0 + 1, f"Expected exactly N0+1 ledger events ({n0+1}), got {n_post}"

    cur.execute("SELECT payload_json FROM ledger_events ORDER BY id DESC LIMIT 1")
    stored_payload_json = cur.fetchone()[0]
    stored = json.loads(stored_payload_json)
    conn.close()

    assert stored.get("job_hash") == job_hash, f"Stored job_hash {stored.get('job_hash')} != emitted {job_hash}"
    assert stored.get("signature_hex") == sig_hex, f"Stored signature_hex != emitted signature_hex"

    log_audit_entry(
        "TEST 4.1",
        "Full Cockpit UI-to-Database Spend Mandate Reconciliation",
        {
            "sqlite_events_n0": n0,
            "sqlite_events_n_post": n_post,
            "delta_rows": n_post - n0,
            "emitted_signature_hex_len": len(sig_hex),
            "wire_job_hash": job_hash,
            "database_stored_job_hash": stored.get("job_hash"),
            "database_stored_signature": stored.get("signature_hex")[:32] + "...",
            "exact_byte_and_hash_match": True,
        },
    )


def test_4_2_realtime_telemetry_synchronization_fleet_radar():
    """
    Test 4.2: Real-Time Telemetry Synchronization (Fleet Radar)
    """
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()
        page.goto(OBSERVATORY_BASE, wait_until="networkidle")

        # Switch to tab Fleet Radar (⌘6)
        radar_btn = page.locator("#tab-surface5")
        radar_btn.click()

        ind_8080 = page.locator('[data-testid="status-indicator-8080"]')
        ind_50051 = page.locator('[data-testid="status-indicator-50051"]')

        ind_8080.wait_for(timeout=5000)
        ind_50051.wait_for(timeout=5000)

        initial_8080_text = ind_8080.inner_text()
        initial_50051_text = ind_50051.inner_text()
        assert "LISTENING" in initial_8080_text, f"Port 8080 expected LISTENING, got {initial_8080_text}"
        assert "LISTENING" in initial_50051_text, f"Port 50051 expected LISTENING, got {initial_50051_text}"

        # 1. Terminate the Python bridge process (targeting the LISTEN daemon only)
        out = subprocess.check_output(["lsof", "-ti", ":50051", "-sTCP:LISTEN"]).decode().strip()
        pids = [int(x) for x in out.split() if x]
        assert len(pids) > 0, "No Python bridge process found on port 50051"
        for pid in pids:
            try:
                os.kill(pid, signal.SIGTERM)
            except ProcessLookupError:
                pass

        # 2. Assert within 3000ms the browser updates indicator to OFFLINE / DISCONNECTED
        t0_dis = time.time()
        ind_50051.filter(has_text="OFFLINE").wait_for(timeout=3000)
        elapsed_disconnect_ms = (time.time() - t0_dis) * 1000.0
        disconnected_text = ind_50051.inner_text()
        assert "OFFLINE" in disconnected_text or "DISCONNECTED" in disconnected_text

        # 3. Restart the Python bridge service
        proc = subprocess.Popen(
            [os.path.join(REPO_ROOT, "platform", ".venv", "bin", "python3"), "server.py"],
            cwd=BRIDGE_DIR,
        )

        # 4. Assert within 3000ms the browser updates indicator back to LISTENING / SOVEREIGN
        t0_rec = time.time()
        ind_50051.filter(has_text="LISTENING").wait_for(timeout=3000)
        elapsed_reconnect_ms = (time.time() - t0_rec) * 1000.0
        reconnected_text = ind_50051.inner_text()
        assert "LISTENING" in reconnected_text or "SOVEREIGN" in reconnected_text

        browser.close()

    log_audit_entry(
        "TEST 4.2",
        "Real-Time Telemetry Radar Synchronization (Kill & Recover)",
        {
            "port_8080_status": initial_8080_text,
            "port_50051_initial_status": initial_50051_text,
            "killed_bridge_pids": pids,
            "disconnect_detected_in_ms": round(elapsed_disconnect_ms, 2),
            "disconnected_ui_text": disconnected_text,
            "restarted_bridge_pid": proc.pid,
            "recovery_detected_in_ms": round(elapsed_reconnect_ms, 2),
            "recovered_ui_text": reconnected_text,
            "sla_within_3000ms": elapsed_disconnect_ms <= 3000 and elapsed_reconnect_ms <= 3000,
        },
    )


# ============================================================================
# HARNESS MAIN EXECUTION & FORMATTED AUDIT REPORT
# ============================================================================

def run_all_tests():
    print("=" * 80)
    print("      KUBONA MASTER FORENSIC AUDIT: ZERO-SURFACE LIVE-STACK VERIFICATION")
    print("=" * 80)
    start_time = time.time()

    tests = [
        ("Phase 1 - Test 1.1", test_1_1_ap2_spend_mandate_valid_flow_and_ledger_commit),
        ("Phase 1 - Test 1.2", test_1_2_signature_forgery_and_tamper_rejection),
        ("Phase 1 - Test 1.3", test_1_3_ap2_spend_ceiling_clamping_and_rejection),
        ("Phase 2 - Test 2.1", test_2_1_electronic_structure_chemistry_rhf_fci),
        ("Phase 2 - Test 2.2", test_2_2_quantum_circuit_execution_and_entanglement_invariant),
        ("Phase 2 - Test 2.3", test_2_3_circuit_wire_cutting_subcircuit_recombination),
        ("Phase 3 - Test 3.1", test_3_1_epistemic_wavefunction_claim_verification),
        ("Phase 3 - Test 3.2", test_3_2_multi_agent_tribunal_deliberation_state),
        ("Phase 4 - Test 4.1", test_4_1_live_cockpit_spend_approval_and_database_reconciliation),
        ("Phase 4 - Test 4.2", test_4_2_realtime_telemetry_synchronization_fleet_radar),
    ]

    for label, test_func in tests:
        step_start = time.time()
        print(f"\n---> Executing {label} [{test_func.__name__}]...")
        test_func()
        step_dur = time.time() - step_start
        print(f"---> {label} PASSED ({step_dur:.2f}s)")

    total_time = time.time() - start_time
    print("\n" + "=" * 80)
    print(f"      FORENSIC AUDIT COMPLETE: ALL 10 TESTS VERIFIED ({total_time:.2f}s runtime)")
    print("=" * 80)
    assert total_time <= 45.0, f"Execution harness exceeded 45s SLA threshold ({total_time:.2f}s > 45s)"


if __name__ == "__main__":
    run_all_tests()
