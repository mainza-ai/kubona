"""Slice 46: factored divstep decoders (depths 6-8, multi-level CSE).

Production bar: the factored builders consume the identical minimized
cover as slice 42 (one home: `_minimized_decoder_cover` — the T-counts
compare emission disciplines, never different logic), and every factored
circuit is proven exhaustive against the same decoder_circuit_spec.
Measured: 234T -> 136T (depth 6), 960T -> 424T (depth 7), 4568T ->
1606T (depth 8); structural cost 2T per live node in all three. The
factored temps exceed the SOP-sized spec interface; every temp's
free_marker is dirty-checked in simulation, so nothing escapes
verification. Depth 9+ still refuses (beyond exhaustive proof).
"""

import random

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_decoder_circuit_factored,
    build_decoder_circuit_minimized,
    decoder_circuit_spec,
    eval_decoder,
    factor_and_network,
)


def test_factored_decoders_prove_spec():
    expect = {6: (46, 136, 68), 7: (139, 424, 212), 8: (505, 1606, 803)}
    sop = {6: 234, 7: 960, 8: 4568}
    ev = KickmixEvaluator()
    for depth, (terms, ccx, nnodes) in expect.items():
        (b, regs) = build_decoder_circuit_factored(depth)
        n_in = 2 * (depth - 1)
        assert regs["kind"] == "factored-anf"
        assert regs["terms"] == terms  # identical cover as minimized
        assert regs["nodes"] == nnodes and regs["moves"] == nnodes
        assert regs["temps"] == list(range(n_in + 1, n_in + 1 + nnodes))
        assert regs["ccx"] == ccx == 2 * regs["nodes"]
        assert regs["ccx"] <= sop[depth]  # factoring cannot lose: same cover
        (b0, regs0) = build_decoder_circuit_minimized(depth)
        assert regs0["ccx"] == sop[depth] and regs0["terms"] == terms
        spec = decoder_circuit_spec(depth)
        for mask in range(1 << n_in):
            init = {q: (mask >> q) & 1 for q in range(n_in)}
            res = ev.check_one(b, spec, init, random.Random(1))
            assert res.valid, (depth, mask, res.reason)
    with pytest.raises(ValueError):
        build_decoder_circuit_factored(9)
    with pytest.raises(ValueError):
        build_decoder_circuit_factored(1)


def test_factored_network_matches_rule():
    # emitter-independent: the factored network decides the decoder rule
    # on every full domain (depth 8 is 16384 masks of pure-Python eval).
    from kubona.codesign.divsteps import _minimized_decoder_cover
    from kubona.codesign.minimize import eval_network
    for depth in (6, 7, 8):
        names, _b, cubes, const = _minimized_decoder_cover(depth)
        n_in = len(names)
        cover = [cubes + ([(0, 0)] if const % 2 else [])]
        net = factor_and_network(cover, n_in)
        assert factor_and_network(cover, n_in) == net  # deterministic
        for mask in range(1 << n_in):
            want = eval_decoder(depth, {name: (mask >> i) & 1
                                        for i, name in enumerate(names)})
            assert eval_network(net, mask) == [want], (depth, mask)
