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
    SANDBOX_MANAGED_LABEL,
    SANDBOX_MANAGED_VALUE,
    SANDBOX_PIDS_LIMIT,
    SANDBOX_REAP_AFTER_ENV,
    SANDBOX_TMPFS,
    SANDBOX_USER,
    build_container_command,
    reap_stale_sandboxes,
    stop_sandbox_container,
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
    """Default-on: with no seccomp argument the invocation carries the
    shipped profile (and never unconfined / never profile-less)."""
    from kubona.sandbox.execution import _default_seccomp_profile
    cmd = build_container_command(image=SANDBOX_IMAGE_DEFAULT,
                                  workdir="/tmp/w", max_memory_mb=512,
                                  run_id="aaabbbccdd")
    assert cmd[:3] == ["docker", "run", "--rm"]
    assert "--name" in cmd and "kubona-sandbox-aaabbbcc" in cmd
    assert "kubona.managed=sandbox" in " ".join(cmd)
    assert "kubona.run-id=aaabbbccdd" in " ".join(cmd)
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
    assert "seccomp=unconfined" not in joined  # never unconfined
    default = _default_seccomp_profile()
    assert default is not None, "shipped profile must resolve in a checkout"
    assert f"--security-opt seccomp={default}" in joined
    assert joined.count("seccomp=") == 1  # exactly one profile flag
    assert cmd[-2:] == [SANDBOX_IMAGE_DEFAULT, "workload.py"]


def test_container_command_applies_seccomp_profile_when_set():
    """Operator custom profile: an explicit path appends exactly one
    --security-opt seccomp=<path> pair (and never seccomp=unconfined)."""
    from kubona.sandbox.execution import SANDBOX_SECCOMP_PROFILE_ENV  # noqa: F401
    cmd = build_container_command(image=SANDBOX_IMAGE_DEFAULT,
                                  workdir="/tmp/w", max_memory_mb=512,
                                  run_id="1122334455",
                                  seccomp_profile="/profiles/kubona.json")
    joined = " ".join(cmd)
    assert "--security-opt seccomp=/profiles/kubona.json" in joined
    assert joined.count("seccomp=") == 1
    assert "seccomp=unconfined" not in joined
    assert cmd[-2:] == [SANDBOX_IMAGE_DEFAULT, "workload.py"]


def test_container_command_empty_profile_means_no_flag_test_only():
    """Differential hook: seccomp_profile="" emits no flag (Docker default).
    Test-only — execute_script never passes "" (it has no profile-less
    path); this exists so the suite can prove the backstop matters."""
    cmd = build_container_command(image=SANDBOX_IMAGE_DEFAULT,
                                  workdir="/tmp/w", max_memory_mb=512,
                                  run_id="6677889900",
                                  seccomp_profile="")
    assert "seccomp=" not in " ".join(cmd)


def test_container_command_missing_shipped_profile_fails_closed(monkeypatch):
    """No silent fallback: when the shipped file is absent the builder
    raises instead of emitting a profile-less invocation."""
    import kubona.sandbox.execution as ex
    monkeypatch.setattr(ex, "_default_seccomp_profile", lambda: None)
    with pytest.raises(FileNotFoundError, match="refusing"):
        build_container_command(image=SANDBOX_IMAGE_DEFAULT,
                                workdir="/tmp/w", max_memory_mb=512,
                                run_id="abcdef0123")


def test_go_rail_carries_seccomp_default_on():
    """Cross-rail pin: docker.go must embed the profile, apply it
    unconditionally, and reject unconfined (parity with this rail)."""
    src = GO_DOCKER_RAIL.read_text()
    assert "go:embed seccomp-profile.json" in src
    assert '"--security-opt", "seccomp=" + profilePath' in src
    # The "unconfined" literal appears in CODE only in the rejection guard
    # (the equality check and its error) — the rail builds seccomp=<path>
    # unconditionally and never emits unconfined as a value. Comment prose
    # is stripped before checking.
    code = "\n".join(
        line.split("//")[0] for line in src.splitlines())
    hits = [i for i, line in enumerate(code.splitlines()) if '"unconfined"' in line]
    assert hits, "expected the unconfined rejection guard in docker.go"
    for i in hits:
        line = code.splitlines()[i]
        assert 'SeccompProfile == "unconfined"' in line or "prohibited" in line, \
            f"unconfined literal outside the rejection guard (line {i + 1}): {line}"


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


