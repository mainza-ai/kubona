"""Real variational experiment runners used by the bridge RPC.

This module is intentionally narrower than the research notebooks:

- H2 VQE in the STO-3G 2-qubit reduced space, with a live PySCF FCI anchor.
- H2 potential-energy scan built from the same VQE implementation.

The optimizer uses an exact statevector objective on Qiskit Aer's statevector
path; the reported energy is a *shot-measured* Pauli expectation, with the
exact optimizer value returned beside it and labelled. Unsupported molecules
refuse instead of borrowing H2 numbers.
"""
from __future__ import annotations

import base64
import io
import math
import os
import time
from datetime import datetime, timezone
from typing import Any

import numpy as np

try:  # Qiskit is part of the bridge image; source checkouts may miss it.
    from qiskit import QuantumCircuit
    from qiskit.quantum_info import Statevector
    HAS_QISKIT = True
except ImportError:  # pragma: no cover - bridge image always has it
    HAS_QISKIT = False

try:
    from . import simulator as sim_policy
except ImportError:  # bridge runs with the bridge dir on sys.path
    import simulator as sim_policy

try:
    from krylov import build_h2_hamiltonian_matrix
    HAS_PYSCF = True
except ImportError:  # pragma: no cover
    HAS_PYSCF = False

try:
    from qiskit_nature.second_q.drivers import PySCFDriver
    from qiskit_nature.second_q.transformers import ActiveSpaceTransformer
    from qiskit_nature.second_q.mappers import JordanWignerMapper
    from qiskit_nature.second_q.circuit.library import UCCSD, HartreeFock
    HAS_NATURE = True
except ImportError:  # pragma: no cover - bridge image installs qiskit extra
    HAS_NATURE = False


_I = np.eye(2, dtype=complex)
_X = np.array([[0, 1], [1, 0]], dtype=complex)
_Y = np.array([[0, -1j], [1j, 0]], dtype=complex)
_Z = np.array([[1, 0], [0, -1]], dtype=complex)
_PAULI = {"I": _I, "X": _X, "Y": _Y, "Z": _Z}
_TERMS = ("II", "IZ", "ZI", "ZZ", "XX", "YY")


def _ci95_mha(uncertainty_ha: float) -> float:
    """95% normal-approximation half-width, explicitly derived from the
    shot standard error. It is a sampling interval, not a systematic budget."""
    return float(1.96 * float(uncertainty_ha) * 1000.0)


def _claimable_against_anchor(energy: float, anchor: float, uncertainty: float,
                              limit_ha: float = 1.6e-3) -> bool:
    """Truth gate for a shot-measured energy against an exact anchor.

    A point below the anchor by more than three standard errors is a
    contradiction, not a pass: it must not be promoted to chemical accuracy
    merely because |delta| is small. The uncertainty must also fit inside the
    claim tolerance.
    """
    if not all(math.isfinite(v) for v in (float(energy), float(anchor), float(uncertainty))):
        return False
    if float(uncertainty) > float(limit_ha):
        return False
    if abs(float(energy) - float(anchor)) > float(limit_ha):
        return False
    return float(energy) >= float(anchor) - 3.0 * float(uncertainty)


def _calibration_quality(calibration: Any, max_age_hours: float | None = None) -> dict[str, Any]:
    """Derive freshness and deterministic snapshot ID from the provider snapshot."""
    limit = float(max_age_hours if max_age_hours is not None else
                  os.getenv("KUBONA_CALIBRATION_MAX_AGE_HOURS", "24"))
    if not isinstance(calibration, dict):
        return {"calibration_age_hours": None, "calibration_fresh": None,
                "calibration_checked_at": None, "calibration_max_age_hours": limit,
                "calibration_snapshot_id": None}
    raw = calibration.get("calibrated_at")
    if not isinstance(raw, str) or not raw.strip():
        return {"calibration_age_hours": None, "calibration_fresh": None,
                "calibration_checked_at": None, "calibration_max_age_hours": limit,
                "calibration_snapshot_id": None}
    snap_id = calibration.get("snapshot_id") or calibration.get("calibration_snapshot_id")
    if not isinstance(snap_id, str) or not snap_id.strip():
        import hashlib as _hashlib
        backend_key = str(calibration.get("backend") or calibration.get("backend_name") or "").strip()
        version_key = str(calibration.get("version") or "").strip()
        digest = _hashlib.sha256(f"{backend_key}|{version_key}|{raw.strip()}".encode("utf-8")).hexdigest()[:12]
        snap_id = f"cal_{digest}"
    try:
        at = datetime.fromisoformat(raw.replace("Z", "+00:00"))
        if at.tzinfo is None:
            at = at.replace(tzinfo=timezone.utc)
        age = max(0.0, (datetime.now(timezone.utc) - at).total_seconds() / 3600.0)
    except ValueError:
        return {"calibration_age_hours": None, "calibration_fresh": None,
                "calibration_checked_at": raw, "calibration_max_age_hours": limit,
                "calibration_snapshot_id": snap_id}
    return {"calibration_age_hours": age, "calibration_fresh": age <= limit,
            "calibration_checked_at": raw, "calibration_max_age_hours": limit,
            "calibration_snapshot_id": snap_id}


def _pauli_matrix(name: str) -> np.ndarray:
    if len(name) != 2 or any(ch not in _PAULI for ch in name):
        raise ValueError(f"invalid two-qubit Pauli label {name!r}")
    return np.kron(_PAULI[name[0]], _PAULI[name[1]])


def h2_hamiltonian(r: float) -> tuple[np.ndarray, float, float]:
    """Return the 4x4 H2 Hamiltonian, FCI energy, and RHF energy."""
    if not HAS_PYSCF:
        raise RuntimeError("PySCF unavailable: cannot compute the H2 Hamiltonian")
    h4, fci, _nuc = build_h2_hamiltonian_matrix(r=r)
    # The 4x4 matrix is ordered |1a1b>, |1a2b>, |2a1b>, |2a2b>. Qiskit's
    # 2-qubit statevector index q0 + 2*q1 uses the same four slots for this
    # reduced model; the labels are a basis relabelling, not a claim about
    # orbital identity.
    h4 = np.asarray(h4, dtype=complex)
    # RHF is the diagonal HF matrix element in the same reduced basis.
    rhf = float(np.real(h4[0, 0]))
    return h4, float(fci), rhf


def pauli_coefficients(h4: np.ndarray) -> dict[str, float]:
    """Decompose a 4x4 Hermitian matrix over the normalized Pauli basis."""
    out: dict[str, float] = {}
    for term in _TERMS:
        p = _pauli_matrix(term)
        c = float(np.real(np.trace(p @ h4)) / 4.0)
        if abs(c) > 1e-12:
            out[term] = c
    return out


def _ansatz(theta: np.ndarray, layers: int) -> QuantumCircuit:
    qc = QuantumCircuit(2)
    need = 4 * layers
    if len(theta) != need:
        raise ValueError(f"ansatz needs {need} parameters, got {len(theta)}")
    k = 0
    for _ in range(layers):
        qc.ry(float(theta[k]), 0)
        k += 1
        qc.ry(float(theta[k]), 1)
        k += 1
        qc.rz(float(theta[k]), 0)
        k += 1
        qc.rz(float(theta[k]), 1)
        k += 1
        qc.cx(0, 1)
    return qc


def _energy_exact(theta: np.ndarray, h4: np.ndarray, layers: int) -> float:
    state = Statevector.from_instruction(_ansatz(theta, layers)).data
    return float(np.real(np.vdot(state, h4 @ state)))


def _shot_expectation(
    qc: QuantumCircuit, term: str, shots: int, seed: int, sim: Any
) -> tuple[float, float]:
    """Measure one Pauli string and return (mean, standard error).

    Follows _pauli_matrix (np.kron(P[term[0]], P[term[1]])) and Qiskit's
    little-endian qubit convention: term[-1] acts on qubit 0 (LSB), term[0]
    acts on qubit n-1 (MSB).
    """
    if set(term) <= {"I"}:
        return 1.0, 0.0
    measured = qc.copy()
    rev_term = term[::-1]
    for q, ch in enumerate(rev_term):
        if ch == "X":
            measured.h(q)
        elif ch == "Y":
            measured.sdg(q)
            measured.h(q)
        # Z and I need no basis rotation.
    measured.measure_all()
    counts = sim.run(measured, shots=shots, seed_simulator=seed).result().get_counts()
    total = sum(counts.values())
    if total <= 0:
        raise RuntimeError("measurement returned zero counts")
    # Qiskit prints c[n-1]..c[0]; rev_term[0] is q0, so reverse the bitstring too.
    acc = 0
    for bitstring, n in counts.items():
        bits = bitstring.replace(" ", "")[::-1]
        parity = sum(1 for q, ch in enumerate(rev_term) if ch != "I" and bits[q] == "1")
        acc += n * (1 if parity % 2 == 0 else -1)
    mean = acc / total
    std = math.sqrt(max(0.0, 1.0 - mean * mean) / total)
    return float(mean), float(std)


def _shot_energy(
    theta: np.ndarray, coeffs: dict[str, float], layers: int,
    shots: int, seed: int,
) -> dict[str, Any]:
    sim = sim_policy.aer_simulator()
    qc = _ansatz(theta, layers)
    by_term: dict[str, Any] = {}
    energy = 0.0
    variance = 0.0
    for idx, (term, c) in enumerate(sorted(coeffs.items())):
        if set(term) <= {"I"}:
            mean, std = 1.0, 0.0
        else:
            mean, std = _shot_expectation(qc, term, shots, seed + idx, sim)
        by_term[term] = {"coefficient": c, "expectation": mean, "std_error": std}
        energy += c * mean
        variance += (c * std) ** 2
    return {
        "energy": float(energy),
        "uncertainty": float(math.sqrt(max(0.0, variance))),
        "shots": shots,
        "seed": seed,
        "terms": by_term,
    }


def _ansatz_qasm(theta: np.ndarray, layers: int) -> str:
    """Expose the optimized ansatz (without measurement) for admission/fidelity.

    This is a local CPU artifact, not a hardware result. It lets the Go parent
    experiment bind its mandate/fidelity check to the actual circuit before the
    bridge prepares measurement PUBs.
    """
    if not HAS_QISKIT:
        return ""
    try:
        from qiskit import qasm2
        return qasm2.dumps(_ansatz(theta, int(layers)))
    except Exception:
        return ""


def _hardware_batch(qasm_list: list[str], shots: int, seed: int,
                    backend: str | None = None,
                    shots_per_circuit: list[int] | None = None) -> dict[str, Any]:
    """Run one provider batch. Overridable in tests; no live call in CI."""
    from kubona.providers.qiskit_provider import QiskitProvider
    from kubona.schema import load_provider
    provider = QiskitProvider(load_provider("ibm-qiskit"), backend_name=backend)
    if shots_per_circuit is not None:
        return provider.submit_batch(
            qasm_list, shots=int(shots), seed=int(seed),
            shots_per_circuit=[int(s) for s in shots_per_circuit],
        )
    return provider.submit_batch(qasm_list, shots=int(shots), seed=int(seed))


def _layout_physical_qubits(layout: Any) -> list[int] | None:
    """Logical→physical qubits in logical order (falls back to layout.initial)."""
    if not isinstance(layout, dict):
        return None
    pq = layout.get("physical_qubits")
    if isinstance(pq, list) and all(isinstance(v, int) for v in pq):
        return pq
    initial = layout.get("initial")
    if isinstance(initial, dict):
        try:
            return [int(initial[k]) for k in sorted(initial, key=lambda x: int(x))]
        except Exception:
            return None
    return None


def _readout_mitigation_factor(term: str, calibration: dict[str, Any] | None, used_qubits: list[int] | None = None) -> float:
    """Diagonal TREX readout correction factor Π_{q ∈ supp(P)} (1 - 2 ε_q).

    Uses per-qubit readout error from live backend calibration when available,
    falling back to device median readout_error. Returns 1.0 (no-op) if
    calibration is absent or out of physical bounds (0, 0.25).
    """
    if not isinstance(calibration, dict):
        return 1.0
    per_q = calibration.get("per_qubit") if isinstance(calibration.get("per_qubit"), dict) else {}
    med_ro = calibration.get("readout_error")
    factor = 1.0
    for q, ch in enumerate(term[::-1]):
        if ch == "I":
            continue
        phys_q = used_qubits[q] if (used_qubits and q < len(used_qubits)) else q
        q_entry = per_q.get(str(phys_q)) if isinstance(per_q, dict) else None
        ro = q_entry.get("readout_error") if isinstance(q_entry, dict) else None
        if ro is None:
            ro = med_ro
        if ro is None:
            continue
        eps = float(ro)
        if 0.0 < eps < 0.25:
            factor *= (1.0 - 2.0 * eps)
    return factor


def _hardware_counts_to_energy(
    counts_list: list[dict[str, int]],
    coeffs: dict[str, float],
    *,
    calibration: dict[str, Any] | None = None,
    used_qubits: list[int] | None = None,
    readout_mitigation: bool = False,
) -> dict[str, Any]:
    by_term: dict[str, Any] = {}
    energy = 0.0
    variance = 0.0
    ro_factors: dict[str, float] = {}
    for term, counts in zip(sorted(coeffs), counts_list):
        c = float(coeffs[term])
        if term == "II" or set(term) == {"I"}:
            mean, std = 1.0, 0.0
            factor = 1.0
        else:
            total = sum(int(v) for v in counts.values())
            if total <= 0:
                raise RuntimeError(f"hardware measurement for {term} returned zero counts")
            acc = 0
            for bitstring, n in counts.items():
                bits = str(bitstring).replace(" ", "")[::-1]
                if len(bits) < len(term):
                    raise RuntimeError(
                        f"counts bitstring {bitstring!r} has {len(bits)} bits but term {term!r} needs {len(term)}"
                    )
                parity = sum(1 for q, ch in enumerate(term[::-1]) if ch != "I" and bits[q] == "1")
                acc += int(n) * (1 if parity % 2 == 0 else -1)
            raw_mean = acc / total
            raw_std = math.sqrt(max(0.0, 1.0 - raw_mean * raw_mean) / total)
            factor = _readout_mitigation_factor(term, calibration, used_qubits) if readout_mitigation else 1.0
            mean = max(-1.0, min(1.0, raw_mean / factor)) if factor > 0 else raw_mean
            std = (raw_std / factor) if factor > 0 else raw_std
        ro_factors[term] = float(factor)
        by_term[term] = {"coefficient": c, "expectation": float(mean), "std_error": float(std)}
        energy += c * mean
        variance += (c * std) ** 2
    out = {
        "energy": float(energy),
        "uncertainty": float(math.sqrt(max(0.0, variance))),
        "terms": by_term,
    }
    if readout_mitigation:
        out["readout_mitigation"] = {
            "enabled": True,
            "applied": any(abs(f - 1.0) > 1e-9 for f in ro_factors.values()),
            "method": "diagonal_trex_from_calibration",
            "factors": ro_factors,
        }
    return out


