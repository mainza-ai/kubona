"""GQE harness (4th harness, §11.5): same four-part contract, wired into the engine."""

from kubona.autoresearch import Orchestrator
from kubona.autoresearch.gqe import build_gqe_harness


def test_gqe_harness_contract_and_search():
    h = build_gqe_harness()
    orch = Orchestrator(h, budget_steps=20, seed=7)
    # use the GQE proposal strategy (the flywheel's transformer stand-in)
    default_propose = orch.propose

    def gqe_propose(current):
        return h.propose(current)

    orch.propose = gqe_propose
    history = orch.search(initial=h.mutable_artifact())
    best = orch.best(history)
    assert best is not None and best.valid
    assert 0.0 <= best.metric <= 1.0
    assert 0 <= len(best.artifact) <= h.max_gates
