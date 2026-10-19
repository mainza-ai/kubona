"""Slice 38: divstep decoder circuits at depths 6-8 (exhaustive).

Production bar: the depth-2..5 circuit scope is lifted to 2..8 — the
exhaustive domains (1024/4096/16384 inputs) are feasible, so refusing
them was scope-dodging, not prudence. Every rule proves bit-exact on its
full domain (88/88-style sweeps at 1k/4k/16k); costs pinned (344/1552/
8448T); depth 9+ still refuses (65536 inputs exceed the exhaustive
contract).
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_decoder_circuit,
    decoder_circuit_spec,
)


def _prove(depth, n_inputs, ccx, n_temps):
    (b, regs) = build_decoder_circuit(depth)
    assert regs["ccx"] == ccx and len(regs["temps"]) == n_temps
    assert regs["terms"] == {6: 79, 7: 268, 8: 1092}[depth]
    spec = decoder_circuit_spec(depth)
    ev = KickmixEvaluator()
    n_in = 2 * (depth - 1)
    for mask in range(n_inputs):
        init = {q: (mask >> q) & 1 for q in range(n_in)}
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (depth, mask, res.reason)


def test_decoder_depth6_exhaustive():
    _prove(6, 1024, 344, 3)


def test_decoder_depth7_exhaustive():
    _prove(7, 4096, 1552, 4)


def test_decoder_depth8_exhaustive():
    _prove(8, 16384, 8448, 6)


def test_decoder_scope_refuses():
    for bad in (9, 16, 0, -1, True, "6"):
        with pytest.raises(ValueError):
            build_decoder_circuit(bad)
        with pytest.raises(ValueError):
            decoder_circuit_spec(bad)
