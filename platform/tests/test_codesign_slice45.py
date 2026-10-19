"""Slice 45: multi-level factorization of the trace readout.

Production bar: the greedy CSE is a stated heuristic (exact multi-level
minimization is NP-hard), but every factored network is verified twice —
eval_network decides every table on all 512 masks, and the emitted
circuit is proven exhaustive against the same spec as the SOP builders.
Measured: normal 1434T -> 438T (219 nodes, 3.3x), specialized 1508T ->
370T (185 nodes, 4.1x). The specialized basis factors better than SOP
suggested (904 vs 922 terms looked like a wash) — the paper's coordinate
choice pays off under multi-level sharing, not two-level minimization.
The gap to the paper's hand-optimized 20T stands (its hand structure is
not recoverable from the SI text: the SI gives the truth table, Fig
plaquette-layer(c), never the factored circuit) — stated, never closed
by invention.
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_spec_readout_factored,
    build_spec_readout_minimized,
    build_trace_readout_factored,
    build_trace_readout_minimized,
    emit_factored,
    eval_network,
    factor_and_network,
    spec_flag_table,
    spec_readout_spec,
    to_internal,
    trace_class,
    trace_readout_spec,
)
from kubona.codesign.group72 import _flag_tables, _spec_flag_tables
from kubona.codesign.kickmix import Builder, TaskSpec


def _bruteforce_cover(covers, nvars):
    """Independent check: evaluate cube XOR-sums directly on all masks."""
    from kubona.codesign.minimize import _covers
    res = []
    for mask in range(1 << nvars):
        row = []
        for cov in covers:
            bit = 0
            for (value, care) in cov:
                if _covers((value, care), mask):
                    bit ^= 1
            row.append(bit)
        res.append(row)
    return res


def test_factor_shared_pair_known_answer():
    # out0 = a&b&c, out1 = a&b&d: the pair (a,b) occurs twice -> one shared
    # node, then each remainder binarizes T-neutrally. Three nodes total:
    # one shared leaf-pair node plus one top node per output.
    a = ("lit", 0, 1)
    b = ("lit", 1, 1)
    c = ("lit", 2, 1)
    d = ("lit", 3, 1)
    covers = [[(0b0111, 0b0111)], [(0b1011, 0b1011)]]
    net = factor_and_network(covers, 4)
    assert len(net["nodes"]) == 3 and net["moves"] == 3
    leaf_nodes = [nid for nid, (x, y) in net["nodes"].items()
                  if isinstance(x, tuple) and isinstance(y, tuple)]
    assert len(leaf_nodes) == 1  # exactly one shared leaf-pair node
    shared = net["nodes"][leaf_nodes[0]]
    assert {shared[0], shared[1]} == {a, b}
    # each output is a single top node joining the shared node to its rest
    # (binarization sorts leaf-tuples before node-ids by repr, pinned here)
    assert len(net["outputs"]) == 2
    for js, rest in zip(net["outputs"], (c, d)):
        assert len(js) == 1 and len(js[0]) == 1
        (top,) = tuple(js[0])
        assert isinstance(top, int) and top != leaf_nodes[0]
        assert set(net["nodes"][top]) == {rest, leaf_nodes[0]}
    # exhaustive agreement with the cube semantics
    want = _bruteforce_cover(covers, 4)
    for m in range(16):
        assert eval_network(net, m) == want[m]
    # determinism: set iteration must never leak into the network
    assert factor_and_network(covers, 4) == net


def test_factor_const_output_end_to_end():
    # const-1 output (empty cube) + var0: the ONE ref parses, evaluates,
    # and emits (X gate, 0T). Zero nodes -> zero temps needed.
    covers = [[(0, 0)], [(1, 1)]]
    net = factor_and_network(covers, 2)
    assert len(net["nodes"]) == 0
    for m in range(4):
        assert eval_network(net, m) == [1, (m >> 0) & 1]
    b = Builder(4)
    n_ccx = [0]
    emit_factored(b, n_ccx, net, [], [2, 3])
    assert n_ccx[0] == 0
    ev = KickmixEvaluator()

    def _perm(bits):
        out = dict(bits)
        out[2] = 1
        out[3] = bits.get(0, 0) & 1
        return out

    spec = TaskSpec(num_qubits=4, inputs=(0, 1), outputs=(2, 3),
                    ancillas=(), target_perm=_perm)
    for m in range(4):
        init = {q: (m >> q) & 1 for q in (0, 1)}
        res = ev.check_one(b, spec, init, random.Random(2))
        assert res.valid, (m, res.reason)


def test_factor_refusals():
    with pytest.raises(ValueError):
        factor_and_network([], 4)
    with pytest.raises(ValueError):
        factor_and_network([[(1, 1)]], 0)
    with pytest.raises(ValueError):
        factor_and_network([[(1, 1)]], True)
    # structural: no 10-var cap (depth-8 decoders need 14), but cube bits
    # outside the variable space refuse instead of silently dropping
    assert factor_and_network([[(1, 1)]], 11)["nodes"] == {}
    with pytest.raises(ValueError):
        factor_and_network([[(4, 4)]], 2)
    with pytest.raises(ValueError):
        factor_and_network([[(4, 3)]], 2)
    with pytest.raises(ValueError):
        factor_and_network([[((1, 1, 1))]], 4)
    with pytest.raises(ValueError):
        factor_and_network("nope", 4)
    net = factor_and_network([[(0b11, 0b11)]], 2)
    assert len(net["nodes"]) == 1  # single cube binarizes to one node
    with pytest.raises(ValueError):
        emit_factored(Builder(3), [0], net, [], [0])  # temps short
    with pytest.raises(ValueError):
        emit_factored(Builder(4), [0], net, [2], [])  # outs short


def test_spec_tables_match_normal_values():
    from kubona.codesign import all_elements
    # reverse map once (216 solves, not 216^2): values must still agree
    # element-by-element with the normal-coords table underneath.
    by_spec = {to_internal(g): g for g in all_elements()}
    assert len(by_spec) == 216  # to_internal injective (slice 35 pin)
    tab = spec_flag_table()
    assert len(tab) == 216
    for t, (v, fl) in tab.items():
        assert v == trace_class(by_spec[t])
    # one-hot iff non-identity (identity reads all-zero, matching normal)
    n_onehot = sum(1 for (_v, fl) in tab.values() if sum(fl) == 1)
    n_zero = sum(1 for (_v, fl) in tab.values() if sum(fl) == 0)
    assert n_onehot == 215 and n_zero == 1


def test_spec_minimized_readout_proves_table():
    (b, regs) = build_spec_readout_minimized()
    assert regs["kind"] == "spec minimized QM+greedy"
    assert regs["ccx"] == 1508 and regs["temps"] == 8 and regs["terms"] == 105
    spec = spec_readout_spec()  # specialized spec, zero at invalid pairs
    ev = KickmixEvaluator()
    for mask in range(512):
        init = {q: (mask >> q) & 1 for q in range(9)}
        res = ev.check_one(b, spec, init, random.Random(3))
        assert res.valid, (mask, res.reason)


def test_factored_normal_proves_same_spec():
    (b, regs) = build_trace_readout_factored()
    assert regs["kind"] == "factored CSE"
    assert regs["terms"] == 100  # same minimized cover as the 1434T SOP
    assert regs["nodes"] == 219 and regs["moves"] == 219
    assert regs["temps"] == 219
    assert regs["ccx"] == 438 == 2 * regs["nodes"]  # structural: compute+mirror
    assert regs["ccx"] <= 1434  # factoring the minimized cover cannot lose
    (b0, regs0) = build_trace_readout_minimized()
    assert regs0["ccx"] == 1434  # the beaten pin still holds, unchanged
    spec = trace_readout_spec()  # same spec as naive + minimized
    ev = KickmixEvaluator()
    for mask in range(512):
        init = {q: (mask >> q) & 1 for q in range(9)}
        res = ev.check_one(b, spec, init, random.Random(4))
        assert res.valid, (mask, res.reason)
    # network-level agreement (independent of the emitter)
    from kubona.codesign.minimize import greedy_multioutput_cover
    net = factor_and_network(greedy_multioutput_cover(_flag_tables(), 9), 9)
    tables = _flag_tables()
    for mask in range(512):
        assert eval_network(net, mask) == [t.get(mask, 0) for t in tables]


def test_factored_spec_proves_same_spec():
    (b, regs) = build_spec_readout_factored()
    assert regs["kind"] == "spec factored CSE"
    assert regs["terms"] == 105  # same minimized cover as the 1508T SOP
    assert regs["nodes"] == 185 and regs["moves"] == 185
    assert regs["temps"] == 185
    assert regs["ccx"] == 370 == 2 * regs["nodes"]
    assert regs["ccx"] <= 1508
    (b0, regs0) = build_spec_readout_minimized()
    assert regs0["ccx"] == 1508
    spec = spec_readout_spec()
    ev = KickmixEvaluator()
    for mask in range(512):
        init = {q: (mask >> q) & 1 for q in range(9)}
        res = ev.check_one(b, spec, init, random.Random(5))
        assert res.valid, (mask, res.reason)
