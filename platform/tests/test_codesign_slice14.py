"""Slice 14: transfer bounds, exact binomial tail, routing kernel, sharing, toy BM.

Production bar: every new behavior proven by measurement, refusals pinned,
paper numbers reproduced in direction + order (no invented constants).
"""

import math

import pytest

from kubona.codesign import (
    Builder,
    FamilySearch,
    KickmixEvaluator,
    alias_schedule,
    berlekamp_massey_toy,
    binom_tail,
    build_routed_1bit_madd,
    emit_cswap,
    eps_upper_exact,
    fold_tildeP,
    locator_roots,
    madd_costs,
    representatives,
    routed_1bit_spec,
    routed_madd_costs,
    schedule_loops,
    single_error_syndromes,
    tildeP_update,
    transfer_bounds,
    transform_cost,
    unfold_tildeP,
)
from kubona.codesign.gf import F24


def test_transfer_bounds_paper_shapes():
    t = transfer_bounds(1e-4)
    assert t["state_dist"] == pytest.approx(0.02)
    assert t["tv_bound"] == pytest.approx(0.02)
    assert t["fidelity_floor"] == pytest.approx((1 - 2e-4) ** 2)
    assert transfer_bounds(0.0) == {"state_dist": 0.0, "fidelity_floor": 1.0, "tv_bound": 0.0}
    vac = transfer_bounds(0.75)
    assert vac["fidelity_floor"] == 0.0  # vacuous, never flattering
    assert vac["state_dist"] == pytest.approx(min(2.0, 2 * math.sqrt(0.75)))
    for bad in (-0.1, 1.5, float("nan"), True, "x"):
        with pytest.raises(ValueError):
            transfer_bounds(bad)


def test_binom_tail_exact_shapes():
    assert binom_tail(10, 10, 0.3) == 1.0
    assert binom_tail(10, 0, 0.0) == 1.0
    assert binom_tail(10, 3, 1.0) == 0.0
    assert binom_tail(10, 0, 0.1) == pytest.approx(0.9 ** 10)
    # monotone decreasing in eps
    assert binom_tail(50, 5, 0.05) > binom_tail(50, 5, 0.2)
    # paper DQI scale: raw tail at the paper's doubled eps is tiny (detection
    # doubling explained in the eps_upper test below)
    assert binom_tail(100_000, 100, 0.002434) < 1e-10
    for bad in [(0, 0, 0.1), (10, 11, 0.1), (10, -1, 0.1), (10, 0, 2.0)]:
        with pytest.raises(ValueError):
            binom_tail(*bad)


def test_eps_upper_exact_inverts_tail():
    assert eps_upper_exact(10, 10) == 1.0
    closed = 1.0 - (0.025 ** (1.0 / 10))
    assert eps_upper_exact(10, 0) == pytest.approx(closed, rel=1e-9)
    # inversion property: tail at the bound ~= delta
    for k, b in ((20, 2), (100, 5)):
        e = eps_upper_exact(k, b, 0.025)
        assert binom_tail(k, b, e) == pytest.approx(0.025, rel=1e-6)
        assert binom_tail(k, b, e * 0.5) > 0.025
    # DQI gate: raw bound ~1.216e-3; paper reports ~2.434e-3 because a wrong
    # sign is detected with probability >= 1/2 (factor 2, SI I). Pin both.
    raw = eps_upper_exact(100_000, 100, 0.025)
    assert 1.0e-3 < raw < 1.5e-3
    assert 2 * raw == pytest.approx(2.434e-3, rel=0.02)
    with pytest.raises(ValueError):
        eps_upper_exact(0, 0)


def test_routed_madd_costs_paper_formula():
    assert madd_costs(1) == {"n": 1, "naive_toffoli": 18, "routed_toffoli": 9, "saved": 9}
    assert routed_madd_costs(1) == madd_costs(1)
    c8 = madd_costs(8)
    assert c8["naive_toffoli"] == 3 * 64 + 17 * 8 - 2 == 326
    assert c8["routed_toffoli"] == 163
    for bad in (0, -3, True, "8"):
        with pytest.raises(ValueError):
            madd_costs(bad)


