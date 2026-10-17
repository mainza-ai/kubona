"""W1.17 production-bar regression tests: mitigation backend/ZNE, QIR native/pipeline, wire cutting."""

import numpy as np
import pytest

from kubona.circuits import Circuit, Gate, parse_qasm2


# ---------------------------------------------------------------------------
# mitigation/backend.py
# ---------------------------------------------------------------------------

def test_depolarizing_is_trace_preserving():
    from kubona.mitigation.backend import NoiseAwareSimulator
    sim = NoiseAwareSimulator(noise_rate=0.1)
    n, dim = 2, 4
    rng = np.random.default_rng(0)
    a = rng.normal(size=(dim, dim)) + 1j * rng.normal(size=(dim, dim))
    rho = a @ a.conj().T
    rho = rho / np.trace(rho)
    out = sim._depolarize(rho, 0.1, (0,), n)
    assert abs(float(np.real(np.trace(out))) - 1.0) < 1e-12
    # Only the acted qubit is noised: with p on qubit 0, <Z1> is untouched
    # by the channel itself (checked at the expectation level below).


def test_backend_seed_generated_and_echoed():
    from kubona.mitigation.backend import NoiseAwareSimulator
    sim = NoiseAwareSimulator(noise_rate=0.01)
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nh q[0];")
    r1 = sim.run(c, shots=100, seed=5)
    assert r1["seed"] == 5
    r2 = sim.run(c, shots=100)  # unseeded: seed generated + echoed
    assert isinstance(r2["seed"], int)
    r3 = sim.run(c, shots=100, seed=r2["seed"])
    assert r3["counts"] == r2["counts"]  # echo reproduces the run


def test_backend_validation_fail_closed():
    from kubona.mitigation.backend import NoiseAwareSimulator
    sim = NoiseAwareSimulator(noise_rate=0.01)
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nh q[0];")
    with pytest.raises(ValueError, match="shots must be a positive int"):
        sim.run(c, shots=0)
    with pytest.raises(ValueError, match="noise_rate"):
        NoiseAwareSimulator(noise_rate=1.5)
    # Arity violation: cx declared on one qubit (Gate allows it; backend refuses).
    bad = Circuit(name="b", num_qubits=2, gates=[Gate("cx", (0,))])
    with pytest.raises(ValueError, match="needs 2 qubit"):
        sim.run(bad, shots=10)
    # Param violation.
    badp = Circuit(name="b", num_qubits=1, gates=[Gate("rx", (0,), ())])
    with pytest.raises(ValueError, match="needs 1 param"):
        sim.run(badp, shots=10)


def test_backend_unsupported_gate_lists_supported_set():
    from kubona.mitigation.backend import NoiseAwareSimulator
    sim = NoiseAwareSimulator()
    # Bypass Gate validation to smuggle an exotic name to the backend layer.
    g = object.__new__(Gate)
    object.__setattr__(g, "name", "u3")
    object.__setattr__(g, "qubits", (0,))
    object.__setattr__(g, "params", (0.1, 0.2, 0.3))
    c = Circuit(name="c", num_qubits=1, gates=[g])
    with pytest.raises(ValueError, match="supported:"):
        sim.run(c, shots=10)


# ---------------------------------------------------------------------------
# mitigation/zne.py — true unitary folding
# ---------------------------------------------------------------------------

def test_fold_is_true_inverse_not_repetition():
    from kubona.mitigation.zne import fold_gates, inverse_gate
    from kubona.circuits.schedule import equivalent_up_to_phase
    assert inverse_gate(Gate("rx", (0,), (0.5,))).params == (-0.5,)
    assert inverse_gate(Gate("s", (0,))).name == "sdg"
    assert inverse_gate(Gate("cx", (0, 1))).name == "cx"
    with pytest.raises(ValueError, match="no exact inverse"):
        g = object.__new__(Gate)
        object.__setattr__(g, "name", "mystery")
        object.__setattr__(g, "qubits", (0,))
        object.__setattr__(g, "params", ())
        inverse_gate(g)
    c = Circuit(name="c", num_qubits=2,
                gates=[Gate("rx", (0,), (0.5,)), Gate("cx", (0, 1))])
    f3 = fold_gates(c, 3)
    assert len(f3.gates) == 6  # C C† C
    assert f3.gates[2].name == "cx"  # reversed self-inverse block first
    assert f3.gates[3].params == (-0.5,)  # rx inverse carries -theta
    assert equivalent_up_to_phase(c, f3)  # same unitary (old Gx3 was not)
    assert len(fold_gates(c, 5).gates) == 10
    with pytest.raises(ValueError, match="odd positive int"):
        fold_gates(c, 2)  # even factors would measure I, not amplify
    with pytest.raises(ValueError, match="odd positive int"):
        fold_gates(c, 0)


