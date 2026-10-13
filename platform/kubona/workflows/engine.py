"""Minimal workflow engine: YAML steps -> MCP tools with gates.

Workflow shape:
  name, version, trigger (manual|schedule|event), schedule (opaque mapping),
  created_by_agent (bool), budget_ceiling_usd, steps: [{id, tool, args,
  requires_approval}].
Trigger discipline: only manual runs by default — schedule/event never fire
(no scheduler exists); run() refuses them without force=True, dry_run=True
always plans without touching tools. Unknown triggers refuse at parse.
Args may template {{session_id}}, {{prev.<step_id>.<field>}}, {{input.<field>}}.
Each executed step is written to the ledger (kind="workflow_step") so every
outcome traces to a record (§13.4/provenance-first).
"""

from __future__ import annotations

import copy
import json
import re
import time
import uuid
from dataclasses import dataclass, field
from typing import Any, Callable

from kubona.mcp.gateway import KubonaGateway
from kubona.provenance import Ledger

_TEMPLATE = re.compile(r"\{\{\s*([^}]+)\s*\}\}")

# tools that take a session_id (only these get automatic injection)
SESSION_ID_TOOLS = {
    "cost/estimate_cost", "execution/submit_job", "execution/session_status",
    "execution/close_session", "cost/session_burn_rate", "cost/quote_payment",
    "cost/verify_payment",
}


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
        return cls(name=d["name"], steps=steps, version=d.get("version", 1),
                   trigger=trigger, schedule=schedule or {},
                   created_by_agent=d.get("created_by_agent", False),
                   budget_ceiling_usd=d.get("budget_ceiling_usd"))

    def requires_approval(self) -> bool:
        return self.created_by_agent or any(s.get("requires_approval") for s in self.steps)


def parse_workflow(source: str | dict[str, Any]) -> Workflow:
    if isinstance(source, dict):
        return Workflow.from_dict(source)
    import yaml

    return Workflow.from_dict(yaml.safe_load(source))


def _templated(value: Any, ctx: dict[str, Any]) -> Any:
    if isinstance(value, str):
        def sub(m):
            path = m.group(1).strip().split(".")
            node: Any = ctx
            for k in path:
                node = node.get(k, {}) if isinstance(node, dict) else None
                if node is None:
                    return ""
            return json.dumps(node) if isinstance(node, (dict, list)) else str(node)
        return _TEMPLATE.sub(sub, value)
    if isinstance(value, dict):
        return {k: _templated(v, ctx) for k, v in value.items()}
    if isinstance(value, list):
        return [_templated(v, ctx) for v in value]
    return value


class WorkflowEngine:
    def __init__(self, gateway: KubonaGateway, ledger: Ledger | None = None):
        self.gateway = gateway
        self.ledger = ledger

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
        if workflow.status in ("running",):
            raise WorkflowError("workflow already running")

        prev: dict[str, Any] = {}
        ctx: dict[str, Any] = {"session_id": session_id or "", "input": inputs or {}, "prev": {}}

        if dry_run:
            # Plan only: validate scopes, template every step, touch nothing.
            unknown = self.validate(workflow)
            if unknown:
                raise WorkflowError(f"dry run refused: unknown tools {unknown}")
            planned = []
            for step in workflow.steps:
                args = _templated(copy.deepcopy(step.get("args", {})), ctx)
                if step["tool"] in SESSION_ID_TOOLS and session_id and "session_id" not in args:
                    args["session_id"] = session_id
                planned.append({"id": step["id"], "tool": step["tool"], "args": args})
                prev[step["id"]] = {"dry_run": True}
                ctx["prev"] = prev
            if self.ledger is not None:
                self.ledger.append("workflow_dry_run", {"workflow": workflow.name,
                                                        "trigger": workflow.trigger,
                                                        "planned_steps": planned,
                                                        "session_id": session_id})
            return {"ok": True, "dry_run": True, "workflow_id": workflow.workflow_id,
                    "steps": planned}

        workflow.status = "running"
        step_records: list[str] = []

        for step in workflow.steps:
            sid, tool = step["id"], step["tool"]
            args = _templated(copy.deepcopy(step.get("args", {})), ctx)
            if tool in SESSION_ID_TOOLS and session_id and "session_id" not in args:
                args["session_id"] = session_id
            server, _, toolname = tool.partition("/")
            try:
                result = self.gateway.call(server, toolname, args, token=token)
            except Exception as e:  # noqa: BLE001 — a failing step fails the workflow
                workflow.status = "failed"
                return {"ok": False, "workflow_id": workflow.workflow_id, "step": sid,
                        "error": str(e), "step_records": step_records}
            prev[sid] = result
            ctx["prev"] = prev
            rec = None
            if self.ledger is not None:
                rec = self.ledger.append("workflow_step", {"workflow": workflow.name,
                                                           "trigger": workflow.trigger,
                                                           "step": sid, "tool": tool,
                                                           "result_summary": str(result)[:400],
                                                           "session_id": session_id})
                step_records.append(rec.record_id)

        workflow.status = "completed"
        return {"ok": True, "workflow_id": workflow.workflow_id, "steps": len(workflow.steps),
                "step_records": step_records, "last_results": prev}

    def validate(self, workflow: Workflow) -> list[str]:
        """Every tool must map to a known scope (fail-fast on typos)."""
        from kubona.security.policy import TOOL_SCOPES

        return [s["tool"] for s in workflow.steps if s["tool"] not in TOOL_SCOPES]
