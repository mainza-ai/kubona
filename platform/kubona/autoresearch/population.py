"""Population-coordinated replica search over QUBO instances.

One optimizer alone stalls in local optima; a population that shares
discoveries under the same total sample budget often does not. This module
makes that comparison controlled and ground-truth anchored:

  - instances: random QUBOs (seeded) at n <= 14, optimum known by brute
    force — every gap reported is against the true optimum, never a guess;
  - replicas: seeded greedy descents with per-step sample accounting;
  - policies: `independent` (no sharing, the baseline) vs `best_share`
    (every C steps, replicas restart from the perturbed global best);
  - budgets: total hill-climb steps identical across policies; repeated
    over seeds; per-replica RNG streams so sharing is the only difference.

Report vocabulary is fixed: best gap to optimum, replicas holding an
optimal solution, steps used. Coordination improving solution quality is
not a runtime claim — the report never phrases it as one.
"""

from __future__ import annotations

import itertools
from typing import Any

import numpy as np


def random_qubo(n: int, seed: int, density: float = 0.5,
                scale: float = 2.0) -> np.ndarray:
    """Seeded random symmetric QUBO matrix (upper-triangle sampled)."""
    rng = np.random.default_rng(seed)
    Q = np.zeros((n, n))
    for i in range(n):
        for j in range(i, n):
            if rng.random() < density or i == j:
                Q[i, j] = rng.uniform(-scale, scale)
    return (Q + Q.T) / 2.0


def qubo_energy(Q: np.ndarray, x: np.ndarray) -> float:
    return float(x @ Q @ x)


def brute_optimum(Q: np.ndarray) -> tuple[float, list[int]]:
    """Exact optimum by enumeration (n <= 20 guard: exponential by design)."""
    n = Q.shape[0]
    if n > 20:
        raise ValueError(f"brute force refused for n={n} > 20")
    best_e, best_x = float("inf"), None
    for bits in range(1 << n):
        x = np.array([(bits >> i) & 1 for i in range(n)], dtype=float)
        e = qubo_energy(Q, x)
        if e < best_e:
            best_e, best_x = e, [int(v) for v in x]
    return best_e, best_x


def _greedy_descent(Q: np.ndarray, start: np.ndarray, steps: int,
                    rng: np.random.Generator) -> tuple[np.ndarray, float, int]:
    """Greedy single-bit flips; returns (best_x, best_e, samples_used).

    Each candidate evaluation is one sample — the budget unit. Random
    neighbor choice keeps replicas diverse under their own streams.
    """
    n = Q.shape[0]
    x = start.copy()
    best_e = qubo_energy(Q, x)
    used = 0
    for _ in range(steps):
        i = int(rng.integers(0, n))
        cand = x.copy()
        cand[i] = 1 - cand[i]
        used += 1
        e = qubo_energy(Q, cand)
        if e < best_e:
            x, best_e = cand, e
    return x, best_e, used


def run_population(Q: np.ndarray, n_replicas: int, steps_per_replica: int,
                   policy: str = "best_share", sync_every: int = 10,
                   perturb_bits: int = 2, seed: int = 7) -> dict[str, Any]:
    """Run one population; return the anchored report.

    best_share: every `sync_every` steps each replica restarts from the
    global best with `perturb_bits` random flips (diversity preserved by
    perturbation, not by isolation). independent: no communication.
    """
    if policy not in ("independent", "best_share"):
        raise ValueError(f"unknown policy {policy!r}")
    n = Q.shape[0]
    master = np.random.default_rng(seed)
    streams = [np.random.default_rng(int(master.integers(0, 2**31)))
               for _ in range(n_replicas)]
    states = [streams[k].integers(0, 2, size=n).astype(float)
              for k in range(n_replicas)]
    energies = [qubo_energy(Q, x) for x in states]
    total_used = 0
    remaining = steps_per_replica
    while remaining > 0:
        chunk = min(sync_every, remaining)
        for k in range(n_replicas):
            states[k], energies[k], used = _greedy_descent(
                Q, states[k], chunk, streams[k])
            total_used += used
        remaining -= chunk
        if policy == "best_share" and remaining > 0:
            best = int(np.argmin(energies))
            for k in range(n_replicas):
                if k == best:
                    continue
                cand = states[best].copy()
                flips = streams[k].choice(n, size=min(perturb_bits, n),
                                          replace=False)
                cand[flips] = 1 - cand[flips]
                states[k] = cand
                energies[k] = qubo_energy(Q, cand)
    optimum, _ = brute_optimum(Q)
    best_e = min(energies)
    return {
        "policy": policy,
        "n_replicas": n_replicas,
        "steps_per_replica": steps_per_replica,
        "total_samples": total_used,
        "best_energy": best_e,
        "optimum": optimum,
        "gap_to_optimum": best_e - optimum,
        "replicas_optimal": sum(1 for e in energies if e == optimum),
    }


def compare_policies(Q: np.ndarray, n_replicas: int = 8,
                     steps_per_replica: int = 200,
                     seeds: list[int] | None = None, **kwargs) -> dict[str, Any]:
    """Independent vs coordinated under identical budgets, over seeds.

    The only difference between arms is communication: same instances,
    same per-replica step counts, same seed set, disjoint RNG streams
    within each run. Mean gaps reported with per-seed detail (no hiding
    behind averages: correlated outcomes stay visible).
    """
    seeds = seeds if seeds is not None else [7, 11, 13]
    optimum, _ = brute_optimum(Q)
    arms: dict[str, list[dict[str, Any]]] = {"independent": [], "best_share": []}
    for seed in seeds:
        for policy in arms:
            arms[policy].append(run_population(
                Q, n_replicas, steps_per_replica, policy=policy,
                seed=seed, **kwargs))
    summary = {}
    for policy, runs in arms.items():
        gaps = [r["gap_to_optimum"] for r in runs]
        summary[policy] = {
            "mean_gap": float(np.mean(gaps)),
            "per_seed_gap": gaps,
            "total_optimal_replicas": sum(r["replicas_optimal"] for r in runs),
            "total_samples_each": runs[0]["total_samples"],
        }
    for policy, runs in arms.items():
        assert all(r["total_samples"] == summary[policy]["total_samples_each"]
                   for r in runs), "budget accounting must be exact"
    assert (summary["independent"]["total_samples_each"]
            == summary["best_share"]["total_samples_each"]), \
        "arms must run equal budgets or the comparison is void"
    return {"optimum": optimum, "seeds": seeds, "arms": summary}
