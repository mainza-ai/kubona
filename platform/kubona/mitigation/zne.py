"""Zero-Noise Extrapolation (ZNE) — the first mitigation technique (Phase 1).

Pipeline: run the circuit at increasing noise scales via TRUE unitary folding
(C -> C (C† C)^k: the folded circuit appends exact gate inverses, so the
logical unitary is unchanged and only the physical noise is amplified), estimate
the observable at each scale, then extrapolate to zero-noise (λ -> 0).

Mirrors Mitiq's approach but is implemented in-house on the noise-aware backend,
so the eval harness has a controllable, auditable reference implementation.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

import numpy as np

from kubona.circuits import Circuit, Gate
from kubona.circuits.model import SUPPORTED_GATES
from kubona.mitigation.backend import NoiseAwareSimulator

OBSERVABLES = {"z": lambda q, n: _observable_z(q, n)}


def _observable_z(q: int, n: int) -> np.ndarray:
    """Diagonal Z-on-qubit-q observable in the computational basis."""
    if not isinstance(q, int) or isinstance(q, bool) or q < 0 or q >= n:
        raise ValueError(f"observable qubit must be in [0, {n}), got {q!r}")
    s = 1 << (n - 1 - q)
    diag = np.array([1.0 if (i & s) == 0 else -1.0 for i in range(1 << n)])
    return diag


#: Self-inverse single-qubit gates (own inverse).
_SELF_INVERSE_1Q = frozenset({"id", "x", "y", "z", "h"})
#: Inverse-name pairs.
_INVERSE_PAIRS = {"s": "sdg", "sdg": "s", "t": "tdg", "tdg": "t"}
#: Gates inverted by negating the rotation angle.
_ANGLE_NEGATE = frozenset({"rx", "ry", "rz", "cp"})
#: Self-inverse two-qubit gates.
_SELF_INVERSE_2Q = frozenset({"cx", "cz", "swap"})

#: Richardson Vandermonde conditioning refusal threshold (v1 heuristic).
RICHARDSON_MAX_COND_V1_HEURISTIC = 1e12


def inverse_gate(gate: Gate) -> Gate:
    """Exact inverse of one gate (fail-closed: unknown inversions refuse).

    Every gate in SUPPORTED_GATES has an analytic inverse; anything else
    raises instead of being repeated (repetition is NOT an inverse for
    parameterized rotations — the old fold repeated G xN, which squares the
    rotation angle instead of amplifying noise at fixed unitary).
    """
    if gate.name in _SELF_INVERSE_1Q or gate.name in _SELF_INVERSE_2Q:
        return Gate(gate.name, gate.qubits, gate.params)
    if gate.name in _INVERSE_PAIRS:
        return Gate(_INVERSE_PAIRS[gate.name], gate.qubits, gate.params)
    if gate.name in _ANGLE_NEGATE:
        if len(gate.params) != 1:
            raise ValueError(f"cannot invert {gate.name!r} with params {gate.params!r}")
        return Gate(gate.name, gate.qubits, (-gate.params[0],))
    raise ValueError(
        f"no exact inverse for gate {gate.name!r}: ZNE folding refuses "
        f"(supported: {sorted(SUPPORTED_GATES)})"
    )


def fold_gates(circuit: Circuit, factor: int) -> Circuit:
    """True global unitary folding: C -> C (C† C)^k for scale factor 2k+1.

    factor = 1 -> unfolded (scale 1); factor = 3 -> C C† C (scale 3).
    The folded circuit implements EXACTLY the same unitary (each C† is built
    from exact gate inverses in reverse order) — only the executed depth, and
    hence the physical noise, scales. Factors must be odd positive ints:
    even factors refuse (C C† = I would measure the noise floor, not the
    observable — a silent identity dressed as amplification).
    """
    if isinstance(factor, bool) or not isinstance(factor, int) or factor < 1 \
            or factor % 2 == 0:
        raise ValueError(
            f"fold factor must be an odd positive int (1, 3, 5, ...), got {factor!r}"
        )
    if factor == 1:
        return Circuit(name=circuit.name, num_qubits=circuit.num_qubits,
                       gates=list(circuit.gates),
                       num_classical=circuit.num_classical)
    inverse_block = [inverse_gate(g) for g in reversed(circuit.gates)]
    gates = list(circuit.gates)
    for _ in range(factor // 2):
        gates.extend(inverse_block)
        gates.extend(circuit.gates)
    return Circuit(name=f"{circuit.name}+foldx{factor}", num_qubits=circuit.num_qubits,
                   gates=gates, num_classical=circuit.num_classical)


def expectation_from_counts(counts: dict[str, int], obs: np.ndarray, n: int) -> float:
    if not counts:
        raise ValueError(
            "expectation refused: empty counts dict (no shots recorded — "
            "a 0.0 expectation from no data is fabrication)"
        )
    total = sum(counts.values())
    if total <= 0:
        raise ValueError("expectation refused: counts sum to zero shots")
    e = 0.0
    for key, c in counts.items():
        i = int(key, 2)
        e += c * obs[i]
    return e / total


def _check_scales_values(scales: list[float], values: list[float],
                         need: int, name: str) -> None:
    if len(scales) != len(values):
        raise ValueError(f"{name}: {len(scales)} scales vs {len(values)} values")
    if len(scales) < need:
        raise ValueError(
            f"{name} needs >= {need} scale points, got {len(scales)}: refusing "
            "instead of fitting an underdetermined model"
        )
    if len(set(scales)) != len(scales):
        raise ValueError(f"{name}: duplicate scale factors {scales!r}")
    if any(not math.isfinite(s) or not math.isfinite(v)
           for s, v in zip(scales, values)):
        raise ValueError(f"{name}: non-finite scale/value refused")


def _vander_cond(scales: list[float], deg: int) -> float:
    V = np.vander(np.asarray(scales, dtype=float), N=deg + 1, increasing=True)
    return float(np.linalg.cond(V))


def _richardson(scales: list[float], values: list[float]) -> float:
    """Polynomial extrapolation to λ=0 (Richardson): fit degree n-1, eval at 0."""
    _check_scales_values(scales, values, 1, "richardson")
    deg = len(scales) - 1
    cond = _vander_cond(scales, deg)
    if cond > RICHARDSON_MAX_COND_V1_HEURISTIC:
        raise ValueError(
            f"richardson refused: Vandermonde cond {cond:.2e} > "
            f"{RICHARDSON_MAX_COND_V1_HEURISTIC:.0e} (extrapolation would "
            "amplify shot noise into fiction)"
        )
    coeffs = np.polyfit(scales, values, deg=deg)
    return float(np.polyval(coeffs, 0.0))


def _linear(scales: list[float], values: list[float]) -> float:
    """Linear fit through scale points, eval at 0 (conservative alternative)."""
    _check_scales_values(scales, values, 2, "linear")
    coeffs = np.polyfit(scales, values, deg=1)
    return float(np.polyval(coeffs, 0.0))


def _exponential(scales: list[float], values: list[float]) -> float:
    """Exponential-decay fit eval at 0 (for noise that compounds per fold).

    Fail-closed: non-positive values cannot log-fit — raises instead of
    inventing a number.
    """
    _check_scales_values(scales, values, 2, "exponential")
    if any(v <= 0 for v in values):
        raise ValueError("exponential extrapolator needs positive values")
    logged = [math.log(v) for v in values]
    coeffs = np.polyfit(scales, logged, deg=1)
    return float(math.exp(np.polyval(coeffs, 0.0)))


EXTRAPOLATORS = {"richardson": _richardson, "linear": _linear,
                 "exponential": _exponential}


def _prediction_weights(scales: list[float], deg: int) -> np.ndarray:
    """Row vector mapping per-scale values to the fit value at λ=0.

    The λ=0 prediction is linear in the values (m = w·v, Lagrange basis for
    exactly-determined fits, least-squares row for over-determined ones).
    """
    V = np.vander(np.asarray(scales, dtype=float), N=deg + 1, increasing=True)
    return np.linalg.pinv(V)[0]


def _fit_interval(scales: list[float], values: list[float],
                  extrapolator: str, shots: int) -> tuple[float, float, float]:
    """(value, 95%-ish half-width, fit condition number) for an extrapolator.

    Half-widths propagate per-scale shot noise through the λ=0 prediction
    weights: each scale value is a mean of ``shots`` ±1 samples (Var ≤ 1/shots),
    so Var(m) ≤ ||w||²/shots with w the prediction weights. A shot-noise-driven
    uncertainty interval, not a guarantee. Richardson conditioning is checked here too.
    """
    if shots < 1:
        raise ValueError("shots must be >= 1 for interval estimation")
    if extrapolator in ("richardson", "linear"):
        deg = len(scales) - 1 if extrapolator == "richardson" else 1
        cond = _vander_cond(scales, deg)
        if extrapolator == "richardson" and cond > RICHARDSON_MAX_COND_V1_HEURISTIC:
            raise ValueError(
                f"richardson refused: Vandermonde cond {cond:.2e} exceeds "
                f"{RICHARDSON_MAX_COND_V1_HEURISTIC:.0e}"
            )
        w = _prediction_weights(scales, deg)
        val = float(np.dot(w, np.asarray(values, dtype=float)))
        half = 1.96 * float(np.linalg.norm(w)) / math.sqrt(shots)
        return val, half, cond
    if extrapolator == "exponential":
        if any(v <= 0 for v in values):
            raise ValueError("exponential extrapolator needs positive values")
        w = _prediction_weights(scales, 1)
        logged = np.array([math.log(v) for v in values])
        log_val = float(np.dot(w, logged))
        val = float(math.exp(log_val))
        v = np.asarray(values, dtype=float)
        var_log = float(np.sum((w / v) ** 2)) / shots  # delta method
        half = 1.96 * val * math.sqrt(max(var_log, 0.0))
        return val, half, _vander_cond(scales, 1)
    raise ValueError(f"unknown extrapolator {extrapolator!r}; known: {sorted(EXTRAPOLATORS)}")


@dataclass
class ZNEResult:
    raw_value: float
    mitigated_value: float
    scale_values: dict[float, float]
    technique: str = "ZNE"
    fold: str = "global-unitary-fold"
    extrapolator: str = "Richardson"
    shots: int = 0
    seed: int | None = None
    # --- production-bar fields (defaults keep old constructions valid) ---
    scale_seeds: dict[float, int] = field(default_factory=dict)
    condition_number: float = 0.0
    interval_95: list[float] = field(default_factory=list)
    clip: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {"raw_value": self.raw_value, "mitigated_value": self.mitigated_value,
                "scale_values": {str(k): v for k, v in self.scale_values.items()},
                "technique": self.technique, "fold": self.fold, "extrapolator": self.extrapolator,
                "shots": self.shots, "seed": self.seed,
                "scale_seeds": {str(k): v for k, v in self.scale_seeds.items()},
                "condition_number": self.condition_number,
                "interval_95": list(self.interval_95),
                "clip": dict(self.clip)}

    def refit(self, extrapolator: str) -> "ZNEResult":
        """Re-estimate from the SAME shots with a different extrapolator.

        No new execution: the per-scale values are the archived measurement
        record, and each refit is one more estimator over identical data.
        Unknown extrapolators refuse (fail-closed, never a default swap).
        """
        if extrapolator not in EXTRAPOLATORS:
            raise ValueError(f"unknown extrapolator {extrapolator!r}; "
                             f"known: {sorted(EXTRAPOLATORS)}")
        scales = sorted(self.scale_values)
        values = [self.scale_values[s] for s in scales]
        val, half, cond = _fit_interval(list(map(float, scales)), values,
                                        extrapolator, self.shots)
        clipped, clip_flag = _bound_z_expectation(val)
        return ZNEResult(
            raw_value=self.raw_value,
            mitigated_value=clipped,
            scale_values=dict(self.scale_values),
            technique=self.technique, fold=self.fold,
            extrapolator=extrapolator, shots=self.shots, seed=self.seed,
            scale_seeds=dict(self.scale_seeds), condition_number=cond,
            interval_95=[clipped - half, clipped + half], clip=clip_flag)


def _bound_z_expectation(val: float) -> tuple[float, dict]:
    """<Z> lives in [-1, 1]: clip out-of-range extrapolations WITH a reason
    flag (or, when the violation is gross, the caller refuses — here the
    flag travels with the number so no reader mistakes it for a measurement).
    """
    if not math.isfinite(val):
        raise ValueError(f"mitigated value {val!r} is non-finite: refusing")
    if -1.0 <= val <= 1.0:
        return val, {"clipped": False, "reason": None}
    clipped = max(-1.0, min(1.0, val))
    return clipped, {"clipped": True,
                     "reason": f"extrapolated {val:.4f} outside [-1,1]; clipped (flagged, not measured)"}


def run_zne(circuit: Circuit, observable: str = "z", qubit: int = 0,
            shots: int | None = None, noise_rate: float | None = None,
            scales: tuple[int, ...] | None = None, seed: int | None = None,
            extrapolator: str = "richardson") -> ZNEResult:
    """Run ZNE on a circuit for a single-qubit Z observable.

    ``shots``, ``noise_rate``, ``scales`` and ``seed`` are REQUIRED (no
    silent 8000 / 0.02 / (1,3,5) / 7 defaults — a mitigation run must say
    what it ran). ``scales`` must be odd positive ints CONTAINING 1 (the raw
    reference); per-scale shots draw from SeedSequence-spawned child seeds so
    scale points are statistically independent yet reproducible.
    ``extrapolator`` selects the zero-noise estimator upfront.
    """
    if shots is None or noise_rate is None or scales is None or seed is None:
        raise ValueError(
            "run_zne requires explicit shots, noise_rate, scales and seed "
            "(no silent defaults: state what the mitigation ran)"
        )
    if isinstance(shots, bool) or not isinstance(shots, int) or shots < 1:
        raise ValueError(f"shots must be a positive int, got {shots!r}")
    if not isinstance(noise_rate, (int, float, np.floating)) \
            or not math.isfinite(noise_rate) or not 0.0 <= noise_rate <= 1.0:
        raise ValueError(f"noise_rate must be in [0, 1], got {noise_rate!r}")
    if observable not in OBSERVABLES:
        raise ValueError(
            f"unknown observable {observable!r}; known: {sorted(OBSERVABLES)}"
        )
    if extrapolator not in EXTRAPOLATORS:
        raise ValueError(f"unknown extrapolator {extrapolator!r}; "
                         f"known: {sorted(EXTRAPOLATORS)}")
    scales_t = tuple(int(s) for s in scales)
    if len(scales_t) < 2:
        raise ValueError(
            f"ZNE needs >= 2 scale points to extrapolate, got {scales_t!r} "
            "(a single scale is the raw value dressed as mitigation)"
        )
    if any(isinstance(s, bool) or s < 1 or s % 2 == 0 for s in scales_t):
        raise ValueError(
            f"scales must be odd positive ints (true unitary folding), got {scales_t!r}"
        )
    if 1 not in scales_t:
        raise ValueError(
            f"scales must contain 1 (the raw reference point), got {scales_t!r}"
        )
    if not isinstance(seed, (int, np.integer)):
        raise ValueError(f"seed must be an int, got {seed!r}")
    backend = NoiseAwareSimulator(noise_rate=float(noise_rate))
    n = circuit.num_qubits
    obs = _observable_z(qubit, n)
    # Per-scale SeedSequence children: independent yet reproducible streams.
    children = np.random.SeedSequence(int(seed)).spawn(len(scales_t))
    child_seeds = [int(c.generate_state(1)[0]) % (2 ** 63) for c in children]
    scale_values: dict[float, float] = {}
    scale_seeds: dict[float, int] = {}
    raw = None
    for s, cs in zip(scales_t, child_seeds):
        folded = circuit if s == 1 else fold_gates(circuit, s)
        counts = backend.run(folded, shots=shots, seed=cs)["counts"]
        val = expectation_from_counts(counts, obs, n)
        if s == 1:
            raw = val
        scale_values[float(s)] = val
        scale_seeds[float(s)] = cs
    ordered = sorted(scale_values)
    mit, half, cond = _fit_interval(ordered, [scale_values[s] for s in ordered],
                                    extrapolator, shots)
    clipped, clip_flag = _bound_z_expectation(mit)
    return ZNEResult(raw_value=float(raw), mitigated_value=clipped,
                     scale_values=scale_values, extrapolator=extrapolator,
                     shots=shots, seed=int(seed), scale_seeds=scale_seeds,
                     condition_number=cond,
                     interval_95=[clipped - half, clipped + half], clip=clip_flag)
