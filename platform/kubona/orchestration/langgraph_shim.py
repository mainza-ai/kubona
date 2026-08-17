"""LangGraph shim — Kubona tools as LangGraph tools + a VQE StateGraph.

Reference pattern for the §2.4 compatibility layer: whatever framework a team
already runs (LangGraph/CrewAI/AutoGen/custom mesh) can drive Kubona through
its MCP tools; the shim adds the graph plumbing, not a parallel API.
"""

from __future__ import annotations

import math
from typing import Any, Callable, TypedDict

from kubona.circuits import parse_qasm2
from kubona.cost.governance import CostGovernanceService
from kubona.providers import get_provider
from kubona.session import SessionStore

# -- tools exposed to the framework (the same ones the MCP servers expose) --
TOOL_SCHEMAS: dict[str, Callable[..., dict]] = {}


def _tool(fn: Callable[..., dict]) -> Callable[..., dict]:
    TOOL_SCHEMAS[fn.__name__] = fn
    return fn


@_tool
def estimate_and_preflight(circuit_qasm: str, session_id: str,
                           shots: int = 4096) -> dict:
    from kubona.schema import load_provider

    circuit = parse_qasm2(circuit_qasm)
    gov = CostGovernanceService(SessionStore())
    session = gov.sessions.get(session_id)
    pf = gov.preflight(load_provider("local-simulator"), circuit, shots, session)
    return {"ok": pf["ok"], "reason": pf["reason"], "estimate": pf["estimate"]}


@_tool
def run_simulator(circuit_qasm: str, shots: int = 4096, seed: int = 7) -> dict:
    circuit = parse_qasm2(circuit_qasm)
    res = get_provider("local-simulator").submit(circuit, shots, seed=seed)
    return {"ok": True, **res}


class VQEState(TypedDict, total=False):
    session_id: str
    thetas: list[float]
    energies: list[float]
    step: int
    best_energy: float
    circuit_qasm: str
    done: bool


def _energy_of(qasm: str) -> float:
    """<H>(theta) = <Z0Z1> + 0.5<X0X1> for the symmetric RY-RY-CX ansatz.

    Matches the P1 gate test: <Z0Z1> from the computational basis, <X0X1> from
    the rotated basis (H on both qubits before measurement).
    """
    sim = get_provider("local-simulator")
    counts = sim.submit(parse_qasm2(qasm), 8192, seed=3)["counts"]
    total = sum(counts.values()) or 1
    zz = sum(c * (1.0 if int(k, 2) in (0, 3) else -1.0) for k, c in counts.items()) / total
    rotated = parse_qasm2(qasm.replace(";\ncx q[0],q[1];", ";\ncx q[0],q[1];\nh q[0];\nh q[1];"))
    counts_x = sim.submit(rotated, 8192, seed=3)["counts"]
    xx = sum(c * (1.0 if int(k, 2) in (0, 3) else -1.0) for k, c in counts_x.items()) / total
    return zz + 0.5 * xx


def make_step(theta: float, theta_idx: int) -> Callable[[VQEState], VQEState]:
    def step(state: VQEState) -> VQEState:
        qasm = ("OPENQASM 2.0;\nqreg q[2];\nry({t}) q[0];\nry({t}) q[1];\ncx q[0],q[1];"
                .format(t=round(theta, 4)))
        e = _energy_of(qasm)
        return {"thetas": state.get("thetas", []) + [theta],
                "energies": state.get("energies", []) + [e],
                "step": theta_idx + 1, "circuit_qasm": qasm, "done": False}
    return step


def finalize(state: VQEState) -> VQEState:
    es = state.get("energies", [])
    return {**state, "best_energy": min(es) if es else 0.0, "done": True}


def build_vqe_graph():
    """Return a compiled LangGraph StateGraph running a 3-point VQE scan."""
    try:
        from langgraph.graph import END, START, StateGraph
    except ImportError as e:  # pragma: no cover
        raise RuntimeError("langgraph not installed: pip install langgraph") from e

    g = StateGraph(VQEState)
    g.add_node("finalize", finalize)
    thetas = [2.6, 3.2, 3.6]
    g.add_node("step0", make_step(thetas[0], 0))
    g.add_node("step1", make_step(thetas[1], 1))
    g.add_node("step2", make_step(thetas[2], 2))
    # sequential edges: one update per channel per step (LangGraph LastValue rule)
    g.add_edge(START, "step0")
    g.add_edge("step0", "step1")
    g.add_edge("step1", "step2")
    g.add_edge("step2", "finalize")
    g.add_edge("finalize", END)
    return g.compile()


def kubona_tools() -> dict[str, Callable[..., dict]]:
    return dict(TOOL_SCHEMAS)
