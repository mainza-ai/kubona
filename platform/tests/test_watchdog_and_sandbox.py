"""Tests for Ring 0 Mathematical Watchdog and Workload Sandbox."""

import pytest
from kubona.autoresearch import Orchestrator, build_vqe_ansatz_harness
from kubona.control.watchdog import Ring0Watchdog, WatchdogThresholds
from kubona.sandbox import SecurityPolicyViolation, WorkloadSandbox


def test_watchdog_nominal_continue():
    dog = Ring0Watchdog()
    # Decreasing energy / improving metric
    v1 = dog.record_step(0, 0.5, 0.1)
    assert v1.action == "CONTINUE"
    v2 = dog.record_step(1, 0.6, 0.2)
    assert v2.action == "CONTINUE"


def test_watchdog_detects_nan_divergence():
    dog = Ring0Watchdog()
    dog.record_step(0, 1.0, 0.5)
    v = dog.record_step(1, float("nan"), 0.5)
    assert v.action == "ABORT_TO_SNAPSHOT"
    assert "Numerical instability" in v.reason


def test_watchdog_detects_slope_divergence():
    dog = Ring0Watchdog(WatchdogThresholds(max_divergence_slope=3.0))
    dog.record_step(0, 1.0, 0.1)
    dog.record_step(1, 2.0, 0.2)
    dog.record_step(2, 3.0, 0.3)
    v = dog.record_step(3, 20.0, 0.4)  # Sudden spike
    assert v.action == "ABORT_TO_SNAPSHOT"
    assert "Cost slope divergence" in v.reason


def test_watchdog_detects_stagnation():
    dog = Ring0Watchdog(WatchdogThresholds(stagnation_steps=4, stagnation_delta=1e-4))
    for i in range(3):
        v = dog.record_step(i, 0.500001, 0.1)
        assert v.action == "CONTINUE"
    v4 = dog.record_step(3, 0.500001, 0.1)
    assert v4.action == "PAUSE"
    assert "stagnant" in v4.reason


def test_watchdog_detects_barren_plateau_and_rescopes():
    # Identical parameters over window -> zero variance in parameter updates
    dog = Ring0Watchdog()
    for i in range(4):
        dog.record_step(i, 0.5, 0.1000)
    v5 = dog.record_step(4, 0.5, 0.1000)
    assert v5.action == "RE_SCOPE"
    assert "Barren Plateau" in v5.reason


def test_sandbox_ast_validation_allows_pure_math():
    sb = WorkloadSandbox()
    sb.validate_ast("import math\nx = math.sin(0.5)")


def test_sandbox_ast_validation_blocks_os_and_subprocess():
    sb = WorkloadSandbox()
    with pytest.raises(SecurityPolicyViolation) as exc:
        sb.validate_ast("import os\nos.system('ls')")
    assert "Disallowed import" in str(exc.value)

    with pytest.raises(SecurityPolicyViolation) as exc2:
        sb.validate_ast("import subprocess\nsubprocess.run(['ls'])")
    assert "Disallowed import" in str(exc2.value)


def test_sandbox_ast_validation_blocks_eval_exec():
    sb = WorkloadSandbox()
    with pytest.raises(SecurityPolicyViolation) as exc:
        sb.validate_ast("eval('1 + 1')")
    assert "Disallowed call to builtin" in str(exc.value)


def test_sandbox_subprocess_execution():
    sb = WorkloadSandbox(max_cpu_seconds=2)
    script = """
import math
print(f"VAL={math.sqrt(16.0)}")
"""
    res = sb.execute_script(script)
    assert res.success
    assert "VAL=4.0" in res.stdout


def test_sandbox_timeout_terminates_process_group():
    sb = WorkloadSandbox(max_cpu_seconds=5)
    script = """
import time
time.sleep(5)
print("COMPLETED")
"""
    res = sb.execute_script(script, timeout_sec=0.5)
    assert not res.success
    assert "timed out" in res.stderr
    assert res.violation == "timeout"


