"""Phase 0 & Phase 1 Circuit Codesign durability and CircuitClaim C0-C7 tests."""

from __future__ import annotations

import sys
from pathlib import Path

from kubona.codesign.claims import (
    CircuitClaim,
    canonical_circuit_sha256,
    evaluate_circuit_gates,
    sign_discovery_claim_ed25519,
)

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)


def test_phase0_op_evaluate_emits_hashes_and_tool_versions():
    from codesign_ops import op_evaluate

    art = [["ccx", [0, 1, 2], []]]
    out = op_evaluate({"spec_kind": "and2", "artifact": art, "num_tests": 16, "kappa": 1, "seed": 7})
    assert out["ok"] is True
    assert out["artifact_sha256"] == canonical_circuit_sha256(art)
    assert len(out["spec_table_sha256"]) == 64
    assert "python" in out["tool_versions"]
    assert out["result"]["valid"] is True
    assert out["result"]["verification_method"] == "exhaustive"
    assert out["result"]["eps_bound_97_5"] == 0.0


def test_phase1_circuit_claim_c0_to_c7_refusals_and_ed25519_signoff():
    # 1. Wrong-output circuit fails C1 with not_equivalent
    bad = CircuitClaim(
        claim_id="cc_bad",
        code_name="bad_and2",
        spec_kind="and2",
        artifact=[["cx", [0, 2], []]],
        baseline_tq=10,
    )
    rep_bad = evaluate_circuit_gates(bad, catalog={})
    assert rep_bad["all_deterministic_passed"] is False
    assert rep_bad["failure_code"] == "not_equivalent"

    # 2. Tampered artifact hash fails C0 with artifact_mismatch
    good_art = [["ccx", [0, 1, 2], []]]
    tampered = CircuitClaim(
        claim_id="cc_tamper",
        code_name="tampered_and2",
        spec_kind="and2",
        artifact=good_art,
        baseline_tq=10,
        expected_artifact_sha256="0" * 64,
    )
    rep_tamper = evaluate_circuit_gates(tampered, catalog={})
    assert rep_tamper["failure_code"] == "artifact_mismatch"

    # 3. Known published canonical 1-CCX and2 is refused at C6 (known)
    known = CircuitClaim(
        claim_id="cc_known",
        code_name="and2_canonical",
        spec_kind="and2",
        artifact=good_art,
        baseline_tq=10,
    )
    rep_known = evaluate_circuit_gates(known)
    assert rep_known["failed_gate"] == "gate6_novel"
    assert rep_known["failure_code"] == "known"

    # 4. Novel exhaustive circuit vs empty catalog -> verified -> Ed25519 claimed
    novel = CircuitClaim(
        claim_id="cc_novel_1",
        code_name="and2_novel",
        spec_kind="and2",
        artifact=good_art,
        baseline_tq=10,
        seed=7,
        repro_seed=11,
    )
    rep1 = evaluate_circuit_gates(novel, catalog={})
    assert rep1["all_deterministic_passed"] is True
    assert novel.status == "verified"

    pub_hex, sig_hex = sign_discovery_claim_ed25519(novel.claim_id, novel.artifact_sha256, novel.gates_digest, "22" * 32)
    novel.operator_pubkey_hex = pub_hex
    novel.operator_signature_hex = sig_hex
    rep2 = evaluate_circuit_gates(novel, catalog={})
    assert rep2["claimed"] is True
    assert novel.status == "claimed"


def test_phase1_manuscript_table_renders_circuit_claim_row():
    from manuscript_ops import _results_table

    tex = _results_table([
        {
            "job_id": "cs_12345678",
            "kind": "circuit_claim",
            "toffoli": 1,
            "peak_qubits": 3,
            "tq": 3,
            "verification_method": "exhaustive",
            "backend": "kickmix",
        }
    ])
    assert r"1T $\times$ 3Q" in tex
    assert "TQ=3" in tex
    assert r"exhaustive ($\varepsilon=0$)" in tex


def test_r1_canonical_novelty_defeats_identity_padding_and_commutation():
    from kubona.codesign.claims import lookup_circuit_catalog

    # Identity-padded ccx + x + x reduces to canonical and2_single_ccx
    padded = [["ccx", [0, 1, 2], []], ["x", [0], []], ["x", [0], []]]
    hit = lookup_circuit_catalog("and2", padded)
    assert hit["known"] is True
    assert hit["matched_name"] == "and2_single_ccx"

    # Commuted XOR2 cx order reduces to canonical xor2_two_cx
    commuted_xor = [["cx", [1, 2], []], ["cx", [0, 2], []]]
    hit_xor = lookup_circuit_catalog("xor2", commuted_xor)
    assert hit_xor["known"] is True
    assert hit_xor["matched_name"] == "xor2_two_cx"
