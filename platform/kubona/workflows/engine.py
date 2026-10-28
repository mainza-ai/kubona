"""Minimal workflow engine: YAML steps -> MCP tools with gates.

Workflow shape:
  name, version, trigger (manual|schedule|event), schedule (opaque mapping),
  created_by_agent (bool), budget_ceiling_usd, steps: [{id, tool, args,
  requires_approval}], on_failure: [{id, tool, args}] (compensation steps).
Trigger discipline: only manual runs by default — schedule/event never fire
(no scheduler exists); run() refuses them without force=True, dry_run=True
always plans without touching tools. Unknown triggers refuse at parse.
Args may template {{session_id}}, {{prev.<step_id>.<field>}}, {{input.<field>}}.

Budget ceiling (W2.6): `budget_ceiling_usd` BINDS. The engine tracks spend
across the run — declared per-step costs (`step["cost_usd"]`) plus reported
spend in step results (`cost_usd` / `spent_usd` / `charged_usd` top-level
numbers) — and refuses any step whose projected cost would breach the
ceiling, exactly like CostGovernanceService.record_spend refuses over-ceiling
writes. Server-side session ceilings still apply per call; this ceiling is
workflow-scoped on top. A ceiling that doesn't bind is a lie, so `None`
means "no engine ceiling" and any number binds.

Template discipline (W2.6): missing keys REFUSE. A real run raises
WorkflowError naming the unresolvable reference — silent "" rendering hid
typos and wired empty strings into jobs. dry_run cannot know runtime result
fields, so it plans with explicit `{{UNRESOLVED:<path>}}` markers and lists
every unresolved reference in the plan (`"unresolved": [...]`) instead of
pretending.

Compensation semantics (W2.6): `on_failure` steps are compensating actions.
They run, in order, only after a main-step failure (exception, budget
refusal, or template refusal). Each compensation is best-effort: its outcome
(success or error) is recorded to the ledger (kind="workflow_compensation")
and returned under `"compensations"` — a failing compensation never masks
the original error. Compensations do not run on success and never run during
dry_run (they are listed as `"compensations_planned"`).

Session-id injection (W2.6): dynamic discovery, never a stale literal. A
tool gets automatic `session_id` injection iff its registered function
accepts a `session_id` parameter (inspected live on the gateway registry)
or its tool name starts with `session_` — or it is named in the explicit
`session_id_tools` override passed to the engine. New session tools are
picked up without editing this file.

Status discipline (W2.6): the draft -> running -> completed|failed
check-and-set runs under a threading + file lock (same pattern as
CostGovernanceService._money_locked), so two concurrent engines cannot both
claim the same workflow object.

Each executed step is written to the ledger (kind="workflow_step") so every
outcome traces to a record (§13.4/provenance-first).
"""

from __future__ import annotations

import copy
import inspect
import json
import re
import threading
import time
import uuid
from contextlib import contextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterator

from kubona.mcp.gateway import KubonaGateway
from kubona.provenance import Ledger

try:
    import fcntl  # POSIX: cross-process file lock; absent on Windows
    _HAS_FCNTL = True
except ImportError:  # pragma: no cover — Windows fallback: threading lock only
    fcntl = None  # type: ignore[assignment]
    _HAS_FCNTL = False

_TEMPLATE = re.compile(r"\{\{\s*([^}]+)\s*\}\}")

# Prefix heuristic for session-id injection (W2.6): tools named session_*
# take a session by convention. The authoritative rule is the live signature
# check below — the prefix only covers dynamically-registered callables whose
# signatures cannot be inspected.
SESSION_ID_TOOL_PREFIX = "session_"

# Result keys that count as reported spend against the workflow ceiling.
# Estimates (estimated_usd) are NOT spend — only realized numbers bind.
_SPEND_KEYS = ("cost_usd", "spent_usd", "charged_usd")


class WorkflowError(RuntimeError):
    pass


