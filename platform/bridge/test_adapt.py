"""Tests for the full-space ADAPT-VQE implementation (bridge image deps)."""
from __future__ import annotations

import pytest


pytest.importorskip("qiskit")
pytest.importorskip("qiskit_nature")


def _lih_problem():
    from qiskit_nature.second_q.drivers import PySCFDriver
    from qiskit_nature.second_q.mappers import JordanWignerMapper

    problem = PySCFDriver(atom="Li 0 0 0; H 0 0 1.5", basis="sto-3g").run()
    mapper = JordanWignerMapper()
    return problem, mapper, mapper.map(problem.hamiltonian.second_q_op())


def test_uccsd_pool_is_explicit_and_hermitian():
    import adapt

    problem, mapper, _ = _lih_problem()
    pool = adapt.build_pool(problem, mapper, "uccsd")
    assert len(pool) > 1
    for name, op in pool:
        assert name.startswith("uccsd[")
        assert op.num_qubits == 12
        assert max(abs(complex(c).imag) for c in op.coeffs) < 1e-9


def test_ceo_pool_contains_coupled_exchange_operators():
    import adapt

    problem, mapper, _ = _lih_problem()
    pool = adapt.build_pool(problem, mapper, "ceo")
    names = [name for name, _ in pool]
    assert any(name.startswith("ceo+[") for name in names)
    assert any(name.startswith("ceo-[") for name in names)
    assert any(name.startswith("qe-single[") for name in names)
    # No duplicate or empty labelled operator.
    assert len(names) == len(set(names))
    for _, op in pool:
        assert len(op) > 0


def test_unknown_pool_refuses():
    import adapt

    problem, mapper, _ = _lih_problem()
    with pytest.raises(adapt.AdaptError):
        adapt.build_pool(problem, mapper, "made-up")


def test_adapt_never_claims_with_low_shot_uncertainty():
    import adapt
    from experiments import _full_fci_and_casci

    problem, mapper, qop = _lih_problem()
    constant = float(sum(problem.hamiltonian.constants.values()))
    fci, _ = _full_fci_and_casci("Li 0 0 0; H 0 0 1.5", 2, 2)
    res = adapt.run_adapt_vqe(
        problem, qop, constant, mapper, fci,
        adapt.AdaptConfig(pool="uccsd", max_ops=1, max_iterations=1,
                          shots=128, time_limit_s=120, seed=7),
    )
    assert res.selected
    assert res.uncertainty > 1.6e-3
    assert res.delta_mha >= 0
    # The exact optimizer value is not allowed to turn the claim on.
    assert res.energy_measured != 0

def test_shot_noise_cannot_override_exact_target_gate(monkeypatch):
    import adapt
    from experiments import _full_fci_and_casci

    problem, mapper, qop = _lih_problem()
    constant = float(sum(problem.hamiltonian.constants.values()))
    fci, _ = _full_fci_and_casci("Li 0 0 0; H 0 0 1.5", 2, 2)

    def fake_measure(state, hamiltonian, const, shots, seed):
        # Measured energy lands inside the target, while the exact state
        # (one operator, one iteration) is still far outside it.
        return fci + 0.0002, 0.0001, {}

    monkeypatch.setattr(adapt, "_measure_state", fake_measure)
    res = adapt.run_adapt_vqe(
        problem, qop, constant, mapper, fci,
        adapt.AdaptConfig(pool="uccsd", max_ops=1, max_iterations=1,
                          shots=128, time_limit_s=120, seed=7),
    )
    assert res.delta_mha <= 1.6
    assert res.uncertainty <= 1.6
    assert res.exact_delta_mha > 1.6
    assert res.stopped == "exact_target_not_reached"
