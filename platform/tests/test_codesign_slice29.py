"""Slice 29: HHL toy task + general-keep alias pipeline.

Production bar: the task composes proven pieces (dyadic select, barrel
shift, Horner accumulate) with fitted tables and proves end-to-end over
the full input domain; alias corrections generalize to every 2-bit keep
with per-keep ANF plans (the pinned default is one instance).
"""

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    alias_step_spec,
    build_alias_step,
    build_hhl_toy_task,
    hhl_toy_fits,
    hhl_toy_spec,
)


def test_hhl_toy_fits_and_task_proves():
    fits = hhl_toy_fits()
    assert {L: (f["g0"], f["g1"], f["max_err"]) for L, f in fits.items()} == {
        "A": (55, 1, 0), "B": (35, 0, 2), "C": (35, 1, 3)}
    (b, regs) = build_hhl_toy_task(fits)
    res = KickmixEvaluator().evaluate(b, hhl_toy_spec(fits), num_tests=64, seed=7)
    assert res["valid"] is True and res["tests"] == 16
    assert res["toffoli"] == regs["ccx"]  # reported count is the measured one
    with pytest.raises(ValueError):
        build_hhl_toy_task({"A": 1})  # fits must cover A/B/C


def test_alias_general_keep_proves():
    for keep in (0, 1, 2, 3):
        for alt in (0, 1):
            (b, regs) = build_alias_step(keep, alt)
            assert regs["keep"] == keep and regs["alt"] == alt
            res = KickmixEvaluator().evaluate(b, alias_step_spec(keep, alt),
                                               num_tests=64, seed=7)
            assert res["valid"] is True and res["tests"] == 64, (keep, alt)
    with pytest.raises(ValueError):
        build_alias_step(keep=4)  # outside 2-bit scope
    with pytest.raises(ValueError):
        alias_step_spec(keep=1, alt=2)
    # dropping the keep=1 CZ correction corrupts its phase: caught.
    from kubona.codesign import Builder
    from kubona.codesign.horner import emit_add, emit_sub
    b2 = Builder(13)
    for q in (4, 5, 6, 7, 8, 9, 10, 11, 12):
        b2.alloc(q)
    b2.x(9)  # keep = 1
    b2.cx(0, 4)
    b2.cx(1, 5)
    emit_sub(b2, [4, 5, 6], [9, 10], [7, 8])
    b2.cx(6, 12)
    emit_add(b2, [4, 5, 6], [9, 10], [7, 8])
    b2.cx(0, 4)
    b2.cx(1, 5)
    r0 = b2.hmr(9)
    b2.global_flip(cond=(r0,))
    r1 = b2.hmr(10)
    b2.cx(2, 3)
    b2.cx(2, 11)
    b2.x(12)
    b2.ccx(12, 11, 3)
    b2.x(12)
    b2.cx(2, 11)
    r2 = b2.hmr(12)
    b2.global_flip(cond=(r2,))
    b2.z(0, cond=(r2,))
    b2.z(1, cond=(r2,))
    # MISSING: cz(0, 1, cond=(r2,)) for the pair term (the fault)
    for q in (4, 5, 6, 7, 8, 11):
        b2.free(q)
    bad = KickmixEvaluator().evaluate(b2, alias_step_spec(keep=1, alt=0),
                                       num_tests=64, seed=7)
    assert bad["valid"] is False