def test_cswap_and_routed_madd_proven():
    b = Builder(3)
    assert emit_cswap(b, 0, 1, 2) == 1
    assert b.toffoli_count() == 1
    rb, regs = build_routed_1bit_madd()
    assert regs["ccx"] == 3 and rb.toffoli_count() == 3
    res = KickmixEvaluator().evaluate(rb, routed_1bit_spec(), num_tests=64, seed=7)
    assert res["valid"] is True and res["tests"] == 16  # 16 inputs x 1 record (no MBU here)
    assert res["toffoli"] == 3 and res["peak_qubits"] == 5
    assert "transfer" in res and res["transfer"]["fidelity_floor"] > 0
    # without the final swap-back, y/z leak (inputs not preserved) -> refuse
    bad = Builder(5)
    emit_cswap(bad, 0, 2, 3)
    bad.ccx(1, 2, 4)
    r2 = KickmixEvaluator().evaluate(bad, routed_1bit_spec(), num_tests=32, seed=7)
    assert r2["valid"] is False


def test_share_elites_deterministic_and_loud():
    fs = FamilySearch(["eea", "bm", "pgz"])
    assert fs.share_elites() == {"offers": 0, "improvements": 0, "details": []}
    fs.offer("eea", [], 100, 50)
    fs.offer("bm", [], 80, 60)
    rep = fs.share_elites()
    assert rep["offers"] == 4  # 2 elites into 2 other families each
    assert rep["improvements"] == 2  # bm->eea, bm->pgz (min-TQ per cell)
    assert all(d["from"] != d["to"] for d in rep["details"])
    assert any((e["parent"] or "").startswith("x-poll:bm") for e in fs.lineage)
    rep2 = fs.share_elites()
    assert rep2["improvements"] == 0  # converged: sharing is idempotent


def test_toy_bm_rescaled_recovers_single_error():
    for pos in (0, 1, 3, 5, 9, 14):
        syn = single_error_syndromes(F24, 2, pos, 7, 6)
        rep = berlekamp_massey_toy(syn, F24)
        assert rep["invariant_holds"] is True
        assert locator_roots(rep["sigma"], F24, 2) == [pos]
    with pytest.raises(ValueError):
        single_error_syndromes(F24, 2, 3, 0, 6)  # zero error refuses
    with pytest.raises(ValueError):
        berlekamp_massey_toy([], F24)


def test_tildeP_fold_round_trip_and_update_gate():
    for P in range(3):
        for Q in range(3):
            for R in range(3):
                assert unfold_tildeP(fold_tildeP(P, Q, R), Q, R) == P
                for p in range(3):
                    for q in range(3):
                        for r in range(3):
                            t2, Q2, R2 = tildeP_update(fold_tildeP(P, Q, R), Q, R, p, q, r)
                            assert (Q2, R2) == ((Q + q) % 3, (R + r) % 3)
    with pytest.raises(ValueError):
        fold_tildeP(3, 0, 0)


def test_loop_schedule_reuses_shared_segments():
    reps = representatives((1, 2, -1, -2))
    assert len(reps) == 8 and (1, 2, -1, -2) in reps
    assert (-2, -1, 2, 1) in reps  # inverted representative shares the trace
    assert transform_cost((), (1, 2)) == (2, 0)
    assert transform_cost((1, 2, 3), (1, 2, 4)) == (2, 0)
    loops = [(1, 2, -1, -2), (1, 2, 5, -1, -2, -5), (3, 4, -3, -4)]
    rep = schedule_loops(loops)
    assert rep["mults"] < rep["naive_mults"]  # retention beats rebuild+reverse
    assert rep["saved"] == rep["naive_mults"] - rep["mults"]
    assert sorted(rep["order"]) == [0, 1, 2]
    with pytest.raises(ValueError):
        schedule_loops([(1,)] * 9)  # DP cap refuses, no heuristic as optimal


def test_alias_schedule_prefers_runs_without_hurting_cost():
    rows = ["A", "B", "A", "B", "A", "B", "A", "B"]
    rep = alias_schedule(rows)
    assert rep["ordered"] == ["A"] * 4 + ["B"] * 4  # runs preferred
    assert rep["cost"] <= rep["naive_cost"]
    assert rep["adjacencies"] == 6
    with pytest.raises(ValueError):
        alias_schedule(["A"] * 7)  # non-2^k refuses
