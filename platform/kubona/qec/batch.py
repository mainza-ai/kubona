"""Phase 6 multi-worker sinter batch driver with checkpointed CSV resume."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any, Sequence

import sinter

from kubona.qec.circuits import build_surface_code_memory_circuit, si1000_noise
from kubona.qec.sim import wilson_ci_95


def build_surface_sinter_tasks(
    distances: Sequence[int],
    physical_error_rates: Sequence[float],
    *,
    rounds: int | None = None,
) -> list[sinter.Task]:
    """Build sinter.Task objects for rotated surface-code threshold sweeps."""
    tasks: list[sinter.Task] = []
    for d in distances:
        for p in physical_error_rates:
            nm = si1000_noise(float(p))
            r = rounds if rounds is not None else int(d)
            circ, _ = build_surface_code_memory_circuit(int(d), rounds=r, noise=nm)
            tasks.append(
                sinter.Task(
                    circuit=circ,
                    json_metadata={
                        "code_name": f"surface_d{d}",
                        "distance": int(d),
                        "rounds": r,
                        "p": float(p),
                        "noise": nm.name,
                    },
                )
            )
    return tasks


def run_sinter_batch(
    tasks: Sequence[sinter.Task],
    *,
    num_workers: int | None = None,
    max_shots: int = 2000,
    max_errors: int = 50,
    decoders: Sequence[str] = ("pymatching",),
    save_resume_filepath: str | Path | None = None,
) -> list[dict[str, Any]]:
    """Run parallel sinter collection with early stopping and optional CSV checkpoint resume."""
    workers = num_workers if num_workers is not None else max(1, min(4, os.cpu_count() or 1))
    resume_path = str(save_resume_filepath) if save_resume_filepath is not None else None
    stats = sinter.collect(
        num_workers=workers,
        tasks=list(tasks),
        decoders=list(decoders),
        max_shots=int(max_shots),
        max_errors=int(max_errors),
        save_resume_filepath=resume_path,
    )
    out: list[dict[str, Any]] = []
    for st in stats:
        shots = int(st.shots)
        errs = int(st.errors)
        ler = errs / shots if shots > 0 else 0.0
        ci_lo, ci_hi = wilson_ci_95(errs, shots)
        out.append({
            "decoder": st.decoder,
            "metadata": dict(st.json_metadata) if isinstance(st.json_metadata, dict) else {},
            "shots": shots,
            "errors": errs,
            "discards": int(st.discards),
            "seconds": round(float(st.seconds), 4),
            "ler": round(ler, 6),
            "ci_low": round(ci_lo, 6),
            "ci_high": round(ci_hi, 6),
        })
    return out
