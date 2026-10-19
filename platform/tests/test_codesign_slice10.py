"""Slice 10: F24 Fermat inverter — naive baseline, fully verified.

88T/28Q/TQ=2464 reproduces the paper's STARTING point (their trial Fermat
ran 66T/20Q), not its MBU-packed endpoint (5T/10Q). The gap is the
co-design story, measured here at the near end.
"""

from kubona.codesign import (
    Builder,
    KickmixEvaluator,
    TaskSpec,
    build_f24_inv_spec,
    build_inverter,
    emit_mul,
    emit_square,
    tq_cost,
)


def _word_value(bits, qs):
    return sum(bits.get(q, 0) << i for i, q in enumerate(qs))


def test_multiplier_22t_exhaustive():
    from kubona.codesign.gf import F24
    b = Builder(12)
    a, c, out = [0, 1, 2, 3], [4, 5, 6, 7], [8, 9, 10, 11]
    assert emit_mul(b, a, c, out) == 22

    def perm(bits):
        p = F24.mul(_word_value(bits, a), _word_value(bits, c))
        r = dict(bits)
        for i, q in enumerate(out):
            r[q] = (p >> i) & 1
        return r

    spec = TaskSpec(12, tuple(a + c), tuple(out), target_perm=perm)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=256, seed=7)
    assert res["valid"] is True and res["tests"] == 256
    assert res["toffoli"] == 22


def test_squarer_clifford_exhaustive():
    from kubona.codesign.gf import F24
    b = Builder(8)
    emit_square(b, [0, 1, 2, 3], [4, 5, 6, 7])

    def perm(bits):
        p = F24.sqr(_word_value(bits, [0, 1, 2, 3]))
        r = dict(bits)
        for i in range(4):
            r[4 + i] = (p >> i) & 1
        return r

    spec = TaskSpec(8, (0, 1, 2, 3), (4, 5, 6, 7), target_perm=perm)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=16, seed=7)
    assert res["valid"] is True and res["toffoli"] == 0  # CNOT-only


def test_inverter_88t_28q_exhaustive():
    b, regs = build_inverter()
    assert regs["ccx"] == 88
    assert regs["ancillas"] and len(regs["ancillas"]) == 20
    spec = build_f24_inv_spec()
    res = KickmixEvaluator().evaluate(b, spec, num_tests=64, seed=7)
    assert res["valid"] is True, res["reason"]
    assert res["tests"] == 16  # 4 inputs, no measurements: fully exhaustive
    assert (res["toffoli"], res["peak_qubits"]) == (88, 28)
    assert tq_cost(88, 28) == 2464


def test_inverter_zero_edge_and_inputs_preserved():
    # 0^-1 = 0 (h untouched) and x never modified: checked per-trajectory.
    import random
    b, regs = build_inverter()
    spec = build_f24_inv_spec()
    ev = KickmixEvaluator()
    for x in (0, 1, 15):
        init = {i: (x >> i) & 1 for i in range(4)}
        init.update({q: 0 for q in range(4, 8)})
        r = ev.check_one(b, spec, dict(init), random.Random(1))
        assert r.valid, (x, r.reason)
        assert r.final_bits[0] == (x & 1)  # input preserved bit-level
