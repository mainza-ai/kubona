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
        except (ImportError, OSError) as e:
            # OSError covers an installed-but-unusable SDK (e.g. pytket's config
            # dir XDG_CONFIG_HOME is not writable) -> treat as SDK-not-ready, so
            # the canary reports it as a SKIP (config gap), never a regression.
            self._ready, self._err = False, str(e)

    def _credential(self) -> str | None:
        # W2.18: fresh per call (match qiskit) — no cached SecretStore. A
        # cached store froze BAO_ADDR/rotation at first use; building fresh
        # per call keeps rotation and revocation live.
        from kubona.secrets import SecretStore
        if self._secrets is not None:
            return self._secrets.get("quantinuum", "api_key")
        # Bao-backed when BAO_ADDR is set (authoritative, fail-closed),
        # environment otherwise.
        return SecretStore.from_env().get("quantinuum", "api_key")

    def cancel(self, hardware_job_id: str, client_factory=None) -> dict[str, Any]:
        """Vendor recall via the Nexus client (verified API:
        qnexus.client.jobs.cancel). client_factory injects the client in
        tests (no live calls in the suite). Nexus cancellation is
        best-effort once running — always re-check job status. Legacy
        pytket-quantinuum submissions predate handle retention; only
        Nexus-tracked refs are recallable."""
        token = self._credential()
        if not token:
            raise ProviderError(
                "cannot recall without credentials: KUBONA_QUANTINUUM_API_KEY unset")
        try:
            if client_factory is None:
                import qnexus as qnx
                qnx.client.jobs.cancel(hardware_job_id)
            else:
                client_factory(token).cancel(hardware_job_id)
        except ImportError as e:
            raise ProviderError(f"qnexus client not installed: {e}")
        except Exception as e:  # noqa: BLE001 — vendor refusal is data, not a crash
            raise ProviderError(f"quantinuum recall refused for job {hardware_job_id!r}: {e}")
        return {"provider": self.capability.provider, "hardware_job_id": hardware_job_id,
                "recall_requested": True,
                "note": "cancel is best-effort once running; re-check job status"}

    @property
    def device(self) -> str | None:
        """Target backend, or None when unset (W2.18).

        The old code defaulted to "H1-1" — an unbilled hardware default: a
        misconfigured capability would aim jobs (and spend) at a real QPU
        nobody named. submit() refuses a None device instead.
        """
        return self.capability.backend or None

    def _to_tket(self, circuit: Circuit):
        """Convert a Kubona circuit to a pytket Circuit (native-set directed).

        Angle units (W2.18): radians on both sides — Kubona rx/ry/rz take
        radians (see local_simulator._rx) and pytket OpType.Rx/Ry/Rz take
        radians, so parameters pass through verbatim. No conversion, no
        half-turn ambiguity; per-gate passthrough is pinned by tests.
        """
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

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "quantinuum requires optional deps: pip install pytket pytket-quantinuum "
                f"(missing: {self._err})."
            )
        if isinstance(shots, bool) or not isinstance(shots, int) or shots <= 0:
            raise ProviderError(f"shots must be a positive int, got {shots!r}")
        if self.device is None:
            raise ProviderError(
                "quantinuum execution refuses: no backend configured on the "
                "capability (no unbilled hardware default — set the backend "
                "field to the intended device)")
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
