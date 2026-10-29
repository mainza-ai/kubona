"""Phase-2 cross-tenant grants (Python rail).

Contract (mirrored in platform/core/pkg/tenants/grants_test.go — same
allow/deny matrix, same fixed vector, byte-identical canonical payloads):

    issue -> use allows | no grant denies (default deny) | revoke -> reuse
    denies | expired denies | out-of-scope denies | foreign-grantor forgery
    refuses | revoked-grant reuse refuses | wildcard scope refuses at ISSUE.
"""

from datetime import datetime, timedelta, timezone

import pytest

import sqlite3

from kubona.mcp.servers._mount import build_gateway
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
from kubona.security.tenant import (
    CrossTenantRefusal,
    GrantStore,
    grant_canonical_payload,
    grant_id_for,
    reset_grants,
    resolve_tenant,
    revoke_canonical_payload,
)

# Fixed cross-language vector (same bytes as the Go suite: identical seed,
# payload, signature, and grant id verify on both rails).
VECTOR_SEED = "9d61b19deffd5a60ba844af492ec2af44449c5697b326919703bac031cae7f60"
VECTOR_PUB = "b9ce0f24f866c0fa02ab7c1ff345f9bb7c73205b4c2ec76a77295ef16592dc0d"
VECTOR_SIG = ("e7c204b282be93c2e7727dd71a0d96b9b96564c5b87f15eaf28385a7b31b584"
              "dffd3c46d945e8613747883074018bef665a36449b0555fa667d8547ae21abc08")
VECTOR_ID = ("8014c4d29d1218b5000be9fb8b1941b81a869b13918620b5beb14fe1c5989130")
VECTOR_REVOKE_SIG = "a414c227aa1fbea0aa19d4f0aff92c56c5d53a4304ff6293982f422880850c179f207481834f5c8a5ebca424ac634edf4bc613d9c326218cfb9d72926d6d3906"

EXEC_SCOPES = ("compiler:read", "execution:submit", "execution:read",
               "execution:override", "cost:read", "cost:manage",
               "providers:read")
AUTHZ_SCOPES = EXEC_SCOPES + ("provenance:read", "provenance:write",
                              "control:read", "control:manage")


def _keypair():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    priv = Ed25519PrivateKey.generate()
    pubhex = priv.public_key().public_bytes_raw().hex()
    return priv, pubhex


def _sign(priv, canonical: bytes) -> str:
    return priv.sign(canonical).hex()


def _store_with_labs():
    """GrantStore with lab-owner + lab-guest admins registered; returns
    (store, owner_priv, owner_pub, guest_priv, guest_pub)."""
    st = GrantStore()
    o_priv, o_pub = _keypair()
    g_priv, g_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)
    st.register_admin_key("lab-guest", g_pub)
    return st, o_priv, o_pub, g_priv, g_pub


def _issue(st, priv, pub, source="lab-owner", target="lab-guest",
           scopes=("read",), hours=1):
    exp = datetime.now(timezone.utc) + timedelta(hours=hours)
    canon = grant_canonical_payload(source, target, scopes, exp)
    return st.issue_grant(source, target, scopes, exp, pub, _sign(priv, canon))


@pytest.fixture()
def clean_registry():
    reset_grants()
    yield
    reset_grants()


# -- fixed cross-language vector -------------------------------------------


def test_grant_vector_matches_go_rail():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import (
        Ed25519PrivateKey, Ed25519PublicKey)
    priv = Ed25519PrivateKey.from_private_bytes(bytes.fromhex(VECTOR_SEED))
    assert priv.public_key().public_bytes_raw().hex() == VECTOR_PUB
    exp = datetime(2030, 1, 1, tzinfo=timezone.utc)
    canon = grant_canonical_payload("lab-owner", "lab-guest", ("read",), exp)
    assert canon == b"grant|lab-owner|lab-guest|read|2030-01-01T00:00:00Z"
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(VECTOR_PUB)).verify(
        bytes.fromhex(VECTOR_SIG), canon)
    assert grant_id_for(canon, bytes.fromhex(VECTOR_SIG)) == VECTOR_ID
    Ed25519PublicKey.from_public_bytes(bytes.fromhex(VECTOR_PUB)).verify(
        bytes.fromhex(VECTOR_REVOKE_SIG),
        revoke_canonical_payload(VECTOR_ID, "drill"))


# -- contract matrix --------------------------------------------------------


