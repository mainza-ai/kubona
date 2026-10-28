"""Qiskit/Aer as a development provider — open-source, no credentials.

Answers the maintainer direction: "we can test with Qiskit while we're
developing." Runs the governed chain (session -> preflight -> submit -> result
-> provenance) over the QiskitProvider (AerSimulator), the same path a real
IBM backend will take once credentials exist.
"""

import json
import os
import sys

import pytest

qiskit = pytest.importorskip("qiskit")

from kubona.circuits import parse_qasm2
from kubona.providers import get_provider
from kubona.schema import load_all_providers


def test_qiskit_provider_submits_bell():
    provider = get_provider("qiskit-aer-simulator")
    circuit = parse_qasm2("OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];")
    res = provider.submit(circuit, shots=2048, seed=42)
    assert set(res["counts"]) <= {"00", "11"}
    assert sum(res["counts"].values()) == 2048


def test_qiskit_provider_in_registry():
    caps = load_all_providers()
    assert caps["qiskit-aer-simulator"].status == "GA"
    assert caps["qiskit-aer-simulator"].pricing.model == "free"


@pytest.mark.asyncio
async def test_gateway_chain_over_qiskit():
    """The P1-style chain (cost -> execution -> provenance) on the Qiskit provider.

    NOTE: mcp 2.0's stdio client hangs when closing one of several concurrently
    open sessions (upstream quirk); tests use one session at a time, and the
    execution server is held open only for submit + get_result (its job store is
    per-process, mirroring the stateless-server/session model).
    """
    from mcp import ClientSession, StdioServerParameters
    from mcp.client.stdio import stdio_client

    ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    BELL = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"

    async def one_call(module, tool, args):
        params = StdioServerParameters(command=sys.executable, args=["-m", module],
                                       cwd=ROOT, env={**os.environ, "PYTHONPATH": ROOT})
        async with stdio_client(params) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                result = await session.call_tool(tool, args)
                return json.loads(result.content[0].text)

    async def exec_chain(calls):
        """Multiple calls on ONE execution-server session (per-process job state)."""
        params = StdioServerParameters(command=sys.executable, args=["-m", "kubona.mcp.servers.execution"],
                                       cwd=ROOT, env={**os.environ, "PYTHONPATH": ROOT})
        client = stdio_client(params)
        read, write = await client.__aenter__()
        session = ClientSession(read, write)
        await session.__aenter__()
        await session.initialize()
        try:
            out = []
            for tool, args in calls:
                result = await session.call_tool(tool, args)
                out.append(json.loads(result.content[0].text))
            return out
        finally:
            await session.__aexit__(None, None, None)
            await client.__aexit__(None, None, None)

    async def exec_chain_submit_result(bell: str, sid: str):
        """submit_job then get_result on the same execution-server session."""
        params = StdioServerParameters(command=sys.executable, args=["-m", "kubona.mcp.servers.execution"],
                                       cwd=ROOT, env={**os.environ, "PYTHONPATH": ROOT})
        client = stdio_client(params)
        read, write = await client.__aenter__()
        session = ClientSession(read, write)
        await session.__aenter__()
        await session.initialize()
        try:
            r1 = await session.call_tool("submit_job", {
                "circuit_qasm": bell, "provider": "qiskit-aer-simulator", "shots": 2048,
                "session_id": sid, "seed": 42})
            job = json.loads(r1.content[0].text)
            r2 = await session.call_tool("get_result", {"job_id": job["job_id"]})
            return job, json.loads(r2.content[0].text)
        finally:
            await session.__aexit__(None, None, None)
            await client.__aexit__(None, None, None)

    # 1. cost server: create session + preflight (one-shot)
    sess = await one_call("kubona.mcp.servers.cost", "create_session",
                          {"name": "qiskit-dev", "owner": "researcher", "budget_ceiling_usd": 100.0})
    est = await one_call("kubona.mcp.servers.cost", "estimate_cost",
                         {"circuit_qasm": BELL, "provider": "qiskit-aer-simulator", "shots": 2048,
                          "session_id": sess["session_id"]})
    assert est["preflight_ok"] is True

    # 2. execution server: submit + get_result on ONE session (job store is per-process)
    job, result = await exec_chain_submit_result(BELL, sess["session_id"])
    assert job["ok"] and job["status"] == "completed", job
    assert result["ok"] and set(result["result"]["counts"]) <= {"00", "11"}

    # 3. provenance server: record + chain check (one-shot)
    rec = await one_call("kubona.mcp.servers.provenance", "write_record", {
        "kind": "experiment",
        "payload": {"job_id": job["job_id"], "circuit_hash": job["circuit_hash"],
                    "provider": "qiskit-aer-simulator", "shots": 2048,
                    "results": result["result"]["counts"], "cost_usd": 0.0}})
    assert rec["chain_ok"] is True


