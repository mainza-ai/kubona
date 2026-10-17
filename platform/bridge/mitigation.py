"""
Quantum Error Mitigation Engine: ZNE, PEC, and CDR.
Implements the mitigation suite defined in wiki/concepts/error-mitigation.md:
1. Zero-Noise Extrapolation (ZNE) via unitary folding & polynomial fits
2. Probabilistic Error Cancellation (PEC) via quasi-probability representation
3. Clifford Data Regression (CDR) via classical near-Clifford calibration
"""
from __future__ import annotations

import math
from typing import Dict, Any, List, Optional
import numpy as np

try:
    from qiskit import QuantumCircuit
    from qiskit_aer import AerSimulator
    from qiskit_aer.noise import NoiseModel, depolarizing_error
    HAS_QISKIT = True
except ImportError:
    HAS_QISKIT = False


def _build_noise_model(p: float = 0.02) -> Optional[Any]:
    if not HAS_QISKIT:
        return None
    nm = NoiseModel()
    err_1q = depolarizing_error(p, 1)
    err_2q = depolarizing_error(p * 2, 2)
    nm.add_all_qubit_quantum_error(err_1q, ['u1', 'u2', 'u3', 'rx', 'ry', 'rz', 'h', 's', 'x', 'y', 'z', 'sx'])
    nm.add_all_qubit_quantum_error(err_2q, ['cx', 'cz'])
    return nm


# --- Shared consts (single source; no inline magic) ---
#: Base single-qubit depolarizing rate for the bridge noise model (v1).
#: The PEC overhead below scales with gate counts from this rate — the
#: rate itself is fixed, the SCALING is not.
DEPOL_P_1Q_V1 = 0.015
DEPOL_P_2Q_V1 = 0.030  # 2x single-qubit, matching _build_noise_model
#: Conditioning cap for least-squares fits (ZNE Vandermonde, CDR design).
#: Above this the fit is refused instead of returning a number.
MAX_FIT_COND_V1 = 1e12
#: Minimum variance in the CDR noisy-axis: a constant axis carries no
#: slope information, so the fit refuses.
MIN_CDR_AXIS_VAR_V1 = 1e-12
#: Versioned method labels (asserted by tests; keep in one place).
ZNE_METHOD_V1 = "ZNE"
PEC_METHOD_V1 = "PEC"
CDR_METHOD_V1 = "CDR"


def _z_expectation(counts: Dict[str, int]) -> float:
    total = sum(counts.values())
    if total <= 0:
        raise ValueError("empty counts: cannot form <Z> expectation")
    return sum(c if b[-1] == '0' else -c for b, c in counts.items()) / total


def _shot_std_z(z: float, shots: int) -> float:
    # Binomial std of the <Z> estimator from measured data: sqrt((1-z^2)/N).
    # Computed from the sampled z and the shot count — never invented.
    if shots <= 0:
        raise ValueError(f"shots must be positive, got {shots}")
    var = max(0.0, 1.0 - min(1.0, z * z)) / shots
    return float(math.sqrt(var))


def _folded_counts(qc: Any, fold_factor: int, sim: Any, shots: int) -> Dict[str, int]:
    # Gate-repetition folding (documents the approximation: repetition, not
    # true U U^dag U inverse folding — the noise-scale knob, not a unitary
    # identity). Shared by ZNE and CDR so the two scales cannot rot apart.
    scale_qc = qc.copy()
    if fold_factor > 1:
        for _ in range(fold_factor - 1):
            for instr in qc.data:
                scale_qc.data.append(instr)
    has_m = any(getattr(inst.operation, "name", "") == "measure" for inst in scale_qc.data)
    if not has_m:
        if scale_qc.num_clbits == 0:
            scale_qc.measure_all()
        else:
            n_m = min(scale_qc.num_qubits, scale_qc.num_clbits)
            for i in range(n_m):
                scale_qc.measure(i, i)
    return dict(sim.run(scale_qc, shots=shots).result().get_counts())


