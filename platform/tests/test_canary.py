"""Canary CI: passes on healthy providers, CATCHES a real regression."""

from kubona.canary import canary_check, main, release_gate, run_all
from kubona.providers import list_providers


def test_canary_passes_local_simulator():
    res = canary_check("local-simulator", shots=4096)
    assert res["ok"] is True, res
    assert set(res["counts"]) <= {"00", "11"}


def test_canary_passes_qiskit():
    pytest = __import__("pytest")
    pytest.importorskip("qiskit")
    res = canary_check("qiskit-aer-simulator", shots=4096)
    assert res["ok"] is True, res


def test_canary_catches_regression():
    """The Phase-3 exit criterion: canary CI catches a real regression in staging."""
    import kubona.providers.local_simulator as ls

    orig = ls.LocalSimulator.submit

    def broken_submit(self, circuit, shots, seed=None):  # calibration-drift-style bug
        return {"counts": {"00": shots}, "shots": shots, "seed": seed}  # always 00

    ls.LocalSimulator.submit = broken_submit
    try:
        res = canary_check("local-simulator", shots=2048)
        assert res["ok"] is False
        assert res["checks"]["balanced_50_50"] is False
    finally:
        ls.LocalSimulator.submit = orig


def test_run_all_writes_canary_ledger(tmp_path):
    from kubona.provenance import Ledger

    ledger = Ledger(path=str(tmp_path / "ledger.jsonl"))
    # executable plugins (qiskit installed): canary must PASS
    providers = ["local-simulator", "ibm-qiskit"]
    results = run_all(ledger=ledger, providers=providers)
    assert all(r["ok"] for r in results), results
    assert sum(1 for r in ledger if r.kind == "canary") == len(providers)

    # an inaccessible provider (missing creds/SDK) is surfaced by the canary as
    # NOT ok with a reason — the canary's job is to flag it before an agent hits it.
    flagged = run_all(providers=["quantinuum"])[0]
    assert flagged["ok"] is False and flagged["error"] and flagged.get("skip") is True


# --- release-gate verdict semantics -------------------------------------------
# The gate was red on the hosted pipeline for weeks while green locally, because
# ci.yml asserted all(r['ok']) (skips count as failures) and ci.sh filtered skips.
# Both now call release_gate(); these tests pin the rule so it cannot fork again.

def test_release_gate_treats_fail_closed_skips_as_non_failures():
    results = [
        {"provider": "local-simulator", "ok": True, "checks": {"balanced_50_50": True}},
        {"provider": "quantinuum", "ok": False, "skip": True, "error": "no credentials"},
        {"provider": "dwave-annealing", "ok": False, "skip": True, "error": "QUBO, not circuits"},
    ]
    verdict = release_gate(results)
    assert verdict["ok"] is True
    assert verdict["ran"] == 1 and verdict["skipped"] == 2 and verdict["failures"] == []


def test_release_gate_blocks_a_real_execution_regression():
    """The fix must not soften the gate: a provider that RAN and is unhealthy blocks."""
    results = [
        {"provider": "local-simulator", "ok": False, "skip": False,
         "checks": {"balanced_50_50": False}},
        {"provider": "quantinuum", "ok": False, "skip": True, "error": "no credentials"},
    ]
    verdict = release_gate(results)
    assert verdict["ok"] is False
    assert [f["provider"] for f in verdict["failures"]] == ["local-simulator"]


def test_release_gate_refuses_an_all_skip_run():
    """A gate that executed nothing proves nothing — it must not go green."""
    verdict = release_gate([{"provider": "quantinuum", "ok": False, "skip": True}])
    assert verdict["ok"] is False and verdict["ran"] == 0


def test_release_gate_covers_non_gate_modalities_as_skips():
    """Annealing/AHS/CV providers are probed but reported as skips, not failures."""
    verdict = release_gate(run_all(providers=[
        name for name, cap in list_providers().items() if cap.execution_plugin
    ]))
    assert verdict["ok"] is True, verdict
    assert verdict["ran"] >= 1
    non_gate = {"dwave-annealing", "xanadu-strawberry-fields", "neutral-atom-quera-pasqal"}
    assert non_gate & set(verdict["skip_reasons"]), verdict["skip_reasons"]


def test_gate_cli_exit_code_is_the_verdict(tmp_path, monkeypatch):
    """The CLI exit code IS the gate verdict — this is what ci.yml now runs."""
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))
    assert main(["--gate"]) == 0
