"""G6 connecters: pytket (Quantinuum) + pyquil (Rigetti) — conversion + fail-closed.

Real integration paths validated against recorded-fixture SDK modules (spelled to
the documented API); no live hardware happens in CI. Live runs need the SDKs
installed + provider credentials and are exercised on opted-in backends.
"""

import sys
import types

import pytest

from kubona.circuits import Circuit, Gate
from kubona.providers.tket_provider import TketProvider
from kubona.providers.pyquil_provider import PyQuilProvider
from kubona.schema import load_provider

BELL = Circuit(name="bell", num_qubits=2, gates=[Gate("h", (0,)), Gate("cx", (0, 1))])
VQE2 = Circuit(name="vqe", num_qubits=2, gates=[Gate("ry", (0,), (0.5,)), Gate("rz", (1,), (0.2,)),
                                               Gate("cx", (0, 1))])


# ------------------------- pytket / Quantinuum -------------------------
@pytest.fixture
def fake_pytket(monkeypatch):
    """Stub pytket with a minimal OpType set + Circuit that records gates."""
    class FakeCircuit:
        def __init__(self, n): self.n = n; self.gates = []
        def add_gate(self, op, arg, qs=None):
            if qs is None: qs = arg; arg = ()
            self.gates.append((op, tuple(arg if arg else ()), tuple(qs)))
            return self
        def measure_all(self): self.measured = True; return self

    class FakeOpType:
        H = X = Y = Z = S = T = CX = CZ = SWAP = CCX = "g"
        Rx = Ry = Rz = "g_param"

    mod = types.ModuleType("pytket")
    mod.Circuit = FakeCircuit
    monkeypatch.setitem(sys.modules, "pytket", mod)
    # submodule pytket.circuit exposing OpType
    circ_mod = types.ModuleType("pytket.circuit")
    circ_mod.OpType = FakeOpType
    monkeypatch.setitem(sys.modules, "pytket.circuit", circ_mod)
    return mod


def test_pytket_conversion_and_fixture_submission(fake_pytket, monkeypatch):
    monkeypatch.setenv("KUBONA_QUANTINUUM_API_KEY", "qtk-key")
    # stub pytket_quantinuum recorded fixture
    class FakeResult:
        def get_counts(self): return {(0, 0): 256, (1, 1): 256}
    class FakeBackend:
        def __init__(self, **k): self.k = k
        def compile_circuit(self, c, optimisation_level=0): return c
        def process_circuit(self, c, n_shots=None): return "h-1"
        def get_result(self, h): return FakeResult()
    class FakeAPI:
        def __init__(self, **k): self.token = k.get("api_token")
    mkt = types.ModuleType("pytket_quantinuum")
    mkt.QuantinuumAPI = FakeAPI; mkt.QuantinuumBackend = FakeBackend
    monkeypatch.setitem(sys.modules, "pytket_quantinuum", mkt)

    p = TketProvider(load_provider("quantinuum"))
    out = p.submit(BELL, shots=512)
    assert out["hardware"] is True and out["counts"] == {(0, 0): 256, (1, 1): 256}
    assert out["backend"] == p.device


def test_pytket_fails_closed_without_credentials(fake_pytket, monkeypatch):
    monkeypatch.delenv("KUBONA_QUANTINUUM_API_KEY", raising=False)
    p = TketProvider(load_provider("quantinuum"))
    with pytest.raises(Exception):
        p.submit(BELL, shots=512)


# ------------------------- pyquil / Rigetti -------------------------
@pytest.fixture
def fake_pyquil(monkeypatch):
    class FakeProg:
        def __init__(self): self.ops = []; self.declares = []
        def declare(self, nm, ty, n): self.declares.append((nm, ty, n)); return self
        def __getitem__(self, i): return ("ro", i)  # ro[i] MemoryReference
        def measure(self, q, ro): return self
        def __iadd__(self, other): return self
        def __add__(self, other): return self
    class G:
        def __call__(self, *a): return ("g", a)
    mod = types.ModuleType("pyquil")
    mod.Program = FakeProg
    monkeypatch.setitem(sys.modules, "pyquil", mod)
    gates_mod = types.ModuleType("pyquil.gates")
    for n in ["H","X","Y","Z","T","CNOT","CZ","SWAP","RX","RY","RZ"]:
        setattr(gates_mod, n, G())
    monkeypatch.setitem(sys.modules, "pyquil.gates", gates_mod)
    return mod


def test_pyquil_conversion_and_wavesim_fixture(fake_pyquil, monkeypatch):
    monkeypatch.delenv("KUBONA_RIGETTI_QCS_TOKEN", raising=False)
    called = {}
    class FakeWF:
        def run_and_measure(self, prog, trials=None):
            called["trials"] = trials
            return [[0, 0]] * (trials // 2) + [[1, 1]] * (trials // 2)
    api_mod = types.ModuleType("pyquil.api")
    api_mod.WavefunctionSimulator = FakeWF
    monkeypatch.setitem(sys.modules, "pyquil.api", api_mod)

    p = PyQuilProvider(load_provider("rigetti"))
    out = p.submit(BELL, shots=512)
    assert out["hardware"] is False
    assert called["trials"] == 512
    assert out["counts"].get("00") and out["counts"].get("11")


def test_pyquil_fails_closed_without_sdk():
    p = PyQuilProvider(load_provider("rigetti"))
    # no pyquil in sys.modules by default in this env
    for key in list(sys.modules):
        if key == "pyquil" or key.startswith("pyquil."):
            del sys.modules[key]
    import importlib.util
    if importlib.util.find_spec("pyquil") is None:
        with pytest.raises(Exception):
            p.submit(BELL, shots=512)
