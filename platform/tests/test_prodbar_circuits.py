"""W1.15 production-bar regression tests: circuits (model/parser/schedule/shadows/krylov/cutter).

Every changed behavior gets a refusal-path or labeling test. Strengthen, never weaken.
"""

import math

import numpy as np
import pytest

from kubona.circuits import Circuit, Gate, canonical_hash, parse_qasm2, to_qasm
from kubona.circuits import model as model_mod
from kubona.circuits.schedule import (
    FRAGILE_LATE_SUFFIX,
    commutes,
    equivalent_up_to_phase,
    fragile_metrics,
    schedule_fragile_late,
)


# ---------------------------------------------------------------------------
# model.py
# ---------------------------------------------------------------------------

def test_canonical_repr_exact_not_rounded():
    a = Circuit(name="c", num_qubits=1, gates=[Gate("rx", (0,), (0.1234567890123,))])
    b = Circuit(name="c", num_qubits=1, gates=[Gate("rx", (0,), (0.1234567890199,))])
    # Differ only at 1e-11: the old round(p, 10) aliased them to one hash.
    assert a.canonical_repr() != b.canonical_repr()
    assert canonical_hash(a) != canonical_hash(b)


def test_canonical_hash_full_hexdigest():
    c = Circuit(name="c", num_qubits=1, gates=[Gate("h", (0,))])
    h = canonical_hash(c)
    assert len(h) == 64 and all(ch in "0123456789abcdef" for ch in h)


def test_depth_is_real_layers_not_gate_count():
    # Two disjoint 1q gates share one layer: depth 1, count 2.
    parallel = Circuit(name="p", num_qubits=2,
                       gates=[Gate("h", (0,)), Gate("h", (1,))])
    assert parallel.depth() == 1
    assert parallel.gate_count() == 2
    serial = Circuit(name="s", num_qubits=1,
                     gates=[Gate("h", (0,)), Gate("x", (0,))])
    assert serial.depth() == 2
    assert Circuit(name="e", num_qubits=2, gates=[]).depth() == 0


def test_gate_rejects_negative_qubit_and_nonfinite_param():
    with pytest.raises(ValueError, match="non-negative"):
        Gate("h", (-1,))
    with pytest.raises(ValueError, match="non-finite"):
        Gate("rx", (0,), (float("inf"),))
    with pytest.raises(ValueError, match="invalid gate name"):
        Gate("", (0,))
    with pytest.raises(ValueError, match="duplicate qubit operands"):
        Gate("cx", (0, 0))


def test_parser_multi_qreg_offsets_and_bounds():
    src = "OPENQASM 2.0;\nqreg a[2];\nqreg b[2];\ncx a[1], b[0];\nh b[1];"
    c = parse_qasm2(src)
    assert c.num_qubits == 4
    assert c.gates[0].name == "cx"
    assert c.gates[0].qubits == (1, 2)
    assert c.gates[1].name == "h"
    assert c.gates[1].qubits == (3) or c.gates[1].qubits == (3,)
    with pytest.raises(ValueError, match="duplicate qubit operands"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[2];\ncx q[0], q[0];")


def test_circuit_cross_validates_width():
    with pytest.raises(ValueError, match="beyond circuit width"):
        Circuit(name="c", num_qubits=2, gates=[Gate("h", (5,))])
    with pytest.raises(ValueError, match="positive int"):
        Circuit(name="c", num_qubits=0, gates=[])
    with pytest.raises(ValueError, match="non-empty string"):
        Circuit(name="", num_qubits=1, gates=[])


def test_to_qasm_preserves_num_classical_and_round_trips():
    c = Circuit(name="q", num_qubits=2, gates=[Gate("h", (0,)), Gate("cx", (0, 1))],
                num_classical=2)
    qasm = to_qasm(c)
    assert "creg c[2];" in qasm
    back = parse_qasm2(qasm, name="q")
    assert back.num_classical == 2
    assert [g.name for g in back.gates] == ["h", "cx"]


def test_to_qasm_refuses_invalid_gate_name():
    g = object.__new__(Gate)
    object.__setattr__(g, "name", "not a gate!")
    object.__setattr__(g, "qubits", (0,))
    object.__setattr__(g, "params", ())
    with pytest.raises(ValueError, match="invalid name"):
        to_qasm(Circuit(name="c", num_qubits=1, gates=[g]))


# ---------------------------------------------------------------------------
# parser.py
# ---------------------------------------------------------------------------

def test_parser_name_derived_never_untitled():
    a = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];")
    b = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nx q[0];")
    assert a.name == "q"  # derived from the qreg register
    assert "untitled" not in (a.name, b.name)
    explicit = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nh q[0];", name="bell")
    assert explicit.name == "bell"


