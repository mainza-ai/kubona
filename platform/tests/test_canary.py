"""Canary CI: passes on healthy providers, CATCHES a real regression."""

from kubona.canary import canary_check, run_all
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

    # a provider without deps is FLAGGED by the canary (ok=False + reason) — the
    # canary's job is to surface it before an agent hits it.
    flagged = run_all(providers=["quantinuum"])[0]
    assert flagged["ok"] is False and "pytket" in flagged["error"]
