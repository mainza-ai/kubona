"""Slice 4: independent rerun (blank start, bounded budget, receipt)."""

import tempfile

from kubona.codesign import IndependentRerun, build_spec
from kubona.provenance import Ledger


def _candidate_list(cands):
    def _prop(_current, step):
        return list(cands[step % len(cands)])
    return _prop


def test_rerun_finds_and2_from_blank():
    spec = build_spec("and2")
    cands = [
        [],  # blank: identity, invalid for AND
        [("cx", (0, 2), ())],  # wrong: copies a
        [("ccx", (0, 1, 2), ())],  # valid AND, T=1 Q=3 TQ=3
    ]
    rr = IndependentRerun(spec, "and2", budget_steps=3, seed=7,
                          num_tests=8, proposer=_candidate_list(cands))
    receipt = rr.run()
    assert receipt["spec_label"] == "and2" and len(receipt["trials"]) == 3
    assert receipt["best"] is not None and receipt["best"]["tq"] == 3
    assert receipt["best"]["valid"] is True
    assert receipt["ledger_ref"] is None  # no ledger passed, honestly none


def test_rerun_blank_only_honest_no_result():
    spec = build_spec("and2")
    rr = IndependentRerun(spec, "and2", budget_steps=2, seed=7,
                          num_tests=8, proposer=lambda _c, _s: [])
    receipt = rr.run()
    assert receipt["best"] is None  # no fake best, just none
    assert all(t["valid"] is False for t in receipt["trials"])


def test_rerun_proposer_fault_is_trial_not_abort():
    spec = build_spec("and2")

    def _bad(_c, _s):
        raise RuntimeError("proposer blew up")

    receipt = IndependentRerun(spec, "and2", budget_steps=2,
                               proposer=_bad).run()
    assert len(receipt["trials"]) == 2
    assert all("proposer refused" in t["reason"] for t in receipt["trials"])
    assert receipt["best"] is None


def test_rerun_seals_ledger_trial():
    import os
    os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-rerun-"))
    spec = build_spec("and2")
    led = Ledger(path=tempfile.mktemp(prefix="kubona-rerun-ledger-", suffix=".jsonl"))
    cands = [[("ccx", (0, 1, 2), ())]]
    receipt = IndependentRerun(spec, "and2", budget_steps=1, seed=7,
                               num_tests=8, proposer=_candidate_list(cands),
                               ledger=led).run()
    assert receipt["ledger_ref"], "ledger seal must be recorded"
    assert receipt["ledger_error"] is None
