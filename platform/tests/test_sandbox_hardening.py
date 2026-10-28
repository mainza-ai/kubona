"""Phase-1 sandbox hardening pins (image parity, flag parity, escape re-tests).

Hermetic tests run everywhere; live-container tests skip cleanly without a
daemon (same style as the existing skips) and run for real in CI's
sandbox-container job (KUBONA_SANDBOX_CONTAINER=1 + built runner image).
"""

import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

import pytest

from kubona.sandbox import SecurityPolicyViolation, WorkloadSandbox
from kubona.sandbox.execution import (
    SANDBOX_CPUS,
    SANDBOX_IMAGE_DEFAULT,
    SANDBOX_PIDS_LIMIT,
    SANDBOX_TMPFS,
    SANDBOX_USER,
    build_container_command,
)

GO_DOCKER_RAIL = (
    Path(__file__).resolve().parent.parent / "core" / "pkg" / "sandbox" / "docker.go"
)


def _go_literals(pattern: str) -> list[str]:
    src = GO_DOCKER_RAIL.read_text()
    return re.findall(pattern, src)


# ---------------------------------------------------------------------------
# Single source of truth: the Python literal must match the Go rail.
# ---------------------------------------------------------------------------

def test_sandbox_image_matches_go_rail():
    """Cross-language pin: the Go rail's `DefaultRunnerImage` const in
    docker.go must equal SANDBOX_IMAGE_DEFAULT, no stale `Image:` string
    literal may remain, and the retired `kubona/quantum-runner:latest` pin
    must be gone (bump both rails together, never one alone)."""
    src = GO_DOCKER_RAIL.read_text()
    m = re.search(r'DefaultRunnerImage\s*=\s*"([^"]+)"', src)
    assert m, "Go DefaultRunnerImage const not found in docker.go"
    assert SANDBOX_IMAGE_DEFAULT == m.group(1), (
        f"Python {SANDBOX_IMAGE_DEFAULT!r} != Go DefaultRunnerImage {m.group(1)!r} "
        "(see the pin comment on SANDBOX_IMAGE_DEFAULT)")
    assert not re.findall(r'Image:\s*"[^"]+"', src), \
        "docker.go still carries a raw Image: literal beside the const"
    assert "kubona/quantum-runner:latest" not in src, \
        "stale kubona/quantum-runner:latest pin still in docker.go"


def test_sandbox_user_matches_go_rail():
    """--user parity: every User literal in docker.go must equal SANDBOX_USER."""
    go_users = _go_literals(r'User:\s*"([^"]+)"')
    assert go_users, "no User literal found in docker.go"
    assert len(set(go_users)) == 1, f"Go rail drifts from itself: {set(go_users)}"
    assert SANDBOX_USER == go_users[0] == "1000:1000"


# ---------------------------------------------------------------------------
# Container flag parity (hermetic: asserts the exact invocation, no daemon).
# ---------------------------------------------------------------------------

def test_container_command_reaches_go_parity():
    cmd = build_container_command(image=SANDBOX_IMAGE_DEFAULT,
                                  workdir="/tmp/w", max_memory_mb=512)
    assert cmd[:3] == ["docker", "run", "--rm"]
    joined = " ".join(cmd)
    for flag in ("--read-only",
                 "--tmpfs " + SANDBOX_TMPFS,
                 "--cap-drop=ALL",
                 "--security-opt=no-new-privileges",
                 "--network none",
                 "--memory 512m",
                 "--memory-swap 512m",
                 "--cpus " + SANDBOX_CPUS,
                 f"--pids-limit={SANDBOX_PIDS_LIMIT}",
                 "--user " + SANDBOX_USER,
                 "--entrypoint python3"):
        assert flag in joined, flag
    assert "/tmp/w:/work:ro" in cmd
    assert "-w" in cmd and "/work" in cmd
    assert "-e" in cmd and "PYTHONUNBUFFERED=1" in cmd
    assert "seccomp=unconfined" not in joined  # default profile stays on
    assert cmd[-2:] == [SANDBOX_IMAGE_DEFAULT, "workload.py"]


