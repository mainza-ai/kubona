"""Jurisdiction + export-control aware routing.

A provider's capability carries `jurisdiction` (e.g. "US/UK") and
`export_control_flag` (e.g. "none" | "review_required_non_allied"). Routing
checks both against the caller's allowed jurisdictions and their export-control
classification — enforced at the routing layer, BEFORE a job reaches a provider
(the answer to the \u00a72.9 compliance gap).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from kubona.schema import ProviderCapability

ROUTE_ELIGIBLE = "ELIGIBLE"
ROUTE_INELIGIBLE = "INELIGIBLE"


# Export-control tiers: who may route to a given flag.
# "none" -> anyone; "allied_only" -> allied operators only; "review_required_non_allied" -> needs case-by-case review.
_ALLOWED_FOR_CLASS: dict[str, set[str]] = {
    "none": {"any"},
    "allied_only": {"allied"},
    "review_required_non_allied": {"allied", "reviewed_non_allied"},
}


@dataclass(frozen=True)
class Route:
    verdict: str
    provider: str
    reason: str
    checks: dict[str, Any]

    def to_dict(self) -> dict[str, Any]:
        return {"verdict": self.verdict, "provider": self.provider, "reason": self.reason,
                "checks": self.checks}


def route(capability: ProviderCapability,
          caller_jurisdictions: list[str],
          caller_export_class: str = "any",
          require_review: bool = False) -> Route:
    """Decide whether a caller may route to this provider."""
    checks = {}

    # -- jurisdiction: provider must be reachable from at least one caller region
    prov_jurs = _to_set(capability.jurisdiction)
    caller_jurs = {j.upper() for j in caller_jurisdictions}
    jur_ok = ((prov_jurs & caller_jurs) or ("NONE" in prov_jurs)
              or ("WORLD" in prov_jurs) or ("GLOBAL" in prov_jurs)
              or not caller_jurisdictions)  # no caller region constraint = open
    checks["jurisdiction"] = {"provider": sorted(prov_jurs), "caller": sorted(caller_jurs), "ok": jur_ok}

    # -- export control
    flag = capability.export_control_flag or "none"
    allowed_classes = _ALLOWED_FOR_CLASS.get(flag, {"any"})
    ec_ok = "any" in allowed_classes or caller_export_class in allowed_classes
    checks["export_control"] = {"flag": flag, "caller_class": caller_export_class, "ok": ec_ok}

    # -- a flag that needs review gates the route unless reviewed
    review_ok = True
    if flag == "review_required_non_allied" and caller_export_class == "reviewed_non_allied":
        review_ok = require_review
    checks["review"] = {"required": flag == "review_required_non_allied", "granted": review_ok}

    ok = jur_ok and ec_ok and review_ok
    reason = "eligible" if ok else (
        "jurisdiction mismatch" if not jur_ok else
        "export-control class not allowed" if not ec_ok else
        "export-control review required but not granted")
    return Route(verdict=ROUTE_ELIGIBLE if ok else ROUTE_INELIGIBLE,
                 provider=capability.provider, reason=reason, checks=checks)


def _to_set(s: str) -> set[str]:
    return {x.strip().upper() for x in s.replace("and", "/").split("/") if x.strip()}
