"""Science & Discovery MCP server (Phase 3): bridges external MCP agents
to Kubona's sovereign Go orchestrator (kubonad) for classical anchors,
structured experiments, discovery planning, hypotheses, research graphs,
persistent memory, manuscript verification, and session autonomy.

Every tool fails closed without KUBONA_CORE_URL / KUBONA_API_TOKEN (or when
kubonad refuses under budget, autonomy, or lock gates). Enforcement stays in
Go core — this server is a thin, fail-closed adapter.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from typing import Any
from urllib.parse import quote, urlencode

from mcp.server.mcpserver import MCPServer

mcp = MCPServer(
    "kubona-science",
    instructions=(
        "Kubona Science & Discovery server. Follow Kubona's scientific rule: "
        "always compute a classical anchor (get_classical_anchor) before claiming "
        "a quantum energy, gap, or advantage. Respect session autonomy "
        "(get_session_autonomy) before launching discovery or experiments."
    ),
)

ALLOWED_EXPERIMENT_KINDS = frozenset({
    "vqe",
    "geometry_optimization",
    "vqe_repeats",
    "pes",
    "qaoa",
    "excited_state",
    "reaction",
    "spin_chain",
    "thermodynamic_limit",
    "conjecture",
    "lattice_dynamics",
})


def _core_url() -> str:
    return os.environ.get("KUBONA_CORE_URL", "http://127.0.0.1:8080").rstrip("/")


def _token() -> str | None:
    return os.environ.get("KUBONA_API_TOKEN") or None


def _unconfigured() -> dict:
    return {
        "ok": False,
        "error_code": "server_misconfigured",
        "error": (
            "science tools unconfigured: set KUBONA_CORE_URL and "
            "KUBONA_API_TOKEN (operator credential, never committed)"
        ),
    }


def _request(method: str, path: str, *, params: dict | None = None,
             payload: dict | None = None, timeout: int = 30) -> dict:
    token = _token()
    if not token:
        return _unconfigured()
    url = _core_url() + path
    if params:
        clean = {k: v for k, v in params.items() if v is not None and v != ""}
        if clean:
            url += "?" + urlencode(clean)
    headers = {"Authorization": f"Bearer {token}"}
    data: bytes | None = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            parsed = json.loads(raw) if raw.strip() else {}
            return {"ok": True, "status": resp.status, "data": parsed}
    except urllib.error.HTTPError as e:
        raw = ""
        try:
            raw = e.read().decode("utf-8", errors="replace")
        except Exception:  # noqa: BLE001
            pass
        detail: Any = raw
        error_code = "bad_request" if e.code < 500 else "internal"
        try:
            body = json.loads(raw) if raw.strip() else {}
            if isinstance(body, dict):
                detail = body
                if isinstance(body.get("code"), str):
                    error_code = body["code"]
                elif isinstance(body.get("error_code"), str):
                    error_code = body["error_code"]
        except Exception:  # noqa: BLE001
            pass
        if e.code == 401:
            error_code = "unauthorized"
        elif e.code == 403 and error_code == "bad_request":
            error_code = "forbidden"
        elif e.code == 404 and error_code == "bad_request":
            error_code = "not_found"
        return {"ok": False, "status": e.code, "error_code": error_code, "error": detail}
    except Exception as e:  # noqa: BLE001 — surfaced as structured data
        return {"ok": False, "error_code": "bridge_unavailable", "error": f"kubonad request failed: {e}"}


def _validate_id(value: str, label: str) -> str | None:
    v = (value or "").strip()
    if not v or "/" in v or ".." in v:
        return f"invalid {label}: must be a non-empty identifier without '/' or '..'"
    return None


def get_classical_anchor(
    molecule: str,
    bond_distance_angstrom: float = 0.74,
    basis: str = "sto-3g",
) -> dict:
    """Compute a PySCF classical reference anchor (RHF/FCI) via kubonad + bridge.

    Always call this before claiming a quantum energy, gap, or advantage.
    Fails closed when kubonad or the Python chemistry bridge is unavailable.
    """
    mol = (molecule or "").strip()
    if not mol:
        return {"ok": False, "error_code": "bad_request", "error": "molecule is required"}
    if not (0.1 <= float(bond_distance_angstrom) <= 20.0):
        return {
            "ok": False,
            "error_code": "bad_request",
            "error": "bond_distance_angstrom must be between 0.1 and 20.0",
        }
    return _request(
        "POST",
        "/api/chemistry/anchor",
        payload={
            "molecule": mol,
            "bond_distance_angstrom": float(bond_distance_angstrom),
            "basis": (basis or "sto-3g").strip(),
        },
        timeout=60,
    )


def run_experiment(
    session_id: str,
    kind: str,
    params: dict[str, Any] | None = None,
    budget_usd: float | None = None,
    request_id: str = "",
) -> dict:
    """Run a structured Kubona experiment via POST /api/experiments.

    Supported kinds: vqe, pes, qaoa, spin_chain, geometry_optimization,
    excited_state, reaction, lattice_dynamics, conjecture,
    thermodynamic_limit, vqe_repeats.
    """
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    k = (kind or "").strip()
    if k not in ALLOWED_EXPERIMENT_KINDS:
        return {
            "ok": False,
            "error_code": "bad_request",
            "error": f"unknown experiment kind {k!r}; allowed: {sorted(ALLOWED_EXPERIMENT_KINDS)}",
        }
    spec: dict[str, Any] = {"kind": k}
    if params:
        if not isinstance(params, dict):
            return {"ok": False, "error_code": "bad_request", "error": "params must be a JSON object"}
        spec.update(params)
        spec["kind"] = k
    body: dict[str, Any] = {"session_id": session_id.strip(), "kind": k, "spec": spec}
    if budget_usd is not None:
        if budget_usd < 0:
            return {"ok": False, "error_code": "bad_request", "error": "budget_usd must be >= 0"}
        body["budget_usd"] = float(budget_usd)
    if (request_id or "").strip():
        body["request_id"] = request_id.strip()
    return _request("POST", "/api/experiments", payload=body, timeout=120)


def plan_discovery(goal: str) -> dict:
    """Generate a pre-registered discovery plan via POST /api/discovery/plan."""
    g = (goal or "").strip()
    if not g:
        return {"ok": False, "error_code": "bad_request", "error": "goal is required"}
    return _request("POST", "/api/discovery/plan", payload={"goal": g}, timeout=90)


def run_discovery(
    session_id: str,
    goal: str,
    steps: list[dict[str, Any]],
    step_timeout_s: int = 0,
    request_id: str = "",
    allow_failure: bool = False,
) -> dict:
    """Execute a pre-registered discovery plan via POST /api/discovery/run."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    if not isinstance(steps, list) or not steps:
        return {"ok": False, "error_code": "bad_request", "error": "steps list is required"}
    body: dict[str, Any] = {
        "session_id": session_id.strip(),
        "goal": (goal or "").strip(),
        "steps": steps,
    }
    if step_timeout_s > 0:
        body["step_timeout_s"] = int(step_timeout_s)
    if (request_id or "").strip():
        body["request_id"] = request_id.strip()
    if allow_failure:
        body["allow_failure"] = True
    return _request("POST", "/api/discovery/run", payload=body, timeout=120)


