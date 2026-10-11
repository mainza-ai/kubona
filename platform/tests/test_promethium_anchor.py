"""Promethium anchor adapter: stub-driven, credential-free tests.

The live vendor binding does not exist yet (trial-gated); everything here
runs against stub drivers through the PromethiumDriver seam.
"""

import pytest

from kubona.integrations import promethium as pm
from kubona.provenance import Ledger


class StubDriver:
    def __init__(self, statuses, results=None):
        self.statuses = list(statuses)
        self.results = results or {}
        self.submitted = []

    def submit_job(self, spec):
        assert spec["kind"] == "SinglePointCalculation"
        self.submitted.append(spec)
        return "wf-test-1"

    def get_status(self, job_id):
        assert job_id == "wf-test-1"
        return self.statuses.pop(0) if self.statuses else "RUNNING"

    def get_results(self, job_id):
        return self.results


def _good_results():
    return {"results": {"rhf": {"converged": True, "energy": -75.123,
                                "scalars": {"Escf": -75.123}}}}


def test_anchor_parses_completed_run_and_logs_ledger():
    ledger = Ledger()
    out = pm.single_point_anchor("C", driver=StubDriver(["RUNNING", "COMPLETED"], _good_results()),
                                 basis="def2-SVP", poll_s=0, ledger=ledger)
    assert out["ok"] is True and out["energy_hartree"] == -75.123
    assert out["workflow_id"] == "wf-test-1" and out["ledger_ref"]
    assert out["scalars"] == {"Escf": -75.123}


def test_failed_cancelled_timeout_all_refuse():
    for statuses in (["FAILED"], ["CANCELLED"], []):
        with pytest.raises(pm.PromethiumError):
            pm.single_point_anchor("C", driver=StubDriver(statuses, _good_results()),
                                   poll_s=0, timeout_s=0.01)


def test_unconverged_completion_refuses():
    bad = {"results": {"rhf": {"converged": False, "energy": -1.0}}}
    with pytest.raises(pm.PromethiumError, match="unconverged"):
        pm.single_point_anchor("C", driver=StubDriver(["COMPLETED"], bad), poll_s=0)


def test_no_driver_no_key_refuses_before_network(monkeypatch):
    monkeypatch.delenv(pm.API_KEY_ENV, raising=False)
    with pytest.raises(pm.PromethiumError, match="not set"):
        pm.single_point_anchor("C", driver=None)
