"""Per-call identity-aware authorization (\u00a74.7) — Phase 3.

Replaces coarse role buckets: every tool maps to the fine-grained scope it
requires, and every principal (user or agent) carries exactly the scopes it
holds (agents via owner delegation, Phase-2 identity registry). A call is
authorized iff the principal's scopes cover the tool's required scope — checked
per call at the gateway, with delegation revocation taking effect immediately.
"""

from __future__ import annotations

from dataclasses import dataclass

from kubona.mcp.auth import AuthError, Principal
from kubona.identity import IdentityRegistry

# -- fine-grained scopes (tool -> required scope) ---------------------------
TOOL_SCOPES: dict[str, str] = {
    # compiler
    "compiler/transpile_circuit": "compiler:read",
    "compiler/lower_circuit": "compiler:read",
    # execution
    "execution/submit_job": "execution:submit",
    "execution/get_job_status": "execution:read",
    "execution/get_result": "execution:read",
    "execution/run_zne_job": "execution:submit",
    "execution/session_status": "execution:read",
    "execution/close_session": "execution:submit",
    "execution/list_provider_registry": "providers:read",
    # cost governance
    "cost/create_session": "cost:manage",
    "cost/estimate_cost": "cost:read",
    "cost/request_approval": "cost:manage",
    "cost/hold_approval": "cost:approve",
    "cost/approve_approval": "cost:approve",
    "cost/reject_approval": "cost:approve",
    "cost/session_burn_rate": "cost:read",
    "cost/quote_payment": "cost:read",
    "cost/verify_payment": "cost:read",
    # provenance
    "provenance/write_record": "provenance:write",
    "provenance/query_records": "provenance:read",
    # control plane
    "control/register_loop": "control:manage",
    "control/step_loop": "control:manage",
    "control/list_loops": "control:read",
    "control/loop_status": "control:read",
    "control/pause_loop": "control:override",
    "control/resume_loop": "control:override",
    "control/kill_loop": "control:override",
}

# Operator's default full scope set (dev principal).
OPERATOR_SCOPES = ("*",)

# A "submit-only researcher agent" example used by tests/docs:
AGENT_RESEARCHER_SCOPES = (
    "compiler:read", "execution:submit", "execution:read", "cost:read",
    "providers:read", "provenance:write", "provenance:read", "control:read",
)


@dataclass
class Decision:
    allowed: bool
    reason: str


class PolicyEngine:
    """Resolves a call to a decision based on the principal's effective scopes."""

    def __init__(self, identities: IdentityRegistry | None = None):
        self.identities = identities or IdentityRegistry()

    def effective_scopes(self, principal: Principal) -> tuple[str, ...]:
        """A user's scopes come from their token identity; an agent's from a
        LIVE owner delegation (revocation is immediate — checked per call)."""
        if principal.scopes == ("*",) and principal.role == "operator":
            return ("*",)
        if self.identities is not None:
            agent_scopes = self.identities.access(principal.sub)
            if agent_scopes is not None:
                return agent_scopes
        return principal.scopes

    def decide(self, principal: Principal, tool_path: str) -> Decision:
        required = TOOL_SCOPES.get(tool_path)
        if required is None:
            return Decision(False, f"no scope mapping for {tool_path}")
        scopes = self.effective_scopes(principal)
        if "*" in scopes or required in scopes:
            return Decision(True, f"{principal.sub} authorized for {tool_path}")
        return Decision(False, f"{principal.sub} lacks {required!r} for {tool_path}")
