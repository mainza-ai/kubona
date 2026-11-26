"""Z2 Pauli symmetry detection, Z-basis post-selection, and Bonet-Monroig projection.

Implements exact symmetry verification ``E_SV = Tr(H P_S rho) / Tr(P_S rho)``
(Bonet-Monroig et al., Phys. Rev. A 98, 062339, 2018) with zero extra circuits
and zero free parameters whenever all symmetry-multiplied Pauli terms lie within
the measured QWC groups (e.g., H2 STO-3G with ``S = ZZ``).
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

from .qwc import (
    _index_to_bitstring,
    group_observable_vector,
    linear_functional_mean_and_var,
    pauli_parity_eigenvalue,
)

# Single-qubit Pauli multiplication table: (P, Q) -> (phase, R) where P*Q = phase*R
_PAULI_MUL: dict[tuple[str, str], tuple[complex, str]] = {
    ("I", "I"): (1, "I"), ("I", "X"): (1, "X"), ("I", "Y"): (1, "Y"), ("I", "Z"): (1, "Z"),
    ("X", "I"): (1, "X"), ("X", "X"): (1, "I"), ("X", "Y"): (1j, "Z"), ("X", "Z"): (-1j, "Y"),
    ("Y", "I"): (1, "Y"), ("Y", "X"): (-1j, "Z"), ("Y", "Y"): (1, "I"), ("Y", "Z"): (1j, "X"),
    ("Z", "I"): (1, "Z"), ("Z", "X"): (1j, "Y"), ("Z", "Y"): (-1j, "X"), ("Z", "Z"): (1, "I"),
}


def multiply_paulis(a: str, b: str) -> tuple[complex, str]:
    """Return ``(phase, product_str)`` for two equal-length Pauli strings."""
    if len(a) != len(b):
        raise ValueError(f"Pauli length mismatch: {a!r} vs {b!r}")
    phase: complex = 1.0 + 0.0j
    chars: list[str] = []
    for ca, cb in zip(a, b):
        p, r = _PAULI_MUL[(ca, cb)]
        phase *= p
        chars.append(r)
    return phase, "".join(chars)


def _commutes(pauli: str, z_sym: str) -> bool:
    anticommutes = 0
    for cp, cz in zip(pauli, z_sym):
        if cz == "Z" and cp in ("X", "Y"):
            anticommutes += 1
    return (anticommutes % 2) == 0


def _statevector_z_expectation(state: np.ndarray, z_sym: str) -> float:
    n_qubits = len(z_sym)
    probs = np.abs(state) ** 2
    exp_val = 0.0
    for idx, p in enumerate(probs):
        if p <= 0.0:
            continue
        bs = _index_to_bitstring(idx, n_qubits)
        exp_val += float(p) * pauli_parity_eigenvalue(z_sym, bs)
    return float(exp_val)


def detect_z_symmetries(
    coeffs: dict[str, float],
    *,
    target_state: np.ndarray | None = None,
    exact_circuit: Any = None,
    eigenvalue_tol: float = 1e-2,
) -> list[dict[str, Any]]:
    """Find independent ``{I, Z}^n`` symmetries commuting with ``coeffs``.

    If ``target_state`` or ``exact_circuit`` is provided, restricts to symmetries
    for which the target state is a definite ±1 eigenstate (within
    ``eigenvalue_tol``) and attaches the exact sector eigenvalue ``+1`` or ``-1``.
    Returns a linearly independent generator set (over GF(2)).
    """
    if not coeffs:
        return []
    terms = [str(t).strip().upper() for t, c in coeffs.items() if abs(float(c)) > 0.0]
    if not terms:
        return []
    n_qubits = len(terms[0])

    state = target_state
    if state is None and exact_circuit is not None:
        from qiskit.quantum_info import Statevector
        state = np.asarray(Statevector.from_instruction(exact_circuit).data, dtype=complex)

    candidates: list[str] = []
    if n_qubits <= 10:
        for mask in range(1, 1 << n_qubits):
            s = "".join("Z" if (mask >> (n_qubits - 1 - q)) & 1 else "I" for q in range(n_qubits))
            candidates.append(s)
    else:
        # Scalable candidate set for large-n: all-Z plus weight-1 and weight-2 Z strings.
        candidates.append("Z" * n_qubits)
        for i in range(n_qubits):
            chars = ["I"] * n_qubits
            chars[i] = "Z"
            candidates.append("".join(chars))
        for i in range(n_qubits):
            for j in range(i + 1, n_qubits):
                chars = ["I"] * n_qubits
                chars[i] = "Z"
                chars[j] = "Z"
                candidates.append("".join(chars))

    # Prefer higher-weight global parity symmetries last or deterministic lexicographic?
    # Sort by (weight, string) so simple symmetries come first; keep GF(2)-independent basis.
    commuting: list[tuple[str, int]] = []
    for sym in sorted(candidates, key=lambda s: (s.count("Z"), s)):
        if not all(_commutes(t, sym) for t in terms):
            continue
        if state is not None:
            exp_val = _statevector_z_expectation(state, sym)
            if abs(abs(exp_val) - 1.0) > eigenvalue_tol:
                continue
            eig = 1 if exp_val >= 0.0 else -1
        else:
            eig = 1
        commuting.append((sym, eig))

    # Extract GF(2) linearly independent generators via Gaussian elimination on bitmasks.
    basis_masks: list[int] = []
    generators: list[dict[str, Any]] = []
    for sym, eig in commuting:
        mask = int("".join("1" if ch == "Z" else "0" for ch in sym), 2)
        reduced = mask
        for b in basis_masks:
            if reduced ^ b < reduced:
                reduced ^= b
        if reduced != 0:
            basis_masks.append(reduced)
            basis_masks.sort(reverse=True)
            generators.append({"pauli": sym, "eigenvalue": int(eig)})
    return generators


def postselect_z_counts(
    counts: dict[str, int],
    symmetries: list[dict[str, Any]],
) -> tuple[dict[str, int], float]:
    """Post-select Z-basis counts onto the valid symmetry sector."""
    total = sum(int(v) for v in counts.values() if int(v) > 0)
    if total <= 0:
        raise ValueError("cannot post-select empty counts")
    if not symmetries:
        return {str(k): int(v) for k, v in counts.items() if int(v) > 0}, 1.0
    kept: dict[str, int] = {}
    kept_total = 0
    for bs, n in counts.items():
        cnt = int(n)
        if cnt <= 0:
            continue
        ok = True
        for sym in symmetries:
            if pauli_parity_eigenvalue(str(sym["pauli"]), str(bs)) != int(sym["eigenvalue"]):
                ok = False
                break
        if ok:
            kept[str(bs)] = cnt
            kept_total += cnt
    return kept, float(kept_total) / float(total)


def _expand_symmetry_group(
    symmetries: list[dict[str, Any]],
    n_qubits: int,
) -> list[tuple[int, str]]:
    """Return all elements ``(sign, pauli_str)`` of the abelian group generated by ``symmetries``."""
    elements: list[tuple[int, str]] = [(1, "I" * n_qubits)]
    for sym in symmetries:
        s_pauli = str(sym["pauli"])
        s_eig = int(sym["eigenvalue"])
        new_elems: list[tuple[int, str]] = []
        for sign, elem in elements:
            phase, prod = multiply_paulis(elem, s_pauli)
            # Both elem and s_pauli are in {I, Z}^n so phase is strictly +1.
            new_elems.append((int(sign * s_eig * round(phase.real)), prod))
        elements.extend(new_elems)
    return elements


def _term_compatible_group_index(term: str, groups: list[dict[str, Any]]) -> int | None:
    if set(term) <= {"I"}:
        return -1  # identity (constant)
    for idx, grp in enumerate(groups):
        basis = grp["basis"]
        if all(b == "I" or t == "I" or b == t for b, t in zip(basis, term)):
            # Need every non-I position of term to actually be measured in that basis.
            if all(t == "I" or b == t for b, t in zip(basis, term)):
                return idx
    return None


def symmetry_verified_energy(
    grouped: dict[str, Any],
    group_counts: list[dict[str, int]],
    coeffs: dict[str, float],
    symmetries: list[dict[str, Any]],
    *,
    readout_inverse: np.ndarray | None = None,
    assignment_matrix: np.ndarray | None = None,
    cal_shots_per_state: list[int] | None = None,
    min_survival_prob: float = 0.05,
) -> dict[str, Any]:
    """Compute Bonet-Monroig symmetry-verified energy ``Tr(H P_S rho) / Tr(P_S rho)``.

    Uses exact delta-method error propagation (including Z-group numerator/denominator
    covariance and optional full readout matrix inverse + calibration shot variance).
    If any symmetry-multiplied non-Z Pauli term is not measurable from the supplied
    QWC groups, falls back transparently to Z-basis post-selection and sets
    ``projector_exact=False``.
    """
    groups = grouped["groups"]
    n_qubits = int(grouped["n_qubits"])
    if len(group_counts) != len(groups):
        raise ValueError(f"expected {len(groups)} QWC count dicts, got {len(group_counts)}")
    if not symmetries:
        raise ValueError("symmetry_verified_energy requires at least one symmetry generator")
    if n_qubits > 10:
        raise ValueError(f"Bonet-Monroig dense projector unsupported for n_qubits={n_qubits} > 10")

    z_idx = next((i for i, g in enumerate(groups) if g.get("is_z_basis")), None)
    if z_idx is None:
        raise ValueError("symmetry verification requires a Z-basis QWC measurement group")

    sym_group = _expand_symmetry_group(symmetries, n_qubits)
    group_order = float(len(sym_group))

    # Denominator D = Tr(P_S rho) = (1/|G|) sum_{g in G} chi(g) <g>
    # Every g in G is in {I, Z}^n so D lives purely on the Z-basis group.
    denom_const = 0.0
    denom_z_terms: dict[str, float] = {}
    for sign, g_pauli in sym_group:
        w = float(sign) / group_order
        if set(g_pauli) <= {"I"}:
            denom_const += w
        else:
            denom_z_terms[g_pauli] = denom_z_terms.get(g_pauli, 0.0) + w

    d_vec = denom_const * np.ones(1 << n_qubits, dtype=float)
    if denom_z_terms:
        d_vec = d_vec + group_observable_vector(denom_z_terms, n_qubits)
    denom_val, denom_var, _ = linear_functional_mean_and_var(
        d_vec, group_counts[z_idx], n_qubits,
        readout_inverse=readout_inverse,
        assignment_matrix=assignment_matrix,
        cal_shots_per_state=cal_shots_per_state,
    )
    if not math.isfinite(denom_val) or denom_val < float(min_survival_prob):
        raise ValueError(
            f"symmetry survival probability {denom_val:.4f} below fail-closed floor {min_survival_prob}"
        )

    # Numerator N = Tr(H P_S rho) = (1/|G|) sum_{P} c_P sum_{g in G} chi(g) <P g>
    num_const = 0.0
    num_by_group: list[dict[str, float]] = [{} for _ in groups]
    uncovered: list[str] = []
    for term, coeff in coeffs.items():
        c = float(coeff)
        if abs(c) == 0.0:
            continue
        t = str(term).strip().upper()
        for sign, g_pauli in sym_group:
            phase, prod = multiply_paulis(t, g_pauli)
            if abs(phase.imag) > 1e-9:
                raise ValueError(f"symmetry {g_pauli!r} does not commute with Hamiltonian term {t!r}")
            w = c * float(sign) * float(phase.real) / group_order
            g_idx = _term_compatible_group_index(prod, groups)
            if g_idx is None:
                uncovered.append(prod)
            elif g_idx == -1:
                num_const += w
            else:
                num_by_group[g_idx][prod] = num_by_group[g_idx].get(prod, 0.0) + w

    # Raw Z-basis acceptance rate (unmitigated diagnostic)
    _, raw_acceptance = postselect_z_counts(group_counts[z_idx], symmetries)

    if not uncovered:
        # Exact Bonet-Monroig projector across all terms!
        num_val = float(num_const)
        group_x_vecs: list[np.ndarray] = []
        for idx in range(len(groups)):
            x_g = (
                group_observable_vector(num_by_group[idx], n_qubits)
                if num_by_group[idx]
                else np.zeros(1 << n_qubits, dtype=float)
            )
            group_x_vecs.append(x_g)
            if np.any(x_g):
                g_mean, _, _ = linear_functional_mean_and_var(
                    x_g, group_counts[idx], n_qubits,
                    readout_inverse=readout_inverse,
                    assignment_matrix=assignment_matrix,
                    cal_shots_per_state=cal_shots_per_state,
                )
                num_val += g_mean
        energy_sv = float(num_val / denom_val)

        # Exact delta-method variance: Var(N/D) = Var(N - E_SV * D) / D^2.
        # Group z_idx shares both N_z and D, so combine linearly before variance!
        total_delta_var = 0.0
        total_delta_var_shot = 0.0
        total_delta_var_cal = 0.0
        for idx in range(len(groups)):
            y_g = group_x_vecs[idx] - (energy_sv * d_vec if idx == z_idx else 0.0)
            if not np.any(y_g):
                continue
            _, v_g, v_meta = linear_functional_mean_and_var(
                y_g, group_counts[idx], n_qubits,
                readout_inverse=readout_inverse,
                assignment_matrix=assignment_matrix,
                cal_shots_per_state=cal_shots_per_state,
            )
            total_delta_var += v_g
            total_delta_var_shot += v_meta["var_shot"]
            total_delta_var_cal += v_meta["var_cal"]
        d2 = denom_val * denom_val
        sv_var = max(0.0, total_delta_var / d2)
        sv_var_shot = max(0.0, total_delta_var_shot / d2)
        sv_var_cal = max(0.0, total_delta_var_cal / d2)
        return {
            "energy": float(energy_sv),
            "uncertainty": float(math.sqrt(sv_var)),
            "variance_shot": float(sv_var_shot),
            "variance_cal": float(sv_var_cal),
            "method": "bonet_monroig_exact_projector",
            "projector_exact": True,
            "symmetries": symmetries,
            "survival_probability": float(denom_val),
            "survival_std_error": float(math.sqrt(max(0.0, denom_var))),
            "raw_z_acceptance_rate": float(raw_acceptance),
            "numerator": float(num_val),
            "denominator": float(denom_val),
        }

    # Transparent partial fallback: post-select Z group only, keep uncovered non-Z unprojected.
    z_terms = groups[z_idx]["terms"]
    # Build Z-only numerator Tr(H_Z P_S rho) / Tr(P_S rho) + identity_offset
    nz_const = float(grouped["identity_offset"]) * denom_const
    nz_terms: dict[str, float] = {}
    for sign, g_pauli in sym_group:
        w = float(grouped["identity_offset"]) * float(sign) / group_order
        if set(g_pauli) <= {"I"}:
            continue
        nz_terms[g_pauli] = nz_terms.get(g_pauli, 0.0) + w
    for term, coeff in z_terms.items():
        for sign, g_pauli in sym_group:
            phase, prod = multiply_paulis(term, g_pauli)
            w = float(coeff) * float(sign) * float(phase.real) / group_order
            if set(prod) <= {"I"}:
                nz_const += w
            else:
                nz_terms[prod] = nz_terms.get(prod, 0.0) + w
    nz_vec = nz_const * np.ones(1 << n_qubits, dtype=float)
    if nz_terms:
        nz_vec = nz_vec + group_observable_vector(nz_terms, n_qubits)
    nz_val, _, _ = linear_functional_mean_and_var(
        nz_vec, group_counts[z_idx], n_qubits,
        readout_inverse=readout_inverse,
        assignment_matrix=assignment_matrix,
        cal_shots_per_state=cal_shots_per_state,
    )
    e_z = float(nz_val / denom_val)
    _, var_z_num, meta_z_num = linear_functional_mean_and_var(
        nz_vec - e_z * d_vec, group_counts[z_idx], n_qubits,
        readout_inverse=readout_inverse,
        assignment_matrix=assignment_matrix,
        cal_shots_per_state=cal_shots_per_state,
    )
    d2 = denom_val * denom_val
    total_e = e_z
    total_v = var_z_num / d2
    total_v_shot = meta_z_num["var_shot"] / d2
    total_v_cal = meta_z_num["var_cal"] / d2
    for idx, grp in enumerate(groups):
        if idx == z_idx:
            continue
        x_g = group_observable_vector(grp["terms"], n_qubits)
        m_g, v_g, meta_g = linear_functional_mean_and_var(
            x_g, group_counts[idx], n_qubits,
            readout_inverse=readout_inverse,
            assignment_matrix=assignment_matrix,
            cal_shots_per_state=cal_shots_per_state,
        )
        total_e += m_g
        total_v += v_g
        total_v_shot += meta_g["var_shot"]
        total_v_cal += meta_g["var_cal"]
    return {
        "energy": float(total_e),
        "uncertainty": float(math.sqrt(max(0.0, total_v))),
        "variance_shot": float(max(0.0, total_v_shot)),
        "variance_cal": float(max(0.0, total_v_cal)),
        "method": "z_postselection_only",
        "projector_exact": False,
        "uncovered_product_terms": sorted(set(uncovered)),
        "symmetries": symmetries,
        "survival_probability": float(denom_val),
        "survival_std_error": float(math.sqrt(max(0.0, denom_var))),
        "raw_z_acceptance_rate": float(raw_acceptance),
    }


__all__ = [
    "detect_z_symmetries",
    "multiply_paulis",
    "postselect_z_counts",
    "symmetry_verified_energy",
]
