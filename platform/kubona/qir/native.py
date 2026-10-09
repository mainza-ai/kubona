"""Invoke the Rust `qir-core` reference compiler on the lower_circuit path.

Production posture: the QIR/MLIR lowering (the plan's "highest-leverage single
hire", §8) runs on the Rust core, not the Python reference. `lower_with_rust`
locates the compiled binary (env KUBONA_QIR_CORE, else crates/qir-core target)
and emits the same QIR-ish result shape as the Python reference; when the binary
is absent it returns None so the caller degrades gracefully to the Python
reference (marked `engine: "python-reference"`) — the CI rust job builds it, and
a PyO3 wheel is the packaged production path.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path
from typing import Any


def _candidate_paths() -> list[Path]:
    env = os.environ.get("KUBONA_QIR_CORE")
    if env:
        return [Path(env).expanduser()]
    crate = Path(__file__).resolve().parent.parent.parent / "crates" / "qir-core" / "target"
    return [crate / "release" / "qir-core", crate / "debug" / "qir-core"]


def find_binary() -> Path | None:
    for cand in _candidate_paths():
        if cand.exists() and os.access(cand, os.X_OK):
            return cand
    # last resort: use cargo if present (slow; production ships the built binary)
    if shutil.which("cargo"):
        crate = Path(__file__).resolve().parent.parent.parent / "crates" / "qir-core"
        if (crate / "Cargo.toml").exists():
            return None  # return None; caller falls back to Python (CI builds it)
    return None


def rust_available() -> bool:
    return find_binary() is not None


def lower_with_rust(qasm: str, bin_path: Path | None = None) -> dict[str, Any] | None:
    """Run the Rust core; returns the QIR text + parsed metadata, or None if unavailable."""
    binary = bin_path or find_binary()
    if binary is None:
        return None
    try:
        proc = subprocess.run([str(binary)], input=qasm.encode(), capture_output=True,
                              timeout=15, check=True)
        qir_text = proc.stdout.decode()
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError):
        return None
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
        "circuit_name": "untitled",
        "num_qubits": num_qubits,
        "native_gate_set": list(native_gate_count.keys()) or ["rx", "ry", "rz", "cx", "x", "h"],
        "passes": ["parse_qasm", "transpile_to_native", "emit_qir (Rust qir-core)"],
        "qir_text": qir_text,
        "native_gate_count": native_gate_count,
        "engine": "rust-qir-core",
    }
