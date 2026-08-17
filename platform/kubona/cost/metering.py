"""Algorithm-library metering (\u00a712.3) + x402 activation (\u00a712.7, Phase 5).

The algorithm library is a "niche data refinery": free low-resolution answers,
a sub-threshold x402 microtransaction for the full validated config (with
provenance). Below the approval threshold, this skips the human queue — the
x402 rail activates here. The ledger doubles as the payment-proof ledger.
"""

from __future__ import annotations

import hashlib
import time
import uuid
from dataclasses import dataclass
from typing import Any

from kubona.cost.payment import PaymentInterface, payments_for
from kubona.library import AlgorithmLibrary, LibraryEntry
from kubona.provenance import Ledger

APPROVAL_THRESHOLD_USD = 5.0  # below this, x402 sub-threshold flow (no human queue)


class MeteringError(RuntimeError):
    pass


@dataclass
class PurchaseResult:
    entry: LibraryEntry | None
    quote: dict[str, Any]
    verified: bool
    flow: str
    receipt_id: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return {"entry_id": self.entry.entry_id if self.entry else None,
                "quote": self.quote, "verified": self.verified, "flow": self.flow,
                "receipt_id": self.receipt_id}


class LibraryMeter:
    """Free low-res summary + x402 microtransaction for the full validated entry."""

    def __init__(self, library: AlgorithmLibrary, payments: PaymentInterface | None = None,
                 ledger: Ledger | None = None):
        self.library = library
        self.payments = payments
        self.ledger = ledger or Ledger()

    def free_summary(self, harness: str) -> dict[str, Any]:
        """Free tier: low-resolution answer (no full config)."""
        best = self.library.best(harness)
        if best is None:
            return {"ok": False, "error": "no validated library entries yet"}
        return {"ok": True, "harness": harness, "metric": best.metric,
                "summary": "validated configuration exists (full config requires a microtransaction)"}

    def quote(self, entry_id: str) -> dict[str, Any]:
        entry = self._get(entry_id)
        amount = min(0.99, APPROVAL_THRESHOLD_USD)
        pay = (self.payments or payments_for(amount))
        q = pay.quote(job_ref=entry_id, amount_usd=amount)
        return {"required": q.required, "amount_usd": q.amount_usd, "currency": q.currency,
                "flow": q.flow, "entry_id": entry_id}

    def purchase(self, entry_id: str, payment_proof: str | None = None) -> PurchaseResult:
        """Pay (x402 proof) -> verify (edge) -> release full validated config."""
        entry = self._get(entry_id)
        amount = min(0.99, APPROVAL_THRESHOLD_USD)
        pay = self.payments or payments_for(amount)
        quote = pay.quote(job_ref=entry_id, amount_usd=amount)
        verification = pay.verify(payment_proof or "", quote)
        if not verification["ok"]:
            raise MeteringError(verification["reason"])
        receipt_id = uuid.uuid4().hex[:12]
        self.ledger.append("payment", {"kind": "library_purchase", "entry_id": entry_id,
                                       "amount_usd": amount, "receipt_id": receipt_id,
                                       "proof_hash": hashlib.sha256((payment_proof or "").encode()).hexdigest()})
        self.ledger.append("library_release", {"entry_id": entry_id, "harness": entry.harness,
                                               "metric": entry.metric, "cost_usd": entry.cost_usd,
                                               "ledger_ref": entry.ledger_ref})
        return PurchaseResult(entry=entry, quote={"amount_usd": amount, "flow": quote.flow},
                              verified=True, flow=quote.flow, receipt_id=receipt_id)

    def _get(self, entry_id: str) -> LibraryEntry:
        for e in self.library.all():
            if e.entry_id == entry_id:
                return e
        raise MeteringError(f"unknown library entry {entry_id}")
