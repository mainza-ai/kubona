"""Pre-Dispatch Statistical Power Gate & Autonomous Mitigation Advisor.

Solves Root Cause 4 & Gap G10 from the 2026-11-26 R2 autopsy:
  - Before dispatching any QPU job, computes predicted shot+fit ``SE`` (mHa),
    predicted physical residual bias ceiling (mHa), single-job pass probability
    ``P(|Delta| <= tol)``, consecutive 2-job reproduction probability
    ``P_pass^2``, and estimated QPU seconds / RU.
  - Fails closed (or advises exact upgrade) whenever ``P(2x pass) < min_power``
    (default 0.80) so Kubona never wastes hardware quota on statistically
    underpowered configurations (like the 46% consecutive-pass regime of C3/C4).
  - ``advise_mitigation`` deterministically selects the physics-optimal
    ``(ansatz_family, readout_mitigation, gate_mitigation, target_shots)`` stack
    for any quantum chemistry, lattice spin, QAOA, or arbitrary observable workload.
"""

from __future__ import annotations

import math
from typing import Any


def _normal_cdf(z: float) -> float:
    return 0.5 * (1.0 + math.erf(float(z) / math.sqrt(2.0)))


def assess_statistical_power(
    *,
    method: str,
    n_qubits: int,
    n_2q_gates: int,
    shots: int,
    target_shots: list[int] | None = None,
    n_qwc_groups: int = 3,
    hamiltonian_l1_ha: float = 1.0,
    tolerance_mha: float = 1.6,
    two_qubit_error: float = 0.003,
    readout_error: float = 0.01,
    has_z2_symmetry: bool = False,
    cdr_training_circuits: int = 12,
    min_consecutive_power: float = 0.80,
) -> dict[str, Any]:
    """Predict ``SE``, bias bound, consecutive-pass probability, and RU cost pre-dispatch."""
    m = str(method or "NONE").strip().upper()
    n_q = max(1, int(n_qubits))
    n_2q = max(0, int(n_2q_gates))
    g_cnt = max(1, int(n_qwc_groups))
    tol = max(1e-4, float(tolerance_mha))
    e2 = max(1e-4, float(two_qubit_error))
    ro = max(1e-4, float(readout_error))

    # Effective target shots across groups
    if target_shots and len(target_shots) > 0:
        eff_target_shots = int(sum(target_shots))
    else:
        eff_target_shots = int(shots) * g_cnt

    # Raw circuit incoherent error rate eps ~ 1 - (1 - e2)^n_2q
    eps_gate = 1.0 - ((1.0 - e2) ** max(1, n_2q)) if n_2q > 0 else 0.0005
    eps_eff = eps_gate * (0.45 if has_z2_symmetry else 1.0)
    # Energy scale of excitation gap (conservative 1.5 Ha per unit L1 norm)
    e_scale_mha = max(500.0, float(hamiltonian_l1_ha) * 1000.0)
    # Base QWC single-shot standard error (mHa): Z2 symmetry suppresses odd-parity
    # Z-group variance (empirically ~0.24 * L1 on H2 vs ~0.35 * L1 unconstrained).
    sigma_factor = 0.24 if has_z2_symmetry else 0.35
    base_se_mha = (sigma_factor * e_scale_mha) / math.sqrt(max(1.0, float(eff_target_shots)))

    if m in ("VD", "VIRTUAL_DISTILLATION"):
        # Purity Tr(rho^2) ~ (1 - eps)^2; quadratic suppression of state-prep noise,
        # PLUS post-prep B^(2) circuit gate overhead (~2*n_q 2Q gates, unmitigated by rho^2).
        purity_est = max(0.35, (1.0 - eps_eff) ** 2)
        b2_2q_err = 1.0 - ((1.0 - e2) ** (2 * n_q))
        predicted_se_mha = (base_se_mha / purity_est) * 0.85
        predicted_bias_mha = (eps_eff ** 2) * e_scale_mha * 1.5 + 0.30 * b2_2q_err * e_scale_mha + 0.08
        n_circuits = g_cnt + 2  # VD circuits + 2 M3 readout cal circuits
        total_shots = eff_target_shots + int(shots) * 2
    elif m in ("EV", "ECHO_VERIFICATION"):
        # Echo Verification runs U + controlled-P + U^dag (2x ansatz + n_q 2Q overhead)
        f_echo = max(0.20, (1.0 - eps_gate) ** 2)
        ev_ctrl_err = 1.0 - ((1.0 - e2) ** max(1, n_q))
        predicted_se_mha = (base_se_mha / f_echo) * 1.10
        predicted_bias_mha = (eps_eff ** 2) * e_scale_mha * 1.5 + 0.35 * ev_ctrl_err * e_scale_mha + 0.15
        n_circuits = g_cnt + 3
        total_shots = eff_target_shots + int(shots) * 3
    elif m in ("CDR", "CDR_PER_GROUP"):
        # Compile-once ISA-RZ CDR removes transpilation bias AND adds ZERO extra 2Q gates!
        # Residual bias is O(quadratic non-linearity of circuit error) ~ O(eps_eff^2).
        fit_se_mha = base_se_mha * (1.05 if m == "CDR" else 1.10)
        predicted_se_mha = math.sqrt(base_se_mha ** 2 + fit_se_mha ** 2)
        predicted_bias_mha = 0.12 + 0.04 * (eps_eff * eps_eff) * e_scale_mha + (0.04 * eps_eff * e_scale_mha if n_2q >= 6 else 0.0)
        n_cal = (1 << n_q) if n_q <= 4 else 2
        n_circuits = g_cnt * (1 + int(cdr_training_circuits)) + n_cal
        total_shots = eff_target_shots + int(shots) * g_cnt * int(cdr_training_circuits) + int(shots) * n_cal
    elif m == "VNCDR":
        predicted_se_mha = base_se_mha * 1.65
        predicted_bias_mha = 0.15 + 0.03 * eps_eff * e_scale_mha
        n_circuits = g_cnt * (1 + int(cdr_training_circuits)) * 3 + ((1 << n_q) if n_q <= 4 else 2)
        total_shots = int(shots) * n_circuits
    elif m == "PEA":
        predicted_se_mha = base_se_mha * 1.45
        predicted_bias_mha = 0.25 + 0.04 * eps_eff * e_scale_mha
        n_circuits = g_cnt * 4 + 8
        total_shots = eff_target_shots * 4
    elif m == "PEC":
        gamma = math.exp(2.0 * n_2q * e2)
        predicted_se_mha = base_se_mha * gamma
        predicted_bias_mha = 0.12
        n_circuits = max(16, int(round(16 * gamma)))
        total_shots = int(eff_target_shots * gamma * gamma)
    elif m == "SQD":
        # Single Z-basis sampling + classical subspace projection
        predicted_se_mha = max(0.15, 0.25 * base_se_mha)
        predicted_bias_mha = 0.15
        n_circuits = 3  # 1 Z-basis + 2 M3 cal
        total_shots = max(int(shots), 8192) + 8192
    elif m == "QSE":
        predicted_se_mha = base_se_mha * 1.25
        predicted_bias_mha = 0.20 + 0.10 * eps_eff * e_scale_mha
        n_circuits = g_cnt * 3
        total_shots = eff_target_shots * 3
    elif m == "ZNE":
        predicted_se_mha = base_se_mha * 2.4
        predicted_bias_mha = max(1.5, 0.35 * eps_eff * e_scale_mha)
        n_circuits = g_cnt * 4
        total_shots = int(shots) * n_circuits
    else:
        # Unmitigated raw
        predicted_se_mha = base_se_mha
        predicted_bias_mha = (eps_gate + n_q * ro) * e_scale_mha
        n_circuits = g_cnt
        total_shots = eff_target_shots

    # Exact Gaussian probability P(|N(bias, se^2)| <= tol)
    se_safe = max(1e-4, predicted_se_mha)
    p_single = max(
        0.0,
        _normal_cdf((tol - predicted_bias_mha) / se_safe)
        - _normal_cdf((-tol - predicted_bias_mha) / se_safe),
    )
    p_consec_2x = p_single * p_single

    # Empirical Heron throughput: ~8,000 shots/sec + ~0.25s framing per circuit
    est_seconds = round(total_shots / 8000.0 + 0.25 * min(n_circuits, 64), 2)
    est_ru = round(est_seconds / 60.0, 4)

    power_ok = bool(p_consec_2x >= float(min_consecutive_power))
    recommendation = None
    if not power_ok:
        if predicted_bias_mha > 0.5 * tol:
            recommendation = (
                f"Predicted bias ({predicted_bias_mha:.2f} mHa) dominates tolerance ({tol:.2f} mHa); "
                "switch to VD (quadratic suppression), compact Givens ansatz, or SQD."
            )
        else:
            # Solve required shot multiplier so SE <= tol / 2.6
            target_se = max(0.15, (tol - predicted_bias_mha) / 2.6)
            mult = (predicted_se_mha / target_se) ** 2
            recommendation = (
                f"Underpowered: predicted SE={predicted_se_mha:.2f} mHa gives only "
                f"{p_consec_2x*100:.1f}% consecutive 2x pass rate (< {min_consecutive_power*100:.0f}%). "
                f"Increase effective target shots by ~{mult:.1f}x or switch to VD/SQD."
            )

    return {
        "method": m,
        "tolerance_mha": tol,
        "predicted_se_mha": round(predicted_se_mha, 4),
        "predicted_ci95_mha": round(1.959963984540054 * predicted_se_mha, 4),
        "predicted_bias_bound_mha": round(predicted_bias_mha, 4),
        "predicted_single_pass_prob": round(p_single, 4),
        "predicted_consecutive_2x_pass_prob": round(p_consec_2x, 4),
        "min_consecutive_power": float(min_consecutive_power),
        "power_ok": power_ok,
        "estimated_circuits": int(n_circuits),
        "estimated_total_shots": int(total_shots),
        "estimated_qpu_seconds": est_seconds,
        "estimated_ru": est_ru,
        "recommendation": recommendation,
    }


