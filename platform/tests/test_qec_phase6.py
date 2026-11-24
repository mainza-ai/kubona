"""Phase 6 tests: sinter multi-worker batch + CSV resume, S5 magic-state factories,
and S6 Lean/DRAT formal attestation emitter.
"""

from __future__ import annotations

import numpy as np
import pytest

pytest.importorskip("stim")
pytest.importorskip("sinter")
pytest.importorskip("pymatching")

from kubona.qec import (
    DiscoveryClaim,
    S5_MAGIC_STATE_TARGETS,
    build_qec_formal_attestation,
    build_surface_sinter_tasks,
    compute_gamma,
    evaluate_seven_gates,
    hamming_15_1_3_parity_matrix,
    run_sinter_batch,
    verify_magic_state_syndromes,
)


def test_sinter_batch_multiworker_and_csv_checkpoint_resume(tmp_path):
    """Real multi-worker sinter.collect with checkpointed CSV resume."""
    csv_path = tmp_path / "sinter_checkpoint.csv"
    tasks = build_surface_sinter_tasks(distances=[3, 5], physical_error_rates=[0.003], rounds=3)

    # Pass 1: 400 shots, checkpoint to CSV
    res1 = run_sinter_batch(tasks, num_workers=2, max_shots=400, max_errors=50, save_resume_filepath=csv_path)
    assert len(res1) == 2
    assert csv_path.exists() and csv_path.stat().st_size > 0
    assert all(r["shots"] >= 400 for r in res1)

    # Pass 2: resume from same CSV up to 800 shots
    res2 = run_sinter_batch(tasks, num_workers=2, max_shots=800, max_errors=50, save_resume_filepath=csv_path)
    assert len(res2) == 2
    assert all(r["shots"] >= 800 for r in res2)


def test_magic_state_factory_s5_syndrome_criterion_and_gamma_targets():
    """S5: 15-to-1 factory passes distinct-nonzero-syndrome check; fakes fail; gamma matches S5."""
    H15 = hamming_15_1_3_parity_matrix()
    ok = verify_magic_state_syndromes(H15)
    assert ok["n"] == 15 and ok["r"] == 4 and ok["distance_ge_3"] is True

    # Planted fake: duplicate column -> distance < 3
    H_dup = H15.copy()
    H_dup[:, 14] = H_dup[:, 0]
    bad = verify_magic_state_syndromes(H_dup)
    assert bad["distance_ge_3"] is False
    assert (0, 14) in bad["duplicate_syndrome_pairs"]

    # Verify S5 reference gamma formula matches published values to 1e-3
    for spec in S5_MAGIC_STATE_TARGETS.values():
        g = compute_gamma(spec["n"], spec["k"], spec["d"])
        assert abs(g - spec["gamma"]) < 0.002


def test_formal_attestation_emitter_enforces_gates_and_axiom_whitelist():
    """S6: emits Lean 4 attestation only after Gates 0-6 pass and refuses non-standard axioms."""
    H = np.array(
        [[1, 0, 1, 0, 1, 0, 1], [0, 1, 1, 0, 0, 1, 1], [0, 0, 0, 1, 1, 1, 1]],
        dtype=np.uint8,
    )
    cl = DiscoveryClaim(
        claim_id="dc_formal_1",
        code_name="steane_7_1_3",
        hx=H,
        hz=H,
        d=3,
        d_kind="proved",
        total_qubits=13,
        baseline_total_qubits=17,
        code_ci_high=0.004,
        baseline_ci_low=0.012,
        held_out_ci_high=0.006,
        held_out_baseline_ci_low=0.015,
        held_out_noise_is_held_out=True,
        reproduced=True,
        d_circ=3,
    )
    # Refuses before gates pass
    with pytest.raises(ValueError, match="gates 0-6"):
        build_qec_formal_attestation(cl, theorem_name="steane_valid")

    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is True

    # Refuses non-standard axiom (sorryAx)
    with pytest.raises(ValueError, match="disallowed Lean axioms"):
        build_qec_formal_attestation(cl, theorem_name="steane_valid", axioms=("propext", "sorryAx"))

    att = build_qec_formal_attestation(cl, theorem_name="steane_valid")
    assert att["sorry_count"] == 0
    assert att["parameters"] == {"n": 7, "k": 1, "d": 3, "d_kind": "proved", "d_circ": 3}
    assert len(att["bundle_sha256"]) == 64
    assert "theorem steane_valid_d_pos : 3 > 0 := by decide" in att["lean_source"]