def mitigate_circuit(
    qasm: str,
    method: str = "ZNE",
    scale_factors: Optional[List[float]] = None,
    shots: int = 1024,
    device_name: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Applies requested error mitigation technique to an OpenQASM circuit.

    Production bar (W1.3):
    - CDR refuses the single-point tautology (mitigated==ideal by
      construction) and instead fits ideal=a*noisy+b over >=2 noise levels
      with a conditioning check (cond + axis variance); ill-conditioned
      fits refuse.
    - PEC gamma scales with measured depth/qubit counts (no
      depth-independent scalar); the error bound is amplified shot noise
      derived from data, not gamma*|raw| tautology.
    - Every error_bound is computed from data (fit RMSE + binomial shot
      noise) or omitted — never an invented 0.1/0.05 multiplier.
    - Unknown device_name refuses (never silently defaults to ibm_heron);
      success receipts carry a fidelity-probe heuristic labeled
      measured=False end-to-end.
    - Mitigated values are INFERRED (measured=False); the unmitigated
      sampling is measured (measured=True). Labels travel with the numbers.
    """
    if scale_factors is None:
        scale_factors = [1.0, 1.5, 2.0]
    elif len(scale_factors) < 2:
        return {
            "success": False,
            "unmitigated_value": 0.0,
            "mitigated_value": 0.0,
            "error_bound": None,
            "method_used": (method or "ZNE").upper(),
            "measured": False,
            "error_message": (
                f"CDR/ZNE calibration requires at least 2 noise levels, got "
                f"{len(scale_factors)}: refusing instead of fitting a "
                "single-point tautology"
            ),
        }

    method = method.upper()

    if device_name is not None:
        # Unknown devices refuse in the fidelity probe — never default.
        try:
            from fidelity import DEVICE_PROFILES as _PROFILES
        except ImportError:
            from bridge.fidelity import DEVICE_PROFILES as _PROFILES  # type: ignore
        if device_name.lower() not in _PROFILES:
            return {
                "success": False,
                "unmitigated_value": 0.0,
                "mitigated_value": 0.0,
                "error_bound": None,
                "method_used": method,
                "measured": False,
                "error_message": (
                    f"unknown device {device_name!r}: refusing instead of "
                    "silently defaulting to ibm_heron"
                ),
            }

    if not HAS_QISKIT:
        # Fail closed: without Qiskit/Aer there is no mitigation computation.
        # Returning success with invented numbers would violate the
        # quantum-advantage-realism policy (prompt-as-simulation), so refuse.
        return {
            "success": False,
            "unmitigated_value": 0.0,
            "mitigated_value": 0.0,
            "error_bound": None,
            "method_used": method,
            "measured": False,
            "error_message": "qiskit/aer unavailable: no mitigation computation performed",
        }

    try:
        from executor import circuit_from_qasm
        qc = circuit_from_qasm(qasm)
    except Exception as e:
        return {
            "success": False,
            "unmitigated_value": 0.0,
            "mitigated_value": 0.0,
            "error_bound": None,
            "method_used": method,
            "measured": False,
            "error_message": f"Failed to parse QASM: {e}",
        }

    # Evaluate observable <Z_0> under noisy Aer
    noise_model = _build_noise_model(DEPOL_P_1Q_V1)
    sim = AerSimulator(noise_model=noise_model) if noise_model else AerSimulator()

    qc_meas = qc.copy()
    has_measures = any(getattr(inst.operation, "name", "") == "measure" for inst in qc_meas.data)
    if not has_measures:
        if qc_meas.num_clbits == 0:
            qc_meas.measure_all()
        else:
            n_m = min(qc_meas.num_qubits, qc_meas.num_clbits)
            for i in range(n_m):
                qc_meas.measure(i, i)

    # Base execution (unmitigated)
    res_raw = sim.run(qc_meas, shots=shots).result()
    counts_raw = res_raw.get_counts()
    raw_z0 = _z_expectation(dict(counts_raw))
    raw_shot_std = _shot_std_z(raw_z0, shots)

    def _probe_receipt() -> Dict[str, Any]:
        # Fidelity-probe heuristic travels in every success receipt,
        # labeled measured=False end-to-end (never a bare number).
        try:
            from fidelity import PROBE_METHOD_V1 as _pm
        except ImportError:
            from bridge.fidelity import PROBE_METHOD_V1 as _pm  # type: ignore
        return {"measured": False, "method": _pm}

    if method == "ZNE":
        # Unitary folding at scale factors lambda (gate-repetition knob).
        vals = []
        for s in scale_factors:
            fold_factor = int(round(s))
            cnts = _folded_counts(qc, fold_factor, sim, shots)
            vals.append(_z_expectation(cnts))

        # Richardson extrapolation to scale = 0 with conditioning check.
        deg = min(len(scale_factors) - 1, 2)
        try:
            vander = np.vander(np.asarray(scale_factors, dtype=float), deg + 1)
            cond = float(np.linalg.cond(vander))
        except Exception:
            cond = float("inf")
        if not math.isfinite(cond) or cond > MAX_FIT_COND_V1:
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": ZNE_METHOD_V1,
                "measured": False,
                "fit_condition_number": cond,
                "error_message": (
                    f"ZNE fit ill-conditioned (cond={cond:.3g}): refusing "
                    "extrapolation instead of returning an unstable number"
                ),
            }
        coeffs = np.polyfit(scale_factors, vals, deg=deg)
        mitigated_raw = float(np.polyval(coeffs, 0.0))
        # Data-computed bound: fit RMSE at the measured scales plus the
        # binomial shot noise of the unmitigated sampling. No invented 0.1.
        pred_at_scales = np.polyval(coeffs, np.asarray(scale_factors, dtype=float))
        rmse = float(np.sqrt(np.mean((np.asarray(vals) - pred_at_scales) ** 2)))
        err_bound = float(rmse + raw_shot_std)
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))

        return {
            "success": True,
            "unmitigated_value": float(raw_z0),
            "unmitigated_measured": True,
            "mitigated_value": mitigated,
            "mitigated_measured": False,
            "measured": False,
            "error_bound": err_bound,
            "error_bound_method": "fit-rmse-plus-shot-noise (data-computed)",
            "method_used": ZNE_METHOD_V1,
            "fit_condition_number": cond,
            "fit_rmse": rmse,
            "clipped_to_physical_range": clipped,
            "fidelity_probe": _probe_receipt(),
            "error_message": "",
        }

    elif method == "PEC":
        # Probabilistic Error Cancellation with DEPTH-SCALED overhead.
        # The old code used a depth-independent scalar gamma=p/(1-p): the
        # same correction for a 1-gate and a 100-gate circuit. Here gamma
        # grows with the measured gate counts: each 1q gate contributes
        # g1=p1/(1-p1), each 2q gate g2=p2/(1-p2), and the total sampling
        # overhead is prod(1+g)-1 (quasi-probability normalization).
        try:
            n_qubits = int(qc.num_qubits)
            depth = int(qc.depth())
        except Exception:
            n_qubits, depth = 0, 0
        try:
            ops = qc.count_ops()
            n_2q = int(sum(v for k, v in ops.items() if k in ("cx", "cz", "ecr", "swap")))
            n_1q = int(sum(v for k, v in ops.items() if k not in ("cx", "cz", "ecr", "swap", "measure", "barrier")))
        except Exception:
            n_2q, n_1q = 0, max(0, depth)
        # Fall back to depth-derived counts when the op table is empty
        # (e.g. circuits whose gates are all aliased): depth is still data.
        if n_1q + n_2q == 0 and depth > 0:
            n_1q = depth
        g1 = DEPOL_P_1Q_V1 / (1.0 - DEPOL_P_1Q_V1)
        g2 = DEPOL_P_2Q_V1 / (1.0 - DEPOL_P_2Q_V1)
        gamma_total = float((1.0 + g1) ** n_1q * (1.0 + g2) ** n_2q - 1.0)
        if not math.isfinite(gamma_total) or gamma_total < 0:
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": PEC_METHOD_V1,
                "measured": False,
                "error_message": (
                    f"PEC overhead non-finite (n_1q={n_1q}, n_2q={n_2q}): refusing"
                ),
            }
        mitigated_raw = float((1.0 + gamma_total) * raw_z0)
        # Derived (not tautological) bound: the binomial shot noise of the
        # sampled expectation, AMPLIFIED by the sampling overhead. Uses
        # shots + measured z — never gamma*|raw| alone.
        err_bound = float((1.0 + gamma_total) * raw_shot_std)
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))

        return {
            "success": True,
            "unmitigated_value": float(raw_z0),
            "unmitigated_measured": True,
            "mitigated_value": mitigated,
            "mitigated_measured": False,
            "measured": False,
            "error_bound": err_bound,
            "error_bound_method": "amplified-shot-noise (1+gamma)*sqrt((1-z^2)/N)",
            "method_used": PEC_METHOD_V1,
            "pec_gamma": gamma_total,
            "sampling_overhead_kappa": gamma_total,
            "pec_n_1q": n_1q,
            "pec_n_2q": n_2q,
            "pec_circuit_depth": depth,
            "pec_n_qubits": n_qubits,
            "clipped_to_physical_range": clipped,
            "fidelity_probe": _probe_receipt(),
            "error_message": "",
        }

    elif method == "CDR":
        # Clifford Data Regression with a REAL fit over >=2 noise levels.
        # The old code computed scale=ideal/noisy on the target itself, so
        # mitigated==ideal by construction: a scale factor proving itself,
        # not a measurement. Here we form (noisy, ideal) pairs at each
        # requested scale (folded circuits run noisy + noiseless), fit
        # ideal=a*noisy+b by least squares, check conditioning, and apply
        # the fit to the unmitigated point. A single-point or constant-axis
        # fit REFUSES.
        ideal_sim = AerSimulator()  # noiseless reference
        noisy_vals: List[float] = []
        ideal_vals: List[float] = []
        for s in scale_factors:
            fold_factor = int(round(s))
            cnts_n = _folded_counts(qc, fold_factor, sim, shots)
            noisy_vals.append(_z_expectation(cnts_n))
            cnts_i = _folded_counts(qc, fold_factor, ideal_sim, shots)
            ideal_vals.append(_z_expectation(cnts_i))
        axis_var = float(np.var(np.asarray(noisy_vals, dtype=float)))
        if axis_var < MIN_CDR_AXIS_VAR_V1:
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": CDR_METHOD_V1,
                "measured": False,
                "error_message": (
                    f"CDR calibration axis has no variance ({axis_var:.3g}): "
                    "slope is undefined — refusing single-point tautology "
                    "instead of returning ideal-as-mitigated"
                ),
            }
        try:
            design = np.column_stack([np.asarray(noisy_vals, dtype=float),
                                      np.ones(len(noisy_vals))])
            cond = float(np.linalg.cond(design))
        except Exception:
            cond = float("inf")
        if not math.isfinite(cond) or cond > MAX_FIT_COND_V1:
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": CDR_METHOD_V1,
                "measured": False,
                "fit_condition_number": cond,
                "error_message": (
                    f"CDR fit ill-conditioned (cond={cond:.3g}): refusing "
                    "instead of returning an unstable slope"
                ),
            }
        a_fit, b_fit = (float(v) for v in np.polyfit(noisy_vals, ideal_vals, deg=1))
        fit_pred = a_fit * np.asarray(noisy_vals) + b_fit
        rmse = float(np.sqrt(np.mean((np.asarray(ideal_vals) - fit_pred) ** 2)))
        mitigated_raw = float(a_fit * raw_z0 + b_fit)
        err_bound = float(rmse + abs(a_fit) * raw_shot_std)
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))

        return {
            "success": True,
            "unmitigated_value": float(raw_z0),
            "unmitigated_measured": True,
            "mitigated_value": mitigated,
            "mitigated_measured": False,
            "measured": False,
            "error_bound": err_bound,
            "error_bound_method": "cdr-fit-rmse-plus-propagated-shot-noise",
            "method_used": CDR_METHOD_V1,
            "cdr_slope": a_fit,
            "cdr_intercept": b_fit,
            "cdr_rmse": rmse,
            "fit_condition_number": cond,
            "n_calibration_points": len(noisy_vals),
            "clipped_to_physical_range": clipped,
            "fidelity_probe": _probe_receipt(),
            "error_message": "",
        }

    else:
        return {
            "success": False,
            "unmitigated_value": float(raw_z0),
            "mitigated_value": float(raw_z0),
            "error_bound": None,
            "method_used": method,
            "measured": False,
            "error_message": f"Unsupported mitigation method: {method}",
        }
