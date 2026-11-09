"""Adaptive VQE for the full molecular orbital space.

This module implements two real operator pools:

* ``uccsd`` — the fermionic singles/doubles pool from Qiskit Nature.  Each
  pool element is mapped through the same Jordan-Wigner mapper as the
  Hamiltonian; the anti-Hermitian fermionic generator becomes a Hermitian
  Pauli generator ``B`` with ``U(theta) = exp(i theta B)``.
* ``ceo`` — a Coupled Exchange Operator pool built from generalized qubit
  excitations (singles plus the OVP ``+``/``-`` coupled doubles described in
  Ramôa et al., npj Quantum Information 11, 82 (2025)).  The pool is
  constructed explicitly from qubit creation/annihilation operators; no
  fermionic anti-commutation string is inserted.

The algorithm is the standard ADAPT-VQE loop: evaluate the exact energy
derivative of every pool element at the current state, append the largest,
optimize every parameter, and stop on a gradient threshold, an operator cap,
a wall-clock cap, or when the *shot-measured* result reaches the target.  The
last point matters: the exact statevector energy is kept only as a labelled
optimizer diagnostic.  ``energy`` is always the shot-measured value and
``chemical_accuracy`` requires both the measured delta and its standard
uncertainty to be within the 1.6 mHa limit.
"""
from __future__ import annotations

import itertools
import math
import time
from dataclasses import dataclass, field
from typing import Any

import numpy as np

try:  # pragma: no cover - the bridge image always has these
    from qiskit.circuit.library import SdgGate, HGate
    from qiskit.quantum_info import SparsePauliOp, Statevector
    from qiskit_nature.second_q.circuit.library import HartreeFock, UCCSD
    from qiskit_nature.second_q.mappers import JordanWignerMapper

    HAS_ADAPT_DEPS = True
except Exception:  # pragma: no cover - exercised only in dependency-free shells
    HAS_ADAPT_DEPS = False


CHEMICAL_ACCURACY_MHA = 1.6


class AdaptError(RuntimeError):
    """A refused or invalid adaptive-VQE request."""


@dataclass(frozen=True)
class AdaptConfig:
    pool: str = "ceo"
    max_ops: int = 8
    max_iterations: int = 120
    gradient_tol: float = 1e-5
    time_limit_s: float = 900.0
    target_mha: float = CHEMICAL_ACCURACY_MHA
    shots: int = 4096
    seed: int = 7


@dataclass
class AdaptResult:
    energy_exact: float
    energy_measured: float
    uncertainty: float
    delta_mha: float
    exact_delta_mha: float
    measurements: dict[str, Any]
    theta: list[float]
    selected: list[dict[str, Any]]
    evaluations: int
    stopped: str
    elapsed_s: float
    pool_size: int
    metadata: dict[str, Any] = field(default_factory=dict)


def _require_deps() -> None:
    if not HAS_ADAPT_DEPS:
        raise AdaptError("qiskit-nature/qiskit unavailable: refusing adaptive VQE")


def _qdag(i: int, n: int) -> SparsePauliOp:
    return SparsePauliOp.from_sparse_list(
        [("X", [i], 0.5), ("Y", [i], -0.5j)], num_qubits=n
    )


def _qann(i: int, n: int) -> SparsePauliOp:
    return SparsePauliOp.from_sparse_list(
        [("X", [i], 0.5), ("Y", [i], 0.5j)], num_qubits=n
    )


def _hermitian_generator(op: SparsePauliOp) -> SparsePauliOp:
    """Return the Hermitian ``B`` with ``op = i B``.

    Qubit-excitation expressions are anti-Hermitian; multiplying by ``-i``
    yields the Hermitian generator accepted by ``exp(i theta B)``.  Tiny
    imaginary residues are rejected rather than silently dropped: a generator
    whose Hermiticity is not exact is a construction bug.
    """
    b = (op.simplify() * -1j).simplify()
    paulis: list[str] = []
    coeffs: list[complex] = []
    for pauli, coeff in zip(b.paulis, b.coeffs):
        if abs(complex(coeff).imag) > 1e-8:
            raise AdaptError(f"non-Hermitian generator coefficient {coeff!r}")
        if abs(complex(coeff).real) > 1e-12:
            paulis.append(pauli)
            coeffs.append(complex(complex(coeff).real, 0.0))
    if not paulis:
        raise AdaptError("empty generator")
    return SparsePauliOp(paulis, coeffs)


