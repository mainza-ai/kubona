"""LOCC Circuit Cutting & Partitioning Optimizer (Ring 3).

Partitions circuits that exceed target physical QPU width/connectivity.
Evaluates the classical quasi-probability shot overhead multiplier (4^K for K cut gates)
and enforces cost-governance budget feasibility before approving circuit cutting.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any, Dict, List, Tuple


@dataclass
class CutPartitionPlan:
    original_qubits: int
    subcircuit_qubits: List[int]
    cut_locations: List[int]
    k_cuts: int
    classical_shot_multiplier: float  # 4^k
    estimated_total_shots: int
    cost_feasible: bool
    overhead_warning: str | None = None


class CircuitCuttingOptimizer:
    def __init__(self, max_shot_multiplier: float = 256.0):
        self.max_shot_multiplier = max_shot_multiplier

    def plan_cuts(
        self,
        n_qubits: int,
        target_qpu_width: int,
        base_shots: int = 4096,
        budget_usd_cap: float = 50.0,
        shot_rate_per_k: float = 0.001,
    ) -> CutPartitionPlan:
        """Determines optimal cut partition across target sub-processors."""
        if n_qubits <= target_qpu_width:
            return CutPartitionPlan(
                original_qubits=n_qubits,
                subcircuit_qubits=[n_qubits],
                cut_locations=[],
                k_cuts=0,
                classical_shot_multiplier=1.0,
                estimated_total_shots=base_shots,
                cost_feasible=True,
                overhead_warning=None,
            )

        # Minimum wire/gate cuts required to divide into partitions <= target_qpu_width
        num_partitions = math.ceil(n_qubits / target_qpu_width)
        k_cuts = num_partitions - 1
        sub_sizes = [target_qpu_width] * (num_partitions - 1)
        sub_sizes.append(n_qubits - sum(sub_sizes))

        multiplier = 4.0 ** k_cuts
        total_shots = int(base_shots * multiplier)
        estimated_cost = (total_shots / 1000.0) * shot_rate_per_k
        feasible = estimated_cost <= budget_usd_cap and multiplier <= self.max_shot_multiplier

        warning = None
        if not feasible:
            warning = f"Quasi-probability multiplier {multiplier:.1f}x exceeds feasibility threshold or budget (${estimated_cost:.2f} > ${budget_usd_cap:.2f})."

        return CutPartitionPlan(
            original_qubits=n_qubits,
            subcircuit_qubits=sub_sizes,
            cut_locations=list(range(k_cuts)),
            k_cuts=k_cuts,
            classical_shot_multiplier=multiplier,
            estimated_total_shots=total_shots,
            cost_feasible=feasible,
            overhead_warning=warning,
        )
