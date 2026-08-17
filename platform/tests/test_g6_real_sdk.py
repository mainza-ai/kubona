"""Real-SDK validation for the G6 connecters (skip if SDK not installed).

Uses the ACTUAL pytket / pyquil SDKs to validate the Kubona-Circuit -> SDK
conversion paths (no hardware, no credentials). These two conversions caught real
bugs the stubs missed (pyquil CX->CNOT, integer qubits), so they are worth running
whenever the SDKs are present.
"""

import importlib.util
import os

import pytest

from kubona.circuits import Circuit, Gate
from kubona.schema import load_provider

HAVE_PYTKET = importlib.util.find_spec("pytket") is not None
HAVE_PYQUIL = importlib.util.find_spec("pyquil") is not None


@pytest.fixture
def writable_config(monkeypatch, tmp_path):
    """pytket (and some SDKs) write a config under XDG_CONFIG_HOME; on hosts
    where ~/.config is root-owned (e.g. this one) that import fails. Point it
    at a writable temp dir so the real-SDK test is robust on any host."""
    monkeypatch.setenv("XDG_CONFIG_HOME", str(tmp_path))
    # ensure a fresh pytket import happens after the env is set
    if "pytket" in __import__("sys").modules:
        __import__("sys").modules.pop("pytket", None)
    return tmp_path

BELL = Circuit(name="bell", num_qubits=2, gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
VQE2 = Circuit(name="vqe", num_qubits=2,
               gates=[Gate("ry", (0,), (0.5,)), Gate("rz", (1,), (0.2,)), Gate("cx", (0, 1))])


@pytest.mark.skipif(not HAVE_PYTKET, reason="pytket not installed")
def test_pytket_real_conversion(writable_config):
    from kubona.providers.tket_provider import TketProvider
    p = TketProvider(load_provider("quantinuum"))
    cmds = {c.op.type.name for c in p._to_tket(BELL).get_commands()}
    assert {"H", "CX", "Measure"} <= cmds
    # parametric gates must survive with their parameters
    v = p._to_tket(VQE2).get_commands()
    assert any(c.op.type.name == "Ry" for c in v)
    assert any(c.op.type.name == "Rz" for c in v)


@pytest.mark.skipif(not HAVE_PYQUIL, reason="pyquil not installed")
def test_pyquil_real_conversion(writable_config):
    from kubona.providers.pyquil_provider import PyQuilProvider
    q = PyQuilProvider(load_provider("rigetti"))
    instr = [str(i) for i in q._to_pyquil(BELL).instructions]
    assert "H 0" in instr and "CNOT 0 1" in instr  # controlled-X is CNOT in pyquil
    assert "MEASURE 0 ro[0]" in instr
    v = [str(i) for i in q._to_pyquil(VQE2).instructions]
    assert "RY(0.5) 0" in v and "RZ(0.2) 1" in v
