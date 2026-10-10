"""Observable-error-bounded Hamiltonian truncation.

Depth-cutting is only honest with a measured error bound: drop the
smallest-|coefficient| Pauli terms progressively and report the observable
error against the full Hamiltonian at each step, instead of assuming small
coefficients mean small effects. The epsilon-threshold point (smallest kept
set with error <= eps) turns "how much can we cut" into a governed trade.

Chemistry grounding without new dependencies: H2 STO-3G Pauli terms are
built from PySCF integrals through an explicit Jordan-Wigner mapping and
verified against the FCI classical anchor before any curve is trusted.
"""

from __future__ import annotations

from typing import Any

import numpy as np

_PAULI_MATS = {
    "I": np.array([[1, 0], [0, 1]], complex),
    "X": np.array([[0, 1], [1, 0]], complex),
    "Y": np.array([[0, -1j], [1j, 0]], complex),
    "Z": np.array([[1, 0], [0, -1]], complex),
}


def _mul_terms(a: dict[str, complex], b: dict[str, complex]) -> dict[str, complex]:
    """Multiply two Pauli-sum dicts ({pauli_str: coeff})."""
    mult = {
        ("I", "I", "I"), ("I", "X", "X"), ("I", "Y", "Y"), ("I", "Z", "Z"),
        ("X", "I", "X"), ("X", "X", "I"), ("X", "Y", "Z"), ("X", "Z", "Y"),
        ("Y", "I", "Y"), ("Y", "X", "Z"), ("Y", "Y", "I"), ("Y", "Z", "X"),
        ("Z", "I", "Z"), ("Z", "X", "Y"), ("Z", "Y", "X"), ("Z", "Z", "I"),
    }
    phase = {
        ("X", "Y"): 1j, ("Y", "Z"): 1j, ("Z", "X"): 1j,
        ("Y", "X"): -1j, ("Z", "Y"): -1j, ("X", "Z"): -1j,
    }
    table = {(x, y): (z, phase.get((x, y), 1)) for x, y, z in mult}
    out: dict[str, complex] = {}
    for sa, ca in a.items():
        for sb, cb in b.items():
            chars, ph = [], 1
            for x, y in zip(sa, sb):
                z, p = table[(x, y)]
                chars.append(z)
                ph *= p
            key = "".join(chars)
            out[key] = out.get(key, 0) + ca * cb * ph
    return {k: v for k, v in out.items() if abs(v) > 1e-12}


def _jw_ops(n: int) -> tuple[list[dict[str, complex]], list[dict[str, complex]]]:
    """Jordan-Wigner a_j and a†_j as Pauli sums (qubit j is string position j)."""
    annihilate, create = [], []
    for j in range(n):
        aj: dict[str, complex] = {}
        cj: dict[str, complex] = {}
        # (X_j + iY_j)/2 with Z string on k<j; (X_j - iY_j)/2 for creation.
        for op, coeff in (("X", 0.5), ("Y", 0.5j)):
            s = ["I"] * n
            s[j] = op
            for k in range(j):
                s[k] = "Z"
            aj["".join(s)] = aj.get("".join(s), 0) + coeff
        for op, coeff in (("X", 0.5), ("Y", -0.5j)):
            s = ["I"] * n
            s[j] = op
            for k in range(j):
                s[k] = "Z"
            cj["".join(s)] = cj.get("".join(s), 0) + coeff
        annihilate.append(aj)
        create.append(cj)
    return annihilate, create


