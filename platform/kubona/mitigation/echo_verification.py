"""General N-qubit Echo Verification / Dual-State Purification (EV).

Implements O'Brien et al. (PRX Quantum 2, 020317, 2021) & Huo & Li (PRA 105,
022427, 2022) using only ``n + 1`` qubits (one ancilla + ``n`` system qubits,
no spatial doubling):
  - Reference echo circuit (``P = I``): ``U`` followed by ``U^dag``, measuring
    the Loschmidt echo return probability ``F_echo = P(sys == 0^n)``.
  - Observable echo circuit (for each non-identity Pauli ``P``):
    ``H(anc) -> U(sys) -> controlled-P(anc, sys) -> U^dag(sys) -> H(anc)``,
    computing ``N_P = E[Z_anc * 1_{sys == 0^n}]``.
  - Exact quadratic-suppression ratio estimator: ``<P>_EV = N_P / F_echo``.
    Under incoherent errors ``rho = (1 - eps)|psi><psi| + eps rho_perp``, both
    ``N_P`` and ``F_echo`` scale as ``(1 - eps)^2 + O(eps^2 / 2^n)``, canceling
    leading-order error with exact sign preservation and zero training bias.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

Z95 = 1.959963984540054


def _append_controlled_pauli(qc: Any, anc: int, pauli: str) -> None:
    """Append controlled-``P`` (little-endian Pauli string) controlled on ``anc``."""
    for i, ch in enumerate(pauli[::-1]):
        if ch == "X":
            qc.cx(anc, i)
        elif ch == "Y":
            # Exact qelib1.inc / Aer-native decomposition: S · CX(anc, i) · S^dag == CY(anc, i)
            qc.sdg(i)
            qc.cx(anc, i)
            qc.s(i)
        elif ch == "Z":
            qc.cz(anc, i)
        elif ch != "I":
            raise ValueError(f"unsupported Pauli character {ch!r} in {pauli!r}")


def build_echo_verification_circuits(
    qc: Any,
    coeffs: dict[str, float],
    *,
    symmetries: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Build the ``n + 1``-qubit Echo Verification circuit batch for Hamiltonian ``coeffs``."""
    from qiskit import QuantumCircuit

    n = int(qc.num_qubits)
    if n < 1 or n > 14:
        raise ValueError(f"Echo Verification supports 1 <= n_qubits <= 14, got {n}")
    if int(qc.num_clbits) > 0 or any(
        getattr(inst.operation, "name", "") == "measure" for inst in qc.data
    ):
        raise ValueError("Echo Verification requires a unitary circuit without measurements")

    inv_qc = qc.inverse()
    anc = n
    sym_map = {str(s["pauli"]).strip().upper(): int(s["eigenvalue"]) for s in (symmetries or [])}

    id_offset = 0.0
    exact_sym_terms: dict[str, tuple[float, int]] = {}
    terms_to_measure: list[tuple[str, float]] = []

    for term, coeff in sorted(coeffs.items()):
        t = str(term).strip().upper()
        c = float(coeff)
        if abs(c) == 0.0:
            continue
        if set(t) <= {"I"}:
            id_offset += c
        elif t in sym_map:
            exact_sym_terms[t] = (c, sym_map[t])
        else:
            terms_to_measure.append((t, c))

    circuits: list[Any] = []
    # Circuit 0: Reference Loschmidt echo (P = I) on identical n+1 register for
    # matched timing/layout parity.
    ref_qc = QuantumCircuit(n + 1)
    ref_qc.compose(qc, qubits=list(range(n)), inplace=True)
    ref_qc.barrier()
    ref_qc.compose(inv_qc, qubits=list(range(n)), inplace=True)
    ref_qc.measure_all()
    circuits.append(ref_qc)

    for term, _coeff in terms_to_measure:
        ev_qc = QuantumCircuit(n + 1)
        ev_qc.h(anc)
        ev_qc.compose(qc, qubits=list(range(n)), inplace=True)
        ev_qc.barrier()
        _append_controlled_pauli(ev_qc, anc, term)
        ev_qc.barrier()
        ev_qc.compose(inv_qc, qubits=list(range(n)), inplace=True)
        ev_qc.h(anc)
        ev_qc.measure_all()
        circuits.append(ev_qc)

    return {
        "n_logical_qubits": n,
        "n_physical_qubits": n + 1,
        "ancilla_qubit": anc,
        "identity_offset": float(id_offset),
        "exact_symmetry_terms": exact_sym_terms,
        "terms": [{"term": t, "coefficient": float(c)} for t, c in terms_to_measure],
        "circuits": circuits,
    }


