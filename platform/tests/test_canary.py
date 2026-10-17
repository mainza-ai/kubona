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
    # local-simulator canaries green; uncredentialed hardware SKIPS (W2.18:
    # the old all-ok held only because ibm-qiskit silently ran Aer — the same
    # skip convention quantinuum already had below, now applied uniformly:
    # no quantum-washing in the release gate).
    providers = ["local-simulator", "ibm-qiskit"]
    results = run_all(ledger=ledger, providers=providers)
    by_name = {r["provider"]: r for r in results}
    assert by_name["local-simulator"]["ok"] is True, results
    assert by_name["ibm-qiskit"].get("skip") is True, results
    assert "allow_sim_fallback" in by_name["ibm-qiskit"]["error"], results
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


def test_canary_attests_declared_qubits():
    """Q-14: the canary verifies the schema's declared qubit count against the
    plugin's live health report and seals the attestation."""
    from kubona.canary import attest_capacity
    att = attest_capacity("local-simulator", True)
    assert att["declared_qubits"] == 24, att
    assert att["measured_qubits"] == 24, att
    assert att["qubit_count_ok"] is True, att
    assert len(att["snapshot_hash"]) == 64, att


def test_canary_flags_schema_lie():
    """Q-14: a mismatch between measured and declared qubits fails attestation."""
    import kubona.schema as schema_pkg

    real_load = schema_pkg.load_provider
    cap = real_load("local-simulator").model_copy(update={"physical_qubits": 999})
    schema_pkg.load_provider = lambda name: cap
    try:
        from kubona.canary import attest_capacity
        att = attest_capacity("local-simulator", True)
        assert att["qubit_count_ok"] is False, att
    finally:
        schema_pkg.load_provider = real_load


def test_canary_check_carries_attestation():
    res = canary_check("local-simulator", shots=4096)
    assert res["ok"] is True, res
    assert res["attestation"]["qubit_count_ok"] is True, res


def test_canary_attests_declared_calibration():
    from kubona.canary import attest_calibration
    att = attest_calibration("local-simulator")
    assert att["compared"] is True, att
    assert att["calibration_ok"] is True, att
    assert att["mismatches"] == [], att


def test_canary_rejects_lying_calibration():
    import kubona.schema as schema_pkg
    from kubona.canary import attest_calibration
    real_load = schema_pkg.load_provider
    from kubona.schema.provider import CalibrationSnapshot
    cap = real_load("local-simulator").model_copy(
        update={"declared_calibration": CalibrationSnapshot(
            t1_us=10.0, t2_us=10.0, single_qubit_error=0.0,
            two_qubit_error=0.0, readout_error=0.0, source="fabricated")})
    schema_pkg.load_provider = lambda name: cap
    try:
        att = attest_calibration("local-simulator")
        assert att["calibration_ok"] is False, att
        assert any(m["field"] == "t1_us" for m in att["mismatches"]), att
    finally:
        schema_pkg.load_provider = real_load


def test_canary_unmeasurable_calibration_is_unknown_not_pass():
    from kubona.canary import attest_calibration
    att = attest_calibration("ibm-qiskit")
    assert att["calibration_ok"] is None, att
    assert att["compared"] is False, att


def test_unsound_declared_calibration_refuses_to_load(tmp_path):
    import json
    import pytest
    from kubona.schema.provider import ProviderCapability
    bad = {"provider": "x", "modality": "gate_model", "physical_qubits": 2,
           "declared_calibration": {"t1_us": 10.0, "t2_us": 100.0,
            "single_qubit_error": 0.0, "two_qubit_error": 0.0,
            "readout_error": 0.0, "source": "fabricated"}}
    with pytest.raises(Exception, match="unsound"):
        ProviderCapability.model_validate(bad)


def test_wilson_interval_edges_and_width():
    from kubona.canary.canary import wilson_interval
    lo, hi = wilson_interval(0, 100)
    assert lo == 0.0 and 0 < hi < 0.05
    lo, hi = wilson_interval(100, 100)
    assert hi == 1.0 and 0.95 < lo < 1.0
    lo, hi = wilson_interval(50, 100)
    assert lo < 0.5 < hi and (hi - lo) > 0.15
    # More shots narrow the interval (width carries the sample size).
    _, hi20 = wilson_interval(10, 20)
    lo20, _ = wilson_interval(10, 20)
    lo200, hi200 = wilson_interval(100, 200)
    assert (hi200 - lo200) < (hi20 - lo20)
    import pytest as _pytest
    with _pytest.raises(ValueError):
        wilson_interval(5, 0)
    with _pytest.raises(ValueError):
        wilson_interval(11, 10)


def test_canary_report_carries_intervals():
    # 4096 shots (tolerance unchanged at 5%): the balance assertion is a
    # statistical gate, so it runs where 5% is a robust bar (sigma ~1.6%) —
    # 1024 shots makes ANY correct sampler flaky (~11%/run), which is how the
    # suite got here. Same bar, strictly stronger evidence.
    res = canary_check("local-simulator", shots=4096)
    assert res["ok"] is True, res
    assert set(res["intervals"]) == set(res["counts"])
    for outcome, entry in res["intervals"].items():
        lo, hi = entry["interval_95"]
        assert 0 <= lo <= hi <= 1
        k = entry["k"]
        assert lo <= k / 4096 <= hi or k in (0, 4096)
