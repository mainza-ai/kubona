"""SDK conformance suite: the Phase-4 gate artifact (Q-24, choice C).

Grades sidecar provider candidates fail-closed: schema, trait, seeded
determinism, Bell correctness, bad-shot refusal, attestation honesty (Q-14),
calibration, cost-model sanity, credential hygiene.
"""

import pytest

from kubona.circuits import Circuit, Gate
from kubona.providers.base import ProviderError
from kubona.providers.local_simulator import LocalSimulator
from kubona.sdk import conformance_gate, grade_candidate

_SIM_CAP = {
    "provider": "conformant-sim",
    "modality": "statevector_simulator",
    "physical_qubits": 24,  # matches LocalSimulator's hard cap (cf. local-simulator.json)
    "native_gate_set": ["h", "x", "cx"],
    "pricing": {"model": "free"},
    "status": "beta",
}
_SIM_PLUGIN = "kubona.providers.local_simulator.LocalSimulator"


def _passes(report):
    return {c["check"]: c["pass"] for c in report["checks"]}


def test_conformant_simulator_passes_full_suite():
    report = grade_candidate(_SIM_CAP, _SIM_PLUGIN)
    assert report["pass"] is True, report
    assert report["failures"] == []
    assert _passes(report)["bell_correct"] is True


def test_bad_schema_fails_closed():
    report = grade_candidate({"provider": 42}, _SIM_PLUGIN)
    assert report["pass"] is False
    assert report["failures"] == ["schema_valid"]


def test_missing_plugin_fails_trait():
    report = grade_candidate(_SIM_CAP, "kubona.providers.ghost.NoSuch")
    assert report["pass"] is False
    assert "trait_loads" in report["failures"]


def test_non_provider_plugin_rejected():
    report = grade_candidate(_SIM_CAP, "kubona.circuits.Circuit")
    assert report["pass"] is False
    assert "trait_loads" in report["failures"]


class _LyingQubits(LocalSimulator):
    """Reports a live capacity that contradicts the declared schema (Q-14)."""

    def health(self):
        return {"provider": self.capability.provider, "status": "ok",
                "measured_qubits": 999}


def test_lying_attestation_fails():
    report = grade_candidate(
        _SIM_CAP, "test_sdk_conformance._LyingQubits")
    assert report["pass"] is False
    assert "attestation_honest" in report["failures"]


class _AcceptsBadShots(LocalSimulator):
    """Returns counts for non-positive shots instead of raising."""

    def submit(self, circuit: Circuit, shots: int, seed=None):
        if shots <= 0:
            return {"counts": {"0": 0}, "shots": shots}
        return super().submit(circuit, shots, seed)


def test_bad_shots_acceptance_fails():
    report = grade_candidate(
        _SIM_CAP, "test_sdk_conformance._AcceptsBadShots")
    assert report["pass"] is False
    assert "rejects_bad_shots" in report["failures"]


def test_unpriced_per_second_rail_fails_cost():
    cap = dict(_SIM_CAP, provider="unpriced-sim",
               pricing={"model": "per_second", "per_second_usd": 0.0})
    report = grade_candidate(cap, _SIM_PLUGIN)
    assert report["pass"] is False
    assert "cost_model_sane" in report["failures"]


def test_priced_per_second_reports_basis():
    cap = dict(_SIM_CAP, provider="priced-sim",
               pricing={"model": "per_second", "per_second_usd": 1.60,
                        "notes": "IBM PAYG per-second"})
    report = grade_candidate(cap, _SIM_PLUGIN)
    assert _passes(report)["cost_model_sane"] is True
    detail = [c for c in report["checks"] if c["check"] == "cost_model_sane"][0]["detail"]
    assert "time-model v1" in detail


def test_conformance_gate_requires_graded_candidates():
    assert conformance_gate([])["ok"] is False  # reviewed nothing: not a pass
    ok_report = grade_candidate(_SIM_CAP, _SIM_PLUGIN)
    assert conformance_gate([ok_report])["ok"] is True
    bad_report = grade_candidate({"provider": 42}, _SIM_PLUGIN)
    assert conformance_gate([ok_report, bad_report])["ok"] is False


def test_annealing_graded_on_qubo_not_bell():
    """Annealing speaks QUBO: determinism probed via solve_qubo, Bell is N/A,
    hardware attestation deferred to the admission-time canary."""
    import json
    from pathlib import Path
    cap = json.loads(Path("kubona/schema/providers/dwave-annealing.json").read_text())
    report = grade_candidate(
        cap, "kubona.providers.annealing_provider.AnnealingProvider")
    passed = _passes(report)
    assert passed["deterministic_seeded"] is True
    assert passed["bell_correct"] is None  # N/A, not fail
    assert passed["attestation_honest"] is None  # deferred, named
    assert "bell_correct" in report["na"] and "attestation_honest" in report["na"]
    assert report["pass"] is True, report


def test_unpriced_hardware_rail_still_fails():
    """rigetti.json declares per-second billing with no rate: the gate must
    refuse (a $0.00 default would read as a free tier downstream)."""
    import json
    from pathlib import Path
    cap = json.loads(Path("kubona/schema/providers/rigetti.json").read_text())
    report = grade_candidate(
        cap, "kubona.providers.pyquil_provider.PyQuilProvider")
    assert "cost_model_sane" in report["failures"]
    assert report["pass"] is False
