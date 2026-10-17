"""MCP control-plane tenant isolation: labels route honest callers;
the gateway capability gate authorizes. Cross-tenant reads and
overrides refuse; listings never leak other tenants' loops."""

import kubona.mcp.servers.control as control
from kubona.control import LoopSupervisor


def _isolated():
    control._supervisor = LoopSupervisor()
    return control._supervisor


def test_cross_tenant_override_refused():
    _isolated()
    mine = control.register_loop("a", "s1", tenant="lab-a")["loop_id"]
    theirs = control.register_loop("b", "s2", tenant="lab-b")["loop_id"]

    assert control.pause_loop(theirs, requester="lab-a")["ok"] is False
    assert control.resume_loop(theirs, requester="lab-a")["ok"] is False
    assert control.kill_loop(theirs, requester="lab-a")["ok"] is False
    assert control.loop_status(theirs, requester="lab-a")["ok"] is False

    # Own tenant works; possession step works without requester.
    assert control.pause_loop(mine, requester="lab-a")["ok"] is True
    assert control.step_loop(mine, "x")["ok"] is True
    assert control.kill_loop(theirs, requester="lab-b")["status"] == "killed"


def test_listing_never_leaks():
    _isolated()
    control.register_loop("a", "s1", tenant="lab-a")
    control.register_loop("b", "s2", tenant="lab-b")

    names_a = [l["name"] for l in control.list_loops(requester="lab-a")["loops"]]
    names_b = [l["name"] for l in control.list_loops(requester="lab-b")["loops"]]
    assert names_a == ["a"]
    assert names_b == ["b"]
    # Defaults stay on the default tenant (existing callers unaffected).
    control.register_loop("c", "s3")
    assert [l["name"] for l in control.list_loops()["loops"]] == ["c"]


def test_unknown_loop_still_404s():
    _isolated()
    assert control.loop_status("nope", requester="lab-a") == {
        "ok": False, "error": "unknown loop_id 'nope'"}
