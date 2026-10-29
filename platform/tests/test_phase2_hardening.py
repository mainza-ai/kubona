"""Phase-2 production hardening: physical ledger sharding, strict slugs,
first-class Principal tenant, ownership gates on qubo/close/recall.

- Ledger: per-tenant shard files via the registry (`Ledger.for_tenant` /
  `shard_path_for`); the default file holds only unscoped/public rows for
  new writes. Pre-sharding `tenant/<T>` rows in the default file stay
  readable through the back-compat path (`iter_as`/`query_as` union the
  default file with the requester's shard). Cross-tenant shard reads
  refuse outright. Non-canonical tenants refuse (no `_safe` collapsing).
- Principal: `tenant` is a first-class field constructed at authentication
  time (not attached at decode).
- Ownership: `submit_qubo_job` / `close_session` / `request_provider_recall`
  enforce the same server-side ownership gate as the fixed tools (owner /
  tenant derived from the record; unknown-vs-unowned indistinguishable;
  gate before side effects).
"""

import pytest

from kubona.core.auth import DevAuthProvider, Principal

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
QUBO = [[-1.0, 2.0], [0.0, -1.0]]

EXEC_SCOPES = ("compiler:read", "execution:submit", "execution:read",
               "execution:override", "cost:read", "cost:manage",
               "providers:read")
AUTHZ_SCOPES = EXEC_SCOPES + ("provenance:read", "provenance:write",
                              "control:read", "control:manage")


@pytest.fixture()
def stack():
    from kubona.mcp.servers._mount import build_gateway
    from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    alice = iss.issue("alice", role="user", scopes=AUTHZ_SCOPES, tenant="lab-a")
    bob = iss.issue("bob", role="user", scopes=AUTHZ_SCOPES, tenant="lab-b")
    return gw, alice, bob


# -- ledger sharding ----------------------------------------------------

def test_shard_write_isolation_per_tenant(tmp_path):
    from kubona.provenance.ledger import Ledger
    base = tmp_path / "ledger.jsonl"
    a = Ledger(path=base, tenant="lab-a")
    b = Ledger(path=base, tenant="lab-b")
    assert a.path != b.path
    assert a.path.parent == b.path.parent == base.parent
    ra = a.append("experiment", {"circuit_hash": "aaa"})
    rb = b.append("experiment", {"circuit_hash": "bbb"})
    assert ra.label == "tenant/lab-a" and rb.label == "tenant/lab-b"
    assert [r.record_id for r in a] == [ra.record_id]
    assert [r.record_id for r in b] == [rb.record_id]
    # Default file holds no tenant rows for new writes.
    default = Ledger(path=base)
    assert all(r.label in ("public", "") or r.label == "tenant/default"
               for r in default)


def test_shard_registry_and_strict_refusal(tmp_path):
    from kubona.provenance.ledger import Ledger, shard_path_for
    base = tmp_path / "ledger.jsonl"
    assert Ledger.for_tenant("lab-a", path=base) is Ledger.for_tenant("lab-a", path=base)
    assert Ledger.for_tenant("lab-a", path=base).path == shard_path_for(base, "lab-a")
    for bad in (" lab-a ", "lab-a ", "a/b", "a b", "", "...", "x" * 65):
        with pytest.raises(ValueError):
            Ledger(path=base, tenant=bad)
        with pytest.raises(ValueError):
            shard_path_for(base, bad)


def test_append_hostile_label_refuses(tmp_path):
    from kubona.provenance.ledger import Ledger
    led = Ledger(path=tmp_path / "ledger.jsonl")
    with pytest.raises(ValueError):
        led.append("experiment", {"circuit_hash": "x"}, label="tenant/a b")
    with pytest.raises(ValueError):
        led.append("experiment", {"circuit_hash": "x"}, label="tenant/../etc")


def test_backcompat_read_over_default_and_shard(tmp_path):
    """Pre-sharding `tenant/<T>` rows in the default file stay readable to
    their tenant (and invisible to others), alongside new shard writes."""
    from kubona.provenance.ledger import Ledger
    base = tmp_path / "ledger.jsonl"
    default = Ledger(path=base)
    # Legacy row: written pre-sharding straight into the default file.
    legacy = default.append("experiment", {"circuit_hash": "legacy-a"},
                            label="tenant/lab-a")
    # New write lands in the shard.
    shard = Ledger.for_tenant("lab-a", path=base)
    fresh = shard.append("experiment", {"circuit_hash": "fresh-a"})
    assert shard.path != default.path
    got_a = {r.record_id for r in default.query_as("lab-a")}
    assert legacy.record_id in got_a and fresh.record_id in got_a
    got_b = {r.record_id for r in default.query_as("lab-b")}
    assert legacy.record_id not in got_b and fresh.record_id not in got_b
    # Ambient sees neither.
    got_anon = {r.record_id for r in default.query_as(None)}
    assert legacy.record_id not in got_anon and fresh.record_id not in got_anon


