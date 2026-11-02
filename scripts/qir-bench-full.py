#!/usr/bin/env python3
"""Measure the full qir-core path: process, JSON receipt, QIR copy, and metadata."""
from __future__ import annotations

import argparse
import json
import os
import resource
import statistics
import sys
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "platform" / "crates" / "qir-core" / "target" / "release" / "qir-core"


def qasm(gates: int, qubits: int) -> str:
    lines = ["OPENQASM 2.0;", 'include "qelib1.inc";', f"qreg q[{qubits}];", f"creg c[{qubits}];"]
    for i in range(gates):
        q = i % qubits
        kind = i % 5
        if kind == 0:
            lines.append(f"h q[{q}];")
        elif kind == 1:
            lines.append(f"rz({i * 0.01:.6f}) q[{q}];")
        elif kind == 2:
            lines.append(f"ry({i * 0.02:.6f}) q[{q}];")
        elif kind == 3 and qubits > 1:
            lines.append(f"cx q[{q}],q[{(q + 1) % qubits}];")
        else:
            lines.append(f"x q[{q}];")
    for q in range(qubits):
        lines.append(f"measure q[{q}] -> c[{q}];")
    return "\n".join(lines) + "\n"


def percentile(values: list[float], p: float) -> float:
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, round((len(ordered) - 1) * p))]


def run_case(gates: int, qubits: int, iterations: int) -> dict[str, object]:
    source = qasm(gates, qubits).encode()
    samples: list[float] = []
    bytes_out = 0
    for _ in range(3):
        subprocess.run([str(BINARY), "--json"], input=source, capture_output=True, check=True)
    for _ in range(iterations):
        started = time.perf_counter()
        proc = subprocess.run([str(BINARY), "--json"], input=source, capture_output=True, check=True)
        samples.append((time.perf_counter() - started) * 1000)
        bytes_out = len(proc.stdout)
        receipt = json.loads(proc.stdout)
        if receipt.get("ok") is not True or not receipt.get("qir"):
            raise SystemExit("qir-core returned an invalid JSON receipt")
    return {
        "gates": gates,
        "qubits": qubits,
        "iterations": iterations,
        "qasm_bytes": len(source),
        "receipt_bytes": bytes_out,
        "p50_ms": round(percentile(samples, 0.50), 3),
        "p95_ms": round(percentile(samples, 0.95), 3),
        "p99_ms": round(percentile(samples, 0.99), 3),
        "mean_ms": round(statistics.fmean(samples), 3),
        "max_child_rss_kb": round(resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss / (1024 if sys.platform == "darwin" else 1)),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--iterations", type=int, default=100)
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    if not BINARY.exists():
        raise SystemExit(f"missing {BINARY}: run cargo build --release --bin qir-core")
    cases = [(2, 2), (10, 4), (100, 8), (1_000, 12), (10_000, 16)]
    rows = [run_case(gates, qubits, args.iterations) for gates, qubits in cases]
    if args.json:
        print(json.dumps(rows, separators=(",", ":")))
    else:
        print("gates  qubits  iters  qasm_B  receipt_B  p50_ms  p95_ms  p99_ms  max_child_rss_kb")
        for row in rows:
            print(
                f"{row['gates']:>5}  {row['qubits']:>6}  {row['iterations']:>5}  "
                f"{row['qasm_bytes']:>6}  {row['receipt_bytes']:>9}  {row['p50_ms']:>6}  "
                f"{row['p95_ms']:>6}  {row['p99_ms']:>6}  {row['max_child_rss_kb']:>16}"
            )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