def test_issue_use_revoke_matrix(clean_registry):
    from kubona.security.tenant import get_grant_store
    st = get_grant_store()
    o_priv, o_pub = _keypair()
    _, g_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)
    st.register_admin_key("lab-guest", g_pub)

    # No grant = no access (default deny preserved).
    assert not st.grant_allows("lab-owner", "lab-guest", "read")

    # Issue -> use allows.
    g = _issue(st, o_priv, o_pub)
    assert g.source_tenant == "lab-owner" and g.target_tenant == "lab-guest"
    assert st.grant_allows("lab-owner", "lab-guest", "read")
    # Out-of-scope refuses (grant covers read, not write).
    assert not st.grant_allows("lab-owner", "lab-guest", "write")
    # Wrong grantee refuses; reversed direction refuses.
    assert not st.grant_allows("lab-owner", "lab-stranger", "read")
    assert not st.grant_allows("lab-guest", "lab-owner", "read")

    # Revoke -> reuse refuses (immediately, next check).
    st.revoke_grant(g.grant_id, "drill", o_pub,
                    _sign(o_priv, revoke_canonical_payload(g.grant_id, "drill")))
    assert not st.grant_allows("lab-owner", "lab-guest", "read")

    # Fresh grant after revoke allows again (fresh bytes, new expiry).
    _issue(st, o_priv, o_pub, hours=2)
    assert st.grant_allows("lab-owner", "lab-guest", "read")


def test_revocation_is_sticky_against_replay(clean_registry):
    from kubona.security.tenant import get_grant_store
    st = get_grant_store()
    o_priv, o_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)
    g = _issue(st, o_priv, o_pub)
    st.revoke_grant(g.grant_id, "done", o_pub,
                    _sign(o_priv, revoke_canonical_payload(g.grant_id, "done")))
    # Re-presenting the identical blob stays dead.
    exp = g.expiry
    canon = grant_canonical_payload("lab-owner", "lab-guest", ("read",), exp)
    replayed = st.issue_grant("lab-owner", "lab-guest", ("read",), exp, o_pub,
                              _sign(o_priv, canon))
    assert replayed.revoked
    assert not st.grant_allows("lab-owner", "lab-guest", "read")


def test_expiry_enforced(clean_registry):
    from kubona.security.tenant import get_grant_store
    st = get_grant_store()
    o_priv, o_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)

    # Issuing an already-expired grant refuses at issue time.
    past = datetime.now(timezone.utc) - timedelta(minutes=1)
    canon = grant_canonical_payload("lab-owner", "lab-guest", ("read",), past)
    with pytest.raises(CrossTenantRefusal):
        st.issue_grant("lab-owner", "lab-guest", ("read",), past, o_pub,
                       _sign(o_priv, canon))

    # A grant that lapses stops working (live re-check, no TTL cache).
    g = _issue(st, o_priv, o_pub, hours=1)
    assert st.grant_allows("lab-owner", "lab-guest", "read")
    g.expiry = datetime.now(timezone.utc) - timedelta(seconds=1)
    assert not st.grant_allows("lab-owner", "lab-guest", "read")


def test_foreign_grantor_forgery_refuses():
    st, _, _, _, _ = _store_with_labs()
    evil_priv, evil_pub = _keypair()
    exp = datetime.now(timezone.utc) + timedelta(hours=1)
    canon = grant_canonical_payload("lab-owner", "lab-guest", ("read",), exp)
    # Self-signed foreign grantor refuses at issue (not the source admin).
    with pytest.raises(CrossTenantRefusal, match="not the .* admin|forgery"):
        st.issue_grant("lab-owner", "lab-guest", ("read",), exp,
                       evil_pub, _sign(evil_priv, canon))
    # Even a planted row fails the live check (signature re-verified
    # against the CURRENT source admin key on every use).
    from kubona.security.tenant import Grant
    sig = evil_priv.sign(canon).hex()
    st._grants["planted"] = Grant(
        grant_id="planted", source_tenant="lab-owner",
        target_tenant="lab-guest", scopes=("read",), expiry=exp,
        grantor_pubkey_hex=evil_pub,         signature_hex=sig)
    assert not st.grant_allows("lab-owner", "lab-guest", "read")


def test_wildcard_scope_refused_at_issue():
    st, o_priv, o_pub, _, _ = _store_with_labs()
    exp = datetime.now(timezone.utc) + timedelta(hours=1)
    for scopes in [("*",), ("read", "*"), ("read:*"), ("*write",)]:
        canon = grant_canonical_payload("lab-owner", "lab-guest", scopes, exp)
        with pytest.raises(CrossTenantRefusal, match="wildcard"):
            st.issue_grant("lab-owner", "lab-guest", scopes, exp, o_pub,
                           _sign(o_priv, canon))
    # Empty scope list refuses too.
    with pytest.raises(CrossTenantRefusal):
        st.issue_grant("lab-owner", "lab-guest", (), exp, o_pub, "00" * 64)


