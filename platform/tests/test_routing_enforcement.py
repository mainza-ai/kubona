"""P5 §2.9 routing enforcement: hardware needs declared, eligible callers.

Covers the three rails: submit_job declaration mandate, the bridge executor
hardware gate (no silent simulation on refusal, no spend attempt), and the
Go admission presence check (Go side, in jobs_test.go).
"""

from kubona.mcp.servers import execution as exec_server

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def test_submit_job_hardware_requires_declaration():
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                 budget_ceiling_usd=150.0)
    assert out["ok"] is False
    assert "caller_jurisdictions" in out["error"]


def test_submit_job_ineligible_route_refused():
    out = exec_server.submit_job(BELL, provider="ibm-qiskit", shots=100,
                                 budget_ceiling_usd=150.0,
                                 caller_jurisdictions=["AQ"])
    assert out["ok"] is False
    assert "routing blocked" in out["error"]


def test_submit_job_simulator_needs_no_declaration():
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=100)
    assert out["ok"] is True, out


def _load_executor():
    """Import platform/bridge/executor.py by path (no packaging change)."""
    import importlib.util
    from pathlib import Path
    path = Path(__file__).resolve().parent.parent / "bridge" / "executor.py"
    spec = importlib.util.spec_from_file_location("bridge_executor_routing", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def test_executor_hardware_undeclared_refuses_without_spend(monkeypatch):
    mod = _load_executor()
    called = []

    def _boom(*args, **kwargs):
        called.append(1)
        raise AssertionError("must not attempt hardware execution undeclared")

    monkeypatch.setattr(mod, "_execute_ibm_hardware", _boom)
    out = mod.execute_qasm(BELL, shots=64, requested_backend="ibm_brisbane")
    assert out["success"] is False
    assert "routing refused" in out["error_message"]
    assert "caller_jurisdictions" in out["error_message"]
    assert called == []  # zero spend attempt


def test_executor_hardware_ineligible_refuses_without_simulation(monkeypatch):
    mod = _load_executor()
    called = []
    monkeypatch.setattr(mod, "_execute_ibm_hardware",
                        lambda *a, **k: called.append(1))
    out = mod.execute_qasm(BELL, shots=64, requested_backend="ibm_brisbane",
                           caller_jurisdictions=["AQ"])
    assert out["success"] is False
    assert "routing refused" in out["error_message"]
    assert "jurisdiction mismatch" in out["error_message"]
    assert called == []
    assert "simulation" not in out["degradation_reason"]  # refusal, not theater


def test_executor_hardware_eligible_proceeds_past_gate():
    mod = _load_executor()
    out = mod.execute_qasm(BELL, shots=64, requested_backend="ibm_brisbane",
                           caller_jurisdictions=["US"])
    assert "routing refused" not in out.get("error_message", "")
    assert "routing refused" not in out.get("degradation_reason", "")


def test_executor_simulator_path_untouched_by_routing():
    mod = _load_executor()
    out = mod.execute_qasm(BELL, shots=64, requested_backend="aer_simulator")
    assert out["success"] is True, out
