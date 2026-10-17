"""W0.13: auth defaults are least-privilege and fail-closed."""

import pytest

from kubona.core.auth import DevAuthProvider, Principal
from kubona.mcp.auth import AuthError
from kubona.security.oauth import MAX_TTL_S, OAuth2TokenIssuer


def test_principal_defaults_least_privilege():
    p = Principal(sub="x")
    assert p.role == "user" and p.scopes == (), \
        "forgotten kwargs must never equal authorization"


def test_issue_requires_sub_and_scopes():
    issuer = OAuth2TokenIssuer()
    with pytest.raises(AuthError):
        issuer.issue("", scopes=("cost:read",))
    with pytest.raises(AuthError):
        issuer.issue("u", scopes=())
    with pytest.raises(AuthError):
        issuer.issue("u", scopes=("cost:read", ""))


def test_issue_ttl_clamped():
    issuer = OAuth2TokenIssuer()
    import base64
    import json
    token = issuer.issue("u", scopes=("cost:read",), ttl_s=10 * 365 * 24 * 3600)
    payload = json.loads(base64.urlsafe_b64decode(token.split(".")[1] + "=="))
    assert payload["exp"] - payload["iat"] <= MAX_TTL_S


def test_decode_enforces_header():
    import base64
    import json
    issuer = OAuth2TokenIssuer()
    token = issuer.issue("u", scopes=("cost:read",))
    h, p, s = token.split(".")
    header = {"alg": "none", "typ": "JWT"}
    forged = base64.urlsafe_b64encode(json.dumps(header).encode()).rstrip(b"=").decode()
    with pytest.raises(AuthError):
        issuer.decode(f"{forged}.{p}.{s}")


def test_dev_auth_refuses_in_production(monkeypatch):
    monkeypatch.setenv("KUBONA_ENV", "production")
    with pytest.raises(AuthError):
        DevAuthProvider().verify("dev-token")


def test_revoked_agent_denied_despite_token_scopes(tmp_path):
    from kubona.identity import IdentityRegistry
    from kubona.security.policy import PolicyEngine
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives import serialization as ser

    k = lambda: Ed25519PrivateKey.generate().public_key().public_bytes(
        ser.Encoding.Raw, ser.PublicFormat.Raw).hex()
    reg = IdentityRegistry(path=str(tmp_path / "id.json"))
    reg.register("user-1", "user", k())
    reg.register("agent-1", "agent", k(), owner="user-1")
    reg.delegate("user-1", "agent-1", scopes=("execution:submit",))
    engine = PolicyEngine(reg)
    agent = Principal(sub="agent-1", role="agent", scopes=("execution:submit",))
    assert engine.decide(agent, "execution/submit_job").allowed is True
    reg.revoke_all_for("user-1")  # delegation dies now
    assert engine.decide(agent, "execution/submit_job").allowed is False, \
        "a revoked agent's bearer must die with the delegation"


def test_operator_bypass_removed(tmp_path):
    """Role name alone authorizes nothing: an operator-role principal with
    empty scopes is denied even though the old code special-cased the name."""
    from kubona.identity import IdentityRegistry
    from kubona.security.policy import PolicyEngine
    engine = PolicyEngine(IdentityRegistry(path=str(tmp_path / "id.json")))
    op = Principal(sub="op", role="operator", scopes=())
    assert engine.decide(op, "cost/approve_approval").allowed is False
