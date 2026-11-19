import importlib


def _policy(monkeypatch, budget="4096"):
    monkeypatch.setenv("KUBONA_BRIDGE_MEM_MB", budget)
    monkeypatch.delenv("KUBONA_SIM_METHOD", raising=False)
    import simulator
    importlib.reload(simulator)
    return simulator


def test_budget_and_aer_options_follow_explicit_budget(monkeypatch):
    policy = _policy(monkeypatch, "4096")
    assert policy.memory_budget_mb() == 4096
    opts = policy.aer_options()
    assert opts["max_memory_mb"] == 4096
    assert opts["max_parallel_threads"] >= 1


def test_wide_shallow_chooses_mps_and_narrow_stays_statevector(monkeypatch):
    policy = _policy(monkeypatch, "4096")
    from qiskit import QuantumCircuit

    wide = QuantumCircuit(26)
    wide.h(range(26))
    for q in range(25):
        wide.cx(q, q + 1)
    assert policy.choose_method(wide) == "matrix_product_state"

    narrow = QuantumCircuit(8)
    narrow.h(range(8))
    for q in range(7):
        narrow.cx(q, q + 1)
    assert policy.choose_method(narrow) is None


def test_explicit_method_wins(monkeypatch):
    policy = _policy(monkeypatch, "4096")
    monkeypatch.setenv("KUBONA_SIM_METHOD", "statevector")
    import importlib as _il
    _il.reload(policy)
    from qiskit import QuantumCircuit

    qc = QuantumCircuit(26)
    qc.h(range(26))
    assert policy.choose_method(qc) == "statevector"


def test_chemistry_cache_returns_isolated_copies():
    from bridge import chemistry

    chemistry.solve_classical_anchor.cache_clear() if hasattr(chemistry.solve_classical_anchor, "cache_clear") else None
    first = chemistry.solve_classical_anchor("H2", 0.735, "sto-3g")
    first["rhf_energy"] = -999.0
    second = chemistry.solve_classical_anchor("H2", 0.735, "sto-3g")
    assert second["rhf_energy"] != -999.0
    info = chemistry._cached_anchor.cache_info()
    assert info.hits >= 1
