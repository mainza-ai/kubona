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
    def __init__(self, capability: ProviderCapability, secrets=None, backend_name: str | None = None):
        self.capability = capability
        self._secrets = secrets
        self._backend_name = backend_name
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

    # -- circuit -> Qiskit QuantumCircuit (shared by Aer and IBM Runtime paths) --
    def _to_qiskit(self, circuit: Circuit):
        import math
        from qiskit import QuantumCircuit as QKCircuit

        from .base import ProviderError

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
            elif not hasattr(qc, name):
                raise ProviderError(
                    f"gate {name!r} has no Qiskit mapping in this provider; "
                    f"transpile first (compiler server) or pick another provider"
                )
            elif g.params:
                getattr(qc, name)(*g.params, *qs)
            else:
                getattr(qc, name)(*qs)
        qc.measure_all()  # counts require classical registers
        return qc

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        """Run on real IBM hardware via Qiskit Runtime when credentials exist,
        otherwise the credential-free local Aer simulator (dev provider).

        Real-hardware path: reads KUBONA_IBM_QISKIT_TOKEN via the SecretStore,
        submits to the named IBM backend, and returns counts with a real job_id —
        so the P1 "runs on a real QPU via direct provider SDK" gate is exercised
        whenever credentials are configured. Fails closed (Aer) otherwise, keeping
        the dev/test posture credential-free (open-source first).
        """
        self._ensure_ready()
        qc = self._to_qiskit(circuit)

        token = self._credential()
        if token and self.backend_name:
            return self._submit_ibm_runtime(qc, shots, token)
        # credential-free dev provider (open-source first, per maintainer direction)
        from qiskit_aer import AerSimulator

        backend = AerSimulator(seed_simulator=seed)
        job = backend.run(qc, shots=shots)
        counts = job.result().get_counts()
        return {"counts": counts, "shots": shots, "seed": seed,
                "backend": "qiskit-aer", "hardware": False}

    def _credential(self) -> str | None:
        if self._secrets is None:
            from kubona.secrets import SecretStore
            # Bao-backed when BAO_ADDR is set (authoritative, fail-closed),
            # environment otherwise. This submit path is the first wired to
            # OpenBao; other providers keep EnvSecretSource until their turn.
            self._secrets = SecretStore.from_env()
        return self._secrets.get("ibm-qiskit", "token")

    @property
    def backend_name(self) -> str | None:
        if self._backend_name:
            return self._backend_name
        return self.capability.backend or None

    def _submit_ibm_runtime(self, qc, shots: int, token: str) -> dict[str, Any]:
        """Real IBM hardware via Qiskit Runtime (channel: ibm_quantum)."""
        from qiskit_ibm_runtime import QiskitRuntimeService, SamplerV2  # noqa: F401

        service = QiskitRuntimeService(channel="ibm_quantum", token=token)
        backend = service.backend(self.backend_name)
        sampler = SamplerV2(backend=backend)
        job = sampler.run([qc], shots=shots)
        job_id = job.job_id()
        result = job.result()
        pub = result[0].data.meas
        counts = {
            "".join(str(b) for b in k): v
            for k, v in pub.get_counts().items()
        }
        return {"counts": counts, "shots": shots, "backend": self.backend_name,
                "job_id": job_id, "hardware": True, "channel": "ibm_quantum"}

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready,
                "hardware_ready": self._credential() is not None and bool(self.backend_name)}

    def calibration_snapshot(self) -> dict[str, Any] | None:
        """Live backend calibration medians (Q-14), or None when unmeasurable.

        Best-effort read of backend.properties(); any failure (no credential,
        stale backend name, network error, missing package) returns None —
        unknown, never a fabricated number.
        """
        try:
            token = self._credential()
            if not token or not self.backend_name:
                return None
            from qiskit_ibm_runtime import QiskitRuntimeService
            service = QiskitRuntimeService(channel="ibm_quantum", token=token)
            props = service.backend(self.backend_name).properties()
            if props is None:
                return None
            import statistics
            t1 = statistics.median([q.t1 for q in props.qubits])
            t2 = statistics.median([q.t2 for q in props.qubits])
            ro = statistics.median([q.readout_error for q in props.qubits])
            twoq = [g.parameters[0].value for g in props.gates if len(getattr(g, "qubits", [])) == 2]
            oneq = [g.parameters[0].value for g in props.gates if len(getattr(g, "qubits", [])) == 1]
            snap: dict[str, Any] = {
                "t1_us": float(t1 * 1e6), "t2_us": float(t2 * 1e6),
                "readout_error": float(ro), "source": "backend-properties"}
            # Only report what was actually measured; absent keys compare as
            # unknown downstream, never as fabricated zeros.
            if oneq:
                snap["single_qubit_error"] = float(statistics.median(oneq))
            if twoq:
                snap["two_qubit_error"] = float(statistics.median(twoq))
            return snap
        except Exception:
            return None
