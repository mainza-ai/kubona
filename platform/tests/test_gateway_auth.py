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
    from kubona.security.oauth import OAuth2TokenIssuer as I2
    other = OAuth2TokenIssuer()
    with pytest.raises(AuthError):
        prov.verify(other.issue("a", role="agent", scopes=("*",)))  # wrong key/issuer


def test_dev_auth_rejects_bad_token():
    with pytest.raises(AuthError):
        DevAuthProvider().verify("nope")
