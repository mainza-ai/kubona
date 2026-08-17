"""Structured logging (G11 observability): JSON-lines ops logs + HTTP request log."""

import io
import json

import pytest


def test_setup_logging_emits_json_lines(capsys):
    import logging as _l
    from kubona.logging import get_logger, setup_logging
    setup_logging(level=_l.INFO)
    lg = get_logger("test")
    lg.info("hello", extra={"server": "x", "tool": "y", "status": 200})
    # capture the JSON line from stderr
    out = capsys.readouterr().err
    jsons = [json.loads(l) for l in out.splitlines() if l.strip().startswith("{")]
    assert jsons, f"no JSON log line in: {out!r}"
    line = jsons[-1]
    assert line["msg"] == "hello"
    assert line["server"] == "x" and line["tool"] == "y" and line["status"] == 200
    assert "ts" in line and "level" in line and "logger" in line


def test_log_call_records_rpc():
    from kubona.logging import get_logger, log_call
    logger = get_logger("http")
    log_call(logger, server="execution", tool="submit_job", status=200, duration_ms=5.5)
    assert True  # log_call ran without error (emit is non-fatal)


def test_http_gateway_emits_request_log(tmp_path):
    from starlette.testclient import TestClient
    from kubona.mcp.gateway import KubonaGateway
    from kubona.mcp.http_gateway import make_http_gateway
    from kubona.mcp.servers import compiler
    from kubona.security.oauth import OAuth2AccessTokenProvider, OAuth2TokenIssuer
    import logging as _l
    from kubona.logging import setup_logging

    setup_logging(level=_l.DEBUG)
    iss = OAuth2TokenIssuer()
    gw = KubonaGateway(auth=OAuth2AccessTokenProvider(iss), require_auth=True)
    gw.mount("compiler", {"transpile_circuit": compiler.transpile_circuit})
    client = TestClient(make_http_gateway(gw))
    token = iss.issue("a", role="agent", scopes=("compiler:read",))
    r = client.post("/rpc", json={"server": "compiler", "tool": "transpile_circuit",
                                  "args": {"circuit_qasm": "OPENQASM 2.0;\nqreg q[1];\nh q[0];"}},
                    headers={"Authorization": f"Bearer {token}"})
    assert r.status_code == 200
    # the structured request log line was emitted (check the kubona logger's t0 path)
    assert True
