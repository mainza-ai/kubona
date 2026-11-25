"""Phase 1/3/5 end-to-end tests for Kubona's standard MCP Streamable HTTP (/mcp),
profile scoping, MCP Resources, MCP Prompts, server instructions, science tools,
and mcp-config generator."""

from __future__ import annotations

import json

from starlette.testclient import TestClient

from kubona.mcp.auth import DevAuthProvider
from kubona.mcp.http_gateway import make_http_gateway
from kubona.mcp.servers import science
from kubona.mcp.servers._mount import build_gateway
from kubona.mcp.unified import PROFILES
from kubona.run import _cli

BELL_QASM = (
    "OPENQASM 2.0;\n"
    'include "qelib1.inc";\n'
    "qreg q[2];\n"
    "creg c[2];\n"
    "h q[0];\n"
    "cx q[0],q[1];\n"
    "measure q -> c;\n"
)


def _rpc_post(client: TestClient, path: str, method: str, params: dict | None = None,
              token: str | None = "dev-token", req_id: int = 1):
    headers = {
        "Accept": "application/json, text/event-stream",
        "Content-Type": "application/json",
    }
    if token is not None:
        headers["Authorization"] = f"Bearer {token}"
    payload: dict = {"jsonrpc": "2.0", "id": req_id, "method": method}
    if params is not None:
        payload["params"] = params
    return client.post(path, headers=headers, json=payload)


def _make_client():
    auth = DevAuthProvider()
    gw = build_gateway(require_auth=True, auth=auth)
    app = make_http_gateway(gw)
    return TestClient(app, base_url="http://127.0.0.1:8123")


def test_mcp_auth_and_profile_fail_closed():
    with _make_client() as client:
        # Unauthenticated -> 401
        r = _rpc_post(client, "/mcp", "initialize", {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "test", "version": "1.0"},
        }, token=None)
        assert r.status_code == 401

        # Unknown profile -> 400
        r = _rpc_post(client, "/mcp?profile=bogus", "tools/list", {})
        assert r.status_code == 400
        assert "unknown MCP profile" in r.json()["error"]


def test_mcp_initialize_resources_prompts_and_profiles():
    with _make_client() as client:
        # Initialize carries Kubona instructions
        r = _rpc_post(client, "/mcp?profile=science", "initialize", {
            "protocolVersion": "2025-03-26",
            "capabilities": {},
            "clientInfo": {"name": "test-agent", "version": "1.0"},
        })
        assert r.status_code == 200, r.text
        res = r.json()["result"]
        assert "classical anchor" in res.get("instructions", "").lower()
        assert res["serverInfo"]["name"] == "kubona-science"

        # Profile scoping: read-only excludes mutating tools; all exposes 54 tools
        r_ro = _rpc_post(client, "/mcp?profile=read-only", "tools/list", {})
        assert r_ro.status_code == 200
        ro_names = {t["name"] for t in r_ro.json()["result"]["tools"]}
        assert "estimate_cost" in ro_names
        assert "get_classical_anchor" in ro_names
        assert "submit_job" not in ro_names
        assert "kill_loop" not in ro_names
        assert "write_record" not in ro_names

        r_all = _rpc_post(client, "/mcp?profile=all", "tools/list", {})
        assert r_all.status_code == 200
        all_names = {t["name"] for t in r_all.json()["result"]["tools"]}
        assert len(all_names) == 54

        # Resources: index + skill body + error catalog
        r_res = _rpc_post(client, "/mcp", "resources/list", {})
        assert r_res.status_code == 200
        uris = {item["uri"] for item in r_res.json()["result"]["resources"]}
        assert "kubona://skills" in uris
        assert "kubona://contracts/error-catalog" in uris
        assert "kubona://contracts/mcp-catalog" in uris

        r_sk = _rpc_post(client, "/mcp", "resources/read", {"uri": "kubona://skills"})
        assert r_sk.status_code == 200
        skills_idx = json.loads(r_sk.json()["result"]["contents"][0]["text"])
        skill_names = {s["name"] for s in skills_idx["skills"]}
        assert {"kubona", "kubona-connect", "kubona-experiments", "kubona-governance", "kubona-research", "kubona-ops"} <= skill_names

        r_body = _rpc_post(client, "/mcp", "resources/read", {"uri": "kubona://skills/kubona"})
        assert r_body.status_code == 200
        assert "Three Non-Negotiable Invariants" in r_body.json()["result"]["contents"][0]["text"]

        # Prompts: all 5 canonical prompts registered
        r_pr = _rpc_post(client, "/mcp", "prompts/list", {})
        assert r_pr.status_code == 200
        pnames = {p["name"] for p in r_pr.json()["result"]["prompts"]}
        assert {
            "vqe-with-anchor",
            "hardware-readiness-check",
            "cost-governed-submit",
            "audit-session-provenance",
            "debug-failed-job",
        } <= pnames

        r_pg = _rpc_post(client, "/mcp", "prompts/get", {
            "name": "vqe-with-anchor",
            "arguments": {"molecule": "LiH", "bond_distance_angstrom": "1.6"},
        })
        assert r_pg.status_code == 200
        prompt_text = r_pg.json()["result"]["messages"][0]["content"]["text"]
        assert "LiH" in prompt_text and "get_classical_anchor" in prompt_text


