"""Population-coordinated search: mechanics, budgets, anchors.

Deliberately absent: any assertion that coordination beats independence.
Superiority is instance-dependent; locking it as a test would be fitting
the suite to one landscape. What is locked: determinism under seed, exact
sample accounting, equal budgets between arms, gaps never below zero
(the optimum is the optimum), and fail-closed guards.
"""

import pytest

from kubona.autoresearch.population import (
    brute_optimum,
    compare_policies,
    qubo_energy,
    random_qubo,
    run_population,
)


def test_deterministic_under_seed():
    Q = random_qubo(10, seed=3)
    a = run_population(Q, 6, 100, policy="best_share", seed=7)
    b = run_population(Q, 6, 100, policy="best_share", seed=7)
    assert a == b


def test_sample_accounting_exact():
    Q = random_qubo(10, seed=3)
    out = run_population(Q, 6, 100, policy="best_share", seed=7)
    assert out["total_samples"] == 6 * 100
    assert set(out) == {"policy", "n_replicas", "steps_per_replica",
                        "total_samples", "best_energy", "optimum",
                        "gap_to_optimum", "replicas_optimal"}


def test_gaps_never_below_zero_across_landscapes():
    for seed in (1, 2, 3, 4):
        Q = random_qubo(12, seed=seed)
        out = compare_policies(Q, n_replicas=6, steps_per_replica=120)
        for policy, summary in out["arms"].items():
            assert all(g >= 0 for g in summary["per_seed_gap"]), (seed, policy)
            assert summary["total_samples_each"] == 6 * 120


def test_brute_guard_and_policy_guard():
    import numpy as np
    with pytest.raises(ValueError, match="brute force refused"):
        brute_optimum(np.zeros((21, 21)))
    Q = random_qubo(8, seed=1)
    with pytest.raises(ValueError, match="unknown policy"):
        run_population(Q, 4, 50, policy="telepathy")


def test_qubo_energy_and_optimum_trivial():
    import numpy as np
    Q = np.array([[-1.0, 2.0], [2.0, -1.0]])
    e, x = brute_optimum(Q)
    assert e == -1.0 and sorted(x) == [0, 1]
    assert qubo_energy(Q, np.array([1.0, 1.0])) == 2.0