def test_revoke_authorization():
    st, o_priv, o_pub, g_priv, g_pub = _store_with_labs()
    g = _issue(st, o_priv, o_pub)
    # Only the issuing lab revokes: the grantee's key refuses.
    with pytest.raises(CrossTenantRefusal):
        st.revoke_grant(g.grant_id, "spoof", g_pub,
                        _sign(g_priv, revoke_canonical_payload(g.grant_id, "spoof")))
    # Unknown grant refuses (no silent success).
    with pytest.raises(CrossTenantRefusal):
        st.revoke_grant("no-such-grant", "x", o_pub, "00" * 64)
    assert st.grant_allows("lab-owner", "lab-guest", "read")


def test_principal_bound_grants():
    st, o_priv, o_pub, _, _ = _store_with_labs()
    g = _issue(st, o_priv, o_pub, target="sub:alice")
    assert g.target_tenant == "sub:alice"
    assert st.grant_allows("lab-owner", "lab-guest", "read", grantee_sub="alice")
    assert not st.grant_allows("lab-owner", "lab-guest", "read", grantee_sub="bob")
    assert not st.grant_allows("lab-owner", "lab-guest", "read")


def test_rotation_invalidates_outstanding_grants():
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    st, o_priv, o_pub, _, _ = _store_with_labs()
    _issue(st, o_priv, o_pub)
    assert st.grant_allows("lab-owner", "lab-guest", "read")
    # Rotate the source admin key (authorized by the current key).
    new_priv = Ed25519PrivateKey.generate()
    new_pub = new_priv.public_key().public_bytes_raw().hex()
    st.rotate_admin_key("lab-owner", new_pub,
                        _sign(o_priv, f"adminkey-rotation|lab-owner|{new_pub}".encode()))
    # Outstanding grants fail the live check (verified vs CURRENT key).
    assert not st.grant_allows("lab-owner", "lab-guest", "read")


# -- resolve_tenant matrix --------------------------------------------------


def test_resolve_tenant_grant_matrix(clean_registry):
    from kubona.security.tenant import get_grant_store
    st = get_grant_store()
    o_priv, o_pub = _keypair()
    _, g_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)
    st.register_admin_key("lab-guest", g_pub)

    alice = {"sub": "alice", "tenant": "lab-guest"}
    # Same tenant binds; omitted supply binds; direct calls unchanged.
    assert resolve_tenant(supplied=None, caller=alice) == "lab-guest"
    assert resolve_tenant(supplied="lab-guest", caller=alice) == "lab-guest"
    assert resolve_tenant(supplied="lab-guest", caller=None) == "lab-guest"
    assert resolve_tenant(supplied=None, caller=None) == "default"
    # Cross-tenant without a grant refuses (default deny).
    with pytest.raises(CrossTenantRefusal, match="no live cross-tenant grant"):
        resolve_tenant(supplied="lab-owner", caller=alice)
    # Cross-tenant with a live grant allows (source = requested tenant).
    _issue(st, o_priv, o_pub)
    assert resolve_tenant(supplied="lab-owner", caller=alice) == "lab-owner"
    # Out-of-scope grant does not authorize the read.
    reset_grants()
    st.register_admin_key("lab-owner", o_pub)
    st.register_admin_key("lab-guest", g_pub)
    _issue(st, o_priv, o_pub, scopes=("write",))
    with pytest.raises(CrossTenantRefusal):
        resolve_tenant(supplied="lab-owner", caller=alice)


def test_resolve_tenant_principal_grant(clean_registry):
    from kubona.security.tenant import get_grant_store
    st = get_grant_store()
    o_priv, o_pub = _keypair()
    st.register_admin_key("lab-owner", o_pub)
    _issue(st, o_priv, o_pub, target="sub:alice")
    assert resolve_tenant(supplied="lab-owner",
                          caller={"sub": "alice", "tenant": "lab-guest"}) == "lab-owner"
    with pytest.raises(CrossTenantRefusal):
        resolve_tenant(supplied="lab-owner",
                       caller={"sub": "bob", "tenant": "lab-guest"})


# -- gateway path: grants consulted on cross-tenant calls -------------------


