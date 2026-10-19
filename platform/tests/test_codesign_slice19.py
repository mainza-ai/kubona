"""Slice 19: pruned-QROM emitter, frame-Select emitters, trace-dual agreement.

Production bar: emitters proven over full domains (system data live, not
just |0>); the bit-order mistarget the evaluator caught on the alternating
table stays pinned by that table; H-frames stay outside kickmix by boundary,
not by omission.
"""

import random

import pytest

from kubona.codesign import (
    F2_12,
    F24,
    KickmixEvaluator,
    build_cnot_select,
    build_pruned_qrom,
    build_swap_select,
    cnot_select_spec,
    pruned_qrom_spec,
    swap_select_spec,
    trace_dual,
)


def _full_domain(build, spec, n_addr: int, n_sys: int) -> bool:
    rng = random.Random(7)
    ev = KickmixEvaluator()
    for a in range(1 << n_addr):
        for s in range(1 << n_sys):
            init = {i: (a >> i) & 1 for i in range(n_addr)}
            init.update({n_addr + j: (s >> j) & 1 for j in range(n_sys)})
            if not ev.check_one(build, spec(), init, rng).valid:
                return False
    return True


def test_pruned_qrom_valid_and_shares():
    # uniform: nothing to select (0T both)
    for table in ([1] * 8, [0] * 8):
        (b, regs) = build_pruned_qrom(table, True)
        assert regs["ccx"] == 0
        assert KickmixEvaluator().evaluate(b, pruned_qrom_spec(table),
                                            num_tests=16, seed=7)["valid"] is True
    # paper pattern A,A,B,B,...: identical halves skip the top bit
    table = [0, 0, 1, 1, 0, 0, 1, 1]
    (b, regs) = build_pruned_qrom(table, True)
    (bn, regn) = build_pruned_qrom(table, False)
    assert regs["ccx"] == 2 and regn["ccx"] == 5  # sharing saves the top mux
    assert KickmixEvaluator().evaluate(b, pruned_qrom_spec(table),
                                        num_tests=16, seed=7)["valid"] is True
    # alternating table: maximally bit-sensitive (caught the MSB/LSB mistarget)
    alt = [0, 1] * 4
    (ba, rega) = build_pruned_qrom(alt, True)
    assert rega["ccx"] == 4
    assert KickmixEvaluator().evaluate(ba, pruned_qrom_spec(alt),
                                        num_tests=16, seed=7)["valid"] is True
    # random tables: pruned never costs more than naive, always valid
    rng = random.Random(0)
    for _ in range(6):
        t = [rng.getrandbits(1) for _ in range(8)]
        (bp, rp) = build_pruned_qrom(t, True)
        (bq, rq) = build_pruned_qrom(t, False)
        assert rp["ccx"] <= rq["ccx"]
        assert KickmixEvaluator().evaluate(bp, pruned_qrom_spec(t),
                                            num_tests=16, seed=7)["valid"] is True
    with pytest.raises(ValueError):
        build_pruned_qrom([1, 0, 1])  # non-2^k refuses
    with pytest.raises(ValueError):
        build_pruned_qrom([1] * 32)  # proof scope refuses (k <= 4)


def test_frame_select_full_domain_and_faults():
    assert _full_domain(build_swap_select()[0], swap_select_spec, 1, 2) is True
    assert _full_domain(build_cnot_select()[0], cnot_select_spec, 1, 2) is True
    assert build_swap_select()[1]["ccx"] == 2
    assert build_cnot_select()[1]["ccx"] == 2
    # dropped un-frame (the reversibility fraud): caught on live system data
    from kubona.codesign import Builder
    from kubona.codesign.passes import emit_cswap
    bf = Builder(3)
    emit_cswap(bf, 0, 1, 2)
    bf.x(1)  # frame applied, never un-applied
    assert _full_domain(bf, swap_select_spec, 1, 2) is False
    bg = Builder(3)
    bg.ccx(0, 1, 2)
    bg.x(1)  # conjugation half done
    assert _full_domain(bg, cnot_select_spec, 1, 2) is False


def test_trace_dual_agreement():
    for m in range(16):  # F16 exhaustive: every mask x every element
        mu = trace_dual([(m >> j) & 1 for j in range(4)], F24)
        for e in range(16):
            assert F24.trace(F24.mul(mu, e)) == (bin(m & e).count("1") & 1)
    for m in (0, 1, 0xABC, 0xFFF):  # F2_12: spot masks x full element sweep
        mu = trace_dual([(m >> j) & 1 for j in range(12)], F2_12)
        for e in range(4096):
            assert F2_12.trace(F2_12.mul(mu, e)) == (bin(m & e).count("1") & 1)
    with pytest.raises(ValueError):
        trace_dual([1, 0], F24)  # wrong mask length refuses