def _fold_unitary(qc: Any, scale: int) -> Any:
    """Exact unitary folding for an odd integer noise scale.

    ``U | U^dag | U`` for scale 3, ``U | U^dag | U | U^dag | U`` for scale 5.
    Barriers are inserted between every fold segment so Qiskit's transpiler
    (``optimization_level >= 1``) cannot algebraically cancel ``U U^dag -> I``
    (G1 fix: prevents silent fold collapse on real hardware).
    """
    if isinstance(scale, bool) or not isinstance(scale, int) or scale < 1 or scale % 2 == 0:
        raise ValueError(f"unitary folding scale must be a positive odd integer, got {scale!r}")
    if scale == 1:
        return qc.copy()
    base = qc.copy()
    inverse = base.inverse()
    folded = base.copy()
    for _ in range((scale - 1) // 2):
        folded.barrier()
        folded.compose(inverse, inplace=True)
        folded.barrier()
        folded.compose(base, inplace=True)
    return folded


def _fold_local_2q(qc: Any, scale: float) -> Any:
    """Local barrier-protected two-qubit gate folding for any scale >= 1.0 (P6-4).

    For a fractional scale such as 1.5, 2.0, or 2.5, folds a deterministic
    subset of two-qubit gates ``G -> barrier · G · barrier · G^dag · barrier · G``
    so the effective 2Q gate count scales smoothly without jumping directly into
    exponential noise saturation (global 1->3->5). For odd integer scales on
    circuits with no 2Q gates, falls back to ``_fold_unitary``.
    """
    if isinstance(scale, bool) or not isinstance(scale, (int, float)) or not math.isfinite(float(scale)) or float(scale) < 1.0:
        raise ValueError(f"local folding scale must be a finite number >= 1.0, got {scale!r}")
    s = float(scale)
    if abs(s - 1.0) < 1e-9:
        return qc.copy()
    twoq_indices = [
        i for i, inst in enumerate(qc.data)
        if len(inst.qubits) >= 2 and str(getattr(inst.operation, "name", "")) not in ("barrier", "measure", "reset")
    ]
    if not twoq_indices:
        odd = int(round(s))
        if odd % 2 == 0:
            odd += 1
        return _fold_unitary(qc, max(1, odd))
    # Each extra fold on a 2Q gate adds +2 two-qubit gates (G -> G G^dag G).
    # Target total 2Q gates ≈ round(s * n_2q), so total folds = round((s - 1) * n_2q / 2).
    n_2q = len(twoq_indices)
    total_folds = max(1, int(round((s - 1.0) * n_2q / 2.0)))
    base_folds = total_folds // n_2q
    rem = total_folds % n_2q
    # Deterministically spread remainder folds from the back (highest-impact gates)
    extra_set = set(twoq_indices[n_2q - rem:]) if rem > 0 else set()
    folded = qc.copy_empty_like()
    for i, inst in enumerate(qc.data):
        op, qargs, cargs = inst.operation, inst.qubits, inst.clbits
        folded.append(op, qargs, cargs)
        if i in twoq_indices:
            k_folds = base_folds + (1 if i in extra_set else 0)
            if k_folds > 0:
                inv_op = op.inverse()
                for _ in range(k_folds):
                    folded.barrier(*qargs)
                    folded.append(inv_op, qargs, cargs)
                    folded.barrier(*qargs)
                    folded.append(op, qargs, cargs)
    return folded


def _fold_local_all(qc: Any, scale: float) -> Any:
    """Stratified barrier-protected local folding of ALL gates (1Q and 2Q).

    Folds single-qubit and two-qubit gates independently in proportion to
    ``(scale - 1) / 2`` so fractional scales cannot preferentially fold 2Q
    gates sitting at the tail of the circuit (the A1 defect found in the
    2026-11-26 R2 raw-count audit). Barriers prevent ``G G^dag`` cancellation.
    """
    if isinstance(scale, bool) or not isinstance(scale, (int, float)) or not math.isfinite(float(scale)) or float(scale) < 1.0:
        raise ValueError(f"local folding scale must be a finite number >= 1.0, got {scale!r}")
    s = float(scale)
    if abs(s - 1.0) < 1e-9:
        return qc.copy()
    one_q_indices = [
        i for i, inst in enumerate(qc.data)
        if str(getattr(inst.operation, "name", "")) not in ("barrier", "measure", "reset")
        and len(inst.qubits) < 2
    ]
    two_q_indices = [
        i for i, inst in enumerate(qc.data)
        if str(getattr(inst.operation, "name", "")) not in ("barrier", "measure", "reset")
        and len(inst.qubits) >= 2
    ]
    if not one_q_indices and not two_q_indices:
        return qc.copy()

    def _allocate_folds(indices: list[int], total_folds: int) -> dict[int, int]:
        if not indices or total_folds <= 0:
            return {}
        n = len(indices)
        base = total_folds // n
        rem = total_folds % n
        # Spread remainder evenly across the class instead of slicing the tail.
        extra: set[int] = set()
        if rem > 0:
            for k in range(rem):
                pos = int((k * n) // rem)
                extra.add(indices[pos])
        return {idx: base + (1 if idx in extra else 0) for idx in indices}

    folds_1q = int(round((s - 1.0) * len(one_q_indices) / 2.0)) if one_q_indices else 0
    folds_2q = int(round((s - 1.0) * len(two_q_indices) / 2.0)) if two_q_indices else 0
    if folds_1q == 0 and folds_2q == 0:
        if two_q_indices:
            folds_2q = 1
        else:
            folds_1q = 1
    fold_plan: dict[int, int] = {}
    fold_plan.update(_allocate_folds(one_q_indices, folds_1q))
    fold_plan.update(_allocate_folds(two_q_indices, folds_2q))

    folded = qc.copy_empty_like()
    for i, inst in enumerate(qc.data):
        op, qargs, cargs = inst.operation, inst.qubits, inst.clbits
        folded.append(op, qargs, cargs)
        k_folds = fold_plan.get(i, 0)
        if k_folds > 0:
            inv_op = op.inverse()
            for _ in range(k_folds):
                folded.barrier(*qargs)
                folded.append(inv_op, qargs, cargs)
                folded.barrier(*qargs)
                folded.append(op, qargs, cargs)
    return folded


def _fold_circuit(qc: Any, scale: float) -> Any:
    """Canonical ZNE folding: all-gate local folding for every scale > 1.

    ``_fold_unitary`` remains available for explicit global experiments, but it
    must never be mixed with local folding in one fit (the R2 audit showed that
    mixing global scale 3 with local scales 2/4 made the x-axis meaningless).
    """
    if isinstance(scale, bool) or not isinstance(scale, (int, float)) or not math.isfinite(float(scale)) or float(scale) < 1.0:
        raise ValueError(f"folding scale must be a finite number >= 1.0, got {scale!r}")
    s = float(scale)
    if abs(s - 1.0) < 1e-9:
        return qc.copy()
    return _fold_local_all(qc, s)


def _extrapolate_zne(
    xs: list[float],
    scale_energies: list[float],
    scale_uncertainties: list[float],
    extrapolator: str = "linear",
    asymptote: float | None = None,
) -> dict[str, Any]:
    """Shared ZNE extrapolator supporting WLS linear, 2-param/3-param exponential, and polynomial fits (P6-1 / R2).

    When ``asymptote`` (the exact depolarizing maximally-mixed energy ``c_II``)
    is provided, ``exponential`` fits the 2-parameter model
    ``E(x) = asymptote + b * exp(-c * x)`` so 3 scales give ``dof = 1`` and 4
    scales give ``dof = 2`` (avoiding the 3-point ``dof = 0`` covariance blow-up).
    Without an asymptote, 3-parameter exponential requires ``len(xs) >= 4`` and
    falls back to WLS linear otherwise.
    """
    mode = str(extrapolator or "linear").strip().lower()
    if mode not in ("linear", "exponential", "quadratic", "richardson"):
        raise ValueError(f"unsupported ZNE extrapolator {extrapolator!r}; use linear|exponential|quadratic|richardson")
    xs_arr = np.asarray(xs, dtype=float)
    ys_arr = np.asarray(scale_energies, dtype=float)
    sig_arr = np.maximum(np.asarray(scale_uncertainties, dtype=float), 1e-9)

    used_mode = mode
    if mode == "exponential" and (
        (asymptote is not None and math.isfinite(float(asymptote)) and len(xs) >= 3)
        or len(xs) >= 4
    ):
        try:
            from scipy.optimize import curve_fit

            if asymptote is not None and math.isfinite(float(asymptote)):
                a_fixed = float(asymptote)

                def _exp2(x, b, c):
                    return a_fixed + b * np.exp(-c * x)

                b0 = float(ys_arr[0] - a_fixed)
                if abs(b0) < 1e-9:
                    raise ValueError("scale-1 equals asymptote")
                popt, pcov = curve_fit(
                    _exp2,
                    xs_arr,
                    ys_arr,
                    p0=[b0, 0.25],
                    sigma=sig_arr,
                    absolute_sigma=True,
                    bounds=([-np.inf, 1e-4], [np.inf, 10.0]),
                    maxfev=4000,
                )
                b_fit, c_fit = float(popt[0]), float(popt[1])
                mitigated = float(a_fixed + b_fit)
                predicted = _exp2(xs_arr, b_fit, c_fit)
                rmse = float(np.sqrt(np.mean((ys_arr - predicted) ** 2)))
                var0 = float(pcov[0, 0]) if (pcov is not None and np.isfinite(pcov[0, 0]) and pcov[0, 0] >= 0) else float(sig_arr[0] ** 2)
                dof = max(1, len(xs) - 2)
                chi2_red = float(np.sum(((ys_arr - predicted) / sig_arr) ** 2) / dof)
                uncertainty = float(math.sqrt(max(var0, 0.0) + rmse * rmse))
                return {
                    "extrapolator": mode,
                    "extrapolator_used": "exponential_fixed_asymptote",
                    "fit_degree": 0,
                    "mitigated": mitigated,
                    "uncertainty": uncertainty,
                    "fit_rmse": rmse,
                    "chi2_reduced": chi2_red,
                    "fit_condition_number": float(np.linalg.cond(pcov)) if (pcov is not None and np.all(np.isfinite(pcov))) else 1.0,
                    "intercept_weights": [],
                    "exp_params": {"asymptote": a_fixed, "asymptote_fixed": True, "amplitude": b_fit, "decay_rate": c_fit},
                    "uncertainty_method": "exponential_2param_wls_quadrature_rmse",
                }

            def _exp_model(x, a, b, c):
                return a + b * np.exp(-c * x)

            a0 = float(ys_arr[-1])
            b0 = float(ys_arr[0] - ys_arr[-1])
            if abs(b0) < 1e-9:
                raise ValueError("flat scale energies")
            popt, pcov = curve_fit(
                _exp_model,
                xs_arr,
                ys_arr,
                p0=[a0, b0, 0.5],
                sigma=sig_arr,
                absolute_sigma=True,
                bounds=([-np.inf, -np.inf, 1e-4], [np.inf, np.inf, 10.0]),
                maxfev=4000,
            )
            a_fit, b_fit, c_fit = (float(popt[0]), float(popt[1]), float(popt[2]))
            mitigated = float(a_fit + b_fit)
            predicted = _exp_model(xs_arr, a_fit, b_fit, c_fit)
            rmse = float(np.sqrt(np.mean((ys_arr - predicted) ** 2)))
            if pcov is not None and np.all(np.isfinite(pcov[:2, :2])):
                var0 = float(max(0.0, pcov[0, 0] + pcov[1, 1] + 2.0 * pcov[0, 1]))
            else:
                var0 = float(sig_arr[0] ** 2)
            dof = max(1, len(xs) - 3)
            chi2_red = float(np.sum(((ys_arr - predicted) / sig_arr) ** 2) / dof)
            uncertainty = float(math.sqrt(max(var0, 0.0) + rmse * rmse))
            return {
                "extrapolator": mode,
                "extrapolator_used": "exponential",
                "fit_degree": 0,
                "mitigated": mitigated,
                "uncertainty": uncertainty,
                "fit_rmse": rmse,
                "chi2_reduced": chi2_red,
                "fit_condition_number": float(np.linalg.cond(pcov[:2, :2])) if (pcov is not None and np.all(np.isfinite(pcov[:2, :2]))) else 1.0,
                "intercept_weights": [],
                "exp_params": {"asymptote": a_fit, "asymptote_fixed": False, "amplitude": b_fit, "decay_rate": c_fit},
                "uncertainty_method": "exponential_wls_quadrature_rmse",
            }
        except Exception:
            used_mode = "linear"
    elif mode == "exponential":
        used_mode = "linear"

    if used_mode == "linear":
        deg = 1
        w = 1.0 / (sig_arr ** 2)
        sw = float(np.sum(w))
        sx = float(np.sum(w * xs_arr))
        sxx = float(np.sum(w * xs_arr * xs_arr))
        det = sw * sxx - sx * sx
        if not math.isfinite(det) or det <= 1e-18:
            raise ValueError("ZNE WLS linear fit singular; refusing")
        cond = float(np.linalg.cond(np.vander(xs_arr, 2)))
        intercept_weights = (w * (sxx - sx * xs_arr)) / det
        mitigated = float(np.sum(intercept_weights * ys_arr))
        slope = float(np.sum((w * (sw * xs_arr - sx)) * ys_arr) / det)
        predicted = mitigated + slope * xs_arr
        rmse = float(np.sqrt(np.mean((ys_arr - predicted) ** 2)))
        var0 = float(sxx / det)
        dof = max(1, len(xs) - 2)
        chi2_red = float(np.sum(w * ((ys_arr - predicted) ** 2)) / dof) if len(xs) > 2 else 0.0
        uncertainty = float(math.sqrt(max(var0, 0.0) + rmse * rmse))
        return {
            "extrapolator": mode,
            "extrapolator_used": "linear",
            "fit_degree": deg,
            "mitigated": mitigated,
            "uncertainty": uncertainty,
            "fit_rmse": rmse,
            "chi2_reduced": chi2_red,
            "fit_condition_number": cond,
            "intercept_weights": [float(v) for v in intercept_weights],
            "uncertainty_method": "wls_linear_propagated_plus_fit_rmse",
        }

    deg = min(len(xs) - 1, 2)
    vander = np.vander(xs_arr, deg + 1)
    cond = float(np.linalg.cond(vander))
    if not math.isfinite(cond) or cond > 1e12:
        raise ValueError(f"ZNE fit ill-conditioned (cond={cond:.3g}); refusing")
    coeffs_fit = np.polyfit(xs_arr, ys_arr, deg=deg)
    mitigated = float(np.polyval(coeffs_fit, 0.0))
    predicted = np.polyval(coeffs_fit, xs_arr)
    rmse = float(np.sqrt(np.mean((ys_arr - predicted) ** 2)))
    intercept_weights = np.linalg.pinv(vander)[deg]
    var0 = float(np.sum((intercept_weights * sig_arr) ** 2))
    uncertainty = float(math.sqrt(max(var0, 0.0) + rmse * rmse))
    return {
        "extrapolator": mode,
        "extrapolator_used": mode,
        "fit_degree": deg,
        "mitigated": mitigated,
        "uncertainty": uncertainty,
        "fit_rmse": rmse,
        "chi2_reduced": 0.0,
        "fit_condition_number": cond,
        "intercept_weights": [float(v) for v in intercept_weights],
        "uncertainty_method": "propagated_scale_errors_plus_fit_rmse",
    }


def _measurement_qasm(qc: Any, term: str) -> str:
    from qiskit import qasm2
    measured = qc.copy()
    # Qiskit Pauli labels are little-endian: term[-1] applies to qubit 0.
    for q, ch in enumerate(term[::-1]):
        if ch == "X":
            measured.h(q)
        elif ch == "Y":
            measured.sdg(q)
            measured.h(q)
    measured.measure_all()
    return qasm2.dumps(measured)


def run_vqe_hardware(
    theta: list[float] | None = None,
    pauli_coefficients: dict[str, float] | None = None,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    bond_distance: float = 0.7414,
    fci_exact_energy: float | None = None,
    backend: str | None = None,
    *,
    molecule: str = "H2",
    ansatz_qasm: str | None = None,
    constant_energy: float = 0.0,
    mitigation: str | None = None,
    mitigation_scales: list[int] | None = None,
    extrapolator: str = "linear",
    readout_mitigation: bool = True,
    cdr_training_circuits: int = 12,
    live_readout_cal: bool | None = None,
    symmetry_verification: bool | None = None,
    target_shots: list[int] | None = None,
    qwc_grouping: bool | None = None,
) -> dict[str, Any]:
    """Execute a locally optimized H2 ansatz on a real QPU as one batch.

    The optimizer stays local (``simulated_optimizer:true``); every reported
    energy term comes from hardware counts. One provider batch is one vendor
    job, one usage reading, and one settlement — not N circuit jobs.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing hardware VQE"}
    if ansatz_qasm is None and (theta is None or not isinstance(theta, (list, tuple))):
        return {"success": False, "error": "hardware VQE requires the local optimizer theta or an ansatz QASM"}
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]: refusing"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]: refusing"}
    theta_arr = np.asarray(theta, dtype=float) if theta is not None else np.zeros(0, dtype=float)
    if ansatz_qasm is None:
        if layers < 1 or layers > 8:
            return {"success": False, "error": f"layers {layers!r} outside [1,8]: refusing"}
        if theta_arr.size != 4 * int(layers):
            return {"success": False, "error": f"theta has {theta_arr.size} values; need {4 * int(layers)}"}
        qc = _ansatz(theta_arr, int(layers))
    else:
        # An active-space ansatz QASM is fully bound; its parameter count is
        # not the hardware-efficient 4*layers shape. Do not reject a valid
        # molecular ansatz merely because the source did not report layers.
        layers = max(1, min(8, int(layers or 1)))
        try:
            from qiskit import qasm2
            qc = qasm2.loads(ansatz_qasm)
        except Exception as e:  # noqa: BLE001
            return {"success": False, "error": f"ansatz QASM is not executable: {e}"}
    coeffs = dict(pauli_coefficients or {})
    if not coeffs:
        h4, _, _ = h2_hamiltonian(float(bond_distance))
        coeffs = pauli_coefficients(h4)
    if fci_exact_energy is None:
        _, fci_exact_energy, _ = h2_hamiltonian(float(bond_distance))
    method = (mitigation or "").strip().upper()
    if method and method not in ("ZNE", "CDR"):
        return {"success": False, "error": f"hardware energy mitigation {method!r} is unsupported; use ZNE or CDR"}

    if method == "CDR":
        return _run_vqe_hardware_cdr(
            qc=qc,
            theta_arr=theta_arr,
            coeffs=coeffs,
            layers=int(layers),
            shots=int(shots),
            seed=int(seed),
            bond_distance=float(bond_distance),
            fci_exact_energy=float(fci_exact_energy),
            backend=backend,
            molecule=str(molecule or "H2"),
            ansatz_qasm=ansatz_qasm,
            constant_energy=float(constant_energy),
            readout_mitigation=bool(readout_mitigation),
            cdr_training_circuits=int(cdr_training_circuits or 12),
            live_readout_cal=(True if live_readout_cal is None else bool(live_readout_cal)),
            symmetry_verification=(True if symmetry_verification is None else bool(symmetry_verification)),
            target_shots=target_shots,
        )

    if method == "ZNE":
        if mitigation_scales is None:
            mitigation_scales = [1, 3, 5]
        try:
            xs_valid = [float(s) for s in mitigation_scales]
        except (TypeError, ValueError):
            return {"success": False, "error": "ZNE mitigation_scales must be numeric values >= 1.0"}
        if len(set(xs_valid)) < 2:
            return {"success": False, "error": "ZNE mitigation requires at least two distinct scales"}
        for scale in mitigation_scales:
            if isinstance(scale, bool) or not isinstance(scale, (int, float)) or not math.isfinite(float(scale)) or float(scale) < 1.0 or float(scale) > 15.0:
                return {"success": False, "error": f"ZNE scale {scale!r} must be a finite number in [1.0, 15.0]"}
        if int(qc.num_clbits) > 0 or any(
            getattr(inst.operation, "name", "") == "measure" for inst in qc.data
        ):
            return {"success": False, "error": "ZNE folding requires a unitary ansatz without measurement"}

    qasms: list[str] = []
    fold_keys: list[tuple[str, float]] = []
    scale_map: list[tuple[float, float, Any, tuple[int, int, int]]] = []
    if method:
        # Canonical R2 axis: all-gate local folding; the x coordinate is the
        # measured TOTAL-gate ratio, not the 2Q-only ratio. A scale set that
        # rounds to duplicate/inconsistent total-gate ratios is refused.
        def _gate_counts(circuit: Any) -> tuple[int, int, int]:
            n1 = n2 = 0
            for inst in circuit.data:
                name = str(getattr(inst.operation, "name", ""))
                if name in ("barrier", "measure", "reset"):
                    continue
                if len(inst.qubits) >= 2:
                    n2 += 1
                else:
                    n1 += 1
            return n1, n2, n1 + n2
        base_counts = _gate_counts(qc)
        base_n2 = base_counts[1]
        base_total = base_counts[2]
        if base_total <= 0:
            return {"success": False, "error": "ZNE requires a circuit with at least one gate"}
        seen_scales: set[float] = set()
        previous_actual = 0.0
        previous_2q = -1
        for requested in mitigation_scales:
            folded_qc = _fold_circuit(qc, float(requested))
            counts = _gate_counts(folded_qc)
            actual = round(float(counts[2]) / float(base_total), 6)
            if actual in seen_scales or actual <= previous_actual:
                return {
                    "success": False,
                    "error": (
                        "ZNE scale axis is not strictly increasing in total gates: "
                        f"{requested!r} -> {actual:.3f}x (previous {previous_actual:.3f}x)"
                    ),
                }
            if base_n2 > 0:
                if counts[1] <= previous_2q:
                    return {
                        "success": False,
                        "error": (
                            "ZNE scale axis is not strictly increasing in 2Q gates: "
                            f"{requested!r} -> {counts[1]} 2Q gates (previous {previous_2q}); "
                            "choose scales representable on this circuit's 2Q gate count"
                        ),
                    }
                previous_2q = counts[1]
            seen_scales.add(actual)
            previous_actual = actual
            scale_map.append((float(requested), actual, folded_qc, counts))
        if base_n2 > 0:
            for requested, actual, _fqc, counts in scale_map:
                actual_2q_ratio = float(counts[1]) / float(base_n2)
                if actual > 0 and abs(actual_2q_ratio - actual) / actual > 0.20:
                    return {
                        "success": False,
                        "error": (
                            "ZNE scale axis diverges between 2Q and total gate ratios "
                            f"at {requested!r}: 2Q={actual_2q_ratio:.3f}x vs total={actual:.3f}x (>20%)"
                        ),
                    }
        for term in sorted(coeffs):
            for _, actual, folded_qc, _ in scale_map:
                qasms.append(_measurement_qasm(folded_qc, term))
                fold_keys.append((term, actual))
    else:
        for term in sorted(coeffs):
            qasms.append(_measurement_qasm(qc, term))
    try:
        batch = _hardware_batch(qasms, int(shots), int(seed), backend=backend)
    except Exception as e:  # noqa: BLE001 — provider refusal is data
        return {"success": False, "error": f"hardware batch refused: {e}"}
    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != len(qasms):
        return {"success": False, "error": "hardware batch returned no per-term counts"}

    below_fci = False
    cal_data = batch.get("calibration") if isinstance(batch.get("calibration"), dict) else None
    used_q = _layout_physical_qubits(batch.get("layout"))
    readout_meta = None
    if method:
        xs = [actual for _, actual, _, _ in scale_map]
        # G1 fail-closed guard: if transpilation collapsed folded 2Q gates back to
        # scale-1, refuse rather than reporting a spurious extrapolation.
        t_env = batch.get("transpile") if isinstance(batch.get("transpile"), dict) else None
        if (
            t_env is not None
            and "min_transpiled_two_qubit_gates" in t_env
            and "max_transpiled_two_qubit_gates" in t_env
            and int(t_env["min_transpiled_two_qubit_gates"]) > 0
            and max(xs) > 1.0
            and int(t_env["max_transpiled_two_qubit_gates"]) <= int(t_env["min_transpiled_two_qubit_gates"])
        ):
            return {
                "success": False,
                "error": (
                    "ZNE fold scaling collapsed during transpilation: "
                    f"min_2Q={t_env['min_transpiled_two_qubit_gates']} == max_2Q={t_env['max_transpiled_two_qubit_gates']}"
                ),
            }
        by_scale: dict[float, list[tuple[str, dict[str, int]]]] = {s: [] for s in xs}
        for (term, scale), counts in zip(fold_keys, counts_list):
            by_scale[scale].append((term, counts))
        scale_energies: list[float] = []
        scale_uncertainties: list[float] = []
        for s in xs:
            ordered = [dict(counts) for term, counts in by_scale[s]]
            m = _hardware_counts_to_energy(
                ordered, coeffs,
                calibration=cal_data, used_qubits=used_q, readout_mitigation=bool(readout_mitigation),
            )
            if readout_meta is None and "readout_mitigation" in m:
                readout_meta = m["readout_mitigation"]
            scale_energies.append(float(m["energy"]))
            scale_uncertainties.append(float(m["uncertainty"]))
        try:
            id_key = "I" * int(qc.num_qubits)
            asymptote = float(coeffs[id_key]) if id_key in coeffs else 0.0
            ext = _extrapolate_zne(xs, scale_energies, scale_uncertainties, extrapolator=extrapolator, asymptote=asymptote)
        except ValueError as exc:
            return {"success": False, "error": str(exc)}
        mitigated = float(ext["mitigated"])
        uncertainty = float(ext["uncertainty"])
        unmitigated = float(scale_energies[0])
        measured = {"energy": mitigated, "uncertainty": uncertainty, "terms": {}}
        counts_scale = int(xs[0]) if abs(xs[0] - round(xs[0])) < 1e-9 else xs[0]
        counts_by_term = {
            str(term): {str(k): int(v) for k, v in counts.items()}
            for term, counts in by_scale[xs[0]]
        }
        counts_by_scale = {
            str(scale): {
                str(term): {str(k): int(v) for k, v in counts.items()}
                for term, counts in by_scale[scale]
            }
            for scale in xs
        }
        # R2 Phase 2: per-term ZNE diagnostic. Non-identity Pauli terms decay
        # to zero under a unital Pauli channel; the identity term is constant.
        per_term = []
        per_term_total = 0.0
        per_term_var = 0.0
        for term in sorted(coeffs):
            coefficient = float(coeffs[term])
            if set(term) == {"I"}:
                per_term.append({"term": term, "coefficient": coefficient, "mitigated": coefficient,
                                 "uncertainty": 0.0, "method": "constant_identity"})
                per_term_total += coefficient
                continue
            ys_term, se_term = [], []
            for scale in xs:
                counts = counts_by_scale[str(scale)].get(term) or {}
                if not counts:
                    ys_term = []
                    break
                mterm = _hardware_counts_to_energy(
                    [counts], {term: coefficient}, calibration=cal_data,
                    used_qubits=used_q, readout_mitigation=bool(readout_mitigation),
                )
                ys_term.append(float(mterm["energy"]))
                se_term.append(float(mterm["uncertainty"]))
            if len(ys_term) != len(xs):
                per_term.append({"term": term, "coefficient": coefficient, "error": "missing_term_counts"})
                continue
            try:
                eterm = _extrapolate_zne(xs, ys_term, se_term, extrapolator=extrapolator, asymptote=0.0)
                val = float(eterm["mitigated"])
                unc = float(eterm["uncertainty"])
                per_term.append({"term": term, "coefficient": coefficient, "mitigated": val,
                                 "uncertainty": unc, "method": eterm["extrapolator_used"],
                                 "chi2_reduced": eterm["chi2_reduced"]})
                per_term_total += val
                per_term_var += unc ** 2
            except Exception:
                per_term.append({"term": term, "coefficient": coefficient, "error": "fit_failed"})
        per_term_unc = float(math.sqrt(max(per_term_var, 0.0)))
        # R2: measured total-gate/error axis, model adequacy, and a difference CI.
        e1 = e2 = None
        ro = 0.0
        if isinstance(cal_data, dict):
            pq = cal_data.get("per_qubit") if isinstance(cal_data.get("per_qubit"), dict) else {}
            sx = []
            for q in (used_q or []):
                entry = pq.get(str(q)) if isinstance(pq, dict) else None
                if isinstance(entry, dict) and entry.get("sx_error") is not None:
                    sx.append(float(entry["sx_error"]))
                if isinstance(entry, dict) and entry.get("readout_error") is not None:
                    ro += float(entry["readout_error"])
            if sx:
                e1 = sum(sx) / len(sx)
            e2 = cal_data.get("median_two_qubit_error", cal_data.get("two_qubit_error"))
        e1 = float(e1) if e1 is not None and math.isfinite(float(e1)) else 3.0e-4
        e2 = float(e2) if e2 is not None and math.isfinite(float(e2)) else 3.0e-3
        base_n1, base_n2 = scale_map[0][3][0], scale_map[0][3][1]
        base_noise = float(base_n1) * e1 + float(base_n2) * e2 + ro
        fold_axis = []
        weighted_prev = -1.0
        axis_valid = True
        for requested, actual, _fqc, (n1, n2, total) in scale_map:
            weighted = ((float(n1) * e1 + float(n2) * e2 + ro) / base_noise) if base_noise > 0 else actual
            if weighted <= weighted_prev:
                axis_valid = False
            weighted_prev = weighted
            fold_axis.append({
                "requested": float(requested), "actual_total_ratio": float(actual),
                "logical_1q": int(n1), "logical_2q": int(n2), "total_gates": int(total),
                "weighted_error_scale": round(float(weighted), 6),
            })
        baseline_energy = float(scale_energies[0])
        baseline_unc = float(scale_uncertainties[0])
        ordered_scale0 = [dict(counts) for _term, counts in by_scale[xs[0]]]
        raw_unmitigated_m = _hardware_counts_to_energy(
            ordered_scale0, coeffs,
            calibration=cal_data, used_qubits=used_q, readout_mitigation=False,
        )
        raw_unmitigated_energy = float(raw_unmitigated_m["energy"])
        raw_unmitigated_unc = float(raw_unmitigated_m["uncertainty"])
        difference = mitigated - baseline_energy
        difference_ci = 1.959963984540054 * math.sqrt(uncertainty ** 2 + baseline_unc ** 2)
        difference_vs_raw = mitigated - raw_unmitigated_energy
        difference_vs_raw_ci = 1.959963984540054 * math.sqrt(uncertainty ** 2 + raw_unmitigated_unc ** 2)
        model_points = {}
        for alt in ("linear", "quadratic"):
            try:
                model_points[alt] = float(_extrapolate_zne(
                    xs, scale_energies, scale_uncertainties, extrapolator=alt,
                    asymptote=(float(coeffs["I" * int(qc.num_qubits)]) if ("I" * int(qc.num_qubits)) in coeffs else 0.0),
                )["mitigated"])
            except Exception:
                continue
        model_spread = (max(model_points.values()) - min(model_points.values())) if len(model_points) >= 2 else None
        chi2 = float(ext.get("chi2_reduced") or 0.0)
        model_adequate = bool(axis_valid and chi2 <= 1.5 and (model_spread is None or model_spread <= max(2.0 * uncertainty, 1e-6)))
        lack_of_fit_p = None
        try:
            from scipy.stats import chi2 as _chi2_dist
            dof = int(ext.get("dof") or 0)
            if dof <= 0:
                dof = max(1, len(xs) - (2 if ext.get("exp_params", {}).get("asymptote_fixed") else 3))
            lack_of_fit_p = float(_chi2_dist.sf(chi2 * dof, dof))
        except Exception:
            lack_of_fit_p = None
        energy = mitigated + float(constant_energy)
        delta = abs(energy - float(fci_exact_energy))
        # A ZNE value is an extrapolation, not a direct measurement. It must
        # never become claimable merely because the extrapolated point lands
        # near the anchor; the result keeps measured=false and claimable=false.
        claimable = False
        mitigation_receipt = {
            "method": "ZNE",
            "extrapolator": ext["extrapolator"],
            "extrapolator_used": ext["extrapolator_used"],
            "fit_degree": ext["fit_degree"],
            "scales": [int(s) if abs(s - round(s)) < 1e-9 else s for s in xs],
            "requested_scales": [req for req, _, _, _ in scale_map],
            "scale_energies": scale_energies,
            "scale_uncertainties": scale_uncertainties,
            "fit_condition_number": ext["fit_condition_number"],
            "fit_rmse": ext["fit_rmse"],
            "chi2_reduced": ext["chi2_reduced"],
            "axis_valid": axis_valid,
            "fold_axis": fold_axis,
            "model_points": model_points,
            "model_spread_ha": model_spread,
            "model_adequate": model_adequate,
            "lack_of_fit_p": lack_of_fit_p,
            "per_term": per_term,
            "per_term_energy": per_term_total,
            "per_term_uncertainty": per_term_unc,
            "baseline_energy": baseline_energy,
            "baseline_uncertainty": baseline_unc,
            "raw_unmitigated_energy": raw_unmitigated_energy,
            "raw_unmitigated_uncertainty": raw_unmitigated_unc,
            "readout_mitigated_baseline_energy": baseline_energy,
            "readout_mitigated_baseline_uncertainty": baseline_unc,
            "difference_ha": difference,
            "difference_ci95_ha": difference_ci,
            "difference_ci95_mha": difference_ci * 1000.0,
            "difference_significant": abs(difference) > difference_ci,
            "difference_vs_baseline_ha": difference,
            "difference_vs_baseline_ci95_ha": difference_ci,
            "difference_vs_baseline_significant": abs(difference) > difference_ci,
            "difference_vs_raw_ha": difference_vs_raw,
            "difference_vs_raw_ci95_ha": difference_vs_raw_ci,
            "difference_vs_raw_ci95_mha": difference_vs_raw_ci * 1000.0,
            "difference_vs_raw_significant": abs(difference_vs_raw) > difference_vs_raw_ci,
            "uncertainty_method": ext["uncertainty_method"],
            "intercept_weights": ext["intercept_weights"],
            "unmitigated_energy": unmitigated,
            "mitigated_energy": mitigated,
            "mitigated_measured": False,
            "measured": False,
        }
        if "exp_params" in ext:
            mitigation_receipt["exp_params"] = ext["exp_params"]
        if readout_meta is not None:
            mitigation_receipt["readout_mitigation"] = readout_meta
    else:
        measured = _hardware_counts_to_energy(
            counts_list, coeffs,
            calibration=cal_data, used_qubits=used_q, readout_mitigation=bool(readout_mitigation),
        )
        readout_meta = measured.get("readout_mitigation")
        counts_scale = 1
        counts_by_term = {
            str(term): {str(k): int(v) for k, v in counts.items()}
            for term, counts in zip(sorted(coeffs), counts_list)
        }
        energy = measured["energy"] + float(constant_energy)
        delta = abs(energy - float(fci_exact_energy))
        claimable = _claimable_against_anchor(energy, float(fci_exact_energy), measured["uncertainty"])
        below_fci = bool(energy < float(fci_exact_energy) - 3.0 * measured["uncertainty"])
        mitigation_receipt = None
    calibration_quality = _calibration_quality(batch.get("calibration"))
    if calibration_quality["calibration_fresh"] is False:
        claimable = False
    return {
        "success": True,
        "kind": "vqe_hardware",
        "result_version": "experiment-result-v1",
        "molecule": str(molecule or "H2"),
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": (f"VQE {molecule} active-space UCCSD, local COBYLA optimizer, hardware final state"
                   if ansatz_qasm and str(molecule or "H2").upper() != "H2" else "VQE H2 hardware-efficient, local COBYLA optimizer, hardware final state"),
        "ansatz": (f"active-space UCCSD ({qc.num_qubits} qubits)"
                   if ansatz_qasm and str(molecule or "H2").upper() != "H2" else f"hardware-efficient layers={int(layers)}"),
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "theta": [float(v) for v in theta_arr],
        "constant_energy": float(constant_energy),
        "energy": float(energy),
        "energy_measured": float(energy),
        "energy_measured_source": ("ZNE-extrapolated from QPU term measurements"
                                   if mitigation_receipt else "QPU measurement-basis batch"),
        "measured": not bool(mitigation_receipt),
        "mitigated": bool(mitigation_receipt),
        "mitigation": mitigation_receipt,
        "readout_mitigation": readout_meta,
        "error_suppression": batch.get("error_suppression"),
        "uncertainty": float(measured["uncertainty"]),
        "ci95_mha": _ci95_mha(measured["uncertainty"]),
        "measurements": measured["terms"],
        "counts_by_term": counts_by_term,
        "counts_by_scale": counts_by_scale if mitigation_receipt else None,
        "counts_scale": counts_scale,
        "fold_axis": fold_axis if mitigation_receipt else None,
        "axis_valid": axis_valid if mitigation_receipt else None,
        "model_adequate": model_adequate if mitigation_receipt else None,
        "per_term_energy": per_term_total if mitigation_receipt else None,
        "per_term_uncertainty": per_term_unc if mitigation_receipt else None,
        "fci_exact_energy": float(fci_exact_energy),
        "delta_mha": float(delta * 1000.0),
        "chemical_accuracy": claimable,
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": claimable,
        "below_fci": below_fci,
        "simulated_optimizer": True,
        "hardware": True,
        "backend": str(batch.get("backend") or "ibm-qiskit"),
        "provider": "ibm-qiskit",
        "hardware_job_id": str(batch.get("hardware_job_id") or batch.get("job_id") or ""),
        "layout": batch.get("layout"),
        "layout_quality": batch.get("layout_quality"),
        "transpile": batch.get("transpile"),
        "calibration": batch.get("calibration"),
        **calibration_quality,
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def _run_vqe_hardware_cdr(
    *,
    qc: Any,
    theta_arr: np.ndarray,
    coeffs: dict[str, float],
    layers: int,
    shots: int,
    seed: int,
    bond_distance: float,
    fci_exact_energy: float,
    backend: str | None,
    molecule: str,
    ansatz_qasm: str | None,
    constant_energy: float,
    readout_mitigation: bool,
    cdr_training_circuits: int,
    live_readout_cal: bool,
    symmetry_verification: bool,
    target_shots: list[int] | None,
) -> dict[str, Any]:
    """Execute same-batch QWC + Live 4x4 Readout + Z2 Symmetry + ODR CDR on hardware.

    All target QWC circuits, near-Clifford training circuits, and computational-basis
    readout calibration circuits are submitted in ONE governed ``_hardware_batch``
    call with per-PUB Neyman shot allocation so every mitigation layer shares the
    same calibration epoch and device drift snapshot.
    """
    from qiskit import qasm2

    from kubona.mitigation.cdr import (
        allocate_cdr_training_shots,
        build_readout_cal_circuits,
        evaluate_cdr_batch,
        generate_cdr_training_circuits,
    )
    from kubona.mitigation.qwc import (
        allocate_neyman_shots,
        estimate_circuit_qwc_variances,
        group_pauli_terms,
        measurement_circuit_for_basis,
    )
    from kubona.mitigation.symmetry import detect_z_symmetries

    if cdr_training_circuits < 6 or cdr_training_circuits > 32:
        return {"success": False, "error": f"cdr_training_circuits {cdr_training_circuits} must be in [6, 32]"}
    if int(qc.num_clbits) > 0 or any(
        getattr(inst.operation, "name", "") == "measure" for inst in qc.data
    ):
        return {"success": False, "error": "CDR requires a unitary ansatz without measurement"}

    try:
        grouped = group_pauli_terms(coeffs)
    except ValueError as exc:
        return {"success": False, "error": f"QWC grouping failed: {exc}"}
    groups = grouped["groups"]
    if not groups:
        return {"success": False, "error": "CDR requires at least one non-identity Pauli term"}

    syms = detect_z_symmetries(coeffs, exact_circuit=qc) if symmetry_verification else []
    try:
        train_pkg = generate_cdr_training_circuits(
            qc, coeffs,
            n_circuits=int(cdr_training_circuits),
            seed=int(seed),
            symmetries=(syms if syms else None),
            min_distinct_energies=min(8, int(cdr_training_circuits)),
        )
    except ValueError as exc:
        return {"success": False, "error": f"CDR training generation failed: {exc}"}

    use_live_ro = bool(live_readout_cal and int(qc.num_qubits) <= 4)
    cal_pairs = build_readout_cal_circuits(int(qc.num_qubits)) if use_live_ro else []

    # Resolve per-PUB shots: [G target] + [M * G training] + [2^n readout cal]
    g_count = len(groups)
    sym_active = bool(syms)
    shot_quantum = 2048 if (int(shots) >= 4096 and int(shots) % 2048 == 0) else 1
    if target_shots:
        ts = [int(s) for s in target_shots]
        if len(ts) == g_count:
            resolved_target_shots = ts
        elif len(ts) == 2 and g_count >= 2:
            resolved_target_shots = [ts[0] if g.get("is_z_basis") else ts[1] for g in groups]
        elif len(ts) == 1:
            tgt_vars = estimate_circuit_qwc_variances(qc, grouped, symmetry_protected_z=sym_active)
            resolved_target_shots = allocate_neyman_shots(
                groups, ts[0] * g_count, group_variances=tgt_vars,
                min_shots=min(2048 if shot_quantum == 2048 else 1024, ts[0]),
                quantum=shot_quantum,
            )
        else:
            return {
                "success": False,
                "error": f"target_shots length {len(ts)} incompatible with {g_count} QWC groups",
            }
    else:
        resolved_target_shots = [int(shots)] * g_count

    # C-optimal inverse-variance + exact statevector Neyman shot allocation across
    # the M training circuits at the same total training shot budget (M * G * shots).
    # Concentrates shots on low-variance near-ground anchors to minimize fit_se.
    m_train = len(train_pkg["circuits"])
    train_shots_matrix = allocate_cdr_training_shots(
        train_pkg["circuits"],
        train_pkg["exact_energies"],
        train_pkg["target_exact_energy"],
        grouped,
        int(shots) * g_count * m_train,
        symmetry_protected_z=sym_active,
        min_group_shots=min(2048 if shot_quantum == 2048 else 1024, int(shots)),
        quantum=shot_quantum,
    )
    # In same-batch CDR, shared readout-calibration noise is common-mode and absorbed
    # by the affine fit; 4096 shots/state keeps cond(M) well-conditioned at low RU cost.
    cal_shots_each = int(shots)

    qasms: list[str] = []
    pub_shots: list[int] = []

    for grp, s_cnt in zip(groups, resolved_target_shots):
        qasms.append(qasm2.dumps(measurement_circuit_for_basis(qc, grp["basis"])))
        pub_shots.append(int(s_cnt))

    for tqc, t_shots_row in zip(train_pkg["circuits"], train_shots_matrix):
        for grp, s_cnt in zip(groups, t_shots_row):
            qasms.append(qasm2.dumps(measurement_circuit_for_basis(tqc, grp["basis"])))
            pub_shots.append(int(s_cnt))

    for _bs, cal_qc in cal_pairs:
        qasms.append(qasm2.dumps(cal_qc))
        pub_shots.append(int(cal_shots_each))

    try:
        if len(set(pub_shots)) > 1:
            batch = _hardware_batch(qasms, int(shots), int(seed), backend=backend, shots_per_circuit=pub_shots)
        else:
            batch = _hardware_batch(qasms, int(pub_shots[0]), int(seed), backend=backend)
    except Exception as e:  # noqa: BLE001 — provider refusal is data
        return {"success": False, "error": f"hardware batch refused: {e}"}

    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != len(qasms):
        return {"success": False, "error": "hardware batch returned mismatched CDR circuit counts"}

    cursor = 0
    target_group_counts = [
        {str(k): int(v) for k, v in c.items()}
        for c in counts_list[cursor : cursor + g_count]
    ]
    cursor += g_count

    m_train = len(train_pkg["circuits"])
    train_group_counts: list[list[dict[str, int]]] = []
    cdr_training_counts_archive: dict[str, dict[str, dict[str, int]]] = {}
    for ti in range(m_train):
        row_counts = [
            {str(k): int(v) for k, v in c.items()}
            for c in counts_list[cursor : cursor + g_count]
        ]
        cursor += g_count
        train_group_counts.append(row_counts)
        cdr_training_counts_archive[f"train_{ti:02d}"] = {
            grp["basis"]: cnts for grp, cnts in zip(groups, row_counts)
        }

    readout_cal_counts_list: list[dict[str, int]] | None = None
    readout_cal_counts_archive: dict[str, dict[str, int]] | None = None
    if cal_pairs:
        readout_cal_counts_list = [
            {str(k): int(v) for k, v in c.items()}
            for c in counts_list[cursor : cursor + len(cal_pairs)]
        ]
        readout_cal_counts_archive = {
            bs: cnts for (bs, _), cnts in zip(cal_pairs, readout_cal_counts_list)
        }

    cal_data = batch.get("calibration") if isinstance(batch.get("calibration"), dict) else None
    used_q = _layout_physical_qubits(batch.get("layout"))
    diag_factors: dict[str, float] | None = None
    if not use_live_ro and readout_mitigation and cal_data is not None:
        diag_factors = {}
        for term in coeffs:
            if set(term) <= {"I"}:
                continue
            f_val = _readout_mitigation_factor(term, cal_data, used_q)
            if f_val is not None:
                diag_factors[term] = float(f_val)

    ev = evaluate_cdr_batch(
        grouped,
        coeffs,
        target_group_counts,
        train_group_counts,
        train_pkg["exact_energies"],
        constant_energy=float(constant_energy),
        fci_exact_energy=float(fci_exact_energy),
        readout_cal_counts=readout_cal_counts_list,
        diagonal_readout_factors=diag_factors,
        symmetries=(syms if syms else None),
        train_sector_aligned=bool(train_pkg["sector_aligned"]),
    )
    if not ev.get("ok"):
        return {"success": False, "error": str(ev.get("error") or "CDR evaluation failed")}

    cdr_fit = ev["cdr"]
    ablation = ev["ablation_layers"]
    energy = float(cdr_fit["mitigated"]) + float(constant_energy)
    uncertainty = float(cdr_fit["uncertainty"])
    delta = abs(energy - float(fci_exact_energy))

    # Map QWC target counts back to per-term counts_by_term for supplement compatibility.
    counts_by_term: dict[str, dict[str, int]] = {}
    counts_by_qwc_group: dict[str, dict[str, int]] = {}
    for grp, cnts in zip(groups, target_group_counts):
        counts_by_qwc_group[grp["basis"]] = cnts
        for term in grp["terms"]:
            counts_by_term[str(term)] = cnts

    mitigation_receipt = {
        **cdr_fit,
        "distinct_energies": int(train_pkg["distinct_energies"]),
        "target_strictly_bracketed": bool(train_pkg["target_strictly_bracketed"]),
        "extrapolation_distance_ha": float(train_pkg["extrapolation_distance_ha"]),
        "sector_aligned": bool(train_pkg["sector_aligned"]),
        "cdr_input_layer": ev["cdr_input_layer"],
        "readout_calibration": ev["readout_calibration"],
        "symmetry": ev["symmetry_verified"],
        "ablation_layers": ablation,
        "qwc_groups": [
            {"basis": g["basis"], "terms": sorted(g["terms"].keys()), "target_shots": s}
            for g, s in zip(groups, resolved_target_shots)
        ],
        "shots_per_circuit": pub_shots,
        "total_shots": int(sum(pub_shots)),
        "training_metadata": train_pkg["metadata"],
        "train_noisy_energies": ev["train_noisy_energies"],
        "train_noisy_uncertainties": ev["train_noisy_uncertainties"],
        "train_exact_energies": ev["train_exact_energies"],
        "raw_unmitigated_energy": float(ev["raw"]["energy"]),
        "raw_unmitigated_uncertainty": float(ev["raw"]["uncertainty"]),
        "unmitigated_energy": float(ev["raw"]["energy"]),
        "mitigated_energy": float(cdr_fit["mitigated"]),
        "mitigated_measured": False,
        "measured": False,
    }
    calibration_quality = _calibration_quality(batch.get("calibration"))
    # CDR is a statistical regression estimator, not a direct raw measurement:
    # keep measured=False, claimable=False, chemical_accuracy=False (G6 gates audit it).
    return {
        "success": True,
        "kind": "vqe_hardware",
        "result_version": "experiment-result-v1",
        "molecule": str(molecule or "H2"),
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": (f"VQE {molecule} active-space UCCSD, local COBYLA optimizer, hardware final state"
                   if ansatz_qasm and str(molecule or "H2").upper() != "H2" else "VQE H2 hardware-efficient, local COBYLA optimizer, hardware final state"),
        "ansatz": (f"active-space UCCSD ({qc.num_qubits} qubits)"
                   if ansatz_qasm and str(molecule or "H2").upper() != "H2" else f"hardware-efficient layers={int(layers)}"),
        "layers": int(layers),
        "shots": int(shots),
        "target_shots": resolved_target_shots,
        "total_shots": int(sum(pub_shots)),
        "seed": int(seed),
        "theta": [float(v) for v in theta_arr],
        "constant_energy": float(constant_energy),
        "energy": float(energy),
        "energy_measured": float(energy),
        "energy_measured_source": "CDR same-batch regression from QWC QPU measurements",
        "measured": False,
        "mitigated": True,
        "mitigation": mitigation_receipt,
        "ablation_layers": ablation,
        "symmetry": ev["symmetry_verified"],
        "qwc_groups": mitigation_receipt["qwc_groups"],
        "readout_mitigation": ev["readout_calibration"],
        "error_suppression": batch.get("error_suppression"),
        "uncertainty": float(uncertainty),
        "ci95_mha": _ci95_mha(uncertainty),
        "measurements": ev["readout_mitigated"]["terms"],
        "counts_by_term": counts_by_term,
        "counts_by_qwc_group": counts_by_qwc_group,
        "cdr_training_counts": cdr_training_counts_archive,
        "readout_cal_counts": readout_cal_counts_archive,
        "counts_by_scale": None,
        "counts_scale": 1,
        "fold_axis": None,
        "axis_valid": True,
        "model_adequate": bool(cdr_fit["model_adequate"]),
        "per_term_energy": None,
        "per_term_uncertainty": None,
        "fci_exact_energy": float(fci_exact_energy),
        "delta_mha": float(delta * 1000.0),
        "chemical_accuracy": False,
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": False,
        "below_fci": bool(energy < float(fci_exact_energy) - 3.0 * uncertainty),
        "simulated_optimizer": True,
        "hardware": True,
        "backend": str(batch.get("backend") or "ibm-qiskit"),
        "provider": "ibm-qiskit",
        "hardware_job_id": str(batch.get("hardware_job_id") or batch.get("job_id") or ""),
        "layout": batch.get("layout"),
        "layout_quality": batch.get("layout_quality"),
        "transpile": batch.get("transpile"),
        "calibration": batch.get("calibration"),
        **calibration_quality,
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def _hardware_energy_once(qc: Any, coeffs: dict[str, float], shots: int,
                          seed: int, backend: str | None,
                          constant_energy: float = 0.0) -> dict[str, Any]:
    qasms = [_measurement_qasm(qc, term) for term in sorted(coeffs)]
    batch = _hardware_batch(qasms, int(shots), int(seed), backend=backend)
    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != len(qasms):
        raise RuntimeError("hardware batch returned no per-term counts")
    m = _hardware_counts_to_energy(counts_list, coeffs)
    return {
        "energy": float(m["energy"] + constant_energy),
        "uncertainty": float(m["uncertainty"]),
        "terms": m["terms"],
        "batch": batch,
    }


def run_vqe_hardware_loop(
    theta: list[float] | None = None,
    pauli_coefficients: dict[str, float] | None = None,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    bond_distance: float = 0.7414,
    fci_exact_energy: float | None = None,
    backend: str | None = None,
    *,
    molecule: str = "H2",
    ansatz_qpy: str | None = None,
    constant_energy: float = 0.0,
    max_iterations: int = 8,
    optimizer: str = "spsa",
) -> dict[str, Any]:
    """Hardware-in-the-loop VQE: every objective evaluation is a QPU batch.

    H2 uses the local hardware-efficient ansatz builder; LiH/BeH2 use the QPY
    serialization of the unbound active-space UCCSD ansatz.  There is no local
    statevector objective and no ``simulated_optimizer`` label: COBYLA sees
    only shot-measured energies.  The parent Go job still owns the mandate,
    reservation, settlement, and claim.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing hardware VQE loop"}
    if int(max_iterations) < 1 or int(max_iterations) > 30:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,30]"}
    coeffs = dict(pauli_coefficients or {})
    if not coeffs:
        return {"success": False, "error": "hardware VQE loop requires pauli_coefficients"}
    if fci_exact_energy is None:
        _, fci_exact_energy, _ = h2_hamiltonian(float(bond_distance))
    mol = str(molecule or "H2")
    template = None
    if ansatz_qpy:
        try:
            from qiskit import qpy
            template = qpy.load(io.BytesIO(base64.b64decode(ansatz_qpy)))[0]
        except Exception as e:  # noqa: BLE001
            return {"success": False, "error": f"ansatz QPY is not executable: {e}"}
        if getattr(template, "num_clbits", 0) > 0 or any(
            getattr(inst.operation, "name", "") == "measure" for inst in template.data
        ):
            return {"success": False, "error": "hardware loop requires a unitary ansatz without measurement"}
        n_params = int(template.num_parameters)
        x0 = np.asarray(theta if theta is not None else np.zeros(n_params), dtype=float)
        if x0.size != n_params:
            return {"success": False, "error": f"theta has {x0.size} values; ansatz needs {n_params}"}
    elif mol.upper() == "H2":
        if theta is None or layers < 1 or layers > 8:
            return {"success": False, "error": "H2 hardware loop requires theta and layers in [1,8]"}
        x0 = np.asarray(theta, dtype=float)
        if x0.size != 4 * int(layers):
            return {"success": False, "error": f"theta has {x0.size} values; need {4 * int(layers)}"}
        n_params = x0.size
    else:
        return {"success": False, "error": "active-space hardware loop requires ansatz_qpy"}

    trajectory: list[dict[str, Any]] = []
    hardware_job_ids: list[str] = []
    usage_values: list[float] = []
    actual_values: list[float] = []
    last_layout: Any = None
    last_calibration: Any = None
    opt_name = str(optimizer or "spsa").strip().lower()
    if opt_name not in ("spsa", "cobyla"):
        return {"success": False, "error": f"hardware loop optimizer {optimizer!r} must be spsa|cobyla"}

    def objective(x: np.ndarray) -> float:
        nonlocal last_layout, last_calibration
        try:
            if template is None:
                qc = _ansatz(np.asarray(x, dtype=float), int(layers))
            else:
                qc = template.assign_parameters(np.asarray(x, dtype=float))
            m = _hardware_energy_once(qc, coeffs, int(shots), int(seed), backend, float(constant_energy))
        except Exception as e:  # noqa: BLE001
            raise RuntimeError(f"hardware evaluation failed: {e}") from e
        batch = m["batch"]
        last_layout = batch.get("layout")
        last_calibration = batch.get("calibration")
        job_id = str(batch.get("hardware_job_id") or batch.get("job_id") or "")
        if job_id:
            hardware_job_ids.append(job_id)
        if batch.get("usage_seconds") is not None:
            usage_values.append(float(batch["usage_seconds"]))
        if batch.get("actual_usd") is not None:
            actual_values.append(float(batch["actual_usd"]))
        trajectory.append({
            "iteration": len(trajectory) + 1,
            "energy": float(m["energy"]),
            "uncertainty": float(m["uncertainty"]),
            "theta": [float(v) for v in x],
            "terms": m["terms"],
        })
        return float(m["energy"])

    opt_success = False
    opt_evals = 0
    opt_message = ""
    try:
        if opt_name == "spsa":
            # SPSA is deliberately used on hardware: finite-difference
            # COBYLA stalls when every objective is a noisy 256-shot QPU
            # batch. Two evaluations per iteration estimate a stochastic
            # gradient without pretending the noise is zero.
            rng = np.random.default_rng(int(seed))
            x = np.asarray(x0, dtype=float).copy()
            a, c = 0.20, 0.10
            alpha, gamma = 0.602, 0.101
            for k in range(int(max_iterations)):
                delta = rng.choice(np.array([-1.0, 1.0]), size=x.size)
                ck = c / ((k + 1) ** gamma)
                xp = x + ck * delta
                xm = x - ck * delta
                fp = objective(xp)
                fm = objective(xm)
                opt_evals += 2
                grad = (fp - fm) / (2.0 * ck) * delta
                ak = a / ((k + 1) ** alpha)
                x = x - ak * grad
            final_x = np.asarray(x, dtype=float)
            final = objective(final_x)
            opt_evals += 1
            opt_success = True
            opt_message = "SPSA completed"
        else:
            from scipy.optimize import minimize
            opt = minimize(objective, x0, method="COBYLA",
                           options={"maxiter": int(max_iterations), "rhobeg": 0.15, "tol": 1e-6})
            final_x = np.asarray(opt.x if opt.x is not None else x0, dtype=float)
            final = objective(final_x)
            opt_success = bool(opt.success)
            opt_evals = int(opt.nfev)
            opt_message = str(opt.message)
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"hardware VQE loop refused: {e}"}
    delta = abs(float(final) - float(fci_exact_energy))
    # The final uncertainty is the last shot-measured evaluation's standard
    # error; the fit/optimizer trajectory is not a substitute for it.
    uncertainty = float(trajectory[-1]["uncertainty"]) if trajectory else 0.0
    calibration_quality = _calibration_quality(last_calibration)
    claimable = _claimable_against_anchor(float(final), float(fci_exact_energy), uncertainty)
    if calibration_quality["calibration_fresh"] is False:
        claimable = False
    below_fci = bool(float(final) < float(fci_exact_energy) - 3.0 * uncertainty)
    return {
        "success": True,
        "kind": "vqe_hardware_loop",
        "result_version": "experiment-result-v1",
        "molecule": mol,
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": "VQE hardware-in-the-loop, COBYLA",
        "ansatz": (f"active-space UCCSD ({template.num_qubits} qubits)" if template is not None
                   else f"hardware-efficient layers={int(layers)}"),
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "theta": [float(v) for v in final_x],
        "constant_energy": float(constant_energy),
        "energy": float(final),
        "energy_measured": float(final),
        "energy_measured_source": "QPU measurement-basis batches (hardware-in-the-loop)",
        "uncertainty": uncertainty,
        "ci95_mha": _ci95_mha(uncertainty),
        "measurements": trajectory[-1].get("terms", {}) if trajectory else {},
        "fci_exact_energy": float(fci_exact_energy),
        "delta_mha": float(delta * 1000.0),
        "chemical_accuracy": claimable,
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": claimable,
        "below_fci": below_fci,
        "hardware": True,
        "measured": True,
        "simulated_optimizer": False,
        "optimizer_backend": "ibm-qiskit",
        "measurement_backend": "ibm-qiskit",
        "optimizer": {"name": opt_name.upper(), "success": opt_success,
                      "evaluations": opt_evals, "message": opt_message,
                      "max_iterations": int(max_iterations)},
        "trajectory": trajectory,
        "iteration_batches": len(hardware_job_ids),
        "hardware_job_ids": hardware_job_ids,
        "hardware_job_id": hardware_job_ids[-1] if hardware_job_ids else "",
        "layout": last_layout,
        "calibration": last_calibration,
        **calibration_quality,
        "usage_seconds": sum(usage_values) if usage_values else None,
        "actual_usd": sum(actual_values) if len(actual_values) == len(hardware_job_ids) and actual_values else None,
        "backend": str(trajectory[-1].get("backend", "")) if trajectory and "backend" in trajectory[-1] else "ibm-qiskit",
        "provider": "ibm-qiskit",
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def run_vqe(
    molecule: str = "H2",
    bond_distance: float = 0.7414,
    layers: int = 2,
    shots: int = 16384,
    seed: int = 7,
    max_iterations: int = 80,
    full_space: bool = False,
    full_space_pool: str = "ceo",
    *,
    atom: str | None = None,
    basis: str = "sto-3g",
    charge: int = 0,
    spin: int = 0,
    ncas: int | None = None,
    nelecas: int | None = None,
) -> dict[str, Any]:
    """Run a real H2 VQE with a live anchor and shot-measured final energy."""
    t0 = time.perf_counter()
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing VQE"}
    if atom is not None:
        return run_generic_vqe(
            atom=str(atom), basis=str(basis), charge=int(charge), spin=int(spin),
            ncas=ncas, nelecas=nelecas, layers=int(layers), shots=int(shots), seed=int(seed),
            max_iterations=int(max_iterations),
        )
    mol = (molecule or "").upper()
    if mol in _NATURE_ACTIVE:
        return run_nature_vqe(mol, float(bond_distance), shots=shots,
                              seed=seed, max_iterations=max_iterations,
                              full_space=bool(full_space), layers=int(layers),
                              full_space_pool=str(full_space_pool or "ceo"))
    if mol != "H2":
        return {
            "success": False,
            "error": f"VQE supports H2, {sorted(_NATURE_ACTIVE)}; refusing {mol!r}",
        }
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]: refusing"}
    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]: refusing"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]: refusing"}
    if max_iterations < 1 or max_iterations > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]: refusing"}

    from scipy.optimize import minimize

    r = float(bond_distance)
    h4, fci, rhf = h2_hamiltonian(r)
    coeffs = pauli_coefficients(h4)
    rng = np.random.default_rng(seed)
    x0 = rng.uniform(-0.2, 0.2, size=4 * layers)
    trajectory: list[dict[str, Any]] = []
    best = {"energy": float("inf")}
    evals = 0

    def objective(x: np.ndarray) -> float:
        nonlocal evals
        evals += 1
        e = _energy_exact(x, h4, layers)
        if e < best["energy"]:
            best["energy"] = e
            best["theta"] = [float(v) for v in x]
        if len(trajectory) < 500:
            trajectory.append({"iteration": evals, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    opt = minimize(
        objective, x0, method="COBYLA",
        options={"maxiter": int(max_iterations), "rhobeg": 0.35, "tol": 1e-9},
    )
    theta = np.asarray(opt.x if opt.x is not None else best.get("theta", x0), dtype=float)
    exact_energy = _energy_exact(theta, h4, layers)
    measured = _shot_energy(theta, coeffs, layers, int(shots), int(seed))
    energy = measured["energy"]
    delta = abs(energy - fci)
    elapsed_ms = (time.perf_counter() - t0) * 1000.0
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": "H2",
        "bond_distance": r,
        "basis": "sto-3g",
        "method": "VQE hardware-efficient RY/RZ + CX, COBYLA",
        "ansatz": f"hardware-efficient layers={layers}",
        "layers": layers,
        "shots": int(shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": evals, "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "ansatz_qasm": _ansatz_qasm(theta, layers),
        "energy": float(energy),
        "energy_exact": float(exact_energy),
        "energy_measured": float(energy),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "ci95_mha": _ci95_mha(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": float(fci),
        "rhf_mean_field_energy": float(rhf),
        "delta_mha": float(delta * 1000.0),
        "chemical_accuracy": _claimable_against_anchor(energy, fci, measured["uncertainty"]),
        "claimable": _claimable_against_anchor(energy, fci, measured["uncertainty"]),
        "below_fci": bool(energy < fci - 3.0 * measured["uncertainty"]),
        "chemical_accuracy_limit_mha": 1.6,
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "pauli_coefficients": coeffs,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "execution_time_ms": elapsed_ms,
        "error_message": "",
    }


def _run_full_space_vqe(spec: dict[str, Any], atom: str, bond_distance: float,
                         problem: Any, ncas: int, nelecas: int, shots: int,
                         seed: int, max_iterations: int, layers: int,
                         t0: float) -> dict[str, Any]:
    """Full-orbital-space ADAPT-VQE with a complete, explicit operator pool.

    The old path used an EfficientSU2 circuit and was honestly labelled
    contested because the ansatz had no UCCSD/CEO guarantee.  This path uses
    the real adaptive algorithm in ``adapt.py``: every pool gradient is
    evaluated, the selected operators are optimized, and the result is judged
    on a shot-measured Pauli expectation with its standard uncertainty.
    """
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing molecular VQE"}
    try:
        from adapt import AdaptConfig, AdaptError, run_adapt_vqe
    except Exception as e:  # pragma: no cover - image dependency failure
        return {"success": False, "error": f"adaptive VQE unavailable: {e}"}

    e_full, _ = _full_fci_and_casci(atom, ncas, nelecas)
    mapper = JordanWignerMapper()
    qop = mapper.map(problem.hamiltonian.second_q_op())
    constants = float(sum(problem.hamiltonian.constants.values()))
    if int(qop.num_qubits) > 12:
        return {
            "success": False,
            "error": f"full-space VQE needs {qop.num_qubits} qubits; the local CPU cap is 12. Use active_space instead.",
        }

    pool = str(spec.get("full_space_pool") or os.getenv("KUBONA_ADAPT_POOL") or "ceo").strip().lower()
    if pool not in ("uccsd", "ceo"):
        return {"success": False, "error": f"full_space_pool {pool!r} must be uccsd|ceo"}
    # A full-space claim needs enough samples for its uncertainty not to
    # decide the answer.  The caller's shot count is a floor, never a cap:
    # asking for 8k shots still returns a result, but it cannot silently
    # borrow chemical accuracy from a lower-noise run.
    measurement_shots = max(int(shots), int(os.getenv("KUBONA_ADAPT_MIN_SHOTS", "262144")))
    max_ops = max(40, min(64, int(layers) * 8))
    if os.getenv("KUBONA_ADAPT_MAX_OPS"):
        try:
            max_ops = int(os.environ["KUBONA_ADAPT_MAX_OPS"])
        except ValueError:
            return {"success": False, "error": "KUBONA_ADAPT_MAX_OPS must be an integer"}
    cfg = AdaptConfig(
        pool=pool,
        max_ops=max_ops,
        max_iterations=max(20, int(max_iterations)),
        shots=measurement_shots,
        seed=int(seed),
        time_limit_s=float(os.getenv("KUBONA_ADAPT_TIME_LIMIT_S", "900")),
        target_mha=1.6,
    )
    try:
        res = run_adapt_vqe(problem, qop, constants, mapper, float(e_full), cfg)
    except (AdaptError, ValueError, RuntimeError) as e:
        return {"success": False, "error": f"adaptive VQE refused: {e}"}
    exact_delta = abs(res.energy_exact - e_full) * 1000.0
    claimable = (
        _claimable_against_anchor(res.energy_measured, e_full, res.uncertainty)
        and exact_delta <= 1.6
    )
    if claimable:
        status = "within_target"
    elif res.stopped in ("time_limit", "max_ops") and exact_delta > 1.6:
        status = "resource_exhausted"
    elif exact_delta <= 1.6:
        status = "measurement_noise"
    elif res.delta_mha <= 1.6 and res.uncertainty <= 1.6:
        status = "measurement_noise"
    else:
        status = "contested"
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": spec["name"],
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": f"ADAPT-VQE ({pool.upper()} pool), COBYLA",
        "ansatz": f"ADAPT-VQE {pool.upper()} pool, {len(res.selected)} selected operators",
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas,
                         "spatial_orbitals": int(problem.num_spatial_orbitals),
                         "full_space": True},
        "shots": int(measurement_shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": int(res.evaluations),
                      "success": res.stopped in ("within_target", "gradient_converged"),
                      "message": res.stopped},
        "theta": res.theta,
        "selected_operators": res.selected,
        "pool_size": int(res.pool_size),
        "adapt": {"stopped": res.stopped, "elapsed_s": res.elapsed_s,
                  "gradient_tol": cfg.gradient_tol, "time_limit_s": cfg.time_limit_s},
        "energy": float(res.energy_measured),
        "energy_exact": float(res.energy_exact),
        "energy_measured": float(res.energy_measured),
        "energy_measured_source": "shot-sampled Pauli expectation (qubit-wise commuting groups)",
        "uncertainty": float(res.uncertainty),
        "ci95_mha": _ci95_mha(res.uncertainty),
        "measurements": res.measurements,
        "fci_exact_energy": float(e_full),
        "active_space_fci_energy": float(e_full),
        "delta_mha": float(res.delta_mha),
        "exact_delta_mha": float(exact_delta),
        "active_space_delta_mha": float(res.delta_mha),
        "chemical_accuracy": bool(claimable),
        "active_space_chemical_accuracy": bool(claimable),
        "chemical_accuracy_limit_mha": 1.6,
        "full_space_status": status,
        "claimable": bool(claimable),
        "converged": bool(res.stopped in ("within_target", "gradient_converged")),
        "trajectory": [],
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "limitations": ("Full orbital space; ADAPT-VQE operator pool is explicit. "
                        "The exact statevector value is an optimizer diagnostic; "
                        "the claim uses the shot-measured energy and uncertainty."),
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }

#: Active spaces for the molecular VQEs. These are deliberately smaller than
#: the full molecular orbital space; the result reports both the active-space
#: CASCI and the full-molecule FCI so no one mistakes a reduced model for the
#: full molecule.
_NATURE_ACTIVE = {
    "LIH": {"name": "LiH", "atom": "Li 0 0 0; H 0 0 {r}", "ncas": 2, "nelecas": 2},
    "BEH2": {"name": "BeH2", "atom": "Be 0 0 0; H 0 0 {r}; H 0 0 -{r}", "ncas": 3, "nelecas": 4},
    "H2O": {"name": "H2O", "geometry": "water", "ncas": 4, "nelecas": 4},
}


def _molecule_atom(spec: dict[str, Any], bond_distance: float) -> str:
    """Build the explicit geometry for a supported molecular family.

    H2O uses a fixed 104.5-degree H-O-H angle and the requested O-H length;
    the other families use their linear template with the same length.
    """
    if spec.get("geometry") == "water":
        r = float(bond_distance)
        theta = math.radians(104.5 / 2.0)
        x = r * math.sin(theta)
        y = r * math.cos(theta)
        return f"O 0 0 0; H {x:.8f} {y:.8f} 0; H {-x:.8f} {y:.8f} 0"
    atom = spec.get("atom")
    if not atom:
        raise ValueError(f"no geometry builder for {spec.get('name')!r}")
    return str(atom).format(r=float(bond_distance))


def _full_fci_and_casci(atom: str, ncas: int, nelecas: int,
                        basis: str = "sto-3g", charge: int = 0,
                        spin: int = 0,
                        compute_full_fci: bool = True) -> tuple[float | None, float]:
    """Compute full FCI (when within local orbital cap) and active-space CASCI."""
    from pyscf import gto, scf, fci, mcscf
    mol = gto.M(atom=atom, basis=basis, charge=int(charge), spin=int(spin), verbose=0)
    mf = scf.RHF(mol).run()
    e_fci: float | None = None
    if compute_full_fci:
        val, _ = fci.FCI(mf).kernel()
        e_fci = float(val)
    cas = mcscf.CASCI(mf, ncas, nelecas).run()
    return e_fci, float(cas.e_tot)


def _pauli_measurement_energy(
    ansatz, theta: np.ndarray, qop: Any, shots: int, seed: int,
) -> dict[str, Any]:
    """Shot-measure a Qiskit Nature SparsePauliOp on the ansatz state."""
    qc = ansatz.assign_parameters(theta)
    from qiskit import transpile
    # Qiskit Nature UCCSD carries Pauli-evolution pseudo-ops; Aer needs a
    # concrete basis before measurement. Transpilation is part of execution,
    # not a substitute for it.
    qc = transpile(qc, basis_gates=["u1", "u2", "u3", "cx"],
                   optimization_level=sim_policy.transpile_level(2), seed_transpiler=seed)
    sim = sim_policy.aer_simulator()
    total = 0.0
    variance = 0.0
    terms: dict[str, Any] = {}
    for pauli, coeff in zip(qop.paulis, qop.coeffs):
        label = pauli.to_label()
        c = float(np.real(coeff))
        if abs(c) < 1e-14:
            continue
        if set(label) <= {"I"}:
            mean, std = 1.0, 0.0
        else:
            measured = qc.copy()
            # Qiskit Pauli labels are little-endian (qubit 0 is the LAST
            # character); rotate against the reversed label.
            for q, ch in enumerate(label[::-1]):
                if ch == "X":
                    measured.h(q)
                elif ch == "Y":
                    measured.sdg(q)
                    measured.h(q)
            measured.measure_all()
            counts = sim.run(measured, shots=shots, seed_simulator=seed).result().get_counts()
            n = sum(counts.values())
            acc = 0
            for bitstring, count in counts.items():
                bits = bitstring.replace(" ", "")[::-1]
                parity = sum(1 for q, ch in enumerate(label[::-1]) if ch != "I" and bits[q] == "1")
                acc += count * (1 if parity % 2 == 0 else -1)
            mean = acc / max(1, n)
            std = math.sqrt(max(0.0, 1.0 - mean * mean) / max(1, n))
        terms[label] = {"coefficient": c, "expectation": float(mean), "std_error": float(std)}
        total += c * mean
        variance += (c * std) ** 2
    return {"energy": float(total), "uncertainty": float(math.sqrt(max(0.0, variance))),
            "terms": terms, "shots": shots, "seed": seed}


def _nature_ansatz_qasm(ansatz: Any, theta: np.ndarray, seed: int) -> str:
    """Concrete optimized UCCSD circuit without measurement, for hardware
    admission and measurement-basis preparation. The numerical parameters are
    already bound; no optimizer state is implied to be hardware-derived."""
    if not HAS_QISKIT:
        return ""
    try:
        from qiskit import qasm2, transpile
        bound = ansatz.assign_parameters(theta)
        isa = transpile(bound, basis_gates=["u1", "u2", "u3", "cx"],
                        optimization_level=sim_policy.transpile_level(2), seed_transpiler=int(seed))
        return qasm2.dumps(isa)
    except Exception:
        return ""


def _ansatz_qpy(ansatz: Any) -> str:
    """Serialize the unbound ansatz so a governed hardware loop can assign
    arbitrary parameters.  QASM 2 cannot carry unbound parameters; QPY can.
    This is a circuit representation, not a result or a claimed measurement."""
    if not HAS_QISKIT:
        return ""
    try:
        from qiskit import qpy
        buffer = io.BytesIO()
        qpy.dump(ansatz, buffer)
        return base64.b64encode(buffer.getvalue()).decode("ascii")
    except Exception:
        return ""


def run_nature_vqe(
    molecule: str,
    bond_distance: float,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
    full_space: bool = False,
    layers: int = 2,
    full_space_pool: str = "ceo",
) -> dict[str, Any]:
    """Active-space molecular VQE via Qiskit Nature + PySCF.

    The reported energy is shot-measured. The result explicitly distinguishes
    the active-space CASCI target from the full-molecule FCI anchor: a reduced
    active space is not the full molecule, and the claim must be judged against
    the anchor it actually compared with.
    """
    mol = (molecule or "").upper()
    spec = _NATURE_ACTIVE.get(mol)
    if spec is None:
        return {"success": False, "error": f"molecular VQE supports {sorted(_NATURE_ACTIVE)}"}
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing molecular VQE"}
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]"}
    t0 = time.perf_counter()
    atom = _molecule_atom(spec, float(bond_distance))
    problem = PySCFDriver(atom=atom, basis="sto-3g", charge=0, spin=0).run()
    ncas = int(problem.num_spatial_orbitals)
    particles = problem.num_particles
    nelecas = int(sum(particles) if isinstance(particles, (tuple, list)) else particles)
    requested_full = bool(full_space)
    if requested_full:
        return _run_full_space_vqe({**spec, "full_space_pool": full_space_pool},
                                   atom, float(bond_distance), problem,
                                   ncas, nelecas, shots, seed, max_iterations, layers, t0)
    ncas, nelecas = int(spec["ncas"]), int(spec["nelecas"])
    e_full, e_casci = _full_fci_and_casci(atom, ncas, nelecas)
    reduced = ActiveSpaceTransformer(
        num_electrons=nelecas, num_spatial_orbitals=ncas,
    ).transform(problem)
    mapper = JordanWignerMapper()
    qop = mapper.map(reduced.hamiltonian.second_q_op())
    constants = float(sum(reduced.hamiltonian.constants.values()))
    hf = HartreeFock(reduced.num_spatial_orbitals, reduced.num_particles, mapper)
    ansatz = UCCSD(reduced.num_spatial_orbitals, reduced.num_particles, mapper,
                   initial_state=hf)
    rng = np.random.default_rng(seed)
    x0 = rng.uniform(-0.15, 0.15, size=ansatz.num_parameters)
    trajectory: list[dict[str, Any]] = []

    def objective(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x))
        e = float(np.real(sv.expectation_value(qop)) + constants)
        if len(trajectory) < 500:
            trajectory.append({"iteration": len(trajectory) + 1, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    from scipy.optimize import minimize
    opt = minimize(objective, x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.3, "tol": 1e-8})
    theta = np.asarray(opt.x, dtype=float)
    exact = float(np.real(Statevector.from_instruction(ansatz.assign_parameters(theta))
                          .expectation_value(qop)) + constants)
    measured = _pauli_measurement_energy(ansatz, theta, qop, int(shots), int(seed))
    # The Pauli expectation excludes the active-space constant; add it once
    # (core/nuclear contribution) before comparing with FCI.
    e = measured["energy"] + constants
    measured["constant_energy"] = constants
    # The shot measurement is the result; the exact value is retained as a
    # labelled optimizer diagnostic, never silently substituted.
    delta_full = abs(e - e_full) * 1000.0
    delta_active = abs(e - e_casci) * 1000.0
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": spec["name"],
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "layers": int(layers),
        "method": "VQE UCCSD full-space, COBYLA" if requested_full else "VQE UCCSD active-space, COBYLA",
        "ansatz": (f"UCCSD full-space {reduced.num_spatial_orbitals} orbitals"
                   if requested_full else f"UCCSD active-space {reduced.num_spatial_orbitals} orbitals"),
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas,
                         "spatial_orbitals": int(reduced.num_spatial_orbitals),
                         "full_space": requested_full},
        "shots": int(shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": int(opt.nfev), "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "ansatz_qasm": _nature_ansatz_qasm(ansatz, theta, seed),
        "ansatz_qpy": _ansatz_qpy(ansatz),
        "pauli_coefficients": {
            str(p.to_label()): float(np.real(c))
            for p, c in zip(qop.paulis, qop.coeffs)
            if abs(float(np.real(c))) > 1e-14
        },
        "constant_energy": float(constants),
        "energy": float(e),
        "energy_exact": exact,
        "energy_measured": float(e),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "ci95_mha": _ci95_mha(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": e_full,
        "active_space_fci_energy": e_casci,
        "delta_mha": delta_full,
        "active_space_delta_mha": delta_active,
        "chemical_accuracy": _claimable_against_anchor(e, e_full, measured["uncertainty"]),
        "active_space_chemical_accuracy": _claimable_against_anchor(e, e_casci, measured["uncertainty"]),
        "below_fci": bool(e < e_full - 3.0 * measured["uncertainty"]),
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": _claimable_against_anchor(e, e_full, measured["uncertainty"]),
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "limitations": ("Full orbital space; full-molecule FCI and CASCI coincide."
                        if requested_full else
                        "Active-space model; full-molecule FCI is reported separately and is the physical comparison."),
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def run_generic_vqe(
    atom: str,
    basis: str = "sto-3g",
    charge: int = 0,
    spin: int = 0,
    ncas: int | None = None,
    nelecas: int | None = None,
    layers: int = 1,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
) -> dict[str, Any]:
    """Run a real active-space VQE for a caller-supplied PySCF geometry.

    This is deliberately bounded: closed-shell (neutral or charged ions with
    charge in [-2, 2] and spin=0 — N-08), STO-3G, at most 8 spatial orbitals
    for the full-FCI anchor, 1..4 UCCSD reps (`layers` — N-03), and at most 12
    active-space qubits.
    """
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing generic VQE"}
    if str(basis).strip().lower() != "sto-3g":
        return {"success": False, "error": "generic VQE currently supports basis=sto-3g only"}
    charge_i = int(charge)
    if charge_i < -2 or charge_i > 2:
        return {"success": False, "error": f"generic VQE supports charge in [-2, 2], got {charge_i}"}
    if int(spin) != 0:
        return {"success": False, "error": "generic VQE currently supports closed-shell molecules only (spin=0)"}
    reps = int(layers)
    if reps < 1 or reps > 4:
        return {"success": False, "error": f"generic VQE layers (UCCSD reps) {reps} outside [1, 4]"}
    if ncas is None or nelecas is None:
        return {"success": False, "error": "generic VQE requires explicit ncas and nelecas"}
    ncas, nelecas = int(ncas), int(nelecas)
    if ncas < 1 or nelecas < 2 or nelecas > 2 * ncas:
        return {"success": False, "error": "generic VQE requires 1<=ncas and 2<=nelecas<=2*ncas"}
    if 2 * ncas > 12:
        return {"success": False, "error": f"generic active space needs {2 * ncas} qubits; the local cap is 12"}
    atom = str(atom or "").strip()
    if not atom or len(atom) > 2000 or ";" not in atom:
        return {"success": False, "error": "generic VQE requires an explicit PySCF atom geometry string"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if int(max_iterations) < 1 or int(max_iterations) > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
    # W3-10: typed closed-shell electron-parity pre-check before PySCFDriver.
    try:
        from pyscf.data.elements import charge as _elem_charge
        total_z = 0
        parsed_atoms = 0
        for part in atom.split(";"):
            toks = part.strip().split()
            if toks:
                total_z += int(_elem_charge(toks[0]))
                parsed_atoms += 1
        if parsed_atoms > 0:
            nelec_total = total_z - charge_i
            if nelec_total < 2:
                return {"success": False, "error": f"parity_error: geometry with charge={charge_i} has only {nelec_total} electron(s)"}
            if nelec_total % 2 != 0:
                return {
                    "success": False,
                    "error": f"parity_error: closed-shell spin=0 requires an even electron count; got {nelec_total} electrons for charge={charge_i}",
                }
    except Exception:  # noqa: BLE001 — defer exotic atom syntax to PySCFDriver
        pass
    try:
        problem = PySCFDriver(atom=atom, basis=str(basis), charge=int(charge), spin=int(spin)).run()
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"PySCF could not build the supplied geometry: {e}"}
    spatial = int(problem.num_spatial_orbitals)
    if spatial > 14:
        return {"success": False, "error": f"generic active-space VQE needs {spatial} spatial orbitals; local cap is 14"}
    compute_full_fci = spatial <= 8
    if ncas > spatial:
        return {"success": False, "error": f"ncas {ncas} exceeds the molecule's {spatial} spatial orbitals"}
    electrons = problem.num_particles
    total_electrons = int(sum(electrons) if isinstance(electrons, (tuple, list)) else electrons)
    if nelecas > total_electrons:
        return {"success": False, "error": f"nelecas {nelecas} exceeds the molecule's {total_electrons} electrons"}
    t0 = time.perf_counter()
    try:
        e_full, e_casci = _full_fci_and_casci(
            atom, ncas, nelecas, basis=str(basis), charge=charge_i, spin=0,
            compute_full_fci=compute_full_fci,
        )
        reduced = ActiveSpaceTransformer(num_electrons=nelecas, num_spatial_orbitals=ncas).transform(problem)
        mapper = JordanWignerMapper()
        qop = mapper.map(reduced.hamiltonian.second_q_op())
        constants = float(sum(reduced.hamiltonian.constants.values()))
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"active-space construction refused: {e}"}
    hf = HartreeFock(reduced.num_spatial_orbitals, reduced.num_particles, mapper)
    ansatz = UCCSD(reduced.num_spatial_orbitals, reduced.num_particles, mapper, reps=reps, initial_state=hf)
    rng = np.random.default_rng(int(seed))
    x0 = rng.uniform(-0.15, 0.15, size=ansatz.num_parameters)
    trajectory: list[dict[str, Any]] = []

    def objective(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x))
        e = float(np.real(sv.expectation_value(qop)) + constants)
        if len(trajectory) < 500:
            trajectory.append({"iteration": len(trajectory) + 1, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    from scipy.optimize import minimize
    opt = minimize(objective, x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.3, "tol": 1e-8})
    theta = np.asarray(opt.x, dtype=float)
    exact = float(np.real(Statevector.from_instruction(ansatz.assign_parameters(theta))
                          .expectation_value(qop)) + constants)
    measured = _pauli_measurement_energy(ansatz, theta, qop, int(shots), int(seed))
    energy = float(measured["energy"] + constants)
    # W3-09: compute concrete ansatz depth and 2-qubit gate count.
    from qiskit import transpile as _transpile
    _bound_qc = _transpile(
        ansatz.assign_parameters(theta),
        basis_gates=["u1", "u2", "u3", "cx"],
        optimization_level=0,
    )
    ansatz_depth = int(_bound_qc.depth())
    two_qubit_gate_count = int(_bound_qc.count_ops().get("cx", 0))
    delta_active_mha = abs(energy - e_casci) * 1000.0
    if e_full is not None:
        anchor_mode = "full_fci"
        anchor_energy = float(e_full)
        delta_full_mha: float | None = abs(energy - e_full) * 1000.0
        primary_delta_mha = delta_full_mha
    else:
        anchor_mode = "casci_active_space"
        anchor_energy = float(e_casci)
        delta_full_mha = None
        primary_delta_mha = delta_active_mha
    claimable = _claimable_against_anchor(energy, anchor_energy, measured["uncertainty"])
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": "custom",
        "generic": True,
        "atom": atom,
        "basis": str(basis),
        "charge": int(charge),
        "spin": int(spin),
        "method": "VQE UCCSD active-space (caller geometry), COBYLA",
        "ansatz": f"UCCSD active-space {ncas} orbitals",
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas,
                         "spatial_orbitals": spatial, "full_space": False},
        "layers": reps,
        "num_parameters": int(ansatz.num_parameters),
        "ansatz_depth": ansatz_depth,
        "two_qubit_gate_count": two_qubit_gate_count,
        "anchor_mode": anchor_mode,
        "shots": int(shots), "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": int(opt.nfev), "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "ansatz_qasm": _nature_ansatz_qasm(ansatz, theta, seed),
        "ansatz_qpy": _ansatz_qpy(ansatz),
        "pauli_coefficients": {str(p.to_label()): float(np.real(c))
                               for p, c in zip(qop.paulis, qop.coeffs)
                               if abs(float(np.real(c))) > 1e-14},
        "constant_energy": float(constants),
        "energy": float(energy),
        "energy_exact": exact,
        "energy_measured": float(energy),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "ci95_mha": _ci95_mha(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": float(e_full) if e_full is not None else None,
        "active_space_fci_energy": float(e_casci),
        "delta_mha": primary_delta_mha,
        "full_fci_delta_mha": delta_full_mha,
        "active_space_delta_mha": delta_active_mha,
        "chemical_accuracy": claimable,
        "active_space_chemical_accuracy": _claimable_against_anchor(energy, e_casci, measured["uncertainty"]),
        "below_fci": bool(energy < anchor_energy - 3.0 * measured["uncertainty"]),
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": claimable,
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "limitations": (
            "Caller-supplied geometry; closed-shell STO-3G only; "
            + ("" if e_full is not None else "CASCI active-space anchor only (spatial orbitals > 8 exceed local full-FCI cap); ")
            + "active-space CASCI is reported separately from the full-FCI anchor. "
            + "No geometry relaxation or basis extrapolation."
        ),
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def _atom_fci(symbol: str, spin: int) -> float:
    """Exact STO-3G FCI energy for one atom in its declared spin state."""
    from pyscf import gto, scf, fci
    mol = gto.M(atom=f"{symbol} 0 0 0", basis="sto-3g", charge=0,
                spin=int(spin), verbose=0)
    mf = scf.RHF(mol) if int(spin) == 0 else scf.ROHF(mol)
    mf.run()
    energy, _ = fci.FCI(mf).kernel()
    return float(energy)


def _reaction_atom_products(molecule: str) -> tuple[list[tuple[str, int, int]], str]:
    """Return (element, count, spin) products for an atomization reaction."""
    if molecule == "LIH":
        return [("Li", 1, 1), ("H", 1, 1)], "LiH -> Li + H"
    if molecule == "BEH2":
        return [("Be", 1, 0), ("H", 2, 1)], "BeH2 -> Be + 2 H"
    raise ValueError(f"no atomization reaction for {molecule}")


def _run_atomization_reaction(molecule: str, bond_distance: float, layers: int,
                              shots: int, seed: int, max_iterations: int) -> dict[str, Any]:
    spec = _NATURE_ACTIVE.get(molecule)
    if spec is None:
        return {"success": False, "error": f"reaction energy supports H2, LiH, BeH2, H2O; refusing {molecule}"}
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]"}
    if int(layers) < 1 or int(layers) > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    try:
        products, reaction = _reaction_atom_products(molecule)
    except ValueError as e:
        return {"success": False, "error": str(e)}
    endpoint = run_vqe(molecule, float(bond_distance), int(layers), int(shots), int(seed), int(max_iterations))
    if not endpoint.get("success"):
        return {"success": False, "error": "reaction endpoint failed", "endpoint_error": endpoint.get("error")}
    try:
        product_energy = sum(_atom_fci(symbol, spin) * count for symbol, count, spin in products)
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"exact atom anchor failed: {e}"}
    active_anchor = float(endpoint.get("active_space_fci_energy", endpoint["fci_exact_energy"]))
    full_anchor = float(endpoint["fci_exact_energy"])
    measured = float(endpoint["energy"] - product_energy)
    exact_active = float(active_anchor - product_energy)
    exact_full = float(full_anchor - product_energy)
    uncertainty = float(endpoint["uncertainty"])
    endpoint_ok = bool(endpoint.get("active_space_chemical_accuracy", endpoint.get("claimable", False)))
    claimable = bool(
        endpoint_ok and abs(measured - exact_active) <= 1.6e-3 and uncertainty <= 1.6e-3
    )
    return {
        "success": True,
        "kind": "reaction",
        "result_version": "experiment-result-v1",
        "molecule": molecule,
        "reactant_distance": float(bond_distance),
        "product_distance": 0.0,
        "reaction": reaction,
        "basis": "sto-3g",
        "method": f"{molecule} active-space VQE endpoint + exact atom FCI products",
        "reactant_energy": float(endpoint["energy"]),
        "product_energy": float(product_energy),
        "reaction_energy": measured,
        "reactant_energy_exact": active_anchor,
        "reactant_energy_exact_full": full_anchor,
        "product_energy_exact": float(product_energy),
        "reaction_energy_exact": exact_active,
        "reaction_energy_exact_full": exact_full,
        "uncertainty": uncertainty,
        "delta": abs(measured - exact_active),
        "delta_full": abs(measured - exact_full),
        "claimable": claimable,
        "claimability_scope": f"{molecule} active-space endpoint + exact atomic FCI products",
        "endpoint_claimable": endpoint_ok,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "limitations": "Active-space molecular endpoint plus exact atomic FCI products; the full-molecule FCI delta is reported separately. No geometry relaxation or full-molecule thermochemistry claim.",
        "error_message": "",
    }