def test_orchestrator_runs_with_active_watchdog():
    harness = build_vqe_ansatz_harness()
    orch = Orchestrator(harness, budget_steps=15, seed=42)
    history = orch.search()
    assert len(history) > 0
    # Verify trials carry watchdog metadata
    assert "watchdog" in history[0].meta
    assert history[0].meta["watchdog"]["action"] in ("CONTINUE", "RE_SCOPE", "PAUSE", "ABORT_TO_SNAPSHOT")


def test_mcclean_cerezo_barren_plateau_detector_and_negative_certificate():
    import numpy as np
    from kubona.watchdog.barren_plateau import McCleanCerezoBarrenPlateauDetector

    # Case 1: Trapped barren plateau with near-constant function
    flat_cost_fn = lambda theta: 1.0 + 1e-6 * float(np.sum(np.sin(theta)))
    detector = McCleanCerezoBarrenPlateauDetector(num_qubits=8, variance_floor=1e-5)
    params = np.zeros(8)

    tripped, cert, metrics = detector.check_and_certify(
        flat_cost_fn,
        params,
        ansatz_pattern="HardwareEfficient_DeepAnsatz_CZ",
        hypothesis_hash="0xabcdef1234567890",
        target_system="FeMoco",
    )
    assert tripped is True
    assert cert is not None
    assert cert.refutation_type == "BARREN_PLATEAU"
    assert "HardwareEfficient_DeepAnsatz_CZ" in cert.properties["ansatz_pattern"]
    assert metrics["is_barren_plateau"] is True
    assert metrics["mean_variance"] < 1e-5

    # Case 2: Healthy trainable landscape with significant gradient variance
    steep_cost_fn = lambda theta: float(np.sum(theta ** 2))
    tripped_2, cert_2, metrics_2 = detector.check_and_certify(
        steep_cost_fn,
        np.array([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0]),
        ansatz_pattern="QuantumKrylovSubspace",
    )
    assert tripped_2 is False
    assert cert_2 is None
    assert metrics_2["is_barren_plateau"] is False
    assert metrics_2["mean_variance"] > 1e-3



def test_sandbox_refuses_local_execution_in_production(monkeypatch):
    """2026-10-26 audit P0-4: AST + rlimit is not a security boundary.
    Production must refuse the local path unless the container rail is on."""
    monkeypatch.setenv("KUBONA_ENV", "production")
    monkeypatch.delenv("KUBONA_SANDBOX_CONTAINER", raising=False)
    res = WorkloadSandbox().execute_script("print('should not run')")
    assert res.success is False
    assert res.violation == "isolation_required"
    assert "KUBONA_SANDBOX_CONTAINER" in res.stderr


def test_sandbox_blocks_native_and_file_escape_names():
    sb = WorkloadSandbox()
    for code in (
        "import numpy\nnumpy.ctypeslib.ctypes.CDLL(None)",
        "import numpy\nnumpy.fromfile('/etc/hosts')",
        "import numpy\nnumpy.load('/etc/passwd.npy')",
    ):
        with pytest.raises(SecurityPolicyViolation):
            sb.validate_ast(code)


def test_container_sandbox_runs_pure_math(monkeypatch):
    """Container path (opt-in): the production rail must actually execute."""
    import os
    import shutil
    if os.getenv("KUBONA_SANDBOX_CONTAINER") != "1" or shutil.which("docker") is None:
        pytest.skip("container sandbox not configured (CI builds the runner image)")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.setenv("KUBONA_SANDBOX_IMAGE",
                       os.getenv("KUBONA_SANDBOX_IMAGE", "kubona-quantum-runner:1.1.0"))
    res = WorkloadSandbox().execute_script("import math\nprint(f'CONTAINER_OK={math.sqrt(9.0)}')")
    assert res.success, res.stderr
    assert "CONTAINER_OK=3.0" in res.stdout
