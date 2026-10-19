"""Slice 26: barrel shift + alias pipeline.

Production bar: routing direction pinned against the paper's convention
(right-rotation mistarget caught live); the pipeline proves over all
inputs x all MBU records (corrections genuinely exercised, not present).
"""

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    alias_step_spec,
    barrel_rotl4_spec,
    build_alias_step,
    build_barrel_rotl4,
    rotl_words,
)


def test_barrel_shift_proves_and_direction_pinned():
    assert rotl_words(0b0001, 1, 4) == 0b0010
    assert rotl_words(0b1000, 1, 4) == 0b0001  # cyclic, not shifted out
    with pytest.raises(ValueError):
        rotl_words(16, 1, 4)
    (b, regs) = build_barrel_rotl4()
    assert regs["ccx"] == 5  # 2 (rotl-0/2) + 3 (4-cycle stage)
    res = KickmixEvaluator().evaluate(b, barrel_rotl4_spec(), num_tests=128, seed=7)
    assert res["valid"] is True and res["tests"] == 64


def test_alias_pipeline_proves_over_inputs_x_records():
    (b, regs) = build_alias_step()
    assert regs["ccx"] == 17
    assert (regs["keep"], regs["alt"]) == (2, 1)
    res = KickmixEvaluator().evaluate(b, alias_step_spec(), num_tests=64, seed=7)
    # 8 inputs x 8 measurement records, fully enumerated: every MBU
    # correction (keep-erase flips, h-erase flip + conditioned Z) fires
    # somewhere in here — none of it is untested present.
    assert res["valid"] is True and res["tests"] == 64
    # dropping the keep-erase flip on the set bit leaves (-1)^r1 unfixed:
    # same pipeline minus that one correction -> phase mismatch (the
    # zero-phase spec makes erasure load-bearing, not decorative).
    from kubona.codesign import Builder
    from kubona.codesign.horner import emit_add, emit_sub
    b2 = Builder(13)
    for q in (4, 5, 6, 7, 8, 9, 10, 11, 12):
        b2.alloc(q)
    b2.x(10)  # keep = 2
    b2.cx(0, 4)
    b2.cx(1, 5)
    emit_sub(b2, [4, 5, 6], [9, 10], [7, 8])
    b2.cx(6, 12)
    emit_add(b2, [4, 5, 6], [9, 10], [7, 8])
    b2.cx(0, 4)
    b2.cx(1, 5)
    r0 = b2.hmr(9)
    r1 = b2.hmr(10)
    # MISSING: global_flip(cond=(r1,)) for the set keep bit (the fault)
    b2.cx(2, 3)
    b2.cx(2, 11)
    b2.x(11)
    b2.x(12)
    b2.ccx(12, 11, 3)
    b2.x(12)
    b2.cx(2, 11)
    b2.x(11)
    r2 = b2.hmr(12)
    b2.global_flip(cond=(r2,))
    b2.z(1, cond=(r2,))
    for q in (4, 5, 6, 7, 8, 11):
        b2.free(q)
    bad = KickmixEvaluator().evaluate(b2, alias_step_spec(), num_tests=64, seed=7)
    assert bad["valid"] is False
