"""Slice 17: full-width freeze, schedule-width comparator, scalable scheduler.

Production bar: paper-scale primitives validated the paper's way — exact
edge envelopes where misses hide, sampled trajectories with reported (never
hidden) eps bounds elsewhere, safety properties proven at every scale.
"""

import itertools
import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_index_match_param,
    build_zero_flag_param,
    edge_envelope_index_match,
    edge_envelope_zero_flag,
    index_match_spec_param,
    schedule_alias_words,
    zero_flag_spec_param,
)


def test_zero_flag_256_edge_and_sampled():
    (bld, regs), n = build_zero_flag_param(256), 256
    assert regs["ccx"] == 2 * n - 3 == 509
    assert regs["toffoli"] == 509 and regs["width"] == 256
    assert bld.leaked_allocs() == set()  # temps freed, no silent leak
    env = edge_envelope_zero_flag(bld, regs, n)
    assert env == {"checked": 2 * n + 5, "passed": 2 * n + 5, "width": n}
    res = KickmixEvaluator().evaluate(bld, zero_flag_spec_param(n),
                                       num_tests=256, seed=7)
    assert res["valid"] is True and res["failures"] == 0
    assert res["eps_bound_97_5"] < 0.02  # bound reported, tightness unclaimed
    with pytest.raises(ValueError):
        build_zero_flag_param(257)  # paper-width cap refuses
    with pytest.raises(ValueError):
        build_zero_flag_param(1)


def test_index_match_10bit_schedule_window():
    # the paper tests termination from step 499 of a 533-step schedule:
    # kbits=10 covers [0,1024) which contains the [499,533] window.
    (bld, regs), k, c = build_index_match_param(10, 499), 10, 499
    assert regs["ccx"] == 2 * k - 3 == 17
    env = edge_envelope_index_match(bld, regs, k, c)
    # 14 distinct: const, +-1, extremes, 10 flips, minus 499^1 = 498 (covered)
    assert env["passed"] == env["checked"] == 14
    res = KickmixEvaluator().evaluate(bld, index_match_spec_param(k, c),
                                       num_tests=256, seed=7)
    assert res["valid"] is True and res["failures"] == 0
    # the same circuit against a neighboring const is detected — but only by
    # targeted trajectories: 16 random samples over 10 bits almost surely miss
    # the two distinguishing inputs (sampling blindness the edge envelope
    # exists to cover). check_one on idx=499 and idx=500 both refuse.
    _ev, _rng = KickmixEvaluator(), random.Random(5)
    _spec500 = index_match_spec_param(k, c + 1)
    for v in (c, c + 1):
        init = {q: 0 for q in range(2 * k - 1)}
        for i in range(k):
            init[i] = (v >> i) & 1
        assert _ev.check_one(bld, _spec500, init, _rng).valid is False
    with pytest.raises(ValueError):
        build_index_match_param(10, 1024)  # const overflows width
    with pytest.raises(ValueError):
        build_index_match_param(17, 499)  # schedule-width cap refuses


def test_scheduler_exact_small_and_safe_large():
    # exact path agrees with the proven optimum on toy input
    r = schedule_alias_words(["A", "B", "A", "B", "A", "B", "A", "B"])
    assert r["method"] == "exact" and r["ordered"] == ["A"] * 4 + ["B"] * 4
    # 1024-row synthetic: permutation + never-worse, measured exactly (2^10)
    import random
    from collections import Counter
    rng = random.Random(3)
    words = [f"w{i % 8}" for i in range(1024)]
    rng.shuffle(words)
    r = schedule_alias_words(words)
    assert r["method"] == "greedy"
    assert Counter(r["ordered"]) == Counter(words)  # permutation, exactly
    assert r["cost"] <= r["naive_cost"]  # 15 <= 567 on this seed
    assert r["adjacencies"] >= r["naive_adjacencies"]
    assert max(len(list(g)) for _, g in itertools.groupby(r["ordered"])) == 128
    # non-power-of-two: runs improve, cost honestly unmeasured (None)
    r2 = schedule_alias_words(["a", "b", "c"] * 33 + ["d"])
    assert r2["cost"] is None and r2["method"] == "greedy"
    assert r2["adjacencies"] >= r2["naive_adjacencies"]
    assert Counter(r2["ordered"]) == Counter(["a", "b", "c"] * 33 + ["d"])
    with pytest.raises(ValueError):
        schedule_alias_words([])
    with pytest.raises(ValueError):
        schedule_alias_words([["unhashable"]])