def _gateway_stack():
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    alice = iss.issue("alice", role="user", scopes=AUTHZ_SCOPES, tenant="lab-a")
    bob = iss.issue("bob", role="user", scopes=AUTHZ_SCOPES, tenant="lab-b")
    return gw, alice, bob


def test_gateway_cross_tenant_refuses_without_grant(clean_registry):
    gw, alice, _ = _gateway_stack()
    out = gw.call("control", "register_loop",
                  {"name": "x", "session_id": "s", "tenant": "lab-b"},
                  token=alice)
    assert out["ok"] is False and "cross-tenant" in out["error"]


def test_gateway_cross_tenant_allows_with_grant(clean_registry, tmp_path):
    from kubona.control import LoopSupervisor
    import kubona.mcp.servers.control as control
    from kubona.security.tenant import get_grant_store
    control._supervisor = LoopSupervisor(path=tmp_path / "loops.jsonl")

    gw, alice, bob = _gateway_stack()
    st = get_grant_store()
    # Admin keys for both labs (Python registry is process-local).
    b_priv, b_pub = _keypair()
    _, a_pub = _keypair()
    st.register_admin_key("lab-b", b_pub)
    st.register_admin_key("lab-a", a_pub)
    # lab-b grants lab-a the read scope: alice (lab-a) may act in lab-b.
    exp = datetime.now(timezone.utc) + timedelta(hours=1)
    canon = grant_canonical_payload("lab-b", "lab-a", ("read",), exp)
    st.issue_grant("lab-b", "lab-a", ("read",), exp, b_pub,
                   _sign(b_priv, canon))

    out = gw.call("control", "register_loop",
                  {"name": "granted", "session_id": "s", "tenant": "lab-b"},
                  token=alice)
    assert out["ok"] is True and out["tenant"] == "lab-b"
    listed = gw.call("control", "list_loops", {"requester": "lab-b"},
                     token=alice)
    assert listed["ok"] is True
    assert any(loop["loop_id"] == out["loop_id"] for loop in listed["loops"])
    # Bob (lab-b) sees his own lab without any grant (same-tenant path).
    mine = gw.call("control", "list_loops", {}, token=bob)
    assert mine["ok"] is True


def test_gateway_grant_revoked_takes_effect_immediately(clean_registry, tmp_path):
    from kubona.control import LoopSupervisor
    import kubona.mcp.servers.control as control
    from kubona.security.tenant import get_grant_store
    control._supervisor = LoopSupervisor(path=tmp_path / "loops.jsonl")

    gw, alice, _ = _gateway_stack()
    st = get_grant_store()
    b_priv, b_pub = _keypair()
    _, a_pub = _keypair()
    st.register_admin_key("lab-b", b_pub)
    st.register_admin_key("lab-a", a_pub)
    exp = datetime.now(timezone.utc) + timedelta(hours=1)
    canon = grant_canonical_payload("lab-b", "lab-a", ("read",), exp)
    g = st.issue_grant("lab-b", "lab-a", ("read",), exp, b_pub,
                       _sign(b_priv, canon))
    ok = gw.call("control", "list_loops", {"requester": "lab-b"}, token=alice)
    assert ok["ok"] is True
    st.revoke_grant(g.grant_id, "done", b_pub,
                    _sign(b_priv, revoke_canonical_payload(g.grant_id, "done")))
    denied = gw.call("control", "list_loops", {"requester": "lab-b"}, token=alice)
    assert denied["ok"] is False and "cross-tenant" in denied["error"]


# -- Go-registry lookaside (single-registry rule) ---------------------------
#
# The Go tenants SQLite DB is the durable system of record for HTTP-issued
# grants. attach_go_registry() lets Python enforcement verify exactly those
# rows read-only. These tests build a minimal Go-shaped DB (tenants +
# tenant_grants tables) and pin the allow/deny matrix against it.


