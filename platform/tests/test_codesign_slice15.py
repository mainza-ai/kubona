"""Slice 15: depth layering, half-degree Psi, packing + freeze, frame blocks.

Production bar: exact measurements with load-bearing counterparts
(disjoint != sequential, halved != full, fit != overflow, block != forced).
"""

import pytest

from kubona.codesign import (
    Builder,
    KickmixEvaluator,
    active_qubits,
    apply_perm,
    build_zero_flag,
    circuit_depth,
    eea,
    find_blocks,
    fsqrt,
    pack_or_refuse,
    plan_packing,
    reciprocal_psi,
    toffoli_depth,
    walk_widths,
    zero_flag_spec,
)
from kubona.codesign.gf import F24


def test_depth_parallel_vs_sequential_and_feedforward():
    a = Builder(6)
    a.ccx(0, 1, 2)
    a.ccx(3, 4, 5)
    assert circuit_depth(a.emit()) == 1 and toffoli_depth(a.emit()) == 1
    s = Builder(6)
    s.ccx(0, 1, 2)
    s.ccx(2, 3, 4)  # shares qubit 2 -> serializes
    assert circuit_depth(s.emit()) == 2 and toffoli_depth(s.emit()) == 2
    # Clifford-only layers do not add Toffoli depth
    c = Builder(3)
    c.cx(0, 1)
    c.cx(0, 1)
    assert circuit_depth(c.emit()) == 2 and toffoli_depth(c.emit()) == 0
    # record feedforward orders across disjoint qubits
    from kubona.codesign import TaskSpec
    b = Builder(4)
    b.alloc(3)
    b.ccx(0, 1, 3)
    r = b.hmr(3)
    b.cz(0, 2, cond=(r,))  # disjoint from qubit 3, still after the hmr
    assert circuit_depth(b.emit()) == 3
    assert active_qubits(b.emit()) == 4
    with pytest.raises(ValueError):
        circuit_depth("not-a-list")


def test_evaluate_reports_depth_triple():
    from kubona.codesign import routed_1bit_spec, build_routed_1bit_madd
    (bld, _), spec = build_routed_1bit_madd(), routed_1bit_spec()
    res = KickmixEvaluator().evaluate(bld, spec, num_tests=16, seed=7)
    assert (res["depth"], res["toffoli_depth"], res["active_qubits"]) == (7, 3, 5)


def test_psi_halves_eea_and_verifies_at_roots():
    for a in range(16):
        assert F24.sqr(fsqrt(a, F24)) == a
    sigma = [1, 0, 7, 6]  # (1+x)(1+2x)(1+3x) over F16, 3 distinct roots
    rep = reciprocal_psi(sigma, F24)
    assert rep["deg_sigma"] == 3 and rep["deg_eea"] == 1  # halved
    assert len(rep["roots"]) == 3
    assert rep["A"] != [0] and rep["B"] != [0]  # non-degenerate half EEA
    # Bezout gate on the half pair independently rechecked here
    from kubona.codesign import padd, pmul
    lhs = padd(pmul(rep["A"], rep["E"], F24), pmul(rep["B"], rep["O"], F24), F24)
    assert lhs == [1]
    with pytest.raises(ValueError):
        # E,O share a factor here (honest miss, never a fake Psi) —
        # formerly the degree-cap probe; the cap is lifted (slice 49)
        # and this input still refuses on its mathematical merits.
        reciprocal_psi([1, 1, 1, 1, 1, 1], F24)
    # E,O sharing a factor: sigma = (1+z)^2 has no distinct-root inverse here
    with pytest.raises(ValueError):
        reciprocal_psi([1, 0, 1], F24)


def test_packing_fits_real_walk_and_refuses_overflow():
    trace = walk_widths(257, 123)
    assert len(trace) == 17
    assert trace[0]["wg"] > trace[-1]["wg"]  # g shrinks along the walk
    assert trace[0]["wfv"] < trace[-1]["wfv"]  # f_v grows into vacated bits
    rep = plan_packing(trace, 16)
    assert rep["fits"] is True and rep["peak_fill_a"] <= 16 and rep["peak_fill_b"] <= 16
    pack_or_refuse(trace, 16)
    with pytest.raises(ValueError):
        pack_or_refuse(trace, 4)  # overflow names its step, never wraps
    (bld, regs), spec = build_zero_flag(), zero_flag_spec()
    assert regs["ccx"] == 1
    res = KickmixEvaluator().evaluate(bld, spec, num_tests=16, seed=7)
    assert res["valid"] is True and res["tests"] == 4 and res["toffoli"] == 1


def test_frame_blocks_reconstruct_members():
    perms = [(1, 0, 2, 3), (0, 1, 3, 2)]
    rep = find_blocks([(5, 0), (6, 0), (9, 0), (10, 0)], perms)
    assert len(rep["blocks"]) == 1
    blk = rep["blocks"][0]
    assert blk["size"] == 4 and blk["addr_bits"] == 2  # paper's power-of-two shape
    for m in blk["members"]:
        assert apply_perm(blk["rep"], blk["witnesses"][f"{m}"]) == m
    assert rep["ungrouped"] == []
    with pytest.raises(ValueError):
        find_blocks([], perms)
    with pytest.raises(ValueError):
        apply_perm((1, 0), (0, 0))  # not a permutation: refusing
