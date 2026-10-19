"""Slice 18: 256-bit walk metrology + key-equation decode verdict.

Production bar: paper-scale measurements from fully verified walks
(seed-fixed regression pins, never paper constants); the packing-fit
verdict states its open discrepancy instead of force-fitting.
"""

import statistics

import pytest

from kubona.codesign import (
    F2_12,
    berlekamp_massey,
    check_decode,
    eps_upper_exact,
    locator_roots,
    multi_error_syndromes,
    survey_walks,
)

P256 = 2**256 - 2**32 - 977


def _walks(n: int, seed: int) -> list[int]:
    import random
    rng = random.Random(seed)
    return [rng.randrange(1, P256) for _ in range(n)]


def test_survey_256_stopping_and_stack_match_paper_shape():
    import time
    t = time.time()
    rep = survey_walks(P256, _walks(2000, 20261018))
    assert time.time() - t < 120  # fully verified, still cheap
    assert rep["n"] == 2000
    # stopping window: paper schedules 533 steps, tests from 499 (Fig divstep(b))
    assert (rep["min_steps"], rep["worst_steps"]) == (503, 529)
    assert rep["outside_window_499_533"] == 0
    assert eps_upper_exact(2000, 0) < 0.002  # our-N tail bound, not paper's 1e-5
    # history stack at jstore=5: paper reports mean 21, reserves 60
    assert round(statistics.mean(rep["stack_depths"]), 1) == 21.2
    assert rep["max_stack"] == 48 and rep["over_stack_60"] == 0
    assert eps_upper_exact(2000, rep["over_stack_60"]) < 0.002


def test_survey_256_packing_fit_states_open_discrepancy():
    rep = survey_walks(P256, _walks(2000, 20261018))
    assert (rep["peak_fill_a"], rep["peak_fill_b"]) == (280, 281)
    # OPEN DISCREPANCY, pinned not hidden: under EXACT two's-complement range
    # widths (brute-force-minimal, no +1 conservatism anywhere), shared sums
    # exceed the paper's two 256-bit registers at every surveyed step — while
    # the walk DATA matches the paper exactly (stopping 503..529, stack mean
    # 21.2). So the gap is in width accounting (exact per-step ranges and/or
    # fitted widths with tails failing by design, SI II.1 step-dependent
    # reuse), not the walk. Resolving it the wrong way (shrinking the model
    # to fit) would be scope-dodging the same fiction class the bar catches.
    assert rep["fits_cap"] is False
    assert rep["over_cap_a_steps"] != [] and rep["over_cap_b_steps"] != []


def test_survey_refusals_and_toy_path():
    with pytest.raises(ValueError):
        survey_walks(P256, [])
    with pytest.raises(ValueError):
        survey_walks(P256, [1, 2], jstore=-1)
    rep = survey_walks(257, [1, 2, 3, 4, 5, 6, 7, 8])
    assert rep["n"] == 8 and rep["fits_cap"] is True


def test_tc_width_exact_and_minimal():
    from kubona.codesign.divwalk import _tc_width
    for lo in range(-8, 9):
        for hi in range(lo, 9):
            k = _tc_width(lo, hi)
            assert -(1 << (k - 1)) <= lo and hi <= (1 << (k - 1)) - 1
            if k > 1:
                assert not (-(1 << (k - 2)) <= lo and hi <= (1 << (k - 2)) - 1)
    with pytest.raises(ValueError):
        _tc_width(3, -3)


def test_key_equation_verdict_both_directions():
    S = multi_error_syndromes(F2_12, 2, [(5, 7), (400, 9), (2701, 11)], 70)
    rep = check_decode(berlekamp_massey(S, F2_12)["sigma"], S, F2_12, 2)
    assert len(rep["omega"]) == 3 and rep["tail_checked"] == 67
    assert rep["tail_zero"] is True and rep["splits_fully"] is True
    assert sorted(rep["roots"]) == [5, 400, 2701]
    # past capacity (40 > 35): key equation holds but splitting fails -> refuse
    S40 = multi_error_syndromes(F2_12, 2, [(i * 100, i + 1) for i in range(40)], 70)
    with pytest.raises(ValueError, match="splits into 1 roots"):
        check_decode(berlekamp_massey(S40, F2_12)["sigma"], S40, F2_12, 2)
    with pytest.raises(ValueError):
        check_decode([1], [], F2_12, 2)