def _qe_single(i: int, j: int, n: int) -> SparsePauliOp:
    op = _qdag(j, n) @ _qann(i, n)
    return _hermitian_generator(op - op.adjoint())


def _qe_double(i: int, j: int, a: int, b: int, n: int) -> SparsePauliOp:
    """Generalized double qubit excitation annihilating i,j and creating a,b."""
    op = _qdag(a, n) @ _qdag(b, n) @ _qann(i, n) @ _qann(j, n)
    return _hermitian_generator(op - op.adjoint())


def _uccsd_pool(problem: Any, mapper: JordanWignerMapper) -> list[tuple[str, SparsePauliOp]]:
    hf = HartreeFock(problem.num_spatial_orbitals, problem.num_particles, mapper)
    uccsd = UCCSD(
        problem.num_spatial_orbitals,
        problem.num_particles,
        mapper,
        initial_state=hf,
    )
    pool: list[tuple[str, SparsePauliOp]] = []
    for index, op in enumerate(uccsd.excitation_ops()):
        mapped = mapper.map(op).simplify()
        # Qiskit Nature's UCCSD generators are Hermitian after Jordan-Wigner
        # mapping in the current pinned version.  Keep the conversion
        # defensive so a future convention change cannot silently produce a
        # non-unitary evolution.
        if not np.allclose(np.imag(mapped.coeffs), 0.0, atol=1e-10):
            mapped = _hermitian_generator(mapped)
        pool.append((f"uccsd[{index}]", mapped))
    if not pool:
        raise AdaptError("UCCSD pool is empty")
    return pool


def _ceo_pool(problem: Any, n: int) -> list[tuple[str, SparsePauliOp]]:
    """Build the generalized QE + OVP-CEO pool with explicit spin sectors.

    Qiskit Nature's Jordan-Wigner layout used by the pinned bridge image is
    block-ordered: the first ``nso`` spin orbitals are alpha and the next
    ``nso`` are beta (LiH's HF determinant is q0,q1,q6,q7, for example).
    The old even/odd assumption silently built a pool on the wrong spin
    sectors, which is why CEO-ADAPT stalled.  Derive the layout from the
    particle counts and assert the HF determinant's block occupations before
    constructing any operator.
    """
    nso = int(problem.num_spatial_orbitals)
    if n != 2 * nso:
        raise AdaptError(f"spin-orbital count {n} is not 2 x spatial orbitals {nso}")
    alpha = list(range(0, nso))
    beta = list(range(nso, 2 * nso))
    n_alpha = int(problem.num_particles[0])
    n_beta = int(problem.num_particles[1])
    if n_alpha < 0 or n_beta < 0 or n_alpha > nso or n_beta > nso:
        raise AdaptError("particle numbers do not fit the spin-orbital blocks")
    hf = Statevector.from_instruction(HartreeFock(nso, problem.num_particles, JordanWignerMapper())).data
    hf_index = int(np.argmax(np.abs(hf)))
    if abs(float(np.linalg.norm(hf)) - 1.0) > 1e-8:
        raise AdaptError("HF state is not normalized")
    occupied = {q for q in range(n) if (hf_index >> q) & 1}
    expected = set(range(n_alpha)) | set(range(nso, nso + n_beta))
    if occupied != expected:
        raise AdaptError(
            f"unexpected HF spin-block occupation {sorted(occupied)}; expected {sorted(expected)}"
        )

    pool: list[tuple[str, SparsePauliOp]] = []
    for spins in (alpha, beta):
        for i, j in itertools.combinations(spins, 2):
            pool.append((f"qe-single[{i},{j}]", _qe_single(i, j, n)))

    # Opposite-spin OVP-CEOs: two QEs acting on the same four spin-orbitals.
    for a1, a2 in itertools.combinations(alpha, 2):
        for b1, b2 in itertools.combinations(beta, 2):
            t1 = _qe_double(a1, b1, a2, b2, n)
            t2 = _qe_double(a2, b1, a1, b2, n)
            pool.append((f"ceo+[{a1},{b1},{a2},{b2}]", (t1 + t2).simplify()))
            pool.append((f"ceo-[{a1},{b1},{a2},{b2}]", (t1 - t2).simplify()))

    # Same-spin doubles: three distinct determinant exchanges act on the
    # same four spin-orbitals.  They form a coupled exchange family too; add
    # both individual QEs and the OVP +/- pair combinations so the pool is a
    # complete same-spin CEO construction, not a QEB shortcut.
    for spins in (alpha, beta):
        for comb in itertools.combinations(spins, 4):
            i, j, k, m = comb
            exchanges = [(i, j, k, m), (i, k, j, m), (i, m, j, k)]
            qes = []
            for s1, s2, t1, t2 in exchanges:
                qe = _qe_double(s1, s2, t1, t2, n)
                qes.append((qe,))
                pool.append((f"qe-double[{s1},{s2}->{t1},{t2}]", qe))
            for a in range(len(qes)):
                for b in range(a + 1, len(qes)):
                    first, second = qes[a][0], qes[b][0]
                    pool.append((f"ceo-same+[{i},{j},{k},{m}:{a}{b}]", (first + second).simplify()))
                    pool.append((f"ceo-same-[{i},{j},{k},{m}:{a}{b}]", (first - second).simplify()))
    if not pool:
        raise AdaptError("CEO pool is empty")
    return pool


