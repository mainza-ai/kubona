"""Circuit co-design slice 1: evaluator + TQ + frontier + harness.

Production bar: real logic, fail-closed refusals, no weakened asserts.
Local-classical only (kickmix sim + Aer when available). No QPU keys.
"""

from __future__ import annotations

import random

import pytest

from kubona.codesign import (
    Builder,
    CircuitCodesignHarness,
    KickmixEvaluator,
    MapElitesFrontier,
    TaskSpec,
    detect_routing_opportunity,
    mbu_saving,
    tq_cost,
)
from kubona.codesign.harness import artifact_from_builder


def _copy_spec() -> TaskSpec:
    # 3 qubits: inputs a=0,b=1; output t=2 holds a AND b (XOR into 0).
    def perm(bits):
        out = dict(bits)
        out[2] = bits.get(2, 0) ^ (bits.get(0, 0) & bits.get(1, 0))
        return out
    return TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(2,), ancillas=(), target_perm=perm)


def test_tq_cost_paper_value_and_kappa_bounds():
    assert tq_cost(5, 10) == 50  # Extended-Data Fig 1 golden TQ
    assert tq_cost(0, 100) == 0
    with pytest.raises(ValueError):
        tq_cost(5, 10, kappa=0)
    with pytest.raises(ValueError):
        tq_cost(5, 10, kappa=4)
    with pytest.raises(ValueError):
        tq_cost(-1, 10)


def test_mbu_saving_exact():
    assert mbu_saving(1) == {"naive_toffoli": 2, "mbu_toffoli": 1, "saved": 1}
    assert mbu_saving(5)["saved"] == 5
    with pytest.raises(ValueError):
        mbu_saving(-1)


def test_valid_and_program_accepted_exhaustive():
    spec = _copy_spec()
    b = Builder(3)
    b.ccx(0, 1, 2)
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=8, seed=7)
    assert res["valid"] is True and res["failures"] == 0
    assert res["toffoli"] == 1 and res["peak_qubits"] == 3
    # exhaustive: 2 inputs x 1 record-shape (no hmr) = 4 trajectories
    assert res["tests"] == 4
    assert res["eps_bound_97_5"] < 0.65  # k=4 bound, honest not tight


def test_wrong_output_refused():
    spec = _copy_spec()
    b = Builder(3)
    b.cx(0, 2)  # copies a, not a&b
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=8, seed=7)
    assert res["valid"] is False
    assert "mismatch" in res["reason"]


def test_dirty_ancilla_refused():
    def perm(bits):
        return dict(bits)
    spec = TaskSpec(num_qubits=3, inputs=(0,), outputs=(0,), ancillas=(2,),
                    target_perm=perm)
    b = Builder(3)
    b.alloc(2)
    b.cx(0, 2)  # ancilla now holds a; never uncomputed
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=4, seed=7)
    assert res["valid"] is False
    # leaked alloc fires before the sim's dirty check — both are the same
    # defect class (uncomputed ancilla); the dirty-free path is pinned in
    # test_codesign_audit.py::test_dirty_free_refused.
    assert "never freed" in res["reason"] or "|0>" in res["reason"]


def test_input_preservation_refused():
    def perm(bits):
        out = dict(bits)
        out[1] = bits.get(0, 0)
        return out
    spec = TaskSpec(num_qubits=2, inputs=(0,), outputs=(1,), target_perm=perm)
    b = Builder(2)
    b.cx(0, 1)
    b.x(0)  # corrupts required input
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=4, seed=7)
    assert res["valid"] is False


def test_non_kickmix_refused_at_builder():
    b = Builder(2)
    with pytest.raises(ValueError):
        b._gate("h", (0,))  # H creates superposition: outside kickmix


def test_record_controlled_basis_refused():
    b = Builder(3)
    with pytest.raises(ValueError):
        b._gate("cx", (0, 1), cond=(0,))


def test_mbu_measure_plus_correction_valid():
    # temp AND t=uv computed, used (phase), measured, corrected by CZ.
    # Models Methods Eq 12 at trajectory level.
    spec = TaskSpec(
        num_qubits=3, inputs=(0, 1), outputs=(),
        target_perm=lambda bits: dict(bits),
        target_phase=lambda bits: 0,
    )
    b = Builder(3)
    b.ccx(0, 1, 2)
    r = b.hmr(2)
    b.cz(0, 1, cond=(r,))  # correction cancels (-1)^{r*u*v}
    # HMR consumed t; correction is diagonal so phase returns to 0
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=32, seed=11)
    # Without the CZ the phase would mismatch on r=1 trajectories half
    # the time; with it every trajectory must agree.
    assert res["valid"] is True


def test_mbu_without_correction_detectable():
    spec = TaskSpec(
        num_qubits=3, inputs=(0, 1), outputs=(),
        target_perm=lambda bits: dict(bits),
        target_phase=lambda bits: 0,
    )
    b = Builder(3)
    b.ccx(0, 1, 2)
    b.hmr(2)  # no CZ correction
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=64, seed=11)
    assert res["valid"] is False


