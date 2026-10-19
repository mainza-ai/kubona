"""Circuit co-design harness on the four-part contract.

Fixed setup: TaskSpec + test budget + failure allowance + TQ kappa.
Mutable artifact: Builder-program op list (serializable).
Metric: -TQ when valid (higher is better to match Orchestrator.best),
  -inf-scale penalty when invalid (fail-closed, never silent).
Validity: evaluator verdict within allowance.
"""

from __future__ import annotations

from typing import Any

from kubona.autoresearch.base import Harness

from .frontier import MapElitesFrontier
from .kickmix import Builder, KickmixEvaluator, Op, TaskSpec
from .tq import TQ_KAPPA_DEFAULT, tq_cost


def _op_to_tuple(o: Op) -> tuple:
    return (o.name, tuple(o.qubits), tuple(o.cond))


def _tuple_to_op(t: tuple) -> Op:
    name, qs, cond = t
    return Op(str(name), tuple(int(x) for x in qs), tuple(int(x) for x in cond))


class CircuitCodesignHarness(Harness):
    """Simulator-only harness for reversible circuit tasks."""

    def __init__(
        self,
        spec: TaskSpec,
        num_tests: int = 256,
        max_failures: int = 0,
        kappa: int = TQ_KAPPA_DEFAULT,
        seed: int = 7,
    ):
        super().__init__("circuit_codesign")
        self.spec = spec
        self.num_tests = num_tests
        self.max_failures = max_failures
        self.kappa = kappa
        self.seed = seed
        self.evaluator = KickmixEvaluator()
        self.frontier = MapElitesFrontier(kappa=kappa)

    def fixed_setup(self) -> dict[str, Any]:
        return {
            "num_qubits": self.spec.num_qubits,
            "inputs": list(self.spec.inputs),
            "outputs": list(self.spec.outputs),
            "ancillas": list(self.spec.ancillas),
            "num_tests": self.num_tests,
            "max_failures": self.max_failures,
            "kappa": self.kappa,
            "backend": "kickmix-trajectory-simulator (local-classical, no QPU)",
        }

    def mutable_artifact(self) -> Any:
        return []  # empty op list (blank program)

    def _materialize(self, artifact: Any) -> Builder:
        b = Builder(self.spec.num_qubits)
        if not isinstance(artifact, list):
            raise ValueError("artifact must be a list of op tuples: refusing")
        for t in artifact:
            o = _tuple_to_op(tuple(t))
            # replay through Builder gates so kickmix membership + record
            # rules are re-enforced (a forged list cannot smuggle H in)
            if o.name == "alloc_marker":
                b.alloc(o.qubits[0])
            elif o.name == "x":
                b.x(o.qubits[0])
            elif o.name == "cx":
                b.cx(o.qubits[0], o.qubits[1])
            elif o.name == "ccx":
                b.ccx(o.qubits[0], o.qubits[1], o.qubits[2])
            elif o.name == "swap":
                b.swap(o.qubits[0], o.qubits[1])
            elif o.name == "z":
                b.z(o.qubits[0], o.cond)
            elif o.name == "cz":
                b.cz(o.qubits[0], o.qubits[1], o.cond)
            elif o.name == "ccz":
                b.ccz(o.qubits[0], o.qubits[1], o.qubits[2], o.cond)
            elif o.name == "global_flip":
                b.global_flip(o.cond)
            elif o.name == "hmr":
                b.hmr(o.qubits[0])
            elif o.name == "free_marker":
                # allocation bookkeeping is replayed from allocs below; a
                # bare free marker without live alloc refuses inside free()
                b.free(o.qubits[0])
            else:
                raise ValueError(f"non-kickmix op {o.name!r}: refusing")
        return b

    def run(self, artifact: Any) -> dict[str, Any]:
        try:
            prog = self._materialize(artifact)
        except Exception as e:  # forged artifact -> invalid, never a score
            return {"valid": False, "reason": str(e)[:280], "toffoli": 0,
                    "peak_qubits": self.spec.num_qubits, "tq": 10**18}
        # NOTE: alloc/free liveness is tracked by Builder during real
        # program construction. Serialized artifacts carry ops only, so
        # peak here is the addressable width (conservative floor);
        # full alloc-aware peaks flow via Builder objects in search loops.
        res = self.evaluator.evaluate(
            prog, self.spec, self.num_tests, self.max_failures, self.seed)
        if res["valid"]:
            tq = tq_cost(res["toffoli"], res["peak_qubits"], self.kappa)
        else:
            tq = 10**18  # invalid never outranks valid (fail-closed ranking)
        out = dict(res)
        out["tq"] = tq
        out["kappa"] = self.kappa
        return out

    def metric(self, result: dict[str, Any]) -> float:
        # higher is better (Orchestrator.best takes max over valid)
        return -float(result.get("tq", 10**18))

    def validity_check(self, result: dict[str, Any]) -> bool:
        return bool(result.get("valid", False))


def artifact_from_builder(b: Builder) -> list[tuple]:
    return [_op_to_tuple(o) for o in b.emit()]
