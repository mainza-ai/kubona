"""TQ-cost scoring (single shared const — never two copies).

Paper: TQ = Toffoli-count x peak-qubits (T counts Toffoli+CCZ equally).
The paper's variant is real-valued Q^kappa * T with kappa > 1 (1-3 cited
from factoring estimates); this module discretizes to integer kappa in
{1, 2, 3} so scores stay ledger-exact integers (no float receipts).
Toffoli-depth and active-volume variants are LITERATURE-derived diagnostics
(factories.py: GF2019 CCZ factory, Litinski 2019/2023 — labeled, never
paper-measured), served via KickmixEvaluator.evaluate(factory=...) as the
"factory" report key. Kappa bounds + scoring live here
so cost/estimator + harness + console cannot rot apart.

POLICY (settled): ranking stays TQ. Depth/active_qubits/factory are measured
diagnostics (kickmix.circuit_depth etc. + literature factory model, served
through evaluate/frontier) — never ranking inputs. A5 closed slice 62 by
wiring literature factories instead of inventing weights; consistency alone
is never the argument for gating or re-ranking.
"""

from __future__ import annotations

#: Default kappa for paper-comparability (TQ = T * Q).
TQ_KAPPA_DEFAULT = 1
#: Admissible kappa range per paper (factoring estimates use 1..3).
TQ_KAPPA_MIN = 1
TQ_KAPPA_MAX = 3


def _require_nonneg_int(name: str, v: object) -> int:
    if isinstance(v, bool) or not isinstance(v, int) or v < 0:
        raise ValueError(f"{name} must be a non-negative int, got {v!r}")
    return v


def _require_kappa(kappa: object) -> int:
    if isinstance(kappa, bool) or not isinstance(kappa, int):
        raise ValueError(f"kappa must be an int in [{TQ_KAPPA_MIN},{TQ_KAPPA_MAX}], got {kappa!r}")
    if not (TQ_KAPPA_MIN <= kappa <= TQ_KAPPA_MAX):
        raise ValueError(f"kappa must be in [{TQ_KAPPA_MIN},{TQ_KAPPA_MAX}], got {kappa!r}")
    return kappa


def tq_cost(toffoli: int, qubits: int, kappa: int = TQ_KAPPA_DEFAULT) -> int:
    """TQ cost = T * Q^kappa. Fail-closed on bad inputs.

    T counts Toffoli + CCZ equally (paper Methods). Q is peak allocated.
    kappa=1 reproduces the paper's uniform comparison.
    """
    t = _require_nonneg_int("toffoli", toffoli)
    q = _require_nonneg_int("qubits", qubits)
    k = _require_kappa(kappa)
    return t * (q ** k)


def kappa_sweep(toffoli: int, qubits: int) -> dict[int, int]:
    """TQ under kappa=1..3 (paper's factoring range). One formula, three views.

    Lets the console kappa slider compare without a second implementation:
    Go serves each kappa via Cost(); this sweep pins the same triple here.
    """
    return {k: tq_cost(toffoli, qubits, k) for k in (1, 2, 3)}