def list_hypotheses(session_id: str) -> dict:
    """List persisted hypotheses and Elo rankings for a session."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    return _request("GET", f"/api/sessions/{quote(session_id.strip(), safe='')}/hypotheses")


def propose_hypothesis(
    session_id: str,
    goal: str = "",
    statement: str = "",
    rationale: str = "",
    falsifier: str = "",
    count: int = 3,
    use_llm: bool = True,
) -> dict:
    """Register a manual hypothesis (statement+rationale) or generate hypotheses (goal+use_llm)."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    stmt = (statement or "").strip()
    g = (goal or "").strip()
    if stmt:
        fals = (falsifier or "").strip()
        if not fals:
            return {
                "ok": False,
                "error_code": "bad_request",
                "error": "falsifier is required when submitting a manual hypothesis",
            }
        payload: dict[str, Any] = {
            "statement": stmt,
            "rationale": (rationale or "").strip(),
            "falsifier": fals,
        }
    elif g:
        payload = {"goal": g, "count": max(1, min(int(count), 10)), "use_llm": bool(use_llm)}
    else:
        return {
            "ok": False,
            "error_code": "bad_request",
            "error": "provide either statement (manual hypothesis) or goal (hypothesis generation)",
        }
    return _request(
        "POST",
        f"/api/sessions/{quote(session_id.strip(), safe='')}/hypotheses",
        payload=payload,
        timeout=120,
    )


