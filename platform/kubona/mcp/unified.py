"""Unified, profile-scoped Kubona MCP server (Phase 1).

Aggregates Kubona's federated MCP servers behind a single MCPServer instance
that speaks both `stdio` (`python -m kubona.run serve-mcp`) and standard MCP
Streamable HTTP (`POST /mcp` on `http_gateway.py`), while routing every tool
invocation through `KubonaGateway` so auth, PolicyEngine scopes, caller
injection, MRTR paid-execution approvals, and audit logging are enforced
identically across `/mcp`, `/rpc`, and `stdio`.

Profiles prevent context flooding (wiki/concepts/mcp-and-context-flooding.md):
  - all:        every mounted tool across all 7 servers
  - read-only:  strictly non-mutating inspection, estimation, and recall tools
  - science:    compiler + execution + science + cost estimation + provenance read
  - governance: cost + control + session autonomy + provenance
  - ops:        control + provenance + execution status/cancel + session autonomy
  - world:      observatory world tools + provenance read
"""

from __future__ import annotations

import contextvars
import inspect
from typing import Any

from mcp.server.mcpserver import MCPServer

from kubona.mcp.auth import AuthError
from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.prompts import register_prompts
from kubona.mcp.resources import register_resources

KUBONA_MCP_INSTRUCTIONS = (
    "Kubona is a local-first quantum and scientific discovery platform governed by three "
    "invariants: (1) no quantum energy or advantage claim without a classical anchor "
    "(call `get_classical_anchor` first); (2) no paid execution without a session budget "
    "ceiling and human/mandate approval (`estimate_cost` informs, ceilings bind); "
    "(3) every action produces an immutable ledger receipt. "
    "Read `kubona://skills` and `kubona://contracts/error-catalog` for guided playbooks "
    "and exact error recovery."
)

# Per-request bearer token bound by Streamable HTTP middleware (or stdio env).
CURRENT_BEARER_TOKEN: contextvars.ContextVar[str | None] = contextvars.ContextVar(
    "kubona_mcp_bearer_token", default=None
)

# Profile -> allowed tool paths ("server/tool") or server prefixes ("server/*").
PROFILES: dict[str, tuple[str, ...]] = {
    "all": ("*",),
    "read-only": (
        "compiler/transpile_circuit",
        "compiler/lower_circuit",
        "compiler/compile_circuit",
        "control/list_loops",
        "control/loop_status",
        "cost/estimate_cost",
        "cost/session_burn_rate",
        "cost/quote_payment",
        "execution/get_job_status",
        "execution/get_result",
        "execution/session_status",
        "execution/list_provider_registry",
        "provenance/query_records",
        "science/get_classical_anchor",
        "science/list_hypotheses",
        "science/get_research_graph",
        "science/recall_memory",
        "science/get_session_autonomy",
        "world/world_look",
        "world/world_probe",
        "world/world_replay",
    ),
    "science": (
        "compiler/*",
        "execution/*",
        "science/*",
        "cost/estimate_cost",
        "cost/session_burn_rate",
        "cost/create_session",
        "provenance/query_records",
        "provenance/write_record",
    ),
    "governance": (
        "cost/*",
        "control/*",
        "provenance/*",
        "science/get_session_autonomy",
        "execution/session_status",
        "execution/cancel_job",
        "execution/close_session",
    ),
    "ops": (
        "control/*",
        "provenance/query_records",
        "execution/get_job_status",
        "execution/get_result",
        "execution/cancel_job",
        "execution/session_status",
        "execution/list_provider_registry",
        "cost/session_burn_rate",
        "science/get_session_autonomy",
    ),
    "world": (
        "world/*",
        "provenance/query_records",
        "execution/session_status",
    ),
}


def _profile_allows(profile: str, server: str, tool: str) -> bool:
    patterns = PROFILES[profile]
    full = f"{server}/{tool}"
    for pat in patterns:
        if pat == "*" or pat == full or pat == f"{server}/*":
            return True
    return False


def _make_gateway_wrapper(
    gateway: KubonaGateway,
    server_name: str,
    tool_name: str,
    fn: Any,
    default_token: str | None = None,
) -> Any:
    """Wrap a tool function so MCP SDK sees its exact public signature (minus
    server-injected `caller`), while execution routes through `gateway.call`."""
    sig = inspect.signature(fn)
    public_params = [
        p for name, p in sig.parameters.items()
        if name != "caller"
    ]
    public_sig = sig.replace(parameters=public_params)

    def _wrapper(*args: Any, **kwargs: Any) -> dict:
        bound = public_sig.bind(*args, **kwargs)
        bound.apply_defaults()
        token = CURRENT_BEARER_TOKEN.get()
        if token is None:
            token = default_token
        try:
            return gateway.call(server_name, tool_name, dict(bound.arguments), token=token)
        except AuthError as e:
            msg = str(e)
            code = "approval_required" if "approval" in msg.lower() else "forbidden"
            return {
                "ok": False,
                "error_code": code,
                "error": msg,
                "remediation": (
                    "Paid execution requires human/mandate approval or configured KUBONA_APPROVAL_SECRET; "
                    "use local-simulator / qiskit-aer-simulator for zero-cost runs."
                    if code == "approval_required"
                    else "Verify bearer token scopes and tenant ownership."
                ),
            }
        except KeyError as e:
            return {"ok": False, "error_code": "unknown_tool", "error": str(e)}
        except ValueError as e:
            return {"ok": False, "error_code": "bad_request", "error": str(e)}

    _wrapper.__name__ = tool_name
    _wrapper.__doc__ = fn.__doc__
    _wrapper.__signature__ = public_sig  # type: ignore[attr-defined]
    annotations = {
        k: v for k, v in getattr(fn, "__annotations__", {}).items()
        if k != "caller"
    }
    _wrapper.__annotations__ = annotations
    return _wrapper


def build_unified_mcp_server(
    gateway: KubonaGateway,
    *,
    profile: str = "all",
    default_token: str | None = None,
    include_resources: bool = True,
    include_prompts: bool = True,
) -> MCPServer:
    """Build a unified, profile-scoped MCPServer backed by `gateway`.

    Raises ValueError on an unknown profile (fail-closed).
    """
    if profile not in PROFILES:
        raise ValueError(
            f"unknown MCP profile {profile!r}; allowed: {sorted(PROFILES)}"
        )

    srv_name = "kubona" if profile == "all" else f"kubona-{profile}"
    server = MCPServer(srv_name, instructions=KUBONA_MCP_INSTRUCTIONS)

    # Mount every allowed tool from the gateway registry in deterministic order
    # (sorted by server, tool) for LLM prompt-cache stability.
    for sname in sorted(gateway._registry.keys()):
        tools = gateway._registry[sname]
        for tname in sorted(tools.keys()):
            if not _profile_allows(profile, sname, tname):
                continue
            fn = tools[tname]
            desc = (inspect.getdoc(fn) or f"{sname}/{tname}").splitlines()[0]
            wrapped = _make_gateway_wrapper(
                gateway, sname, tname, fn, default_token=default_token
            )
            server.add_tool(wrapped, name=tname, description=desc)

    if include_resources:
        register_resources(server)
    if include_prompts:
        register_prompts(server)
    return server