def build_pool(problem: Any, mapper: JordanWignerMapper, pool_name: str) -> list[tuple[str, SparsePauliOp]]:
    name = (pool_name or "uccsd").strip().lower()
    if name == "uccsd":
        return _uccsd_pool(problem, mapper)
    if name == "ceo":
        n = 2 * int(problem.num_spatial_orbitals)
        return _ceo_pool(problem, n)
    raise AdaptError(f"unknown full-space pool {pool_name!r}: expected uccsd|ceo")


class _SparseState:
    """Hamiltonian and pool matrices shared by gradient/optimization steps."""

    def __init__(self, hamiltonian: SparsePauliOp, constant: float, pool: list[tuple[str, SparsePauliOp]], hf: np.ndarray):
        self.hamiltonian = hamiltonian
        self.constant = float(constant)
        self.h_matrix = hamiltonian.to_matrix(sparse=True)
        self.dim = int(2 ** hamiltonian.num_qubits)
        if hf.shape != (self.dim,):
            raise AdaptError("HF state dimension does not match the Hamiltonian")
        self.hf = np.asarray(hf, dtype=complex)
        self.pool = [(name, op, op.to_matrix(sparse=True)) for name, op in pool]

    def apply_layers(self, params: np.ndarray, layers: list[tuple[str, SparsePauliOp, Any]]) -> np.ndarray:
        from scipy.sparse.linalg import expm_multiply

        psi = self.hf.copy()
        for theta, (_name, _op, matrix) in zip(params, layers):
            psi = expm_multiply(1j * float(theta) * matrix, psi)
        return np.asarray(psi, dtype=complex)

    def energy_exact(self, psi: np.ndarray) -> float:
        return float(np.real(np.vdot(psi, self.h_matrix.dot(psi)))) + self.constant

    def gradient(self, psi: np.ndarray, matrix: Any) -> float:
        # For B Hermitian and U(theta)=exp(i theta B):
        # dE/dtheta|0 = -2 Im <psi|H B|psi>.
        return -2.0 * float(np.imag(np.vdot(psi, self.h_matrix.dot(matrix.dot(psi)))))


