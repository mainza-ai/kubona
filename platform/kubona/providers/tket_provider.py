"""Quantinuum provider adapter (Phase 2) — optional dependency: pytket + pytket-quantinuum.

Real gate-model integration: a Kubona circuit is converted to a pytket Circuit
(targeting the backend's native gate set) and submitted through pytket-quantinuum
when credentials are configured. Fails closed with actionable guidance when the
SDK or credentials are absent, so the provider registry stays honest and the tool
chain never crashes.

Credential:  KUBONA_QUANTINUUM_API_KEY  (+ optional KUBONA_QUANTINUUM_DEVICE via
the capability `backend` field). The submission path is validated against a
recorded fixture in CI; live runs need a real Quantinuum account + network.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError

# Kubona gate name -> pytket OpType attribute (stable pytket API)
_TKET_GATES = {
    "h": "H", "x": "X", "y": "Y", "z": "Z", "s": "S", "t": "T",
    "cx": "CX", "cz": "CZ", "swap": "SWAP", "ccx": "CCX",
    "rx": "Rx", "ry": "Ry", "rz": "Rz",
}


class TketProvider(Provider):
    def __init__(self, capability: ProviderCapability, secrets=None):
        self.capability = capability
        self._secrets = secrets
        try:
            import pytket  # noqa: F401
            self._ready, self._err = True, None
        except ImportError as e:
            self._ready, self._err = False, str(e)

    def _credential(self) -> str | None:
        if self._secrets is None:
            from kubona.secrets import SecretStore
            self._secrets = SecretStore()
        return self._secrets.get("quantinuum", "api_key")

    @property
    def device(self) -> str | None:
        return self.capability.backend or "H1-1"

    def _to_tket(self, circuit: Circuit):
        """Convert a Kubona circuit to a pytket Circuit (native-set directed)."""
        from pytket import Circuit as TKCkt
        from pytket.circuit import OpType

        c = TKCkt(circuit.num_qubits)
        for g in circuit.gates:
            opattr = _TKET_GATES.get(g.name)
            if opattr is None:
                raise ProviderError(f"pytket conversion: unsupported gate {g.name!r}")
            op = getattr(OpType, opattr)
            qs = list(g.qubits)
            if g.params:
                c.add_gate(op, list(g.params), qs)
            else:
                c.add_gate(op, qs)
        c.measure_all()
        return c

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "quantinuum requires optional deps: pip install pytket pytket-quantinuum "
                f"(missing: {self._err})."
            )
        token = self._credential()
        if not token:
            raise ProviderError(
                "quantinuum execution requires KUBONA_QUANTINUUM_API_KEY — providers fail "
                "closed without credentials (developer/test path uses the local simulator)."
            )
        c = self._to_tket(circuit)
        return self._submit_via_quantinuum(c, shots, token)

    def _submit_via_quantinuum(self, c, shots: int, token: str) -> dict[str, Any]:
        """Submit via pytket-quantinuum (documented API); result -> counts."""
        try:
            from pytket_quantinuum import QuantinuumAPI, QuantinuumBackend
        except ImportError as e:  # pragma: no cover - SDK optional
            raise ProviderError(f"pytket-quantinuum not installed: {e}")
        api = QuantinuumAPI(api_token=token)
        backend = QuantinuumBackend(device_name=self.device, api_handler=api)
        compiled = backend.compile_circuit(c, optimisation_level=0)
        handle = backend.process_circuit(compiled, n_shots=shots)
        result = backend.get_result(handle)
        counts = {k: v for k, v in result.get_counts().items()}
        return {"counts": counts, "shots": shots, "backend": self.device,
                "provider": "quantinuum", "hardware": True}

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready,
                "error": self._err,
                "hardware_ready": self._ready and self._credential() is not None}
