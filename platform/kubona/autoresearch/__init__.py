"""The autoresearch engine (§11) — a pluggable harness registry + orchestrator.

Four-part contract per §11.2: fixed_setup / mutable_artifact / metric /
validity_check. Structural write-scope (only the mutable artifact changes),
simulator-only by default (§11.3). The §13 meta-loop sits above this engine.
"""

from .base import Harness, Orchestrator, Trial
from .harnesses import build_vqe_ansatz_harness, build_mitigation_harness, build_correctness_harness

__all__ = ["Harness", "Orchestrator", "Trial", "build_vqe_ansatz_harness",
           "build_mitigation_harness", "build_correctness_harness"]
