"""Observatory world tools (P5): agents navigate the same world as humans.

Reads go through kubonad (sessions, vault predictions, ledger export);
writes go through the ledger-append rail as ``annotation`` events only —
agents never mint sealed predictions, never touch spend, and never dispatch
Atlas tasks directly. Every tool fails closed without KUBONA_CORE_URL /
KUBONA_API_TOKEN, and every write is bearers-authed server-side.

Governance gates (plan §12): probe returns a cost estimate FIRST and never
executes (dispatch stays human-driven); anchor requires evidence refs;
summon is rate-limited per session; gaze is append-only and always on.
"""

from __future__ import annotations

import json
import os
import time
import urllib.request

from mcp.server.mcpserver import MCPServer

mcp = MCPServer("kubona-world")

CORE_URL = os.environ.get("KUBONA_CORE_URL", "http://localhost:8080").rstrip("/")
LENS_COST_USD = 0.50  # matches lensCostUSD in platform/core/pkg/api/observatory_lens.go
LENS_TASKS = ("images2PosedRGBD", "atlasTextToImage", "atlasGenerate", "atlasChisel")
_SUMMON_WINDOW_S = 3600
_SUMMON_MAX_PER_WINDOW = 3
_summon_log: dict[str, list[float]] = {}


def _token() -> str | None:
    return os.environ.get("KUBONA_API_TOKEN") or None


def _unconfigured() -> dict:
    return {"ok": False, "error": "world tools unconfigured: set KUBONA_CORE_URL and KUBONA_API_TOKEN (operator credential, never committed)"}


def _get(path: str, params: dict | None = None) -> dict:
    token = _token()
    if not token:
        return _unconfigured()
    url = CORE_URL + path
    if params:
        from urllib.parse import urlencode

        url += "?" + urlencode(params)
    req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return {"ok": True, "status": resp.status, "data": json.loads(resp.read().decode())}
    except Exception as e:  # noqa: BLE001 — surfaced as data, never raised
        return {"ok": False, "error": f"kubonad read failed: {e}"}


