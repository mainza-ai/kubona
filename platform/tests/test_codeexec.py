"""Sandboxed code-execution harness: verdict discipline without a daemon.

Unit tests inject the container runner; live-container acceptance (golden
pass, malicious fail-closed, infra named) runs only where a daemon and the
pinned image exist — anything else skips, never passes.
"""

from types import SimpleNamespace

from kubona.eval.codeexec import (
    PyexecSandboxConfig,
    run_codeexec_suite,
    run_codeexec_task,
    sandbox_docker_cmd,
)

GOLDEN = {"name": "golden-add",
          "solution": "def add(a, b):\n    return a + b\n",
          "tests": "from solution import add\nassert add(2, 3) == 5\n"}


def _runner(code=0, stdout="", stderr=""):
    def _run(*args, **kwargs):
        return SimpleNamespace(stdout=stdout, stderr=stderr, returncode=code)
    return _run


def test_docker_cmd_hardened_flag_for_flag(tmp_path):
    cmd = sandbox_docker_cmd(PyexecSandboxConfig(), tmp_path)
    assert cmd[:2] == ["docker", "run"]
    for flag in (["--rm", "--read-only", "--network", "none",
                  "--cap-drop", "ALL", "--memory", "2g",
                  "--cpus", "1.0", "--pids-limit", "128",
                  "--tmpfs", "/tmp"]):
        assert flag in cmd, flag
    assert cmd[-4:] == ["--entrypoint", "python3",
                        "kubona-quantum-runner:1.1.0", "test_main.py"]
    assert f"{tmp_path}:/workspace:rw" in cmd


def test_golden_pass_only_on_exit_zero():
    rec = run_codeexec_task(GOLDEN, runner=_runner(0, "ok\n"))
    assert rec["ok"] is True and rec["skip"] is False
    assert len(rec["solution_sha256"]) == 64 and len(rec["tests_sha256"]) == 64


def test_test_failure_is_fail_not_skip():
    rec = run_codeexec_task(GOLDEN, runner=_runner(1, "", "AssertionError: 6 != 5"))
    assert rec["ok"] is False and rec["skip"] is False
    assert "AssertionError" in rec["stderr_tail"]


def test_malicious_attempt_evidence_captured():
    """A socket attempt dies on --network none; the evidence lands in the record."""
    rec = run_codeexec_task(
        {"name": "evil-socket", "solution": "import socket\n",
         "tests": "import solution\n"},
        runner=_runner(1, "", "socket.gaierror: [Errno 8] nodename nor servname provided"))
    assert rec["ok"] is False and rec["skip"] is False
    assert "socket.gaierror" in rec["stderr_tail"]


def test_infra_failure_is_skip_not_verdict():
    rec = run_codeexec_task(
        GOLDEN, runner=_runner(125, "", "docker: Cannot connect to the Docker daemon"))
    assert rec["ok"] is False and rec["skip"] is True
    assert "infra" in rec["reason"]


def test_timeout_kills_never_passes():
    import subprocess

    def _timeout(*args, **kwargs):
        raise subprocess.TimeoutExpired(cmd="docker", timeout=300)

    rec = run_codeexec_task(GOLDEN, runner=_timeout)
    assert rec["ok"] is False and "timeout" in rec["reason"]


def test_no_daemon_skips_without_running(monkeypatch):
    import kubona.eval.codeexec as ce

    monkeypatch.setattr(ce, "docker_available", lambda: False)
    rec = run_codeexec_task(GOLDEN, runner=None)
    assert rec["skip"] is True and rec["ok"] is False
    assert "no docker daemon" in rec["reason"]


def test_missing_image_refuses_implicit_pull(monkeypatch):
    import kubona.eval.codeexec as ce

    monkeypatch.setattr(ce, "docker_available", lambda: True)
    monkeypatch.setattr(ce, "docker_image_present", lambda image: False)
    rec = run_codeexec_task(GOLDEN, runner=None)
    assert rec["skip"] is True and "implicit pull" in rec["reason"]