def _measure_state(
    state: np.ndarray,
    hamiltonian: SparsePauliOp,
    constant: float,
    shots: int,
    seed: int,
) -> tuple[float, float, dict[str, Any]]:
    """Shot-sample Hamiltonian terms in qubit-wise-commuting groups.

    Qubit-wise commuting terms can be measured from one basis rotation and
    one sample set.  This is the same measurement strategy a quantum backend
    would use; it is not an exact-expectation shortcut.  Each term's standard
    error is computed from its own sampled parity.
    """
    if shots < 1:
        raise AdaptError("shots must be positive")
    n = hamiltonian.num_qubits
    rng = np.random.default_rng(int(seed))
    base = Statevector(np.asarray(state, dtype=complex))
    total = 0.0
    variance = 0.0
    terms: dict[str, Any] = {}
    groups = hamiltonian.group_commuting(qubit_wise=True)
    if not groups:
        raise AdaptError("Hamiltonian has no measureable Pauli terms")
    for group in groups:
        # A qubit-wise commuting group has one basis per qubit.  Read it from
        # any non-identity member; verify every member agrees on that qubit.
        basis = ["I"] * n
        labels: list[tuple[str, float]] = []
        for pauli, coeff in zip(group.paulis, group.coeffs):
            label = pauli.to_label()
            c = float(np.real(coeff))
            if abs(c) < 1e-14:
                continue
            labels.append((label, c))
            for q, ch in enumerate(label[::-1]):
                if ch != "I":
                    if basis[q] not in ("I", ch):
                        raise AdaptError("group_commuting returned a non-commuting group")
                    basis[q] = ch
        if not labels:
            continue
        rotated = base
        for q, ch in enumerate(basis):
            if ch == "X":
                rotated = rotated.evolve(HGate(), qargs=[q])
            elif ch == "Y":
                rotated = rotated.evolve(SdgGate(), qargs=[q]).evolve(HGate(), qargs=[q])
            elif ch not in ("I", "Z"):
                raise AdaptError(f"invalid Pauli label {ch!r}")
        probs = np.abs(np.asarray(rotated.data, dtype=complex)) ** 2
        probs = probs / max(1e-15, float(probs.sum()))
        samples = rng.choice(1 << n, size=int(shots), p=probs)
        bits = np.array([[(int(x) >> q) & 1 for q in range(n)] for x in samples], dtype=np.int8)
        for label, c in labels:
            if set(label) == {"I"}:
                mean, std = 1.0, 0.0
            else:
                parity = np.zeros(len(samples), dtype=np.int8)
                for q, ch in enumerate(label[::-1]):
                    if ch != "I":
                        parity ^= bits[:, q]
                signs = 1.0 - 2.0 * parity.astype(float)
                mean = float(signs.mean())
                std = math.sqrt(max(0.0, 1.0 - mean * mean) / float(shots))
            terms[label] = {"coefficient": c, "expectation": float(mean), "std_error": float(std)}
            total += c * mean
            variance += (c * std) ** 2
    return float(total + constant), float(math.sqrt(max(0.0, variance))), terms


