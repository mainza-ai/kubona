"""Phase 4: the autoresearch engine — three harnesses + Search Mode gates."""

import pytest

from kubona.autoresearch import (Orchestrator, build_correctness_harness,
                                 build_mitigation_harness, build_vqe_ansatz_harness)
from kubona.autoresearch.search_mode import run_search_mode
from kubona.library import AlgorithmLibrary


def test_vqe_harness_contract():
    h = build_vqe_ansatz_harness()
    setup = h.fixed_setup()
    assert setup["hamiltonian"] == "Z0Z1 + 0.5 X0X1"
    result = h.run(3.6)
    assert result["energy"] < -1.0  # near ground -1.118
    assert h.validity_check(result) and h.metric(result) > 1.0


def test_mitigation_harness_contract():
    h = build_mitigation_harness()
    r = h.run((1, 3, 5))
    assert h.validity_check({"scales": (1, 3, 5)})
    assert not h.validity_check({"scales": (2, 4)})   # even scales invalid
    assert not h.validity_check({"scales": (1,)})      # too few
    assert h.metric(r) > -0.01  # ZNE abs_error very small -> metric = -abs_error near 0


def test_correctness_harness_passes_on_healthy_provider():
    h = build_correctness_harness("local-simulator")
    r = h.run(None)
    assert r["pass"] is True, r
    assert r["checks"]["bell_entangled"] and r["checks"]["ghz3_entangled"]


def test_correctness_harness_catches_regression():
    """§11.5 exit addition: the correctness harness caught a real regression."""
    import kubona.providers.local_simulator as ls

    orig = ls.LocalSimulator.submit

    def broken(self, circuit, shots, seed=None):
        return {"counts": {"01": shots}, "shots": shots, "seed": seed}  # Bell broken

    ls.LocalSimulator.submit = broken
    try:
        r = build_correctness_harness("local-simulator").run(None)
        assert r["pass"] is False
        assert r["checks"]["bell_entangled"] is False
    finally:
        ls.LocalSimulator.submit = orig


def test_orchestrator_keep_or_discard_loop():
    h = build_vqe_ansatz_harness()
    orch = Orchestrator(h, budget_steps=15, seed=7)
    history = orch.search()
    best = orch.best(history)
    assert len(history) == 16  # 15 search steps + 1 refinement trial
    assert best is not None
    assert best.metric == max(t.metric for t in history if t.valid)


def test_search_mode_beats_hand_designed_baseline(tmp_path):
    """Phase-4 gate: Search Mode produces an ansatz beating a hand-designed baseline."""
    h = build_vqe_ansatz_harness()
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    out = run_search_mode(h, budget_steps=60, seed=7,
                          baseline_artifact=2.8, library=lib)  # baseline E ≈ -0.77
    assert out["beat_baseline"] is True, out
    assert out["best"]["metric"] > 1.0  # E < -1.0 (close to ground -1.118)
    assert out["library_entry_id"] is not None
    assert len(lib) == 1
    assert lib.best("vqe_ansatz").artifact != 2.8  # the search found something better


def test_library_full_pipeline_cost_approved(tmp_path):
    """§11.5 exit addition: a library entry through the simulator -> cost-approved
    -> library pipeline."""
    from kubona.cost.governance import CostGovernanceService
    from kubona.session import SessionStore
    from kubona.provenance import Ledger

    gov = CostGovernanceService(SessionStore(path=str(tmp_path / "s.jsonl")), store_path=str(tmp_path))
    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    s = gov.sessions.create("library-pipeline", "researcher", budget_ceiling_usd=100.0)

    # simulator tier runs free; the approval gate exercises the paid path
    # (W0.9: approval mints a real signed mandate, so the request carries a
    # positive budget with an explicit lab, program, and operator key).
    import os
    a = gov.request_approval(s.session_id, 5.0, "library entry validation")
    a = gov.hold_approval(a.approval_id, reviewer="pi")
    a = gov.approve(a.approval_id, reviewer="pi", tenant_id="lab-a",
                    job_hash="job-lib", signing_key=os.urandom(32))

    h = build_vqe_ansatz_harness()
    orch = Orchestrator(h, budget_steps=10, seed=7, ledger=ledger)
    best = orch.best(orch.search())
    rec = ledger.append("library_entry", {"harness": h.name, "artifact": best.artifact,
                                          "metric": best.metric, "cost_usd": 0.0,
                                          "approval": a.approval_id})
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    lib.add(harness=h.name, artifact=best.artifact, metric=best.metric, cost_usd=0.0,
            ledger_ref=rec.record_id, approval=a.approval_id)
    entry = lib.best(h.name)
    assert entry is not None and entry.ledger_ref == rec.record_id
    assert ledger.validate_chain()[0] is True
