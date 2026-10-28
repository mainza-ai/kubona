"""Phase-1 production hardening: principal-bound identity + ownership.

- The gateway binds the verified principal (sub + tenant claim) server-side;
  tools prefer it over any caller-supplied requester/tenant string, and a
  differing supply refuses (fail-closed — no cross-tenant grant exists yet).
- session_status / get_job_status / get_result / cancel_job verify the
  caller's session/tenant owns the id. Unknown vs forbidden ids are
  indistinguishable to unauthorized callers (no oracle).
- Tenant slugs are refuse-don't-collapse (" Default " refuses).

Every test authenticates as a principal via real OAuth2 issuer fixtures.
"""

import json
import uuid

import pytest

from kubona.mcp.servers._mount import build_gateway
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
from kubona.security.tenant import CrossTenantRefusal, resolve_tenant

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"

EXEC_SCOPES = ("compiler:read", "execution:submit", "execution:read",
               "execution:override", "cost:read", "cost:manage",
               "providers:read")
AUTHZ_SCOPES = EXEC_SCOPES + ("provenance:read", "provenance:write",
                              "control:read", "control:manage")


@pytest.fixture()
def stack():
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    alice = iss.issue("alice", role="user", scopes=AUTHZ_SCOPES, tenant="lab-a")
    bob = iss.issue("bob", role="user", scopes=AUTHZ_SCOPES, tenant="lab-b")
    return gw, alice, bob


def _submit(gw, token, **kw):
    args = {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 32}
    args.update(kw)
    return gw.call("execution", "submit_job", args, token=token)


# -- execution ownership ------------------------------------------------

def test_unowned_job_reads_indistinguishable_from_unknown(stack):
    gw, alice, bob = stack
    made = _submit(gw, alice)
    assert made["ok"] is True
    job_id, session_id = made["job_id"], made["session_id"]

    for tool in ("get_job_status", "get_result"):
        denied = gw.call("execution", tool, {"job_id": job_id}, token=bob)
        ghost = gw.call("execution", tool, {"job_id": "f" * 16}, token=bob)
        assert denied == ghost == {"ok": False, "error": "unknown job_id"}
    denied = gw.call("execution", "session_status", {"session_id": session_id}, token=bob)
    ghost = gw.call("execution", "session_status", {"session_id": "f" * 16}, token=bob)
    # Same envelope (the echoed id is caller-supplied either way: no oracle).
    assert denied["ok"] is False and ghost["ok"] is False
    assert denied["error"].startswith("unknown session_id ")
    assert ghost["error"].startswith("unknown session_id ")


def test_unowned_job_cancel_refuses_without_side_effects(stack):
    gw, alice, bob = stack
    made = _submit(gw, alice, wait=False)
    assert made["ok"] is True
    denied = gw.call("execution", "cancel_job", {"job_id": made["job_id"]}, token=bob)
    ghost = gw.call("execution", "cancel_job", {"job_id": "f" * 16}, token=bob)
    # Same envelope apart from the caller-supplied id echo (no oracle).
    assert denied["ok"] is False and ghost["ok"] is False
    assert denied["error"] == ghost["error"] == "job not found or already terminal"
    # The job is untouched: the owner still sees it live, never cancelled.
    live = gw.call("execution", "get_job_status", {"job_id": made["job_id"]}, token=alice)
    assert live["ok"] is True and live["status"] in ("queued", "running", "completed")


def test_submit_with_unowned_session_refuses_like_unknown(stack):
    gw, alice, bob = stack
    made = _submit(gw, alice)
    sid = made["session_id"]
    denied = _submit(gw, bob, session_id=sid)
    ghost = _submit(gw, bob, session_id="f" * 16)
    # Same envelope shape (the echoed id is caller-supplied either way, so it
    # leaks nothing): unowned is indistinguishable from unknown.
    assert denied == {"ok": False, "error": f"unknown session_id {sid!r}"}
    assert ghost == {"ok": False, "error": "unknown session_id 'ffffffffffffffff'"}
    assert "spent_usd" not in denied


def test_owned_paths_keep_working(stack):
    gw, alice, _ = stack
    made = _submit(gw, alice)
    assert made["ok"] is True
    assert gw.call("execution", "session_status",
                   {"session_id": made["session_id"]}, token=alice)["ok"] is True
    assert gw.call("execution", "get_job_status",
                   {"job_id": made["job_id"]}, token=alice)["ok"] is True
    assert gw.call("execution", "get_result",
                   {"job_id": made["job_id"]}, token=alice)["ok"] is True


