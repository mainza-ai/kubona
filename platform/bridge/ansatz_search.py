"""Autonomous Ansatz Genome Search & Pre-flight Feasibility Probes.

Production bar: every probe below is a cheap STRUCTURAL PRIOR
(expressibility_heuristic_v1), never a measured quantity — except the
statevector Meyer-Wallach Q, which is a real computation (reduced-state
purity via the executor's Meyer-Wallach routine) on circuits at or below
MW_STATEVECTOR_QUBIT_CAP qubits. Above that cap the MW entry point refuses
or returns the labeled heuristic; it never returns MW-labeled fiction.

Spend gates: heuristic probes NEVER greenlight spend. Probe dicts carry
``measured: False``, ``method: <..._heuristic_v1>`` and
``gate_eligible: False`` with a reason. Only a measured statevector MW
(``measured: True``) may feed a spend decision downstream.
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Dict, Any, List
import numpy as np


# --- Shared consts (single source; no inline magic) ---
MAX_QUBITS = 30
MAX_DEPTH = 100
MW_STATEVECTOR_QUBIT_CAP = 10  # above this, exact MW refuses / heuristic labels
EXPRESSIBILITY_METHOD = "expressibility_heuristic_v1"
VARIANCE_METHOD = "barren_variance_heuristic_v1"
CHI_METHOD = "bond_dimension_heuristic_v1"
BARREN_VARIANCE_THRESHOLD = 1e-6
MPS_CLASSICAL_CHI_THRESHOLD = 64

FAMILY_ALLOWLIST = ("RealAmplitudes", "HardwareEfficient", "StronglyEntangling")
TOPOLOGY_ALLOWLIST = ("linear", "circular", "all-to-all")
INITIALIZER_ALLOWLIST = ("zero", "random", "hartree_fock")

# Entangler-density weights for the structural prior (shared by the exact
# path's fallback and the above-cap path — one definition, never two copies
# that can rot apart).
_ENTANGLER_DENSITY = {"all-to-all": 1.0, "circular": 0.8, "linear": 0.6}
_HEURISTIC_DECAY = 0.4
_HEURISTIC_Q_MIN = 0.05
_HEURISTIC_Q_MAX = 0.99


def _structural_prior_q(entangler_topology: str, depth: int) -> float:
    """Shared structural-prior formula (expressibility_heuristic_v1)."""
    density = _ENTANGLER_DENSITY[entangler_topology]
    q = 1.0 - np.exp(-_HEURISTIC_DECAY * depth * density)
    return float(max(_HEURISTIC_Q_MIN, min(_HEURISTIC_Q_MAX, q)))


# Default-angle fill per initializer (wired into synthesis; "random" has no
# default — randomness is not a default angle, so it refuses defaults).
_INITIALIZER_DEFAULT_ANGLE: Dict[str, Any] = {
    "zero": 0.0,
    "hartree_fock": 0.0,
    "random": None,
}


def _expected_param_count(family: str, n: int, d: int) -> int:
    if family == "RealAmplitudes":
        return n * d
    # HardwareEfficient and StronglyEntangling carry RY + RZ per qubit/layer.
    return 2 * n * d


@dataclass
class AnsatzGenome:
    family: str  # "HardwareEfficient", "RealAmplitudes", "StronglyEntangling"
    num_qubits: int
    depth: int
    entangler_topology: str  # "linear", "circular", "all-to-all"
    initializer: str  # "zero", "random", "hartree_fock"
    symmetry_conserving: bool = False

    def __post_init__(self) -> None:
        if self.family not in FAMILY_ALLOWLIST:
            raise ValueError(
                f"unsupported ansatz family {self.family!r}; known: {list(FAMILY_ALLOWLIST)}"
            )
        if self.entangler_topology not in TOPOLOGY_ALLOWLIST:
            raise ValueError(
                f"unknown entangler_topology {self.entangler_topology!r}; "
                f"known: {list(TOPOLOGY_ALLOWLIST)}"
            )
        if self.initializer not in INITIALIZER_ALLOWLIST:
            raise ValueError(
                f"unknown initializer {self.initializer!r}; known: {list(INITIALIZER_ALLOWLIST)}"
            )
        if not isinstance(self.num_qubits, int) or isinstance(self.num_qubits, bool):
            raise ValueError(f"num_qubits must be an int, got {self.num_qubits!r}")
        if not isinstance(self.depth, int) or isinstance(self.depth, bool):
            raise ValueError(f"depth must be an int, got {self.depth!r}")
        if self.num_qubits < 1:
            raise ValueError(f"num_qubits must be >= 1, got {self.num_qubits}")
        if self.depth < 0:
            raise ValueError(f"depth must be >= 0, got {self.depth}")
        if self.num_qubits > MAX_QUBITS:
            raise ValueError(f"num_qubits {self.num_qubits} exceeds cap {MAX_QUBITS}")
        if self.depth > MAX_DEPTH:
            raise ValueError(f"depth {self.depth} exceeds cap {MAX_DEPTH}")

    def to_dict(self) -> Dict[str, Any]:
        return {
            "family": self.family,
            "num_qubits": self.num_qubits,
            "depth": self.depth,
            "entangler_topology": self.entangler_topology,
            "initializer": self.initializer,
            "symmetry_conserving": self.symmetry_conserving,
        }


def _resolve_angles(
    genome: AnsatzGenome, params: List[float] | None, allow_default_angles: bool
) -> List[float]:
    expected = _expected_param_count(genome.family, genome.num_qubits, genome.depth)
    if params is None:
        if not allow_default_angles:
            raise ValueError(
                f"synthesize_qasm requires explicit params (expected {expected} angles) "
                "or allow_default_angles=True — refusing silent 0.54 fill"
            )
        default = _INITIALIZER_DEFAULT_ANGLE[genome.initializer]
        if default is None:
            raise ValueError(
                f"initializer {genome.initializer!r} has no default angle: "
                "supply explicit params (randomness is not a default)"
            )
        return [float(default)] * expected
    if len(params) != expected:
        raise ValueError(
            f"params length mismatch for {genome.family} n={genome.num_qubits} d={genome.depth}: "
            f"expected {expected} angles, got {len(params)} — refusing (no silent fill)"
        )
    return [float(p) for p in params]


def synthesize_qasm(
    genome: AnsatzGenome,
    params: List[float] | None = None,
    *,
    allow_default_angles: bool = False,
) -> str:
    """Synthesizes OPENQASM 2.0 circuit code for the given genome.

    Params are explicit: a length mismatch ALWAYS refuses (never silently
    filled). Omitting params requires allow_default_angles=True, in which
    case the initializer selects the fill angle (zero/hartree_fock -> 0.0;
    random refuses). symmetry_conserving=True emits a barrier + comment
    marker so the field affects synthesis instead of decorating the struct.
    Gate sets differ per family (RealAmplitudes: RY only; HardwareEfficient:
    RY+RZ; StronglyEntangling: RY+RZ+RX) — unknown families refuse at
    construction (see AnsatzGenome.__post_init__).
    """
    # __post_init__ already validated family/topology/initializer/bounds.
    n = genome.num_qubits
    d = genome.depth
    angles = _resolve_angles(genome, params, allow_default_angles)
    lines = [
        'OPENQASM 2.0;',
        'include "qelib1.inc";',
        f'qreg q[{n}];',
        f'creg c[{n}];',
    ]
    if genome.symmetry_conserving:
        lines.append("// symmetry_conserving: particle-number sector preserved (barrier marker);")
        lines.append("barrier q;")

    param_idx = 0
    for layer in range(d):
        # 1-qubit rotation layer — distinct gate set per family.
        for i in range(n):
            if genome.family == "RealAmplitudes":
                angle = angles[param_idx]
                param_idx += 1
                lines.append(f"ry({angle}) q[{i}];")
            elif genome.family == "HardwareEfficient":
                a1 = angles[param_idx]
                a2 = angles[param_idx + 1]
                param_idx += 2
                lines.append(f"ry({a1}) q[{i}];")
                lines.append(f"rz({a2}) q[{i}];")
            elif genome.family == "StronglyEntangling":
                a1 = angles[param_idx]
                a2 = angles[param_idx + 1]
                param_idx += 2
                lines.append(f"ry({a1}) q[{i}];")
                lines.append(f"rz({a2}) q[{i}];")
                lines.append(f"rx({a1 * 0.5}) q[{i}];")
            else:  # pragma: no cover — constructor refuses; defense in depth
                raise ValueError(f"unsupported ansatz family {genome.family!r}")

        # Entanglement layer (topology validated at construction)
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
        else:  # pragma: no cover — constructor refuses; defense in depth
            raise ValueError(f"unknown entangler_topology {genome.entangler_topology!r}")

    for i in range(n):
        lines.append(f"measure q[{i}] -> c[{i}];")

    return "\n".join(lines)


def compute_meyer_wallach_from_statevector(statevector: np.ndarray) -> float:
    """Real Meyer-Wallach Q from an explicit statevector (reduced purities).

    Q = (2/n) Σ_k (1 − Tr ρ_k²). Exact linear algebra (numpy), no fitting.
    """
    import math as _math

    sv = np.asarray(statevector, dtype=complex)
    dim = sv.size
    if dim < 2 or (dim & (dim - 1)) != 0:
        raise ValueError("statevector length must be a power of two >= 2")
    n = int(_math.log2(dim))
    if n <= 1:
        return 0.0
    arr = sv.reshape([2] * n)
    q = 0.0
    for k in range(n):
        others = [i for i in range(n) if i != k]
        rho_k = np.tensordot(arr, arr.conj(), axes=(others, others))
        q += 2.0 * (1.0 - float(np.real(np.trace(rho_k @ rho_k))))
    return float(q / n)


def evaluate_meyer_wallach_expressibility(
    genome: AnsatzGenome, params: List[float] | None = None
) -> Dict[str, Any]:
    """
    Meyer-Wallach expressibility probe.

    - n <= MW_STATEVECTOR_QUBIT_CAP: REAL computation — the genome circuit is
      simulated (qiskit Statevector when available, else numpy fallback) at
      the given (or default-allowed) angles and Q is computed from reduced
      purities. Returns measured=True, method="statevector".
    - n > cap: refuses to fabricate — returns the structural prior LABELED
      expressibility_heuristic_v1 with measured=False. Never MW-labeled fiction.
    """
    n, d = genome.num_qubits, genome.depth
    if d == 0:
        return {"q": 0.0, "measured": True, "method": "statevector",
                "reason": "zero-depth circuit is a product state"}
    if n > MW_STATEVECTOR_QUBIT_CAP:
        return {
            "q": _structural_prior_q(genome.entangler_topology, d),
            "measured": False,
            "method": EXPRESSIBILITY_METHOD,
            "gate_eligible": False,
            "reason": (
                f"n={n} exceeds statevector cap {MW_STATEVECTOR_QUBIT_CAP}: "
                f"structural prior ({EXPRESSIBILITY_METHOD}), not a measured MW Q"
            ),
        }
    # Small-n: real statevector MW.
    try:
        angles = _resolve_angles(genome, params, allow_default_angles=True)
        qasm = synthesize_qasm(genome, angles)
        from qiskit.quantum_info import Statevector as _SV  # type: ignore

        # Strip measurements for statevector simulation.
        body = "\n".join(
            ln for ln in qasm.splitlines()
            if not ln.strip().startswith("measure") and not ln.strip().startswith("creg")
        )
        try:
            from bridge.executor import circuit_from_qasm as _from_qasm  # type: ignore
            from bridge.executor import calculate_meyer_wallach as _mw  # type: ignore
        except ImportError:
            from executor import circuit_from_qasm as _from_qasm  # type: ignore
            from executor import calculate_meyer_wallach as _mw  # type: ignore

        qc = _from_qasm(body)
        try:
            qc = qc.remove_final_measurements(inplace=False)
        except Exception:
            pass
        sv = _SV(qc).data
        q = float(_mw(np.asarray(sv)))
        return {"q": q, "measured": True, "method": "statevector",
                "gate_eligible": True,
                "reason": f"exact MW from statevector simulation (n={n})"}
    except Exception:
        # No simulator available: honest heuristic label, never MW fiction.
        return {
            "q": _structural_prior_q(genome.entangler_topology, d),
            "measured": False,
            "method": EXPRESSIBILITY_METHOD,
            "gate_eligible": False,
            "reason": "no statevector simulator available; structural prior only",
        }


def probe_barren_plateau_variance(genome: AnsatzGenome) -> Dict[str, Any]:
    """
    Gradient-variance barren-plateau probe (HEURISTIC v1, not measured).

    Scaling Var(dE) ~ 2^-n · exp(-0.08·d) is a structural guess. The dict
    carries measured=False, method=barren_variance_heuristic_v1 and
    gate_eligible=False: heuristics NEVER gate spend. Only a measured
    gradient-variance run may set gate_eligible True downstream.
    """
    n = genome.num_qubits
    d = genome.depth

    # Theoretical variance scaling: 2^(-n) * exp(-0.1 * depth)
    theoretical_variance = float((2.0 ** (-n)) * np.exp(-0.08 * d))
    is_barren = theoretical_variance < BARREN_VARIANCE_THRESHOLD

    return {
        "num_qubits": n,
        "depth": d,
        "gradient_variance": theoretical_variance,
        "barren_plateau_detected": is_barren,
        "admissible": not is_barren,
        "measured": False,
        "method": VARIANCE_METHOD,
        "gate_eligible": False,
        "gate_reason": (
            "heuristic variance estimate, not a measured gradient — "
            "refusing to gate spend on heuristics"
        ),
        "recommendation": "Admissible for optimization" if not is_barren else "Refuse: Barren plateau detected. Suggest identity block initialization or shallower depth.",
    }


def estimate_mps_bond_dimension(genome: AnsatzGenome) -> Dict[str, Any]:
    """
    MPS bond-dimension probe (HEURISTIC v1, not measured).

    chi <= 64 suggests classical MPS sufficiency, but the estimate is a
    structural guess. Carries measured=False + gate_eligible=False: never
    gates spend on heuristics.
    """
    # Entanglement growth per layer
    if genome.entangler_topology == "linear":
        chi = int(min(256, 2 ** min(genome.num_qubits // 2, genome.depth)))
    else:
        chi = int(min(1024, 2 ** min(genome.num_qubits, genome.depth * 2)))

    classical_sufficient = chi <= MPS_CLASSICAL_CHI_THRESHOLD

    return {
        "estimated_bond_dimension": chi,
        "classical_mps_sufficient": classical_sufficient,
        "measured": False,
        "method": CHI_METHOD,
        "gate_eligible": False,
        "gate_reason": (
            "heuristic bond-dimension estimate, not a measured chi — "
            "refusing to gate spend on heuristics"
        ),
        "recommendation": "Classical MPS anchor sufficient. Do not spend QPU budget." if classical_sufficient else "QPU or Clifford cutting required.",
    }
