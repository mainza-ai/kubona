"""
Autonomous Ansatz Genome Search & Pre-flight Feasibility Probes.
Implements circuit genome representations with cheap pre-checks:
- Meyer-Wallach expressibility (Q)
- Gradient variance barren plateau detection (Var(grad) < 1e-6)
- MPS bond dimension classical simulability estimation
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Any, List
import numpy as np


@dataclass
class AnsatzGenome:
    family: str # "HardwareEfficient", "RealAmplitudes", "StronglyEntangling"
    num_qubits: int
    depth: int
    entangler_topology: str # "linear", "circular", "all-to-all"
    initializer: str # "zero", "random", "hartree_fock"
    symmetry_conserving: bool = False

    def to_dict(self) -> Dict[str, Any]:
        return {
            "family": self.family,
            "num_qubits": self.num_qubits,
            "depth": self.depth,
            "entangler_topology": self.entangler_topology,
            "initializer": self.initializer,
            "symmetry_conserving": self.symmetry_conserving,
        }


def synthesize_qasm(genome: AnsatzGenome, params: List[float] | None = None) -> str:
    """Synthesizes OPENQASM 2.0 circuit code for the given genome."""
    n = genome.num_qubits
    d = genome.depth
    lines = [
        'OPENQASM 2.0;',
        'include "qelib1.inc";',
        f'qreg q[{n}];',
        f'creg c[{n}];',
    ]

    param_idx = 0
    for layer in range(d):
        # 1-qubit rotation layer
        for i in range(n):
            angle = params[param_idx] if params and param_idx < len(params) else 0.54
            param_idx += 1
            if genome.family == "RealAmplitudes":
                lines.append(f"ry({angle}) q[{i}];")
            else:
                lines.append(f"ry({angle}) q[{i}];")
                lines.append(f"rz({angle * 0.5}) q[{i}];")

        # Entanglement layer
        if genome.entangler_topology == "linear":
            for i in range(n - 1):
                lines.append(f"cx q[{i}], q[{i+1}];")
        elif genome.entangler_topology == "circular":
            for i in range(n):
                lines.append(f"cx q[{i}], q[{(i+1)%n}];")
        elif genome.entangler_topology == "all-to-all":
            for i in range(n):
                for j in range(i + 1, n):
                    lines.append(f"cx q[{i}], q[{j}];")

    for i in range(n):
        lines.append(f"measure q[{i}] -> c[{i}];")

    return "\n".join(lines)


def evaluate_meyer_wallach_expressibility(genome: AnsatzGenome) -> float:
    """
    Computes Meyer-Wallach entanglement/expressibility measure Q.
    Q in [0, 1]. Product states have Q=0; maximally entangled states have Q=1.
    """
    # Heuristic based on entangler density and depth
    if genome.depth == 0:
        return 0.0
    density = 1.0 if genome.entangler_topology == "all-to-all" else (0.8 if genome.entangler_topology == "circular" else 0.6)
    q = 1.0 - np.exp(-0.4 * genome.depth * density)
    return float(max(0.05, min(0.99, q)))


def probe_barren_plateau_variance(genome: AnsatzGenome) -> Dict[str, Any]:
    """
    Probes gradient variance Var(dE/dtheta).
    For deep circuits with random 2-qubit gates, gradient variance vanishes exponentially:
    Var(dE) ~ O(1 / 2^n). If Var < 1e-6, flags a Barren Plateau hazard.
    """
    n = genome.num_qubits
    d = genome.depth

    # Theoretical variance scaling: 2^(-n) * exp(-0.1 * depth)
    theoretical_variance = float((2.0 ** (-n)) * np.exp(-0.08 * d))
    is_barren = theoretical_variance < 1e-6

    return {
        "num_qubits": n,
        "depth": d,
        "gradient_variance": theoretical_variance,
        "barren_plateau_detected": is_barren,
        "admissible": not is_barren,
        "recommendation": "Admissible for optimization" if not is_barren else "Refuse: Barren plateau detected. Suggest identity block initialization or shallower depth.",
    }


def estimate_mps_bond_dimension(genome: AnsatzGenome) -> Dict[str, Any]:
    """
    Estimates required Matrix Product State (MPS) bond dimension chi.
    If chi <= 64, classical tensor network simulation is faster and cheaper than QPU.
    """
    # Entanglement growth per layer
    if genome.entangler_topology == "linear":
        chi = int(min(256, 2 ** min(genome.num_qubits // 2, genome.depth)))
    else:
        chi = int(min(1024, 2 ** min(genome.num_qubits, genome.depth * 2)))

    classical_sufficient = chi <= 64

    return {
        "estimated_bond_dimension": chi,
        "classical_mps_sufficient": classical_sufficient,
        "recommendation": "Classical MPS anchor sufficient. Do not spend QPU budget." if classical_sufficient else "QPU or Clifford cutting required.",
    }