def _post(path: str, payload: dict) -> dict:
    token = _token()
    if not token:
        return _unconfigured()
    req = urllib.request.Request(
        CORE_URL + path,
        data=json.dumps(payload).encode(),
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return {"ok": True, "status": resp.status, "data": json.loads(resp.read().decode())}
    except Exception as e:  # noqa: BLE001 — surfaced as data, never raised
        return {"ok": False, "error": f"kubonad write failed: {e}"}


def _append_annotation(session_id: str, payload: dict) -> dict:
    body = {"session_id": session_id, "event_type": "annotation", "payload": dict(payload, session_id=session_id)}
    return _post("/api/ledger/append", body)


def world_look(session_id: str, request_lens: str = "") -> dict:
    """Return nearby ledger records and camera context for a session.

    Free for cached records. Read-only: never bills, never mutates.
    With request_lens set to a task name, ALSO return a validated,
    ready-to-dispatch lens request payload (R-29) — the payload is for a
    human to review and dispatch; this tool never dispatches or reserves.
    """
    out: dict = {"ok": True, "session_id": session_id}
    sess = _get(f"/api/sessions/{session_id}")
    if not sess.get("ok"):
        return {"ok": False, "error": sess.get("error", "session lookup failed")}
    data = sess.get("data") or {}
    out["campaign"] = {k: data.get(k) for k in ("id", "title", "target_system", "spend_limit_usd", "current_spend_usd")}
    pred = _get("/api/vault/predictions/latest", {"session_id": session_id})
    if pred.get("ok"):
        pdata = pred.get("data") or {}
        out["sealed"] = pdata.get("sealed")
        out["evaluation"] = pdata.get("evaluation")
    out["camera_hint"] = {"convention": "rub", "grid": [1280, 720], "note": "poses are camera-to-world XYZW quaternions"}
    if request_lens:
        if request_lens not in LENS_TASKS:
            return {"ok": False, "error": f"unknown task_type {request_lens!r}: use one of {sorted(LENS_TASKS)}"}
        target = (data.get("target_system") or "H2") if isinstance(data, dict) else "H2"
        out["lens_request"] = {
            "task_type": request_lens,
            "target_system": target,
            "endpoint": "POST /api/observatory/lens",
            "estimated_cost_usd": LENS_COST_USD,
            "requires": "explicit human confirmation + confirmed AP2 spend envelope — this tool never dispatches or reserves",
        }
    return out


def world_probe(session_id: str, task_type: str, target_system: str) -> dict:
    """Return a cost estimate for a physical measurement — WITHOUT running it.

    Estimate-first governance: the caller gets the price and the required
    envelope, then a human dispatches through the console. This tool NEVER
    executes, reserves, or spends.
    """
    allowed = {"images2PosedRGBD", "atlasTextToImage", "atlasGenerate", "atlasChisel"}
    if task_type not in allowed:
        return {"ok": False, "error": f"unknown task_type {task_type!r}: use one of {sorted(allowed)}"}
    if not target_system:
        return {"ok": False, "error": "target_system is required"}
    return {
        "ok": True,
        "will_execute": False,
        "estimated_cost_usd": LENS_COST_USD,
        "requires": "confirmed AP2 spend envelope + human dispatch via the console lens rail",
        "task_type": task_type,
        "target_system": target_system,
        "session_id": session_id,
    }


def _finite_vec(v: object, n: int) -> bool:
    if not isinstance(v, (list, tuple)) or len(v) != n:
        return False
    return all(isinstance(x, (int, float)) and x == x and abs(x) != float("inf") for x in v)


def world_anchor(session_id: str, evidence_refs: list, pose: dict | None, note: str) -> dict:
    """Attach a hypothesis/finding to a world pose. Requires evidence refs."""
    if not evidence_refs:
        return {"ok": False, "error": "anchor refused: evidence_refs must be non-empty (no evidence-free claims)"}
    if not note:
        return {"ok": False, "error": "anchor refused: note is required"}
    return _append_annotation(session_id, {"world_tool": "anchor", "evidence_refs": evidence_refs, "pose": pose or {}, "note": note})


def world_gaze(session_id: str, position: list, quaternion: list, intent: str) -> dict:
    """Stream the agent's orientation, frustum cone, and intent line.

    Always on; cannot be disabled by the agent. Malformed poses are refused,
    never silently normalized.
    """
    if not _finite_vec(position, 3):
        return {"ok": False, "error": "gaze refused: position must be 3 finite numbers"}
    if not _finite_vec(quaternion, 4):
        return {"ok": False, "error": "gaze refused: quaternion must be 4 finite XYZW numbers"}
    if sum(x * x for x in quaternion) < 1e-18:
        return {"ok": False, "error": "gaze refused: degenerate (zero-norm) quaternion"}
    return _append_annotation(
        session_id, {"world_tool": "gaze", "position": list(position), "quaternion": list(quaternion), "intent": intent or ""}
    )


def world_summon(session_id: str, reason: str, level: str = "notable") -> dict:
    """Request the Attention Director alert a human. Rate-limited per session
    (3/hour); the Director may delay, batch, or refuse based on priority."""
    if level not in ("notable", "anomalous"):
        return {"ok": False, "error": f"unknown summon level {level!r}: notable or anomalous"}
    if not reason:
        return {"ok": False, "error": "summon refused: reason is required"}
    now = time.time()
    window = [t for t in _summon_log.get(session_id, []) if now - t < _SUMMON_WINDOW_S]
    if len(window) >= _SUMMON_MAX_PER_WINDOW:
        return {"ok": True, "suppressed": True, "reason": "summon budget exhausted for this session-hour (director backpressure)"}
    window.append(now)
    _summon_log[session_id] = window
    r = _append_annotation(session_id, {"world_tool": "summon", "reason": reason, "level": level})
    r["suppressed"] = False
    return r


def world_handoff(session_id: str, to_agent: str, camera: dict, context_note: str) -> dict:
    """Pass a camera pose and campaign context to a peer agent. Sealed as a
    handoff event in the ledger."""
    if not to_agent:
        return {"ok": False, "error": "handoff refused: to_agent is required"}
    if not isinstance(camera, dict) or not camera:
        return {"ok": False, "error": "handoff refused: camera pose is required"}
    return _append_annotation(
        session_id, {"world_tool": "handoff", "to_agent": to_agent, "camera": camera, "context_note": context_note or ""}
    )


def world_replay(session_id: str, limit: int = 50) -> dict:
    """Return historical trajectory paths for scrubbable playback. Read-only:
    prediction and annotation events newest-first, capped."""
    try:
        n = max(1, min(int(limit), 200))
    except (TypeError, ValueError):
        return {"ok": False, "error": "replay refused: limit must be an integer 1..200"}
    r = _post("/api/ledger/export", {"session_id": session_id, "format": "json"})
    if not r.get("ok"):
        return {"ok": False, "error": r.get("error", "ledger export failed")}
    data = r.get("data") or {}
    events = data.get("events") or data.get("records") or []
    kinds = {"prediction_sealed", "prediction_evaluated", "annotation"}
    traj = [e for e in events if isinstance(e, dict) and e.get("event_type") in kinds][:n]
    return {"ok": True, "count": len(traj), "trajectory": traj}


mcp.add_tool(world_look, name="world_look", description="Return nearby ledger records and camera context. Read-only.")
mcp.add_tool(world_probe, name="world_probe", description="Cost estimate for a measurement. Never executes — dispatch stays human-driven.")
mcp.add_tool(world_anchor, name="world_anchor", description="Attach a hypothesis to a world pose. Requires evidence refs.")
mcp.add_tool(world_gaze, name="world_gaze", description="Stream agent orientation and intent. Always on, append-only.")
mcp.add_tool(world_summon, name="world_summon", description="Request a human alert. Rate-limited; the Director may delay or refuse.")
mcp.add_tool(world_handoff, name="world_handoff", description="Pass camera pose + context to a peer agent. Sealed in the ledger.")
mcp.add_tool(world_replay, name="world_replay", description="Historical trajectory paths for scrubbable playback. Read-only.")
