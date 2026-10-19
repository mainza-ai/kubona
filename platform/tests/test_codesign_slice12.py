"""Slice 12: Karatsuba AND-reduction — 27T/35Q measured, map machine-proved.

Naive Fermat 88T/28Q/TQ=2464 -> MBU 48T/29Q/TQ=1392 -> Karatsuba+MBU
27T/35Q/TQ=945. Each step measured on the (T,Q) frontier; paper's 5T/10Q
still needs tower-field restructuring — queued, not claimed.
"""

from kubona.codesign import (
    CircuitCodesignHarness,
    KickmixEvaluator,
    MapElitesFrontier,
    build_f24_inv_spec,
    build_kara_inverter,
    kappa_sweep,
    tq_cost,
)
from kubona.codesign.f24 import kara_product
from kubona.codesign.gf import F24
from kubona.codesign.harness import artifact_from_builder


def test_kara_map_classical_exhaustive():
    # The AND->out map is MACHINE-DERIVED (elimination); this 256-pair
    # check against F24.mul is its proof. A hand transcription stood here
    # and was refuted — the solver output replaced it (see KARA_OUT note).
    for a in range(16):
        for b in range(16):
            assert kara_product(a, b) == F24.mul(a, b), (a, b)


def test_kara_inverter_27t_35q():
    b, regs = build_kara_inverter()
    assert regs["ccx"] == 27, regs
    spec = build_f24_inv_spec()
    res = KickmixEvaluator().evaluate(b, spec, num_tests=256, seed=7)
    assert res["valid"] is True, res["reason"]
    assert (res["toffoli"], res["peak_qubits"]) == (27, 35)
    assert tq_cost(27, 35) == 945
    assert kappa_sweep(27, 35) == {1: 945, 2: 33075, 3: 1157625}


def test_kara_corrections_load_bearing():
    b, _ = build_kara_inverter()
    spec = build_f24_inv_spec()
    art = artifact_from_builder(b)
    out, dropped = [], False
    for t in art:
        if not dropped and t[0] == "cz" and tuple(t[2]):
            dropped = True
            continue
        out.append(t)
    assert dropped
    h = CircuitCodesignHarness(spec, num_tests=64, seed=7)
    assert h.run(out)["valid"] is False


def test_three_point_frontier_ordered():
    f = MapElitesFrontier()
    assert f.offer("naive-fermat", 88, 28, {}) is True
    assert f.offer("mbu-packed", 48, 29, {}) is True
    assert f.offer("kara-mbu", 27, 35, {}) is True
    best = f.best()
    assert (best.toffoli, best.qubits, best.tq) == (27, 35, 945)
