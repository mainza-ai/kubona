"""Gateway auth boundary tests (OAuth 2.1-shaped)."""

import pytest

from kubona.mcp.auth import AuthError, DevAuthProvider
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
from kubona.mcp.servers._mount import build_gateway


def test_gateway_requires_token():
    gw = build_gateway()
    with pytest.raises(AuthError):
        gw.call("compiler", "transpile_circuit", {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];"})
    with pytest.raises(AuthError):
        gw.call("compiler", "transpile_circuit", {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];"}, token="wrong")


def test_gateway_accepts_dev_token_and_audits():
    gw = build_gateway()
    out = gw.call("compiler", "transpile_circuit", {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];"}, token="dev-token")
    assert out["ok"] is True
    assert len(gw.audit_log()) == 1
    assert gw.audit_log()[0]["sub"] == "researcher@kubona.local"


def test_oauth2_real_issuer_contract():
    """The production OAuth2 issuer (the shipped auth, not a stub) verifies:
    correct tokens pass, tampered/expired/wrong-audience tokens are rejected."""
    iss = OAuth2TokenIssuer()
    prov = OAuth2AccessTokenProvider(iss)
    token = iss.issue("agent-1", role="agent", scopes=("execution:submit",))
    p = prov.verify(token)
    assert p.sub == "agent-1" and p.role == "agent" and p.scopes == ("execution:submit",)
    with pytest.raises(AuthError):
        prov.verify(token + "x")          # tampered
    with pytest.raises(AuthError):
        prov.verify(iss.issue("a", role="agent", scopes=("*",), ttl_s=-1))  # expired
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    other = OAuth2TokenIssuer(signing_key=Ed25519PrivateKey.generate())
    with pytest.raises(AuthError):
        prov.verify(other.issue("a", role="agent", scopes=("*",)))  # wrong key
    other_idp = OAuth2TokenIssuer(issuer="https://foreign.idp")
    with pytest.raises(AuthError):
        prov.verify(other_idp.issue("a", role="agent", scopes=("*",)))  # wrong issuer


def test_dev_auth_rejects_bad_token():
    with pytest.raises(AuthError):
        DevAuthProvider().verify("nope")


def test_gateway_binds_principal_tenant_server_side(tmp_path):
    """Phase-1: the gateway injects the verified principal's tenant into
    tool args — an omitted tenant binds to it, and the audit records it."""
    from kubona.control import LoopSupervisor
    import kubona.mcp.servers.control as control
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    control._supervisor = LoopSupervisor(path=tmp_path / "loops.jsonl")
    token = iss.issue("op", role="user",
                      scopes=("control:read", "control:manage", "control:override"),
                      tenant="lab-a")
    out = gw.call("control", "register_loop",
                  {"name": "bound", "session_id": "s"}, token=token)
    assert out["ok"] is True and out["tenant"] == "lab-a"
    entry = gw.audit_log()[-1]
    assert entry["sub"] == "op" and entry["tenant"] == "lab-a"


def test_gateway_spoofed_tenant_refuses():
    """A caller-supplied tenant that differs from the principal's tenant
    refuses with the named cross-tenant error (fail-closed)."""
    iss = OAuth2TokenIssuer()
    gw = build_gateway(auth=OAuth2AccessTokenProvider(iss))
    token = iss.issue("op", role="user",
                      scopes=("control:read", "control:manage", "control:override"),
                      tenant="lab-a")
    out = gw.call("control", "register_loop",
                  {"name": "x", "session_id": "s", "tenant": "lab-b"}, token=token)
    assert out["ok"] is False
    assert "cross-tenant" in out["error"]

    from kubona.security.tenant import CrossTenantRefusal
    with pytest.raises(CrossTenantRefusal):
        from kubona.security.tenant import resolve_tenant
        resolve_tenant(supplied="lab-b", caller={"sub": "op", "tenant": "lab-a"})
