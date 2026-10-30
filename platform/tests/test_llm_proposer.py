"""Tests for the live LLM proposer and the strict gate-proposal parser.

The HTTP path is exercised against a local stub server (no network). The
deterministic mutator remains the default; this is the opt-in live path.
"""

from __future__ import annotations

import json
import threading
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

from kubona.codesign.families import (
    NvidiaProposer,
    make_gate_proposer,
    parse_gate_proposal,
)


class _Handler(BaseHTTPRequestHandler):
    calls = 0
    fail_first = False
    status = 200

    def do_POST(self):  # noqa: N802 — http.server API
        length = int(self.headers.get("Content-Length", "0"))
        self.rfile.read(length)
        type(self).calls += 1
        if type(self).fail_first and type(self).calls == 1:
            body = b'{"error":"transient"}'
            self.send_response(500)
        elif type(self).status != 200:
            body = b'{"error":"refused"}'
            self.send_response(type(self).status)
        else:
            content = '{"ops": [["x", [0], []], ["cx", [0, 1], []]]}'
            body = json.dumps({"choices": [{"message": {"content": content}}]}).encode()
            self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):  # keep test output clean
        return


@contextmanager
def stub_server():
    handler = type("StubHandler", (_Handler,), {"calls": 0, "fail_first": False, "status": 200})
    srv = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    try:
        yield srv, handler
    finally:
        srv.shutdown()
        srv.server_close()
        thread.join(timeout=2)


def _proposer(port: int) -> NvidiaProposer:
    return NvidiaProposer(
        provider="strata",
        model="stub-model",
        base_url=f"http://127.0.0.1:{port}/v1",
        timeout_sec=5,
        max_retries=1,
    )


def test_propose_calls_openai_compatible_endpoint():
    with stub_server() as (srv, handler):
        text = _proposer(srv.server_port).propose("propose a circuit")
        assert "ops" in text
        assert handler.calls == 1


def test_propose_retries_one_transport_or_5xx_failure():
    with stub_server() as (srv, handler):
        handler.fail_first = True
        text = _proposer(srv.server_port).propose("propose a circuit")
        assert "ops" in text
        assert handler.calls == 2


def test_propose_does_not_retry_4xx():
    with stub_server() as (srv, handler):
        handler.status = 400
        with pytest.raises(ValueError, match="refused"):
            _proposer(srv.server_port).propose("propose a circuit")
        assert handler.calls == 1


def test_parse_accepts_code_fence_and_bare_list():
    assert parse_gate_proposal('```json\n{"ops": [["x", [0], []]]}\n```', 2) == [("x", (0,), ())]
    assert parse_gate_proposal('[["cx", [0, 1], []]]', 2) == [("cx", (0, 1), ())]


@pytest.mark.parametrize(
    "text",
    [
        "",
        "not json",
        '{"ops": []}',
        '{"ops": [["h", [0], []]]}',
        '{"ops": [["cx", [0], []]]}',
        '{"ops": [["x", [2], []]]}',
        '{"ops": "not a list"}',
    ],
)
def test_parse_refuses_invalid(text):
    with pytest.raises(ValueError):
        parse_gate_proposal(text, num_qubits=2)


def test_parse_refuses_too_many_ops():
    ops = [["x", [0], []]] * 4
    with pytest.raises(ValueError, match="cap"):
        parse_gate_proposal(json.dumps({"ops": ops}), 2, max_ops=3)


class _StubProposer:
    def propose(self, prompt: str) -> str:
        assert "Allowed gates" in prompt
        assert "Current ops" in prompt
        return '{"ops": [["x", [0], []]]}'


def test_make_gate_proposer_adapts_to_rerun_contract():
    propose = make_gate_proposer(_StubProposer(), num_qubits=2)
    assert propose([], 0) == [("x", (0,), ())]


def test_gqe_harness_is_exported():
    from kubona.autoresearch import build_gqe_harness

    assert build_gqe_harness().name == "gqe_circuit"
