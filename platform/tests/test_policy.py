"""Per-call identity-aware authorization tests (Phase 3)."""

import pytest

from kubona.identity import IdentityRegistry, create_identity
from kubona.mcp.auth import AuthError, DevAuthProvider
from kubona.mcp.servers._mount import build_gateway
from kubona.security.policy import AGENT_RESEARCHER_SCOPES, PolicyEngine


def _isolated_gw(tmp_path, sub: str, owner: str, scopes):
    """Gateway with an isolated identity registry + agent auth for `sub`."""
    reg = IdentityRegistry(path=str(tmp_path / f"identity-{sub}.json"))
    pub_user, _ = create_identity()
    pub_agent, _ = create_identity()
    reg.register(owner, "user", pub_user)
    reg.register(sub, "agent", pub_agent, owner=owner)
    reg.delegate(owner, sub, scopes=scopes)
    gw = build_gateway()
    gw.policy = PolicyEngine(reg)
    gw.auth = _AgentAuth(sub)
    return gw


def test_agent_scoped_to_submit_not_approve(tmp_path):
    """An agent may submit simulator jobs but NEVER approve spend (not coarse roles)."""
    gw = _isolated_gw(tmp_path, "agent-1", "researcher", AGENT_RESEARCHER_SCOPES)

    # allowed: submit a simulator job (researcher scope includes execution:submit)
    ok = gw.call("execution", "submit_job",
                 {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
                  "provider": "local-simulator", "shots": 256, "seed": 1}, token="agent-token")
    assert ok["ok"] is True

    # denied: approving spend is not in the agent's scopes
    with pytest.raises(AuthError):
        gw.call("cost", "approve_approval", {"approval_id": "x"}, token="agent-token")

    # denied: killing loops (override scope) not granted
    with pytest.raises(AuthError):
        gw.call("control", "kill_loop", {"loop_id": "x"}, token="agent-token")


def test_revocation_takes_effect_immediately(tmp_path):
    reg = IdentityRegistry(path=str(tmp_path / "identity-revoke.json"))
    pub_user, _ = create_identity()
    pub_agent, _ = create_identity()
    reg.register("u", "user", pub_user)
    reg.register("agent-1", "agent", pub_agent, owner="u")
    reg.delegate("u", "agent-1", scopes=("execution:submit", "execution:read"))

    gw = build_gateway()
    gw.policy = PolicyEngine(reg)
    gw.auth = _AgentAuth("agent-1")

    gw.call("execution", "submit_job",
            {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
             "provider": "local-simulator", "shots": 64, "seed": 1}, token="agent-token")

    reg.revoke_all_for("u")  # owner revoked -> agent's delegation dies
    with pytest.raises(AuthError):
        gw.call("execution", "submit_job",
                {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
                 "provider": "local-simulator", "shots": 64, "seed": 1}, token="agent-token")


def test_operator_has_full_access():
    gw = build_gateway()
    # operator dev principal has full scope: the call REACHES the tool and fails on
    # the unknown approval (KeyError), NOT on authorization (AuthError).
    with pytest.raises(KeyError):
        gw.call("cost", "approve_approval", {"approval_id": "x"}, token="dev-token")


class _AgentAuth(DevAuthProvider):
    """Dev auth that authenticates as the given agent principal (scopes resolved live)."""

    def __init__(self, sub: str):
        super().__init__(sub=sub, role="agent")

    def verify(self, token):
        if token != "agent-token":
            raise AuthError("bad token")
        from kubona.mcp.auth import Principal
        return Principal(sub=self._sub, role="agent", scopes=())
