"""x402-compatible payment interface tests."""

from kubona.cost.payment import FreeTierPayments, Quote, X402Payments, payments_for


def test_free_tier_no_payment():
    pay = FreeTierPayments()
    q = pay.quote("job-1", 0.0)
    assert q.required is False and q.flow == "free"
    assert pay.verify("", q)["ok"] is True


def test_x402_quote_and_verify_flow_fails_closed():
    pay = X402Payments()
    q = pay.quote("job-2", 0.25)
    assert q.required is True and q.flow == "http-402"
    assert q.amount_usd == 0.25
    # no verifier injected -> fail closed: never accept a proof
    assert pay.verify("x402:some-proof", q)["ok"] is False
    assert pay.verify("bad-proof", q)["ok"] is False


def test_x402_verify_activates_only_with_a_real_verifier():
    def real_verifier(proof, quote):
        return {"ok": proof == "x402:valid-" + quote.job_ref, "reason": "edge-verified"}
    pay = X402Payments(verifier=real_verifier)
    q = pay.quote("job-3", 0.25)
    assert pay.verify("x402:valid-job-3", q)["ok"] is True
    assert pay.verify("x402:invalid-job-3", q)["ok"] is False
    assert pay.verify("x402:valid-job-3", Quote(job_ref="job-3", amount_usd=0.0))["ok"] is False


def test_router():
    assert isinstance(payments_for(0.0), FreeTierPayments)
    assert isinstance(payments_for(0.05), X402Payments)