@pytest.mark.parametrize("ack", [None, "", "0", "yes", "TRUE"])
def test_dev_local_without_exact_ack_refuses(monkeypatch, ack):
    """Phase-2 deny-by-default: the hermetic local runner requires EXACTLY
    KUBONA_SANDBOX_UNSAFE_LOCAL=1 — unset, "", "0", or any other spelling
    refuses with unsafe_local_disabled (the historic unset-permits default
    is gone; production behavior unchanged, still isolation_required)."""
    monkeypatch.setenv("KUBONA_ENV", "test")
    if ack is None:
        monkeypatch.delenv("KUBONA_SANDBOX_UNSAFE_LOCAL", raising=False)
    else:
        monkeypatch.setenv("KUBONA_SANDBOX_UNSAFE_LOCAL", ack)
    monkeypatch.delenv("KUBONA_SANDBOX_CONTAINER", raising=False)
    res = WorkloadSandbox().execute_script("print('should not run')")
    assert res.success is False
    assert res.violation == "unsafe_local_disabled"
    assert "KUBONA_SANDBOX_UNSAFE_LOCAL" in res.stderr


def test_container_path_applies_seccomp_profile_by_default(monkeypatch):
    """Hermetic (docker faked): default-on — unset env still carries the
    shipped profile; KUBONA_SANDBOX_SECCOMP_PROFILE=<path> overrides it
    (custom profile, never a disable)."""
    import kubona.sandbox.execution as ex

    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.delenv("KUBONA_SANDBOX_SECCOMP_PROFILE", raising=False)
    monkeypatch.setattr(shutil, "which", lambda _: "/usr/bin/docker")
    seen: dict = {}

    class _FakeProc:
        returncode = 0
        stdout = "ok\n"
        stderr = ""

    def _fake_run(cmd, **kw):
        seen["cmd"] = cmd
        return _FakeProc()

    monkeypatch.setattr(ex.subprocess, "run", _fake_run)
    res = WorkloadSandbox().execute_script("print('ok')")
    assert res.success
    default = ex._default_seccomp_profile()
    assert default is not None
    assert f"--security-opt seccomp={default}" in " ".join(seen["cmd"])
    monkeypatch.setenv("KUBONA_SANDBOX_SECCOMP_PROFILE", "/profiles/kubona.json")
    res = WorkloadSandbox().execute_script("print('ok')")
    assert res.success
    assert "--security-opt seccomp=/profiles/kubona.json" in " ".join(seen["cmd"])


def test_container_path_missing_profile_refuses(monkeypatch):
    """Fail closed (hermetic, docker faked): no shipped file and no
    override refuses with isolation_unavailable — never profile-less."""
    import kubona.sandbox.execution as ex

    monkeypatch.setenv("KUBONA_ENV", "test")
    monkeypatch.setenv("KUBONA_SANDBOX_CONTAINER", "1")
    monkeypatch.delenv("KUBONA_SANDBOX_SECCOMP_PROFILE", raising=False)
    monkeypatch.setattr(shutil, "which", lambda _: "/usr/bin/docker")
    monkeypatch.setattr(ex, "_default_seccomp_profile", lambda: None)
    seen: dict = {}

    def _fake_run(cmd, **kw):
        seen["cmd"] = cmd
        raise AssertionError("docker must not run without a profile")

    monkeypatch.setattr(ex.subprocess, "run", _fake_run)
    res = WorkloadSandbox().execute_script("print('ok')")
    assert res.success is False
    assert res.violation == "isolation_unavailable"
    assert "without a profile" in res.stderr
    assert "cmd" not in seen


def test_go_embedded_profile_matches_canonical():
    """Parity: the Go rail's duplicated seccomp-profile.json must stay
    semantically identical to the canonical platform/bridge copy (parsed
    JSON: defaultAction, arch list, syscall allow sets — formatting drift
    is fine, semantic drift fails)."""
    import json

    canonical = Path(__file__).resolve().parent.parent / "bridge" / "seccomp-profile.json"
    duplicate = GO_DOCKER_RAIL.parent / "seccomp-profile.json"
    assert duplicate.is_file(), "Go embed source missing — re-copy the canonical profile"
    can = json.loads(canonical.read_text())
    dup = json.loads(duplicate.read_text())
    assert dup["defaultAction"] == can["defaultAction"]
    assert dup.get("defaultErrnoRet") == can.get("defaultErrnoRet")
    assert {a["architecture"] for a in dup["archMap"]} == {
        a["architecture"] for a in can["archMap"]}

    def _fingerprint(entries):
        out = {}
        for entry in entries:
            names = tuple(sorted(entry["names"]))
            rest = {k: v for k, v in entry.items() if k != "names"}
            key = (names, json.dumps(rest, sort_keys=True))
            out[key] = out.get(key, 0) + 1
        return out

    assert _fingerprint(dup["syscalls"]) == _fingerprint(can["syscalls"])


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
                                      max_memory_mb=256, run_id="livehelper01")

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


