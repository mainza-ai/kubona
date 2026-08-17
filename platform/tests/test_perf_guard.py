"""Perf-regression guards (G12): the hot paths must not silently O(exponential).

Ensures the canonicalization/lowering/simulation hot paths stay fast on a
moderate circuit, so a refactor that introduces an accidental blowup is caught
in CI rather than in production.
"""

import time

from kubona.circuits import Circuit, Gate, to_qasm
from kubona.qir import lower
from kubona.providers.local_simulator import LocalSimulator
from kubona.schema import load_provider


def _moderate_circuit(n=10, depth=30):
    gates = []
    for d in range(depth):
        for q in range(n):
            gates.append(Gate("ry", (q,), (0.3,)))
        for q in range(0, n - 1, 2):
            gates.append(Gate("cx", (q, q + 1)))
    return Circuit(name="perf", num_qubits=n, gates=gates)


def test_lower_stays_fast():
    c = _moderate_circuit()
    t0 = time.time()
    out = lower(to_qasm(c))
    assert time.time() - t0 < 2.0


def test_local_simulator_stays_fast():
    c = _moderate_circuit(n=8, depth=15)  # keep small enough to enumerate
    sim = LocalSimulator(load_provider("local-simulator"))
    t0 = time.time()
    res = sim.submit(c, shots=1024, seed=7)
    elapsed = time.time() - t0
    assert "counts" in res
    assert elapsed < 3.0, f"local sim too slow: {elapsed:.2f}s"
