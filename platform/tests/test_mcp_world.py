"""P5 world-player tools: governance gates, fail-closed config, ledger-backed reads."""

from __future__ import annotations

import os
from unittest.mock import patch

from kubona.mcp.servers import world


def _env(**kw):
    base = {"KUBONA_CORE_URL": "http://core:9", "KUBONA_API_TOKEN": "tok"}
    base.update(kw)
    return base


def test_probe_never_executes_and_validates_task():
    with patch.dict(os.environ, _env(), clear=False):
        r = world.world_probe("sess-1", "nope", "H2")
        assert r["ok"] is False
        r = world.world_probe("sess-1", "atlasChisel", "H2")
        assert r["ok"] is True and r["will_execute"] is False
        assert r["estimated_cost_usd"] == 0.50


def test_unconfigured_fails_closed():
    env = {"KUBONA_CORE_URL": "http://core:9"}
    env.pop("KUBONA_API_TOKEN", None)
    with patch.dict(os.environ, env, clear=True):
        # clear=True drops the real token: network tools must refuse, not dial out.
        assert world.world_look("s")["ok"] is False
        # The estimate is pure computation (no credentials, no network).
        assert world.world_probe("s", "atlasChisel", "H2")["ok"] is True


def test_anchor_requires_evidence():
    assert world.world_anchor("s", [], {}, "note")["ok"] is False
    assert world.world_anchor("s", ["ref"], {}, "")["ok"] is False


def test_anchor_posts_annotation():
    seen = {}

    def fake_post(path, payload):
        seen["path"] = path
        seen["payload"] = payload
        return {"ok": True, "data": {"record_hash": "h"}}

    with patch.dict(os.environ, _env(), clear=False), patch.object(world, "_post", fake_post):
        r = world.world_anchor("sess-9", ["pred_abc"], {"x": 1}, "found a gap")
        assert r["ok"] is True
        assert seen["path"] == "/api/ledger/append"
        assert seen["payload"]["event_type"] == "annotation"
        assert seen["payload"]["payload"]["evidence_refs"] == ["pred_abc"]


def test_gaze_validates_pose():
    assert world.world_gaze("s", [0, 0], [0, 0, 0, 1], "")["ok"] is False
    assert world.world_gaze("s", [0, 0, 0], [0, 0, 0], "")["ok"] is False  # zero quaternion
    with patch.dict(os.environ, _env(), clear=False), patch.object(
        world, "_post", lambda p, b: {"ok": True, "data": {}}
    ):
        r = world.world_gaze("s", [0, 1.5, 3.5], [0, 0, 0, 1], "surveying")
        assert r["ok"] is True


def test_summon_rate_limited_per_session():
    world._summon_log.clear()
    with patch.dict(os.environ, _env(), clear=False), patch.object(
        world, "_post", lambda p, b: {"ok": True, "data": {}}
    ):
        for _ in range(3):
            r = world.world_summon("sess-r", "stall?", "notable")
            assert r.get("suppressed") is False
        r = world.world_summon("sess-r", "stall again", "notable")
        assert r.get("suppressed") is True
    assert world.world_summon("s", "x", "bogus")["ok"] is False
    assert world.world_summon("s", "", "notable")["ok"] is False


def test_handoff_requires_peer_and_pose():
    assert world.world_handoff("s", "", {}, "")["ok"] is False
    assert world.world_handoff("s", "a2", {}, "")["ok"] is False
    with patch.dict(os.environ, _env(), clear=False), patch.object(
        world, "_post", lambda p, b: {"ok": True, "data": {"record_hash": "h"}}
    ):
        r = world.world_handoff("s", "a2", {"position": [0, 0, 0]}, "ctx")
        assert r["ok"] is True


def test_replay_caps_limit_and_filters():
    events = [{"event_type": "prediction_sealed"}, {"event_type": "loop_step"}, {"event_type": "annotation"}]

    def fake_post(path, payload):
        assert path == "/api/ledger/export"
        return {"ok": True, "data": {"events": events}}

    with patch.dict(os.environ, _env(), clear=False), patch.object(world, "_post", fake_post):
        assert world.world_replay("s", limit=999)["count"] == 2  # capped + filtered
        assert world.world_replay("s", limit="x")["ok"] is False


def test_look_reads_session_and_predictions():
    def fake_get(path, params=None):
        if path.startswith("/api/sessions/"):
            return {"ok": True, "data": {"id": "s", "target_system": "H2", "current_spend_usd": 1.0}}
        return {"ok": True, "data": {"sealed": {"prediction_hash": "h"}, "evaluation": None}}

    with patch.dict(os.environ, _env(), clear=False), patch.object(world, "_get", fake_get):
        r = world.world_look("s")
        assert r["ok"] is True
        assert r["campaign"]["target_system"] == "H2"
        assert r["sealed"]["prediction_hash"] == "h"
        assert r["camera_hint"]["convention"] == "rub"


def test_look_lens_request_returns_validated_draft_without_dispatch():
    def fake_get(path, params=None):
        if path.startswith("/api/sessions/"):
            return {"ok": True, "data": {"id": "s", "target_system": "LiH"}}
        return {"ok": True, "data": {}}

    with patch.dict(os.environ, _env(), clear=False), patch.object(world, "_get", fake_get):
        r = world.world_look("s", request_lens="nope")
        assert r["ok"] is False
        r = world.world_look("s", request_lens="atlasChisel")
        assert r["ok"] is True
        draft = r["lens_request"]
        assert draft["task_type"] == "atlasChisel"
        assert draft["target_system"] == "LiH"
        assert draft["estimated_cost_usd"] == 0.50
        assert "human" in draft["requires"].lower()