# ---------------------------------------------------------------------------
# Shipped seccomp profile, default-on (Phase-2): the profile is verified
# green end-to-end (golden workload + syscall backstop) through the NEW
# default path — no KUBONA_SANDBOX_SECCOMP_PROFILE flag — never prose. Live
# half runs for real in CI's sandbox-container job; skips cleanly without
# a daemon.
# ---------------------------------------------------------------------------

def _live_default_image(monkeypatch) -> str:
    """Pinned image via the default path: refuse when the shipped profile
    is absent (fail closed — no profile-less live runs)."""
    from kubona.sandbox.execution import _default_seccomp_profile
    image = _live_image(monkeypatch)
    monkeypatch.delenv("KUBONA_SANDBOX_SECCOMP_PROFILE", raising=False)
    if _default_seccomp_profile() is None:
        pytest.skip("shipped seccomp profile absent: refusing to test prose")
    return image


def test_live_default_profile_golden_through_wiring(monkeypatch):
    """End-to-end default-on: the numpy/scipy golden runs through
    execute_script with NO env flag (proves the default wiring AND the
    profile agree with the workload)."""
    _live_default_image(monkeypatch)
    res = WorkloadSandbox().execute_script(
        "import math, numpy, scipy.linalg\n"
        "print(f'GOLDEN={math.sqrt(16.0)},{numpy.pi:.5f},{scipy.linalg.norm([3.0, 4.0])}')")
    assert res.success, res.stderr
    assert "GOLDEN=4.0,3.14159,5.0" in res.stdout


def test_live_profile_denies_inet_socket_at_syscall(monkeypatch):
    """Syscall-layer backstop on the default path, differential vs Docker
    default: under the shipped profile the container is still
    seccomp-confined (Seccomp: 2), AF_INET socket() dies with EPERM at the
    syscall (Docker default CREATES the socket — only the netns stops
    connect there), AF_UNIX still works."""
    image = _live_default_image(monkeypatch)
    with tempfile.TemporaryDirectory(prefix="kubona-sandbox-test-") as tmpdir:
        os.chmod(tmpdir, 0o755)
        # Default path: no seccomp argument (resolves the shipped profile).
        cmd = build_container_command(image=image, workdir=tmpdir,
                                      max_memory_mb=256, run_id="livediff02")
        assert "seccomp=" in " ".join(cmd)  # the differential below is real
        # Test-only differential: explicit "" recovers Docker's default.
        nocmd = build_container_command(image=image, workdir=tmpdir,
                                        max_memory_mb=256, run_id="livediff03",
                                        seccomp_profile="")

        def _run_script(cmd: list, body: str) -> subprocess.CompletedProcess:
            Path(tmpdir, "workload.py").write_text(body)
            os.chmod(Path(tmpdir, "workload.py"), 0o644)
            return subprocess.run(cmd, capture_output=True, text=True, timeout=120)

        proc = _run_script(
            cmd, "print('\\n'.join(l for l in "
            "open('/proc/self/status').read().splitlines() if 'eccomp' in l))")
        assert proc.returncode == 0, proc.stderr
        assert "Seccomp:\t2" in proc.stdout
        assert "Seccomp_filters:\t1" in proc.stdout
        proc = _run_script(cmd, "import socket\n"
                           "socket.socket(socket.AF_INET, socket.SOCK_STREAM)\n"
                           "print('INET_CREATED')")
        assert proc.returncode != 0
        assert "Operation not permitted" in proc.stderr
        assert "INET_CREATED" not in proc.stdout
        # Differential: Docker's default profile CREATES the socket (only
        # the netns stops connect) — proves the shipped profile is the
        # layer doing the refusing.
        proc = _run_script(nocmd, "import socket\n"
                           "socket.socket(socket.AF_INET, socket.SOCK_STREAM)\n"
                           "print('INET_CREATED')")
        assert proc.returncode == 0, proc.stderr
        assert "INET_CREATED" in proc.stdout
        proc = _run_script(cmd, "import socket\n"
                           "s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)\n"
                           "s.close()\nprint('UNIX_OK')")
        assert proc.returncode == 0, proc.stderr
        assert "UNIX_OK" in proc.stdout


