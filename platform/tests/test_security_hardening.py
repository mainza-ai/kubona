"""Phase-gap closures: real OAuth token issuance, ML-KEM secure RPC, routing in execution."""

import time

import pytest

from kubona.mcp.auth import AuthError
from kubona.mcp.servers._mount import build_gateway
from kubona.security import HybridKeyPair, handshake_initiator, handshake_responder
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
from kubona.security.secure_rpc import SecureRpc
from kubona.security.hybrid_transport import TransportError


# -- OAuth 2.1-shaped tokens ----------------------------------------------
def test_oauth_issue_verify_and_scopes():
    issuer = OAuth2TokenIssuer()
    token = issuer.issue("agent-1", role="agent", scopes=("execution:submit", "cost:read"))
    principal = issuer.decode(token)
    assert principal.sub == "agent-1" and principal.role == "agent"
    assert set(principal.scopes) == {"execution:submit", "cost:read"}


def test_oauth_expired_token_rejected():
    issuer = OAuth2TokenIssuer(ttl_s=1)
    token = issuer.issue("u")
    time.sleep(1.1)
    with pytest.raises(AuthError):
        issuer.decode(token)


def test_oauth_tampered_token_rejected():
    issuer = OAuth2TokenIssuer()
    token = issuer.issue("u")
    parts = token.split(".")
    tampered = parts[0] + "." + parts[1][:-2] + "XX" + "." + parts[2]
    with pytest.raises(AuthError):
        issuer.decode(tampered)


def test_oauth_provider_on_gateway():
    issuer = OAuth2TokenIssuer()
    gw = build_gateway()
    gw.auth = OAuth2AccessTokenProvider(issuer)
    token = issuer.bearer("researcher", "operator", ("*",))
    out = gw.call("compiler", "transpile_circuit",
                  {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];"}, token=token)
    assert out["ok"] is True
    # agent token with restricted scopes cannot approve spend
    agent_token = issuer.bearer("agent-1", "agent", ("execution:submit",))
    with pytest.raises(AuthError):
        gw.call("cost", "approve_approval", {"approval_id": "x"}, token=agent_token)


# -- ML-KEM secure RPC -------------------------------------------------------
def test_secure_rpc_roundtrip_and_tamper():
    client = HybridKeyPair.generate()
    server = HybridKeyPair.generate()
    s_client, ct = handshake_initiator(client, server.public_bytes())
    s_server = handshake_responder(server, client.public_bytes(), ct)
    gw = build_gateway()

    rpc_client = SecureRpc(s_client)
    envelope = rpc_client.wrap_call("compiler", "transpile_circuit",
                                    {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];"},
                                    token="dev-token")
    response_enc = SecureRpc(s_server, gateway=gw).handle(envelope)
    result = rpc_client.unwrap_response(response_enc)
    assert result["ok"] is True

    # tampering with the encrypted envelope is rejected
    bad = envelope[:-1] + bytes([envelope[-1] ^ 0xFF])
    with pytest.raises(TransportError):
        SecureRpc(s_server, gateway=gw).handle(bad)


# -- routing enforcement inside execution -----------------------------------
def test_execution_blocks_routing_ineligible_via_gateway():
    gw = build_gateway()
    # US-only caller to quantinuum (US/UK) with export class 'any' -> export-control not allowed
    out = gw.call("execution", "submit_job", {
        "circuit_qasm": "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];",
        "provider": "quantinuum", "shots": 1024, "seed": 1,
        "caller_jurisdictions": ["US"], "caller_export_class": "any"}, token="dev-token")
    assert out["ok"] is False
    assert "routing blocked" in out["error"]
    assert out["routing"]["verdict"] == "INELIGIBLE"


def test_execution_allows_local_simulator_from_anywhere():
    gw = build_gateway()
    out = gw.call("execution", "submit_job", {
        "circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];",
        "provider": "local-simulator", "shots": 256, "seed": 1,
        "caller_jurisdictions": ["EU"]}, token="dev-token")
    assert out["ok"] is True and out["status"] == "completed"
