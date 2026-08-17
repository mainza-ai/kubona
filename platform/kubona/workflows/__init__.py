"""Workflow & agent authoring (Phase 6): versioned user/agent-created workflows.

YAML-declared steps -> MCP tools, with step gates (cost / capability). An
agent-created workflow requires human approval before execution; everything is
versioned, reversible, and ledger-recorded. See wiki/concepts/workflows-and-agent-authoring.
"""

from .engine import WorkflowEngine, WorkflowError, parse_workflow
from .library import WorkflowLibrary

__all__ = ["WorkflowEngine", "WorkflowError", "parse_workflow", "WorkflowLibrary"]
