"""DQI acceptance math (SI Eq 25 + gate): sampling + loss bound + Prange compare.

Does NOT claim the paper's exact |ae|^2 distribution (unpublished in full):
the sampler is a documented toy (uniform weight 0..ell, uniform positions)
used to exercise the acceptance machinery — distribution shape is stated,
never smuggled in as the paper's. The bound math (Eq 25, gate conversion,
Prange advantage fractions) is exact and ledger-ready.
"""

from __future__ import annotations

import math
import random


def sample_sparse_error(n_positions: int, ell: int, rng: random.Random) -> set[int]:
    """Toy sparse error: uniform weight w in [0,ell], uniform positions.

    Labeled toy — drives the acceptance harness, not a claim about the
    paper's |ae|^2. Fail-closed on bad geometry.
    """
    if not isinstance(n_positions, int) or n_positions < 1:
        raise ValueError(f"n_positions must be positive int, got {n_positions!r}: refusing")
    if not isinstance(ell, int) or ell < 0 or ell > n_positions:
        raise ValueError(f"ell must be in [0,n_positions], got {ell!r}: refusing")
    w = rng.randint(0, ell)
    return set(rng.sample(range(n_positions), w))


def eps_from_gate(num_tests: int, allowed_failures: int = 100,
                  confidence: float = 0.975) -> float:
    """Gate conversion: <=allowed_failures/num_tests tests -> eps upper bound.

    Default mirrors the paper's DQI gate scale (100/100k -> ~2.4e-3 @97.5%
    exact Clopper-Pearson; ours is a conservative Hoeffding tail).
    Closed form for 0 failures (1-delta^{1/k}), Hoeffding tail otherwise.
    Refuses non-positive tests or out-of-range failures.
    """
    if not isinstance(num_tests, int) or num_tests <= 0:
        raise ValueError("num_tests must be positive: refusing")
    if not (0 <= allowed_failures <= num_tests):
        raise ValueError("allowed_failures must be in [0,num_tests]: refusing")
    if not (0.0 < confidence < 1.0):
        raise ValueError("confidence must be in (0,1): refusing")
    delta = 1.0 - confidence
    # scale the paper's 100k reference: effective k = num_tests (tests actually run)
    k = num_tests
    b = allowed_failures
    if b == 0:
        return 1.0 - (delta ** (1.0 / k))
    phat = b / k
    return min(1.0, phat + math.sqrt(math.log(1.0 / (delta / 2.0)) / (2.0 * k)))


def dqi_loss_bound(eps: float, f: float) -> float:
    """SI Eq 25: |<f>impl - <f>| <= 4*eps*<f>. Fail-closed on bad inputs."""
    for name, v in (("eps", eps), ("f", f)):
        if isinstance(v, bool) or not isinstance(v, (int, float)) \
                or not math.isfinite(float(v)) or float(v) < 0:
            raise ValueError(f"{name} must be finite >= 0, got {v!r}: refusing")
    if float(eps) > 1.0:
        raise ValueError(f"eps must be <= 1, got {eps!r}: refusing")
    return 4.0 * float(eps) * float(f)


def prange_advantage_report(f_dqi: float, f_prange: float, eps: float) -> dict:
    """Advantage fractions as in the paper (7.4%/4.9% style). No advantage claimed.

    Returns {advantage, max_loss, loss_share_of_advantage}. Refuses when
    f_dqi <= f_prange (no advantage to erode) — reporting a share of a
    non-advantage would be theater.
    """
    for name, v in (("f_dqi", f_dqi), ("f_prange", f_prange)):
        if isinstance(v, bool) or not isinstance(v, (int, float)) \
                or not math.isfinite(float(v)) or not (0.0 <= float(v) <= 1.0):
            raise ValueError(f"{name} must be finite in [0,1], got {v!r}: refusing")
    if f_dqi <= f_prange:
        raise ValueError(f"no DQI advantage (f_dqi={f_dqi} <= f_prange={f_prange}): refusing share math")
    loss = dqi_loss_bound(eps, f_dqi)
    adv = f_dqi - f_prange
    return {"advantage": adv, "max_loss": loss,
            "loss_share_of_advantage": min(1.0, loss / adv) if adv > 0 else 1.0}
