"""Slice 5: numerics kernel + Aer adder sweep + pack promotion gate."""

import pytest

from kubona.codesign import (
    accuracy_sweep,
    default_registry,
    dyadic_leaves,
    fit_quadratic_integer,
    promote,
    shared_coordinate,
    truncated_product,
)
from kubona.provenance import Ledger


def test_dyadic_leaves_cover_and_refuse():
    assert dyadic_leaves(3, [2, 2]) == [(0, 2), (4, 2)]
    assert dyadic_leaves(3, [1, 1, 1, 1]) == [(0, 1), (2, 1), (4, 1), (6, 1)]
    with pytest.raises(ValueError):
        dyadic_leaves(3, [1, 2])  # L=2 misaligned to 2^2
    with pytest.raises(ValueError):
        dyadic_leaves(3, [2])  # gap: covers 4 of 8
    with pytest.raises(ValueError):
        dyadic_leaves(3, [3, 0])  # overrun past 8


def test_shared_coordinate_paper_example():
    # width-2^12 leaf in a 15-bit stage: d = 2^3*delta + cL
    L, b, c = 3 * (1 << 12), 12, 15
    x = L + 5
    d, delta, cL = shared_coordinate(x, L, b, c)
    assert delta == 5 and cL == 3 and d == (5 << 3) + 3
    with pytest.raises(ValueError):
        shared_coordinate(L + (1 << b), L, b, c)  # outside leaf
    with pytest.raises(ValueError):
        shared_coordinate(x, L + 1, b, c)  # misaligned L


def test_truncated_product_bound():
    kept, bound = truncated_product(13, 11, 0)
    assert (kept, bound) == (143, 0)
    kept, bound = truncated_product(13, 11, 4)
    assert kept == (143 >> 4) << 4 and bound == 15
    assert 0 <= 143 - kept <= bound
    with pytest.raises(ValueError):
        truncated_product(-1, 2, 0)


def test_fit_quadratic_integer_exact_recovery():
    # ground truth q(d) = 10 - d*(3 - 1*d) over d=0..4
    xs = list(range(5))
    ys = [10 - d * (3 - 1 * d) for d in xs]
    fit = fit_quadratic_integer(xs, ys, (4, 2, 2))
    assert fit["max_err"] == 0
    assert (fit["g0"], fit["g1"], fit["g2"]) == (10, 3, 1)
    rep = accuracy_sweep(xs, lambda d: 10 - d * (3 - 1 * d), fit)
    assert rep["max_err"] == 0
    with pytest.raises(ValueError):
        fit_quadratic_integer(xs, ys, (12, 12, 12), max_combos=10)  # space guard


def _adder_circuit(a: int, b: int):
    from qiskit import QuantumCircuit
    # 2-bit adder: s0=a0^b0, c0=a0&b0, s1=a1^b1^c0, cout=maj(a1,b1,c0).
    # cout = (a1&b1)^(a1&c0)^(b1&c0): three Toffolis, no scratch.
    qc = QuantumCircuit(8, 3)
    a0, a1, b0, b1, s0, s1, c0, cout = 0, 1, 2, 3, 4, 5, 6, 7
    if a & 1:
        qc.x(a0)
    if a & 2:
        qc.x(a1)
    if b & 1:
        qc.x(b0)
    if b & 2:
        qc.x(b1)
    qc.cx(a0, s0)
    qc.cx(b0, s0)
    qc.ccx(a0, b0, c0)
    qc.cx(a1, s1)
    qc.cx(b1, s1)
    qc.cx(c0, s1)
    qc.ccx(a1, b1, cout)
    qc.ccx(a1, c0, cout)
    qc.ccx(b1, c0, cout)
    qc.measure([s0, s1, cout], [0, 1, 2])
    return qc


def test_aer_two_bit_adder_full_sweep_live():
    aer = pytest.importorskip("qiskit_aer")
    backend = aer.AerSimulator(seed_simulator=7)
    # Exhaustive: all 16 (a,b); expected (cout,s1,s0) with s=(a+b)%4, cout=(a+b)//4.
    # Bit order c2c1c0 = cout,s1,s0.
    for a in range(4):
        for b in range(4):
            s = (a + b) % 4
            c = (a + b) // 4
            want = f"{c}{(s >> 1) & 1}{s & 1}"
            counts = backend.run(_adder_circuit(a, b), shots=256).result().get_counts()
            top = max(counts, key=counts.get).replace(" ", "")
            assert top == want, f"a={a} b={b}: got {top}, want {want}"
            assert counts[top] / 256 > 0.9, f"a={a} b={b}: weak majority {counts}"


def test_numerics_pack_promotion_needs_ledger_refs(tmp_path):
    led = Ledger(path=str(tmp_path / "num-ledger.jsonl"))
    anchor = led.append("trial", {"harness": "numerics_anchor_fixture"})
    proof = led.append("trial", {"harness": "numerics_fit_fixture"})
    reg = default_registry()
    active = promote(reg["numerics"],
                     getattr(anchor, "record_id", None) or anchor.record_hash,
                     getattr(proof, "record_id", None) or proof.record_hash)
    assert active.status == "active"
    with pytest.raises(ValueError):
        promote(reg["numerics"], "", "")
