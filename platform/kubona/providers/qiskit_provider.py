"""Qiskit-backed provider adapter (optional dependency).

Second Phase-0 implementation target per the roadmap: Qiskit + local/GPU simulator.
Qiskit/Aer are NOT hard dependencies — import only when this plugin is used.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit, to_qasm
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


class QiskitProvider(Provider):
    def __init__(self, capability: ProviderCapability):
        self.capability = capability
        try:
            from qiskit import QuantumCircuit  # noqa: F401
            from qiskit_aer import AerSimulator  # noqa: F401
            self._ready = True
        except ImportError as e:  # pragma: no cover
            self._ready = False
            self._import_error = str(e)

    def _ensure_ready(self) -> None:
        if not self._ready:
            raise ProviderError(
                "qiskit provider requires optional deps: pip install 'kubona-platform[qiskit]' "
                f"(missing: {self._import_error})"
            )

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        self._ensure_ready()
        from qiskit import QuantumCircuit as QKCircuit
        from qiskit_aer import AerSimulator

        qc = QKCircuit(circuit.num_qubits)
        for g in circuit.gates:
            qs = list(g.qubits)
            name = g.name
            if name == "u3":
                qc.u(*g.params, qs[0])
            elif name == "u2":
                qc.u(math.pi / 2, g.params[0], g.params[1], qs[0])
            elif name == "u1":
                qc.p(g.params[0], qs[0])
            elif name == "id":
                qc.id(qs[0])
            elif g.params:
                getattr(qc, name)(*g.params, *qs)
            else:
                getattr(qc, name)(*qs)
        qc.measure_all()  # counts require classical registers (all-qubit measurement)
        backend = AerSimulator(seed_simulator=seed)
        job = backend.run(qc, shots=shots)
        counts = job.result().get_counts()
        return {"counts": counts, "shots": shots, "seed": seed, "backend": "qiskit-aer"}

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready}