def test_parser_records_skipped_directives_and_creg():
    c = parse_qasm2(
        'OPENQASM 2.0;\ninclude "qelib1.inc";\nqreg q[2];\ncreg c[2];\n'
        "barrier q[0];\nh q[0];\n")
    assert any("include" in s for s in c.skipped_directives)
    assert any(s.startswith("creg") for s in c.skipped_directives)
    assert any(s.startswith("barrier") for s in c.skipped_directives)
    assert c.num_classical == 2
    assert [g.name for g in c.gates] == ["h"]


def test_parser_measure_recorded_or_refused():
    rec = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];")
    assert any(s.startswith("measure") for s in rec.skipped_directives)
    with pytest.raises(ValueError, match="measure statement refused"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[1];\ncreg c[1];\nh q[0];\nmeasure q[0] -> c[0];",
                    refuse_measure=True)


def test_parser_qreg_widths_accumulate():
    c = parse_qasm2("OPENQASM 2.0;\nqreg a[2];\nqreg b[3];\nh a[0];")
    assert c.num_qubits == 5  # old code overwrote -> 3, shrinking the circuit
    assert c.name == "a"


def test_parser_arity_and_range_fail_closed():
    with pytest.raises(ValueError, match="needs 2 qubit"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[2];\ncx q[0];")
    with pytest.raises(ValueError, match="needs 1 param"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nrx q[0];")
    with pytest.raises(ValueError, match="unknown gate"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nfoo q[0];")
    with pytest.raises(ValueError, match="beyond circuit width"):
        parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[5];")
    with pytest.raises(ValueError, match="malformed qreg"):
        parse_qasm2("OPENQASM 2.0;\nqreg q;\nh q[0];")
    with pytest.raises(ValueError, match="empty QASM"):
        parse_qasm2("OPENQASM 2.0;")


def test_parser_opaque_block_skipped_to_endopaque():
    src = ("OPENQASM 2.0;\nqreg q[2];\n"
           "opaque mygate a;\n"
           "h a[0];\n"          # would parse as a gate if only line-skipped
           "cx a[0],a[1];\n"    # likewise
           "endopaque q[0];\n"
           "h q[0];\n")
    c = parse_qasm2(src)
    assert [g.name for g in c.gates] == ["h"]
    assert any("opaque" in s for s in c.skipped_directives)


# ---------------------------------------------------------------------------
# schedule.py
# ---------------------------------------------------------------------------

def test_equivalent_up_to_phase_tol_validated():
    c = Circuit(name="c", num_qubits=1, gates=[Gate("h", (0,))])
    assert equivalent_up_to_phase(c, c)
    with pytest.raises(ValueError, match="tol must be positive"):
        equivalent_up_to_phase(c, c, tol=0.0)
    with pytest.raises(ValueError, match="tol must be positive"):
        equivalent_up_to_phase(c, c, tol=float("inf"))


def test_supported_gate_guard_names_set():
    from kubona.circuits.schedule import _gate_unitary, circuit_unitary
    with pytest.raises(ValueError, match="no exact unitary.*supported set"):
        _gate_unitary(Gate("u1", (0,), (0.5,)), 1)
    c = Circuit(name="c", num_qubits=1, gates=[Gate("u1", (0,), (0.5,))])
    with pytest.raises(ValueError, match="supported"):
        circuit_unitary(c)
    with pytest.raises(ValueError, match="supported"):
        equivalent_up_to_phase(c, c)


def test_cx_control_target_convention():
    # |10> (control=1) flips the target -> |11>; |00> is identity.
    from kubona.circuits.schedule import circuit_unitary
    flip = Circuit(name="f", num_qubits=2, gates=[Gate("cx", (0, 1))])
    u = circuit_unitary(flip)
    assert abs(u[3, 2] - 1.0) < 1e-12  # col |10>=2 -> row |11>=3
    assert abs(u[0, 0] - 1.0) < 1e-12  # |00> fixed
    assert abs(u[2, 3] - 1.0) < 1e-12  # |11> -> |10> (control still 1)


def test_shared_gate_sets_single_source():
    import kubona.qir.portfolio as portfolio
    import kubona.circuits.schedule as schedule
    assert portfolio.NON_CLIFFORD is model_mod.NON_CLIFFORD
    assert portfolio.EXOTIC_GATES is model_mod.EXOTIC_GATES
    assert schedule.CLIFFORD_1Q is model_mod.CLIFFORD_1Q
    assert schedule.DIAGONAL is model_mod.DIAGONAL


def test_fragile_suffix_idempotent():
    c = Circuit(name="c", num_qubits=2,
                gates=[Gate("t", (0,)), Gate("h", (1,)), Gate("h", (0,)),
                       Gate("cx", (1, 0)), Gate("x", (1,))])
    once = schedule_fragile_late(c)
    twice = schedule_fragile_late(once)
    assert once.name.endswith(FRAGILE_LATE_SUFFIX)
    assert twice.name == once.name  # old code re-appended per pass
    assert equivalent_up_to_phase(once, twice)


# ---------------------------------------------------------------------------
# u3/u2 decomposition order vs the textbook unitary (qir/pipeline.py proof)
# ---------------------------------------------------------------------------

def _u3_matrix(th: float, ph: float, la: float) -> np.ndarray:
    # Textbook u3: [[c, -e^{iλ}s], [e^{iφ}s, e^{i(φ+λ)}c]], c=cos(θ/2), s=sin(θ/2).
    c, s = np.cos(th / 2), np.sin(th / 2)
    return np.array([[c, -np.exp(1j * la) * s],
                     [np.exp(1j * ph) * s, np.exp(1j * (ph + la)) * c]], complex)


def test_u3_u2_decomposition_matches_unitary():
    from kubona.circuits.schedule import circuit_unitary
    from kubona.qir.pipeline import transpile_to_native
    for (th, ph, la) in [(0.5, 0.2, 0.1), (1.2, -0.4, 2.1), (np.pi / 2, 0.0, np.pi)]:
        native = transpile_to_native(
            Circuit(name="u", num_qubits=1, gates=[Gate("u3", (0,), (th, ph, la))]))
        got = circuit_unitary(native)
        want = _u3_matrix(th, ph, la)
        dim = 2
        overlap = abs(np.vdot(got.ravel(), want.ravel())) / dim
        assert overlap > 1 - 1e-9, (th, ph, la, overlap)
        # u2(phi, lam) == u3(pi/2, phi, lam)
        native2 = transpile_to_native(
            Circuit(name="u", num_qubits=1, gates=[Gate("u2", (0,), (ph, la))]))
        got2 = circuit_unitary(native2)
        overlap2 = abs(np.vdot(got2.ravel(), _u3_matrix(np.pi / 2, ph, la).ravel())) / dim
        assert overlap2 > 1 - 1e-9, (ph, la, overlap2)


# ---------------------------------------------------------------------------
# shadows.py
# ---------------------------------------------------------------------------

def _bell_state() -> np.ndarray:
    s = np.zeros(4, dtype=complex)
    s[0] = s[3] = 1.0 / np.sqrt(2)
    return s


def test_shadows_rng_required():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    with pytest.raises(ValueError, match="rng is required"):
        E.sample_shadows_from_state(_bell_state(), 2, num_snapshots=10, rng=None)


def test_shadows_empty_snapshots_raise():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    with pytest.raises(ValueError, match="empty snapshot"):
        E.estimate_observable_from_snapshots([], "ZZ")
    with pytest.raises(ValueError, match="empty snapshot"):
        E.estimate_matrix_observable([], np.eye(4))


def test_shadows_unknown_basis_raises():
    from kubona.circuits.shadows import (
        ClassicalShadowEngine as E, ShadowSnapshot)
    with pytest.raises(ValueError, match="unknown snapshot basis"):
        E.reconstruct_snapshot_matrix(ShadowSnapshot(bases=["W"], outcomes=[0]))


def test_shadows_out_of_range_params_raise():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    with pytest.raises(ValueError, match="epsilon"):
        E.sample_complexity_bound(100, 0.0)
    with pytest.raises(ValueError, match="epsilon"):
        E.sample_complexity_bound(100, 1.5)
    with pytest.raises(ValueError, match="failure_prob_delta"):
        E.sample_complexity_bound(100, 0.1, failure_prob_delta=0.9 + 0.2)
    with pytest.raises(ValueError, match="num_observables_m"):
        E.sample_complexity_bound(0, 0.1)
    with pytest.raises(ValueError, match="pauli_weight"):
        E.compute_shadow_norm(-1)


def test_shadows_named_consts_and_norm():
    from kubona.circuits import shadows as sh
    assert sh.DEFAULT_NUM_SNAPSHOTS_V1_HEURISTIC == 500
    assert sh.DEFAULT_NUM_CHUNKS_V1_HEURISTIC == 5
    assert sh.SHADOW_NORM_BASE_V1 == 3.0
    assert sh.ClassicalShadowEngine.compute_shadow_norm(2) == 9.0


def test_shadows_balanced_median_of_means():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    # array_split([1..5], 3) -> [1,2],[3,4],[5]: means 1.5,3.5,5 -> median 3.5.
    # The old len//chunks slicing dropped the tail (median 2.0).
    assert E._median_of_means([1, 2, 3, 4, 5], 3) == pytest.approx(3.5)


def test_shadows_qubit_cap_and_width_validation():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    with pytest.raises(ValueError, match="above the cap|refused for n="):
        E.sample_shadows_from_state(np.zeros(4, dtype=complex), 21,
                                    num_snapshots=2, rng=np.random.default_rng(0))
    with pytest.raises(ValueError, match="!="):
        E.sample_shadows_from_state(np.zeros(8, dtype=complex), 2,
                                    num_snapshots=2, rng=np.random.default_rng(0))
    with pytest.raises(ValueError, match="non-zero norm"):
        E.sample_shadows_from_state(np.zeros(4, dtype=complex), 2,
                                    num_snapshots=2, rng=np.random.default_rng(0))


def test_shadows_bases_are_plain_str():
    from kubona.circuits.shadows import ClassicalShadowEngine as E
    snaps = E.sample_shadows_from_state(_bell_state(), 2, num_snapshots=5,
                                        rng=np.random.default_rng(0))
    assert all(type(b) is str for s in snaps for b in s.bases)


# ---------------------------------------------------------------------------
# krylov.py
# ---------------------------------------------------------------------------

def _tfim() -> np.ndarray:
    Z = np.array([[1, 0], [0, -1]], dtype=complex)
    X = np.array([[0, 1], [1, 0]], dtype=complex)
    I = np.eye(2, dtype=complex)
    return -np.kron(Z, Z) - 0.5 * (np.kron(X, I) + np.kron(I, X))


def test_krylov_reference_state_required():
    from kubona.circuits.krylov import OrthogonalQKSDSolver, QKSDSolver
    H = _tfim()
    with pytest.raises(ValueError, match="initial_state is required"):
        QKSDSolver.solve(H, subspace_dim=2, time_step=0.2)
    with pytest.raises(ValueError, match="initial_state is required"):
        OrthogonalQKSDSolver.solve(H, subspace_dim=2)


def test_krylov_provenance_and_full_precision():
    from kubona.circuits.krylov import QKSDSolver
    H = _tfim()
    psi0 = np.zeros(4, dtype=complex)
    psi0[0] = 1.0
    res = QKSDSolver.solve(H, initial_state=psi0, subspace_dim=4, time_step=0.2)
    assert res.provenance == "classical-emulation"
    d = res.to_dict()
    assert d["provenance"] == "classical-emulation"
    # Full precision: ground energy survives with >6dp fidelity.
    assert d["ground_state_energy"] == pytest.approx(res.ground_state_energy, rel=1e-12)
    assert res.is_converged is True
    assert math.isfinite(res.ritz_residual)


def test_krylov_truncation_recorded_and_all_singular_unconverged():
    from kubona.circuits.krylov import QKSDSolver
    H = _tfim()
    psi0 = np.zeros(4, dtype=complex)
    psi0[0] = 1.0
    # Absurd cutoff filters every mode -> fallback path, unconverged, recorded.
    res = QKSDSolver.solve(H, initial_state=psi0, subspace_dim=4, time_step=0.2,
                           regularization_cutoff=1e6)
    assert res.is_converged is False
    assert res.discarded_modes == 3


def test_krylov_cond_inf_and_hermiticity_refusal():
    from kubona.circuits.krylov import QKSDSolver
    H = _tfim()
    psi0 = np.zeros(4, dtype=complex)
    psi0[0] = 1.0
    # time_step=0: all Krylov states identical -> singular overlap -> cond inf.
    res = QKSDSolver.solve(H, initial_state=psi0, subspace_dim=4, time_step=0.0)
    assert res.condition_number == math.inf
    assert res.discarded_modes == 3
    bad = np.array([[0, 1], [0, 0]], dtype=complex)
    with pytest.raises(ValueError, match="not Hermitian"):
        QKSDSolver.solve(bad, initial_state=np.array([1, 0], dtype=complex))
    with pytest.raises(ValueError, match="square"):
        QKSDSolver.solve(np.zeros((2, 3), dtype=complex),
                         initial_state=np.array([1, 0], dtype=complex))


def test_krylov_small_subspace_honestly_unconverged():
    from kubona.circuits.krylov import QKSDSolver
    # Diagonal 4-level system; 1-dim Krylov from |+> cannot resolve the ground state.
    H = np.diag([0.0, 1.0, 2.0, 3.0]).astype(complex)
    psi0 = np.ones(4, dtype=complex) / 2.0
    res = QKSDSolver.solve(H, initial_state=psi0, subspace_dim=1, time_step=0.5)
    assert res.is_converged is False  # old code returned unconditional True


# ---------------------------------------------------------------------------
# cutter.py
# ---------------------------------------------------------------------------

def _pricing(**kw):
    from kubona.circuits.cutter import PricingConfig
    base = dict(max_shot_multiplier=4096.0, base_shots=1000,
                budget_usd_cap=50.0, shot_rate_per_k=0.001)
    base.update(kw)
    return PricingConfig(**base)


def test_cutter_requires_explicit_pricing():
    from kubona.circuits.cutter import CircuitCuttingOptimizer
    with pytest.raises((ValueError, TypeError)):
        CircuitCuttingOptimizer()  # type: ignore[call-arg]
    with pytest.raises(ValueError, match="base_shots"):
        _pricing(base_shots=0)


def test_cutter_circuit_aware_k_counts_spanning_gates():
    from kubona.circuits.cutter import CircuitCuttingOptimizer
    opt = CircuitCuttingOptimizer(_pricing())
    # 4 qubits -> partitions [0,1],[2,3]; one CX(1,2) spans -> K=1.
    one_span = Circuit(name="c", num_qubits=4,
                       gates=[Gate("h", (0,)), Gate("cx", (1, 2)), Gate("h", (3,))])
    plan = opt.plan_cuts(4, 2, circuit=one_span)
    assert plan.k_cuts == 1
    assert plan.cut_locations == [1]
    assert plan.classical_shot_multiplier == 4.0
    # No inter-partition 2q gates -> K=0 (the old num_partitions-1 bound said 1).
    no_span = Circuit(name="c", num_qubits=4,
                      gates=[Gate("h", (0,)), Gate("h", (3,))])
    plan0 = opt.plan_cuts(4, 2, circuit=no_span)
    assert plan0.k_cuts == 0
    assert plan0.classical_shot_multiplier == 1.0
    # No circuit -> documented bound, not a measurement.
    bound = opt.plan_cuts(4, 2)
    assert bound.k_cuts == 1
    assert "bound, not a measurement" in (bound.overhead_warning or "")


def test_cutter_advisory_label_and_width_validation():
    from kubona.circuits.cutter import CircuitCuttingOptimizer
    opt = CircuitCuttingOptimizer(_pricing())
    plan = opt.plan_cuts(4, 2)
    assert plan.cost_feasible_advisory is plan.cost_feasible
    assert "ADVISORY" in (plan.overhead_warning or "")
    with pytest.raises(ValueError, match="target_qpu_width"):
        opt.plan_cuts(4, 0)
    with pytest.raises(ValueError, match="width"):
        opt.plan_cuts(4, 2, circuit=Circuit(name="c", num_qubits=3,
                                            gates=[Gate("h", (0,))]))


def test_cutter_cap_refuses_before_multiply():
    from kubona.circuits.cutter import CircuitCuttingOptimizer
    opt = CircuitCuttingOptimizer(_pricing(max_shot_multiplier=16.0))
    # K=3 spanning gates -> 64 > 16: refuse, never multiply into shots.
    gates = [Gate("cx", (1, 2)), Gate("cx", (1, 2)), Gate("cx", (1, 2))]
    c = Circuit(name="c", num_qubits=4, gates=gates)
    with pytest.raises(ValueError, match="above the configured"):
        opt.plan_cuts(4, 2, circuit=c)
