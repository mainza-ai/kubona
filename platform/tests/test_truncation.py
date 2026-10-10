"""Observable-error-bounded truncation: verified Hamiltonian, honest curves.

The H2 Jordan-Wigner build is verified against the FCI classical anchor
(-1.137284 Ha) — during development this caught a two-body sign error AND
a missing nuclear-repulsion shift, both invisible without the anchor.
"""

import numpy as np
import pytest

from kubona.eval.truncation import (
    _jw_ops,
    cnot_estimate,
    epsilon_threshold,
    ground_energy,
    h2_pauli_terms,
    pauli_matrix,
    truncate_curve,
)

FCI_H2 = -1.137284


def test_h2_matches_fci_anchor():
    terms = h2_pauli_terms()
    assert len(terms) == 15  # known H2 JW term count
    assert ground_energy(terms) == pytest.approx(FCI_H2, abs=1e-4)


def test_jw_fermion_algebra():
    from kubona.eval.truncation import _mul_terms

    def mat(d):
        out = np.zeros((16, 16), complex)
        for k, v in d.items():
            out += v * pauli_matrix([(k, 1.0)])
        return out

    ann, cre = _jw_ops(4)
    A = [mat(a) for a in ann]
    C = [mat(c) for c in cre]
    I = np.eye(16)
    for i in range(4):
        for j in range(4):
            assert np.allclose(A[i] @ C[j] + C[j] @ A[i], I if i == j else 0)


def test_curve_endpoints_honest():
    terms = h2_pauli_terms()
    curve = truncate_curve(terms)
    full = [p for p in curve if p["keep_fraction"] == 1.0][0]
    assert full["abs_error_ha"] < 1e-9  # eigvalsh roundoff, not model error
    assert full["n_terms"] == len(terms)
    # Aggressive cuts must show error (a curve that stays flat is lying).
    deep = [p for p in curve if p["keep_fraction"] == 0.1][0]
    assert deep["abs_error_ha"] > 0.1
    assert deep["est_cnots"] <= full["est_cnots"]


def test_cnot_estimate_h2():
    assert cnot_estimate(h2_pauli_terms()) == 36


def test_epsilon_threshold_picks_min_terms():
    terms = h2_pauli_terms()
    curve = truncate_curve(terms)
    loose = epsilon_threshold(curve, 1.0)
    assert loose["abs_error_ha"] <= 1.0
    # No smaller qualifying set exists.
    for p in curve:
        if p["n_terms"] < loose["n_terms"]:
            assert p["abs_error_ha"] > 1.0


def test_epsilon_threshold_fail_closed():
    terms = h2_pauli_terms()
    curve = truncate_curve(terms)
    # Negative eps is unmeetable even by the full Hamiltonian: must refuse.
    with pytest.raises(ValueError, match="no truncation point meets"):
        epsilon_threshold(curve, -1.0)