def test_container_path_uses_pinned_image_by_default(monkeypatch):
    """Hermetic (docker faked): without KUBONA_SANDBOX_IMAGE the invocation
    carries the pinned default — the image name cannot drift silently."""
    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.delenv("KUBONA_SANDBOX_IMAGE", raising=False)
    monkeypatch.setattr(shutil, "which", lambda _: "/usr/bin/docker")
    seen: dict = {}

    class _FakeProc:
        returncode = 0
        stdout = "PIN_OK\n"
        stderr = ""

    import kubona.sandbox.execution as ex

    def _fake_run(cmd, **kw):
        seen["cmd"] = cmd
        return _FakeProc()

    monkeypatch.setattr(ex.subprocess, "run", _fake_run)
    res = WorkloadSandbox().execute_script("import math\nprint('PIN_OK')")
    assert res.success and "PIN_OK" in res.stdout
    assert SANDBOX_IMAGE_DEFAULT in seen["cmd"]


def test_container_path_honors_image_override(monkeypatch):
    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.setenv("KUBONA_SANDBOX_IMAGE", "example/custom-runner:9.9.9")
    monkeypatch.setattr(shutil, "which", lambda _: "/usr/bin/docker")
    seen: dict = {}

    class _FakeProc:
        returncode = 0
        stdout = "ok\n"
        stderr = ""

    import kubona.sandbox.execution as ex

    def _fake_run(cmd, **kw):
        seen["cmd"] = cmd
        return _FakeProc()

    monkeypatch.setattr(ex.subprocess, "run", _fake_run)
    res = WorkloadSandbox().execute_script("print('ok')")
    assert res.success
    assert "example/custom-runner:9.9.9" in seen["cmd"]


# ---------------------------------------------------------------------------
# Typed refusals: no silent fallback, in dev or production.
# ---------------------------------------------------------------------------

def test_dev_explicit_disable_refuses_with_typed_violation(monkeypatch):
    """KUBONA_SANDBOX_UNSAFE_LOCAL=0 opts dev out of the local runner: refuse
    with a violation naming the missing precondition (container or ack)."""
    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_UNSAFE_LOCAL", "0")
    monkeypatch.delenv("KUBONA_SANDBOX_CONTAINER", raising=False)
    res = WorkloadSandbox().execute_script("print('should not run')")
    assert res.success is False
    assert res.violation == "unsafe_local_disabled"
    assert "KUBONA_SANDBOX_CONTAINER" in res.stderr
    assert "KUBONA_SANDBOX_UNSAFE_LOCAL" in res.stderr


def test_container_requested_but_docker_missing_refuses(monkeypatch):
    """Container rail selected but no daemon: isolation_unavailable — never a
    silent downgrade to the local runner."""
    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.setattr(shutil, "which", lambda _: None)
    res = WorkloadSandbox().execute_script("import math\nprint(math.pi)")
    assert res.success is False
    assert res.violation == "isolation_unavailable"
    assert "docker" in res.stderr


def test_dev_local_runs_with_explicit_ack(monkeypatch):
    """Positive control: dev + KUBONA_SANDBOX_UNSAFE_LOCAL=1 runs hermetic."""
    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_UNSAFE_LOCAL", "1")
    monkeypatch.delenv("KUBONA_SANDBOX_CONTAINER", raising=False)
    res = WorkloadSandbox().execute_script("import math\nprint(f'ACK={math.sqrt(4.0)}')")
    assert res.success, res.stderr
    assert "ACK=2.0" in res.stdout


# ---------------------------------------------------------------------------
# ESCAPE RE-TESTS (exit criterion): every previously-documented escape plus
# newly-invented ones must fail — as tests, not prose. Hermetic half asserts
# the validator refuses; the live half (below) asserts the hardened path
# refuses end-to-end.
# ---------------------------------------------------------------------------

