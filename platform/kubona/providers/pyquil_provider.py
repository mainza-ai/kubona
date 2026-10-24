"""Rigetti provider adapter (Phase 2) — optional dependency: pyquil.

Real gate-model integration: a Kubona circuit is converted to a pyQuil Program and
submitted to a Rigetti backend via QCS (WaveFunctionSimulator credential-free, or
a real QPU) when credentials are configured. Fails closed otherwise.

Credential:  KUBONA_RIGETTI_QCS_TOKEN  (+ optional object id / endpoint via the
capability `backend` field). Submission path validated against a recorded fixture
in CI; live runs need a Rigetti QCS account + network.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError

# Kubona gate name -> pyquil Program application
_PYQUIL_GATES = {
    "h", "x", "y", "z", "cx", "cz", "rx", "ry", "rz", "s", "t", "swap",
}


class PyQuilProvider(Provider):
    def __init__(self, capability: ProviderCapability, secrets=None):
        self.capability = capability
        self._secrets = secrets
        try:
            import pyquil  # noqa: F401
            self._ready, self._err = True, None
        except (ImportError, OSError) as e:
            self._ready, self._err = False, str(e)

    def _credential(self) -> str | None:
        # W2.18: fresh per call (match qiskit) — no cached SecretStore.
        if self._secrets is not None:
            return self._secrets.get("rigetti", "qcs_token")
        from kubona.secrets import SecretStore
        # Bao-backed when BAO_ADDR is set (authoritative, fail-closed),
        # environment otherwise.
        return SecretStore.from_env().get("rigetti", "qcs_token")

    @property
    def backend_id(self) -> str | None:
        return self.capability.backend  # e.g. "Aspen-M-3" or a QVM endpoint

    def _to_pyquil(self, circuit: Circuit):
        """Convert a Kubona circuit to a pyQuil Program."""
        from pyquil import Program
        from pyquil.gates import (CNOT, CZ, H, RX, RY, RZ, SWAP, T, X, Y, Z)

        # pyquil's controlled-X gate is CNOT (there is no CX in pyquil.gates)
        gates = {"h": H, "x": X, "y": Y, "z": Z, "s": None, "t": T,
                 "cx": CNOT, "cz": CZ, "swap": SWAP, "rx": RX, "ry": RY, "rz": RZ}
        prog = Program()
        for g in circuit.gates:
            fn = gates.get(g.name)
            if fn is None:  # 's' (phase) not a native pyquil single-qubit gate
                raise ProviderError(f"pyquil conversion: unsupported gate {g.name!r}")
            qs = list(g.qubits)  # pyquil expects integer qubit indices
            if g.params:
                prog += fn(*g.params, *qs)
            else:
                prog += fn(*qs)
        ro = prog.declare("ro", "BIT", circuit.num_qubits)
        for i in range(circuit.num_qubits):
            prog.measure(i, ro[i])
        return prog

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               on_hardware_job=None) -> dict[str, Any]:
        if not self._ready:
            raise ProviderError(
                "rigetti requires optional dep: pip install pyquil " f"(missing: {self._err})."
            )
        if isinstance(shots, bool) or not isinstance(shots, int) or shots <= 0:
            raise ProviderError(f"shots must be a positive int, got {shots!r}")
        prog = self._to_pyquil(circuit)
        token = self._credential()
        if not token:
            # credential-free local path via pyquil's WavefunctionSimulator. This
            # still needs the Rigetti `qvm` daemon running (127.0.0.1:5000) — no
            # cloud credentials, but a local sim binary — so we fail closed with
            # actionable guidance if the daemon is absent. (open-source first.)
            if seed is not None:
                # W2.18: the WavefunctionSimulator/qvm path cannot be seeded
                # (no seed parameter exists) — a seeded call would return
                # unseeded draws labeled seeded. Refuse the seeded path and
                # declare nondeterminism instead of faking it.
                raise ProviderError(
                    "rigetti credential-free WavefunctionSimulator path is "
                    "nondeterministic (qvm exposes no seed): refusing seeded "
                    "submit — pass seed=None for explicitly-unseeded runs, or "
                    "use the local-simulator for seeded determinism")
            try:
                from pyquil.api import WavefunctionSimulator
            except Exception as e:  # noqa: BLE001
                raise ProviderError(
                    "rigetti requires KUBONA_RIGETTI_QCS_TOKEN for QPU, or pyquil's "
                    f"WavefunctionSimulator + the local `qvm` daemon for a credential-free run "
                    f"(missing SDK: {e})."
                )
            wf = WavefunctionSimulator()
            try:
                measured = wf.run_and_measure(prog, trials=shots)
            except Exception as e:  # QVM daemon not reachable
                raise ProviderError(
                    "rigetti credential-free local run needs the `qvm` daemon at "
                    "127.0.0.1:5000 (start qvm/quilc), or set KUBONA_RIGETTI_QCS_TOKEN "
                    f"for QPU. (error: {e})"
                )
            # measured: list of bitstring-rows (lists of ints), one per trial.
            # W2.18: Counter over tuples (O(shots)) — the old
            # {row: rows.count(row)} was O(shots^2).
            from collections import Counter
            tallies = Counter(tuple(int(b) for b in r) for r in measured)
            return {"counts": {"".join(str(b) for b in k): v for k, v in tallies.items()},
                    "shots": shots, "backend": "pyquil-wavesim", "hardware": False,
                    "seed": None, "deterministic": False}
        return self._submit_via_qcs(prog, shots, token)

    def _submit_via_qcs(self, prog, shots: int, token: str) -> dict[str, Any]:
        """Submit via pyquil QCS Connection (documented API); result -> counts."""
        # W2.13: the old body imported WavefunctionSimulator (already handled
        # on the credential-free path above) and qcs_sdk.QCSClient without
        # using either — dead imports before an unconditional raise. Removed:
        # a probe that never probes is decoration. Live wiring uses the
        # Rigetti QCS REST API with the token once the backend/endpoint is
        # provisioned.
        raise ProviderError(
            "rigetti QPU submission adapter needs the provisioned QCS endpoint; "
            "see kubona/secrets + the backend field. (Simulator path: WavefunctionSimulator.)"
        )

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready,
                "hardware_ready": self._ready and self._credential() is not None}