def run_hardware_pes(
    points: list[dict[str, Any]] | None = None,
    molecule: str = "H2",
    shots: int = 4096,
    seed: int = 7,
    backend: str | None = None,
    mitigation: str | None = None,
    mitigation_scales: list[float] | None = None,
    source_job_id: str = "",
    extrapolator: str = "linear",
    readout_mitigation: bool = True,
) -> dict[str, Any]:
    """Execute a PES curve as one governed QPU batch.

    Each source point already carries its bound ansatz QASM and Pauli
    coefficients from a measured simulator PES job. This runner validates
    those artifacts, then submits all point measurement circuits in one
    provider batch. Optional ZNE folds each point; a mitigated point is
    reported as inferred and non-claimable, never as a direct measurement.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing hardware PES"}
    if not isinstance(points, list) or len(points) < 2 or len(points) > 12:
        return {"success": False, "error": "hardware PES requires 2..12 source points"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    method = str(mitigation or "").strip().upper()
    if method and method != "ZNE":
        return {"success": False, "error": f"hardware PES mitigation {method!r} is unsupported; use ZNE"}
    scales = [float(x) for x in (mitigation_scales or ([1.0, 3.0, 5.0] if method else []))]
    if method:
        if len(set(scales)) < 2 or any(not math.isfinite(s) or s < 1.0 or s > 15.0 for s in scales):
            return {"success": False, "error": "hardware PES ZNE needs >=2 distinct scales in [1.0, 15.0]"}
    try:
        from qiskit import qasm2
        from experiments import _measurement_qasm, _hardware_batch, _hardware_counts_to_energy, _fold_circuit, _calibration_quality, _claimable_against_anchor
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": f"hardware PES dependencies unavailable: {exc}"}

    prepared: list[dict[str, Any]] = []
    qasms: list[str] = []
    for i, point in enumerate(points):
        if not isinstance(point, dict):
            return {"success": False, "error": f"point {i} is not an object"}
        try:
            bond = float(point["bond_distance"])
            coeffs = {str(k): float(v) for k, v in (point.get("pauli_coefficients") or {}).items()}
            fci = float(point["fci_exact_energy"])
            constant = float(point.get("constant_energy", 0.0))
        except (KeyError, TypeError, ValueError) as exc:
            return {"success": False, "error": f"point {i} missing numeric evidence: {exc}"}
        if not math.isfinite(bond) or not math.isfinite(fci) or not math.isfinite(constant):
            return {"success": False, "error": f"point {i} has non-finite evidence"}
        if not coeffs or any(not math.isfinite(v) for v in coeffs.values()):
            return {"success": False, "error": f"point {i} needs finite pauli_coefficients"}
        qasm = str(point.get("ansatz_qasm") or point.get("qasm") or "").strip()
        if not qasm:
            return {"success": False, "error": f"point {i} has no bound ansatz_qasm; re-run the source PES"}
        try:
            qc = qasm2.loads(qasm)
        except Exception as exc:  # noqa: BLE001
            return {"success": False, "error": f"point {i} ansatz QASM is not executable: {exc}"}
        if getattr(qc, "num_clbits", 0) > 0 or any(
            getattr(inst.operation, "name", "") == "measure" for inst in qc.data
        ):
            return {"success": False, "error": f"point {i} ansatz carries measurement; hardware PES needs a unitary"}
        point_qasms: list[str] = []
        if method:
            for term in sorted(coeffs):
                for scale in scales:
                    point_qasms.append(_measurement_qasm(_fold_circuit(qc, float(scale)), term))
        else:
            for term in sorted(coeffs):
                point_qasms.append(_measurement_qasm(qc, term))
        qasms.extend(point_qasms)
        prepared.append({"bond_distance": bond, "coeffs": coeffs, "fci": fci,
                         "constant": constant, "qasms": point_qasms, "index": i})
    if len(qasms) > 2000:
        return {"success": False, "error": f"hardware PES would submit {len(qasms)} circuits; cap is 2000"}
    try:
        batch = _hardware_batch(qasms, int(shots), int(seed), backend=backend)
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": f"hardware PES batch refused: {exc}"}
    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != len(qasms):
        return {"success": False, "error": "hardware PES batch returned no aligned counts"}
    cal_data = batch.get("calibration") if isinstance(batch.get("calibration"), dict) else None
    used_q = _layout_physical_qubits(batch.get("layout"))
    readout_meta = None
    if method:
        t_env = batch.get("transpile") if isinstance(batch.get("transpile"), dict) else None
        if (
            t_env is not None
            and "min_transpiled_two_qubit_gates" in t_env
            and "max_transpiled_two_qubit_gates" in t_env
            and int(t_env["min_transpiled_two_qubit_gates"]) > 0
            and max(float(s) for s in scales) > 1.0
            and int(t_env["max_transpiled_two_qubit_gates"]) <= int(t_env["min_transpiled_two_qubit_gates"])
        ):
            return {
                "success": False,
                "error": (
                    "ZNE fold scaling collapsed during transpilation: "
                    f"min_2Q={t_env['min_transpiled_two_qubit_gates']} == max_2Q={t_env['max_transpiled_two_qubit_gates']}"
                ),
            }
    quality = _calibration_quality(batch.get("calibration"))
    out_points: list[dict[str, Any]] = []
    cursor = 0
    all_claimable = True
    all_measured = True
    for point in prepared:
        term_count = len(point["qasms"])
        chunk = counts_list[cursor:cursor + term_count]
        cursor += term_count
        if method:
            xs = [float(s) for s in scales]
            by_scale: dict[float, list[dict[str, int]]] = {s: [] for s in xs}
            idx = 0
            for term in sorted(point["coeffs"]):
                for s in xs:
                    by_scale[s].append(chunk[idx])
                    idx += 1
            energies, uncs = [], []
            for s in xs:
                m = _hardware_counts_to_energy(
                    by_scale[s], point["coeffs"],
                    calibration=cal_data, used_qubits=used_q, readout_mitigation=bool(readout_mitigation),
                )
                if readout_meta is None and "readout_mitigation" in m:
                    readout_meta = m["readout_mitigation"]
                energies.append(float(m["energy"]))
                uncs.append(float(m["uncertainty"]))
            try:
                sample_lbl = next(iter(point["coeffs"]))
                id_key = "I" * len(sample_lbl)
                asymptote = float(point["coeffs"][id_key]) if id_key in point["coeffs"] else 0.0
                ext = _extrapolate_zne(xs, energies, uncs, extrapolator=extrapolator, asymptote=asymptote)
            except ValueError as exc:
                return {"success": False, "error": str(exc)}
            energy = float(ext["mitigated"]) + point["constant"]
            uncertainty = float(ext["uncertainty"])
            measured = False
            claimable = False
        else:
            m = _hardware_counts_to_energy(
                chunk, point["coeffs"],
                calibration=cal_data, used_qubits=used_q, readout_mitigation=bool(readout_mitigation),
            )
            if readout_meta is None and "readout_mitigation" in m:
                readout_meta = m["readout_mitigation"]
            energy = float(m["energy"] + point["constant"])
            uncertainty = float(m["uncertainty"])
            measured = True
            claimable = bool(
                _claimable_against_anchor(energy, point["fci"], uncertainty)
                and quality["calibration_fresh"] is not False
            )
        delta = abs(energy - point["fci"])
        actual_backend = str(batch.get("backend") or "ibm-qiskit")
        req_backend = str(backend or actual_backend)
        parent_vendor_ref = str(batch.get("hardware_job_id") or batch.get("job_id") or "")
        out_points.append({
            "bond_distance": point["bond_distance"],
            "energy": energy,
            "energy_exact": point["fci"],
            "fci_exact_energy": point["fci"],
            "uncertainty": uncertainty,
            "ci95_mha": float(1.96 * uncertainty * 1000.0) if math.isfinite(uncertainty) else None,
            # One histogram exists per Pauli term (and per ZNE scale). Store the
            # scale-1 histograms keyed by term instead of mislabelling the first
            # term's counts as the whole point's distribution.
            "counts_by_term": {
                term: (by_scale[float(scales[0])][sorted(point["coeffs"]).index(term)]
                       if method and by_scale.get(float(scales[0])) else chunk[sorted(point["coeffs"]).index(term)])
                for term in sorted(point["coeffs"])
            },
            "counts_scale": (int(scales[0]) if abs(float(scales[0]) - round(float(scales[0]))) < 1e-9 else float(scales[0])) if method else 1,
            "delta_mha": delta * 1000.0,
            "claimable": claimable,
            "measured": measured,
            "mitigated": bool(method),
            "hardware": True,
            "backend": actual_backend,
            "requested_backend": req_backend,
            "vendor_ref": parent_vendor_ref or None,
            "calibration_snapshot_id": quality.get("calibration_snapshot_id"),
        })
        all_claimable = all_claimable and claimable
        all_measured = all_measured and measured
    actual_backend = str(batch.get("backend") or "ibm-qiskit")
    req_backend = str(backend or actual_backend)
    parent_vendor_ref = str(batch.get("hardware_job_id") or batch.get("job_id") or "")
    return {
        "success": True,
        "kind": "hardware_pes",
        "result_version": "experiment-result-v1",
        "molecule": str(molecule or "H2"),
        "backend": actual_backend,
        "requested_backend": req_backend,
        "provider": "ibm-qiskit",
        "shots": int(shots),
        "seed": int(seed),
        "readout_mitigation": readout_meta,
        "error_suppression": batch.get("error_suppression"),
        "points": out_points,
        "point_count": len(out_points),
        "hardware_job_id": parent_vendor_ref,
        "vendor_ref": parent_vendor_ref or None,
        "layout": batch.get("layout"),
        "layout_quality": batch.get("layout_quality"),
        "transpile": batch.get("transpile"),
        "calibration": batch.get("calibration"),
        **quality,
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "measured": all_measured,
        "hardware": True,
        "mitigated": bool(method),
        "claimable": all_claimable and not method,
        "source_job_id": str(source_job_id),
        "limitations": "One QPU batch across the source PES points; per-point claimability uses FCI and shot uncertainty. ZNE points are inferred and non-claimable. No thermodynamic-limit claim.",
        "error_message": "",
    }


def run_repeated_vqe(
    molecule: str = "H2",
    bond_distance: float = 0.7414,
    layers: int = 2,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
    repeats: int = 3,
) -> dict[str, Any]:
    """Run independent VQE optimizations at one geometry and report variance.

    Each repeat is a full optimizer run with its own seed; the returned energy
    is the mean of the shot-measured energies, with both the shot uncertainty
    and the between-run standard error. A result is claimable only when every
    repeat carries its own anchor and the combined interval fits the limit.
    """
    if int(repeats) < 2 or int(repeats) > 5:
        return {"success": False, "error": f"repeats {repeats!r} outside [2,5]"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    runs: list[dict[str, Any]] = []
    for i in range(int(repeats)):
        res = run_vqe(molecule, float(bond_distance), int(layers), int(shots),
                      int(seed) + 101 * i, int(max_iterations))
        if not res.get("success"):
            return {"success": False, "error": f"repeat {i} failed", "repeat_error": res.get("error")}
        runs.append({
            "repeat": i,
            "seed": int(seed) + 101 * i,
            "energy": float(res["energy"]),
            "uncertainty": float(res["uncertainty"]),
            "energy_exact": float(res["energy_exact"]),
            "fci_exact_energy": float(res["fci_exact_energy"]),
            "claimable": bool(res.get("claimable", False)),
            "converged": bool(res.get("converged", False)),
        })
    energies = [r["energy"] for r in runs]
    mean = float(sum(energies) / len(energies))
    fci = float(runs[0]["fci_exact_energy"])
    if len(energies) > 1:
        variance = sum((e - mean) ** 2 for e in energies) / (len(energies) - 1)
        between_se = float(math.sqrt(max(0.0, variance) / len(energies)))
    else:
        between_se = 0.0
    mean_shot = float(sum(r["uncertainty"] for r in runs) / len(runs))
    combined = float(math.sqrt(between_se * between_se + mean_shot * mean_shot))
    all_endpoint_claimable = all(r["claimable"] for r in runs)
    claimable = bool(
        all_endpoint_claimable
        and _claimable_against_anchor(mean, fci, combined)
    )
    return {
        "success": True,
        "kind": "vqe_repeats",
        "result_version": "experiment-result-v1",
        "molecule": str(molecule or "H2"),
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": "Repeated VQE optimizations, mean of shot-measured energies",
        "shots": int(shots),
        "seed": int(seed),
        "repeats": int(repeats),
        "runs": runs,
        "energy": mean,
        "energy_measured": mean,
        "energy_exact": float(runs[0]["energy_exact"]),
        "fci_exact_energy": fci,
        "uncertainty": combined,
        "between_run_standard_error": between_se,
        "mean_shot_uncertainty": mean_shot,
        "delta_mha": abs(mean - fci) * 1000.0,
        "claimable": claimable,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "limitations": "Independent optimizer seeds at one geometry; the interval combines shot noise and between-run standard error. It is not a calibration or systematic-error budget.",
        "error_message": "",
    }


def _relax_geometry_rhf(
    atom: str,
    basis: str,
    charge: int,
    spin: int,
    max_steps: int,
) -> dict[str, Any]:
    """Relax every nuclear coordinate with an analytic RHF gradient.

    This is a real multidimensional relaxation, but it is deliberately scoped:
    the energy/gradient model is RHF/STO-3G, not a FCI gradient. The VQE claim
    is made at the relaxed geometry and the relaxation method is reported.
    """
    if int(charge) != 0 or int(spin) != 0:
        return {"success": False, "error": "multidimensional relaxation requires a neutral closed-shell molecule (charge=0, spin=0)"}
    if str(basis).strip().lower() != "sto-3g":
        return {"success": False, "error": "multidimensional relaxation currently supports basis=sto-3g only"}
    atom = str(atom or "").strip()
    if not atom or len(atom) > 2000 or ";" not in atom:
        return {"success": False, "error": "multidimensional relaxation requires an explicit PySCF atom geometry string"}
    if max_steps < 1 or max_steps > 300:
        return {"success": False, "error": f"max_geometry_steps {max_steps!r} outside [1,300]"}
    try:
        from pyscf import gto, scf, grad
        from scipy.optimize import minimize
        mol = gto.M(atom=atom, basis=str(basis), charge=int(charge), spin=int(spin), verbose=0)
        symbols = [str(mol.atom_pure_symbol(i)) for i in range(mol.natm)]
        coords = np.asarray(mol.atom_coords(unit="Angstrom"), dtype=float)
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"PySCF could not parse the supplied geometry: {e}"}
    if not (2 <= len(symbols) <= 6):
        return {"success": False, "error": f"multidimensional relaxation supports 2..6 atoms, got {len(symbols)}"}
    if coords.shape != (len(symbols), 3) or not np.all(np.isfinite(coords)):
        return {"success": False, "error": "supplied geometry has non-finite coordinates"}
    coords = coords - coords.mean(axis=0)

    def encode(x: np.ndarray) -> str:
        return "; ".join(
            f"{symbol} {float(x[3*i]):.10f} {float(x[3*i+1]):.10f} {float(x[3*i+2]):.10f}"
            for i, symbol in enumerate(symbols)
        )

    def objective(x: np.ndarray) -> tuple[float, np.ndarray]:
        m = gto.M(atom=encode(np.asarray(x, dtype=float)), basis=str(basis),
                  charge=int(charge), spin=int(spin), verbose=0)
        mf = scf.RHF(m)
        mf.conv_tol = 1e-10
        mf.max_cycle = 200
        mf.run()
        if not mf.converged:
            raise RuntimeError("RHF did not converge while evaluating the geometry")
        g = np.asarray(grad.RHF(mf).kernel(), dtype=float).reshape(-1)
        if not np.all(np.isfinite(g)):
            raise RuntimeError("RHF returned a non-finite nuclear gradient")
        return float(mf.e_tot), g

    try:
        opt = minimize(
            objective, coords.reshape(-1), method="BFGS", jac=True,
            options={"maxiter": int(max_steps), "gtol": 1e-5},
        )
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"geometry relaxation failed: {e}"}
    final_x = np.asarray(opt.x, dtype=float).reshape(-1, 3)
    if not np.all(np.isfinite(final_x)):
        return {"success": False, "error": "geometry relaxation returned non-finite coordinates"}
    final_x = final_x - final_x.mean(axis=0)
    gradient = np.asarray(getattr(opt, "jac", np.zeros_like(final_x)), dtype=float).reshape(-1)
    gradient_norm = float(np.linalg.norm(gradient))
    pair_distances = []
    for i in range(len(symbols)):
        for j in range(i + 1, len(symbols)):
            pair_distances.append(float(np.linalg.norm(final_x[i] - final_x[j])))
    minimum_pair = min(pair_distances) if pair_distances else 0.0
    converged = bool(opt.success or gradient_norm <= 1e-4)
    if minimum_pair < 0.4:
        return {"success": False, "error": f"geometry relaxation collapsed two atoms to {minimum_pair:.4f} Å; refusing"}
    if not converged:
        return {
            "success": False,
            "error": f"geometry relaxation did not converge: {opt.message}",
            "relaxation": {
                "method": "RHF/STO-3G analytic nuclear gradient + BFGS",
                "success": False,
                "iterations": int(getattr(opt, "nit", 0)),
                "function_evaluations": int(getattr(opt, "nfev", 0)),
                "gradient_norm": gradient_norm,
                "final_rhf_energy": float(opt.fun),
                "message": str(opt.message),
            },
        }
    optimized_atom = encode(final_x.reshape(-1))
    geometry = [
        {"symbol": symbol, "x": float(final_x[i, 0]), "y": float(final_x[i, 1]), "z": float(final_x[i, 2])}
        for i, symbol in enumerate(symbols)
    ]
    bonds = []
    for i in range(len(symbols)):
        for j in range(i + 1, len(symbols)):
            bonds.append({"i": i, "j": j, "symbols": f"{symbols[i]}-{symbols[j]}",
                          "distance_angstrom": float(np.linalg.norm(final_x[i] - final_x[j]))})
    return {
        "success": True,
        "optimized_atom": optimized_atom,
        "optimized_geometry": geometry,
        "bond_lengths": bonds,
        "optimized_bond_distance": float(np.linalg.norm(final_x[0] - final_x[1])) if len(symbols) == 2 else None,
        "relaxation": {
            "method": "RHF/STO-3G analytic nuclear gradient + BFGS",
            "success": True,
            "converged": converged,
            "iterations": int(getattr(opt, "nit", 0)),
            "function_evaluations": int(getattr(opt, "nfev", 0)),
            "gradient_evaluations": int(getattr(opt, "njev", 0)),
            "gradient_norm": gradient_norm,
            "final_rhf_energy": float(opt.fun),
            "minimum_pair_distance_angstrom": minimum_pair,
            "degrees_of_freedom": max(0, 3 * len(symbols) - 6),
            "message": str(opt.message),
        },
    }


def run_geometry_optimization(
    molecule: str = "H2",
    initial_bond_distance: float = 0.7414,
    layers: int = 2,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
    max_geometry_steps: int = 100,
    *,
    atom: str | None = None,
    basis: str = "sto-3g",
    charge: int = 0,
    spin: int = 0,
    ncas: int | None = None,
    nelecas: int | None = None,
) -> dict[str, Any]:
    """Optimize a molecular geometry, then VQE the optimized geometry.

    With ``atom`` supplied this performs a genuine multidimensional relaxation
    of every Cartesian coordinate under an RHF/STO-3G analytic gradient, then
    runs the bounded active-space VQE at the relaxed geometry. Without
    ``atom`` the curated one-coordinate templates remain available.
    """
    if atom is not None:
        if ncas is None or nelecas is None:
            return {"success": False, "error": "multidimensional geometry optimization requires explicit ncas and nelecas for the final VQE"}
        if not HAS_NATURE or not HAS_QISKIT:
            return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing geometry optimization"}
        if int(shots) < 128 or int(shots) > 1_000_000:
            return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
        if int(max_iterations) < 1 or int(max_iterations) > 1000:
            return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
        relaxed = _relax_geometry_rhf(str(atom), str(basis), int(charge), int(spin), int(max_geometry_steps))
        if not relaxed.get("success"):
            return {"success": False, "error": relaxed.get("error", "geometry relaxation failed"),
                    "relaxation": relaxed.get("relaxation")}
        optimized_atom = str(relaxed["optimized_atom"])
        final = run_generic_vqe(
            atom=optimized_atom, basis=str(basis), charge=int(charge), spin=int(spin),
            ncas=int(ncas), nelecas=int(nelecas), shots=int(shots), seed=int(seed),
            max_iterations=int(max_iterations),
        )
        if not final.get("success"):
            return {"success": False, "error": "VQE at the relaxed geometry failed",
                    "vqe_error": final.get("error"), "relaxation": relaxed.get("relaxation"),
                    "optimized_atom": optimized_atom}
        out = {
            "success": True,
            "kind": "geometry_optimization",
            "result_version": "experiment-result-v1",
            "molecule": "custom",
            "generic": True,
            "atom": str(atom),
            "basis": str(basis),
            "charge": int(charge),
            "spin": int(spin),
            "method": "Live PySCF RHF/STO-3G multidimensional geometry relaxation + active-space VQE",
            "initial_geometry": str(atom),
            "optimized_atom": optimized_atom,
            "optimized_geometry": relaxed["optimized_geometry"],
            "bond_lengths": relaxed["bond_lengths"],
            "relaxation": relaxed["relaxation"],
            "geometry_relaxed": True,
            "classical_energy_exact": float(final["fci_exact_energy"]),
            "classical_optimizer": {
                "name": "BFGS with analytic RHF/STO-3G nuclear gradient",
                "function_evaluations": int(relaxed["relaxation"]["function_evaluations"]),
                "iterations": int(relaxed["relaxation"]["iterations"]),
                "gradient_norm": float(relaxed["relaxation"]["gradient_norm"]),
                "success": True,
                "message": str(relaxed["relaxation"]["message"]),
                "degrees_of_freedom": int(relaxed["relaxation"]["degrees_of_freedom"]),
            },
            "energy": float(final["energy"]),
            "energy_exact": float(final["energy_exact"]),
            "fci_exact_energy": float(final["fci_exact_energy"]),
            "active_space_fci_energy": final.get("active_space_fci_energy"),
            "uncertainty": float(final["uncertainty"]),
            "delta_mha": float(final.get("delta_mha", 0.0)),
            "claimable": bool(final.get("claimable", False)),
            "measured": True,
            "hardware": False,
            "final_vqe": final,
            "limitations": ("All Cartesian coordinates are relaxed under an RHF/STO-3G "
                            "analytic gradient, then the final energy is a separate active-space "
                            "VQE at that geometry. This is not an FCI-gradient relaxation and "
                            "the final VQE remains subject to its declared active-space and "
                            "shot-noise uncertainty."),
            "error_message": "",
        }
        if relaxed.get("optimized_bond_distance") is not None:
            out["optimized_bond_distance"] = float(relaxed["optimized_bond_distance"])
            out["bond_distance"] = float(relaxed["optimized_bond_distance"])
        return out
    mol = (molecule or "").upper()
    if mol not in ("H2", "LIH", "BEH2", "H2O"):
        return {"success": False, "error": f"geometry optimization supports H2, LiH, BeH2, H2O, or an explicit atom geometry; refusing {molecule}"}
    if not (0.3 <= float(initial_bond_distance) <= 10.0):
        return {"success": False, "error": f"initial_bond_distance {initial_bond_distance!r} outside [0.3,10.0]"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if int(max_iterations) < 1 or int(max_iterations) > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
    if mol == "H2":
        lower, upper = 0.45, 2.5
    elif mol == "H2O":
        lower, upper = 0.55, 1.8
    else:
        lower, upper = 0.7, 4.0

    def objective(r: float) -> float:
        if mol == "H2":
            _, fci, _ = h2_hamiltonian(float(r))
            return fci
        spec = _NATURE_ACTIVE[mol]
        atom = _molecule_atom(spec, float(r))
        e_full, _ = _full_fci_and_casci(atom, int(spec["ncas"]), int(spec["nelecas"]))
        return e_full

    from scipy.optimize import minimize_scalar
    try:
        opt = minimize_scalar(objective, bounds=(lower, upper), method="bounded",
                              options={"xatol": 1e-3})
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"geometry optimization failed: {e}"}
    optimized = float(opt.x)
    final = run_vqe(mol, optimized, int(layers), int(shots), int(seed), int(max_iterations))
    if not final.get("success"):
        return {"success": False, "error": "VQE at optimized geometry failed", "vqe_error": final.get("error")}
    spec = _NATURE_ACTIVE.get(mol)
    atom = (f"H 0 0 0; H 0 0 {optimized:.8f}" if mol == "H2"
            else _molecule_atom(spec, optimized))
    return {
        "success": True,
        "kind": "geometry_optimization",
        "result_version": "experiment-result-v1",
        "molecule": mol,
        "method": "Live PySCF bounded geometry search + shot-measured VQE at the optimized geometry",
        "initial_bond_distance": float(initial_bond_distance),
        "optimized_bond_distance": optimized,
        "bond_distance": optimized,
        "optimized_atom": atom,
        "classical_energy_exact": float(opt.fun),
        "classical_optimizer": {
            "name": "bounded scalar minimization",
            "function_evaluations": int(getattr(opt, "nfev", 0)),
            "success": bool(opt.success),
            "message": str(opt.message),
            "bounds": [lower, upper],
            "xatol": 1e-3,
        },
        "energy": float(final["energy"]),
        "energy_exact": float(final["energy_exact"]),
        "fci_exact_energy": float(final["fci_exact_energy"]),
        "active_space_fci_energy": final.get("active_space_fci_energy"),
        "uncertainty": float(final["uncertainty"]),
        "delta_mha": float(final.get("delta_mha", 0.0)),
        "claimable": bool(final.get("claimable", False)),
        "measured": True,
        "hardware": False,
        "final_vqe": final,
        "limitations": "One internal bond coordinate is optimized; angles/molecular orientation are held fixed by the curated geometry template. The final claim is the VQE claim at the optimized geometry, not the classical optimizer's value.",
        "error_message": "",
    }


def _counts_cut(counts: dict[str, int], edges: list[tuple[int, int]]) -> float:
    """Mean MaxCut value of a Qiskit counts distribution.

    Qiskit bitstrings put qubit 0 at the rightmost character, which is the
    integer LSB — the same convention as a Statevector amplitude index. Do not
    reverse the string: that permutes qubits and silently changes the cut.
    """
    total = sum(int(v) for v in counts.values())
    if total <= 0:
        return 0.0
    acc = 0
    for bs, count in counts.items():
        mask = int(str(bs).replace(" ", ""), 2)
        acc += int(count) * sum(
            1 for a, b in edges if ((mask >> a) & 1) != ((mask >> b) & 1))
    return float(acc) / float(total)


def _ratio_ci_from_counts(
    counts: dict[str, int],
    edges: list[tuple[int, int]],
    optimal_cut: int,
    mean_cut: float,
) -> tuple[list[float] | None, float | None]:
    """Normal-approximation 95% CI for a MaxCut ratio from a shot histogram.

    Qiskit bitstrings place qubit 0 at the rightmost character, so the cut is
    computed from ``int(bitstring, 2)`` exactly as ``_counts_cut`` does. Do not
    reverse the string here: that double-reverses the qubit order and changes
    the sampled variance.
    """
    total = sum(int(v) for v in counts.values())
    if total <= 1 or int(optimal_cut) <= 0:
        return None, None
    sq_diff = 0.0
    for bitstring, cnt in counts.items():
        mask = int(str(bitstring).replace(" ", ""), 2)
        c_val = sum(1 for a, b in edges if ((mask >> int(a)) & 1) != ((mask >> int(b)) & 1))
        sq_diff += int(cnt) * ((float(c_val) - float(mean_cut)) ** 2)
    var = sq_diff / (total - 1)
    se = math.sqrt(max(var, 0.0) / total)
    ratio = float(mean_cut) / float(optimal_cut)
    half = 1.959963984540054 * (se / float(optimal_cut))
    return [round(max(0.0, ratio - half), 6), round(min(1.0, ratio + half), 6)], se


def run_qaoa(
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    max_iterations: int = 80,
    graph_seed: int = 3,
    restarts: int = 0,
) -> dict[str, Any]:
    """Run QAOA MaxCut on a fixed 8-node 3-regular graph."""
    t0 = time.perf_counter()
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing QAOA"}
    import networkx as nx
    from qiskit import QuantumCircuit
    from qiskit.quantum_info import Statevector
    from scipy.optimize import minimize

    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    if int(restarts) < 0 or int(restarts) > 20:
        return {"success": False, "error": f"restarts {restarts!r} outside [0,20]"}
    g = nx.random_regular_graph(3, 8, seed=int(graph_seed))
    edges = sorted(tuple(sorted(e)) for e in g.edges())
    n = g.number_of_nodes()
    # Exact MaxCut, computed here (not hardcoded).
    best = -1
    mask_best = 0
    for mask in range(1 << n):
        cut = sum(1 for a, b in edges if ((mask >> a) & 1) != ((mask >> b) & 1))
        if cut > best:
            best, mask_best = cut, mask

    def qaoa(params: np.ndarray) -> QuantumCircuit:
        qc = QuantumCircuit(n)
        qc.h(range(n))
        for layer in range(int(layers)):
            gamma, beta = float(params[2 * layer]), float(params[2 * layer + 1])
            for a, b in edges:
                qc.cx(a, b)
                qc.rz(2.0 * gamma, b)
                qc.cx(a, b)
            qc.rx(2.0 * beta, range(n))
        return qc

    def cut_value(params: np.ndarray) -> float:
        sv = Statevector.from_instruction(qaoa(params)).data
        probs = np.abs(sv) ** 2
        val = 0.0
        for idx, p in enumerate(probs):
            if p <= 0:
                continue
            val += p * sum(1 for a, b in edges if ((idx >> a) & 1) != ((idx >> b) & 1))
        return float(val)

    rng = np.random.default_rng(seed)
    x0 = rng.uniform(0.0, np.pi, size=2 * int(layers))
    opt = minimize(lambda x: -cut_value(x), x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.2, "tol": 1e-8})
    best_x = np.asarray(opt.x, dtype=float)
    best_value = cut_value(best_x)
    best_success = bool(opt.success)
    total_evals = int(opt.nfev)
    # Multi-start: p=2 sits near its ~0.90 ceiling; independent starts recover
    # a few tenths of a percent and expose optimizer variance honestly.
    for _ in range(int(restarts)):
        x0r = rng.uniform(0.0, np.pi, size=2 * int(layers))
        cand = minimize(lambda x: -cut_value(x), x0r, method="COBYLA",
                        options={"maxiter": int(max_iterations), "rhobeg": 0.2, "tol": 1e-8})
        total_evals += int(cand.nfev)
        cand_value = cut_value(np.asarray(cand.x, dtype=float))
        if cand_value > best_value:
            best_value = cand_value
            best_x = np.asarray(cand.x, dtype=float)
            best_success = bool(cand.success)
    theta = best_x
    value = best_value
    qc = qaoa(theta)
    qc.measure_all()
    counts = sim_policy.aer_simulator(qc).run(qc, shots=int(shots), seed_simulator=int(seed)).result().get_counts()
    try:
        from qiskit import qasm2
        qaoa_qasm = qasm2.dumps(qc)
    except Exception:
        qaoa_qasm = ""
    shot_cut = _counts_cut(counts, edges)
    # R-A: retain shot histogram and derive normal-approximation 95% CI from the
    # sampled cut distribution (counts x graph edges), matching Go ratioCIFromCounts.
    counts_clean = {str(k): int(v) for k, v in counts.items()}
    ratio_ci95, cut_se = _ratio_ci_from_counts(counts_clean, edges, int(best), shot_cut)
    return {
        "success": True, "kind": "qaoa", "result_version": "experiment-result-v1", "molecule": "MaxCut",
        "graph": {"nodes": n, "edges": edges, "regular_degree": 3},
        "circuit_qasm": qaoa_qasm,
        "counts": counts_clean,
        "layers": int(layers), "shots": int(shots), "seed": int(seed),
        "theta": [float(v) for v in theta],
        "energy": float(shot_cut), "energy_exact": float(value),
        "energy_measured": float(shot_cut),
        "cut_value": float(shot_cut), "cut_value_exact": float(value),
        "optimal_cut": int(best), "optimal_assignment": int(mask_best),
        "approximation_ratio": float(shot_cut / best) if best > 0 else None,
        "ratio_ci95": ratio_ci95,
        "ratio_ci_method": "normal_from_sampled_cuts" if ratio_ci95 is not None else None,
        "uncertainty": float(cut_se) if cut_se is not None else None, "measured": True,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "optimizer": {"name": "COBYLA", "success": bool(best_success),
                      "evaluations": int(total_evals), "message": str(opt.message),
                      "max_iterations": int(max_iterations), "restarts": int(restarts),
                      "status": "converged" if best_success else "max_iterations"},
        "converged": bool(best_success),
        "optimizer_status": "converged" if best_success else "max_iterations",
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def run_qaoa_hardware_loop(
    nodes: int,
    edges: list[list[int]],
    optimal_cut: int,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    max_iterations: int = 8,
    backend: str | None = None,
) -> dict[str, Any]:
    """Hardware-in-the-loop QAOA for a caller-stated graph.

    The circuit is built locally, but every objective evaluation is a QPU
    batch; the optimizer never sees a statevector value.  The exact optimum is
    supplied only for the approximation-ratio denominator.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing hardware QAOA loop"}
    if int(nodes) < 2 or int(nodes) > 20 or not edges:
        return {"success": False, "error": "QAOA loop requires 2..20 nodes and edges"}
    if int(optimal_cut) <= 0:
        return {"success": False, "error": "QAOA loop requires a positive optimal_cut"}
    if int(layers) < 1 or int(layers) > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    if int(max_iterations) < 1 or int(max_iterations) > 30:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,30]"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    from qiskit import QuantumCircuit, qasm2

    def circuit(x: np.ndarray) -> QuantumCircuit:
        qc = QuantumCircuit(int(nodes))
        qc.h(range(int(nodes)))
        for layer in range(int(layers)):
            gamma, beta = float(x[2 * layer]), float(x[2 * layer + 1])
            for a, b in edges:
                qc.cx(int(a), int(b))
                qc.rz(2.0 * gamma, int(b))
                qc.cx(int(a), int(b))
            qc.rx(2.0 * beta, range(int(nodes)))
        qc.measure_all()
        return qc

    trajectory: list[dict[str, Any]] = []
    hardware_job_ids: list[str] = []
    usage_values: list[float] = []
    actual_values: list[float] = []
    last_layout: Any = None
    last_calibration: Any = None

    def objective(x: np.ndarray) -> float:
        nonlocal last_layout, last_calibration
        try:
            batch = _hardware_batch([qasm2.dumps(circuit(x))], int(shots), int(seed), backend=backend)
        except Exception as e:  # noqa: BLE001
            raise RuntimeError(f"hardware QAOA evaluation failed: {e}") from e
        counts_list = batch.get("counts")
        if not isinstance(counts_list, list) or len(counts_list) != 1:
            raise RuntimeError("hardware QAOA batch returned no counts")
        counts = counts_list[0]
        total = sum(int(v) for v in counts.values())
        if total <= 0:
            raise RuntimeError("hardware QAOA returned zero counts")
        cut_sum = 0
        for bitstring, n in counts.items():
            bits = str(bitstring).replace(" ", "")[::-1]
            mask = 0
            for i, b in enumerate(bits):
                if b == "1":
                    mask |= 1 << i
            cut_sum += int(n) * sum(1 for a, b in edges if ((mask >> int(a)) & 1) != ((mask >> int(b)) & 1))
        cut = float(cut_sum) / float(total)
        job_id = str(batch.get("hardware_job_id") or batch.get("job_id") or "")
        if job_id:
            hardware_job_ids.append(job_id)
        if batch.get("usage_seconds") is not None:
            usage_values.append(float(batch["usage_seconds"]))
        if batch.get("actual_usd") is not None:
            actual_values.append(float(batch["actual_usd"]))
        trajectory.append({"iteration": len(trajectory) + 1, "measured_cut": cut,
                           "theta": [float(v) for v in x]})
        return -cut

    x0 = np.zeros(2 * int(layers), dtype=float)
    x0[0::2] = 0.5
    x0[1::2] = 0.5
    try:
        from scipy.optimize import minimize
        opt = minimize(objective, x0, method="COBYLA",
                       options={"maxiter": int(max_iterations), "rhobeg": 0.2, "tol": 1e-6})
        final_x = np.asarray(opt.x if opt.x is not None else x0, dtype=float)
        final_cut = -float(objective(final_x))
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"hardware QAOA loop refused: {e}"}
    return {
        "success": True,
        "kind": "qaoa_hardware_loop",
        "result_version": "experiment-result-v1",
        "molecule": "MaxCut",
        "graph": {"nodes": int(nodes), "edges": edges},
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "theta": [float(v) for v in final_x],
        "energy": float(final_cut),
        "measured_cut": float(final_cut),
        "optimal_cut": int(optimal_cut),
        "approximation_ratio": float(final_cut / float(optimal_cut)),
        "hardware": True,
        "measured": True,
        "energy_measured": float(final_cut),
        "energy_measured_source": "QPU cut expectation (hardware-in-the-loop)",
        "uncertainty": None,
        "claimable": False,
        "simulated_optimizer": False,
        "optimizer_backend": "ibm-qiskit",
        "measurement_backend": "ibm-qiskit",
        "optimizer": {"name": "COBYLA", "success": bool(opt.success),
                      "evaluations": int(opt.nfev), "message": str(opt.message),
                      "max_iterations": int(max_iterations)},
        "trajectory": trajectory,
        "iteration_batches": len(hardware_job_ids),
        "hardware_job_ids": hardware_job_ids,
        "hardware_job_id": hardware_job_ids[-1] if hardware_job_ids else "",
        "layout": last_layout,
        "calibration": last_calibration,
        **_calibration_quality(last_calibration),
        "usage_seconds": sum(usage_values) if usage_values else None,
        "actual_usd": sum(actual_values) if len(actual_values) == len(hardware_job_ids) and actual_values else None,
        "provider": "ibm-qiskit",
        "backend": "ibm-qiskit",
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def run_qaoa_hardware(
    qasm: str,
    optimal_cut: int,
    nodes: int,
    edges: list[list[int]],
    shots: int = 4096,
    seed: int = 7,
    energy_exact: float = 0.0,
    backend: str | None = None,
) -> dict[str, Any]:
    """Execute a locally optimized QAOA final state on a real QPU.

    One provider batch/job; no optimizer runs on hardware. The reported cut is
    computed from hardware counts against the exact graph optimum.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing hardware QAOA"}
    if not str(qasm or "").strip():
        return {"success": False, "error": "hardware QAOA requires circuit_qasm"}
    if int(optimal_cut) <= 0 or int(nodes) <= 0 or not edges:
        return {"success": False, "error": "hardware QAOA requires optimal_cut, nodes, and edges"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]: refusing"}
    try:
        batch = _hardware_batch([str(qasm)], int(shots), int(seed), backend=backend)
    except Exception as e:  # noqa: BLE001
        return {"success": False, "error": f"hardware batch refused: {e}"}
    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != 1:
        return {"success": False, "error": "hardware batch returned no QAOA counts"}
    counts = counts_list[0]
    total = sum(int(v) for v in counts.values())
    if total <= 0:
        return {"success": False, "error": "hardware QAOA returned zero counts"}
    cut_sum = 0
    for bitstring, n in counts.items():
        bits = str(bitstring).replace(" ", "")[::-1]
        mask = 0
        for i, b in enumerate(bits):
            if b == "1":
                mask |= 1 << i
        cut_sum += int(n) * sum(1 for a, b in edges if ((mask >> int(a)) & 1) != ((mask >> int(b)) & 1))
    measured_cut = float(cut_sum) / float(total)
    ratio = measured_cut / float(optimal_cut)
    ratio_ci95, cut_se = _ratio_ci_from_counts(counts, edges, int(optimal_cut), measured_cut)
    return {
        "success": True,
        "kind": "qaoa_hardware",
        "result_version": "experiment-result-v1",
        "molecule": "MaxCut",
        "graph": {"nodes": int(nodes), "edges": edges},
        "shots": int(shots),
        "seed": int(seed),
        "counts": counts,
        "energy": measured_cut,
        "measured_cut": measured_cut,
        "energy_exact": float(energy_exact),
        "optimal_cut": int(optimal_cut),
        "approximation_ratio": float(ratio),
        "ratio_ci95": ratio_ci95,
        "ratio_ci_method": "normal_from_sampled_cuts" if ratio_ci95 is not None else None,
        "uncertainty": float(cut_se) if cut_se is not None else None,
        "hardware": True,
        "measured": True,
        "simulated_optimizer": True,
        "provider": "ibm-qiskit",
        "backend": str(batch.get("backend") or "ibm-qiskit"),
        "hardware_job_id": str(batch.get("hardware_job_id") or batch.get("job_id") or ""),
        "layout": batch.get("layout"),
        "layout_quality": batch.get("layout_quality"),
        "transpile": batch.get("transpile"),
        "error_suppression": batch.get("error_suppression"),
        "readout_mitigation": {"enabled": False, "applied": False, "method": "not_applicable_bitstring_cut"},
        "calibration": batch.get("calibration"),
        **_calibration_quality(batch.get("calibration")),
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def run_pes(
    molecule: str = "H2",
    r_min: float = 0.5,
    r_max: float = 2.0,
    steps: int = 5,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    max_iterations: int = 60,
    full_space: bool = False,
    full_space_pool: str = "ceo",
) -> dict[str, Any]:
    """Run a real VQE potential-energy scan over a caller-stated grid.

    Supported molecules are H2 (the original hardware-efficient path) and
    LiH/BeH2 through the same molecular VQE rail.  ``full_space`` selects the
    ADAPT full-orbital-space path; active-space LiH/BeH2 remains the default.
    Every point carries its own measured energy, uncertainty, FCI anchor, and
    delta; a failed point refuses the scan instead of returning a partial
    curve as if it were complete.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing PES"}
    mol = (molecule or "").upper()
    if mol not in ("H2", "LIH", "BEH2", "H2O"):
        return {"success": False, "error": f"PES supports H2, LiH, BeH2, H2O; refusing {molecule!r}"}
    try:
        rmin, rmax, n = float(r_min), float(r_max), int(steps)
    except (TypeError, ValueError):
        return {"success": False, "error": "r_min/r_max/steps must be numeric"}
    if not (0.3 <= rmin < rmax <= 10.0):
        return {"success": False, "error": "require 0.3 <= r_min < r_max <= 10.0: refusing"}
    if n < 2 or n > 21:
        return {"success": False, "error": "steps must be in [2,21]: refusing"}
    if bool(full_space) and mol == "H2":
        return {"success": False, "error": "H2 has no active-space reduction; full_space is for LiH/BeH2/H2O"}
    t0 = time.perf_counter()
    points = []
    for i in range(n):
        r = rmin + (rmax - rmin) * i / (n - 1)
        res = run_vqe(molecule=mol, bond_distance=r, layers=layers,
                      shots=shots, seed=seed + i, max_iterations=max_iterations,
                      full_space=bool(full_space),
                      full_space_pool=full_space_pool)
        if not res.get("success"):
            return {"success": False, "error": f"PES point {r:.4f} failed: {res.get('error')}",
                    "failed_point": r, "completed_points": points}
        points.append({
            "bond_distance": r,
            "energy": res["energy"],
            "energy_exact": res["energy_exact"],
            "uncertainty": res["uncertainty"],
            "ci95_mha": res.get("ci95_mha", _ci95_mha(res["uncertainty"])),
            "fci_exact_energy": res["fci_exact_energy"],
            "active_space_fci_energy": res.get("active_space_fci_energy"),
            "rhf_mean_field_energy": res.get("rhf_mean_field_energy"),
            "delta_mha": res["delta_mha"],
            "exact_delta_mha": res.get("exact_delta_mha"),
            "chemical_accuracy": res["chemical_accuracy"],
            "claimable": res.get("claimable", res["chemical_accuracy"]),
            "full_space_status": res.get("full_space_status"),
            "method": res.get("method"),
            "theta": res.get("theta", []),
            "ansatz_qasm": res.get("ansatz_qasm", ""),
            "pauli_coefficients": res.get("pauli_coefficients", {}),
            "constant_energy": res.get("constant_energy", 0.0),
        })
    return {
        "success": True,
        "kind": "pes",
        "result_version": "experiment-result-v1",
        "molecule": res.get("molecule", mol),
        "basis": "sto-3g",
        "r_min": rmin,
        "r_max": rmax,
        "steps": n,
        "full_space": bool(full_space),
        "points": points,
        "backend": "qiskit-aer-simulator",
        "measured": True,
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def run_excited_state(
    molecule: str = "H2",
    bond_distance: float = 0.7414,
    layers: int = 2,
    shots: int = 16384,
    seed: int = 7,
    max_iterations: int = 80,
    penalty: float = 10.0,
) -> dict[str, Any]:
    """Deflation-based VQE for the first excited state of H2/STO-3G.

    The ground state is optimized first; the excited objective adds a
    positive overlap penalty against that state. Both reported energies are
    shot-measured Pauli expectations; the exact 4x4 spectrum is the anchor.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing excited-state VQE"}
    mol = str(molecule or "").upper()
    if mol != "H2":
        return run_nature_excited_state(mol, float(bond_distance), shots=shots, seed=seed,
                                        max_iterations=max_iterations, penalty=penalty)
    r = float(bond_distance)
    if not (0.3 <= r <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]"}
    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if max_iterations < 1 or max_iterations > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
    if not math.isfinite(float(penalty)) or float(penalty) <= 0:
        return {"success": False, "error": "penalty must be finite and positive"}
    t0 = time.perf_counter()
    h4, fci, rhf = h2_hamiltonian(r)
    coeffs = pauli_coefficients(h4)
    spectrum = [float(x) for x in np.linalg.eigvalsh(h4)]
    if len(spectrum) < 2:
        return {"success": False, "error": "H2 spectrum has no excited state"}
    from scipy.optimize import minimize
    rng = np.random.default_rng(int(seed))
    x0 = rng.uniform(-0.2, 0.2, size=4 * int(layers))

    def exact_energy(x: np.ndarray) -> float:
        return _energy_exact(np.asarray(x, dtype=float), h4, int(layers))

    ground_opt = minimize(
        exact_energy, x0, method="COBYLA",
        options={"maxiter": int(max_iterations), "rhobeg": 0.35, "tol": 1e-9},
    )
    ground_theta = np.asarray(ground_opt.x if ground_opt.x is not None else x0, dtype=float)
    ground_psi = Statevector.from_instruction(_ansatz(ground_theta, int(layers))).data

    def excited_objective(x: np.ndarray) -> float:
        psi = Statevector.from_instruction(_ansatz(np.asarray(x, dtype=float), int(layers))).data
        energy = float(np.real(np.vdot(psi, h4 @ psi)))
        overlap = float(abs(np.vdot(ground_psi, psi)) ** 2)
        return energy + float(penalty) * overlap

    excited_opt = minimize(
        excited_objective, rng.uniform(-0.2, 0.2, size=4 * int(layers)),
        method="COBYLA",
        options={"maxiter": int(max_iterations), "rhobeg": 0.35, "tol": 1e-9},
    )
    excited_theta = np.asarray(excited_opt.x if excited_opt.x is not None else x0, dtype=float)
    excited_psi = Statevector.from_instruction(_ansatz(excited_theta, int(layers))).data
    overlap = float(abs(np.vdot(ground_psi, excited_psi)) ** 2)
    ground_exact_ansatz = float(np.real(np.vdot(ground_psi, h4 @ ground_psi)))
    excited_exact_ansatz = float(np.real(np.vdot(excited_psi, h4 @ excited_psi)))
    ground_measured = _shot_energy(ground_theta, coeffs, int(layers), int(shots), int(seed))
    excited_measured = _shot_energy(excited_theta, coeffs, int(layers), int(shots), int(seed) + 1)
    gap_measured = float(excited_measured["energy"] - ground_measured["energy"])
    gap_exact = float(spectrum[1] - spectrum[0])
    uncertainty = float(math.sqrt(
        float(ground_measured["uncertainty"]) ** 2
        + float(excited_measured["uncertainty"]) ** 2
    ))
    claimable = bool(
        _claimable_against_anchor(ground_measured["energy"], spectrum[0], ground_measured["uncertainty"])
        and _claimable_against_anchor(excited_measured["energy"], spectrum[1], excited_measured["uncertainty"])
        and gap_measured > 0
    )
    return {
        "success": True,
        "kind": "excited_state",
        "result_version": "experiment-result-v1",
        "molecule": "H2",
        "bond_distance": r,
        "basis": "sto-3g",
        "method": "Deflation VQE, hardware-efficient ansatz, COBYLA",
        "ansatz": f"hardware-efficient layers={int(layers)}",
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "penalty": float(penalty),
        "ground_theta": [float(v) for v in ground_theta],
        "excited_theta": [float(v) for v in excited_theta],
        "ground_energy": float(ground_measured["energy"]),
        "ground_energy_exact": float(spectrum[0]),
        "ground_energy_optimizer_exact": float(ground_exact_ansatz),
        "excited_energy": float(excited_measured["energy"]),
        "excited_energy_exact": float(spectrum[1]),
        "excited_energy_optimizer_exact": float(excited_exact_ansatz),
        "gap": float(gap_measured),
        "gap_exact": gap_exact,
        "uncertainty": uncertainty,
        "ci95_mha": _ci95_mha(uncertainty),
        "overlap_squared": overlap,
        "spectrum": spectrum,
        "measurements": {"ground": ground_measured["terms"], "excited": excited_measured["terms"]},
        "fci_exact_energy": float(fci),
        "rhf_mean_field_energy": float(rhf),
        "claimable": claimable,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "optimizer": {
            "ground": {"name": "COBYLA", "success": bool(ground_opt.success),
                       "evaluations": int(ground_opt.nfev), "message": str(ground_opt.message)},
            "excited": {"name": "COBYLA", "success": bool(excited_opt.success),
                        "evaluations": int(excited_opt.nfev), "message": str(excited_opt.message)},
        },
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "limitations": "H2/STO-3G first excited state; deflation overlap penalty; no hardware run.",
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def _oxygen_atom_fci() -> float:
    from pyscf import gto, scf, fci
    mol = gto.M(atom="O 0 0 0", basis="sto-3g", charge=0, spin=2, verbose=0)
    mf = scf.ROHF(mol).run()
    e, _ = fci.FCI(mf).kernel()
    return float(e)


def _run_water_reaction(reactant_distance: float, shots: int, seed: int,
                        max_iterations: int, layers: int = 2) -> dict[str, Any]:
    """H2O -> H2 + O with a measured H2O/H2 VQE pair and exact atom anchors.

    The primary reaction delta is measured against the active-space water
    anchor (the model the water VQE actually optimized), with the full-molecule
    FCI delta retained separately. The O atom is an exact FCI anchor; the H2
    endpoint is measured. A reaction claim is therefore explicitly scoped to
    that mixed active-space/exact-atom model, never presented as a full-molecule
    thermochemical claim.
    """
    r = float(reactant_distance)
    if not (0.3 <= r <= 10.0):
        return {"success": False, "error": f"bond_distance {reactant_distance!r} outside [0.3,10.0]"}
    if int(layers) < 1 or int(layers) > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    spec = _NATURE_ACTIVE["H2O"]
    atom = _molecule_atom(spec, r)
    e_h2o_full, e_h2o_active = _full_fci_and_casci(atom, 4, 4)
    _, e_h2, _ = h2_hamiltonian(0.7414)
    e_o = _oxygen_atom_fci()
    water = run_vqe("H2O", r, int(layers), shots, seed, max_iterations)
    hydrogen = run_vqe("H2", 0.7414, int(layers), shots, seed + 1, max_iterations)
    if not water.get("success") or not hydrogen.get("success"):
        return {"success": False, "error": "one water-reaction endpoint failed",
                "water_error": water.get("error"), "hydrogen_error": hydrogen.get("error")}
    measured = float(water["energy"] - hydrogen["energy"] - e_o)
    exact_active = float(e_h2o_active - (e_h2 + e_o))
    exact_full = float(e_h2o_full - (e_h2 + e_o))
    uncertainty = float(math.sqrt(float(water["uncertainty"]) ** 2 + float(hydrogen["uncertainty"]) ** 2))
    water_endpoint_ok = bool(
        water.get("active_space_chemical_accuracy", water.get("claimable", False))
    )
    h2_endpoint_ok = bool(hydrogen.get("claimable", False))
    claimable = bool(
        water_endpoint_ok and h2_endpoint_ok
        and abs(measured - exact_active) <= 1.6e-3
        and uncertainty <= 1.6e-3
    )
    return {
        "success": True,
        "kind": "reaction",
        "result_version": "experiment-result-v1",
        "molecule": "H2O",
        "reactant_distance": r,
        "product_distance": 0.0,
        "reaction": "H2O -> H2 + O",
        "basis": "sto-3g",
        "method": "H2O active-space VQE + H2 VQE against exact O-atom FCI and active-space water anchor",
        "reactant_energy": float(water["energy"]),
        "product_energy": float(hydrogen["energy"] + e_o),
        "reaction_energy": measured,
        "reactant_energy_exact": float(e_h2o_active),
        "reactant_energy_exact_full": float(e_h2o_full),
        "product_energy_exact": float(e_h2 + e_o),
        "reaction_energy_exact": exact_active,
        "reaction_energy_exact_full": exact_full,
        "uncertainty": uncertainty,
        "delta": abs(measured - exact_active),
        "delta_full": abs(measured - exact_full),
        "claimable": claimable,
        "claimability_scope": "H2O active-space (ncas=4, nelecas=4) + measured H2 + exact O-atom FCI",
        "water_endpoint_claimable": water_endpoint_ok,
        "h2_endpoint_claimable": h2_endpoint_ok,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "limitations": "H2O active-space VQE + H2 VQE + exact O-atom FCI; the full-molecule FCI delta is reported separately, so this is not a full-molecule thermochemical claim.",
        "error_message": "",
    }


def run_reaction_energy(
    molecule: str = "H2",
    reactant_distance: float = 0.7414,
    product_distance: float = 10.0,
    layers: int = 2,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
) -> dict[str, Any]:
    """H2 dissociation energy: H2 -> 2 H at a stretched endpoint.

    Exact FCI energies at both geometries are the anchor. The measured
    reaction energy is the difference of two shot-measured VQE energies;
    it is explicitly non-claimable when either endpoint misses its anchor.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing reaction energy"}
    mol = str(molecule or "").upper()
    if mol not in ("H2", "H2O", "LIH", "BEH2"):
        return {"success": False, "error": "reaction energy supports H2, LiH, BeH2, H2O; refusing " + str(molecule)}
    if mol == "H2O":
        return _run_water_reaction(reactant_distance, shots, seed, max_iterations, layers)
    if mol in ("LIH", "BEH2"):
        return _run_atomization_reaction(mol, reactant_distance, layers, shots, seed, max_iterations)
    r0, r1 = float(reactant_distance), float(product_distance)
    if not (0.3 <= r0 < r1 <= 10.0):
        return {"success": False, "error": "require 0.3 <= reactant_distance < product_distance <= 10.0"}
    _, fci0, _ = h2_hamiltonian(r0)
    _, fci1, _ = h2_hamiltonian(r1)
    left = run_vqe("H2", r0, layers, shots, seed, max_iterations)
    right = run_vqe("H2", r1, layers, shots, seed + 1, max_iterations)
    if not left.get("success") or not right.get("success"):
        return {"success": False, "error": "one reaction endpoint failed",
                "reactant_error": left.get("error"), "product_error": right.get("error")}
    measured = float(right["energy"] - left["energy"])
    exact = float(fci1 - fci0)
    uncertainty = float(math.sqrt(float(left["uncertainty"]) ** 2 + float(right["uncertainty"]) ** 2))
    claimable = bool(
        left.get("claimable", False)
        and right.get("claimable", False)
        and abs(measured - exact) <= 1.6e-3
        and uncertainty <= 1.6e-3
    )
    return {
        "success": True,
        "kind": "reaction",
        "result_version": "experiment-result-v1",
        "molecule": "H2",
        "reactant_distance": r0,
        "product_distance": r1,
        "reaction": "H2 -> 2 H",
        "basis": "sto-3g",
        "method": "Two-endpoint VQE difference against FCI endpoints",
        "reactant_energy": float(left["energy"]),
        "product_energy": float(right["energy"]),
        "reaction_energy": measured,
        "reactant_energy_exact": float(fci0),
        "product_energy_exact": float(fci1),
        "reaction_energy_exact": exact,
        "uncertainty": uncertainty,
        "delta": abs(measured - exact),
        "claimable": claimable,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "shots": int(shots),
        "seed": int(seed),
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "limitations": "H2/STO-3G endpoint difference only; no geometry relaxation beyond the two stated points.",
        "error_message": "",
    }


def run_nature_excited_state(
    molecule: str,
    bond_distance: float = 1.5957,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
    penalty: float = 10.0,
) -> dict[str, Any]:
    """Active-space deflation VQE excited state for LiH/BeH2/H2O.

    Exact active-space eigenvalues are the anchor. Full-molecule FCI and
    CASCI are reported separately; the excited-state claim is explicitly
    active-space-only.
    """
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing molecular excited state"}
    mol = (molecule or "").upper()
    spec = _NATURE_ACTIVE.get(mol)
    if spec is None or mol == "H2":
        return {"success": False, "error": f"molecular excited state supports {sorted(k for k in _NATURE_ACTIVE if k != 'H2')}"}
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if max_iterations < 1 or max_iterations > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
    if not math.isfinite(float(penalty)) or float(penalty) <= 0:
        return {"success": False, "error": "penalty must be finite and positive"}
    t0 = time.perf_counter()
    atom = _molecule_atom(spec, float(bond_distance))
    problem = PySCFDriver(atom=atom, basis="sto-3g", charge=0, spin=0).run()
    ncas = int(spec["ncas"])
    nelecas = int(spec["nelecas"])
    e_full, e_casci = _full_fci_and_casci(atom, ncas, nelecas)
    reduced = ActiveSpaceTransformer(num_electrons=nelecas, num_spatial_orbitals=ncas).transform(problem)
    mapper = JordanWignerMapper()
    qop = mapper.map(reduced.hamiltonian.second_q_op())
    constants = float(sum(reduced.hamiltonian.constants.values()))
    h_active = np.asarray(qop.to_matrix(), dtype=complex) + constants * np.eye(2 ** qop.num_qubits)
    # Project onto the physical (n_alpha, n_beta) particle-number sector in
    # Jordan-Wigner ordering (lower n_orb qubits = alpha, upper n_orb = beta)
    # so ionized/wrong-electron-number states do not contaminate the spectrum.
    n_orb = int(reduced.num_spatial_orbitals)
    n_alpha, n_beta = int(reduced.num_particles[0]), int(reduced.num_particles[1])
    alpha_mask = (1 << n_orb) - 1
    sector_idx = [
        b for b in range(1 << qop.num_qubits)
        if bin(b & alpha_mask).count("1") == n_alpha
        and bin(b >> n_orb).count("1") == n_beta
    ]
    if len(sector_idx) >= 2:
        h_sector = h_active[np.ix_(sector_idx, sector_idx)]
        spectrum = [float(x) for x in np.linalg.eigvalsh(h_sector)]
    else:
        spectrum = [float(x) for x in np.linalg.eigvalsh(h_active)]
    if len(spectrum) < 2:
        return {"success": False, "error": "active-space spectrum has no excited state"}
    hf = HartreeFock(reduced.num_spatial_orbitals, reduced.num_particles, mapper)
    ansatz = UCCSD(reduced.num_spatial_orbitals, reduced.num_particles, mapper, initial_state=hf)
    rng = np.random.default_rng(int(seed))

    def exact_energy(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x))
        return float(np.real(sv.expectation_value(qop)) + constants)

    from scipy.optimize import minimize
    ground_opt = minimize(exact_energy, rng.uniform(-0.15, 0.15, size=ansatz.num_parameters),
                          method="COBYLA", options={"maxiter": int(max_iterations), "rhobeg": 0.3, "tol": 1e-8})
    ground_theta = np.asarray(ground_opt.x, dtype=float)
    ground_psi = Statevector.from_instruction(ansatz.assign_parameters(ground_theta)).data

    def excited_objective(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x)).data
        energy = float(np.real(np.vdot(sv, h_active @ sv)))
        overlap = float(abs(np.vdot(ground_psi, sv)) ** 2)
        return energy + float(penalty) * overlap

    excited_opt = minimize(excited_objective, rng.uniform(-0.15, 0.15, size=ansatz.num_parameters),
                           method="COBYLA", options={"maxiter": int(max_iterations), "rhobeg": 0.3, "tol": 1e-8})
    excited_theta = np.asarray(excited_opt.x, dtype=float)
    excited_psi = Statevector.from_instruction(ansatz.assign_parameters(excited_theta)).data
    overlap = float(abs(np.vdot(ground_psi, excited_psi)) ** 2)
    ground_measured = _pauli_measurement_energy(ansatz, ground_theta, qop, int(shots), int(seed))
    excited_measured = _pauli_measurement_energy(ansatz, excited_theta, qop, int(shots), int(seed) + 1)
    ground_energy = float(ground_measured["energy"] + constants)
    excited_energy = float(excited_measured["energy"] + constants)
    gap = float(excited_energy - ground_energy)
    gap_exact = float(spectrum[1] - spectrum[0])
    uncertainty = float(math.sqrt(float(ground_measured["uncertainty"]) ** 2 + float(excited_measured["uncertainty"]) ** 2))
    claimable = bool(
        _claimable_against_anchor(ground_energy, spectrum[0], ground_measured["uncertainty"])
        and _claimable_against_anchor(excited_energy, spectrum[1], excited_measured["uncertainty"])
        and gap > 0
    )
    return {
        "success": True,
        "kind": "excited_state",
        "result_version": "experiment-result-v1",
        "molecule": spec["name"],
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": "Active-space deflation VQE (UCCSD), COBYLA",
        "ansatz": f"UCCSD active-space {ncas} orbitals",
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas, "full_space": False},
        "shots": int(shots),
        "seed": int(seed),
        "penalty": float(penalty),
        "ground_energy": ground_energy,
        "ground_energy_exact": float(spectrum[0]),
        "excited_energy": excited_energy,
        "excited_energy_exact": float(spectrum[1]),
        "gap": gap,
        "gap_exact": gap_exact,
        "uncertainty": uncertainty,
        "ci95_mha": _ci95_mha(uncertainty),
        "overlap_squared": overlap,
        "spectrum": spectrum,
        "measurements": {"ground": ground_measured["terms"], "excited": excited_measured["terms"]},
        "fci_exact_energy": float(e_full),
        "active_space_fci_energy": float(e_casci),
        "claimable": claimable,
        "active_space_claimable": claimable,
        "measured": True,
        "hardware": False,
        "simulated_optimizer": True,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "optimizer": {
            "ground": {"name": "COBYLA", "success": bool(ground_opt.success),
                       "evaluations": int(ground_opt.nfev), "message": str(ground_opt.message)},
            "excited": {"name": "COBYLA", "success": bool(excited_opt.success),
                        "evaluations": int(excited_opt.nfev), "message": str(excited_opt.message)},
        },
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "limitations": "Active-space excited-state model; full-molecule FCI is reported separately and may not equal the active-space spectrum.",
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }
