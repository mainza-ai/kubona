"""HTTP gateway transport (G8): plain OAuth RPC + real X25519+ML-KEM hybrid envelope."""

import base64
import json

from starlette.testclient import TestClient

from kubona.mcp.gateway import KubonaGateway
from kubona.mcp.http_gateway import make_http_gateway
from kubona.mcp.servers._mount import build_gateway
from kubona.security.hybrid_transport import (
    HybridKeyPair, handshake_initiator, handshake_responder,
)
from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer

BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"


def _issuer_gateway() -> tuple[KubonaGateway, OAuth2TokenIssuer]:
    iss = OAuth2TokenIssuer()
    gw = build_gateway(require_auth=True)
    gw.auth = OAuth2AccessTokenProvider(iss)
    return gw, iss


def test_health_reports_servers():
    gw, _ = _issuer_gateway()
    client = TestClient(make_http_gateway(gw))
    r = client.get("/health")
    assert r.status_code == 200 and r.json()["ok"] is True
    # W0.14: minimal health — no server inventory for recon.
    assert "servers" not in r.json()


def test_plain_authenticated_rpc():
    gw, iss = _issuer_gateway()
    client = TestClient(make_http_gateway(gw))
    token = iss.issue("http-agent", role="agent", scopes=("compiler:read",))
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": BELL}},
                    headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 200 and r.json()["ok"] is True
    assert r.json()["result"]["num_qubits"] == 2


def test_unauthenticated_rpc_rejected():
    gw, _ = _issuer_gateway()
    client = TestClient(make_http_gateway(gw))
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": BELL}})
    assert r.status_code == 401


def test_scoped_deny_over_http():
    gw, iss = _issuer_gateway()
    client = TestClient(make_http_gateway(gw))
    token = iss.issue("http-agent", role="agent", scopes=("compiler:read",))
    r = client.post("/rpc", json={"server": "cost", "tool": "approve_approval",
                                  "args": {"approval_id": "x"}},
                    headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 403  # lacks cost:approve


def test_hybrid_mlkem_envelope_over_http():
    """A real two-party X25519+ML-KEM session carries the RPC envelope end to end."""
    gw, iss = _issuer_gateway()
    # two-party real handshake
    client_kp = HybridKeyPair.generate()
    server_kp = HybridKeyPair.generate()
    client_session, ct = handshake_initiator(client_kp, server_kp.public_bytes())
    server_session = handshake_responder(server_kp, client_kp.public_bytes(), ct)

    app = make_http_gateway(gw, secure_session=server_session)
    client = TestClient(app)
    token = iss.issue("secure-agent", role="agent", scopes=("compiler:read",))

    # client wraps the call in the hybrid session (real envelope discipline:
    # nonce + expiry + directional AAD, inner token bound to the bearer)
    from kubona.security.secure_rpc import SecureRpc
    rpc_client = SecureRpc(client_session)
    enc = rpc_client.wrap_call("compiler", "transpile_circuit",
                               {"circuit_qasm": BELL}, token=token)
    r = client.post("/rpc", json={"envelope": base64.b64encode(enc).decode()},
                    headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 200 and r.json()["ok"] is True
    resp = rpc_client.unwrap_response(base64.b64decode(r.json()["envelope"]))
    assert resp["num_qubits"] == 2

    # a tampered envelope must be rejected
    bad = enc[:-3] + bytes([enc[-1] ^ 0xFF])
    r2 = client.post("/rpc", json={"envelope": base64.b64encode(bad).decode()},
                     headers={"Authorization": f"Bearer {token}"})
    assert r2.status_code == 400

    # a replayed envelope must be rejected (single-use nonces)
    r3 = client.post("/rpc", json={"envelope": base64.b64encode(enc).decode()},
                     headers={"Authorization": f"Bearer {token}"})
    assert r3.status_code == 400

    # inner/outer identity mismatch must be refused (confused deputy)
    other = iss.issue("other-agent", role="agent", scopes=("compiler:read",))
    enc2 = rpc_client.wrap_call("compiler", "transpile_circuit",
                                {"circuit_qasm": BELL}, token=other)
    r4 = client.post("/rpc", json={"envelope": base64.b64encode(enc2).decode()},
                     headers={"Authorization": f"Bearer {token}"})
    assert r4.status_code == 200
    refused = rpc_client.unwrap_response(base64.b64decode(r4.json()["envelope"]))
    assert refused["ok"] is False


def test_secrets_fail_closed():
    from kubona.secrets import SecretStore
    store = SecretStore()
    assert store.get("xanadu-strawberry-fields", "api_key") is None
    import pytest
    with pytest.raises(KeyError):
        store.require("xanadu-strawberry-fields", "api_key")


def test_require_secure_refuses_plaintext_rpc():
    gw, iss = _issuer_gateway()
    client_kp = HybridKeyPair.generate()
    server_kp = HybridKeyPair.generate()
    client_session, ct = handshake_initiator(client_kp, server_kp.public_bytes())
    server_session = handshake_responder(server_kp, client_kp.public_bytes(), ct)

    app = make_http_gateway(gw, secure_session=server_session, require_secure=True)
    client = TestClient(app)
    token = iss.issue("http-agent", role="agent", scopes=("compiler:read",))
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": BELL}},
                    headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 400
    assert "secure envelope required" in r.json()["error"]


def test_secret_get_required_fails_closed(monkeypatch):
    from kubona.secrets import SecretStore
    monkeypatch.delenv("KUBONA_IONQ_API_KEY", raising=False)
    try:
        SecretStore().get("ionq", "api_key", required=True)
    except KeyError as exc:
        assert "ionq" in str(exc)
    else:
        raise AssertionError("required missing secret must raise, not return None")
