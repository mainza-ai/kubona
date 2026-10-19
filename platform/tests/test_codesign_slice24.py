"""Slice 24: decoder hardware + Coulomb lines.

Production bar: the branch rules run as circuits (proven exhaustive, with
a vacuity guard); the line table is fit and swept over the full attainable
domain with parity-correct scaling (the sqrt(2) mistarget stays pinned by
the odd-line construction) — coarser than the paper's search, stated.
"""

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    attainable_d2,
    build_decoder_circuit,
    decoder_circuit_spec,
    fit_coulomb_lines,
)


def test_decoder_circuits_proven_exhaustive():
    expect_t = {2: 0, 3: 0, 4: 8, 5: 68}
    expect_n = {2: 4, 3: 16, 4: 64, 5: 256}
    for depth in (2, 3, 4, 5):
        (b, regs) = build_decoder_circuit(depth)
        assert regs["ccx"] == expect_t[depth]
        res = KickmixEvaluator().evaluate(b, decoder_circuit_spec(depth),
                                           num_tests=512, seed=7)
        assert res["valid"] is True and res["tests"] == expect_n[depth]
    with pytest.raises(ValueError):
        build_decoder_circuit(9)  # beyond exhaustive-proof scope (2^16 inputs)
    # vacuity guard: extra output flip is detected, not absorbed
    (b2, regs0) = build_decoder_circuit(2)
    b2.x(regs0["b"])
    res = KickmixEvaluator().evaluate(b2, decoder_circuit_spec(2),
                                       num_tests=8, seed=7)
    assert res["valid"] is False


def test_coulomb_lines_full_domain():
    d2 = attainable_d2(8)
    assert len(d2) == 139637
    rep = fit_coulomb_lines(8)
    assert rep["n_lines"] == 128 and rep["n_points"] == 139637
    assert all(c >= 565 for _, _, c in rep["lines"].values())
    # scale-weighted minimax reaches 1.34e-6 < 2e-6 (paper: 1.91e-6): the
    # structure plus fitting to committed error recovers the bound with our
    # own deterministic method — no borrowed constants anywhere.
    assert 1.0e-6 < rep["worst"] < 1.7e-6
    assert rep["within_tol"] is True
    assert 1 <= rep["worst_at_d2"] <= 195075
    with pytest.raises(ValueError):
        attainable_d2(9)
