#!/usr/bin/env python3
"""SC-02: Quantum Chemistry & PySCF Anchors Live Audit.
Tests multi-molecule ab-initio potential energy surface scans across
equilibrium and stretched geometries, variational bounds, and fail-closed
inputs against Python Bridge (localhost:50051) and kubonad (:8080).
"""
import sys
import grpc
import requests

sys.path.insert(0, "platform/bridge/proto")
import kubona_pb2, kubona_pb2_grpc

GRPC_TARGET = "localhost:50051"
API_BASE = "http://127.0.0.1:8080"

def run_sc02():
    print(f"=== [SC-02] Testing Ab-Initio PySCF Classical Anchors ===")
    
    channel = grpc.insecure_channel(GRPC_TARGET)
    stub = kubona_pb2_grpc.QuantumBridgeServiceStub(channel)

    test_cases = [
        # (Molecule, Bond Distance, Basis, Description)
        ("H2", 0.7414, "sto-3g", "H2 equilibrium"),
        ("H2", 1.5, "sto-3g", "H2 moderately stretched"),
        ("H2", 2.0, "sto-3g", "H2 strongly stretched (static correlation)"),
        ("LiH", 1.595, "sto-3g", "LiH equilibrium"),
        ("LiH", 2.5, "sto-3g", "LiH stretched"),
        ("BeH2", 1.326, "sto-3g", "BeH2 equilibrium"),
        ("H2O", 0.9578, "sto-3g", "H2O equilibrium"),
    ]

    for mol, r, basis, desc in test_cases:
        req = kubona_pb2.ClassicalAnchorRequest(
            molecule=mol,
            bond_distance=r,
            basis_set=basis
        )
        resp = stub.ComputeClassicalAnchor(req)
        assert resp.success, f"Anchor failed for {desc}: {resp.error_message}"
        
        # Variational Theorem: E_FCI <= E_RHF
        assert resp.fci_exact_energy <= resp.rhf_energy + 1e-7, (
            f"Variational violation for {desc}: FCI ({resp.fci_exact_energy}) > RHF ({resp.rhf_energy})"
        )
        
        corr_energy = resp.rhf_energy - resp.fci_exact_energy
        assert corr_energy >= -1e-7, f"Negative correlation energy: {corr_energy}"
        assert resp.nuclear_repulsion > 0, f"Nuclear repulsion must be positive: {resp.nuclear_repulsion}"
        
        print(f"✓ {desc:35s}: RHF={resp.rhf_energy:.6f} Ha, FCI={resp.fci_exact_energy:.6f} Ha, "
              f"ΔE_corr={corr_energy*1000:.2f} mHa, V_nn={resp.nuclear_repulsion:.4f} Ha")

    # Physical check: In H2, correlation energy must increase as bond stretches from 0.7414 to 2.0 A
    h2_eq = stub.ComputeClassicalAnchor(kubona_pb2.ClassicalAnchorRequest(molecule="H2", bond_distance=0.7414, basis_set="sto-3g"))
    h2_str = stub.ComputeClassicalAnchor(kubona_pb2.ClassicalAnchorRequest(molecule="H2", bond_distance=2.0, basis_set="sto-3g"))
    corr_eq = h2_eq.rhf_energy - h2_eq.fci_exact_energy
    corr_str = h2_str.rhf_energy - h2_str.fci_exact_energy
    assert corr_str > corr_eq, f"Static correlation should increase with stretch: {corr_str} <= {corr_eq}"
    print(f"✓ Static correlation scaling verified: H2 corr increases from {corr_eq*1000:.2f} mHa to {corr_str*1000:.2f} mHa")

    # Fail-closed check: unphysical negative or zero bond distance
    unphysical_req = kubona_pb2.ClassicalAnchorRequest(molecule="H2", bond_distance=-0.5, basis_set="sto-3g")
    unphysical_resp = stub.ComputeClassicalAnchor(unphysical_req)
    assert not unphysical_resp.success or unphysical_resp.fci_exact_energy == 0, (
        f"Fail-closed expected for R=-0.5, got success={unphysical_resp.success}"
    )
    print("✓ Fail-closed verified on unphysical negative bond distance")

    # Fail-closed check: unknown molecule
    bogus_req = kubona_pb2.ClassicalAnchorRequest(molecule="KryptoniteUnobtainium", bond_distance=1.0, basis_set="sto-3g")
    bogus_resp = stub.ComputeClassicalAnchor(bogus_req)
    assert not bogus_resp.success, f"Fail-closed expected for unknown molecule, got {bogus_resp.success}"
    print(f"✓ Fail-closed verified on unknown molecule: {bogus_resp.error_message.strip()[:60]}")

    # Now test through kubonad orchestrator :8080
    print("\n--- Testing via kubonad orchestrator :8080 ---")
    res = requests.post(f"{API_BASE}/api/sessions", json={
        "title": "SC-02 LiH Synthesis",
        "tenant_id": "default",
        "spend_limit_usd": 150.0,
        "target_molecule": "LiH",
        "fresh": True
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 201, f"Create session failed: {res.status_code} {res.text}"
    sess = res.json()
    sess_id = sess["id"]
    print(f"✓ Session {sess_id} created with target_molecule=LiH")

    # Fetch campaign state
    res = requests.get(f"{API_BASE}/api/sessions/{sess_id}/campaign", headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 200, f"Get campaign failed: {res.status_code} {res.text}"
    camp = res.json()
    q_factors = camp.get("quantum_factors", {})
    assert q_factors.get("target_molecule") == "LiH", f"Unexpected molecule in campaign: {q_factors}"
    print(f"✓ Campaign synthesized via orchestrator: target={q_factors.get('target_molecule')}, backend={q_factors.get('recommended_backend')}")

    # Re-synthesize campaign with prompt
    res = requests.post(f"{API_BASE}/api/sessions/{sess_id}/campaign/synthesize", json={
        "prompt": "LiH ground state R=1.595 A"
    }, headers={"Authorization": "Bearer dev-token"})
    assert res.status_code == 200, f"Re-synthesize failed: {res.status_code} {res.text}"
    re_camp = res.json()
    assert re_camp.get("target_system") == "LiH" or "LiH" in str(re_camp), f"Re-synthesized state missing LiH: {re_camp}"
    print("✓ Dynamic campaign re-synthesis endpoint verified")

    print(f"=== [SC-02] PASSED 100% ===\n")

if __name__ == "__main__":
    run_sc02()
