"""Invoke the Rust `qir-core` reference compiler on the lower_circuit path.

Production posture: the QIR/MLIR lowering (the plan's "highest-leverage single
hire", §8) runs on the Rust core, not the Python reference. `lower_with_rust`
locates the compiled binary (env KUBONA_QIR_CORE, else crates/qir-core target)
and emits the same QIR-ish result shape as the Python reference.

Failure contract: None is returned ONLY when no binary is present (caller
degrades to the Python reference, labeled `engine: "python-reference"`).
A PRESENT binary that fails (nonzero exit, timeout, empty output) RAISES with
stderr attached — the old code returned None there too, so a broken Rust core
silently masqueraded as "binary absent" and nobody ever noticed.
"""

from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Any


def _default_crate_target() -> Path:
    return Path(__file__).resolve().parent.parent.parent / "crates" / "qir-core" / "target"


def _allowed_binary_dirs() -> list[Path]:
    """Binary allowlist: the repo's own qir-core build outputs, plus operator-
    configured dirs via KUBONA_QIR_ALLOWED_DIRS (colon-separated). Anything
    else in KUBONA_QIR_CORE refuses — an env pointer at an arbitrary executable
    is a code-execution primitive and must not be honored silently."""
    dirs = [_default_crate_target() / "release", _default_crate_target() / "debug"]
    extra = os.environ.get("KUBONA_QIR_ALLOWED_DIRS", "")
    for chunk in extra.split(":"):
        chunk = chunk.strip()
        if chunk:
            dirs.append(Path(chunk).expanduser())
    return [d.resolve() for d in dirs]


def _candidate_paths() -> list[Path]:
    env = os.environ.get("KUBONA_QIR_CORE")
    if env:
        cand = Path(env).expanduser()
        # Resolve symlinks, then containment-check against the allowlist.
        # Unresolvable / outside-allowlist paths REFUSE (never silently fall
        # back: a misconfigured production compiler path must be loud).
        try:
            resolved = cand.resolve()
        except OSError as e:
            raise ValueError(f"KUBONA_QIR_CORE={env!r} does not resolve: {e}")
        if not resolved.is_file() or not os.access(resolved, os.X_OK):
            raise ValueError(
                f"KUBONA_QIR_CORE={env!r} is not an executable file: refusing"
            )
        allowed = _allowed_binary_dirs()
        if not any(resolved == d or d in resolved.parents for d in allowed):
            raise ValueError(
                f"KUBONA_QIR_CORE={env!r} resolves outside the binary allowlist "
                f"({[str(d) for d in allowed]}): refusing (set "
                "KUBONA_QIR_ALLOWED_DIRS to extend it explicitly)"
            )
        return [resolved]
    crate = _default_crate_target()
    return [crate / "release" / "qir-core", crate / "debug" / "qir-core"]


def find_binary() -> Path | None:
    for cand in _candidate_paths():
        if cand.exists() and os.access(cand, os.X_OK):
            return cand
    return None


def rust_available() -> bool:
    return find_binary() is not None


def lower_with_rust(qasm: str, bin_path: Path | None = None) -> dict[str, Any] | None:
    """Run the Rust core; returns the QIR text + parsed metadata.

    Returns None ONLY when no binary is present. A present-but-failing binary
    raises RuntimeError with stderr (fail-closed: a broken compiler is never
    mistaken for an absent one). Empty QIR output raises (a compiler that
    prints nothing proved nothing).
    """
    binary = bin_path or find_binary()
    if binary is None:
        return None
    if not qasm or not qasm.strip():
        raise ValueError("lower_with_rust refused: empty QASM input")
    try:
        proc = subprocess.run([str(binary)], input=qasm.encode(), capture_output=True,
                              timeout=15, check=False)
    except subprocess.TimeoutExpired as e:
        raise RuntimeError(
            f"qir-core timed out after 15s on {len(qasm)} chars of QASM: refusing "
            "(a hung compiler is not an absent compiler)"
        ) from e
    except OSError as e:
        raise RuntimeError(f"qir-core failed to execute {binary}: {e}") from e
    if proc.returncode != 0:
        stderr = proc.stderr.decode(errors="replace").strip()
        raise RuntimeError(
            f"qir-core exited {proc.returncode}: {stderr or '(empty stderr)'}"
        )
    qir_text = proc.stdout.decode()
    if not qir_text.strip():
        raise RuntimeError("qir-core returned empty QIR output: refusing")
    # derive circuit metadata from the emitted text (consistent with Python reference)
    num_qubits = 0
    native_gate_count: dict[str, int] = {}
    for line in qir_text.splitlines():
        if "call %Qubit* @__quantum__rt__qubit_allocate" in line:
            num_qubits += 1
        qm = __import__("re").search(r"call void @__quantum__qis__(\w+)__body", line)
        if qm:
            g = qm.group(1)
            if g == "cnot":
                g = "cx"
            native_gate_count[g] = native_gate_count.get(g, 0) + 1
    return {
        "ok": True,
        "tier": "rust-qir-core",
        "circuit_name": "rust-lowered",
        "num_qubits": num_qubits,
        "native_gate_set": list(native_gate_count.keys()) or ["rx", "ry", "rz", "cx", "x", "h"],
        "passes": ["parse_qasm", "transpile_to_native", "emit_qir (Rust qir-core)"],
        "qir_text": qir_text,
        "native_gate_count": native_gate_count,
        "engine": "rust-qir-core",
    }