def test_live_custom_profile_override_still_applies(monkeypatch):
    """Operator override: an explicit KUBONA_SANDBOX_SECCOMP_PROFILE still
    selects that file (custom profile, never a disable)."""
    _live_default_image(monkeypatch)
    from kubona.sandbox.execution import _default_seccomp_profile
    monkeypatch.setenv("KUBONA_SANDBOX_SECCOMP_PROFILE",
                       _default_seccomp_profile())
    res = WorkloadSandbox().execute_script("import math\nprint(f'OVERRIDE={math.sqrt(16.0)}')")
    assert res.success, res.stderr
    assert "OVERRIDE=4.0" in res.stdout


@pytest.mark.parametrize("code", [
    # AST-blocked: refuses before the container even starts (integration
    # proof the profile wiring never bypasses the gate).
    "import socket\nsocket.create_connection(('8.8.8.8', 53), timeout=5)",
    "import scipy.io\nscipy.io.loadmat('/etc/hostname')",
    "import numpy\nnumpy.f2py.run_main(['-c', 'x'])",
])
def test_live_profile_escapes_fail_on_hardened_path(monkeypatch, code):
    _live_default_image(monkeypatch)
    res = WorkloadSandbox().execute_script(code, timeout_sec=60.0)
    assert res.success is False, f"escape succeeded under profile: {code!r}\n{res.stdout}"


# ---------------------------------------------------------------------------
# Orphan prevention (2026-10-29 incident: nine 100%-CPU workload.py
# containers from killed test clients — killing the `docker run` client
# never stops the container).
# ---------------------------------------------------------------------------

def _managed_names() -> set[str]:
    """Names of currently present label-scoped sandbox containers."""
    probe = subprocess.run(
        ["docker", "ps", "-a", "--filter",
         f"label={SANDBOX_MANAGED_LABEL}={SANDBOX_MANAGED_VALUE}",
         "--format", "{{.Names}}"],
        capture_output=True, text=True, timeout=60)
    assert probe.returncode == 0, probe.stderr
    return {n.strip() for n in probe.stdout.splitlines() if n.strip()}


def _wait_gone(names: set[str], timeout_s: float = 20.0) -> set[str]:
    """Poll until the named containers disappear (`--rm` removal lags the
    stop call — asserting immediately after `docker stop` is a race)."""
    import time as _time
    deadline = _time.monotonic() + timeout_s
    remaining = set(names)
    while remaining and _time.monotonic() < deadline:
        _time.sleep(1.0)
        remaining &= _managed_names()
    return remaining


def test_timeout_kills_the_container_not_just_the_client(monkeypatch):
    """A validator-clean infinite loop must leave zero containers behind:
    the timeout path stops the NAMED container (client death alone
    orphans it — the incident this test locks out)."""
    _live_image(monkeypatch)
    before = _managed_names()
    res = WorkloadSandbox().execute_script("while True:\n pass", timeout_sec=8.0)
    assert res.success is False
    assert res.violation == "timeout"
    leaked = _wait_gone(_managed_names() - before)
    assert not leaked, f"orphaned sandbox containers: {leaked}"


def test_reaper_sweeps_label_scoped_orphans_only(monkeypatch):
    """The reaper stops stale label-scoped containers and touches nothing
    else (labels are the blast-radius boundary)."""
    _live_image(monkeypatch)
    image = os.getenv("KUBONA_SANDBOX_IMAGE", SANDBOX_IMAGE_DEFAULT)
    planted = f"kubona-sandbox-reaptest{os.getpid() % 100000:05d}"
    start = subprocess.run(
        ["docker", "run", "-d", "--rm",
         "--label", f"{SANDBOX_MANAGED_LABEL}={SANDBOX_MANAGED_VALUE}",
         "--label", "kubona.run-id=reaptest",
         "--name", planted, "--entrypoint", "sleep",
         image, "300"],
        capture_output=True, text=True, timeout=120)
    assert start.returncode == 0, start.stderr
    try:
        assert planted in _managed_names()
        monkeypatch.setenv(SANDBOX_REAP_AFTER_ENV, "0")
        reaped = reap_stale_sandboxes()
        assert planted in reaped, f"reaper missed the planted orphan: {reaped}"
        assert not _wait_gone({planted}), "reaped container still present"
    finally:
        stop_sandbox_container(planted)
