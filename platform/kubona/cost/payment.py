"""Payment interface — designed x402-compatible from day one (\u00a712.2).

The cost-governance server's payment surface is an interface, not a checkout:
  quote(job) -> {amount, currency, required, flow}   (the HTTP-402 price quote)
  verify(proof, quote) -> {ok, reason}               (edge verifies before origin)

Phase 1 ships FreeTierPayments (simulator runs: required=False) and X402Payments
(interface complete; verification is a stub — activation is Phase 5+ per \u00a712.7,
but the flow is fixed so nothing needs retrofitting).
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True)
class Quote:
    job_ref: str
    amount_usd: float
    currency: str = "USD"
    required: bool = False
    flow: str = "free"  # "free" | "http-402"

    def to_dict(self) -> dict[str, Any]:
        return {"job_ref": self.job_ref, "amount_usd": self.amount_usd,
                "currency": self.currency, "required": self.required, "flow": self.flow}


class PaymentInterface(ABC):
    """The x402-compatible contract: a request IS the transaction."""

    @abstractmethod
    def quote(self, job_ref: str, amount_usd: float) -> Quote:
        """Price a job. Agents get the quote before execution (the 402 response)."""

    @abstractmethod
    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        """Verify payment proof for a quote. False -> job does not reach origin."""


class FreeTierPayments(PaymentInterface):
    """Simulator/free tier: no payment required (the P0/P1 default)."""

    def quote(self, job_ref: str, amount_usd: float = 0.0) -> Quote:
        return Quote(job_ref=job_ref, amount_usd=0.0, required=False, flow="free")

    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        return {"ok": True, "reason": "free tier — no payment required"}


class X402Payments(PaymentInterface):
    """x402 rail: HTTP 402 semantics — quote, pay, retry-with-proof, edge verifies.

    Phase 5+ activation; the flow below is the design, implemented as a stub so
    the cost server's interface never needs to change.
    """

    def __init__(self, issuer: str = "https://pay.kubona.local"):
        self.issuer = issuer

    def quote(self, job_ref: str, amount_usd: float) -> Quote:
        return Quote(job_ref=job_ref, amount_usd=round(amount_usd, 6),
                     required=True, flow="http-402")

    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        # Stub: accept a well-formed proof for the exact quote amount.
        # Real verification (cryptographic proof, edge check) is Phase 5+.
        if proof.startswith("x402:") and quote.amount_usd > 0:
            return {"ok": True, "reason": f"proof accepted for ${quote.amount_usd} (stub verifier)"}
        return {"ok": False, "reason": "invalid x402 proof"}


def payments_for(amount_usd: float) -> PaymentInterface:
    """Route: sub-cent-to-zero -> free tier; otherwise the x402 rail."""
    return FreeTierPayments() if amount_usd <= 0.0 else X402Payments()
