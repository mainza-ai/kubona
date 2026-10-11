"""Promethium classical-anchor adapter (GPU DFT single-points as anchors).

Uses the anchor to ground quantum chemistry claims: a fast classical DFT
energy that must exist before any QPU spend is compared against it.

Architecture (credential-free until trial):
  - `PromethiumDriver` protocol: submit_job(spec) -> id, get_status(id),
    get_results(id). The vendor-SDK binding (private distribution, account
    required) is written at trial time against this protocol — no vendor
    method names are guessed here.
  - `single_point_anchor(...)`: submit -> bounded poll -> parse RHF energy
    + scalars -> normalized anchor dict + ledger record. FAILED/CANCELLED/
    timeout refuse (never partial data as an anchor).
  - Credential rule: the live driver reads KUBONA_PROMETHIUM_API_KEY from
    the environment (vault-leased in production); absent key refuses
    before any network touch.
"""

from __future__ import annotations

import os
import time
from typing import Any, Protocol

from kubona.provenance import Ledger

API_KEY_ENV = "KUBONA_PROMETHIUM_API_KEY"


class PromethiumDriver(Protocol):
    """Minimal seam a vendor binding must satisfy (defined here so the
    adapter core is testable without accounts, SDKs, or network)."""

    def submit_job(self, spec: dict[str, Any]) -> str: ...
    def get_status(self, job_id: str) -> str: ...
    def get_results(self, job_id: str) -> dict[str, Any]: ...


class PromethiumError(RuntimeError):
    """Raised when the anchor cannot be produced (fail-closed)."""


def _require_api_key() -> str:
    key = os.environ.get(API_KEY_ENV, "").strip()
    if not key:
        raise PromethiumError(
            f"Promethium anchor refused: {API_KEY_ENV} not set "
            "(trial account + vault-leased key required; refusing network)")
    return key


def single_point_anchor(smiles_or_xyz: str, driver: PromethiumDriver | None = None,
                        basis: str = "def2-SVP", timeout_s: float = 600.0,
                        poll_s: float = 10.0,
                        ledger: Ledger | None = None) -> dict[str, Any]:
    """Run a DFT single-point through the driver; return the anchor record.

    driver=None means "live path": requires the API key (refuses without
    it) and a bound SDK driver — until trial, pass an explicit driver
    (tests pass stubs). Polls bounded; any non-COMPLETED terminal state
    or timeout refuses rather than returning partial numbers.
    """
    if driver is None:
        _require_api_key()
        raise PromethiumError(
            "live vendor binding not yet written (trial step): pass an "
            "explicit PromethiumDriver")
    spec = {"kind": "SinglePointCalculation", "molecule": smiles_or_xyz,
            "basis": basis}
    job_id = driver.submit_job(spec)
    deadline = time.time() + timeout_s
    status = ""
    while time.time() < deadline:
        status = (driver.get_status(job_id) or "").upper()
        if status in ("COMPLETED", "FAILED", "CANCELLED", "ERROR"):
            break
        time.sleep(poll_s)
    if status != "COMPLETED":
        raise PromethiumError(
            f"Promethium job {job_id} did not complete (status={status or 'timeout'}): "
            "no anchor produced")
    results = driver.get_results(job_id) or {}
    rhf = (results.get("results") or {}).get("rhf") or {}
    if not rhf.get("converged") or rhf.get("energy") is None:
        raise PromethiumError(
            f"Promethium job {job_id} completed unconverged: no anchor produced")
    anchor = {
        "ok": True,
        "workflow_id": job_id,
        "method": "promethium-dft-single-point",
        "basis": basis,
        "energy_hartree": float(rhf["energy"]),
        "converged": True,
        "scalars": dict(rhf.get("scalars") or {}),
    }
    if ledger is not None:
        record = ledger.append("promethium-anchor", {
            "workflow_id": job_id, "basis": basis,
            "energy_hartree": anchor["energy_hartree"]})
        anchor["ledger_ref"] = record.record_id if record else None
    return anchor
