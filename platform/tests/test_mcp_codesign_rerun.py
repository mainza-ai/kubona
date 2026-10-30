"""C5b MCP rail: the cost server forwards to the governed Go API, never
holds an LLM key, and returns the server-generated request id for retries."""

import json
import urllib.request

from kubona.mcp.servers import cost


class _Resp:
    status = 200

    def __init__(self, payload: dict):
        self._body = json.dumps(payload).encode()

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


def test_codesign_rerun_forwards_bearer_and_request_id(monkeypatch):
    seen = {}

    def fake_urlopen(req, timeout=0):
        seen["req"] = req
        seen["timeout"] = timeout
        return _Resp({"ok": True, "run_id": "run-1", "billing": "provider", "payer": "user-key"})

    monkeypatch.setenv("KUBONA_API_TOKEN", "test-token")
    monkeypatch.setenv("KUBONA_API_URL", "http://127.0.0.1:8080")
    monkeypatch.setattr(urllib.request, "urlopen", fake_urlopen)
    out = cost.codesign_rerun(
        "sess-1", spec_kind="and2", budget_steps=2, num_tests=8,
        max_tokens_per_step=128, request_id="rid-1",
    )
    assert out["ok"] is True and out["run_id"] == "run-1"
    req = seen["req"]
    assert req.full_url == "http://127.0.0.1:8080/api/codesign/rerun"
    assert req.headers["Authorization"] == "Bearer test-token"
    body = json.loads(req.data)
    assert body["session_id"] == "sess-1"
    assert body["request_id"] == "rid-1"
    assert body["budget_steps"] == 2
    assert seen["timeout"] == 660


def test_codesign_rerun_refuses_without_token(monkeypatch):
    monkeypatch.delenv("KUBONA_API_TOKEN", raising=False)

    def should_not_run(*args, **kwargs):
        raise AssertionError("urlopen must not run without bearer auth")

    monkeypatch.setattr(urllib.request, "urlopen", should_not_run)
    out = cost.codesign_rerun("sess-1")
    assert out["ok"] is False
    assert "KUBONA_API_TOKEN" in out["error"]


def test_codesign_rerun_returns_request_id_on_http_error(monkeypatch):
    import urllib.error

    def fake_urlopen(req, timeout=0):
        raise urllib.error.HTTPError(
            req.full_url, 403, "forbidden", {},
            __import__("io").BytesIO(b'{"message":"cloud opt-in missing"}'),
        )

    monkeypatch.setenv("KUBONA_API_TOKEN", "test-token")
    monkeypatch.setattr(urllib.request, "urlopen", fake_urlopen)
    out = cost.codesign_rerun("sess-1", request_id="rid-2")
    assert out["ok"] is False
    assert out["request_id"] == "rid-2"
    assert "cloud opt-in missing" in out["error"]