@dataclass
class Workflow:
    name: str
    steps: list[dict[str, Any]]
    version: int = 1
    trigger: str = "manual"
    # Opaque schedule description (cron/interval/…): stored and stamped, never
    # interpreted — no scheduler exists yet, so schedule/event triggers never
    # fire on their own (run() refuses them without force; see below).
    schedule: dict[str, Any] = field(default_factory=dict)
    created_by_agent: bool = False
    budget_ceiling_usd: float | None = None
    approved: bool = False
    approved_by: str | None = None
    workflow_id: str = field(default_factory=lambda: uuid.uuid4().hex[:16])
    status: str = "draft"  # draft | running | completed | failed | revoked
    # Compensating actions: run in order after a main-step failure (W2.6).
    on_failure: list[dict[str, Any]] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Workflow":
        steps = d.get("steps")
        if not steps or not isinstance(steps, list) or not all("id" in s and "tool" in s for s in steps):
            raise WorkflowError("workflow needs a non-empty list of {id, tool, ...} steps")
        trigger = d.get("trigger", "manual")
        if trigger not in ("manual", "schedule", "event"):
            raise WorkflowError(f"unknown trigger {trigger!r}: manual|schedule|event")
        schedule = d.get("schedule", {})
        if trigger in ("schedule", "event") and schedule and not isinstance(schedule, dict):
            raise WorkflowError("schedule must be a mapping describing the trigger")
        on_failure = d.get("on_failure", [])
        if on_failure and (not isinstance(on_failure, list)
                           or not all("id" in s and "tool" in s for s in on_failure)):
            raise WorkflowError("on_failure needs a list of {id, tool, ...} steps")
        ceiling = d.get("budget_ceiling_usd")
        if ceiling is not None:
            import math
            if (not isinstance(ceiling, (int, float)) or not math.isfinite(ceiling)
                    or ceiling < 0):
                raise WorkflowError(
                    f"budget_ceiling_usd must be finite >= 0 or None, got {ceiling!r}")
        return cls(name=d["name"], steps=steps, version=d.get("version", 1),
                   trigger=trigger, schedule=schedule or {},
                   created_by_agent=d.get("created_by_agent", False),
                   budget_ceiling_usd=ceiling,
                   workflow_id=d.get("workflow_id") or uuid.uuid4().hex[:16],
                   on_failure=on_failure)

    def requires_approval(self) -> bool:
        return self.created_by_agent or any(s.get("requires_approval") for s in self.steps)


def parse_workflow(source: str | dict[str, Any]) -> Workflow:
    if isinstance(source, dict):
        return Workflow.from_dict(source)
    import yaml

    return Workflow.from_dict(yaml.safe_load(source))


def _resolve_path(ctx: dict[str, Any], path: list[str]) -> tuple[bool, Any]:
    node: Any = ctx
    for k in path:
        if isinstance(node, dict) and k in node:
            node = node[k]
        else:
            return False, None
    return True, node


def _render_strict(value: Any, ctx: dict[str, Any]) -> Any:
    """Render templates, REFUSING on missing keys (fail-closed, W2.6)."""
    if isinstance(value, str):
        def sub(m: re.Match) -> str:
            raw = m.group(1).strip()
            path = raw.split(".")
            ok, node = _resolve_path(ctx, path)
            if not ok or node is None:
                raise WorkflowError(
                    f"template reference {{{{{raw}}}}} does not resolve "
                    "(missing key — refusing silent empty render)")
            return json.dumps(node) if isinstance(node, (dict, list)) else str(node)
        return _TEMPLATE.sub(sub, value)
    if isinstance(value, dict):
        return {k: _render_strict(v, ctx) for k, v in value.items()}
    if isinstance(value, list):
        return [_render_strict(v, ctx) for v in value]
    return value