DOCUMENTED_ESCAPES = [
    # scipy.io host-file read (validator gap: root `scipy` is allowlisted).
    "import scipy.io\nscipy.io.loadmat('/etc/hostname')",
    "import scipy.io\nscipy.io.mmread('/etc/hosts')",
    "import scipy.io\nscipy.io.mmwrite('/tmp/pwned.mtx', [[1]])",
    "from scipy.io import loadmat\nloadmat('/etc/passwd')",
    "from scipy import io\nio.loadmat('/etc/passwd')",
    # numpy.f2py codegen (validator gap: `f2py`/`run_main` were unblocked).
    "import numpy\nnumpy.f2py.run_main(['-c', 'x'])",
    "from numpy import f2py\nf2py.run_main([])",
    "import numpy\nnumpy.f2py.crackfortran('subroutine x\nend')",
    "import numpy\nx = numpy.f2py",
    # tb_frame attribute reach (validator gap: tb_frame/f_back unblocked,
    # and `__traceback__` sailed through the old listed-names-only check).
    "try:\n x = 1/0\nexcept Exception as e:\n f = e.__traceback__.tb_frame",
    "try:\n x = 1/0\nexcept Exception as e:\n f = e.__traceback__.tb_next",
    "f = some_frame.f_back",
    # Bound-method / loader reach via unlisted dunders.
    "f = obj.__self__",
    "import numpy\nnumpy.__loader__.get_source('x')",
    "x = obj.__wrapped__",
]

# New escapes invented for this hardening round (each verified to fail).
NEW_ESCAPES = [
    # Process / thread / async spawn rails.
    "import multiprocessing\nmultiprocessing.Process(target=f).start()",
    "import threading\nthreading.Thread(target=f).start()",
    "import asyncio\nasyncio.run(main())",
    # Host network reach.
    "import socket\nsocket.create_connection(('8.8.8.8', 53), timeout=5)",
    # Host file read via pathlib.
    "from pathlib import Path\nPath('/etc/hostname').read_text()",
    # Remaining numpy text/file verbs (same class as the blocked loadtxt).
    "import numpy\nnumpy.genfromtxt('/etc/hostname')",
    "import numpy\nnumpy.fromregex('/etc/hosts', r'(.*)', 'S16')",
    # Generator/coroutine frame reach (same class as gi_frame).
    "g = (x for x in range(3))\nf = g.ag_frame",
    "g = (x for x in range(3))\nf = g.gi_frame",
]


@pytest.mark.parametrize("code", DOCUMENTED_ESCAPES, ids=[f"doc-{i}" for i in range(len(DOCUMENTED_ESCAPES))])
def test_documented_escapes_refused_by_validator(code):
    with pytest.raises(SecurityPolicyViolation):
        WorkloadSandbox().validate_ast(code)


@pytest.mark.parametrize("code", NEW_ESCAPES, ids=[f"new-{i}" for i in range(len(NEW_ESCAPES))])
def test_new_escapes_refused_by_validator(code):
    with pytest.raises(SecurityPolicyViolation):
        WorkloadSandbox().validate_ast(code)


def test_benign_dunder_reads_still_allowed():
    """The fail-closed dunder rule must not break harmless introspection."""
    sb = WorkloadSandbox()
    sb.validate_ast("import numpy\nprint(numpy.__version__)")
    sb.validate_ast("import math\nprint(math.__doc__)")
    sb.validate_ast("import math\nprint(math.__name__)")
    sb.validate_ast("if __name__ == '__main__':\n print('main')")


# ---------------------------------------------------------------------------
# Live container half (skip cleanly without a daemon; real in CI).
# ---------------------------------------------------------------------------

