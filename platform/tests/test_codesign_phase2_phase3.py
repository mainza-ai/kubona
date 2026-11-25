"""Tests for Circuit Codesign Phase 2 (persistent islands + stop rules + 5 MCP tools)
and Phase 3 (multi-objective Pareto frontier)."""

from __future__ import annotations

import io
import json
import sys
import urllib.request
from pathlib import Path

BRIDGE_DIR = str(Path(__file__).resolve().parents[1] / "bridge")
if BRIDGE_DIR not in sys.path:
    sys.path.insert(0, BRIDGE_DIR)

import codesign_ops  # noqa: E402
from kubona.codesign import FamilySearch, ParetoFrontier  # noqa: E402
from kubona.mcp.servers import _mount, cost  # noqa: E402
from kubona.security.policy import TOOL_SCOPES  # noqa: E402


class _FakeResp(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False


def test_phase2_persistent_islands_and_stop_rule(tmp_path, monkeypatch):
    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))
    art = [["ccx", [0, 1, 2], []]]

    first = codesign_ops.op_families(
        {
            "spec_kind": "and2",
            "families": ["alpha", "beta"],
            "offers": [
                {"family": "alpha", "artifact": art},
                {"family": "beta", "artifact": art},
            ],
            "campaign_id": "camp_persist_01",
            "max_stagnant_steps": 2,
        }
    )
    assert first["ok"] is True
    assert first["campaign_id"] == "camp_persist_01"
    assert first["persisted"] is True
    assert first["lineage_len"] >= 2
    assert "stop_rule" in first
    assert (tmp_path / "codesign_islands" / "camp_persist_01.json").exists()

    second = codesign_ops.op_families(
        {
            "spec_kind": "and2",
            "families": ["alpha", "beta"],
            "offers": [
                {"family": "alpha", "artifact": art},
                {"family": "beta", "artifact": art},
                {"family": "alpha", "artifact": art},
                {"family": "beta", "artifact": art},
            ],
            "campaign_id": "camp_persist_01",
            "max_stagnant_steps": 2,
        }
    )
    assert second["ok"] is True
    assert second["persisted"] is True
    assert second["lineage_len"] > first["lineage_len"]
    assert second["stop_rule"]["should_stop"] is True

    # Snapshot round-trip directly on FamilySearch
    snap = json.loads((tmp_path / "codesign_islands" / "camp_persist_01.json").read_text())
    restored = FamilySearch.from_snapshot(snap)
    assert len(restored.lineage) == second["lineage_len"]
    stop_info = restored.check_stop_rule(max_stagnant_steps=2)
    assert stop_info["should_stop"] is True
    assert stop_info["min_stagnant"] >= 2
    assert stop_info["reason"] == "all_families_stagnated_ge_2"


def test_phase3_pareto_frontier_non_dominance_and_batch():
    pf = ParetoFrontier(kappa=1)
    # A: low TQ (10*3=30), higher T-depth (4), 3 qubits
    assert pf.offer("art_a", toffoli=10, qubits=3, toffoli_depth=4, meta={"label": "low_tq"}) is True
    # B: higher TQ (12*3=36), lower T-depth (2), 3 qubits -> mutually non-dominated with A
    assert pf.offer("art_b", toffoli=12, qubits=3, toffoli_depth=2, meta={"label": "low_depth"}) is True
    # C: strictly dominated by A (worse TQ=40, worse depth=5, worse qubits=4)
    assert pf.offer("art_c", toffoli=10, qubits=4, toffoli_depth=5, meta={"label": "dominated"}) is False

    labels = {e.meta["label"] for e in pf.entries()}
    assert labels == {"low_tq", "low_depth"}

    # Strictly better than B on all axes replaces B
    assert pf.offer("art_b2", toffoli=11, qubits=3, toffoli_depth=1, meta={"label": "better_depth"}) is True
    labels_after = {e.meta["label"] for e in pf.entries()}
    assert labels_after == {"low_tq", "better_depth"}

    # op_batch emits both MAP-Elites frontier and Pareto frontier + artifact_sha256
    batch_out = codesign_ops.op_batch(
        {
            "spec_kind": "and2",
            "kappa": 1,
            "artifacts": [
                [["ccx", [0, 1, 2], []]],
                [
                    ["ccx", [0, 1, 2], []],
                    ["x", [0], []],
                    ["x", [0], []],
                ],
            ],
        }
    )
    assert batch_out["ok"] is True
    assert "pareto" in batch_out
    assert batch_out["pareto"]["size"] >= 1
    assert all(len(d["artifact_sha256"]) == 64 for d in batch_out["details"])


