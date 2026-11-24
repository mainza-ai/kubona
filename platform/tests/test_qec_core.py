"""Phase 1 QEC core acceptance tests (zero-GPL Apache-2.0/MIT stack).

Verifies:
1. Bravyi et al. (2024) BB [[72, 12, 6]] exact distance PROVED via HiGHS MILP.
2. Gross code [[144, 12, 12]] exact (n=144, k=12), FOM=12.0, indecomposable, catalog match.
3. All 5 published Bravyi et al. 2024 BB specs reproduce exact (n, k) and indecomposability.
4. Direct sum Gross (+) Gross ([[288, 24, 12]]) AND row-scrambled M*(Gross (+) Gross)
   are both rejected as decomposable (Gate 2 / G10).
5. Qubit-relabelled [[72, 12, 6]] and [[144, 12, 12]] are detected as known via
   Apache-2.0 rustworkx VF2 isomorphism (Gate 3 / G1).
6. Hadamard-swapped CSS code is detected as LC-equivalent (Gate 3).
7. upper_bound vs proved are cleanly distinguished and upper_bound is never claimable.
"""

from __future__ import annotations

import numpy as np
import pytest

pytest.importorskip("rustworkx")
pytest.importorskip("highspy")
pytest.importorskip("ldpc")

from kubona.qec import (
    KNOWN_BB_SPECS,
    are_tanner_isomorphic,
    build_bb_code,
    build_builtin_catalog,
    css_parameters,
    direct_sum_css,
    evaluate_bb_code,
    evaluate_css_code,
    is_decomposable_css,
    is_lc_equivalent_css,
    lookup_catalog,
    relabel_qubits_css,
)


def test_bb_72_12_6_proved_distance_milp():
    """BB(6,6, A=x^3+y+y^2, B=y^3+x+x^2) -> [[72, 12, 6]], distance PROVED."""
    res = evaluate_bb_code(
        6,
        6,
        a_terms=[(3, 0), (0, 1), (0, 2)],
        b_terms=[(0, 3), (1, 0), (2, 0)],
        name="bb_72_12_6",
        prove_distance=True,
        time_limit_s=30.0,
    )
    assert res["result_version"] == "experiment-result-v1"
    assert res["success"] is True
    assert res["n"] == 72
    assert res["k"] == 12
    assert res["d"] == 6
    assert res["d_kind"] == "proved"
    assert res["fom"] == pytest.approx(6.0)
    assert res["decomposable"] is False
    assert res["catalog_known"] is True
    assert res["catalog_match"] == "bravyi2024_bb_72_12_6"
    assert res["claimable"] is False  # known catalog code is not claimable as new


def test_gross_code_144_12_12_parameters_and_upper_bound():
    """Gross code BB(12,6) -> [[144, 12, 12]], FOM = 12.0, upper_bound labelled honestly."""
    res = evaluate_bb_code(
        12,
        6,
        a_terms=[(3, 0), (0, 1), (0, 2)],
        b_terms=[(0, 3), (1, 0), (2, 0)],
        name="gross_144",
        prove_distance=False,
        bposd_trials=36,
    )
    assert res["n"] == 144
    assert res["k"] == 12
    assert res["d_kind"] == "upper_bound"
    assert res["d"] is not None and res["d"] >= 12
    # BP-OSD upper bound on the gross code hits d=12 reliably (FOM = 12*12^2/144 = 12.0)
    if res["d"] == 12:
        assert res["fom"] == pytest.approx(12.0)
    assert res["decomposable"] is False
    assert res["catalog_known"] is True
    assert res["catalog_match"] == "bravyi2024_gross_144_12_12"
    assert res["claimable"] is False  # upper_bound never enters a claim


def test_all_published_bravyi2024_specs_match_exact_nk():
    """All 5 Bravyi et al. 2024 BB specs reproduce exact (n, k) and are indecomposable."""
    for spec in KNOWN_BB_SPECS:
        hx, hz = build_bb_code(spec.ell, spec.m, spec.a_terms, spec.b_terms)
        params = css_parameters(hx, hz)
        assert params["n"] == spec.n, spec.name
        assert params["k"] == spec.k, spec.name
        decomp = is_decomposable_css(hx, hz)
        assert decomp["decomposable"] is False, spec.name


def test_direct_sum_and_row_scrambled_gross_codes_detected_decomposable():
    """Gate 2 / G10: [[288, 24, 12]] = Gross (+) Gross is caught even after invertible row mixing."""
    hx, hz = build_bb_code(12, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    hx2, hz2 = direct_sum_css(hx, hz, hx, hz)
    params = css_parameters(hx2, hz2)
    assert params["n"] == 288
    assert params["k"] == 24

    # 1. Raw block-diagonal direct sum
    raw_decomp = is_decomposable_css(hx2, hz2)
    assert raw_decomp["decomposable"] is True
    assert raw_decomp["component_qubit_sizes"] == [144, 144]

    # 2. Row-scrambled direct sum: add rows from block 2 into block 1 (invertible over GF(2))
    # Naive connected components on the raw check matrix would see 1 component!
    hx_mix = hx2.copy()
    hz_mix = hz2.copy()
    for i in range(hx.shape[0]):
        hx_mix[i] ^= hx_mix[hx.shape[0] + i]
        hz_mix[i] ^= hz_mix[hz.shape[0] + i]
    mixed_decomp = is_decomposable_css(hx_mix, hz_mix)
    assert mixed_decomp["decomposable"] is True
    assert mixed_decomp["component_qubit_sizes"] == [144, 144]


def test_relabelled_gross_code_and_lc_hadamard_swap_caught():
    """Gate 3 / G1: rustworkx VF2 catches qubit-relabelled codes and Hadamard X<->Z swap."""
    catalog = build_builtin_catalog()
    rng = np.random.default_rng(42)

    hx, hz = build_bb_code(12, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    perm = rng.permutation(144).tolist()
    hx_rel, hz_rel = relabel_qubits_css(hx, hz, perm)

    assert are_tanner_isomorphic(hx, hz, hx_rel, hz_rel) is True
    hit = lookup_catalog(hx_rel, hz_rel, catalog=catalog)
    assert hit["known"] is True
    assert hit["matched_name"] == "bravyi2024_gross_144_12_12"
    assert hit["relation"] == "tanner_isomorphic"

    # Hadamard swap on a non-self-dual CSS code (Steane-like asymmetric check weights)
    hx_s = np.array([[1, 1, 1, 1, 0, 0, 0]], dtype=np.uint8)
    hz_s = np.array([[1, 1, 0, 0, 0, 0, 0], [0, 0, 1, 1, 0, 0, 0]], dtype=np.uint8)
    eq, rel = is_lc_equivalent_css(hz_s, hx_s, hx_s, hz_s)
    assert eq is True
    assert rel == "lc_hadamard_swap"


def test_custom_small_css_code_proved_distance():
    """[[7, 1, 3]] Steane code: exact GF(2) rank k=1, MILP proved d=3, indecomposable."""
    H = np.array(
        [
            [1, 0, 1, 0, 1, 0, 1],
            [0, 1, 1, 0, 0, 1, 1],
            [0, 0, 0, 1, 1, 1, 1],
        ],
        dtype=np.uint8,
    )
    res = evaluate_css_code(H, H, name="steane_7_1_3", prove_distance=True, time_limit_s=10.0)
    assert res["n"] == 7
    assert res["k"] == 1
    assert res["d"] == 3
    assert res["d_kind"] == "proved"
    assert res["decomposable"] is False
    assert res["catalog_known"] is False
    assert res["claimable"] is True