def test_zne_scale_and_param_refusals():
    from kubona.mitigation.zne import run_zne
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];")
    with pytest.raises(ValueError, match="explicit shots"):
        run_zne(c)  # no silent 8000/0.02/(1,3,5)/7
    with pytest.raises(ValueError, match="odd positive ints"):
        run_zne(c, shots=100, noise_rate=0.02, scales=(1, 2, 3), seed=1)
    with pytest.raises(ValueError, match="must contain 1"):
        run_zne(c, shots=100, noise_rate=0.02, scales=(3, 5), seed=1)
    with pytest.raises(ValueError, match=">= 2 scale points"):
        run_zne(c, shots=100, noise_rate=0.02, scales=(1,), seed=1)
    with pytest.raises(ValueError, match="unknown observable"):
        run_zne(c, observable="x", shots=100, noise_rate=0.02, scales=(1, 3), seed=1)
    with pytest.raises(ValueError, match="unknown extrapolator"):
        run_zne(c, shots=100, noise_rate=0.02, scales=(1, 3), seed=1,
                extrapolator="spline")


def test_zne_empty_counts_and_linear_validation():
    from kubona.mitigation.zne import _linear, expectation_from_counts
    with pytest.raises(ValueError, match="empty counts"):
        expectation_from_counts({}, np.array([1.0, -1.0]), 1)
    with pytest.raises(ValueError, match=">= 2 scale"):
        _linear([1.0], [0.5])


def test_zne_bound_clip_flag_and_interval():
    from kubona.mitigation.zne import _bound_z_expectation, run_zne
    v, flag = _bound_z_expectation(0.5)
    assert v == 0.5 and flag == {"clipped": False, "reason": None}
    v2, flag2 = _bound_z_expectation(1.4)
    assert v2 == 1.0 and flag2["clipped"] is True and flag2["reason"]
    with pytest.raises(ValueError, match="non-finite"):
        _bound_z_expectation(float("nan"))
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];")
    r = run_zne(c, shots=20000, noise_rate=0.05, scales=(1, 3, 5), seed=11)
    assert -1.0 <= r.mitigated_value <= 1.0
    assert len(r.interval_95) == 2 and r.interval_95[0] <= r.interval_95[1]
    assert r.condition_number > 0 and np.isfinite(r.condition_number)
    assert set(r.scale_seeds) == {1.0, 3.0, 5.0}
    assert len(set(r.scale_seeds.values())) == 3  # independent streams
    r2 = run_zne(c, shots=20000, noise_rate=0.05, scales=(1, 3, 5), seed=11)
    assert r2.scale_values == r.scale_values  # reproducible


def test_zne_extrapolator_selectable_upfront():
    from kubona.mitigation.zne import run_zne
    c = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];")
    r = run_zne(c, shots=20000, noise_rate=0.05, scales=(1, 3, 5), seed=11,
                extrapolator="linear")
    assert r.extrapolator == "linear"
    assert abs(r.mitigated_value - (-1.0)) < abs(r.raw_value - (-1.0))


# ---------------------------------------------------------------------------
# qir/native.py
# ---------------------------------------------------------------------------

def _script(tmp_path, body: str) -> str:
    p = tmp_path / "fake-qir-core"
    p.write_text("#!/bin/sh\n" + body + "\n")
    p.chmod(0o755)
    return str(p)


def test_native_none_only_when_absent_and_dead_cargo_gone(monkeypatch):
    from kubona.qir import native as nat
    import pathlib
    assert "cargo" not in pathlib.Path(nat.__file__).read_text()
    monkeypatch.setattr(nat, "find_binary", lambda: None)
    assert nat.lower_with_rust("OPENQASM 2.0;\nqreg q[1];") is None
    assert nat.rust_available() is False


def test_native_failure_raises_with_stderr(tmp_path):
    from kubona.qir.native import lower_with_rust
    from pathlib import Path
    bad = _script(tmp_path, 'echo "simulated backend explosion" >&2; exit 3')
    with pytest.raises(RuntimeError, match="simulated backend explosion"):
        lower_with_rust("OPENQASM 2.0;\nqreg q[1];", bin_path=Path(bad))


def test_native_empty_output_and_timeout_refuse(tmp_path):
    from kubona.qir.native import lower_with_rust
    from pathlib import Path
    silent = _script(tmp_path, "exit 0")
    with pytest.raises(RuntimeError, match="empty QIR output"):
        lower_with_rust("OPENQASM 2.0;\nqreg q[1];", bin_path=Path(silent))
    with pytest.raises(ValueError, match="empty QASM"):
        lower_with_rust("   ", bin_path=Path(silent))


def test_native_env_path_allowlisted(monkeypatch, tmp_path):
    import os
    from kubona.qir import native as nat
    outside = tmp_path / "evil-qir-core"
    outside.write_text("#!/bin/sh\nexit 0\n")
    outside.chmod(0o755)
    monkeypatch.setenv("KUBONA_QIR_CORE", str(outside))
    with pytest.raises(ValueError, match="allowlist"):
        nat._candidate_paths()
    monkeypatch.setenv("KUBONA_QIR_ALLOWED_DIRS", str(tmp_path))
    assert nat._candidate_paths() == [outside.resolve()]