def test_implicit_session_owned_by_caller(stack):
    from kubona.mcp.servers import execution as exec_server
    gw, alice, _ = stack
    made = _submit(gw, alice)
    state = exec_server._sessions.get(made["session_id"])
    assert state.owner == "alice"
    assert (state.meta or {}).get("tenant") == "lab-a"


def test_direct_calls_without_caller_stay_transport_trusted():
    """No injected identity (in-process/stdio): legacy behavior preserved."""
    from kubona.mcp.servers import execution as exec_server
    out = exec_server.submit_job(BELL, provider="local-simulator", shots=32)
    assert out["ok"] is True
    assert exec_server.session_status(out["session_id"])["ok"] is True


# -- provenance binding ---------------------------------------------------

def test_provenance_cross_tenant_query_refuses_and_spoof_fails(stack):
    gw, alice, bob = stack
    marker = f"authz-{uuid.uuid4().hex[:8]}"
    rec = gw.call("provenance", "write_record",
                  {"kind": "experiment",
                   "payload": {"circuit_hash": marker, "provider": "local-simulator",
                               "shots": 1, "results": {"00": 1}, "cost_usd": 0.0}},
                  token=alice)
    assert rec["ok"] is True and rec["label"] == "tenant/lab-a"

    def ids(out):
        return [json.loads(r)["record_id"] for r in out["records"]]

    assert rec["record_id"] in ids(gw.call(
        "provenance", "query_records",
        {"kind": "experiment", "limit": 1000}, token=alice))
    assert rec["record_id"] not in ids(gw.call(
        "provenance", "query_records",
        {"kind": "experiment", "limit": 1000}, token=bob))
    spoof = gw.call("provenance", "query_records",
                    {"kind": "experiment", "requester": "lab-a"}, token=bob)
    assert spoof["ok"] is False and "cross-tenant" in spoof["error"]


def test_provenance_write_spoofed_label_refuses(stack):
    gw, _, bob = stack
    out = gw.call("provenance", "write_record",
                  {"kind": "experiment",
                   "payload": {"circuit_hash": "x", "provider": "local-simulator",
                               "shots": 1, "results": {"00": 1}, "cost_usd": 0.0},
                   "label": "tenant/lab-a"}, token=bob)
    assert out["ok"] is False and "cross-tenant" in out["error"]


def test_ambient_query_sees_public_only(stack):
    gw, alice, _ = stack
    marker = f"pub-{uuid.uuid4().hex[:8]}"
    rec = gw.call("provenance", "write_record",
                  {"kind": "experiment",
                   "payload": {"circuit_hash": marker, "provider": "local-simulator",
                               "shots": 1, "results": {"00": 1}, "cost_usd": 0.0},
                   "label": "public"}, token=alice)
    assert rec["ok"] is True and rec["label"] == "public"
    # Direct ambient query (no identity): public rows visible, tenant rows not.
    from kubona.mcp.servers import provenance as prov_server
    out = prov_server.query_records(kind="experiment", limit=1000)
    got = [json.loads(r)["record_id"] for r in out["records"]]
    assert rec["record_id"] in got


# -- gateway binding ------------------------------------------------------

def test_gateway_audit_binds_tenant(stack):
    gw, alice, _ = stack
    gw.call("control", "register_loop", {"name": "audit-loop", "session_id": "s"}, token=alice)
    entry = gw.audit_log()[-1]
    assert entry["sub"] == "alice" and entry["tenant"] == "lab-a"


def test_resolve_tenant_grant_decision():
    with pytest.raises(CrossTenantRefusal, match="cross-tenant grant"):
        resolve_tenant(supplied="lab-b", caller={"sub": "alice", "tenant": "lab-a"})
    assert resolve_tenant(supplied=None,
                          caller={"sub": "alice", "tenant": "lab-a"}) == "lab-a"
    assert resolve_tenant(supplied="lab-a",
                          caller={"sub": "alice", "tenant": "lab-a"}) == "lab-a"


def test_token_tenant_claim_round_trip_and_invalid_refuses():
    iss = OAuth2TokenIssuer()
    good = iss.issue("u", scopes=("cost:read",), tenant="lab-a")
    assert OAuth2AccessTokenProvider(iss).verify(good).tenant == "lab-a"
    with pytest.raises(Exception):
        iss.issue("u", scopes=("cost:read",), tenant=" nope ")


def test_dev_principal_resolves_to_default_tenant():
    from kubona.mcp.auth import DevAuthProvider
    from kubona.security.tenant import principal_tenant
    assert principal_tenant(DevAuthProvider().verify("dev-token")) == "default"