def run_adapt_vqe(
    problem: Any,
    hamiltonian: SparsePauliOp,
    constant: float,
    mapper: JordanWignerMapper,
    fci_energy: float,
    config: AdaptConfig | None = None,
) -> AdaptResult:
    """Run ADAPT-VQE against the full Hamiltonian.  No active-space reduction."""
    _require_deps()
    cfg = config or AdaptConfig()
    if cfg.max_ops < 1 or cfg.max_ops > 64:
        raise AdaptError(f"max_ops {cfg.max_ops} outside [1,64]")
    if cfg.max_iterations < 1 or cfg.max_iterations > 1000:
        raise AdaptError(f"max_iterations {cfg.max_iterations} outside [1,1000]")
    if cfg.time_limit_s <= 0:
        raise AdaptError("time_limit_s must be positive")
    if cfg.shots < 128 or cfg.shots > 1_000_000:
        raise AdaptError(f"shots {cfg.shots} outside [128,1000000]")

    hf = Statevector.from_instruction(
        HartreeFock(int(problem.num_spatial_orbitals), problem.num_particles, mapper)
    ).data
    pool = build_pool(problem, mapper, cfg.pool)
    engine = _SparseState(hamiltonian, constant, pool, hf)

    layers: list[tuple[str, SparsePauliOp, Any]] = []
    params = np.zeros(0, dtype=float)
    selected: list[dict[str, Any]] = []
    evaluations = 0
    started = time.monotonic()
    stopped = "max_ops"
    best_energy = engine.energy_exact(engine.apply_layers(params, layers))

    from scipy.optimize import minimize

    for iteration in range(cfg.max_ops):
        if time.monotonic() - started > cfg.time_limit_s:
            stopped = "time_limit"
            break
        psi = engine.apply_layers(params, layers)
        gradients: list[tuple[float, str, SparsePauliOp, Any]] = []
        for name, op, matrix in engine.pool:
            if time.monotonic() - started > cfg.time_limit_s:
                stopped = "time_limit"
                break
            gradients.append((abs(engine.gradient(psi, matrix)), name, op, matrix))
        if stopped == "time_limit":
            break
        if not gradients:
            stopped = "empty_pool"
            break
        norm = math.sqrt(sum(g[0] ** 2 for g in gradients))
        if norm <= cfg.gradient_tol:
            stopped = "gradient_converged"
            break
        gradients.sort(key=lambda item: item[0], reverse=True)
        chosen_grad, chosen_name, chosen_op, chosen_matrix = gradients[0]
        layers.append((chosen_name, chosen_op, chosen_matrix))
        params = np.r_[params, 0.0]
        selected.append({
            "iteration": iteration + 1,
            "operator": chosen_name,
            "gradient": float(chosen_grad),
        })

        def objective(x: np.ndarray) -> float:
            nonlocal evaluations
            evaluations += 1
            return engine.energy_exact(engine.apply_layers(np.asarray(x, dtype=float), layers))

        opt = minimize(
            objective,
            params,
            method="COBYLA",
            options={"maxiter": int(cfg.max_iterations), "rhobeg": 0.25, "tol": 1e-8},
        )
        candidate = np.asarray(opt.x if opt.x is not None else params, dtype=float)
        candidate_energy = engine.energy_exact(engine.apply_layers(candidate, layers))
        # ADAPT is variational: an optimizer step that increases the energy is
        # rejected rather than written into the answer.  The zero appended
        # parameter keeps the previous state exactly representable.
        if not np.isfinite(candidate_energy) or candidate_energy > best_energy + 1e-9:
            candidate = params
            candidate_energy = best_energy
        params = candidate
        best_energy = candidate_energy
        selected[-1]["energy_exact"] = float(best_energy)
        selected[-1]["parameters"] = [float(v) for v in params]

        # Do not stop on the exact statevector target: the claim is judged
        # on the shot-measured energy.  The loop runs to the operator cap (or
        # the time/gradient cap) and the measurement below decides whether the
        # target was actually reached.

    final_psi = engine.apply_layers(params, layers)
    exact = engine.energy_exact(final_psi)
    measured_energy, uncertainty, measurements = _measure_state(
        final_psi, hamiltonian, constant, cfg.shots, cfg.seed
    )
    delta_mha = abs(measured_energy - fci_energy) * 1000.0
    exact_delta_mha = abs(exact - fci_energy) * 1000.0
    # A shot result can land inside the target by noise even when the
    # variational state did not.  Chemical accuracy is claimed only when the
    # exact statevector diagnostic AND the measured value with its uncertainty
    # all pass.  The exact value is never reported as the measured energy.
    if delta_mha <= cfg.target_mha and uncertainty <= cfg.target_mha and exact_delta_mha <= cfg.target_mha:
        stopped = "within_target"
    elif delta_mha <= cfg.target_mha and uncertainty <= cfg.target_mha:
        stopped = "exact_target_not_reached"
    elif stopped == "max_ops" or stopped == "gradient_converged":
        stopped = "shot_noise_above_target"
    return AdaptResult(
        energy_exact=float(exact),
        energy_measured=float(measured_energy),
        uncertainty=float(uncertainty),
        delta_mha=float(delta_mha),
        exact_delta_mha=float(exact_delta_mha),
        measurements=measurements,
        theta=[float(v) for v in params],
        selected=selected,
        evaluations=int(evaluations),
        stopped=stopped,
        elapsed_s=float(time.monotonic() - started),
        pool_size=len(engine.pool),
        metadata={
            "pool": cfg.pool,
            "num_qubits": int(hamiltonian.num_qubits),
            "gradient_tol": cfg.gradient_tol,
            "time_limit_s": cfg.time_limit_s,
            "target_mha": cfg.target_mha,
            "shots": cfg.shots,
            "seed": cfg.seed,
        },
    )
