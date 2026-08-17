"""Phase 5: breadth providers, routing/export-control, multi-tenant scale, x402 metering, meta auto-apply."""

import threading

import pytest
import numpy as np

from kubona.autoresearch.meta import MetaLoop, reflect_on_orchestrator
from kubona.cost.metering import APPROVAL_THRESHOLD_USD, LibraryMeter, MeteringError
from kubona.cost.payment import X402Payments

def _test_verifier(proof, quote):
    return {"ok": proof.startswith("x402:"), "reason": "edge-verified (test)"}

from kubona.library import AlgorithmLibrary
from kubona.provenance import Ledger
from kubona.providers import get_provider, list_providers
from kubona.routing import ROUTE_ELIGIBLE, ROUTE_INELIGIBLE, route
from kubona.schema import load_provider


# -- breadth providers ------------------------------------------------------
def test_registry_has_phase5_providers():
    caps = list_providers()
    assert "xanadu-strawberry-fields" in caps and "dwave-annealing" in caps
    assert caps["xanadu-strawberry-fields"].modality.value == "photonic"
    assert caps["dwave-annealing"].modality.value == "annealing"


def test_annealing_solves_qubo():
    prov = get_provider("dwave-annealing")
    # min x0^2 + x1^2 - 2 x0 x1 + ... -> Q for (x0 - x1)^2 = x0^2 + x1^2 - 2x0x1
    Q = np.array([[1.0, -1.0], [-1.0, 1.0]])
    res = prov.solve_qubo(Q, seed=1)
    assert res["energy"] == pytest.approx(0.0)  # x0 == x1 (00 or 11) minimizes to 0
    assert res["solution"][0] == res["solution"][1]


def test_annealing_rejects_gate_circuits():
    from kubona.circuits import parse_qasm2
    from kubona.providers.base import ProviderError

    with pytest.raises(ProviderError):
        get_provider("dwave-annealing").submit(parse_qasm2("OPENQASM 2.0;\nqreg q[1];\nh q[0];"), 10)


# -- routing / export control ----------------------------------------------
def test_routing_allowed():
    cap = load_provider("quantinuum")  # jurisdiction US/UK, review_required_non_allied
    r = route(cap, caller_jurisdictions=["US"], caller_export_class="allied")
    assert r.verdict == ROUTE_ELIGIBLE, r


def test_routing_jurisdiction_mismatch():
    cap = load_provider("quantinuum")
    r = route(cap, caller_jurisdictions=["EUEAST"], caller_export_class="allied")
    assert r.verdict == ROUTE_INELIGIBLE and "jurisdiction" in r.reason


def test_routing_export_control_review_gate():
    cap = load_provider("quantinuum")  # flag = review_required_non_allied
    r = route(cap, caller_jurisdictions=["US"], caller_export_class="reviewed_non_allied", require_review=False)
    assert r.verdict == ROUTE_INELIGIBLE  # non-allied needs review; not granted
    r2 = route(cap, caller_jurisdictions=["US"], caller_export_class="reviewed_non_allied", require_review=True)
    assert r2.verdict == ROUTE_ELIGIBLE


def test_routing_none_flag_open():
    cap = load_provider("local-simulator")  # export_control_flag = none
    assert route(cap, caller_jurisdictions=["UK"], caller_export_class="any").verdict == ROUTE_ELIGIBLE


# -- multi-tenant scale -----------------------------------------------------
def test_tenant_ledgers_do_not_collide(tmp_path):
    a = Ledger(path=str(tmp_path / "led.jsonl"), tenant="lab-a")
    b = Ledger(path=str(tmp_path / "led.jsonl"), tenant="lab-b")
    a.append("experiment", {"circuit_hash": "a"})
    b.append("experiment", {"circuit_hash": "b"})
    # distinct files -> no cross-tenant namespace collision
    assert str(a.path) != str(b.path)
    assert a.path.exists() and b.path.exists()
    assert [r.payload["circuit_hash"] for r in a] == ["a"]
    assert [r.payload["circuit_hash"] for r in b] == ["b"]
    assert a.validate_chain()[0] and b.validate_chain()[0]