def test_calibration_snapshot_parsing_skips_missing_and_measurement():
    from types import SimpleNamespace
    from datetime import datetime, timezone
    from kubona.providers.qiskit_provider import _snapshot_from_properties

    class Props:
        qubits = [None, None]
        last_update_date = datetime(2026, 10, 27, 12, 0, tzinfo=timezone.utc)
        gates = [
            SimpleNamespace(name="sx0", qubits=[0], parameters=[SimpleNamespace(value=0.0002)]),
            SimpleNamespace(name="x1", qubits=[1], parameters=[SimpleNamespace(value=0.0004)]),
            SimpleNamespace(name="xslow0", qubits=[0], parameters=[SimpleNamespace(value=0.9)]),
            SimpleNamespace(name="measure0", qubits=[0], parameters=[SimpleNamespace(value=0.9)]),
            SimpleNamespace(name="cz0_1", qubits=[0, 1], parameters=[SimpleNamespace(value=0.003)]),
        ]

        def t1(self, q):
            return {0: 100e-6, 1: 200e-6}[q]

        def t2(self, q):
            if q == 1:
                raise RuntimeError("missing T2")
            return 90e-6

        def readout_error(self, q):
            return 0.01

    snap = _snapshot_from_properties(Props(), "ibm_fez")
    assert snap["t1_us"] == 150.0  # median over available
    assert snap["t2_us"] == 90.0
    assert snap["single_qubit_error"] == pytest.approx(0.0003)
    assert snap["two_qubit_error"] == pytest.approx(0.003)
    assert snap["source"] == "backend-properties:ibm_fez"
    assert snap["calibrated_at"].startswith("2026-10-27T12:00:00")


def test_max_execution_time_is_bounded(monkeypatch):
    from kubona.providers import get_provider
    p = get_provider("ibm-qiskit")
    monkeypatch.delenv("KUBONA_IBM_MAX_EXECUTION_SECONDS", raising=False)
    assert p._max_execution_seconds() == 600
    monkeypatch.setenv("KUBONA_IBM_MAX_EXECUTION_SECONDS", "0")
    assert p._max_execution_seconds() == 1
    monkeypatch.setenv("KUBONA_IBM_MAX_EXECUTION_SECONDS", "999999")
    assert p._max_execution_seconds() == 10800
    monkeypatch.setenv("KUBONA_IBM_MAX_EXECUTION_SECONDS", "nonsense")
    assert p._max_execution_seconds() == 600


def test_ibm_job_limits_are_checked_locally():
    from types import SimpleNamespace
    from qiskit import QuantumCircuit
    from kubona.providers import get_provider

    p = get_provider("ibm-qiskit")
    backend = SimpleNamespace(num_qubits=2, name="ibm_fez")
    qc = QuantumCircuit(3)
    qc.measure_all()
    try:
        p._validate_job_limits(qc, 100, backend)
    except Exception as exc:
        assert "qubits" in str(exc)
    else:
        raise AssertionError("over-capacity circuit must be refused")
    p._validate_job_limits(QuantumCircuit(2), 100, backend)


def test_ibm_transpile_produces_isa_circuit():
    from qiskit import QuantumCircuit
    from qiskit_ibm_runtime.fake_provider import FakeManilaV2
    from kubona.providers import get_provider

    qc = QuantumCircuit(2)
    qc.h(0)
    qc.cx(0, 1)
    qc.measure_all()
    isa = get_provider("ibm-qiskit")._transpile_for_backend(qc, FakeManilaV2())
    assert "measure" in isa.count_ops()
    assert set(isa.count_ops()) - {"barrier"} <= set(FakeManilaV2().operation_names)