def test_phase2_five_codesign_mcp_tools_mounted_authorized_and_wired(monkeypatch):
    # Mounted in gateway & authorized in policy
    gw = _mount.build_gateway(require_auth=False)
    mounted = gw.mounted_tools()
    for name in (
        "codesign_spec",
        "codesign_evaluate",
        "codesign_frontier",
        "codesign_verify",
        "codesign_claim",
    ):
        assert f"cost/{name}" in TOOL_SCOPES
        assert name in mounted["cost"]

    # 1. codesign_spec (local, no HTTP)
    spec_out = cost.codesign_spec("and2")
    assert spec_out["ok"] is True
    assert spec_out["exhaustive_domain_size"] == 4
    assert "and2" in spec_out["supported_kinds"]

    bad_spec = cost.codesign_spec("nonexistent_kind")
    assert bad_spec["ok"] is False

    # Stub urllib.request.urlopen for HTTP-forwarding tools
    calls: list[tuple[str, str, dict | None, str | None]] = []

    def fake_urlopen(req, timeout=120):
        body = json.loads(req.data.decode("utf-8")) if req.data else None
        calls.append((req.method, req.full_url, body, req.headers.get("Authorization")))
        return _FakeResp(json.dumps({"ok": True, "echo_url": req.full_url, "echo_body": body}).encode("utf-8"))

    monkeypatch.setattr(urllib.request, "urlopen", fake_urlopen)
    monkeypatch.setenv("KUBONA_API_URL", "http://127.0.0.1:8080")
    monkeypatch.setenv("KUBONA_API_TOKEN", "tok-test-123")

    # 2. codesign_evaluate
    ev = cost.codesign_evaluate(
        spec_kind="and2",
        artifact=[["ccx", [0, 1, 2], []]],
        session_id="sess-1",
    )
    assert ev["ok"] is True
    assert calls[-1][0] == "POST"
    assert calls[-1][1].endswith("/api/codesign/evaluate")
    assert calls[-1][2]["session_id"] == "sess-1"
    assert calls[-1][3] == "Bearer tok-test-123"

    # 3. codesign_frontier
    fr = cost.codesign_frontier(entries=[{"label": "a", "toffolis": 1, "qubits": 3}])
    assert fr["ok"] is True
    assert calls[-1][1].endswith("/api/codesign/frontier")

    # 4. codesign_verify
    ver = cost.codesign_verify(session_id="sess-1", run_id="cs_abc")
    assert ver["ok"] is True
    assert calls[-1][0] == "GET"
    assert "/api/codesign/runs/cs_abc?session_id=sess-1" in calls[-1][1]

    # 5. codesign_claim
    cl = cost.codesign_claim(
        session_id="sess-1",
        claim_id="clm_circ_1",
        code_name="and2",
        eval_run_id="cs_1",
        baseline_run_id="cs_2",
        repro_run_id="cs_3",
        held_out_run_id="cs_4",
    )
    assert cl["ok"] is True
    assert calls[-1][0] == "POST"
    assert calls[-1][1].endswith("/api/sessions/sess-1/discovery-claims")
    assert calls[-1][2]["kind"] == "circuit"
    assert calls[-1][2]["eval_run_id"] == "cs_1"
    assert calls[-1][2]["held_out_run_id"] == "cs_4"

    # Fail-closed when token absent for authenticated calls
    monkeypatch.delenv("KUBONA_API_TOKEN", raising=False)
    refused = cost.codesign_verify(session_id="sess-1", run_id="cs_abc")
    assert refused["ok"] is False
    assert "KUBONA_API_TOKEN" in refused["error"]
