"""Slice 7: exact divstep walks — every invariant + inverse proven per walk."""

import pytest

from kubona.codesign import batch_report, bit, run_walk, step, v2


def test_v2_and_bit_helpers():
    assert v2(12) == 2 and v2(7) == 0 and v2(16) == 4
    with pytest.raises(ValueError):
        v2(0)
    assert bit(5, 0) == 1 and bit(5, 1) == 0 and bit(-1, 3) == 1


def test_step_cases_match_eq_a2():
    # g even
    assert step((0, 7, 4, 0, 1))[0] == (1, 7, 2, 0, 1)
    # swap (z>=0, g odd)
    s, (ns, nb) = step((0, 7, 5, 0, 1))[0], step((0, 7, 5, 0, 1))[1:]
    assert (ns, nb) == (1, 1) and s[0] == 0 and s[1] == 5
    # negative-counter nonswap
    s, (ns, nb) = step((-2, 7, 5, 2, 1))[0], step((-2, 7, 5, 2, 1))[1:]
    assert (ns, nb) == (0, 1) and s == (-1, 7, 6, 4, 3)
    # frozen
    assert step((3, 1, 0, 8, 5))[0] == (3, 1, 0, 8, 5)
    with pytest.raises(ValueError):
        run_walk(256, 3)  # even p refuses
    with pytest.raises(ValueError):
        run_walk(257, 257)  # w out of range refuses


def test_single_walk_inverse_proven():
    rep = run_walk(257, 123)
    assert (123 * rep["inverse"]) % 257 == 1
    assert rep["hist"][2] == 4  # pinned excursion profile
    assert rep["checks"]["depth2"] == 4  # every ambiguous depth-2 decoded


def test_full_batch_all_inverses_proven():
    rep = batch_report(257, list(range(1, 257)))
    assert rep["proven"] == 256
    assert rep["worst_active_steps"] == 19
    assert rep["depth_hist"] == {2: 631, 3: 267, 4: 109, 5: 47, 6: 17, 7: 7, 8: 3}
    assert rep["checks"]["swap_visible"] == rep["checks"]["determinant"] == 4064
    assert rep["checks"]["depth2"] == 631
    # window invariant + both A11 decoder forms at every z'<0 nonswap
    assert rep["checks"]["window"] == 507
    assert rep["checks"]["decode1"] == rep["checks"]["decode2"] == 507
    assert rep["checks"]["divisibility"] == 1913


def test_frozen_zero_and_second_prime():
    rep = run_walk(257, 0)
    assert rep["frozen"] is True and rep["inverse"] is None
    rep = batch_report(17, list(range(1, 17)))
    assert rep["proven"] == 16


def test_schedule_cut_paper_pattern():
    # SI II.2 at toy scale: worst-case vs fitted schedule from measured
    # stopping times. budget=0 recovers worst; positive budget cuts with a
    # counted tail (never hidden).
    from kubona.codesign import schedule_cut
    full = schedule_cut(257, list(range(1, 257)), 0.0)
    assert full["cut"] == full["worst"] == 19 and full["over"] == 0
    cut = schedule_cut(257, list(range(1, 257)), 0.05)
    assert cut["cut"] <= cut["worst"] and cut["over_rate"] <= 0.05 + 1e-9
    assert cut["saved_steps"] == cut["worst"] - cut["cut"] >= 0
    with pytest.raises(ValueError):
        schedule_cut(257, [0], 0.0)  # frozen-only: no schedule to fit
    with pytest.raises(ValueError):
        schedule_cut(257, list(range(1, 257)), 1.0)


def test_specialized_euclid_rows_labeled_estimates():
    from kubona.codesign import specialized_euclid_rows, union_bound
    rows = specialized_euclid_rows()
    assert len(rows) == 3
    assert abs(union_bound(rows) - 8.1e-5) < 1e-9