def _live_image(monkeypatch) -> str:
    """Skip unless the container rail is configured AND the image is present."""
    if os.getenv("KUBONA_SANDBOX_CONTAINER") != "1" or shutil.which("docker") is None:
        pytest.skip("container sandbox not configured (CI builds the runner image)")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    image = os.getenv("KUBONA_SANDBOX_IMAGE", SANDBOX_IMAGE_DEFAULT)
    monkeypatch.setenv("KUBONA_SANDBOX_IMAGE", image)
    probe = subprocess.run(["docker", "image", "inspect", image],
                           capture_output=True, text=True, timeout=60)
    if probe.returncode != 0:
        pytest.skip(f"pinned image {image} absent: refusing implicit pull")
    return image


def test_live_science_stack_works_under_hardening(monkeypatch):
    """Golden: numpy/scipy import and run under the exact hardened flags
    (proves the default seccomp profile + read-only rootfs don't break
    legit workloads — the custom-profile follow-up must preserve this)."""
    _live_image(monkeypatch)
    res = WorkloadSandbox().execute_script(
        "import math, numpy, scipy.linalg\n"
        "print(f'GOLDEN={math.sqrt(16.0)},{numpy.pi:.5f},{scipy.linalg.norm([3.0, 4.0])}')")
    assert res.success, res.stderr
    assert "GOLDEN=4.0,3.14159,5.0" in res.stdout


@pytest.mark.parametrize("code", DOCUMENTED_ESCAPES + NEW_ESCAPES)
def test_live_escapes_fail_on_hardened_path(monkeypatch, code):
    """Each escape, executed against the hardened path, must not succeed."""
    _live_image(monkeypatch)
    res = WorkloadSandbox().execute_script(code, timeout_sec=60.0)
    assert res.success is False, f"escape succeeded: {code!r}\n{res.stdout}"


def test_live_infinite_loop_is_contained(monkeypatch):
    """Validator-clean program (`while True`) must die by timeout, not hang."""
    _live_image(monkeypatch)
    res = WorkloadSandbox().execute_script("while True:\n pass", timeout_sec=5.0)
    assert res.success is False
    assert res.violation == "timeout"


def test_live_host_filesystem_is_not_mounted(monkeypatch):
    """True mount-isolation proof (bypasses the AST gate on purpose: this
    test targets the container layer — execute_script would refuse open())."""
    image = _live_image(monkeypatch)
    host_hostname = (Path("/etc/hostname").read_text().strip()
                     if Path("/etc/hostname").exists() else None)
    with tempfile.TemporaryDirectory(prefix="kubona-sandbox-test-") as tmpdir:
        # Same chmod discipline as execute_script: the container runs as UID
        # 1000, so a 0700 tmpdir bind-mounts unreadable on Linux CI.
        os.chmod(tmpdir, 0o755)
        cmd = build_container_command(image=image, workdir=tmpdir,
                                      max_memory_mb=256)

        def _run_script(body: str) -> subprocess.CompletedProcess:
            Path(tmpdir, "workload.py").write_text(body)
            os.chmod(Path(tmpdir, "workload.py"), 0o644)
            return subprocess.run(cmd, capture_output=True, text=True, timeout=60)

        # 1. The container sees its OWN hostname namespace, never the host's
        # (only /work is bind-mounted; host files are not reachable).
        proc = _run_script("print(open('/etc/hostname').read())\n")
        assert proc.returncode == 0, proc.stderr
        if host_hostname:
            assert proc.stdout.strip() != host_hostname, \
                "container read the HOST hostname: mount isolation broken"
        # 2. The mounted workdir itself is readable (bind works) ...
        Path(tmpdir, "canary-inside.txt").write_text("WORKDIR-CANARY")
        os.chmod(Path(tmpdir, "canary-inside.txt"), 0o644)
        proc = _run_script("print(open('/work/canary-inside.txt').read())\n")
        assert proc.returncode == 0, proc.stderr
        assert "WORKDIR-CANARY" in proc.stdout
        # ... but NOT writable (read-only bind + read-only rootfs).
        proc = _run_script("open('/work/should-fail.txt', 'w').write('x')\n")
        assert proc.returncode != 0, proc.stdout
        assert "Read-only file system" in proc.stderr
