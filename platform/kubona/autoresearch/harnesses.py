"""The three shipping harnesses (§11.5) — VQE ansatz, mitigation, correctness.

QAOA and compilation-pass search follow once the engine has proven out.
"""

from __future__ import annotations

import math

from kubona.autoresearch.base import Harness
from kubona.circuits import Circuit, Gate, parse_qasm2
from kubona.mitigation import run_zne
from kubona.providers import get_provider

# -- 1. VQE ansatz search ------------------------------------------------
H_ANSATZ = "OPENQASM 2.0;\nqreg q[2];\nry({theta}) q[0];\nry({theta}) q[1];\ncx q[0],q[1];"


def _energy(theta: float) -> float:
    """<H>(theta) = <Z0Z1> + 0.5<X0X1> (matches P1 gate test, ground -1.118)."""
    sim = get_provider("local-simulator")
    qasm = H_ANSATZ.format(theta=round(theta, 4))
    counts = sim.submit(parse_qasm2(qasm), 8192, seed=3)["counts"]
    total = sum(counts.values()) or 1
    zz = sum(c * (1.0 if int(k, 2) in (0, 3) else -1.0) for k, c in counts.items()) / total
    rotated = parse_qasm2(qasm.replace(";\ncx q[0],q[1];", ";\ncx q[0],q[1];\nh q[0];\nh q[1];"))
    counts_x = sim.submit(rotated, 8192, seed=3)["counts"]
    xx = sum(c * (1.0 if int(k, 2) in (0, 3) else -1.0) for k, c in counts_x.items()) / total
    return zz + 0.5 * xx


class VQEAnsatzHarness(Harness):
    """Mutable: the ansatz parameter theta. Fixed: H, backend, shots."""

    def __init__(self):
        super().__init__("vqe_ansatz")
        self._backend = "local-simulator"
        self._hamiltonian = "Z0Z1 + 0.5 X0X1"
        self._ground_energy = -1.118

    def fixed_setup(self) -> dict:
        return {"backend": self._backend, "hamiltonian": self._hamiltonian,
                "ground_energy": self._ground_energy, "shots": 8192}

    def mutable_artifact(self):
        return 0.0  # starting theta (resampling fresh in propose)

    def metric(self, result: dict) -> float:
        return -result["energy"]  # higher = closer to ground

    def validity_check(self, result: dict) -> bool:
        return -2.0 * math.pi <= result["theta"] <= 2.0 * math.pi

    def run(self, theta: float) -> dict:
        return {"theta": theta, "energy": _energy(theta), "depth": 3}


# -- 2. Mitigation-strategy search ---------------------------------------
class MitigationHarness(Harness):
    """Mutable: the ZNE scale set. Fixed: a noisy |1> preparation."""

    def __init__(self):
        super().__init__("mitigation_strategy")
        self._circuit = parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nx q[0];")
        self._noise = 0.05

    def fixed_setup(self) -> dict:
        return {"circuit": "x|0>", "noise_rate": self._noise, "target": -1.0}

    def mutable_artifact(self):
        return (1, 3, 5)

    def metric(self, result: dict) -> float:
        return -result["abs_error"]

    def validity_check(self, result: dict) -> bool:
        scales = result["scales"]
        return (len(scales) >= 2 and all(s >= 1 and s % 2 == 1 for s in scales)
                and scales == tuple(sorted(scales)))

    def run(self, scales: tuple[int, ...]) -> dict:
        res = run_zne(self._circuit, shots=20000, noise_rate=self._noise,
                      scales=tuple(scales), seed=11)
        return {"scales": tuple(scales), "mitigated": res.mitigated_value,
                "raw": res.raw_value, "abs_error": abs(res.mitigated_value - (-1.0))}


# -- 3. Correctness regression (nothing mutated; gates releases) ---------
BELL = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
GHZ3 = parse_qasm2("OPENQASM 2.0;\nqreg q[3];\nh q[0];\ncx q[0],q[1];\ncx q[0],q[2];")


class CorrectnessHarness(Harness):
    """No mutation: runs known-answer circuits; pass/fail; wired into canary CI.

    gates releases: a failing correctness run blocks the release (the §11.5
    correctness-regression harness).
    """

    def __init__(self, provider: str = "local-simulator", shots: int = 2048):
        super().__init__("correctness_regression")
        self._provider = provider
        self._shots = shots

    def fixed_setup(self) -> dict:
        return {"provider": self._provider, "shots": self._shots,
                "checks": ["bell_entangled", "ghz3_entangled"]}

    def mutable_artifact(self):
        return None  # nothing is agent-mutable

    def metric(self, result: dict) -> float:
        return 1.0 if result["pass"] else 0.0

    def validity_check(self, result: dict) -> bool:
        return True

    def run(self, _artifact) -> dict:
        sim = get_provider(self._provider)
        bell = sim.submit(BELL, self._shots, seed=5)["counts"]
        ghz = sim.submit(GHZ3, self._shots, seed=5)["counts"]
        bell_ok = set(bell) <= {"00", "11"} and abs(bell.get("00", 0) - bell.get("11", 0)) <= 0.1 * self._shots
        # GHZ3: outcomes must be only 000 or 111
        ghz_ok = set(ghz) <= {"000", "111"}
        checks = {"bell_entangled": bell_ok, "ghz3_entangled": ghz_ok}
        return {"pass": all(checks.values()), "checks": checks, "provider": self._provider}


def build_vqe_ansatz_harness() -> Harness:
    return VQEAnsatzHarness()


def build_mitigation_harness() -> Harness:
    return MitigationHarness()


def build_correctness_harness(provider: str = "local-simulator") -> Harness:
    return CorrectnessHarness(provider=provider)