def test_cross_tenant_shard_read_refuses(tmp_path):
    from kubona.provenance.ledger import Ledger
    base = tmp_path / "ledger.jsonl"
    shard_b = Ledger.for_tenant("lab-b", path=base)
    shard_b.append("experiment", {"circuit_hash": "secret-b"})
    with pytest.raises(PermissionError):
        list(shard_b.iter_as("lab-a"))
    with pytest.raises(PermissionError):
        list(shard_b.iter_as(None))
    with pytest.raises(PermissionError):
        shard_b.query_as("lab-a")
    # Owner reads fine.
    assert len(shard_b.query_as("lab-b")) == 1


# -- principal ----------------------------------------------------------

def test_tenant_is_first_class_principal_field():
    assert "tenant" in Principal.__dataclass_fields__
    assert Principal(sub="x").tenant == "default"
    assert DevAuthProvider().verify("dev-token").tenant == "default"


def test_oauth_decode_constructs_tenant_at_auth_time():
    from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
    iss = OAuth2TokenIssuer()
    good = iss.issue("u", scopes=("cost:read",), tenant="lab-a")
    principal = OAuth2AccessTokenProvider(iss).verify(good)
    assert isinstance(principal, Principal)
    assert principal.tenant == "lab-a"
    # No post-construction attachment: a frozen Principal carries the claim
    # from construction; tampered payloads still refuse at verify.
    with pytest.raises(Exception):
        iss.issue("u", scopes=("cost:read",), tenant=" nope ")


# -- ownership ----------------------------------------------------------

def test_submit_qubo_with_unowned_session_refuses_like_unknown(stack):
    gw, alice, bob = stack
    made = gw.call("execution", "submit_job",
                   {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 32},
                   token=alice)
    assert made["ok"] is True
    sid = made["session_id"]
    denied = gw.call("execution", "submit_qubo_job",
                     {"qubo_matrix": QUBO, "session_id": sid}, token=bob)
    ghost = gw.call("execution", "submit_qubo_job",
                    {"qubo_matrix": QUBO, "session_id": "f" * 16}, token=bob)
    assert denied == {"ok": False, "error": f"unknown session_id {sid!r}"}
    assert ghost == {"ok": False, "error": "unknown session_id 'ffffffffffffffff'"}


def test_submit_qubo_implicit_session_owned_by_caller(stack):
    from kubona.mcp.servers import execution as exec_server
    gw, alice, _ = stack
    made = gw.call("execution", "submit_qubo_job", {"qubo_matrix": QUBO}, token=alice)
    assert made["ok"] is True, made
    state = exec_server._sessions.get(made["session_id"])
    assert state.owner == "alice"
    assert (state.meta or {}).get("tenant") == "lab-a"


def test_close_session_unowned_refuses_without_side_effects(stack):
    gw, alice, bob = stack
    made = gw.call("execution", "submit_job",
                   {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 32},
                   token=alice)
    assert made["ok"] is True
    sid = made["session_id"]
    denied = gw.call("execution", "close_session", {"session_id": sid}, token=bob)
    ghost = gw.call("execution", "close_session", {"session_id": "f" * 16}, token=bob)
    assert denied == {"ok": False, "error": f"unknown session_id {sid!r}"}
    assert ghost == {"ok": False, "error": "unknown session_id 'ffffffffffffffff'"}
    # Untouched: the owner still sees it open.
    live = gw.call("execution", "session_status", {"session_id": sid}, token=alice)
    assert live["ok"] is True and live["status"] == "open"
    assert gw.call("execution", "close_session", {"session_id": sid}, token=alice)["ok"] is True


def test_request_provider_recall_unowned_refuses_without_side_effects(stack):
    from kubona.mcp.servers import execution as exec_server
    gw, alice, bob = stack
    made = gw.call("execution", "submit_job",
                   {"circuit_qasm": BELL, "provider": "local-simulator", "shots": 32},
                   token=alice)
    assert made["ok"] is True
    alice_caller = {"sub": "alice", "tenant": "lab-a"}
    bob_caller = {"sub": "bob", "tenant": "lab-b"}
    # Queued job with no hardware id: unowned recall refuses, fence untouched.
    job = exec_server._runner.store.create(
        provider="local-simulator", session_id=made["session_id"],
        circuit_hash="recall-drill", estimate={"estimated_usd": 0.0})
    denied = exec_server.request_provider_recall(job.job_id, caller=bob_caller)
    ghost = exec_server.request_provider_recall("f" * 16, caller=bob_caller)
    assert denied == {"ok": False, "job_id": job.job_id, "error": "unknown job_id"}
    assert ghost == {"ok": False, "job_id": "f" * 16, "error": "unknown job_id"}
    assert exec_server._runner.store.get(job.job_id).status == "queued"
    # Owner proceeds (fence applies, no hardware id to recall).
    out = exec_server.request_provider_recall(job.job_id, caller=alice_caller)
    assert out["ok"] is True and out["status"] == "cancelled"
    assert out["provider_recall_attempted"] is False
