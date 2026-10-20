"""Slice 62: literature factory-cost model (U2 — unblocks A5, no paper data).

The SI only motivates the T-proxy via factory throughput (main.tex:1018)
and gives no numbers; this slice wires in PUBLISHED factory costs instead,
every figure traced to its citation, labeled literature-derived (never
paper-measured). GF2019 CCZ factory: 12d x 6d footprint, 5.5d cycles/CCZ,
~1e10 states @ 1e-3 (Quantum 3:135, arXiv:1812.01238). Cost unit d^3 and
the not-dominant verdict per Litinski 2019 (Quantum 3:205); the n_Q x n_Tof
baseline diagnostic per Litinski 2023 (arXiv:2306.08585); 2026 spatial
numbers (-15%) kept as a docstring sensitivity bound, not a selectable
factory (one vintage would pretend false precision).

Ranking stays TQ (tq.py POLICY): the factory report is a diagnostic served
alongside depth/toffoli_depth/active_qubits — never a ranking input.
code_distance and cycle_us are REQUIRED (no silent physics); cycle ledgers
are exact-int half-cycles, wall seconds labeled estimates.
"""

import pytest

from kubona.codesign import KickmixEvaluator, build_f24_inv_spec, build_kara_inverter
from kubona.codesign.factories import FACTORIES, factory_report


def test_factory_table_single_source():
    assert sorted(FACTORIES) == ["gf2019_ccz"]
    spec = FACTORIES["gf2019_ccz"]
    assert spec["space_patches_per_d2"] == 72  # 12d x 6d, GF2019
    assert spec["half_cycles_per_state_per_d"] == 11  # 5.5d, exact halves
    assert spec["states_before_failure"] == 1e10
    assert "arXiv:1812.01238" in spec["source"]


def test_factory_report_hand_computed():
    # toffoli=100, qubits=50, d=2, 1us cycle — every field hand-derived:
    # patches 72*4=288; half/state 11*2=22; serial 100*22=2200 halves;
    # seconds 1100 cycles * 1us = 1.1ms; spacetime (50+288)*2200=743600.
    rep = factory_report(toffoli=100, qubits=50, toffoli_depth=7,
                         factory="gf2019_ccz", code_distance=2, cycle_us=1.0)
    assert rep["factory_patches"] == 288
    assert rep["half_cycles_per_state"] == 22
    assert rep["serial_half_cycles"] == 2200
    assert rep["serial_seconds_est"] == pytest.approx(1.1e-3)
    assert rep["spacetime_qubit_half_cycles"] == 743600
    assert rep["active_volume_baseline"] == 5000  # n_Q x n_T, LIT2023
    assert rep["states_needed"] == 100
    assert rep["within_failure_budget"] is True
    assert rep["toffoli_depth"] == 7


def test_factory_report_decoder_scale_end_to_end():
    # The verified RS decoder triple (slice 60: 3,928,980T, 110,132Q)
    # through the literature factory at d=27, 1us:
    # serial 3,928,980 * 5.5 * 27 = 583,453,530 cycles ~ 583s (~10 min);
    # accuracy budget 3.9M << 1e10 (single factory suffices on accuracy).
    rep = factory_report(toffoli=3928980, qubits=110132, toffoli_depth=0,
                         factory="gf2019_ccz", code_distance=27, cycle_us=1.0)
    assert rep["serial_half_cycles"] == 3928980 * 11 * 27
    assert rep["serial_seconds_est"] == pytest.approx(583.45353, rel=1e-6)
    assert rep["factory_patches"] == 72 * 27 * 27
    assert rep["within_failure_budget"] is True
    assert rep["spacetime_qubit_half_cycles"] == (
        (110132 + 72 * 27 * 27) * (3928980 * 11 * 27))


def test_factory_report_refusals():
    good = dict(toffoli=10, qubits=10, toffoli_depth=1,
                factory="gf2019_ccz", code_distance=3, cycle_us=1.0)
    with pytest.raises(ValueError):
        factory_report(**{**good, "factory": "nvidia_magic"})
    with pytest.raises(ValueError):
        factory_report(**{**good, "toffoli": -1})
    with pytest.raises(ValueError):
        factory_report(**{**good, "qubits": True})
    with pytest.raises(ValueError):
        factory_report(**{**good, "code_distance": 0})
    with pytest.raises(ValueError):
        factory_report(**{**good, "cycle_us": 0})
    with pytest.raises(ValueError):
        factory_report(**{**good, "cycle_us": float("nan")})
    with pytest.raises(TypeError):
        factory_report(toffoli=10, qubits=10, toffoli_depth=1)  # no silent physics


def test_evaluate_factory_wiring():
    b, _regs = build_kara_inverter()
    spec = build_f24_inv_spec()
    ev = KickmixEvaluator()
    plain = ev.evaluate(b, spec, num_tests=8, seed=7)
    assert plain["valid"] is True and plain["factory"] is None
    wired = ev.evaluate(b, spec, num_tests=8, seed=7,
                        factory="gf2019_ccz", code_distance=3, cycle_us=1.0)
    assert wired["valid"] is True
    assert wired["factory"]["factory_patches"] == 72 * 9
    assert wired["factory"]["serial_half_cycles"] == wired["toffoli"] * 11 * 3
    # ranking untouched: same T/Q feed the same TQ
    assert (wired["toffoli"], wired["peak_qubits"]) == (
        plain["toffoli"], plain["peak_qubits"])
    with pytest.raises(ValueError):
        ev.evaluate(b, spec, num_tests=8, seed=7,
                    factory="gf2019_ccz", code_distance=3)  # partial refuses
    with pytest.raises(ValueError):
        ev.evaluate(b, spec, num_tests=8, seed=7,
                    code_distance=3, cycle_us=1.0)  # d without factory refuses
    with pytest.raises(ValueError):
        ev.evaluate(b, spec, num_tests=8, seed=7,
                    factory="nope", code_distance=3, cycle_us=1.0)
    # determinism: pure functions, double-run equality
    again = ev.evaluate(b, spec, num_tests=8, seed=7,
                        factory="gf2019_ccz", code_distance=3, cycle_us=1.0)
    assert again["factory"] == wired["factory"]