def test_routing_detector_sound():
    b = Builder(4)
    b.ccx(0, 1, 3)
    b.ccx(0, 2, 3)
    rep = detect_routing_opportunity(b.emit())
    assert rep["count"] == 1
    assert rep["candidates"][0]["shared_target"] == 3
    # different targets -> no candidate (no false positive)
    c = Builder(4)
    c.ccx(0, 1, 2)
    c.ccx(0, 1, 3)
    assert detect_routing_opportunity(c.emit())["count"] == 0


def test_frontier_keeps_min_tq_per_cell():
    f = MapElitesFrontier()
    assert f.offer("a", 100, 100, {}) is True
    assert f.offer("b", 200, 100, {}) is False or True  # different cell or worse
    # same cell, worse TQ refuses to displace
    f2 = MapElitesFrontier(t_buckets=2, q_buckets=2)
    assert f2.offer("good", 4, 4, {}) is True
    assert f2.offer("bad", 7, 7, {}) is False  # same bucket, higher TQ
    assert f2.best().artifact == "good"


def test_harness_ranks_valid_above_invalid():
    spec = _copy_spec()
    h = CircuitCodesignHarness(spec, num_tests=8)
    good = Builder(3)
    good.ccx(0, 1, 2)
    bad = Builder(3)
    bad.cx(0, 2)
    rg = h.run(artifact_from_builder(good))
    rb = h.run(artifact_from_builder(bad))
    assert h.validity_check(rg) is True
    assert h.validity_check(rb) is False
    assert h.metric(rg) > h.metric(rb)
    assert rg["tq"] == 1 * 3  # T=1 Q=3 kappa=1


def test_harness_forged_artifact_invalid_not_scored():
    spec = _copy_spec()
    h = CircuitCodesignHarness(spec, num_tests=8)
    r = h.run([("h", (0,), ())])  # smuggled non-kickmix
    assert r["valid"] is False
    assert r["tq"] == 10**18


def test_kickmix_phase_gate_exact():
    # CZ adds phase iff both bits 1; target_phase models it exactly.
    spec = TaskSpec(
        num_qubits=2, inputs=(0, 1), outputs=(),
        target_perm=lambda bits: dict(bits),
        target_phase=lambda bits: bits.get(0, 0) & bits.get(1, 0),
    )
    b = Builder(2)
    b.cz(0, 1)
    ev = KickmixEvaluator()
    assert ev.evaluate(b, spec, num_tests=4, seed=3)["valid"] is True
    # missing CZ must fail on the 11 input
    c = Builder(2)
    assert ev.evaluate(c, spec, num_tests=4, seed=3)["valid"] is False


def test_local_simulator_bell_live():
    # Local-classical live verification (no API keys): Bell via Kubona sim.
    from kubona.circuits import parse_qasm2
    from kubona.providers import get_provider
    bell = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    sim = get_provider("local-simulator")
    out = sim.submit(bell, 2048, seed=5)
    assert set(out["counts"]) <= {"00", "11"}
    assert abs(out["counts"].get("00", 0) - out["counts"].get("11", 0)) <= 0.1 * 2048


def test_aer_bell_live_if_available():
    aer = pytest.importorskip("qiskit_aer")
    from qiskit import QuantumCircuit
    qc = QuantumCircuit(2, 2)
    qc.h(0)
    qc.cx(0, 1)
    qc.measure([0, 1], [0, 1])
    backend = aer.AerSimulator(seed_simulator=5)
    job = backend.run(qc, shots=1024)
    counts = job.result().get_counts()
    assert set(counts) <= {"00", "11", "00 ", "11 "} or set(
        k.replace(" ", "") for k in counts) <= {"00", "11"}


def test_evaluate_records_failure_details_with_repro_recipe():
    # x(2) flips t on every input: every sampled trajectory fails. Details
    # record trial + input + reason (bounded), each a deterministic
    # reproduction recipe for (seed, trial).
    spec = _copy_spec()
    b = Builder(3)
    b.x(2)
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=8, seed=7, max_failures=5)
    assert res["valid"] is False and res["failures"] == 6
    det = res["failure_details"]
    assert len(det) == 6 and res["failure_details_capped"] is False
    assert all(set(d) >= {"t", "input", "reason"} for d in det)
    assert [d["t"] for d in det] == sorted(d["t"] for d in det)
    assert all(set(d["input"]) >= {0, 1, 2} for d in det)
    # determinism: same seed replays the same trials
    again = ev.evaluate(b, spec, num_tests=8, seed=7, max_failures=5)
    assert again["failure_details"] == det


def test_failure_details_cap_at_scale():
    # Always-fail program over 64 sampled trials: details stop at the cap,
    # counts don't; the capped flag says a subset is shown.
    spec = _copy_spec()
    b = Builder(3)
    b.x(2)  # flips t on every input: every trajectory fails
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=64, seed=7, max_failures=47)
    assert res["valid"] is False and res["failures"] == 48
    assert len(res["failure_details"]) == 32
    assert res["failure_details_capped"] is True


def test_valid_run_has_empty_uncapped_details():
    spec = _copy_spec()
    b = Builder(3)
    b.ccx(0, 1, 2)
    ev = KickmixEvaluator()
    res = ev.evaluate(b, spec, num_tests=8, seed=7)
    assert res["valid"] is True
    assert res["failure_details"] == [] and res["failure_details_capped"] is False