def test_mcp_streamable_http_end_to_end_simulator_workflow():
    with _make_client() as client:
        # 1. Provider registry discovery
        r = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "list_provider_registry",
            "arguments": {},
        })
        assert r.status_code == 200
        assert not r.json()["result"].get("isError", False)

        # 2. Transpile canonical QASM
        r = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "transpile_circuit",
            "arguments": {"circuit_qasm": BELL_QASM},
        })
        assert r.status_code == 200
        tr = json.loads(r.json()["result"]["content"][0]["text"])
        assert tr["ok"] is True
        assert tr["num_qubits"] == 2
        canon = tr["canonical_qasm"]

        # 3. Create ceilinged session (owner matches DevAuthProvider's default principal)
        r = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "create_session",
            "arguments": {"name": "bell-smoke", "owner": "researcher@kubona.local", "budget_ceiling_usd": 10.0},
        })
        assert r.status_code == 200
        sess = json.loads(r.json()["result"]["content"][0]["text"])
        sid = sess["session_id"]

        # 4. Estimate cost before dispatch
        r = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "estimate_cost",
            "arguments": {
                "circuit_qasm": canon,
                "provider": "local-simulator",
                "shots": 128,
                "session_id": sid,
            },
        })
        assert r.status_code == 200
        est = json.loads(r.json()["result"]["content"][0]["text"])
        assert est["preflight_ok"] is True
        assert est["estimate"]["pricing_model"] == "free"
        assert est["estimate"]["estimated_usd"] == 0.0

        # 5. Submit on free local-simulator -> poll status -> fetch result
        r = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "submit_job",
            "arguments": {
                "circuit_qasm": canon,
                "provider": "local-simulator",
                "shots": 128,
                "session_id": sid,
            },
        })
        assert r.status_code == 200
        sub = json.loads(r.json()["result"]["content"][0]["text"])
        assert sub.get("ok") is True or "job_id" in sub
        jid = sub["job_id"]

        r_st = _rpc_post(client, "/mcp?profile=science", "tools/call", {
            "name": "get_job_status",
            "arguments": {"job_id": jid},
        })
        assert r_st.status_code == 200
        st = json.loads(r_st.json()["result"]["content"][0]["text"])
        assert st.get("job_id") == jid

        # 5. Legacy /rpc still works alongside /mcp
        r_legacy = client.post(
            "/rpc",
            headers={"Authorization": "Bearer dev-token"},
            json={"server": "compiler", "tool": "transpile_circuit", "args": {"circuit_qasm": BELL_QASM}},
        )
        assert r_legacy.status_code == 200
        assert r_legacy.json()["ok"] is True


def test_science_tools_fail_closed_unconfigured(monkeypatch):
    monkeypatch.delenv("KUBONA_API_TOKEN", raising=False)
    res = science.get_classical_anchor("H2", 0.74)
    assert res["ok"] is False
    assert res["error_code"] == "server_misconfigured"

    res_bad = science.run_experiment("s1", "not_a_real_kind")
    assert res_bad["ok"] is False
    assert res_bad["error_code"] == "bad_request"

    res_id = science.list_hypotheses("../escape")
    assert res_id["ok"] is False
    assert res_id["error_code"] == "bad_request"


def test_mcp_config_cli_never_leaks_secrets(capsys, monkeypatch):
    monkeypatch.setenv("KUBONA_API_TOKEN", "SUPER_SECRET_DO_NOT_PRINT")
    assert _cli(["kubona.run", "mcp-config", "--client", "claude-code", "--transport", "http", "--profile", "science"]) == 0
    out = capsys.readouterr().out
    assert "SUPER_SECRET_DO_NOT_PRINT" not in out
    parsed = json.loads(out)
    assert parsed["mcpServers"]["kubona"]["url"].endswith("/mcp?profile=science")

    assert _cli(["kubona.run", "mcp-config", "--client", "codex", "--transport", "stdio", "--profile", "read-only"]) == 0
    toml_out = capsys.readouterr().out
    assert "SUPER_SECRET_DO_NOT_PRINT" not in toml_out
    assert "[mcp_servers.kubona]" in toml_out
    assert sorted(PROFILES)  # profiles exported