def test_suite_all_skip_is_not_a_pass():
    assert run_codeexec_suite([])["ok"] is False
    all_skip = run_codeexec_suite(
        [dict(GOLDEN, name="a")],
        runner=_runner(125, "", "docker: Cannot connect to the Docker daemon"))
    assert all_skip["ok"] is False and all_skip["ran"] == 0
    green = run_codeexec_suite(
        [dict(GOLDEN, name="a"), dict(GOLDEN, name="b")],
        runner=_runner(0, "ok\n"))
    assert green["ok"] is True and green["ran"] == 2
    red = run_codeexec_suite(
        [dict(GOLDEN, name="a"), dict(GOLDEN, name="b")],
        runner=_runner(1, "", "AssertionError"))
    assert red["ok"] is False and red["failures"] == ["a", "b"]


def test_live_golden_and_evil_in_container():
    """End-to-end where the pinned image exists (skipped otherwise): a golden
    completion passes and a socket attempt fails closed with named evidence."""
    import pytest

    from kubona.eval.codeexec import docker_available, docker_image_present
    cfg = PyexecSandboxConfig()
    if not (docker_available() and docker_image_present(cfg.image)):
        pytest.skip("needs docker daemon + kubona-quantum-runner image")
    golden = run_codeexec_task(GOLDEN)
    assert golden["ok"] is True and golden["skip"] is False
    evil = run_codeexec_task(
        {"name": "live-evil-socket",
         "solution": "import socket\nsocket.create_connection((\"8.8.8.8\", 53), timeout=5)\n",
         "tests": "import solution\n"})
    assert evil["ok"] is False and evil["skip"] is False
    assert "Network is unreachable" in (evil.get("stderr_tail") or "")


def test_static_gate_refuses_unparseable():
    rec = run_codeexec_task(
        {"name": "broken", "solution": "def f(:\n", "tests": "x\n"},
        runner=_runner(0, "ok\n"))
    assert rec["ok"] is False and rec["skip"] is False
    assert "static gate refused" in rec["reason"]


def test_static_gate_closed_world_imports():
    base = {"name": "w", "solution": "import numpy\n",
            "tests": "import solution\n",
            "requires": ["numpy"]}
    rec = run_codeexec_task(dict(base), runner=_runner(0, "ok\n"))
    assert rec["ok"] is True, rec
    rec = run_codeexec_task(
        dict(base, tests="import solution\nimport seaborn\n"),
        runner=_runner(0, "ok\n"))
    assert rec["ok"] is False and "seaborn" in rec["reason"]
    # Open-world tasks (no requires) skip the import half.
    rec = run_codeexec_task(
        {"name": "w", "solution": "import anything\n", "tests": "x\n"},
        runner=_runner(0, "ok\n"))
    assert rec["ok"] is True


def test_dep_preflight_names_missing():
    from types import SimpleNamespace

    def probe_runner(cmd, **kwargs):
        if cmd[-1] == "_probe.py":
            return SimpleNamespace(stdout='["seaborn"]\n', stderr="", returncode=0)
        return SimpleNamespace(stdout="ok\n", stderr="", returncode=0)

    rec = run_codeexec_task(
        {"name": "w", "solution": "x = 1\n", "tests": "import solution\n",
         "requires": ["seaborn"]}, runner=probe_runner)
    assert rec["ok"] is False and rec["skip"] is True
    assert rec["missing_dependencies"] == ["seaborn"]
    assert "seaborn" in rec["reason"]


def test_dep_preflight_proceeds_when_present():
    from types import SimpleNamespace

    def probe_runner(cmd, **kwargs):
        if cmd[-1] == "_probe.py":
            return SimpleNamespace(stdout='[]\n', stderr="", returncode=0)
        return SimpleNamespace(stdout="ok\n", stderr="", returncode=0)

    rec = run_codeexec_task(
        {"name": "w", "solution": "x = 1\n", "tests": "import solution\n",
         "requires": ["numpy"]}, runner=probe_runner)
    assert rec["ok"] is True and rec["skip"] is False