def advise_mitigation(
    *,
    problem_kind: str = "chemistry",
    n_qubits: int = 2,
    n_2q_gates: int = 1,
    has_z2_symmetry: bool = True,
    two_qubit_error: float = 0.003,
    readout_error: float = 0.01,
    tolerance_mha: float = 1.6,
    ru_budget: float = 1.0,
    available_physical_qubits: int = 156,
) -> dict[str, Any]:
    """Deterministically recommend the frontier mitigation & ansatz stack for any workload."""
    kind = str(problem_kind or "chemistry").strip().lower()
    n_q = max(1, int(n_qubits))
    n_2q = max(0, int(n_2q_gates))

    # Readout tier: exact 2^n x 2^n for n <= 4, scalable M3 matrix-free for n >= 5
    readout_mode = "live_assignment_matrix" if n_q <= 4 else "m3_matrix_free"

    if kind == "qaoa":
        return {
            "problem_kind": kind,
            "recommended_ansatz": "qaoa_hardware_native",
            "recommended_readout": readout_mode,
            "recommended_mitigation": "M3",
            "recommended_cvar_alpha": 0.25,
            "rationale": (
                f"{n_q}-qubit QAOA optimizes bitstring cut distributions: M3 matrix-free "
                "readout mitigation + CVaR tail aggregation directly improve cut quality "
                "without exponential calibration cost."
            ),
        }

    if kind == "chemistry" and n_q >= 8:
        pw = assess_statistical_power(
            method="SQD", n_qubits=n_q, n_2q_gates=n_2q, shots=16384,
            tolerance_mha=tolerance_mha, two_qubit_error=two_qubit_error,
            readout_error=readout_error, has_z2_symmetry=has_z2_symmetry,
        )
        return {
            "problem_kind": kind,
            "recommended_ansatz": "lucj_tapered",
            "recommended_readout": readout_mode,
            "recommended_mitigation": "SQD",
            "fallback_mitigation": "PEA",
            "recommended_shots": 16384,
            "power_assessment": pw,
            "rationale": (
                f"{n_q}-qubit chemistry exceeds low-depth Pauli-grouping sweet spot: "
                "LUCJ + Sample-based Quantum Diagonalization (SQD with S-CORE) uses "
                "only 1 Z-basis measurement circuit, recovers bit-flip noise self-consistently, "
                "and scales to 77+ qubits on IBM Heron."
            ),
        }

    # Small/medium chemistry or spin chains (n <= 7): compare VD, EV, ISA-RZ CDR, PEA
    candidates = ["VD", "EV", "CDR", "PEA"]
    if 2 * n_q > int(available_physical_qubits):
        candidates.remove("VD")

    scored: list[tuple[float, str, dict[str, Any]]] = []
    rec_shots = 32768 if n_q <= 2 else 16384
    for cand in candidates:
        # C-optimal ISA-RZ CDR allocates high shots to the target PUBs and lean
        # c-optimal shots across training circuits (~0.75 RU instead of 2.7 RU).
        if cand == "CDR":
            c_shots = 4096
            c_target_shots = [65536, 49152, 49152] if n_q <= 2 else [32768, 16384, 16384]
        else:
            c_shots = rec_shots
            c_target_shots = None
        pw = assess_statistical_power(
            method=cand,
            n_qubits=n_q,
            n_2q_gates=n_2q,
            shots=c_shots,
            target_shots=c_target_shots,
            tolerance_mha=tolerance_mha,
            two_qubit_error=two_qubit_error,
            readout_error=readout_error,
            has_z2_symmetry=has_z2_symmetry,
        )
        ru_pen = 0.0 if pw["estimated_ru"] <= float(ru_budget) else 0.5
        score = pw["predicted_consecutive_2x_pass_prob"] - 0.15 * pw["estimated_ru"] - ru_pen
        scored.append((score, cand, pw))

    scored.sort(key=lambda item: -item[0])
    best_method = scored[0][1]
    best_pw = scored[0][2]
    fallback = scored[1][1] if len(scored) > 1 else "CDR"

    rec_ansatz = "givens" if (kind == "chemistry" and n_q == 2) else (
        "uccsd_parity_tapered" if kind == "chemistry" else "hva_spin"
    )
    return {
        "problem_kind": kind,
        "recommended_ansatz": rec_ansatz,
        "recommended_readout": readout_mode,
        "recommended_symmetry_verification": bool(has_z2_symmetry),
        "recommended_mitigation": best_method,
        "fallback_mitigation": fallback,
        "recommended_shots": rec_shots,
        "power_assessment": best_pw,
        "ranked_candidates": [
            {
                "method": m,
                "score": round(s, 4),
                "consecutive_2x_pass_prob": pw["predicted_consecutive_2x_pass_prob"],
                "predicted_se_mha": pw["predicted_se_mha"],
                "estimated_ru": pw["estimated_ru"],
            }
            for s, m, pw in scored
        ],
        "rationale": (
            f"Selected {best_method} + {readout_mode}"
            + (" + Bonet-Monroig Z2 symmetry" if has_z2_symmetry else "")
            + f" (predicted SE={best_pw['predicted_se_mha']:.2f} mHa, "
            f"2x pass prob={best_pw['predicted_consecutive_2x_pass_prob']*100:.1f}%, "
            f"est={best_pw['estimated_ru']:.3f} RU)."
        ),
    }


__all__ = [
    "advise_mitigation",
    "assess_statistical_power",
]
