"""Tests for OpenAI Math Repository integration, catalog, and Comparator verification."""

import pytest
from pathlib import Path
from kubona.science.math_catalog import MathCatalog, ResultFamily
from kubona.eval.comparator_eval import (
    ComparatorChallengeRegistry,
    ComparatorChallengeSpec,
    STANDARD_PERMITTED_AXIOMS,
)
from kubona.provenance import Ledger


def test_math_catalog_initialization():
    catalog = MathCatalog()
    catalog.initialize()
    stats = catalog.summary_stats()
    assert stats["total_families"] > 0
    assert stats["disciplines_count"] > 0
    assert stats["quantum_relevant_count"] >= 15
    assert stats["reasoning_traces_count"] >= 10


def test_math_catalog_family_retrieval_and_search():
    catalog = MathCatalog()
    # Test retrieving quantum-relevant family 271 (Bloch law / Heisenberg)
    fam271 = catalog.get_family(271)
    assert fam271 is not None
    assert fam271.family_id == 271
    assert "ferromagnetic" in fam271.title.lower() or "bloch" in fam271.title.lower()
    assert fam271.is_quantum_relevant
    assert fam271.has_reasoning_trace

    # Test search
    results = catalog.search("Heisenberg")
    assert any(f.family_id == 271 for f in results)

    # Test filtering by discipline
    phys_families = catalog.list_families(discipline="Mathematical physics")
    assert len(phys_families) > 0


def test_comparator_challenge_registry():
    registry = ComparatorChallengeRegistry()
    specs = registry.load_all()
    assert len(specs) > 0

    # Test challenge retrieval
    challenge = registry.get_challenge("QuasiRiemannHypothesis")
    assert challenge is not None
    assert "propext" in challenge.permitted_axioms
    assert "Quot.sound" in challenge.permitted_axioms
    assert "Classical.choice" in challenge.permitted_axioms


def test_comparator_axiom_verification(tmp_path):
    registry = ComparatorChallengeRegistry()
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))

    # Valid proof: only permitted axioms used
    res_valid = registry.verify_axioms(
        "QuasiRiemannHypothesis",
        ["propext", "Classical.choice"],
        ledger=ledger,
    )
    assert res_valid.ok
    assert len(res_valid.unpermitted_axioms) == 0
    assert res_valid.ledger_ref is not None

    # Invalid proof: unpermitted axiom / cheat axiom used
    res_invalid = registry.verify_axioms(
        "QuasiRiemannHypothesis",
        ["propext", "sorryAx", "arbitrary_magic_axiom"],
        ledger=ledger,
    )
    assert not res_invalid.ok
    assert "sorryAx" in res_invalid.unpermitted_axioms
    assert "arbitrary_magic_axiom" in res_invalid.unpermitted_axioms
    assert "Forbidden axioms detected" in (res_invalid.error_message or "")


def test_comparator_live_or_fallback(tmp_path):
    registry = ComparatorChallengeRegistry()
    ledger = Ledger(path=str(tmp_path / "ledger_live.jsonl"))

    # When lake is absent (standard host), verify_lean4_live falls back gracefully to AST containment
    res = registry.verify_lean4_live("QuasiRiemannHypothesis", ledger=ledger)
    assert res.ok
    assert res.verification_mode in ("live_kernel_proven", "static_ast_containment")
    if res.verification_mode == "static_ast_containment":
        assert res.kernel_execution_skipped is True
    else:
        assert res.kernel_execution_skipped is False
    assert res.ledger_ref is not None



def test_lean_sandbox_cmd_flags(tmp_path):
    """Q-31: the exact container invocation carries every resource cap."""
    from kubona.eval.comparator_eval import LeanSandboxConfig, sandbox_docker_cmd
    cfg = LeanSandboxConfig()
    cmd = sandbox_docker_cmd(cfg, tmp_path, "QuasiRiemannHypothesis")
    for flag in ["--read-only", "--network", "none", "--cap-drop", "ALL",
                 "--memory", "4g", "--memory-swap", "4g", "--cpus", "2.0",
                 "--pids-limit", "256", "--tmpfs", "/tmp"]:
        assert flag in cmd, (flag, cmd)
    assert cmd[-1] == "QuasiRiemannHypothesis"
    assert "kubona-lean:4.34.1" in cmd


def test_lean_sandbox_absent_daemon_falls_back(tmp_path):
    """Q-31: no daemon/image means AST fallback with reason, never an error."""
    from kubona.eval import comparator_eval as ce
    from kubona.eval.comparator_eval import (
        ComparatorChallengeRegistry, LeanSandboxConfig)
    from kubona.provenance import Ledger
    orig_avail, orig_present = ce.docker_available, ce.docker_image_present
    ce.docker_available = lambda: False
    ce.docker_image_present = lambda image: False
    try:
        registry = ComparatorChallengeRegistry()
        ledger = Ledger(path=str(tmp_path / "ledger_sandbox.jsonl"))
        res = registry.verify_lean4_live(
            "QuasiRiemannHypothesis", sandbox=LeanSandboxConfig(), ledger=ledger)
        assert res.verification_mode == "static_ast_containment"
        assert res.kernel_execution_skipped is True
        assert res.error_message is not None and "sandbox" in res.error_message
    finally:
        ce.docker_available, ce.docker_image_present = orig_avail, orig_present


def test_lean_sandbox_real_flag_execution():
    """Q-31: the exact sandbox flag set executes against the live daemon
    (busybox stands in for the Lean image, which needs a GB build)."""
    import shutil
    import subprocess
    from kubona.eval.comparator_eval import LeanSandboxConfig
    if shutil.which("docker") is None:
        pytest = __import__("pytest")
        pytest.skip("no docker daemon")
    cfg = LeanSandboxConfig(image="busybox:latest")
    cmd = ["docker", "run", "--rm", "--read-only", "--network", "none",
           "--cap-drop", "ALL", "--memory", cfg.memory,
           "--memory-swap", cfg.memory_swap, "--cpus", cfg.cpus,
           "--pids-limit", str(cfg.pids_limit), "--tmpfs", "/tmp",
           cfg.image, "true"]
    proc = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
    assert proc.returncode == 0, proc.stderr[:500]


def test_infra_failure_detector():
    from kubona.eval.comparator_eval import _is_infra_failure
    assert _is_infra_failure("docker: Cannot connect to the Docker daemon") is True
    assert _is_infra_failure("Error: no such image") is True
    assert _is_infra_failure("error: unknown tactic 'sorryAx', sorry") is False
    assert _is_infra_failure("") is False