def _go_db_with_grant(path, owner_pub, scopes=("read",), hours=1,
                      revoked=0, expiry_text=None, target="lab-guest",
                      signer_priv=None, admin_pub=None):
    """Seed a Go-shaped registry DB. Returns (grant_id, canonical)."""
    from datetime import timezone as _tz
    con = sqlite3.connect(str(path))
    con.execute("CREATE TABLE tenants (id TEXT PRIMARY KEY, admin_pubkey_hex TEXT NOT NULL DEFAULT '')")
    con.execute("""CREATE TABLE tenant_grants (
        grant_id TEXT PRIMARY KEY, source_tenant TEXT NOT NULL,
        target_tenant TEXT NOT NULL, scopes_csv TEXT NOT NULL,
        expiry DATETIME NOT NULL, grantor_pubkey_hex TEXT NOT NULL,
        signature_hex TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0,
        created_at DATETIME)""")
    con.execute("INSERT INTO tenants (id, admin_pubkey_hex) VALUES (?, ?)",
                ("lab-owner", admin_pub or owner_pub))
    exp = datetime.now(_tz.utc) + timedelta(hours=hours)
    canon = grant_canonical_payload("lab-owner", target, scopes, exp)
    sig = _sign(signer_priv, canon) if signer_priv is not None else "00" * 64
    from kubona.security.tenant import grant_id_for as _gid
    gid = _gid(canon, bytes.fromhex(sig)) if signer_priv is not None else "deadbeef"
    exp_text = expiry_text if expiry_text is not None else exp.strftime("%Y-%m-%d %H:%M:%S.%f+00:00")
    con.execute("INSERT INTO tenant_grants VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (gid, "lab-owner", target, ",".join(scopes), exp_text,
                 owner_pub, sig, revoked, exp_text))
    con.commit()
    con.close()
    return gid, canon


def test_go_registry_allows_live_grant(tmp_path):
    from kubona.security.tenant import GrantStore
    st = GrantStore()
    o_priv, o_pub = _keypair()
    db = tmp_path / "kubona.db"
    _go_db_with_grant(db, o_pub, signer_priv=o_priv)
    st.attach_go_registry(db)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is True


def test_go_registry_fail_closed_matrix(tmp_path):
    from kubona.security.tenant import GrantStore
    st = GrantStore()
    o_priv, o_pub = _keypair()
    evil_priv, _evil_pub = _keypair()
    # revoked row denies
    db = tmp_path / "a.db"
    _go_db_with_grant(db, o_pub, signer_priv=o_priv, revoked=1)
    st.attach_go_registry(db)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # out-of-scope denies
    db2 = tmp_path / "b.db"
    _go_db_with_grant(db2, o_pub, signer_priv=o_priv, scopes=("control:read",))
    st.attach_go_registry(db2)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # expired denies
    db3 = tmp_path / "c.db"
    _go_db_with_grant(db3, o_pub, signer_priv=o_priv, hours=-1)
    st.attach_go_registry(db3)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # rotated admin: row signed by the OLD key denies against the new row key
    db4 = tmp_path / "d.db"
    _new_priv, new_pub = _keypair()
    _go_db_with_grant(db4, o_pub, signer_priv=o_priv, admin_pub=new_pub)
    st.attach_go_registry(db4)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # foreign grantor (valid sig, wrong key) denies
    db5 = tmp_path / "e.db"
    _go_db_with_grant(db5, o_pub, signer_priv=evil_priv)
    st.attach_go_registry(db5)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # malformed expiry denies
    db6 = tmp_path / "f.db"
    _go_db_with_grant(db6, o_pub, signer_priv=o_priv, expiry_text="not-a-time")
    st.attach_go_registry(db6)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # missing file denies (fail-closed while attached)
    st.attach_go_registry(tmp_path / "nope.db")
    assert st.grant_allows("lab-owner", "lab-guest", "read") is False
    # unknown source denies
    st.attach_go_registry(db)
    assert st.grant_allows("lab-ghost", "lab-guest", "read") is False


def test_go_registry_nanosecond_expiry_parses(tmp_path):
    """Go stores nanosecond fractions; the reader truncates, not refuses."""
    from kubona.security.tenant import GrantStore
    st = GrantStore()
    o_priv, o_pub = _keypair()
    db = tmp_path / "n.db"
    exp = datetime.now(timezone.utc) + timedelta(hours=1)
    nano = exp.strftime("%Y-%m-%d %H:%M:%S.123456789+00:00")
    _go_db_with_grant(db, o_pub, signer_priv=o_priv, expiry_text=nano)
    st.attach_go_registry(db)
    assert st.grant_allows("lab-owner", "lab-guest", "read") is True


def test_memory_and_go_union(tmp_path):
    """In-memory grants still allow while attached (union of allows)."""
    st, o_priv, o_pub, _g_priv, _g_pub = _store_with_labs()
    _issue(st, o_priv, o_pub)
    st.attach_go_registry(tmp_path / "empty-missing.db")
    assert st.grant_allows("lab-owner", "lab-guest", "read") is True
    st.detach_go_registry()
    assert st.grant_allows("lab-owner", "lab-guest", "read") is True