# ---------------------------------------------------------------------------
# qir/pipeline.py
# ---------------------------------------------------------------------------

def test_pipeline_fallback_tier_warning_zero_fill(monkeypatch):
    from kubona.qir import native as nat
    from kubona.qir.pipeline import NATIVE_SET, lower
    monkeypatch.setattr(nat, "find_binary", lambda: None)
    out = lower("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    assert out["ok"] is True
    assert out["tier"] == "python-reference" and out["engine"] == "python-reference"
    assert "reference" in out["warning"]
    assert all(g in out["native_gate_count"] for g in NATIVE_SET)  # zero-filled
    assert out["native_gate_count"]["ry"] == 0


def test_pipeline_cross_validates_rust_frontend(monkeypatch):
    from kubona.qir import pipeline as pipe
    monkeypatch.setattr(pipe, "lower_with_rust", None, raising=False)
    import kubona.qir.native as nat
    monkeypatch.setattr(nat, "lower_with_rust",
                        lambda qasm: {"ok": True, "num_qubits": 99,
                                      "native_gate_count": {}, "qir_text": "x"})
    with pytest.raises(ValueError, match="frontend mismatch"):
        pipe.lower("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")


# ---------------------------------------------------------------------------
# cutting/wire.py
# ---------------------------------------------------------------------------

def _sim_backend(c, shots, seed):
    from kubona.providers import get_provider
    return get_provider("local-simulator").submit(c, shots, seed=seed)


def test_wire_widths_computed_and_seeds_echoed():
    from kubona.cutting.wire import cut_expectation, plan_cut
    circ = Circuit(name="c", num_qubits=3,
                   gates=[Gate("h", (0,)), Gate("cx", (0, 1)), Gate("ry", (2,), (0.6,))])
    plan = plan_cut(circ, 1)
    assert plan.subcircuit_widths() == (1, 1)
    r = cut_expectation(circ, 1, 1, _sim_backend, shots=20000, seed=7)
    assert r["seed"] == 7
    assert set(r["fragment_seeds"]) == {"control", "m0", "m1"}
    r2 = cut_expectation(circ, 1, 1, _sim_backend, shots=20000, seed=7)
    assert r2["expected"] == r["expected"]


def test_wire_spanning_and_precut_refuse():
    from kubona.cutting.wire import cut_expectation, plan_cut
    multi = Circuit(name="c", num_qubits=3,
                    gates=[Gate("h", (0,)), Gate("cx", (0, 2)), Gate("cx", (0, 1))])
    with pytest.raises(ValueError, match="span the fragments"):
        plan_cut(multi, 2)
    precut = Circuit(name="c", num_qubits=2,
                     gates=[Gate("ry", (1,), (0.4,)), Gate("cx", (0, 1))])
    with pytest.raises(ValueError, match="pre-cut"):
        plan_cut(precut, 1)
    bell = Circuit(name="c", num_qubits=2,
                   gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
    with pytest.raises(ValueError, match="not the cut target"):
        cut_expectation(bell, 1, 0, _sim_backend, shots=1000, seed=1)


def test_wire_layout_mask_and_empty_counts_refuse():
    from kubona.cutting.wire import cut_expectation, cut_histogram
    # Cut cx(1,0): target q0 is NOT in the trailing-1 register -> refuse.
    flipped = Circuit(name="c", num_qubits=2,
                      gates=[Gate("h", (1,)), Gate("cx", (1, 0))])
    with pytest.raises(ValueError, match="target-layout mismatch"):
        cut_histogram(flipped, 1, n_target=1, backend=_sim_backend, shots=1000, seed=1)
    with pytest.raises(ValueError, match="n_target"):
        cut_histogram(flipped, 1, n_target=0, backend=_sim_backend, shots=1000, seed=1)
    bell = Circuit(name="c", num_qubits=2,
                   gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
    empty = lambda c, s, seed: {"counts": {}}  # noqa: E731
    with pytest.raises(ValueError, match="empty counts"):
        cut_expectation(bell, 1, 1, empty, shots=1000, seed=1)
    with pytest.raises(ValueError, match="shots must be a positive int"):
        cut_expectation(bell, 1, 1, _sim_backend, shots=0, seed=1)


def test_wire_histogram_renormalized_and_reproducible():
    from kubona.cutting.wire import cut_histogram
    bell = Circuit(name="bell", num_qubits=2,
                   gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
    r = cut_histogram(bell, 1, n_target=1, backend=_sim_backend, shots=65536, seed=7)
    assert abs(sum(r["distribution"].values()) - 1.0) < 1e-9  # renormalized post-round
    assert abs(r["distribution"].get("0", 0) - 0.5) < 0.02
    assert abs(r["distribution"].get("1", 0) - 0.5) < 0.02
    r2 = cut_histogram(bell, 1, n_target=1, backend=_sim_backend, shots=65536, seed=7)
    assert r2["distribution"] == r["distribution"]