def evaluate_echo_verification_counts(
    ev_plan: dict[str, Any],
    counts_list: list[dict[str, int]],
    *,
    constant_energy: float = 0.0,
    min_echo_fidelity: float = 0.05,
    raw_baseline_energy: float | None = None,
    raw_baseline_uncertainty: float | None = None,
    m3_calibration: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Evaluate Echo Verification energy with exact ratio-variance propagation.

    When ``m3_calibration`` is provided, applies scalable M3 readout mitigation
    to the ``n + 1``-qubit counts before computing ``F_echo`` and ``N_P``,
    eliminating linear post-echo ancilla readout error.
    """
    terms = ev_plan["terms"]
    n = int(ev_plan["n_logical_qubits"])
    if len(counts_list) != len(terms) + 1:
        raise ValueError(f"expected {len(terms) + 1} EV count dicts, got {len(counts_list)}")

    sys_mask = (1 << n) - 1

    def _dist(cnts: dict[str, int]) -> tuple[dict[str, float], int, float]:
        tot = sum(int(v) for v in cnts.values() if int(v) > 0)
        if tot <= 0:
            return {}, 0, 1.0
        if m3_calibration is not None:
            from .readout_m3 import mitigate_counts_m3
            m3_r = mitigate_counts_m3(cnts, m3_calibration)
            qp = {str(k): float(v) for k, v in m3_r["quasi_probs"].items()}
            l1 = sum(abs(v) for v in qp.values())
            return qp, tot, max(1.0, l1 * l1)
        return {str(k): float(int(v)) / float(tot) for k, v in cnts.items() if int(v) > 0}, tot, 1.0

    # 1. Reference echo fidelity F_echo = P(sys == 0^n) from circuit 0
    ref_dist, ref_total, ref_scale = _dist(counts_list[0])
    if ref_total <= 0:
        raise ValueError("Echo Verification reference circuit returned zero counts")
    f_echo = 0.0
    for raw_bs, w in ref_dist.items():
        clean = str(raw_bs).replace(" ", "").zfill(n + 1)
        idx = int(clean[-(n + 1):], 2)
        if (idx & sys_mask) == 0:
            f_echo += float(w)

    var_f = max(0.0, ref_scale * max(0.0, f_echo * (1.0 - min(1.0, max(0.0, f_echo)))) / float(max(1, ref_total - 1)))
    if not math.isfinite(f_echo) or f_echo < float(min_echo_fidelity):
        return {
            "ok": False,
            "error": (
                f"Echo Verification fidelity {f_echo:.4f} below fail-closed floor "
                f"{min_echo_fidelity}"
            ),
            "echo_fidelity": f_echo,
        }

    # 2. Observable numerators N_P = E[Z_anc * 1_{sys == 0^n}]
    elec_const = float(ev_plan["identity_offset"])
    for _t, (c, eig) in ev_plan.get("exact_symmetry_terms", {}).items():
        elec_const += float(c) * float(eig)

    num_total = 0.0
    var_num_total = 0.0
    per_term: dict[str, dict[str, Any]] = {}
    for spec, cnts in zip(terms, counts_list[1:]):
        c = float(spec["coefficient"])
        dist_p, total, v_scale = _dist(cnts)
        if total <= 0:
            raise ValueError(f"Echo Verification circuit for {spec['term']} returned zero counts")
        mean_np = 0.0
        s2 = 0.0
        for raw_bs, w in dist_p.items():
            clean = str(raw_bs).replace(" ", "").zfill(n + 1)
            idx = int(clean[-(n + 1):], 2)
            if (idx & sys_mask) == 0:
                anc_bit = (idx >> n) & 1
                z_anc = 1.0 if anc_bit == 0 else -1.0
                mean_np += float(w) * z_anc
                s2 += abs(float(w))
        var_np = max(0.0, v_scale * max(0.0, s2 - mean_np * mean_np) / float(max(1, total - 1)))
        exp_p = mean_np / f_echo
        num_total += c * mean_np
        var_num_total += (c * c) * var_np
        per_term[spec["term"]] = {
            "coefficient": c,
            "numerator": float(mean_np),
            "expectation": float(exp_p),
            "std_error": float(math.sqrt(max(0.0, (var_np + (exp_p * exp_p) * var_f) / (f_echo * f_echo)))),
            "shots": int(total),
        }

    # E_EV = elec_const + (sum_P c_P N_P) / F_echo
    # Shared denominator F_echo couples all terms:
    # Var(N_tot / F) = (Var(N_tot) + (N_tot / F)^2 * Var(F)) / F^2
    ratio_energy = num_total / f_echo
    elec_energy = elec_const + ratio_energy
    total_var = max(0.0, (var_num_total + (ratio_energy * ratio_energy) * var_f) / (f_echo * f_echo))
    se = math.sqrt(total_var)

    diff_raw = (elec_energy - float(raw_baseline_energy)) if raw_baseline_energy is not None else 0.0
    diff_se = math.sqrt(max(0.0, total_var + float(raw_baseline_uncertainty or 0.0) ** 2))
    diff_ci = Z95 * diff_se

    return {
        "ok": True,
        "method": "EV",
        "mitigated": float(elec_energy),
        "energy": float(elec_energy + float(constant_energy)),
        "uncertainty": float(se),
        "ci95_mha": float(Z95 * se * 1000.0),
        "echo_fidelity": float(f_echo),
        "echo_fidelity_std_error": float(math.sqrt(var_f)),
        "echo_fidelity_floor": float(min_echo_fidelity),
        "model_adequate": bool(f_echo >= float(min_echo_fidelity)),
        "per_term": per_term,
        "difference_ha": float(diff_raw),
        "difference_ci95_ha": float(diff_ci),
        "difference_ci95_mha": float(diff_ci * 1000.0),
        "difference_significant": bool(raw_baseline_energy is not None and abs(diff_raw) > diff_ci),
    }


__all__ = [
    "build_echo_verification_circuits",
    "evaluate_echo_verification_counts",
]
