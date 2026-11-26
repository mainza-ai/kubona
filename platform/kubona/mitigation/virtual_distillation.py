"""General N-qubit Transversal Virtual Distillation (VD) via ancilla-free B^(2) circuits.

Implements Huggins et al. (PRX 11, 041036, 2021) and Koczor (PRX 11, 031057, 2021)
for arbitrary ``n``-qubit states using ``2n`` qubits (two copies ``rho \\otimes rho``)
and zero ancillas:
  - Weight-1 Pauli terms in a shared QWC basis are measured simultaneously in ONE
    circuit via the transversal 2-qubit diagonalizing gate ``B^(2) = CH(b, a) CX(a, b)``.
  - Higher-weight Pauli terms ``P`` (weight ``w >= 2``) are mapped exactly to
    single-qubit ``Z_t`` via a Clifford parity ladder ``U_P \\otimes U_P`` before
    applying ``B^(2)`` on every pair ``(a_i, b_i)`` — yielding exact
    ``Tr(P rho^2) / Tr(rho^2)`` to machine precision for ANY ``n``-qubit Pauli string.
  - Suppresses incoherent excitation/depolarizing errors quadratically (``eps -> eps^2``)
    with zero training-circuit bias and exact coupled delta-method variance propagation.
"""

from __future__ import annotations

import math
from typing import Any

import numpy as np

from .qwc import group_pauli_terms

Z95 = 1.959963984540054


def _apply_b2(qc: Any, a: int, b: int) -> None:
    """Simultaneous eigenbasis unitary for SWAP ``S`` and ``(1/2) S (Z_a + Z_b)``.

    Maps:
      ``|00>  -> |00>`` (``S = +1``, ``Z_eff = +1``)
      ``|11>  -> |10>`` (``S = +1``, ``Z_eff = -1``)
      ``|Psi+ -> |01>`` (``S = +1``, ``Z_eff =  0``)
      ``|Psi- -> |11>`` (``S = -1``, ``Z_eff =  0``)

    Uses the exact 1-CX ``qelib1.inc`` / Aer-native decomposition of ``CH(b, a)``:
    ``Ry(-pi/4, a) · CX(b, a) · Ry(+pi/4, a) == CH(b, a)`` (zero global phase error).
    """
    qc.cx(a, b)
    qc.ry(math.pi / 4.0, a)
    qc.cx(b, a)
    qc.ry(-math.pi / 4.0, a)


def _apply_pauli_to_z(qc: Any, pauli: str, qubits: list[int]) -> int:
    """Apply Clifford unitary mapping ``pauli`` (little-endian) to ``Z`` on ``qubits[target]``.

    Returns the logical index ``target`` in ``0..len(qubits)-1``.
    """
    supp: list[int] = []
    for i, ch in enumerate(pauli[::-1]):
        q = qubits[i]
        if ch == "X":
            qc.h(q)
            supp.append(q)
        elif ch == "Y":
            qc.sdg(q)
            qc.h(q)
            supp.append(q)
        elif ch == "Z":
            supp.append(q)
    if not supp:
        return 0
    target_q = supp[0]
    for src_q in supp[1:]:
        qc.cx(src_q, target_q)
    return qubits.index(target_q)


