"""Phase 4 (QEC cross-domain link) & Phase 5 (pack promotion & housekeeping) tests."""

from __future__ import annotations

import math
import sys
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import codesign_ops  # noqa: E402
from kubona.codesign.qec_joint import (  # noqa: E402
    FUNSEARCH_5_1_3_CNOT_BENCHMARK,
    build_joint_qec_codesign_claim,
    verify_five_qubit_logical_gate,
    verify_magic_state_factory,
    verify_steane_logical_cnot,
)
from kubona.codesign.specs import NAMED_KINDS, build_spec  # noqa: E402


def test_phase4_steane_logical_cnot_and_adversarial_rejection():
    # Transversal 7-CNOT logical CNOT on Steane [[7,1,3]] passes Stim stabilizer + single-fault checks
    res = verify_steane_logical_cnot()
    assert res["ok"] is True
    assert res["passed"] is True
    assert res["stabilizers_preserved"] is True
    assert res["logical_action_exact"] is True
    assert res["single_fault_check"]["scope"] == "all_qubits_every_step_pre_and_post"
    assert res["single_fault_check"]["faults_tested"] == (7 + 1) * 14 * 3
    assert res["single_fault_check"]["undetectable_logical_faults"] == 0
    assert res["depth"] == 1
    assert len(res["artifact_sha256"]) == 64

    # Adversarial: missing one CNOT breaks stabilizers and logical action
    broken = verify_steane_logical_cnot([("cx", [i, i + 7], []) for i in range(6)])
    assert broken["passed"] is False
    assert broken["stabilizers_preserved"] is False

    # Named spec is registered and buildable
    assert "steane-cnot" in NAMED_KINDS
    spec = build_spec("steane-cnot")
    assert spec.num_qubits == 14


def test_phase4_five_qubit_logical_gate_and_naive_cnot_refusal():
    # Transversal H*S on [[5,1,3]] passes exact Stim check
    res = verify_five_qubit_logical_gate()
    assert res["ok"] is True
    assert res["passed"] is True
    assert res["stabilizers_preserved"] is True
    assert res["logical_action_exact"] is True
    assert res["single_fault_check"]["undetectable_logical_faults"] == 0
    assert res["benchmark"]["funsearch_depth"] == FUNSEARCH_5_1_3_CNOT_BENCHMARK["funsearch_depth"] == 9
    assert res["benchmark"]["baseline_depth"] == 84
    assert res["benchmark"]["reproduced_in_repo"] is False
    assert res["benchmark"]["status"] == "reference_only_not_reproduced"

    # Adversarial: naive transversal CNOT on non-CSS [[5,1,3]] does NOT preserve stabilizers
    naive_cnot = [("cx", [i, i + 5], []) for i in range(5)]
    bad = verify_five_qubit_logical_gate(naive_cnot, gate="logical_cnot")
    assert bad["passed"] is False
    assert bad["stabilizers_preserved"] is False


def test_phase4_magic_state_factory_and_joint_claim():
    fac = verify_magic_state_factory()
    assert fac["ok"] is True
    assert fac["method"] == "exact_distinct_nonzero_syndromes"
    assert fac["distance_ge_3"] is True
    assert math.isclose(fac["gamma"], math.log(15) / math.log(3), rel_tol=1e-5)
    assert "jain2026_1715_287_6" in fac["s5_reference_targets"]

    steane = verify_steane_logical_cnot()
    joint = build_joint_qec_codesign_claim(
        claim_id="joint_steane_01",
        qec_code_name="steane_7_1_3",
        qec_code_record={"n": 7, "k": 1, "d": 3, "d_kind": "proved"},
        stim_verification=steane,
        factory_verification=fac,
    )
    assert joint["ready_for_signoff"] is True
    assert len(joint["bundle_sha256"]) == 64

    # Heuristic distance fails joint sign-off gate
    not_ready = build_joint_qec_codesign_claim(
        claim_id="joint_steane_02",
        qec_code_name="steane_7_1_3",
        qec_code_record={"n": 7, "k": 1, "d": 3, "d_kind": "heuristic"},
        stim_verification=steane,
        factory_verification=fac,
    )
    assert not_ready["ready_for_signoff"] is False
    assert not_ready["gates"]["qec_distance_proved"] is False


def test_phase5_pack_promotion_refusal_and_persistence(tmp_path, monkeypatch):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))

    # Refuses missing anchor_ref or proof_ref
    refused = codesign_ops.run_op("pack_promote", {"name": "numerics", "anchor_ref": "evt_1"})
    assert refused["ok"] is False

    # Succeeds with both refs and persists into op_packs
    ok = codesign_ops.run_op(
        "pack_promote",
        {"name": "numerics", "anchor_ref": "ledger:anchor:42", "proof_ref": "ledger:proof:99"},
    )
    assert ok["ok"] is True
    assert ok["pack"]["status"] == "active"
    assert ok["pack"]["anchor_ref"] == "ledger:anchor:42"
    assert ok["pack"]["proof_ref"] == "ledger:proof:99"

    listed = codesign_ops.run_op("packs", {})
    assert listed["ok"] is True
    num_pack = next(p for p in listed["packs"] if p["name"] == "numerics")
    assert num_pack["status"] == "active"
    assert num_pack["anchor_ref"] == "ledger:anchor:42"
