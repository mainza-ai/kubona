"""Phase-4 evals: code-gen corpus + agent-behavior dimensions."""

from kubona.eval import (budget_compliance_eval, evaluate_codegen,
                         simulator_first_eval, wbp_response_eval)


def test_codegen_corpus_evaluates_perfect_generator():
    """A generator that returns the exact reference QASM passes every task."""
    from kubona.eval.corpora import TASKS

    def perfect(name):
        return TASKS[name]["qasm"]

    out = evaluate_codegen(perfect)
    assert out["score"] == 1.0
    assert all(v["ok"] for v in out["tasks"].values())


def test_codegen_corpus_fails_broken_generator():
    def broken(name):
        return "OPENQASM 2.0;\nqreg q[1];"  # doesn't satisfy any task

    out = evaluate_codegen(broken)
    assert out["score"] == 0.0


def test_budget_compliance_blocks_past_ceiling():
    from kubona.cost.governance import GovernanceError

    def submit(sid, amount):
        raise GovernanceError("ceiling")

    assert budget_compliance_eval(submit)["ok"] is True


def test_simulator_first_blocks_paid_without_preflight():
    def attempt(sid):
        raise ValueError("paid provider requires budget ceiling")

    assert simulator_first_eval(attempt)["ok"] is True


def test_wbp_response_restart_not_loop():
    assert wbp_response_eval(lambda s: "restart")["ok"] is True
    assert wbp_response_eval(lambda s: "continue")["ok"] is False