def h2_pauli_terms(bond_angstrom: float = 0.74) -> list[tuple[str, float]]:
    """H2 STO-3G qubit Hamiltonian via PySCF integrals + Jordan-Wigner.

    Spin orbitals ordered [0a, 0b, 1a, 1b]; chemists'-notation ERIs expanded
    to spin orbitals. Verified against the FCI anchor by the test suite —
    never trusted blind.
    """
    from pyscf import ao2mo, gto, scf

    mol = gto.M(atom=f"H 0 0 0; H 0 0 {bond_angstrom}", basis="sto-3g", verbose=0)
    mf = scf.RHF(mol).run()
    n_spatial = mf.mo_coeff.shape[1]
    h1 = mf.mo_coeff.T @ mf.get_hcore() @ mf.mo_coeff
    eri_full = ao2mo.kernel(mol, mf.mo_coeff, compact=False).reshape(
        (n_spatial,) * 4)
    n = 2 * n_spatial
    e_nuc = float(mol.energy_nuc())

    def so(p: int, s: int) -> int:
        return 2 * p + s

    h1_so = np.zeros((n, n))
    for p in range(n_spatial):
        for q in range(n_spatial):
            for s in (0, 1):
                h1_so[so(p, s), so(q, s)] = h1[p, q]
    eri_so = np.zeros((n,) * 4)
    for p in range(n_spatial):
        for q in range(n_spatial):
            for r in range(n_spatial):
                for s_ in range(n_spatial):
                    v = eri_full[p, r, q, s_]
                    for a in (0, 1):
                        for b in (0, 1):
                            eri_so[so(p, a), so(q, b), so(r, a), so(s_, b)] = v

    annihilate, create = _jw_ops(n)
    ham: dict[str, complex] = {}
    for p in range(n):
        for q in range(n):
            if h1_so[p, q] == 0:
                continue
            for k, v in _mul_terms(create[p], annihilate[q]).items():
                ham[k] = ham.get(k, 0) + h1_so[p, q] * v
    for p in range(n):
        for q in range(n):
            for r in range(n):
                for s_ in range(n):
                    if eri_so[p, q, r, s_] == 0:
                        continue
                    op = _mul_terms(_mul_terms(create[p], create[q]),
                                    _mul_terms(annihilate[r], annihilate[s_]))
                    # Normal-ordered sign: E_pq E_rs expands with a minus on
                    # c†c†cc (verified against the FCI anchor — plus here
                    # turns repulsion attractive and collapses the spectrum).
                    for k, v in op.items():
                        ham[k] = ham.get(k, 0) - 0.5 * eri_so[p, q, r, s_] * v
    terms = [(k, float(v.real)) for k, v in ham.items() if abs(v) > 1e-10]
    # Nuclear repulsion is a constant shift: fold it into the identity so
    # ground_energy() is directly comparable to total (FCI) energies.
    terms = [(k, v + (e_nuc if k == "I" * n else 0.0)) for k, v in terms]
    return terms


def pauli_matrix(terms: list[tuple[str, float]]) -> np.ndarray:
    """Exact matrix for a Pauli sum (small n only — verification scale)."""
    n = len(terms[0][0])
    dim = 1 << n
    out = np.zeros((dim, dim), complex)
    for ps, coeff in terms:
        mat = _PAULI_MATS[ps[0]]
        for ch in ps[1:]:
            mat = np.kron(mat, _PAULI_MATS[ch])
        out += coeff * mat
    return out


def ground_energy(terms: list[tuple[str, float]]) -> float:
    """Lowest eigenvalue of the Pauli sum (exact diagonalization)."""
    return float(np.linalg.eigvalsh(pauli_matrix(terms))[0])


def cnot_estimate(terms: list[tuple[str, float]]) -> int:
    """2-CNOT staircase cost per Pauli-exponential term: 2*(weight-1)."""
    total = 0
    for ps, _ in terms:
        w = sum(1 for ch in ps if ch != "I")
        if w > 1:
            total += 2 * (w - 1)
    return total


def truncate_curve(terms: list[tuple[str, float]],
                   keep_fractions: list[float] | None = None) -> list[dict[str, Any]]:
    """Depth-vs-error curve: keep the largest-|coeff| fraction, measure the
    ground-energy error against the full Hamiltonian at each step."""
    if keep_fractions is None:
        keep_fractions = [1.0, 0.8, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1]
    reference = ground_energy(terms)
    ordered = sorted(terms, key=lambda t: -abs(t[1]))
    curve = []
    for frac in keep_fractions:
        k = max(1, round(len(ordered) * frac))
        kept = ordered[:k]
        err = abs(ground_energy(kept) - reference)
        curve.append({"keep_fraction": frac, "n_terms": len(kept),
                      "est_cnots": cnot_estimate(kept),
                      "abs_error_ha": err})
    return curve


def epsilon_threshold(curve: list[dict[str, Any]], eps: float) -> dict[str, Any]:
    """Smallest kept set with error <= eps. Fail-closed: no point qualifies
    unless measured (an unmeasured cut is a guess, not a threshold)."""
    candidates = [p for p in curve if p["abs_error_ha"] <= eps]
    if not candidates:
        raise ValueError(f"no truncation point meets eps={eps}: deepen, do not guess")
    return min(candidates, key=lambda p: p["n_terms"])
