"""Qiskit-backed provider adapter (optional dependency).

Second Phase-0 implementation target per the roadmap: Qiskit + local/GPU simulator.
Qiskit/Aer are NOT hard dependencies — import only when this plugin is used.
"""

from __future__ import annotations

from typing import Any

from kubona.circuits import Circuit, to_qasm
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


def _snapshot_from_properties(props: Any, backend_name: str) -> dict[str, Any] | None:
    """Median calibration from BackendProperties; missing values are skipped,
    never zero-filled. Kept separate so tests exercise the real parsing."""
    import statistics
    n = len(props.qubits)

    def _vals(fn):
        out = []
        for q in range(n):
            try:
                v = fn(q)
            except Exception:
                continue
            if v is not None:
                out.append(v)
        return out

    t1_vals, t2_vals, ro_vals = _vals(props.t1), _vals(props.t2), _vals(props.readout_error)
    if not t1_vals or not t2_vals or not ro_vals:
        return None
    t1 = statistics.median(t1_vals)
    t2 = statistics.median(t2_vals)
    ro = statistics.median(ro_vals)
    # Representative native 1q gates; id/rx/rz are virtual and measure/reset
    # are not coherent gate errors.
    oneq = [g.parameters[0].value for g in props.gates
            if len(getattr(g, "qubits", [])) == 1
            and str(g.name).rstrip("0123456789") in ("sx", "x")]
    twoq = [g.parameters[0].value for g in props.gates if len(getattr(g, "qubits", [])) == 2]
    snap: dict[str, Any] = {
        "t1_us": float(t1 * 1e6), "t2_us": float(t2 * 1e6),
        "readout_error": float(ro), "source": f"backend-properties:{backend_name}"}
    measured = getattr(props, "last_update_date", None)
    if measured is not None:
        snap["calibrated_at"] = measured.isoformat() if hasattr(measured, "isoformat") else str(measured)
    if oneq:
        snap["single_qubit_error"] = float(statistics.median(oneq))
    if twoq:
        snap["two_qubit_error"] = float(statistics.median(twoq))
    return snap


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
    # Explicit gate allowlist + arity check (W2.18): the old hasattr duck-typing
    # accepted any attribute (including non-gate helpers) and forwarded params
    # unchecked — a 2-param gate handed 3 params failed deep inside Qiskit.
    # Anything outside _QISKIT_GATES refuses with ProviderError (fail-closed).
    #
    # Scope note: this is the SDK CONVERSION set (gates with explicit branches
    # or direct QuantumCircuit methods below) — deliberately NOT the hardware
    # basis set from the capability JSON. Basis lowering stays the compiler
    # server's job; the provider converts, it does not silently recompile.
    _QISKIT_GATES = frozenset({
        "h", "x", "y", "z", "s", "sdg", "t", "tdg", "id",
        "rx", "ry", "rz", "p", "u1", "u2", "u3",
        "cx", "cz", "swap", "cp", "sx", "ecr",
    })
    _PARAM_ARITY: dict[str, int] = {
        "u3": 3, "u2": 2, "u1": 1, "p": 1,
        "rx": 1, "ry": 1, "rz": 1, "rxx": 1, "ryy": 1, "rzz": 1,
        "cp": 1, "crx": 1, "cry": 1, "crz": 1,
    }

    def _to_qiskit(self, circuit: Circuit):
        import math
        from qiskit import QuantumCircuit as QKCircuit

        from .base import ProviderError

        qc = QKCircuit(circuit.num_qubits)
        for g in circuit.gates:
            qs = list(g.qubits)
            name = g.name
            if not qs:
                raise ProviderError(
                    f"gate {name!r} has no qubits (zero-qubit gate): refusing")
            if name not in self._QISKIT_GATES:
                raise ProviderError(
                    f"gate {name!r} has no explicit Qiskit conversion in this "
                    f"provider (convertible: {sorted(self._QISKIT_GATES)}); "
                    "transpile first (compiler server) or pick another provider"
                )
            arity = self._PARAM_ARITY.get(name, 0)
            if len(g.params) != arity:
                raise ProviderError(
                    f"gate {name!r} takes {arity} parameter(s), got {len(g.params)}: "
                    "refusing unchecked params")
            if name == "u3":
                qc.u(*g.params, qs[0])
            elif name == "u2":
                qc.u(math.pi / 2, g.params[0], g.params[1], qs[0])
            elif name == "u1":
                qc.p(g.params[0], qs[0])
            elif name == "id":
                qc.id(qs[0])
            else:
                fn = getattr(qc, name, None)
                if not callable(fn):
                    raise ProviderError(
                        f"gate {name!r} is declared native but has no Qiskit "
                        "method in this SDK version: refusing duck-typed call"
                    )
                if g.params:
                    fn(*g.params, *qs)
                else:
                    fn(*qs)
        qc.measure_all()  # counts require classical registers
        return qc

    def submit(self, circuit: Circuit, shots: int, seed: int | None = None,
               allow_sim_fallback: bool = False,
               on_hardware_job=None,
               wait_for_result: bool = True,
               service_factory=None) -> dict[str, Any]:
        """Run on real IBM hardware via Qiskit Runtime when credentials exist.

        No quantum-washing (W2.18): when this capability is a HARDWARE
        modality (anything but statevector/gpu simulator), a missing
        credential or backend REFUSES — the old code silently ran Aer and
        returned simulator counts for a hardware request. Pass
        `allow_sim_fallback=True` to explicitly opt into the Aer path (the
        result carries `"sim_fallback": True` so the downgrade is labeled,
        never silent). Simulator-tier capabilities (qiskit-aer-simulator)
        run Aer directly with no flag — no downgrade is involved.
        """
        self._ensure_ready()
        if isinstance(shots, bool) or not isinstance(shots, int) or shots <= 0:
            raise ProviderError(f"shots must be a positive int, got {shots!r}")
        qc = self._to_qiskit(circuit)

        modality = getattr(self.capability.modality, "value",
                           str(self.capability.modality))
        is_simulator = modality in ("statevector_simulator", "gpu_simulator")
        token = self._credential()
        if token and self.backend_name and not is_simulator:
            return self._submit_ibm_runtime(
                qc, shots, token,
                on_hardware_job=on_hardware_job,
                wait_for_result=wait_for_result,
                service_factory=service_factory,
            )
        if is_simulator or allow_sim_fallback:
            # credential-free Aer path (open-source first, per maintainer direction)
            from qiskit_aer import AerSimulator

            backend = AerSimulator(seed_simulator=seed)
            job = backend.run(qc, shots=shots)
            counts = job.result().get_counts()
            out: dict[str, Any] = {"counts": counts, "shots": shots, "seed": seed,
                                   "backend": "qiskit-aer", "hardware": False}
            if not is_simulator:
                out["sim_fallback"] = True  # explicitly opted-in downgrade, labeled
            return out
        raise ProviderError(
            f"provider {self.capability.provider!r} is a hardware modality "
            f"({modality}) but no credential (KUBONA_IBM_QISKIT_TOKEN) or "
            "backend is configured: refusing silent Aer downgrade "
            "(pass allow_sim_fallback=True to explicitly run the simulator)")

    def cancel(self, hardware_job_id: str, service_factory=None) -> dict[str, Any]:
        """Vendor recall via Qiskit Runtime (verified API: job.cancel() +
        POST /jobs/{id}/cancel). service_factory injects the service in
        tests (no live calls in the suite); production builds it from the
        stored credential. Cancelling is best-effort once running — always
        re-check job status afterwards."""
        token = self._credential()
        if not token:
            raise ProviderError(
                "cannot recall without credentials: KUBONA_IBM_QISKIT_TOKEN unset")
        try:
            service = self._service(token, service_factory)
            job = service.job(hardware_job_id)
            cancelled = bool(job.cancel())
        except ImportError as e:
            raise ProviderError(f"qiskit-ibm-runtime not installed: {e}")
        except Exception as e:  # noqa: BLE001 — vendor refusal is data, not a crash
            raise ProviderError(f"ibm recall refused for job {hardware_job_id!r}: {e}")
        return {"provider": self.capability.provider, "hardware_job_id": hardware_job_id,
                "recall_requested": True, "cancel_accepted": cancelled,
                "note": "cancel is best-effort once running; re-check job status"}

    def _channel(self) -> str:
        """Runtime channel; Qiskit 2.x uses ibm_quantum_platform. The legacy
        ibm_quantum channel was retired, so it is an explicit override only."""
        import os
        value = os.environ.get("KUBONA_IBM_QISKIT_CHANNEL", "").strip()
        return value or "ibm_quantum_platform"

    def _service(self, token: str, service_factory=None):
        if service_factory is not None:
            return service_factory(token)
        from qiskit_ibm_runtime import QiskitRuntimeService
        return QiskitRuntimeService(channel=self._channel(), token=token)

    def _resolve_backend(self, service):
        """Return the configured backend or the first operational hardware
        backend. Backend names retire (ibm_brisbane did); a stale contract
        name must not turn every calibration/dispatch into a hard failure.
        """
        name = self.backend_name
        if name:
            try:
                return service.backend(name)
            except Exception:
                pass
        candidates = sorted(
            (b for b in service.backends(simulator=False, operational=True)),
            key=lambda b: getattr(b, "name", ""),
        )
        if not candidates:
            raise ProviderError("no operational IBM hardware backend is available")
        return candidates[0]

    def _credential(self) -> str | None:
        # R12: no caching of the default store. An explicitly injected
        # store (constructor arg — tests, deliberate wiring) is honored;
        # otherwise the store is built fresh per call so rotation stays
        # visible: a rotated env var, a newly set BAO_ADDR, or a revoked
        # Bao lease bites on the next submit, not on process restart.
        # from_env() is env reads + object construction (no I/O until
        # .get), so per-call construction costs nothing measurable.
        if self._secrets is not None:
            return self._secrets.get("ibm-qiskit", "token")
        from kubona.secrets import SecretStore
        # Bao-backed when BAO_ADDR is set (authoritative, fail-closed),
        # environment otherwise.
        return SecretStore.from_env().get("ibm-qiskit", "token")

    @property
    def backend_name(self) -> str | None:
        if self._backend_name:
            return self._backend_name
        return self.capability.backend or None

    def _submit_ibm_runtime(self, qc, shots: int, token: str,
                            on_hardware_job=None,
                            wait_for_result: bool = True,
                            service_factory=None) -> dict[str, Any]:
        """Real IBM hardware via Qiskit Runtime (channel: ibm_quantum_platform)."""
        if service_factory is None:
            from qiskit_ibm_runtime import SamplerV2  # noqa: F401

            service = self._service(token)
            backend = self._resolve_backend(service)
            sampler = SamplerV2(backend=backend)
        else:
            service = service_factory(token)
            backend = self._resolve_backend(service)
            if hasattr(service, "sampler"):
                sampler = service.sampler(backend)
            else:
                from qiskit_ibm_runtime import SamplerV2
                sampler = SamplerV2(backend=backend)
        job = sampler.run([qc], shots=shots)
        job_id = str(job.job_id())
        if on_hardware_job is not None:
            on_hardware_job(job_id)
        if not wait_for_result:
            return {
                "status": "running",
                "deferred": True,
                "shots": shots,
                "backend": getattr(backend, "name", str(backend)),
                "job_id": job_id,
                "hardware_job_id": job_id,
                "hardware": True,
                "channel": self._channel(),
            }
        result = job.result()
        pub = result[0].data.meas
        counts = {
            "".join(str(b) for b in k): v
            for k, v in pub.get_counts().items()
        }
        return {"counts": counts, "shots": shots, "backend": self.backend_name,
                "job_id": job_id, "hardware": True, "channel": "ibm_quantum"}

    def poll_hardware_job(self, hardware_job_id: str, service_factory=None) -> dict[str, Any]:
        """Non-blocking vendor status poll for an IBM Runtime job."""
        if not hardware_job_id or not str(hardware_job_id).strip():
            raise ProviderError("hardware_job_id is required")
        token = self._credential()
        if not token:
            raise ProviderError(
                "cannot poll without credentials: KUBONA_IBM_QISKIT_TOKEN unset"
            )
        try:
            service = self._service(token, service_factory)
            job = service.job(hardware_job_id)
            raw_status = job.status()
        except ImportError as e:
            raise ProviderError(f"qiskit-ibm-runtime not installed: {e}")
        except Exception as e:  # noqa: BLE001
            raise ProviderError(f"ibm status poll failed for job {hardware_job_id!r}: {e}")

        status_str = getattr(raw_status, "name", str(raw_status)).strip().lower()
        if status_str in ("done", "completed"):
            try:
                result = job.result()
                pub = result[0].data.meas
                counts = {
                    "".join(str(b) for b in k): v
                    for k, v in pub.get_counts().items()
                }
            except Exception as e:  # noqa: BLE001
                raise ProviderError(
                    f"ibm job {hardware_job_id!r} reported {status_str} but result read failed: {e}"
                )
            return {
                "status": "completed",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
                "result": {
                    "counts": counts,
                    "backend": self.backend_name,
                    "job_id": hardware_job_id,
                    "hardware": True,
                    "channel": self._channel(),
                },
            }
        if status_str in ("error", "failed"):
            err_fn = getattr(job, "error_message", None)
            err_msg = err_fn() if callable(err_fn) else err_fn
            return {
                "status": "failed",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
                "error": str(err_msg or f"ibm runtime job {hardware_job_id} failed ({status_str})"),
            }
        if status_str in ("cancelled", "canceled"):
            return {
                "status": "cancelled",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
            }
        return {
            "status": "running",
            "vendor_status": status_str,
            "hardware_job_id": hardware_job_id,
        }

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
            service = self._service(token)
            backend = self._resolve_backend(service)
            props = backend.properties()
            if props is None:
                return None
            return _snapshot_from_properties(props, backend.name)
        except Exception:
            return None
