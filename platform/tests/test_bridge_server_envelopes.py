"""Bridge handler failure envelopes: unexpected exceptions must come back
as structured success=False responses, never gRPC UNKNOWN and never fake
success. Covers the four handlers whose proto messages carry a
success/error_message envelope (anchor was already wrapped; krylov,
execute, cut, mitigate wrapped in F-5/R9)."""

import sys
import types
from pathlib import Path

import pytest

BRIDGE = Path(__file__).resolve().parent.parent / "bridge"
if str(BRIDGE) not in sys.path:
    sys.path.insert(0, str(BRIDGE))

import server


def _req(**fields):
    return types.SimpleNamespace(**fields)


def test_krylov_exception_returns_structured_failure(monkeypatch):
    def boom(**kwargs):
        raise RuntimeError("pyscf exploded")
    monkeypatch.setattr(server, "solve_quantum_krylov", boom)
    res = server.QuantumBridgeServicer().ExecuteQuantumKrylov(
        _req(molecule="H2", bond_distance=0.7414, subspace_dimension=4,
             time_step=0.2, shots=1024), None)
    assert res.success is False
    assert "pyscf exploded" in res.error_message


def test_execute_exception_returns_structured_failure(monkeypatch):
    def boom(**kwargs):
        raise RuntimeError("aer exploded")
    monkeypatch.setattr(server, "execute_qasm", boom)
    res = server.QuantumBridgeServicer().ExecuteCircuit(
        _req(job_id="job-x", qasm="qreg q[1];", shots=100,
             requested_backend="", allow_classical_fallback=False,
             caller_jurisdictions=[], caller_export_class="",
             export_reviewed=False), None)
    assert res.job_id == "job-x"
    assert res.success is False
    assert "aer exploded" in res.error_message


def test_cut_exception_returns_structured_failure(monkeypatch):
    def boom(*args):
        raise RuntimeError("kahypar exploded")
    monkeypatch.setattr(server, "cut_circuit_qasm", boom)
    res = server.QuantumBridgeServicer().CutCircuit(
        _req(qasm="garbage", max_subcircuit_qubits=2), None)
    assert res.success is False
    assert "kahypar exploded" in res.error_message
    assert list(res.fragments) == []


def test_mitigate_exception_returns_structured_failure(monkeypatch):
    def boom(**kwargs):
        raise RuntimeError("zne exploded")
    monkeypatch.setattr(server, "mitigate_circuit", boom)
    res = server.QuantumBridgeServicer().MitigateCircuit(
        _req(qasm="qreg q[1];", method="ZNE", scale_factors=[],
             shots=100), None)
    assert res.success is False
    assert res.method_used == "ZNE"
    assert "zne exploded" in res.error_message


def test_krylov_solver_attribution_both_paths():
    """J5 + W1.2 recalibration (documented): the handler tags which solver
    ran — numerical-krylov (real expm + Hc=ESc solve) vs the non-H2 refusal
    (analytical-sequence-refused, success=False, no synthetic numbers).
    Downstream folds gate on this flag; the labeled heuristic corridor is
    available only via explicit allow_heuristic=True on solve_quantum_krylov
    (never silently behind success=True)."""
    import server as bridge_server

    servicer = bridge_server.QuantumBridgeServicer()
    h2 = servicer.ExecuteQuantumKrylov(
        _req(molecule="H2", bond_distance=0.7414, subspace_dimension=4,
             time_step=0.2, shots=1024), None)
    assert h2.success is True
    assert h2.solver == "numerical-krylov"

    lih = servicer.ExecuteQuantumKrylov(
        _req(molecule="LiH", bond_distance=1.5957, subspace_dimension=4,
             time_step=0.2, shots=1024), None)
    assert lih.success is False  # fail-closed: no synthetic corridor
    assert lih.solver == "analytical-sequence-refused"
    assert "allow_heuristic" in lih.error_message


def test_krylov_hardware_backend_refused():
    """W1.8: krylov is classical emulation — a hardware backend refuses
    (quantum-washing) instead of running mislabeled."""
    res = server.QuantumBridgeServicer().ExecuteQuantumKrylov(
        _req(molecule="H2", bond_distance=0.7414, subspace_dimension=4,
             time_step=0.2, shots=1024, requested_backend="ibm_brisbane"), None)
    assert res.success is False and "circuit_run" in res.error_message


def test_claim_response_carries_success():
    """W1.8: claim responses carry success/error like every other RPC."""
    res = server.QuantumBridgeServicer().EvaluateWavefunction(
        _req(claim_id="c1", statement="H2 within chemical accuracy",
             quantum_value=-1.1368, classical_anchor_value=-1.13727,
             source_platforms=["pyscf"], total_engagement=12,
             contradiction_signal=0.0), None)
    assert res.success is True and res.error_message == ""
