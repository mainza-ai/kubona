"""Slice 11: MBU-packed F24 inverter — 48T/29Q measured, corrections load-bearing.

Naive Fermat: 88T/28Q/TQ=2464. MBU-packed: 48T/29Q/TQ=1392 (T -45%, Q +1,
TQ -43%). Both are frontier cells (neither dominates); the paper's 5T/10Q
needs tower-field AND-reduction on top — queued, not claimed.
"""

from kubona.codesign import (
    CircuitCodesignHarness,
    KickmixEvaluator,
    build_f24_inv_spec,
    build_mbu_inverter,
    kappa_sweep,
    tq_cost,
)
from kubona.codesign.harness import artifact_from_builder


def test_mbu_inverter_48t_29q():
    b, regs = build_mbu_inverter()
    assert regs["ccx"] == 48
    spec = build_f24_inv_spec()
    res = KickmixEvaluator().evaluate(b, spec, num_tests=256, seed=7)
    assert res["valid"] is True, res["reason"]
    assert (res["toffoli"], res["peak_qubits"]) == (48, 29)
    assert tq_cost(48, 29) == 1392
    assert kappa_sweep(48, 29) == {1: 1392, 2: 40368, 3: 1170672}


def test_mbu_corrections_load_bearing():
    # Drop the first measurement-conditioned CZ: the authored correction is
    # structural, and the evaluator must catch its absence (else the
    # corrections would be untested decoration).
    b, _ = build_mbu_inverter()
    spec = build_f24_inv_spec()
    art = artifact_from_builder(b)
    # remove exactly the first conditioned correction, keep everything else
    out, dropped = [], False
    for t in art:
        if not dropped and t[0] == "cz" and tuple(t[2]):
            dropped = True
            continue
        out.append(t)
    assert dropped
    h = CircuitCodesignHarness(spec, num_tests=64, seed=7)
    assert h.run(out)["valid"] is False


def test_mbu_beats_naive_on_tq_frontier():
    # Neither dominates on (T,Q) alone; TQ decides here — recorded, not assumed.
    from kubona.codesign import MapElitesFrontier
    f = MapElitesFrontier()
    assert f.offer("naive-fermat", 88, 28, {}) is True
    assert f.offer("mbu-packed", 48, 29, {}) is True
    assert f.best().tq == 1392
