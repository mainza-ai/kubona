"""QAgent/Kubona Search Mode (§11.4) — the first shipped use of the engine.

Autonomous ansatz-search: the orchestrator searches the VQE ansatz harness,
simulator-only by default, and returns the best entry plus a comparison against
a hand-designed baseline. Its exit criterion: at least one ansatz beating a
hand-designed baseline on a benchmark problem, entirely in simulation.
"""

from __future__ import annotations

from kubona.autoresearch.base import Harness, Orchestrator, Trial
from kubona.library import AlgorithmLibrary


def run_search_mode(harness: Harness, *, budget_steps: int = 60, seed: int = 7,
                    baseline_artifact=None, library: AlgorithmLibrary | None = None,
                    ledger=None) -> dict:
    """Search, compare vs a hand-designed baseline, file the best into the library."""
    baseline = harness.run(baseline_artifact) if baseline_artifact is not None else None
    baseline_score = harness.metric(baseline) if baseline is not None else None

    orch = Orchestrator(harness, budget_steps=budget_steps, seed=seed, ledger=ledger)
    history = orch.search()
    best: Trial = orch.best(history)

    entry_id = None
    if library is not None:
        entry = library.add(harness=harness.name, artifact=best.artifact, metric=best.metric,
                            cost_usd=best.cost_usd, ledger_ref=best.ledger_ref or "none",
                            baseline_beaten=baseline_score is not None and best.metric > baseline_score)
        entry_id = entry.entry_id

    return {
        "harness": harness.name,
        "baseline": {"artifact": baseline_artifact, "score": baseline_score} if baseline_artifact is not None else None,
        "best": best.to_dict(),
        "beat_baseline": baseline_score is not None and best.metric > baseline_score,
        "library_entry_id": entry_id,
        "steps": len(history),
    }