def test_ledger_concurrent_appends_safe(tmp_path):
    ledger = Ledger(path=str(tmp_path / "led.jsonl"))
    errors = []

    def writer(tag):
        try:
            for _ in range(50):
                ledger.append("job", {"tag": tag})
        except Exception as e:  # noqa: BLE001
            errors.append(e)

    threads = [threading.Thread(target=writer, args=(f"t{i}",)) for i in range(4)]
    for th in threads:
        th.start()
    for th in threads:
        th.join()

    assert not errors
    ok, msg = ledger.validate_chain()
    assert ok, msg
    assert ledger.count() == 200


# -- x402 activation + library metering -------------------------------------
def test_metering_free_summary_and_x402_purchase(tmp_path):
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    entry = lib.add(harness="vqe_ansatz", artifact=3.6, metric=1.118, cost_usd=0.0,
                    ledger_ref="r1")
    meter = LibraryMeter(lib, ledger=Ledger(path=str(tmp_path / "l.jsonl")))

    summary = meter.free_summary("vqe_ansatz")
    assert summary["ok"] and "microtransaction" in summary["summary"]

    quote = meter.quote(entry.entry_id)
    assert quote["required"] is True and quote["flow"] == "http-402"
    assert quote["amount_usd"] <= APPROVAL_THRESHOLD_USD

    # production posture: no injected verifier -> fail closed (never release paid config)
    with pytest.raises(MeteringError):
        meter.purchase(entry.entry_id, payment_proof="x402:proof-1")

    # metering activates only when a real payment/verifier is injected
    meter2 = LibraryMeter(lib, ledger=Ledger(path=str(tmp_path / "l.jsonl")),
                          payments=X402Payments(verifier=_test_verifier))
    result = meter2.purchase(entry.entry_id, payment_proof="x402:proof-1")
    assert result.verified and result.entry.entry_id == entry.entry_id

    with pytest.raises(MeteringError):
        meter2.purchase(entry.entry_id, payment_proof="bad-proof")


def test_metering_records_payment_proof_in_ledger(tmp_path):
    lib = AlgorithmLibrary(path=str(tmp_path / "lib.json"))
    entry = lib.add(harness="h", artifact="a", metric=1.0, cost_usd=0.0, ledger_ref="r")
    ledger = Ledger(path=str(tmp_path / "l.jsonl"))
    LibraryMeter(lib, ledger=ledger, payments=X402Payments(verifier=_test_verifier)).purchase(entry.entry_id, payment_proof="x402:p")
    kinds = [r.kind for r in ledger]
    assert "payment" in kinds and "library_release" in kinds
    assert ledger.validate_chain()[0]


# -- meta-loop auto-apply (staged shadow rollout, §13.5) --------------------
def test_meta_auto_apply_requires_safety_period_and_shadow_win(tmp_path):
    meta = MetaLoop(store_path=str(tmp_path / "meta.json"))
    history = [0.1, 0.2, 0.3, 0.2, 0.5, 0.6, 0.7, 0.8]
    p = reflect_on_orchestrator(history, 0.1, 1.0, meta)
    meta.approve(p.proposal_id, reviewer="pi")

    # rejected: safety period not elapsed
    with pytest.raises(ValueError):
        meta.auto_apply_candidate(p.proposal_id, shadow_scores_incumbent=[0.5] * 5,
                                  shadow_scores_proposed=[0.7] * 5, safety_period_elapsed=False)

    # rejected: shadow mode shows no improvement
    with pytest.raises(ValueError):
        meta.auto_apply_candidate(p.proposal_id, shadow_scores_incumbent=[0.7] * 5,
                                  shadow_scores_proposed=[0.5] * 5, safety_period_elapsed=True)

    # applied: safety elapsed + shadow win across min shadow runs
    applied = meta.auto_apply_candidate(p.proposal_id, shadow_scores_incumbent=[0.5] * 5,
                                        shadow_scores_proposed=[0.75] * 5, safety_period_elapsed=True)
    assert applied.status == "APPLIED"
    assert applied.shadow_evidence["auto_apply"]["shadow_runs"] == 5
