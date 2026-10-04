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


def test_orchestrator_runs_with_active_watchdog():
    harness = build_vqe_ansatz_harness()
    orch = Orchestrator(harness, budget_steps=15, seed=42)
    history = orch.search()
    assert len(history) > 0
    # Verify trials carry watchdog metadata
    assert "watchdog" in history[0].meta
    assert history[0].meta["watchdog"]["action"] in ("CONTINUE", "RE_SCOPE", "PAUSE", "ABORT_TO_SNAPSHOT")
