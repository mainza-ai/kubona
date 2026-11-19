"""Simulator memory and method policy (single source).

Aer's ``max_memory_mb=0`` default consults *system* memory, not the cgroup, so
inside a 4 GiB bridge container on a large host it will happily try a 4 GiB
statevector and let the kernel kill the process. Every Aer construction in the
bridge goes through this module: the budget comes from the cgroup (or an
explicit env override), the method is chosen from circuit shape, and the
parallel thread count is bounded by the CPU quota.

This is admission policy, not a claim that every circuit fits: a caller that
exceeds the budget gets a typed refusal from Aer before allocation.
"""

from __future__ import annotations

import math
import os
from functools import lru_cache
from pathlib import Path
from typing import Any, Optional

#: Leave 20% of the container for the interpreter, Qiskit objects, copies, and
#: the gRPC/server overhead. A budget is not the same number as the cgroup.
_BUDGET_FRACTION = 0.80


def _read_int(path: str) -> Optional[int]:
    try:
        raw = Path(path).read_text().strip()
    except OSError:
        return None
    if raw == "" or raw == "max":
        return None
    try:
        return int(raw)
    except ValueError:
        return None


@lru_cache(maxsize=1)
def cgroup_memory_bytes() -> int:
    """Container limit in bytes, or 0 when no cgroup limit is visible."""
    for path in ("/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"):
        v = _read_int(path)
        if v and v > 0:
            return v
    return 0


@lru_cache(maxsize=1)
def memory_budget_mb() -> int:
    """Admission budget in MiB.

    KUBONA_BRIDGE_MEM_MB wins (operator override); otherwise 80% of the
    cgroup limit; otherwise a conservative 2048 MiB. Never returns 0.
    """
    raw = (os.getenv("KUBONA_BRIDGE_MEM_MB") or "").strip()
    if raw:
        try:
            v = int(raw)
            if v > 0:
                return v
        except ValueError:
            pass
    limit = cgroup_memory_bytes()
    if limit > 0:
        return max(512, int(limit / (1024 * 1024) * _BUDGET_FRACTION))
    return 2048


@lru_cache(maxsize=1)
def cpu_quota() -> int:
    raw = (os.getenv("KUBONA_BRIDGE_THREADS") or "").strip()
    if raw:
        try:
            v = int(raw)
            if v > 0:
                return v
        except ValueError:
            pass
    # cgroup v2: "<quota> <period>" or "max <period>"
    try:
        parts = Path("/sys/fs/cgroup/cpu.max").read_text().split()
        if parts and parts[0] != "max":
            quota, period = int(parts[0]), int(parts[1])
            if quota > 0 and period > 0:
                return max(1, math.ceil(quota / period))
    except (OSError, ValueError, IndexError):
        pass
    return max(1, min(8, os.cpu_count() or 1))


def transpile_level(default: int = 2) -> int:
    """Hardware/simulator transpile level, clamped to Qiskit's 0..3."""
    raw = (os.getenv("KUBONA_TRANSPILE_LEVEL") or "").strip()
    if raw:
        try:
            return max(0, min(3, int(raw)))
        except ValueError:
            pass
    return max(0, min(3, default))


def choose_method(circuit: Any) -> Optional[str]:
    """Pick an Aer method from circuit shape.

    Statevector is exact and is the default. Matrix-product-state is selected
    for wide, shallow circuits where the measured win is large (26 qubits:
    3.48 s statevector vs 0.19 s MPS) and entanglement is unlikely to saturate
    the bond dimension. Deep or narrow circuits keep statevector, and an
    explicit KUBONA_SIM_METHOD always wins.
    """
    forced = (os.getenv("KUBONA_SIM_METHOD") or "").strip().lower()
    if forced in ("statevector", "matrix_product_state", "automatic"):
        return None if forced == "automatic" else forced
    try:
        n = int(circuit.num_qubits)
        depth = int(circuit.depth())
    except Exception:
        return None
    if n >= 24 and depth <= 64:
        return "matrix_product_state"
    return None


def aer_options(circuit: Any = None, **overrides: Any) -> dict[str, Any]:
    """Options for every AerSimulator construction in the bridge."""
    opts: dict[str, Any] = {
        "max_memory_mb": memory_budget_mb(),
        "max_parallel_threads": cpu_quota(),
    }
    method = overrides.pop("method", None)
    # Noise models are not supported by the MPS method; leave the choice to
    # Aer's automatic path when a noise model is present.
    if method is None and circuit is not None and overrides.get("noise_model") is None:
        method = choose_method(circuit)
    if method:
        opts["method"] = method
        if method == "matrix_product_state":
            # 256 is ample for the shallow, low-entanglement circuits this
            # heuristic admits; the result still reports the method used.
            opts.setdefault("matrix_product_state_max_bond_dimension", 256)
    opts.update(overrides)
    return opts


def aer_simulator(circuit: Any = None, **overrides: Any):
    from qiskit_aer import AerSimulator

    return AerSimulator(**aer_options(circuit, **overrides))


def trim_memory() -> None:
    """Return freed glibc arenas to the OS after a large simulation.

    Best-effort: non-glibc platforms have no malloc_trim and no-op.
    """
    try:
        import ctypes

        libc = ctypes.CDLL("libc.so.6")
        libc.malloc_trim(0)
    except Exception:
        return
