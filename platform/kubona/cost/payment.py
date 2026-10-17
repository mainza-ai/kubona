"""Payment interface — designed x402-compatible from day one (\u00a712.2).

The cost-governance server's payment surface is an interface, not a checkout:
  quote(job) -> {amount, currency, required, flow}   (the HTTP-402 price quote)
  verify(proof, quote) -> {ok, reason}               (edge verifies before origin)

Phase 1 ships FreeTierPayments (simulator runs: required=False) and X402Payments
(interface complete; verification is a stub — activation is Phase 5+ per \u00a712.7,
but the flow is fixed so nothing needs retrofitting).
"""

from __future__ import annotations

import math
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Callable


def _check_amount(amount_usd: float) -> float:
    """W0.12: amounts are load-bearing (rail selection + settlement). NaN
    sails through every comparison and negatives invert pricing — both
    refuse. Zero is the only free amount."""
    if not isinstance(amount_usd, (int, float)) or not math.isfinite(amount_usd):
        raise ValueError(f"amount_usd must be a finite number, got {amount_usd!r}")
    if amount_usd < 0:
        raise ValueError(f"amount_usd must be >= 0, got {amount_usd!r}")
    return float(amount_usd)


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
    """The x402-compatible contract: a request IS the transaction.

    Verifier contract (W0.12): a verifier MUST bind the proof to
    (quote.job_ref, quote.amount_usd) and enforce single-use. A verifier
    that checks "a signature exists" without binding or replay protection
    turns every proof into a master key — the rail refuses empty proofs
    pre-verifier, but binding is the verifier's load-bearing duty.
    """

    @abstractmethod
    def quote(self, job_ref: str, amount_usd: float) -> Quote:
        """Price a job. Agents get the quote before execution (the 402 response)."""

    @abstractmethod
    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        """Verify payment proof for a quote. False -> job does not reach origin."""


class FreeTierPayments(PaymentInterface):
    """Simulator/free tier: no payment required (the P0/P1 default). Only
    quotable at exactly 0 — quoting a positive amount "free" is pricing
    fiction (W0.12)."""

    def quote(self, job_ref: str, amount_usd: float = 0.0) -> Quote:
        _check_amount(amount_usd)
        if amount_usd != 0.0:
            raise ValueError(f"free tier cannot quote a positive amount ({amount_usd}): use the x402 rail")
        return Quote(job_ref=job_ref, amount_usd=0.0, required=False, flow="free")

    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        return {"ok": True, "reason": "free tier — no payment required"}


class X402Payments(PaymentInterface):
    """x402 rail: HTTP 402 semantics — quote, pay, retry-with-proof, edge verifies.

    Phase 5+ activation. CRITICAL production posture: the rail FAILS CLOSED by
    default — no proof is ever accepted unless a real `verifier` callable is
    injected (a cryptographically-verifying edge). This prevents an open x402
    endpoint from becoming a free-spend hole while the protocol is stub-stage.
    """

    def __init__(self, issuer: str = "https://pay.kubona.local",
                 verifier: "Callable[[str, Quote], dict[str, Any]] | None" = None):
        self.issuer = issuer
        self._verifier = verifier  # None -> fail closed, never accept a proof
        self._seen_proofs: set[str] = set()  # in-process replay cache (see verify)

    def quote(self, job_ref: str, amount_usd: float) -> Quote:
        _check_amount(amount_usd)
        if amount_usd <= 0:
            raise ValueError("x402 requires a positive amount: zero/negative quotes refuse")
        return Quote(job_ref=job_ref, amount_usd=round(amount_usd, 6),
                     required=True, flow="http-402")

    def verify(self, proof: str, quote: Quote) -> dict[str, Any]:
        if self._verifier is None:
            return {"ok": False,
                    "reason": "x402 verification is not activated (Phase 5+); no verifier configured — failing closed"}
        if not proof or not proof.strip():
            return {"ok": False, "reason": "empty payment proof: refused before verification"}
        if quote.amount_usd <= 0:
            return {"ok": False, "reason": "x402 requires a positive amount"}
        # In-process single-use: one proof buys exactly one release here.
        # Restart resets this set — persistent single-use lives in the
        # payment-proof ledger (metering.purchase scans proof_hash history),
        # which survives restarts. Both layers state their scope honestly.
        if proof in self._seen_proofs:
            return {"ok": False, "reason": "payment proof already spent: replay refused"}
        out = self._verifier(proof, quote)
        if out.get("ok") is True:
            self._seen_proofs.add(proof)
        return out


def payments_for(amount_usd: float) -> PaymentInterface:
    """Route: exactly zero -> free tier; positive -> the x402 rail.
    Negative/non-finite refuses (W0.12): the old `<= 0` routed negative
    prices to the free tier — negative-price jobs quoted free."""
    _check_amount(amount_usd)
    return FreeTierPayments() if amount_usd == 0.0 else X402Payments()
