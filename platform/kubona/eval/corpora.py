"""Built-in code-generation corpus + invariant-based evaluation.

Production corpora (QuantumKatas, Qiskit HumanEval, PennyLang) plug into the same
evaluate_codegen contract; the built-in reference tasks pin the evaluator.
"""

from __future__ import annotations

from kubona.circuits import Circuit, Gate, parse_qasm2
from kubona.providers import get_provider

# -- task definitions: name -> (circuit builder, invariant checker) ----------
TASKS: dict[str, dict] = {}


def _task(name: str, qasm: str, check) -> None:
    TASKS[name] = {"qasm": qasm, "check": check}


def _bell_check(counts, shots) -> bool:
    return set(counts) <= {"00", "11"} and abs(counts.get("00", 0) - counts.get("11", 0)) <= 0.1 * shots


def _ghz3_check(counts, shots) -> bool:
    return set(counts) <= {"000", "111"}


def _qft2_check(counts, shots) -> bool:
    # QFT2 on |00>: uniform superposition of all 4 basis states (up to phases)
    return all(abs(counts.get(k, 0) - shots / 4) <= 0.12 * shots for k in ("00", "01", "10", "11"))


def _dj2_check(counts, shots) -> bool:
    # balanced oracle on 2 qubits: 1st register always |0> or always |1> (deterministic)
    return len(counts) == 1 and set(counts) <= {"00", "01"}


_task("bell", "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];", _bell_check)
_task("ghz3", "OPENQASM 2.0;\nqreg q[3];\nh q[0];\ncx q[0],q[1];\ncx q[0],q[2];", _ghz3_check)
_task("qft2", "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncp(1.5708) q[0],q[1];\nh q[1];", _qft2_check)
def evaluate_codegen(generate: "Callable[[str], str]", provider: str = "local-simulator",
                     shots: int = 4096) -> dict:
    """Run `generate(task_name) -> QASM` for every task; verify invariants on the simulator.

    Returns per-task pass/fail + overall score (the QuantumKatas/HumanEval
    contract; RAG-verified corpora plug in identically).
    """
    sim = get_provider(provider)
    results = {}
    for name, task in TASKS.items():
        try:
            qasm = generate(name)
            circuit = parse_qasm2(qasm)
            counts = sim.submit(circuit, shots, seed=5)["counts"]
            ok = task["check"](counts, shots)
        except Exception as e:  # noqa: BLE001 — a broken generator fails the task, not the eval
            ok = False
            qasm = f"<generator error: {e}>"
        results[name] = {"ok": ok, "qasm": qasm}
    score = sum(1 for r in results.values() if r["ok"]) / max(len(results), 1)
    return {"score": round(score, 3), "tasks": results}
