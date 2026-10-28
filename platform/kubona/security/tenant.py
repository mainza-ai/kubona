"""Principal-bound tenant identity (Phase-1 production hardening).

The gateway resolves the verified principal per call and injects a server-side
`caller` identity (sub + tenant claim) into tool args. Tools MUST prefer the
injected identity over any caller-supplied requester/tenant string:

- effective tenant = the principal's tenant when a caller identity is present;
- a caller-supplied tenant that differs from the principal's tenant REFUSES
  (fail-closed) — see GRANT_DECISION below;
- without an injected identity (direct in-process / stdio calls, which are
  transport-trusted), the supplied tenant is strictly validated but honored.

Tenant syntax is the existing refuse-don't-strip rule (`session.tenant_slug`)
plus exactness: the input must already BE the canonical slug. ``" Default "``
refuses instead of collapsing onto ``"Default"`` — silent collapsing merged
distinct tenants (``"Default"`` vs ``"default"`` vs ``" default "``) into
adjacent namespaces.
"""

from __future__ import annotations

from typing import Any

from kubona.mcp.auth import AuthError
from kubona.session import strict_tenant_slug


class CrossTenantRefusal(AuthError):
    """A caller-supplied tenant differs from the principal's tenant claim.

    Named (not a bare AuthError/ValueError) so operators and tests can match
    the fail-closed cross-tenant refusal precisely, and so the grant
    follow-up has a stable surface to extend.
    """


GRANT_DECISION = (
    "Cross-tenant grant decision (Phase-1): NO cross-tenant grant mechanism "
    "exists yet. Any request whose caller-supplied tenant differs from the "
    "principal's tenant claim refuses with CrossTenantRefusal (fail-closed). "
    "Follow-up before any cross-tenant path is allowed: introduce explicit, "
    "audited cross-tenant grants (grantor, grantee tenant, scope, expiry, "
    "revocation) — never an unchecked grant flag, never silent label widening."
)


def principal_tenant(principal: Any) -> str:
    """Derive the tenant claim from a verified principal (fail-closed).

    Issued by `OAuth2TokenIssuer.issue(tenant=...)` and attached at decode;
    principals without a claim (dev provider, legacy test doubles) resolve to
    the `"default"` tenant. A present-but-malformed claim refuses rather than
    falling back.
    """
    raw = getattr(principal, "tenant", None) or "default"
    try:
        return strict_tenant_slug(raw)
    except ValueError as e:
        raise AuthError(f"principal carries an invalid tenant claim: refusing ({e})") from e


def caller_from_principal(principal: Any) -> dict[str, str]:
    """Server-side caller identity the gateway injects into tool args."""
    return {"sub": principal.sub, "tenant": principal_tenant(principal)}


def resolve_tenant(*, supplied: str | None, caller: dict[str, str] | None,
                   default: str = "default") -> str:
    """Effective tenant for a tool call (injected identity wins, fail-closed).

    - With an injected `caller`: the principal's tenant governs. An omitted
      supply binds to it; a supply that differs refuses with CrossTenantRefusal
      (GRANT_DECISION — no grant mechanism exists yet).
    - Without one (direct in-process / stdio callers): the supply is strictly
      validated, or the legacy default applies when omitted.
    """
    if caller is not None:
        tenant = caller["tenant"]
        if supplied is not None and supplied != tenant:
            raise CrossTenantRefusal(
                f"caller-supplied tenant {supplied!r} differs from the principal's "
                f"tenant {tenant!r}: refusing ({GRANT_DECISION})"
            )
        return tenant
    if supplied is None:
        return default
    return strict_tenant_slug(supplied)
