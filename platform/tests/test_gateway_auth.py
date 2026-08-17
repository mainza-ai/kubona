"""Gateway auth boundary tests (OAuth 2.1-shaped)."""

import pytest

from kubona.mcp.auth import AuthError, DevAuthProvider, OAuth21VerifierStub
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


def test_oauth21_stub_contract():
    v = OAuth21VerifierStub()
    with pytest.raises(NotImplementedError):
        v.verify("anything")


def test_dev_auth_rejects_bad_token():
    with pytest.raises(AuthError):
        DevAuthProvider().verify("nope")
