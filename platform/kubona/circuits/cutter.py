"""LOCC Circuit Cutting & Partitioning Optimizer (Ring 3).

Partitions circuits that exceed target physical QPU width/connectivity.
Evaluates the classical quasi-probability shot overhead multiplier (4^K for K cut gates)
and enforces cost-governance budget feasibility before approving circuit cutting.

``cost_feasible_advisory`` is an ADVISORY estimate only — it never reads as an
approval to cut. Execution-time approval lives with the cost-governance gate.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Any, Dict, List, Tuple

from kubona.circuits.model import Circuit

#: Hard cap on the quasi-probability multiplier evaluation: 4^K is computed in
#: integers and refused above the configured cap BEFORE multiplying into shot
#: counts, so an absurd K can never overflow into a "feasible" plan.
ABSOLUTE_MULTIPLIER_CAP = 4 ** 12


@dataclass(frozen=True)
class PricingConfig:
    """Explicit pricing inputs for cut planning — no hardcoded dollars/shots.

    Every field is required: the old code hid 256 / 4096 / 50.0 / 0.001 as
    defaults, so plans carried a pricing model nobody chose.
    """
    max_shot_multiplier: float
    base_shots: int
    budget_usd_cap: float
    shot_rate_per_k: float  # USD per 1000 shots

    def __post_init__(self) -> None:
        if not math.isfinite(self.max_shot_multiplier) or self.max_shot_multiplier < 1:
            raise ValueError("max_shot_multiplier must be finite >= 1")
        if isinstance(self.base_shots, bool) or not isinstance(self.base_shots, int) \
                or self.base_shots < 1:
            raise ValueError("base_shots must be a positive int")
        if not math.isfinite(self.budget_usd_cap) or self.budget_usd_cap < 0:
            raise ValueError("budget_usd_cap must be finite >= 0")
        if not math.isfinite(self.shot_rate_per_k) or self.shot_rate_per_k < 0:
            raise ValueError("shot_rate_per_k must be finite >= 0")


@dataclass
class CutPartitionPlan:
    original_qubits: int
    subcircuit_qubits: List[int]
    cut_locations: List[int]
    k_cuts: int
    classical_shot_multiplier: float  # 4^k
    estimated_total_shots: int
    cost_feasible_advisory: bool  # ADVISORY ONLY — never an approval to cut
    overhead_warning: str | None = None

    @property
    def cost_feasible(self) -> bool:
        """Deprecated alias for cost_feasible_advisory (kept for readers;
        advisory only, never an approval)."""
        return self.cost_feasible_advisory


class CircuitCuttingOptimizer:
    """Plans wire/gate-cut partitions with explicit pricing and circuit-aware K."""

    def __init__(self, pricing: PricingConfig):
        if not isinstance(pricing, PricingConfig):
            raise ValueError(
                "pricing must be an explicit PricingConfig "
                "(no implicit dollars/shots defaults)"
            )
        self.pricing = pricing

    def plan_cuts(
        self,
        n_qubits: int,
        target_qpu_width: int,
        circuit: Circuit | None = None,
    ) -> CutPartitionPlan:
        """Determines optimal cut partition across target sub-processors.

        Args:
            n_qubits: width of the circuit to partition.
            target_qpu_width: max qubits per subcircuit (must be > 0).
            circuit: when given, K counts the ACTUAL inter-partition 2-qubit
                gates (gates whose qubits land on different sides of a
                partition boundary). Without a circuit, K falls back to the
                partition-count bound (num_partitions - 1) and the plan is
                labeled as such — the bound over-counts when partitions are
                sparsely entangled and under-counts nothing, but it is a
                bound, not a measurement.
        """
        if isinstance(n_qubits, bool) or not isinstance(n_qubits, int) or n_qubits < 1:
            raise ValueError(f"n_qubits must be a positive int, got {n_qubits!r}")
        if isinstance(target_qpu_width, bool) or not isinstance(target_qpu_width, int) \
                or target_qpu_width < 1:
            raise ValueError(
                f"target_qpu_width must be a positive int, got {target_qpu_width!r}"
            )
        if circuit is not None and circuit.num_qubits != n_qubits:
            raise ValueError(
                f"circuit width {circuit.num_qubits} != n_qubits {n_qubits}"
            )
        if n_qubits <= target_qpu_width:
            return CutPartitionPlan(
                original_qubits=n_qubits,
                subcircuit_qubits=[n_qubits],
                cut_locations=[],
                k_cuts=0,
                classical_shot_multiplier=1.0,
                estimated_total_shots=self.pricing.base_shots,
                cost_feasible_advisory=True,
                overhead_warning=None,
            )

        # Partition assignment: contiguous blocks of <= target_qpu_width.
        num_partitions = math.ceil(n_qubits / target_qpu_width)
        boundaries = [p * target_qpu_width for p in range(1, num_partitions)]

        def owner(q: int) -> int:
            for i, b in enumerate(boundaries):
                if q < b:
                    return i
            return len(boundaries)

        sub_sizes = [target_qpu_width] * (num_partitions - 1)
        sub_sizes.append(n_qubits - sum(sub_sizes))

        if circuit is not None:
            # Circuit-aware K: count 2-qubit gates spanning partitions.
            spanning = [
                idx for idx, g in enumerate(circuit.gates)
                if len(g.qubits) == 2 and owner(g.qubits[0]) != owner(g.qubits[1])
            ]
            k_cuts = len(spanning)
            cut_locations = spanning
            method_note = (
                f"circuit-aware K={k_cuts} (inter-partition 2q gates at {spanning})"
            )
        else:
            k_cuts = num_partitions - 1
            cut_locations = list(range(k_cuts))
            method_note = (
                f"partition-count bound K={k_cuts} (no circuit supplied; "
                "bound, not a measurement)"
            )

        # Cap 4^K BEFORE multiplying into shots: exact integer power, refused
        # above the configured cap (and never above the absolute cap).
        multiplier_exact = 4 ** k_cuts
        if multiplier_exact > ABSOLUTE_MULTIPLIER_CAP:
            raise ValueError(
                f"K={k_cuts} gives 4^K={multiplier_exact} above the absolute "
                f"cap {ABSOLUTE_MULTIPLIER_CAP}: refusing instead of planning "
                "an infeasible cut"
            )
        if multiplier_exact > self.pricing.max_shot_multiplier:
            raise ValueError(
                f"K={k_cuts} gives 4^K={multiplier_exact} above the configured "
                f"max_shot_multiplier {self.pricing.max_shot_multiplier}: "
                "refusing (raise the cap explicitly or narrow the circuit)"
            )
        multiplier = float(multiplier_exact)
        total_shots = int(self.pricing.base_shots * multiplier_exact)
        estimated_cost = (total_shots / 1000.0) * self.pricing.shot_rate_per_k
        feasible = estimated_cost <= self.pricing.budget_usd_cap

        warning = (
            f"ADVISORY ONLY ({method_note}): quasi-probability multiplier "
            f"{multiplier:.1f}x, estimated ${estimated_cost:.2f} vs budget "
            f"${self.pricing.budget_usd_cap:.2f} — "
            f"{'within budget' if feasible else 'OVER BUDGET (not an approval to cut)'}. "
            "Execution-time approval lives with the cost-governance gate."
        )

        return CutPartitionPlan(
            original_qubits=n_qubits,
            subcircuit_qubits=sub_sizes,
            cut_locations=cut_locations,
            k_cuts=k_cuts,
            classical_shot_multiplier=multiplier,
            estimated_total_shots=total_shots,
            cost_feasible_advisory=feasible,
            overhead_warning=warning,
        )
