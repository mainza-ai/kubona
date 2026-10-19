"""Slice 30: naive history-stack divstep step (forward + round-trip).

Production bar: all 88 harvested p=13 states prove forward (next state +
history bits vs the step model) and round-trip (all zero but S); branch
coverage and width-fit are asserted, not assumed; history cost (2/step)
and Toffoli count are measured. Signed arithmetic throughout is exact by
sign-extended operands plus the half-modulus single-CX correction (each
earlier mistarget — reversal-as-uncompute, shared-wrap parity, wrong
correction bit, zero-filled loads — stays pinned by these proofs).
"""

from collections import Counter

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    N_QUBITS,
    build_dstep,
    dstep_spec_fwd,
    dstep_spec_rt,
    fit_divstep_state,
    from_bits,
    harvest_states,
    to_bits,
)
from kubona.codesign.dstep import _pack_state


def test_swept_domain_covers_all_branches_and_fits():
    states = harvest_states()
    assert len(states) == 88
    branches = Counter(fit_divstep_state(s)["branch"] for s in states)
    assert branches == {"swap": 44, "even": 30, "odd": 14}
    assert to_bits(-3, 6) == [1, 0, 1, 1, 1, 1]
    assert from_bits([1, 0, 1, 1, 1, 1]) == -3
    assert from_bits(to_bits(17, 8), signed=False) == 17
    with pytest.raises(ValueError):
        fit_divstep_state((0, 1, 2, 3))


def test_forward_proves_all_swept_states():
    import random
    states = harvest_states()
    (b, regs) = build_dstep()
    assert regs["ccx"] == 792 and regs["history_qubits"] == 2
    spec = dstep_spec_fwd()
    ev = KickmixEvaluator()
    for s in states:
        init = {q: 0 for q in range(N_QUBITS)}
        init.update(_pack_state(s))
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (s, res.reason)


def test_roundtrip_proves_reversibility():
    import random
    states = harvest_states()
    (b, regs) = build_dstep(roundtrip=True)
    assert regs["ccx"] == 1584 and regs["history_qubits"] == 0
    spec = dstep_spec_rt()
    ev = KickmixEvaluator()
    for s in states:
        init = {q: 0 for q in range(N_QUBITS)}
        init.update(_pack_state(s))
        res = ev.check_one(b, spec, init, random.Random(0))
        assert res.valid, (s, res.reason)
