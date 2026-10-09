"""Hardware-canary CI (\u00a72.7 / Phase 3): nightly correctness checks.

Runs a canary circuit against every executable provider and verifies expected
results within tolerance, before an agent hits a broken backend mid-workflow.
Results are written to the provenance ledger (kind="canary").
"""

from .canary import CANARY_QASM, attest_capacity, canary_check, release_gate, run_all, main

__all__ = ["CANARY_QASM", "attest_capacity", "canary_check", "release_gate", "run_all", "main"]
