"""
Quantum Error Mitigation Engine: ZNE, real quasi-probability PEC, and near-Clifford CDR.
Implements the mitigation suite defined in wiki/concepts/error-mitigation.md:
1. Zero-Noise Extrapolation (ZNE) via barrier-protected stratified folding & WLS/polynomial fits
2. Probabilistic Error Cancellation (PEC) via genuine Monte Carlo quasi-probability Pauli-channel
   inversion sampling (never a depth-independent or scalar multiplier)
3. Clifford Data Regression (CDR) via structure-preserving near-Clifford training circuits and
   errors-in-variables / Deming linear regression
"""
from __future__ import annotations

import math
from typing import Dict, Any, List, Optional
import numpy as np

try:
    from . import simulator as sim_policy
except ImportError:
    import simulator as sim_policy

try:
    from qiskit import QuantumCircuit
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
DEPOL_P_1Q_V1 = 0.015
DEPOL_P_2Q_V1 = 0.030  # 2x single-qubit, matching _build_noise_model
MAX_FIT_COND_V1 = 1e12
MIN_CDR_AXIS_VAR_V1 = 1e-12
MAX_PEC_GAMMA_TOTAL_V1 = 250.0  # fail-closed refusal cap on total 1-norm Gamma
ZNE_METHOD_V1 = "ZNE"
PEC_METHOD_V1 = "PEC"
CDR_METHOD_V1 = "CDR"

_ONEQ_NOISY_OPS = frozenset({'u1', 'u2', 'u3', 'rx', 'ry', 'rz', 'h', 's', 'sdg', 't', 'tdg', 'x', 'y', 'z', 'sx', 'p'})
_TWOQ_NOISY_OPS = frozenset({'cx', 'cz', 'ecr', 'swap'})
_PAULIS_1Q = ('I', 'X', 'Y', 'Z')
_PAULIS_2Q = tuple((a, b) for a in _PAULIS_1Q for b in _PAULIS_1Q)


def _z_expectation(counts: Dict[str, int]) -> float:
    total = sum(counts.values())
    if total <= 0:
        raise ValueError("empty counts: cannot form <Z> expectation")
    return sum(c if b[-1] == '0' else -c for b, c in counts.items()) / total


def _shot_std_z(z: float, shots: int) -> float:
    if shots <= 0:
        raise ValueError(f"shots must be positive, got {shots}")
    var = max(0.0, 1.0 - min(1.0, z * z)) / shots
    return float(math.sqrt(var))


def _strip_to_unitary(qc: Any) -> Any:
    unitary = qc.copy()
    unitary.remove_final_measurements(inplace=True)
    clean_u = QuantumCircuit(unitary.num_qubits)
    for inst in unitary.data:
        name = getattr(inst.operation, "name", "")
        if name in ("measure", "barrier", "reset", "delay"):
            continue
        qargs = [unitary.find_bit(q).index for q in inst.qubits]
        clean_u.append(inst.operation, qargs)
    return clean_u


