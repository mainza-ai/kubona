"""Orchestration compatibility shim (Phase 2): LangGraph first.

Maps Kubona MCP tools onto LangGraph tools and provides a reference VQE-loop
StateGraph — proving a third-party orchestration framework can drive Kubona
through the same governed chain (submit -> cost gate -> provenance).
"""

from .langgraph_shim import build_vqe_graph, kubona_tools

__all__ = ["build_vqe_graph", "kubona_tools"]
