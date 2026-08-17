"""Evaluation framework (\u00a72.7 / Phase 4): code-gen corpus + agent behavior.

QuantumKatas / Qiskit HumanEval / PennyLang are the production corpora; this
module ships the built-in reference task set + invariant checks, and the
agent-behavior evals (budget compliance, simulator-first, WBP response).
"""

from .corpora import TASKS, evaluate_codegen
from .behavior import (budget_compliance_eval, simulator_first_eval, wbp_response_eval)

__all__ = ["TASKS", "evaluate_codegen", "budget_compliance_eval", "simulator_first_eval", "wbp_response_eval"]