def build_vd_circuits(
    qc: Any,
    coeffs: dict[str, float],
    *,
    symmetries: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Build minimal ancilla-free ``2n``-qubit Transversal Virtual Distillation circuits.

    Optimizations:
      1. Identity terms ``II...I`` require zero quantum circuits.
      2. Terms that are exact ``Z2`` symmetry generators (when ``symmetries`` are
         supplied) are known exactly (or checked on the Z-basis circuit) and skipped
         when exact symmetry pinning is requested.
      3. All weight-1 Pauli terms sharing the same QWC basis (e.g., ``IZ`` and ``ZI``)
         are co-measured in ONE ``2n``-qubit ``B^(2)`` circuit with zero parity CNOTs!
      4. Each multi-qubit Pauli term (weight ``>= 2``) gets its exact ``U_P \\otimes U_P``
         + ``B^(2)`` circuit.
    """
    from qiskit import QuantumCircuit

    n = int(qc.num_qubits)
    if n < 1 or n > 10:
        raise ValueError(f"Transversal VD supports 1 <= n_qubits <= 10 (2n <= 20), got {n}")
    if int(qc.num_clbits) > 0 or any(
        getattr(inst.operation, "name", "") == "measure" for inst in qc.data
    ):
        raise ValueError("Transversal VD requires a unitary state-preparation circuit without measurements")

    grouped = group_pauli_terms(coeffs)
    id_offset = float(grouped["identity_offset"])
    sym_map = {str(s["pauli"]).strip().upper(): int(s["eigenvalue"]) for s in (symmetries or [])}

    # Partition non-identity terms into:
    #   (a) weight-1 QWC groups (all co-measured in ONE circuit per basis!)
    #   (b) multi-qubit terms (weight >= 2, one U_P \otimes U_P + B^(2) circuit each)
    weight1_terms: dict[str, float] = {}
    multi_terms: list[tuple[str, float]] = []
    exact_sym_terms: dict[str, tuple[float, int]] = {}

    for term, coeff in sorted(coeffs.items()):
        t = str(term).strip().upper()
        c = float(coeff)
        if abs(c) == 0.0 or set(t) <= {"I"}:
            continue
        if t in sym_map:
            exact_sym_terms[t] = (c, sym_map[t])
            continue
        weight = sum(1 for ch in t if ch != "I")
        if weight == 1:
            weight1_terms[t] = c
        else:
            multi_terms.append((t, c))

    circuits: list[Any] = []
    specs: list[dict[str, Any]] = []

    def _two_copy_base() -> Any:
        c2 = QuantumCircuit(2 * n)
        c2.compose(qc, qubits=list(range(n)), inplace=True)
        c2.compose(qc, qubits=list(range(n, 2 * n)), inplace=True)
        return c2

    # 1. Weight-1 QWC groups (e.g. {IZ, ZI} -> 1 circuit!)
    if weight1_terms:
        w1_grouped = group_pauli_terms(weight1_terms)
        for grp in w1_grouped["groups"]:
            basis = str(grp["basis"])
            c2 = _two_copy_base()
            # Rotate both copies into the QWC basis
            for i, ch in enumerate(basis[::-1]):
                if ch == "X":
                    c2.h(i)
                    c2.h(n + i)
                elif ch == "Y":
                    c2.sdg(i)
                    c2.h(i)
                    c2.sdg(n + i)
                    c2.h(n + i)
            for i in range(n):
                _apply_b2(c2, i, n + i)
            c2.measure_all()
            # Map each weight-1 term to its single logical support index
            term_targets: list[dict[str, Any]] = []
            for t, c in sorted(grp["terms"].items()):
                t_idx = next(i for i, ch in enumerate(t[::-1]) if ch != "I")
                term_targets.append({"term": t, "coefficient": float(c), "target_index": int(t_idx)})
            circuits.append(c2)
            specs.append({
                "kind": "weight1_qwc",
                "basis": basis,
                "terms": term_targets,
            })

    # 2. Higher-weight Pauli terms (exact U_P \otimes U_P + B^(2))
    for term, coeff in multi_terms:
        c2 = _two_copy_base()
        t_idx = _apply_pauli_to_z(c2, term, list(range(n)))
        _apply_pauli_to_z(c2, term, list(range(n, 2 * n)))
        for i in range(n):
            _apply_b2(c2, i, n + i)
        c2.measure_all()
        circuits.append(c2)
        specs.append({
            "kind": "multi_qubit_pauli",
            "basis": term,
            "terms": [{"term": term, "coefficient": float(coeff), "target_index": int(t_idx)}],
        })

    return {
        "n_logical_qubits": n,
        "n_physical_qubits": 2 * n,
        "identity_offset": id_offset,
        "exact_symmetry_terms": exact_sym_terms,
        "circuits": circuits,
        "specs": specs,
    }


def _eval_b2_counts(
    counts: dict[str, int],
    n: int,
    term_targets: list[dict[str, Any]],
    *,
    m3_calibration: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Compute exact coupled mean and delta-method variance of ``Tr(O_g rho^2) / Tr(rho^2)``.

    For each shot bitstring ``b`` (length ``2n``, Qiskit little-endian where qubit ``q``
    is bit ``(idx >> q) & 1``):
      - ``s(b) = prod_{i=0}^{n-1} (-1)^{m_{a_i} * m_{b_i}}`` (SWAP eigenvalue ``+-1``)
      - For target logical qubit ``t``: ``z_t(b) = +1 if (0,0), -1 if (1,0), 0 if m_{b_t} == 1``
      - Weighted observable sample: ``num(b) = s(b) * sum_k c_k z_{t_k}(b)``
      - Purity denominator sample: ``den(b) = s(b)``
    When ``m3_calibration`` is provided, applies scalable M3 readout mitigation to
    the ``2n``-qubit distribution before evaluating the coupled ratio estimator
    (eliminating post-``B^(2)`` linear readout corruption).
    Exact coupled ratio estimator: ``E_g = mean(num) / mean(den)``, with exact
    delta-method variance ``Var(E_g) = Var(num - E_g * den) / (N * mean(den)^2)``.
    """
    total = sum(int(v) for v in counts.values() if int(v) > 0)
    if total <= 0:
        raise ValueError("VD circuit returned zero counts")

    # Build normalized (quasi-)probability weights over 2n-bit strings.
    weights: dict[str, float]
    m3_var_scale = 1.0
    if m3_calibration is not None:
        from .readout_m3 import mitigate_counts_m3
        m3_res = mitigate_counts_m3(counts, m3_calibration)
        weights = {str(k): float(v) for k, v in m3_res["quasi_probs"].items()}
        # Inflate shot variance conservatively by sum(|q_i|)^2 (1-norm of M3 quasi-probs)
        l1_norm = sum(abs(v) for v in weights.values())
        m3_var_scale = max(1.0, l1_norm * l1_norm)
    else:
        weights = {str(k): float(int(v)) / float(total) for k, v in counts.items() if int(v) > 0}

    mean_num = 0.0
    mean_den = 0.0
    term_means = {t["term"]: 0.0 for t in term_targets}
    samples: list[tuple[float, float, float]] = []  # (num_b, den_b, weight)

    for raw_bs, w_raw in weights.items():
        w = float(w_raw)
        if abs(w) == 0.0:
            continue
        clean = str(raw_bs).replace(" ", "")
        if len(clean) < 2 * n:
            clean = clean.zfill(2 * n)
        idx = int(clean[-(2 * n):], 2)

        s_val = 1.0
        for i in range(n):
            ma = (idx >> i) & 1
            mb = (idx >> (n + i)) & 1
            if ma == 1 and mb == 1:
                s_val = -s_val

        num_b = 0.0
        for t in term_targets:
            ti = int(t["target_index"])
            ma_t = (idx >> ti) & 1
            mb_t = (idx >> (n + ti)) & 1
            if mb_t == 0:
                z_eff = 1.0 if ma_t == 0 else -1.0
                val_t = s_val * z_eff
                term_means[t["term"]] += w * val_t
                num_b += float(t["coefficient"]) * val_t

        mean_num += w * num_b
        mean_den += w * s_val
        samples.append((num_b, s_val, w))

    if not math.isfinite(mean_den) or mean_den <= 1e-6:
        raise ValueError(f"VD purity denominator collapsed: Tr(rho^2) = {mean_den:.6f}")

    ratio = mean_num / mean_den
    # Exact coupled delta-method variance: Y(b) = num(b) - ratio * den(b) (mean == 0!)
    var_y = sum(abs(w) * ((num_b - ratio * den_b) ** 2) for num_b, den_b, w in samples)
    denom_dof = float(total - 1) if total > 1 else float(total)
    var_ratio = max(0.0, (m3_var_scale * var_y) / (denom_dof * mean_den * mean_den))

    # Purity SE
    var_den = max(0.0, m3_var_scale * max(0.0, 1.0 - mean_den * mean_den) / denom_dof)
    term_exps = {t["term"]: float(term_means[t["term"]] / mean_den) for t in term_targets}
    return {
        "energy_contribution": float(ratio),
        "variance": float(var_ratio),
        "std_error": float(math.sqrt(var_ratio)),
        "purity": float(mean_den),
        "purity_std_error": float(math.sqrt(var_den)),
        "shots": int(total),
        "term_expectations": term_exps,
    }


def evaluate_vd_counts(
    vd_plan: dict[str, Any],
    counts_list: list[dict[str, int]],
    *,
    constant_energy: float = 0.0,
    min_purity: float = 0.40,
    raw_baseline_energy: float | None = None,
    raw_baseline_uncertainty: float | None = None,
    m3_calibration: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Evaluate general ``N``-qubit Transversal Virtual Distillation energy and CIs.

    Refuses fail-closed if the mean measured state purity ``Tr(rho^2) < min_purity``
    (indicating a noise regime too severe for quadratic suppression).
    """
    specs = vd_plan["specs"]
    n = int(vd_plan["n_logical_qubits"])
    if len(counts_list) != len(specs):
        raise ValueError(f"expected {len(specs)} VD count dicts, got {len(counts_list)}")

    elec_energy = float(vd_plan["identity_offset"])
    for _t, (c, eig) in vd_plan.get("exact_symmetry_terms", {}).items():
        elec_energy += float(c) * float(eig)

    total_var = 0.0
    purities: list[float] = []
    circuit_summaries: list[dict[str, Any]] = []
    per_term: dict[str, float] = {}

    for spec, cnts in zip(specs, counts_list):
        ev = _eval_b2_counts(cnts, n, spec["terms"], m3_calibration=m3_calibration)
        elec_energy += ev["energy_contribution"]
        total_var += ev["variance"]
        purities.append(ev["purity"])
        per_term.update(ev["term_expectations"])
        circuit_summaries.append({
            "kind": spec["kind"],
            "basis": spec["basis"],
            "shots": ev["shots"],
            "energy_contribution": ev["energy_contribution"],
            "std_error": ev["std_error"],
            "purity": ev["purity"],
            "purity_std_error": ev["purity_std_error"],
        })

    mean_purity = float(np.mean(purities)) if purities else 1.0
    min_obs_purity = float(np.min(purities)) if purities else 1.0
    if min_obs_purity < float(min_purity):
        return {
            "ok": False,
            "error": (
                f"VD state purity collapsed below fail-closed floor: "
                f"min Tr(rho^2)={min_obs_purity:.4f} < {min_purity}"
            ),
            "mean_purity": mean_purity,
            "min_purity": min_obs_purity,
        }

    total_energy = float(elec_energy + float(constant_energy))
    se = math.sqrt(max(0.0, total_var))
    diff_raw = (elec_energy - float(raw_baseline_energy)) if raw_baseline_energy is not None else 0.0
    diff_se = math.sqrt(max(0.0, total_var + float(raw_baseline_uncertainty or 0.0) ** 2))
    diff_ci = Z95 * diff_se

    return {
        "ok": True,
        "method": "VD",
        "mitigated": float(elec_energy),
        "energy": total_energy,
        "uncertainty": float(se),
        "ci95_mha": float(Z95 * se * 1000.0),
        "mean_purity": mean_purity,
        "min_purity": min_obs_purity,
        "purity_floor": float(min_purity),
        "model_adequate": bool(min_obs_purity >= float(min_purity)),
        "per_term_expectations": per_term,
        "circuits": circuit_summaries,
        "difference_ha": float(diff_raw),
        "difference_ci95_ha": float(diff_ci),
        "difference_ci95_mha": float(diff_ci * 1000.0),
        "difference_significant": bool(raw_baseline_energy is not None and abs(diff_raw) > diff_ci),
    }


__all__ = [
    "build_vd_circuits",
    "evaluate_vd_counts",
]