def _render_dry_run(value: Any, ctx: dict[str, Any],
                    unresolved: list[str]) -> Any:
    """Render templates for planning: unresolvable refs become explicit
    {{UNRESOLVED:<path>}} markers AND are appended to `unresolved` — never a
    silent "" (W2.6). Runtime result fields cannot be known before execution,
    so dry-run honesty is explicit marking, not refusal."""
    if isinstance(value, str):
        def sub(m: re.Match) -> str:
            raw = m.group(1).strip()
            path = raw.split(".")
            ok, node = _resolve_path(ctx, path)
            if not ok or node is None:
                unresolved.append(raw)
                return "{{UNRESOLVED:" + raw + "}}"
            return json.dumps(node) if isinstance(node, (dict, list)) else str(node)
        return _TEMPLATE.sub(sub, value)
    if isinstance(value, dict):
        return {k: _render_dry_run(v, ctx, unresolved) for k, v in value.items()}
    if isinstance(value, list):
        return [_render_dry_run(v, ctx, unresolved) for v in value]
    return value


def _tool_takes_session_id(fn: Callable) -> bool:
    """Live signature check: does this tool function accept session_id?"""
    try:
        params = inspect.signature(fn).parameters
    except (TypeError, ValueError):
        return False
    if "session_id" in params:
        return True
    # **kwargs alone does not prove session support — uninspectable or
    # variadic-only callables qualify only via the session_ prefix rule.
    return False


def discover_session_id_tools(gateway: KubonaGateway) -> set[str]:
    """Dynamically discover which tool paths take a session_id (W2.6).

    Reads the live gateway registry: a tool qualifies iff its function
    accepts a `session_id` parameter or its tool name starts with
    `session_`. New session tools are injected with no edit here.
    """
    out: set[str] = set()
    registry: dict[str, dict[str, Callable]] = getattr(gateway, "_registry", {})
    for server, tools in registry.items():
        for name, fn in tools.items():
            if name.startswith(SESSION_ID_TOOL_PREFIX) or _tool_takes_session_id(fn):
                out.add(f"{server}/{name}")
    return out


def _step_declared_cost(step: dict[str, Any]) -> float:
    for key in ("cost_usd", "estimated_cost_usd"):
        v = step.get(key)
        if isinstance(v, (int, float)) and v == v and abs(v) != float("inf"):
            if v < 0:
                raise WorkflowError(f"step {step.get('id')!r}: {key} must be >= 0, got {v!r}")
            return float(v)
    return 0.0


def _result_reported_spend(result: Any) -> float:
    if not isinstance(result, dict):
        return 0.0
    total = 0.0
    for key in _SPEND_KEYS:
        v = result.get(key)
        if isinstance(v, (int, float)) and v == v and abs(v) != float("inf") and v > 0:
            total += float(v)
    return total


