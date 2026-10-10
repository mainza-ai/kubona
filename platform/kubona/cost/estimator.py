"""Cost estimation + mandatory simulator pre-flight (Phase 1 hardens this).

Phase 0 skeleton: static estimate from the capability pricing model and a
pre-flight gate (simulator-only when budget unavailable; local simulator = free).
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability


def estimate(capability: ProviderCapability, circuit: Circuit, shots: int) -> dict[str, Any]:
    est = capability.cost_estimate(shots, circuit)
    est.update({"circuit_name": circuit.name, "circuit_depth": circuit.depth()})
    return est


def preflight(
    capability: ProviderCapability,
    circuit: Circuit,
    shots: int,
    budget_ceiling_usd: float | None = None,
) -> dict[str, Any]:
    """Mandatory pre-flight: refuse paid queues without a budget ceiling."""
    est = estimate(capability, circuit, shots)
    cost = est["estimated_usd"]
    if cost == 0.0:
        ok, reason = True, "free tier (simulator) — no budget required"
    elif budget_ceiling_usd is None:
        ok, reason = False, "paid provider requires a budget ceiling (cost governance, Phase 1)"
    else:
        ok, reason = cost <= budget_ceiling_usd, f"estimate {cost} <= ceiling {budget_ceiling_usd}"
    return {"preflight_ok": ok, "reason": reason, "estimate": est}
