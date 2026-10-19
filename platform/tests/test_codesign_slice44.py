"""Slice 44: divstep packing discrepancy — precise characterization (no fix).

Production bar: these pins are MEASUREMENTS, not approvals. They prove
exactly where naive shared-256 packing fails, so any future claim to
resolve the discrepancy must overturn one of them (new representation
spec, new per-step assignment, or a shown error here — never silence):
- the paper's prime needs 257 signed bits (f_0 = p is unavoidable);
- valid inputs reach 2^255, which also needs 257 signed bits;
- the first even step needs B = f(257) + gv(2) = 259 signed bits
  (257 unsigned) — every even-start walk, step 1, no exceptions;
- a deep in-window excursion needs 267+267 signed (265+265 unsigned)
  bits — all values live and computed, no known-constant escape.
Together: NO binary encoding of the stated shared registers holds these
states. The paper's working circuit therefore uses a representation /
initial-handling / sharing rule not stated in the SI excerpt we hold.
"""

import pytest

from kubona.codesign.divwalk import _tc_width, run_walk, step
from kubona.codesign.divsteps import PAPER_P256


def _ub(v: int) -> int:
    """Unsigned-minimal width: floor(log2)+1 bits, 1 bit minimum for a live value."""
    return max(1, abs(v).bit_length())


def test_prime_needs_257_signed_bits():
    p = PAPER_P256
    assert p.bit_length() == 256
    assert _tc_width(p, p) == 257
    assert _tc_width(2 ** 255, 2 ** 255) == 257
    assert 2 ** 255 < p  # valid inputs exist above signed-256 range


def test_step_zero_is_almost_all_known():
    # S0 = (0, p, w, 0, 1): only w is an unknown input; the rest are
    # compile-time constants (p public, 0/1 trivial). This is the ONLY
    # step where known-constant hardwiring can hide width.
    p = PAPER_P256
    for w in (1, 2, 3, p - 1):
        z, f, g, fv, gv = 0, p, w, 0, 1
        assert (z, f, g, fv, gv) == (0, p, w, 0, 1)
        assert f == p and gv == 1 and fv == 0 and z == 0


def test_step_one_even_start_overflows():
    # After one even step from S0=(0,p,2,0,1): (1,p,1,0,1). f is still
    # the public prime (live value now, not just initial state), and B
    # must hold f alongside gv — 259 signed bits, 257 unsigned.
    p = PAPER_P256
    s1, _, _ = step((0, p, 2, 0, 1))
    z, f, g, fv, gv = s1
    assert (z, f, g, fv, gv) == (1, p, 1, 0, 1)
    assert _tc_width(g, g) + _tc_width(fv, fv) <= 256  # A fits here
    assert _tc_width(f, f) + _tc_width(gv, gv) == 257 + 2
    assert _ub(f) + _ub(gv) == 256 + 1


def test_deep_excursion_overflow_unsigned_and_signed():
    # Walk terminating at step 525 (in [499,533]: the paper must handle
    # it, modulo its tiny tail budget). At step 422 every value is live
    # and computed — no known-constant escape — yet both shared sums
    # exceed 256 under signed AND unsigned-minimal accounting.
    p = PAPER_P256
    w = 46404375513946096203718804850789611223135813129617114862428908103754051118038
    rep = run_walk(p, w)
    assert rep["active_steps"] == 525
    assert 499 <= rep["active_steps"] <= 533
    state = (0, p, w, 0, 1)
    for _ in range(422):
        z, f, g, fv, gv = state
        assert g != 0
        state = step(state)[0]
    z, f, g, fv, gv = state
    assert all(v != 0 for v in (f, g, fv, gv))  # all live, none constant
    assert _tc_width(g, g) + _tc_width(fv, fv) == 267
    assert _tc_width(f, f) + _tc_width(gv, gv) == 267
    assert _ub(f) + _ub(gv) == 265
    assert _ub(g) + _ub(fv) == 265