class WorkflowEngine:
    def __init__(self, gateway: KubonaGateway, ledger: Ledger | None = None,
                 session_id_tools: set[str] | None = None,
                 lock_path: str | Path | None = None):
        self.gateway = gateway
        self.ledger = ledger
        # Explicit registry override (W2.6): when given, it replaces dynamic
        # discovery. None (default) means discover live from the gateway.
        self._session_id_tools = session_id_tools
        self._lock = threading.Lock()
        if lock_path is None:
            from kubona.config import state_path
            lock_path = state_path("workflows.lock")
        self._lock_path = Path(lock_path)
        self._lock_path.parent.mkdir(parents=True, exist_ok=True)

    @contextmanager
    def _status_locked(self) -> Iterator[None]:
        """Serialize the status check-and-set across threads and processes
        sharing this engine's state dir (same shape as
        CostGovernanceService._money_locked)."""
        with self._lock:
            if _HAS_FCNTL:
                with open(self._lock_path, "a+b") as lf:
                    fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
                    try:
                        yield
                    finally:
                        fcntl.flock(lf.fileno(), fcntl.LOCK_UN)
            else:  # pragma: no cover — non-POSIX fallback
                yield

    def _takes_session_id(self, tool_path: str) -> bool:
        if self._session_id_tools is not None:
            return tool_path in self._session_id_tools
        return tool_path in discover_session_id_tools(self.gateway)

    def _inject_session_id(self, tool: str, args: dict[str, Any],
                           session_id: str | None) -> dict[str, Any]:
        if self._takes_session_id(tool) and session_id and "session_id" not in args:
            args["session_id"] = session_id
        return args

    def _check_ceiling(self, workflow: Workflow, spent: float,
                       projected: float, step_id: str) -> None:
        ceiling = workflow.budget_ceiling_usd
        if ceiling is not None and spent + projected > ceiling:
            raise WorkflowError(
                f"workflow {workflow.name}: step {step_id!r} refused — "
                f"projected spend {spent + projected:.6f} would breach ceiling "
                f"{ceiling:.6f} (spent {spent:.6f} + step {projected:.6f})")

    def run(self, workflow: Workflow, *, token: str, session_id: str | None = None,
            inputs: dict[str, Any] | None = None, dry_run: bool = False,
            force: bool = False) -> dict[str, Any]:
        if workflow.requires_approval() and not workflow.approved:
            raise WorkflowError(f"workflow {workflow.name} requires human approval (created_by_agent={workflow.created_by_agent})")
        if workflow.trigger != "manual" and not force and not dry_run:
            raise WorkflowError(
                f"workflow {workflow.name} has trigger {workflow.trigger!r}: no scheduler "
                "is attached, so it never fires on its own — pass force=True to run it "
                "manually and own the consequences, or dry_run=True to plan it")
        with self._status_locked():
            if workflow.status in ("running",):
                raise WorkflowError("workflow already running")
            if not dry_run:
                workflow.status = "running"

        prev: dict[str, Any] = {}
        ctx: dict[str, Any] = {"session_id": session_id or "", "input": inputs or {}, "prev": {}}

        if dry_run:
            # Plan only: validate scopes, template every step, touch nothing.
            unknown = self.validate(workflow)
            if unknown:
                raise WorkflowError(f"dry run refused: unknown tools {unknown}")
            planned = []
            unresolved: list[str] = []
            for step in workflow.steps:
                args = _render_dry_run(copy.deepcopy(step.get("args", {})), ctx, unresolved)
                args = self._inject_session_id(step["tool"], args, session_id)
                planned.append({"id": step["id"], "tool": step["tool"], "args": args})
                prev[step["id"]] = {"dry_run": True}
                ctx["prev"] = prev
            comp_planned = [{"id": s["id"], "tool": s["tool"]} for s in workflow.on_failure]
            if self.ledger is not None:
                self.ledger.append("workflow_dry_run", {"workflow": workflow.name,
                                                        "trigger": workflow.trigger,
                                                        "planned_steps": planned,
                                                        "unresolved": sorted(set(unresolved)),
                                                        "compensations_planned": comp_planned,
                                                        "session_id": session_id})
            return {"ok": True, "dry_run": True, "workflow_id": workflow.workflow_id,
                    "steps": planned, "unresolved": sorted(set(unresolved)),
                    "compensations_planned": comp_planned}

        spent_usd = 0.0
        step_records: list[str] = []

        for step in workflow.steps:
            sid, tool = step["id"], step["tool"]
            try:
                args = _render_strict(copy.deepcopy(step.get("args", {})), ctx)
            except WorkflowError as e:
                return self._fail(workflow, session_id, step_records, sid, str(e),
                                  ctx, prev, spent_usd, token)
            args = self._inject_session_id(tool, args, session_id)
            try:
                self._check_ceiling(workflow, spent_usd, _step_declared_cost(step), sid)
            except WorkflowError as e:
                return self._fail(workflow, session_id, step_records, sid, str(e),
                                  ctx, prev, spent_usd, token)
            server, _, toolname = tool.partition("/")
            try:
                result = self.gateway.call(server, toolname, args, token=token)
            except Exception as e:  # noqa: BLE001 — a failing step fails the workflow
                return self._fail(workflow, session_id, step_records, sid, str(e),
                                  ctx, prev, spent_usd, token)
            if isinstance(result, dict) and result.get("ok") is False:
                err_msg = str(result.get("error") or result.get("rejection_message") or f"step {sid} returned ok=False")
                prev[sid] = result
                ctx["prev"] = prev
                return self._fail(workflow, session_id, step_records, sid, err_msg,
                                  ctx, prev, spent_usd, token)
            spent_usd += _result_reported_spend(result)
            try:
                self._check_ceiling(workflow, spent_usd, 0.0, sid + " (reported spend)")
            except WorkflowError as e:
                prev[sid] = result
                ctx["prev"] = prev
                return self._fail(workflow, session_id, step_records, sid, str(e),
                                  ctx, prev, spent_usd, token)
            prev[sid] = result
            ctx["prev"] = prev
            rec = None
            if self.ledger is not None:
                try:
                    rec = self.ledger.append("workflow_step", {"workflow": workflow.name,
                                                               "trigger": workflow.trigger,
                                                               "step": sid, "tool": tool,
                                                               "result_summary": str(result)[:400],
                                                               "spent_usd": spent_usd,
                                                               "session_id": session_id})
                except Exception as e:  # noqa: BLE001 — no receipt means no completed claim
                    with self._status_locked():
                        workflow.status = "failed"
                    return {"ok": False, "workflow_id": workflow.workflow_id,
                            "steps": len(workflow.steps), "step_records": step_records,
                            "error": f"workflow receipt failed after step {sid}: {e}"}
                step_records.append(rec.record_id)

        with self._status_locked():
            workflow.status = "completed"
        return {"ok": True, "workflow_id": workflow.workflow_id, "steps": len(workflow.steps),
                "step_records": step_records, "last_results": prev, "spent_usd": spent_usd}

    def _fail(self, workflow: Workflow, session_id: str | None,
              step_records: list[str], sid: str, error: str,
              ctx: dict[str, Any], prev: dict[str, Any], spent_usd: float,
              token: str) -> dict[str, Any]:
        """Fail the workflow, then run compensating actions best-effort."""
        with self._status_locked():
            workflow.status = "failed"
        compensations: list[dict[str, Any]] = []
        for step in workflow.on_failure:
            cid, tool = step["id"], step["tool"]
            try:
                args = _render_strict(copy.deepcopy(step.get("args", {})), ctx)
            except WorkflowError as e:
                args = {}
                comp_err = f"compensation template refused: {e}"
                compensations.append({"id": cid, "ok": False, "error": comp_err})
                if self.ledger is not None:
                    self.ledger.append("workflow_compensation",
                                       {"workflow": workflow.name, "step": cid,
                                        "tool": tool, "ok": False, "error": comp_err,
                                        "session_id": session_id})
                continue
            args = self._inject_session_id(tool, args, session_id)
            server, _, toolname = tool.partition("/")
            try:
                result = self.gateway.call(server, toolname, args, token=token)
                compensations.append({"id": cid, "ok": True, "result": result})
                if self.ledger is not None:
                    rec = self.ledger.append(
                        "workflow_compensation",
                        {"workflow": workflow.name, "step": cid, "tool": tool,
                         "ok": True, "result_summary": str(result)[:400],
                         "session_id": session_id})
                    step_records.append(rec.record_id)
            except Exception as e:  # noqa: BLE001 — best-effort: record, never mask
                compensations.append({"id": cid, "ok": False, "error": str(e)})
                if self.ledger is not None:
                    self.ledger.append("workflow_compensation",
                                       {"workflow": workflow.name, "step": cid,
                                        "tool": tool, "ok": False, "error": str(e),
                                        "session_id": session_id})
        return {"ok": False, "workflow_id": workflow.workflow_id, "step": sid,
                "error": error, "step_records": step_records,
                "spent_usd": spent_usd, "compensations": compensations}

    def validate(self, workflow: Workflow) -> list[str]:
        """Every tool must map to a known scope (fail-fast on typos)."""
        from kubona.security.policy import TOOL_SCOPES

        unknown = [s["tool"] for s in workflow.steps if s["tool"] not in TOOL_SCOPES]
        unknown += [s["tool"] for s in workflow.on_failure if s["tool"] not in TOOL_SCOPES]
        return unknown
