"""LangGraph shim: a third-party framework drives Kubona through its tools."""

import pytest

lg = pytest.importorskip("langgraph")

from kubona.orchestration import build_vqe_graph


def test_vqe_graph_completes_with_best_energy():
    graph = build_vqe_graph()
    state = graph.invoke({"session_id": "s1"})
    assert state["done"] is True
    assert len(state["thetas"]) == 3
    assert state["best_energy"] < -1.0, f"VQE via LangGraph should approach ground -1.118, got {state['best_energy']}"
