"""MRTR approval retry: request shape, accept executes once, denials refuse."""

import pytest

from kubona.mcp.approval import (
    ApprovalRegistry, build_packet, elicitation_form, open_state, seal_state,
)
from kubona.mcp.auth import AuthError
from kubona.mcp.servers._mount import build_gateway

SECRET = b"test-secret-32-bytes-long-000000"


ARGS = {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
        "provider": "local-simulator", "shots": 100}


def _builder(principal, args):
    return build_packet(
        agent_id="agent-1", server="execution", tool="submit_job",
        backend=args.get("provider", "local-simulator"), program_hash="prog:abc",
        cost_max_usd=48.0,
        budget_remaining_usd=200.0, cost_basis="price sheet v2026-10-01",
        requestor=principal.sub if principal else "anon")


def _gated():
    gw = build_gateway()
    gw.require_approval("execution", "submit_job", _builder, secret=SECRET)
    return gw


def test_first_call_returns_input_required_without_executing():
    gw = _gated()
    out = gw.call("execution", "submit_job",
                  dict(ARGS), token="dev-token")
    assert out["resultType"] == "input_required"
    form = out["inputRequests"]["approval"]
    assert form["method"] == "elicitation/create"
    assert form["params"]["mode"] == "form"
    schema = form["params"]["requestedSchema"]
    assert schema["properties"]["decision"]["enum"] == ["approve_once", "approve_up_to_max", "deny"]
    assert "requestState" in out and isinstance(out["requestState"], str)
    # nothing executed: no tool result, but the ask was audited
    assert any(e.get("phase") == "approval_requested" for e in gw.audit_log())


def test_accept_with_cost_ack_executes_exactly_once():
    gw = _gated()
    first = gw.call("execution", "submit_job",
                    dict(ARGS), token="dev-token")
    state = first["requestState"]
    data = open_state(state, SECRET)
    second = gw.call("execution", "submit_job",
                     dict(ARGS), token="dev-token",
                     approval_response={"action": "accept",
                                        "content": {"decision": "approve_once",
                                                    "cost_ack_usd": 48.0}},
                     request_state=state)
    assert second.get("resultType") != "input_required"
    # replay of the same state refuses (single-use nonce)
    with pytest.raises(AuthError):
        gw.call("execution", "submit_job",
                dict(ARGS), token="dev-token",
                approval_response={"action": "accept",
                                   "content": {"decision": "approve_once",
                                               "cost_ack_usd": 48.0}},
                request_state=state)
    assert data["cost_max_usd"] == 48.0


def test_deny_never_executes_and_returns_reason():
    gw = _gated()
    first = gw.call("execution", "submit_job", dict(ARGS),
                    token="dev-token")
    out = gw.call("execution", "submit_job", dict(ARGS),
                  token="dev-token",
                  approval_response={"action": "decline", "reason": "too pricey"},
                  request_state=first["requestState"])
    assert out["ok"] is False
    assert "too pricey" in out["rejection_message"]


def test_tampered_expired_and_mismatched_states_refuse():
    gw = _gated()
    first = gw.call("execution", "submit_job", dict(ARGS),
                    token="dev-token")
    good = {"action": "accept",
            "content": {"decision": "approve_once", "cost_ack_usd": 48.0}}
    args = dict(ARGS)
    with pytest.raises(AuthError):
        gw.call("execution", "submit_job", args, token="dev-token",
                approval_response=good, request_state=first["requestState"] + "x")
    with pytest.raises(AuthError):
        gw.call("execution", "submit_job", args, token="dev-token",
                approval_response=good, request_state=None)
    with pytest.raises(AuthError):  # wrong cost ack
        gw.call("execution", "submit_job", args, token="dev-token",
                approval_response={"action": "accept",
                                   "content": {"decision": "approve_once",
                                               "cost_ack_usd": 1.0}},
                request_state=first["requestState"])


def test_builder_none_auto_allows_and_packet_builder_errors_refuse():
    from kubona.mcp.gateway import KubonaGateway
    from kubona.mcp.auth import DevAuthProvider
    gw = KubonaGateway(auth=DevAuthProvider(), require_auth=True)
    gw.mount("execution", {"submit_job": lambda **a: {"ok": True, "simulated": True}})
    gw.require_approval("execution", "submit_job", lambda p, a: None, secret=SECRET)
    out = gw.call("execution", "submit_job", {}, token="dev-token")
    assert out == {"ok": True, "simulated": True}


def test_nonce_registry_rejects_replay_and_sweeps():
    reg = ApprovalRegistry()
    reg.consume("n1", ttl_s=60, now=1000.0)
    with pytest.raises(ValueError):
        reg.consume("n1", now=1001.0)
    reg.consume("n1", now=2000.0)  # expired entries sweep: reusable name, new TTL
    state_pkt = build_packet(agent_id="a", server="s", tool="t", backend="b",
                             program_hash="h", cost_max_usd=1.0)
    form = elicitation_form(state_pkt)
    assert form["params"]["requestedSchema"]["properties"]["cost_ack_usd"]["minimum"] == 1.0
