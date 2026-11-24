"""Phase 3 QEC search engine tests: Islands, MAP-Elites, Verifier-in-Loop & Controller."""

from __future__ import annotations

import pytest

pytest.importorskip("ldpc")
pytest.importorskip("highspy")

from kubona.qec import (
    DiscoveryController,
    IslandSearchPool,
    MapElitesQecArchive,
    evaluate_bb_candidate_fast,
    verifier_in_loop_refine,
)


def test_verifier_in_loop_feeds_failure_and_refines_candidate():
    """S4: proposer receives verifier error report on attempt 1 and fixes it on attempt 2."""
    seen_feedback: list[list[str]] = []

    def stub_proposer(feedback: list[str]):
        seen_feedback.append(list(feedback))
        if not feedback:
            # Attempt 1: degenerate 1-term polynomial -> must be rejected
            return ((0, 0),), ((0, 0),)
        # Attempt 2: after reading failure reason, propose valid BB(6,6)
        return ((3, 0), (0, 1), (0, 2)), ((0, 3), (1, 0), (2, 0))

    cand, fb = verifier_in_loop_refine(stub_proposer, 6, 6, max_retries=2, bposd_trials=12, seed=7)
    assert cand.valid is True
    assert cand.n == 72 and cand.k == 12
    assert len(fb) == 1
    assert "degenerate polynomial" in fb[0]
    assert len(seen_feedback) == 2 and seen_feedback[1] == fb


def test_islands_map_elites_migration_scratchpads_and_controller():
    """Islands + MAP-Elites + scratchpads + B8 DiscoveryController promotion."""
    pool = IslandSearchPool(6, 6, num_islands=4, migration_interval=2, use_scratchpads=True, seed=11)
    # Seed island 0 with a perturbed neighbor of BB(6,6) (catalog hidden)
    pool.seed_island(0, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    for gen in range(1, 4):
        pool.step(gen, bposd_trials=8)

    assert pool.evaluations >= 13
    best = pool.best()
    assert best is not None and best.valid is True
    assert best.fom >= 6.0
    # Migration after gen 2 copies best from island 0 -> island 1
    assert any("migrated" in note for note in pool.islands[1].scratchpad)
    # Pareto frontier is non-empty and sorted by FOM
    front = pool.archive.pareto_frontier()
    assert len(front) >= 1 and front[0].fom == best.fom

    # B8 DiscoveryController promotes high-FOM upper_bound candidate to exact MILP
    ctrl = DiscoveryController()
    act = ctrl.next_action(pool, target_fom=6.0, max_evals=100)
    assert act["action"] == "promote_milp"

    # After proving with MILP, controller promotes to circuit-level LER simulation
    proved = evaluate_bb_candidate_fast(
        6,
        6,
        best.a_terms,
        best.b_terms,
        bposd_trials=8,
        prove_if_fom_ge=5.0,
        milp_time_limit_s=20.0,
    )
    assert proved.d_kind == "proved" and proved.d == 6
    pool.archive.cells[pool.archive.niche_of(proved)] = proved
    act2 = ctrl.next_action(pool, target_fom=6.0, max_evals=100)
    assert act2["action"] == "promote_circuit_sim"


def test_map_elites_archive_keeps_best_per_niche():
    archive = MapElitesQecArchive(qubit_step=36, rate_step=0.05)
    c1 = evaluate_bb_candidate_fast(6, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)], bposd_trials=12)
    assert archive.insert(c1) is True
    assert len(archive.cells) == 1
    # Inserting a strictly worse candidate in the same niche does not overwrite
    worse = evaluate_bb_candidate_fast(6, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)], bposd_trials=12)
    worse.fom = c1.fom - 1.0
    assert archive.insert(worse) is False
    assert archive.top_k(1)[0].fom == c1.fom
