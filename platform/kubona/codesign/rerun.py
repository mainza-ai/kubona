"""Independent rerun (spec + blank -> evaluator only, ledger-sealed receipt).

The credibility piece: a rerun starts from the blank program with a fixed
budget + seed, tries proposer-supplied candidates, scores each with the
KickmixEvaluator (ground truth), and seals a receipt. No human between
rounds, no LLM judging, no SOTA claim without a full-budget receipt.

Ledger: reuses the closed `trial` kind (harness=circuit_codesign_rerun) —
no taxonomy change. A ledger failure is recorded in the receipt, never
silent, and never discards the scored trials.
"""

from __future__ import annotations

from typing import Any, Callable

from .families import deterministic_mutator
from .frontier import MapElitesFrontier
from .harness import CircuitCodesignHarness
from .kickmix import TaskSpec


def _summarize(artifact: Any) -> str:
    try:
        ops = list(artifact)
        names = [tuple(t)[0] if isinstance(t, (list, tuple)) else "?" for t in ops]
        return f"{len(ops)}ops:{','.join(str(n) for n in names[:8])}"
    except Exception:
        return "unserializable-artifact"


class IndependentRerun:
    """Bounded, seeded, evaluator-scored rerun loop."""

    def __init__(
        self,
        spec: TaskSpec,
        spec_label: str,
        budget_steps: int = 8,
        seed: int = 7,
        num_tests: int = 64,
        max_failures: int = 0,
        kappa: int = 1,
        proposer: Callable[[list, int], list] | None = None,
        ledger: Any | None = None,
    ):
        if not spec_label.strip():
            raise ValueError("spec_label must be non-blank: refusing")
        if budget_steps < 1 or budget_steps > 256:
            raise ValueError(f"budget_steps must be in [1,256], got {budget_steps}: refusing")
        self.spec = spec
        self.spec_label = spec_label
        self.budget_steps = budget_steps
        self.seed = seed
        self.harness = CircuitCodesignHarness(
            spec, num_tests=num_tests, max_failures=max_failures, kappa=kappa, seed=seed)
        self.proposer = proposer or deterministic_mutator
        self.ledger = ledger
        self.frontier = MapElitesFrontier(kappa=kappa)

    def run(self) -> dict[str, Any]:
        trials: list[dict[str, Any]] = []
        best: dict[str, Any] | None = None
        current: list = []  # blank program
        for step in range(self.budget_steps):
            try:
                candidate = self.proposer(list(current), step)
            except Exception as e:  # proposer fault -> invalid trial, never abort
                trials.append({"step": step, "valid": False,
                               "reason": f"proposer refused: {e}"[:200],
                               "current": _summarize(current)})
                continue
            res = self.harness.run(candidate)
            entry: dict[str, Any] = {
                "step": step,
                "valid": bool(res.get("valid", False)),
                "toffoli": res.get("toffoli", 0),
                "peak_qubits": res.get("peak_qubits", self.spec.num_qubits),
                "tq": res.get("tq", 10**18),
                "eps_bound_97_5": res.get("eps_bound_97_5"),
                "reason": str(res.get("reason", "ok"))[:200],
                "candidate": _summarize(candidate),
            }
            trials.append(entry)
            if entry["valid"]:
                self.frontier.offer(candidate, entry["toffoli"],
                                    entry["peak_qubits"], {"step": step})
                if best is None or entry["tq"] < best["tq"]:
                    best = dict(entry)
                    best["artifact_summary"] = entry["candidate"]
            # hill-climb: next proposal starts from best-so-far when valid
            if best is not None and entry["valid"] and entry["tq"] <= best["tq"]:
                current = list(candidate)
        receipt: dict[str, Any] = {
            "spec_label": self.spec_label,
            "seed": self.seed,
            "budget_steps": self.budget_steps,
            "kappa": self.harness.kappa,
            "num_tests": self.harness.num_tests,
            "trials": trials,
            "best": best,
            "frontier_cells": len(self.frontier.entries()),
            "ledger_ref": None,
            "ledger_error": None,
        }
        if self.ledger is not None:
            try:
                rec = self.ledger.append("trial", {
                    "harness": "circuit_codesign_rerun",
                    "spec_label": self.spec_label,
                    "seed": self.seed,
                    "budget_steps": self.budget_steps,
                    "best_tq": best["tq"] if best else None,
                    "best_valid": best is not None,
                    "artifact": best["candidate"] if best else "none",
                })
                receipt["ledger_ref"] = getattr(rec, "record_id", None) or getattr(
                    rec, "record_hash", None)
            except Exception as e:  # noqa: BLE001 — sealed as error, never silent
                receipt["ledger_error"] = str(e)[:280]
        return receipt
