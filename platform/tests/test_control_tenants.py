"""MCP control-plane tenant isolation: labels route honest callers;
the gateway capability gate authorizes. Cross-tenant reads and
overrides refuse; listings never leak other tenants' loops.

Phase-1: every tenant assertion below authenticates as a principal (real
OAuth2 issuer fixtures with tenant claims). Raw requester strings only
appear as spoof attempts that must refuse, or as explicit same-tenant
values that must pass.
"""

import pytest

import kubona.mcp.servers.control as control
from kubona.control import LoopSupervisor
from kubona.mcp.servers._mount import build_gateway
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer

CONTROL_SCOPES = ("control:read", "control:manage", "control:override")


@pytest.fixture()
def stack(tmp_path):
    control._supervisor = LoopSupervisor(path=tmp_path / "loops.jsonl")
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    alice = iss.issue("alice", role="user", scopes=CONTROL_SCOPES, tenant="lab-a")
    bob = iss.issue("bob", role="user", scopes=CONTROL_SCOPES, tenant="lab-b")
    return gw, alice, bob


def test_cross_tenant_override_refused(stack):
    gw, alice, bob = stack
    mine = gw.call("control", "register_loop",
                   {"name": "a", "session_id": "s1"}, token=alice)["loop_id"]
    theirs = gw.call("control", "register_loop",
                     {"name": "b", "session_id": "s2"}, token=bob)["loop_id"]

    assert gw.call("control", "pause_loop", {"loop_id": mine}, token=bob)["ok"] is False
    assert gw.call("control", "resume_loop", {"loop_id": mine}, token=bob)["ok"] is False
    assert gw.call("control", "kill_loop", {"loop_id": mine}, token=bob)["ok"] is False
    assert gw.call("control", "loop_status", {"loop_id": mine}, token=bob)["ok"] is False

    # Own tenant works; possession step works without requester.
    assert gw.call("control", "pause_loop", {"loop_id": mine}, token=alice)["ok"] is True
    assert gw.call("control", "step_loop",
                   {"loop_id": mine, "step_desc": "x"}, token=alice)["ok"] is True
    assert gw.call("control", "kill_loop",
                   {"loop_id": theirs}, token=bob)["status"] == "killed"


def test_spoofed_requester_refused_in_favor_of_principal(stack):
    """A caller-supplied requester for another tenant grants nothing: the
    injected principal identity governs, and the mismatch refuses."""
    gw, alice, bob = stack
    mine = gw.call("control", "register_loop",
                   {"name": "a", "session_id": "s1"}, token=alice)["loop_id"]
    spoof = gw.call("control", "loop_status",
                    {"loop_id": mine, "requester": "lab-a"}, token=bob)
    assert spoof["ok"] is False
    assert "cross-tenant" in spoof["error"]
    # An explicit requester that MATCHES the principal tenant passes.
    own = gw.call("control", "loop_status",
                  {"loop_id": mine, "requester": "lab-a"}, token=alice)
    assert own["ok"] is True


def test_spoofed_caller_identity_overwritten_by_gateway(stack):
    """A client-supplied `caller` is attacker-controlled: the gateway
    overwrites it with the verified principal before the tool runs."""
    gw, alice, bob = stack
    mine = gw.call("control", "register_loop",
                   {"name": "a", "session_id": "s1"}, token=alice)["loop_id"]
    out = gw.call("control", "loop_status",
                  {"loop_id": mine, "requester": "lab-b",
                   "caller": {"sub": "bob", "tenant": "lab-b"}}, token=alice)
    assert out["ok"] is False
    assert "cross-tenant" in out["error"]


def test_register_binds_principal_tenant(stack):
    """Omitting the tenant binds the principal's tenant server-side."""
    gw, alice, _ = stack
    out = gw.call("control", "register_loop",
                  {"name": "bound", "session_id": "s1"}, token=alice)
    assert out["ok"] is True
    assert out["tenant"] == "lab-a"


def test_padded_tenant_refuses(stack):
    """" Default " refuses instead of collapsing onto a neighbor tenant."""
    gw, alice, _ = stack
    # Via the gateway the padded supply mismatches the principal tenant.
    out = gw.call("control", "register_loop",
                  {"name": "pad", "session_id": "s1", "tenant": " lab-a "}, token=alice)
    assert out["ok"] is False
    # Direct (transport-trusted) callers get the strict-slug refusal.
    direct = control.register_loop("pad-direct", "s1", tenant=" lab-a ")
    assert direct["ok"] is False


def test_listing_never_leaks(stack):
    gw, alice, bob = stack
    gw.call("control", "register_loop", {"name": "a", "session_id": "s1"}, token=alice)
    gw.call("control", "register_loop", {"name": "b", "session_id": "s2"}, token=bob)

    names_a = [loop["name"] for loop in
               gw.call("control", "list_loops", {}, token=alice)["loops"]]
    names_b = [loop["name"] for loop in
               gw.call("control", "list_loops", {}, token=bob)["loops"]]
    assert names_a == ["a"]
    assert names_b == ["b"]


def test_unknown_loop_still_404s(stack):
    gw, alice, _ = stack
    out = gw.call("control", "loop_status", {"loop_id": "nope"}, token=alice)
    assert out == {"ok": False, "error": "unknown loop_id 'nope'"}


def test_absent_identity_refuses_at_supervisor(tmp_path):
    """None/empty requesters refuse instead of silently scoping to default."""
    import pytest
    sup = LoopSupervisor(path=tmp_path / "loops.jsonl")
    h = sup.register("a", "s1", 5, tenant="lab-a")
    with pytest.raises(PermissionError):
        sup.get_for(h.loop_id, None)
    with pytest.raises(PermissionError):
        sup.list_for(None)
    with pytest.raises(PermissionError):
        sup.get_for(h.loop_id, "")
    # Valid tenants still work.
    assert sup.get_for(h.loop_id, "lab-a").name == "a"
    assert [loop["name"] for loop in sup.list_for("lab-a")] == ["a"]