def get_research_graph(session_id: str, since_seq: int = 0) -> dict:
    """Read the byte-budgeted research graph summary (resumable via since_seq)."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    if since_seq < 0:
        return {"ok": False, "error_code": "bad_request", "error": "since_seq must be >= 0"}
    return _request(
        "GET",
        f"/api/sessions/{quote(session_id.strip(), safe='')}/research-graph",
        params={"since_seq": since_seq},
    )


def recall_memory(
    session_id: str,
    query: str,
    bank: str = "",
    limit: int = 10,
) -> dict:
    """Recall tenant-scoped scientific memory (hybrid lexical + semantic)."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    if not (query or "").strip():
        return {"ok": False, "error_code": "bad_request", "error": "query is required"}
    if not (1 <= int(limit) <= 50):
        return {"ok": False, "error_code": "bad_request", "error": "limit must be 1..50"}
    return _request(
        "GET",
        "/api/memory/recall",
        params={"session_id": session_id.strip(), "q": query.strip(), "bank": bank, "limit": int(limit)},
    )


def retain_memory(
    session_id: str,
    content: str,
    bank: str = "world-facts",
    confidence: float = 0.8,
) -> dict:
    """Persist a verified scientific finding into tenant-scoped memory."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    if not (content or "").strip():
        return {"ok": False, "error_code": "bad_request", "error": "content is required"}
    if not (0.0 <= float(confidence) <= 1.0):
        return {"ok": False, "error_code": "bad_request", "error": "confidence must be in [0, 1]"}
    return _request(
        "POST",
        "/api/memory/retain",
        payload={
            "session_id": session_id.strip(),
            "content": content.strip(),
            "bank": (bank or "world-facts").strip(),
            "confidence": float(confidence),
        },
    )


def verify_manuscript(session_id: str, manuscript_id: str) -> dict:
    """Run numeric & citation verification on a session manuscript before signoff."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    if err := _validate_id(manuscript_id, "manuscript_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    return _request(
        "POST",
        f"/api/sessions/{quote(session_id.strip(), safe='')}/manuscripts/{quote(manuscript_id.strip(), safe='')}/verify",
        payload={},
        timeout=60,
    )


def get_session_autonomy(session_id: str) -> dict:
    """Inspect session agentic mode (agentic_enabled/locked), stage policy, and pending approvals."""
    if err := _validate_id(session_id, "session_id"):
        return {"ok": False, "error_code": "bad_request", "error": err}
    sid = quote(session_id.strip(), safe="")
    policy = _request("GET", f"/api/sessions/{sid}/autonomy")
    if not policy.get("ok"):
        return policy
    approvals = _request("GET", f"/api/sessions/{sid}/approvals")
    return {
        "ok": True,
        "session_id": session_id.strip(),
        "autonomy": policy.get("data"),
        "pending_approvals": approvals.get("data") if approvals.get("ok") else None,
    }


mcp.add_tool(
    get_classical_anchor,
    name="get_classical_anchor",
    description="Compute PySCF classical reference anchor (RHF/FCI) via kubonad + bridge before claiming any quantum energy.",
)
mcp.add_tool(
    run_experiment,
    name="run_experiment",
    description="Run a structured Kubona experiment (vqe, pes, qaoa, spin_chain, geometry_optimization, reaction, etc.) via kubonad.",
)
mcp.add_tool(
    plan_discovery,
    name="plan_discovery",
    description="Generate a pre-registered scientific discovery plan via kubonad.",
)
mcp.add_tool(
    run_discovery,
    name="run_discovery",
    description="Execute a discovery plan via kubonad (subject to session autonomy and budget gates).",
)
mcp.add_tool(
    list_hypotheses,
    name="list_hypotheses",
    description="List persisted hypotheses and Elo tournament state for a session.",
)
mcp.add_tool(
    propose_hypothesis,
    name="propose_hypothesis",
    description="Register a candidate scientific hypothesis in a session.",
)
mcp.add_tool(
    get_research_graph,
    name="get_research_graph",
    description="Read the byte-budgeted research graph summary for a session (resumable via since_seq).",
)
mcp.add_tool(
    recall_memory,
    name="recall_memory",
    description="Recall tenant-scoped scientific memory (hybrid lexical + semantic) for a session.",
)
mcp.add_tool(
    retain_memory,
    name="retain_memory",
    description="Persist a verified scientific fact into tenant-scoped session memory.",
)
mcp.add_tool(
    verify_manuscript,
    name="verify_manuscript",
    description="Verify numeric claims and citations in a session manuscript before signoff.",
)
mcp.add_tool(
    get_session_autonomy,
    name="get_session_autonomy",
    description="Read session agentic mode (agentic_enabled/locked), stage autonomy policy, and pending approvals.",
)

if __name__ == "__main__":
    mcp.run()