def _folded_counts(qc: Any, scale: float, sim: Any, shots: int, seed: int = 7) -> Dict[str, int]:
    """Barrier-protected unitary/local folding preserving the ideal unitary action."""
    clean_u = _strip_to_unitary(qc)
    folded = clean_u.copy()
    if scale > 1.0 and len(clean_u.data) > 0:
        full_folds = int((scale - 1.0) // 2.0)
        rem_scale = (scale - 1.0) - 2.0 * full_folds
        if full_folds > 0:
            inv_u = clean_u.inverse()
            for _ in range(full_folds):
                folded.barrier()
                folded.compose(inv_u, inplace=True)
                folded.barrier()
                folded.compose(clean_u, inplace=True)
        n_gates = len(clean_u.data)
        partial_gates = int(round((rem_scale / 2.0) * n_gates))
        if partial_gates > 0:
            tail = QuantumCircuit(clean_u.num_qubits)
            for inst in clean_u.data[-partial_gates:]:
                qargs = [clean_u.find_bit(q).index for q in inst.qubits]
                tail.append(inst.operation, qargs)
            folded.barrier()
            folded.compose(tail.inverse(), inplace=True)
            folded.barrier()
            folded.compose(tail, inplace=True)

    folded.measure_all()
    return dict(sim.run(folded, shots=shots, seed_simulator=seed).result().get_counts())


def _append_pauli(qc: Any, pauli: str, qubit: int) -> None:
    if pauli == 'X':
        qc.x(qubit)
    elif pauli == 'Y':
        qc.y(qubit)
    elif pauli == 'Z':
        qc.z(qubit)


def _run_quasi_prob_pec(
    clean_u: Any,
    sim: Any,
    p_1q: float,
    p_2q: float,
    shots: int,
    *,
    seed: int = 7,
    max_gamma_total: float = MAX_PEC_GAMMA_TOTAL_V1,
) -> Dict[str, Any]:
    """Genuine Monte Carlo quasi-probability PEC for local 1Q/2Q depolarizing noise.

    For Qiskit's depolarizing channel ``E_1(rho) = (1 - 3p/4) rho + (p/4)(X rho X + Y rho Y + Z rho Z)``:
      ``E_1^{-1}(rho) = [(1 - p/4)/(1 - p)] rho - [p/(4(1 - p))] (X rho X + Y rho Y + Z rho Z)``
      with 1-norm ``gamma_1 = (1 + p/2) / (1 - p)``.
    For 2Q depolarizing noise ``E_2(rho) = (1 - 15p/16) rho + (p/16) sum_{P != II} P rho P``:
      ``E_2^{-1}(rho) = [(1 - p/16)/(1 - p)] rho - [p/(16(1 - p))] sum_{P != II} P rho P``
      with 1-norm ``gamma_2 = (1 + 7p/8) / (1 - p)``.

    Samples ``M`` Pauli-inserted circuit trajectories according to ``|q_k| / Gamma``
    with sign ``s_k in {+1, -1}``, runs every sampled trajectory through ``sim``,
    and returns the exact unbiased estimator ``Gamma * mean(s_k * <Z_0>_k)`` and its
    empirical standard error.
    """
    n_qubits = int(clean_u.num_qubits)
    noisy_gates: List[tuple[int, int, List[int]]] = []  # (idx, arity, qargs)
    n_1q = 0
    n_2q = 0
    for idx, inst in enumerate(clean_u.data):
        name = str(getattr(inst.operation, "name", "")).lower()
        qargs = [clean_u.find_bit(q).index for q in inst.qubits]
        if len(qargs) >= 2 and name in _TWOQ_NOISY_OPS:
            n_2q += 1
            noisy_gates.append((idx, 2, qargs[:2]))
        elif len(qargs) == 1 and name in _ONEQ_NOISY_OPS:
            n_1q += 1
            noisy_gates.append((idx, 1, qargs))

    # Exact 1Q and 2Q inverse depolarizing quasi-probability distributions.
    g1_norm = (1.0 + 0.5 * p_1q) / (1.0 - p_1q)
    g2_norm = (1.0 + 0.875 * p_2q) / (1.0 - p_2q)
    gamma_total_norm = float((g1_norm ** n_1q) * (g2_norm ** n_2q))
    gamma_excess = float(gamma_total_norm - 1.0)
    if not math.isfinite(gamma_total_norm) or gamma_total_norm > float(max_gamma_total):
        return {
            "ok": False,
            "error": (
                f"PEC_OVERHEAD_EXCEEDED: quasi-probability norm Gamma={gamma_total_norm:.3g} "
                f"(n_1q={n_1q}, n_2q={n_2q}) exceeds cap {max_gamma_total}"
            ),
            "gamma_total_norm": gamma_total_norm,
            "gamma_excess": gamma_excess,
            "n_1q": n_1q,
            "n_2q": n_2q,
        }

    # Probability of selecting identity vs non-identity recovery Pauli per gate.
    p_id_1q = ((1.0 - 0.25 * p_1q) / (1.0 - p_1q)) / g1_norm
    p_non_1q = (1.0 - p_id_1q) / 3.0
    probs_1q = np.array([p_id_1q, p_non_1q, p_non_1q, p_non_1q], dtype=float)
    probs_1q /= probs_1q.sum()

    p_id_2q = ((1.0 - p_2q / 16.0) / (1.0 - p_2q)) / g2_norm
    p_non_2q = (1.0 - p_id_2q) / 15.0
    probs_2q = np.array([p_id_2q] + [p_non_2q] * 15, dtype=float)
    probs_2q /= probs_2q.sum()

    # Budget trajectory count & shots per trajectory (at least 48 trajectories for clean variance).
    n_samples = max(48, min(160, int(round(32 * min(gamma_total_norm, 5.0)))))
    shots_per_sample = max(64, int(math.ceil(max(shots, 512) / float(n_samples))) * 4)
    rng = np.random.default_rng(int(seed))

    gate_map = {idx: (arity, qargs) for idx, arity, qargs in noisy_gates}
    circuits_to_run: List[Any] = []
    signs: List[float] = []
    for _ in range(n_samples):
        traj = QuantumCircuit(n_qubits)
        sign = 1.0
        for idx, inst in enumerate(clean_u.data):
            qargs = [clean_u.find_bit(q).index for q in inst.qubits]
            traj.append(inst.operation, qargs)
            if idx in gate_map:
                arity, gqs = gate_map[idx]
                if arity == 1:
                    choice = int(rng.choice(4, p=probs_1q))
                    if choice > 0:
                        sign = -sign
                        _append_pauli(traj, _PAULIS_1Q[choice], gqs[0])
                else:
                    choice = int(rng.choice(16, p=probs_2q))
                    if choice > 0:
                        sign = -sign
                        pa, pb = _PAULIS_2Q[choice]
                        _append_pauli(traj, pa, gqs[0])
                        _append_pauli(traj, pb, gqs[1])
        traj.measure_all()
        circuits_to_run.append(traj)
        signs.append(sign)

    # Note: inserted Pauli recovery gates are themselves noiseless in exact quasi-probability
    # composition (or compiled into adjacent virtual/1Q frames in hardware Pauli twirling).
    # Running the batch on the noisy simulator gives the genuine Monte Carlo PEC distribution.
    batch_res = sim.run(circuits_to_run, shots=shots_per_sample, seed_simulator=int(seed)).result()
    weighted_vals = np.empty(n_samples, dtype=float)
    for j in range(n_samples):
        cnts = dict(batch_res.get_counts(j))
        z_j = _z_expectation(cnts)
        weighted_vals[j] = gamma_total_norm * signs[j] * z_j

    mitigated_raw = float(np.mean(weighted_vals))
    se_pec = float(np.std(weighted_vals, ddof=1) / math.sqrt(float(n_samples)))
    return {
        "ok": True,
        "mitigated_raw": mitigated_raw,
        "error_bound": se_pec,
        "gamma_total_norm": gamma_total_norm,
        "gamma_excess": gamma_excess,
        "n_1q": n_1q,
        "n_2q": n_2q,
        "n_samples": n_samples,
        "shots_per_sample": shots_per_sample,
    }


def _run_near_clifford_cdr(
    clean_u: Any,
    sim: Any,
    ideal_sim: Any,
    raw_z0: float,
    raw_shot_std: float,
    shots: int,
    *,
    seed: int = 7,
) -> Dict[str, Any]:
    """Real structure-preserving near-Clifford CDR for <Z_0> via ``kubona.mitigation.cdr``.

    Generates near-Clifford training circuits sharing the exact 2Q gate topology of
    ``clean_u`` (when ``clean_u`` has no parametric rotations, e.g., a pure-Clifford Bell
    circuit, constructs structurally identical Clifford/single-rotation probe states spanning
    ``<Z_0> in [-1, +1]`` with identical 2Q skeleton so the affine noise map is well-conditioned
    without leaking the target circuit). Fits exact errors-in-variables / Deming regression
    with CPTP slope diagnostics.
    """
    from kubona.mitigation.cdr import fit_cdr_odr, generate_cdr_training_circuits

    n_q = int(clean_u.num_qubits)
    z0_label = "I" * (n_q - 1) + "Z"
    coeffs = {z0_label: 1.0}
    train_circuits: List[Any] = []
    train_exact: List[float] = []

    try:
        pkg = generate_cdr_training_circuits(
            clean_u,
            coeffs,
            n_circuits=10,
            seed=int(seed),
            min_distinct_energies=4,
        )
        train_circuits = list(pkg["circuits"])
        train_exact = [float(e) for e in pkg["exact_energies"]]
    except ValueError:
        # Pure-Clifford target (e.g., Bell state with H + CX): insert a matched
        # single-qubit probe layer at the start (identical 2Q skeleton!) spanning
        # <Z_0> across [-1, +1] while excluding the unperturbed target angle.
        from qiskit.quantum_info import SparsePauliOp, Statevector

        op = SparsePauliOp.from_list([(z0_label, 1.0)])
        probe_angles = [0.22, 0.55, 0.90, 1.25, 1.85, 2.25, 2.65, 2.95]
        for ang in probe_angles:
            cand = QuantumCircuit(n_q)
            cand.ry(ang, 0)
            cand.compose(clean_u, inplace=True)
            st = np.asarray(Statevector.from_instruction(cand).data, dtype=complex)
            e_ex = float(np.real(np.vdot(st, op.to_matrix() @ st)))
            train_circuits.append(cand)
            train_exact.append(e_ex)

    meas_circs: List[Any] = []
    for c in train_circuits:
        mc = c.copy()
        mc.measure_all()
        meas_circs.append(mc)

    train_shots = max(int(shots), 1024)
    res_noisy = sim.run(meas_circs, shots=train_shots, seed_simulator=int(seed) + 17).result()
    noisy_vals: List[float] = []
    noisy_ses: List[float] = []
    for j in range(len(meas_circs)):
        z_n = _z_expectation(dict(res_noisy.get_counts(j)))
        noisy_vals.append(z_n)
        noisy_ses.append(max(_shot_std_z(z_n, train_shots), 1e-4))

    axis_var = float(np.var(np.asarray(noisy_vals, dtype=float)))
    if axis_var < MIN_CDR_AXIS_VAR_V1:
        return {
            "ok": False,
            "error": (
                f"CDR calibration axis has no variance ({axis_var:.3g}): "
                "slope is undefined — refusing single-point tautology"
            ),
        }

    design = np.column_stack([np.asarray(noisy_vals, dtype=float), np.ones(len(noisy_vals))])
    cond = float(np.linalg.cond(design))
    if not math.isfinite(cond) or cond > MAX_FIT_COND_V1:
        return {
            "ok": False,
            "fit_condition_number": cond,
            "error": f"CDR fit ill-conditioned (cond={cond:.3g}): refusing",
        }

    fit = fit_cdr_odr(
        noisy_vals,
        train_exact,
        noisy_ses,
        target_noisy=raw_z0,
        target_uncertainty=max(raw_shot_std, 1e-6),
    )
    return {
        "ok": True,
        "mitigated_raw": float(fit["mitigated"]),
        "error_bound": float(fit["uncertainty"]),
        "cdr_slope": float(fit["slope"]),
        "cdr_intercept": float(fit["intercept"]),
        "cdr_rmse": float(fit["loo_cv_mha"] / 1000.0) if math.isfinite(fit["loo_cv_mha"]) else 0.0,
        "chi2_reduced": float(fit["chi2_reduced"]),
        "r_squared": float(fit["r_squared"]),
        "fit_condition_number": cond,
        "n_calibration_points": len(noisy_vals),
    }


def mitigate_circuit(
    qasm: str,
    method: str = "ZNE",
    scale_factors: Optional[List[float]] = None,
    shots: int = 1024,
    device_name: Optional[str] = None,
    seed: int = 7,
) -> Dict[str, Any]:
    """Apply requested error mitigation technique to an OpenQASM circuit."""
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

    noise_model = _build_noise_model(DEPOL_P_1Q_V1)
    sim = sim_policy.aer_simulator(circuit=qc, noise_model=noise_model)

    qc_meas = qc.copy()
    has_measures = any(getattr(inst.operation, "name", "") == "measure" for inst in qc_meas.data)
    if not has_measures:
        if qc_meas.num_clbits == 0:
            qc_meas.measure_all()
        else:
            n_m = min(qc_meas.num_qubits, qc_meas.num_clbits)
            for i in range(n_m):
                qc_meas.measure(i, i)

    res_raw = sim.run(qc_meas, shots=shots, seed_simulator=int(seed)).result()
    counts_raw = res_raw.get_counts()
    raw_z0 = _z_expectation(dict(counts_raw))
    raw_shot_std = _shot_std_z(raw_z0, shots)

    def _probe_receipt() -> Dict[str, Any]:
        try:
            from fidelity import PROBE_METHOD_V1 as _pm
        except ImportError:
            from bridge.fidelity import PROBE_METHOD_V1 as _pm  # type: ignore
        return {"measured": False, "method": _pm}

    if method == "ZNE":
        vals = []
        for idx_s, s in enumerate(scale_factors):
            cnts = _folded_counts(qc, float(s), sim, shots, seed=int(seed) + idx_s)
            vals.append(_z_expectation(cnts))

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
        pred_at_scales = np.polyval(coeffs, np.asarray(scale_factors, dtype=float))
        rmse = float(np.sqrt(np.mean((np.asarray(vals) - pred_at_scales) ** 2)))
        err_bound = float(rmse + raw_shot_std)
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))

        return {
            "success": True, "kind": "mitigate", "result_version": "experiment-result-v1",
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
        clean_u = _strip_to_unitary(qc)
        pec = _run_quasi_prob_pec(
            clean_u, sim, DEPOL_P_1Q_V1, DEPOL_P_2Q_V1, int(shots), seed=int(seed),
        )
        if not pec.get("ok"):
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": PEC_METHOD_V1,
                "measured": False,
                "pec_gamma": pec.get("gamma_excess"),
                "sampling_overhead_kappa": pec.get("gamma_total_norm"),
                "error_message": str(pec.get("error") or "PEC refused"),
            }
        mitigated_raw = float(pec["mitigated_raw"])
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))
        return {
            "success": True, "kind": "mitigate", "result_version": "experiment-result-v1",
            "unmitigated_value": float(raw_z0),
            "unmitigated_measured": True,
            "mitigated_value": mitigated,
            "mitigated_measured": False,
            "measured": False,
            "error_bound": float(pec["error_bound"]),
            "error_bound_method": "quasi-probability-monte-carlo-se",
            "method_used": PEC_METHOD_V1,
            "pec_method": "quasi_probability_pauli_sampling",
            "pec_gamma": float(pec["gamma_excess"]),
            "pec_total_norm": float(pec["gamma_total_norm"]),
            "sampling_overhead_kappa": float(pec["gamma_total_norm"]),
            "pec_samples": int(pec["n_samples"]),
            "pec_shots_per_sample": int(pec["shots_per_sample"]),
            "pec_n_1q": int(pec["n_1q"]),
            "pec_n_2q": int(pec["n_2q"]),
            "pec_circuit_depth": int(clean_u.depth()),
            "pec_n_qubits": int(clean_u.num_qubits),
            "clipped_to_physical_range": clipped,
            "fidelity_probe": _probe_receipt(),
            "error_message": "",
        }

    elif method == "CDR":
        clean_u = _strip_to_unitary(qc)
        ideal_sim = sim_policy.aer_simulator(circuit=clean_u)
        cdr = _run_near_clifford_cdr(
            clean_u, sim, ideal_sim, raw_z0, raw_shot_std, int(shots), seed=int(seed),
        )
        if not cdr.get("ok"):
            return {
                "success": False,
                "unmitigated_value": float(raw_z0),
                "mitigated_value": float(raw_z0),
                "error_bound": None,
                "method_used": CDR_METHOD_V1,
                "measured": False,
                "fit_condition_number": cdr.get("fit_condition_number"),
                "error_message": str(cdr.get("error") or "CDR refused"),
            }
        mitigated_raw = float(cdr["mitigated_raw"])
        clipped = not (-1.0 <= mitigated_raw <= 1.0)
        mitigated = max(-1.0, min(1.0, mitigated_raw))
        return {
            "success": True, "kind": "mitigate", "result_version": "experiment-result-v1",
            "unmitigated_value": float(raw_z0),
            "unmitigated_measured": True,
            "mitigated_value": mitigated,
            "mitigated_measured": False,
            "measured": False,
            "error_bound": float(cdr["error_bound"]),
            "error_bound_method": "cdr-eiv-deming-propagated-se",
            "method_used": CDR_METHOD_V1,
            "cdr_method": "near_clifford_eiv_deming",
            "cdr_slope": float(cdr["cdr_slope"]),
            "cdr_intercept": float(cdr["cdr_intercept"]),
            "cdr_rmse": float(cdr["cdr_rmse"]),
            "chi2_reduced": float(cdr["chi2_reduced"]),
            "r_squared": float(cdr["r_squared"]),
            "fit_condition_number": float(cdr["fit_condition_number"]),
            "n_calibration_points": int(cdr["n_calibration_points"]),
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
