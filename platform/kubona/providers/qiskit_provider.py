"""Qiskit-backed provider adapter (optional dependency).

Second Phase-0 implementation target per the roadmap: Qiskit + local/GPU simulator.
Qiskit/Aer are NOT hard dependencies — import only when this plugin is used.
"""

from __future__ import annotations

import math
import os
import time
from pathlib import Path
from typing import Any

from kubona.circuits import Circuit
from kubona.schema import ProviderCapability

from .base import Provider, ProviderError


def _aer_memory_mb() -> int:
    """Container-aware Aer ceiling; 0 means unset (Aer's host-memory default
    is exactly the footgun this avoids)."""
    raw = (os.getenv("KUBONA_AER_MAX_MEMORY_MB") or "").strip()
    if raw:
        try:
            return max(0, int(raw))
        except ValueError:
            pass
    for path in ("/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"):
        try:
            value = Path(path).read_text().strip()
        except OSError:
            continue
        if value and value != "max":
            try:
                return max(512, int(int(value) / (1024 * 1024) * 0.8))
            except ValueError:
                pass
    return 0


def _transpile_level(default: int = 2) -> int:
    raw = (os.getenv("KUBONA_TRANSPILE_LEVEL") or "").strip()
    if raw:
        try:
            return max(0, min(3, int(raw)))
        except ValueError:
            pass
    return max(0, min(3, default))



def _extract_usage_seconds(
    job: Any,
    *,
    retry_attempts: int = 0,
    retry_delay: float = 1.5,
) -> float | None:
    """Extract measured QPU execution seconds from an IBM Runtime job (W5-07/W6-04).

    On Open Plan / Runtime v2 (qiskit-ibm-runtime 0.52+), ``job.usage()`` may
    return 0 or None for a few seconds right after ``DONE`` while IBM's billing
    meter finalizes, and ``job.metrics()`` reports ``usage.qpu_charge_time_seconds``
    (plus ``usage.quantum_seconds``, ``bss.seconds``, ``execution.execution_spans``,
    and ``circuits_execution_time_ns``). When ``retry_attempts > 0``, perform a
    short bounded re-poll so settled ledger rows capture the real QPU seconds.
    """
    raw_usage: float | None = None
    exec_ns_fallback: float | None = None

    for attempt in range(max(0, int(retry_attempts)) + 1):
        if attempt > 0 and retry_delay > 0:
            time.sleep(retry_delay)
        try:
            u = job.usage()
            if u is not None:
                raw_usage = float(u)
                if raw_usage > 0.0:
                    return raw_usage
        except Exception:
            pass
        try:
            metrics = job.metrics() if callable(getattr(job, "metrics", None)) else None
            if isinstance(metrics, dict):
                u_block = metrics.get("usage")
                if isinstance(u_block, dict):
                    for key in ("qpu_charge_time_seconds", "quantum_seconds", "seconds"):
                        val = u_block.get(key)
                        if val is not None and float(val) > 0.0:
                            return float(val)
                bss = metrics.get("bss")
                if isinstance(bss, dict) and bss.get("seconds") is not None and float(bss["seconds"]) > 0.0:
                    return float(bss["seconds"])
                exec_block = metrics.get("execution")
                if isinstance(exec_block, dict):
                    spans = exec_block.get("execution_spans")
                    if spans is not None:
                        dur = getattr(spans, "duration", None)
                        if dur is not None and float(dur) > 0.0:
                            return float(dur)
                ns = metrics.get("circuits_execution_time_ns")
                if ns is not None and float(ns) > 0.0:
                    exec_ns_fallback = round(float(ns) / 1e9, 3)
        except Exception:
            pass

    if exec_ns_fallback is not None and exec_ns_fallback > 0.0:
        return exec_ns_fallback
    return raw_usage


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
    # R-B: capture compact per-qubit T1/T2/readout/1Q error alongside device medians
    sx_by_q: dict[int, float] = {}
    for g in getattr(props, "gates", []) or []:
        qs = getattr(g, "qubits", [])
        if len(qs) == 1 and str(getattr(g, "name", "")).rstrip("0123456789") in ("sx", "x"):
            params = getattr(g, "parameters", [])
            if params and getattr(params[0], "value", None) is not None:
                sx_by_q[int(qs[0])] = round(float(params[0].value), 6)
    per_qubit: dict[str, dict[str, float]] = {}
    for q in range(n):
        entry: dict[str, float] = {}
        try:
            v_t1 = props.t1(q)
            if v_t1 is not None:
                entry["t1_us"] = round(float(v_t1 * 1e6), 3)
        except Exception:
            pass
        try:
            v_t2 = props.t2(q)
            if v_t2 is not None:
                entry["t2_us"] = round(float(v_t2 * 1e6), 3)
        except Exception:
            pass
        try:
            v_ro = props.readout_error(q)
            if v_ro is not None:
                entry["readout_error"] = round(float(v_ro), 6)
        except Exception:
            pass
        if q in sx_by_q:
            entry["sx_error"] = sx_by_q[q]
        if entry:
            per_qubit[str(q)] = entry
    snap: dict[str, Any] = {
        "qubits": int(n), "t1_us": float(t1 * 1e6), "t2_us": float(t2 * 1e6),
        "readout_error": float(ro), "source": f"backend-properties:{backend_name}"}
    if per_qubit:
        snap["per_qubit"] = per_qubit
    measured = getattr(props, "last_update_date", None)
    if measured is not None:
        snap["calibrated_at"] = measured.isoformat() if hasattr(measured, "isoformat") else str(measured)
        if snap["calibrated_at"]:
            # BackendProperties dates are sometimes naive; make the timezone
            # explicit so Go's RFC3339 age parser does not silently lose the
            # freshness classification.
            if "T" in snap["calibrated_at"] and not snap["calibrated_at"].endswith(("Z", "+00:00")) and "+" not in snap["calibrated_at"][10:]:
                snap["calibrated_at"] += "+00:00"
            snap["version"] = "live-" + snap["calibrated_at"]
            import hashlib as _hashlib
            _dig = _hashlib.sha256(f"{backend_name}|{snap['version']}|{snap['calibrated_at']}".encode("utf-8")).hexdigest()[:12]
            snap["snapshot_id"] = f"cal_{_dig}"
            snap["backend"] = backend_name
    if oneq:
        snap["single_qubit_error"] = float(statistics.median(oneq))
    if twoq:
        snap["two_qubit_error"] = float(statistics.median(twoq))
    return snap


def _snapshot_from_target(target: Any, backend_name: str) -> dict[str, Any] | None:
    """Fallback per-qubit calibration extractor from Qiskit BackendV2 Target (H4).

    Used when ``backend.properties()`` returns None or omits per-qubit data on
    newer IBM Runtime backends. Never invents values; missing fields are skipped.
    """
    import statistics

    if target is None:
        return None
    qprops = getattr(target, "qubit_properties", None)
    if not qprops:
        return None
    n = len(qprops)
    t1_vals: list[float] = []
    t2_vals: list[float] = []
    ro_vals: list[float] = []
    sx_vals: list[float] = []
    per_qubit: dict[str, dict[str, float]] = {}

    meas_map = target.get("measure", {}) if hasattr(target, "get") else {}
    sx_map = target.get("sx", {}) if hasattr(target, "get") else {}
    if not meas_map and hasattr(target, "__contains__"):
        try:
            meas_map = target["measure"] if "measure" in target else {}
        except Exception:
            meas_map = {}
    if not sx_map and hasattr(target, "__contains__"):
        try:
            sx_map = target["sx"] if "sx" in target else {}
        except Exception:
            sx_map = {}

    for q in range(n):
        qp = qprops[q]
        entry: dict[str, float] = {}
        v_t1 = getattr(qp, "t1", None)
        if v_t1 is not None:
            t1_vals.append(float(v_t1))
            entry["t1_us"] = round(float(v_t1) * 1e6, 3)
        v_t2 = getattr(qp, "t2", None)
        if v_t2 is not None:
            t2_vals.append(float(v_t2))
            entry["t2_us"] = round(float(v_t2) * 1e6, 3)
        mp = meas_map.get((q,)) if isinstance(meas_map, dict) else None
        if mp is not None and getattr(mp, "error", None) is not None:
            ro_vals.append(float(mp.error))
            entry["readout_error"] = round(float(mp.error), 6)
        sp = sx_map.get((q,)) if isinstance(sx_map, dict) else None
        if sp is not None and getattr(sp, "error", None) is not None:
            sx_vals.append(float(sp.error))
            entry["sx_error"] = round(float(sp.error), 6)
        if entry:
            per_qubit[str(q)] = entry

    if not per_qubit:
        return None
    snap: dict[str, Any] = {
        "qubits": int(n),
        "source": f"backend-target:{backend_name}",
        "backend": backend_name,
        "calibration_timestamp_source": "backend_target_unavailable",
        "per_qubit": per_qubit,
    }
    if t1_vals:
        snap["t1_us"] = float(statistics.median(t1_vals) * 1e6)
    if t2_vals:
        snap["t2_us"] = float(statistics.median(t2_vals) * 1e6)
    if ro_vals:
        snap["readout_error"] = float(statistics.median(ro_vals))
    if sx_vals:
        snap["single_qubit_error"] = float(statistics.median(sx_vals))
    return snap


class QiskitProvider(Provider):
    def __init__(self, capability: ProviderCapability, secrets=None, backend_name: str | None = None):
        self.capability = capability
        self._secrets = secrets
        self._backend_name = backend_name
        # Vendor plan is an authorization fact, not a rate guess: Open Plan
        # QPU time is free, so a usage meter must settle to $0 rather than
        # list-price dollars. Detected once from the authenticated instance list.
        self._plan: str | None = None
        self._pricing_type: str | None = None
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
        if isinstance(shots, bool) or not isinstance(shots, int) or shots <= 0 or shots > 1_000_000:
            raise ProviderError(f"shots must be a positive int in 1..1,000,000, got {shots!r}")
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

            backend = AerSimulator(seed_simulator=seed, max_memory_mb=_aer_memory_mb())
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
            cancelled = job.cancel()  # IBM's cancel() returns None; don't claim acceptance
            if cancelled is not None:
                cancelled = bool(cancelled)
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
        import os
        from qiskit_ibm_runtime import QiskitRuntimeService
        kwargs = {"channel": self._channel(), "token": token}
        instance = os.environ.get("KUBONA_IBM_QISKIT_INSTANCE", "").strip()
        if instance:
            kwargs["instance"] = instance
        svc = QiskitRuntimeService(**kwargs)
        self._detect_plan(svc)
        return svc

    def _resolve_backend(self, service):
        """Return the explicitly requested backend (failing closed if unknown or
        unavailable — N-01), or the least-busy operational hardware backend ONLY
        when no specific backend name was requested.
        """
        name = (self.backend_name or "").strip()
        if name:
            try:
                backend = service.backend(name)
            except Exception as exc:
                raise ProviderError(
                    f"requested IBM backend {name!r} is unavailable or unknown: {exc}"
                ) from exc
            if backend is None:
                raise ProviderError(f"requested IBM backend {name!r} returned None")
            return backend
        try:
            return service.least_busy(operational=True, simulator=False)
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
        import os
        if self._backend_name:
            return self._backend_name
        override = os.environ.get("KUBONA_IBM_QISKIT_BACKEND", "").strip()
        if override:
            return override
        return self.capability.backend or None

    def _calibrated_initial_layout(self, qc, backend) -> list[int] | None:
        """Pick a globally low-error connected physical subgraph for this circuit (R1).

        Evaluates connected n-qubit subgraphs seeded from every physical qubit
        on the device, scoring readout error (with a strong >3% outlier penalty),
        minimum-spanning-tree 2Q gate error, and T2 coherence time. This avoids
        the single-edge greedy trap that previously locked 4-qubit TFIM and
        8-qubit QAOA into high-readout clusters (e.g. [58,71,70,69]).
        """
        n = qc.num_qubits
        if n <= 0:
            return None
        props = None
        try:
            props = backend.properties() if hasattr(backend, "properties") else None
        except Exception:
            props = None
        target = getattr(backend, "target", None)
        try:
            nphys = int(backend.num_qubits)
        except Exception:
            return None
        if n > nphys:
            return None

        readout: list[float] = []
        t1_us: list[float] = []
        t2_us: list[float] = []
        if props is not None:
            try:
                for q in range(nphys):
                    ro = props.readout_error(q)
                    readout.append(float(ro) if ro is not None else 0.05)
                    t1 = props.t1(q) if hasattr(props, "t1") else None
                    t1_us.append(float(t1) * 1e6 if t1 is not None else 150.0)
                    t2 = props.t2(q) if hasattr(props, "t2") else None
                    t2_us.append(float(t2) * 1e6 if t2 is not None else 100.0)
            except Exception:
                readout, t1_us, t2_us = [], [], []
        if not readout and target is not None:
            qprops = getattr(target, "qubit_properties", None) or []
            meas_map = {}
            try:
                meas_map = target["measure"] if "measure" in target else {}
            except Exception:
                meas_map = {}
            for q in range(nphys):
                mp = meas_map.get((q,)) if isinstance(meas_map, dict) else None
                ro = getattr(mp, "error", None) if mp is not None else None
                readout.append(float(ro) if ro is not None else 0.05)
                qp = qprops[q] if q < len(qprops) else None
                t1 = getattr(qp, "t1", None) if qp is not None else None
                t1_us.append(float(t1) * 1e6 if t1 is not None else 150.0)
                t2 = getattr(qp, "t2", None) if qp is not None else None
                t2_us.append(float(t2) * 1e6 if t2 is not None else 100.0)
        if len(readout) != nphys:
            return None

        coupling = getattr(backend, "coupling_map", None)
        if coupling is None and target is not None:
            try:
                coupling = target.build_coupling_map()
            except Exception:
                coupling = None
        edges: dict[frozenset[int], float] = {}
        edge_dur_ns: dict[frozenset[int], float] = {}
        adj: dict[int, set[int]] = {q: set() for q in range(nphys)}
        if coupling is not None:
            for a, b in coupling:
                ia, ib = int(a), int(b)
                if ia == ib or not (0 <= ia < nphys and 0 <= ib < nphys):
                    continue
                adj[ia].add(ib)
                adj[ib].add(ia)
                key = frozenset((ia, ib))
                best = edges.get(key, float("inf"))
                best_dur = edge_dur_ns.get(key, float("inf"))
                for gate in ("cx", "ecr", "cz"):
                    err = None
                    dur = None
                    if props is not None:
                        try:
                            err = float(props.gate_error(gate, [ia, ib]))
                        except Exception:
                            err = None
                        try:
                            d_sec = props.gate_length(gate, [ia, ib])
                            if d_sec is not None:
                                dur = float(d_sec) * 1e9
                        except Exception:
                            dur = None
                    if err is None and target is not None:
                        try:
                            gmap = target[gate] if gate in target else {}
                            gp = gmap.get((ia, ib)) if isinstance(gmap, dict) else None
                            if gp is not None:
                                if getattr(gp, "error", None) is not None:
                                    err = float(gp.error)
                                if dur is None and getattr(gp, "duration", None) is not None:
                                    dur = float(gp.duration) * 1e9
                        except Exception:
                            err = None
                    if err is not None:
                        best = min(best, err)
                    if dur is not None and dur > 0:
                        best_dur = min(best_dur, dur)
                edges[key] = best if best < float("inf") else 0.01
                if best_dur < float("inf"):
                    edge_dur_ns[key] = best_dur

        def node_cost(q: int) -> float:
            ro = readout[q]
            outlier_pen = 10.0 * max(0.0, ro - 0.03)
            # Penalize short T1 and T2 (T1 asymmetry drives non-unital chemistry bias).
            coherence_pen = 6.0 / max(t1_us[q], 5.0) + 10.0 / max(t2_us[q], 5.0)
            # Spectator crosstalk proxy: average 2Q error on incident couplers.
            inc = [edges[frozenset((q, nb))] for nb in adj[q] if frozenset((q, nb)) in edges]
            xtalk_pen = 0.25 * (sum(inc) / len(inc)) if inc else 0.0
            return 2.0 * ro + outlier_pen + coherence_pen + xtalk_pen

        def edge_cost(u: int, v: int) -> float:
            key = frozenset((u, v))
            e_err = edges.get(key, 0.5)
            dur = edge_dur_ns.get(key, 100.0)
            # Add decoherence-during-gate penalty: dur_us / min(T1, T2)
            t_eff = max(min(t1_us[u], t2_us[u], t1_us[v], t2_us[v]), 5.0)
            dur_pen = (dur * 1e-3) / t_eff
            return 1.5 * e_err + 0.5 * dur_pen

        if n == 1:
            return [min(range(nphys), key=node_cost)]

        def subgraph_cost(nodes: list[int]) -> float:
            # Node cost + MST 2Q edge cost (error + gate-duration decoherence) via Prim's.
            total = sum(node_cost(q) for q in nodes)
            in_tree = {nodes[0]}
            remaining = set(nodes[1:])
            mst_cost = 0.0
            while remaining:
                best_e = float("inf")
                best_v = None
                for u in in_tree:
                    for v in remaining:
                        if frozenset((u, v)) in edges:
                            e = edge_cost(u, v)
                            if e < best_e:
                                best_e = e
                                best_v = v
                if best_v is None:
                    best_v = min(remaining, key=node_cost)
                    best_e = 0.75
                in_tree.add(best_v)
                remaining.remove(best_v)
                mst_cost += best_e
            return total + mst_cost

        best_layout: list[int] | None = None
        best_score = float("inf")
        # Multi-seed global search across every physical qubit
        for seed_q in range(nphys):
            selected = [seed_q]
            sel_set = {seed_q}
            while len(selected) < n:
                frontier = set()
                for s in selected:
                    frontier.update(adj[s])
                frontier -= sel_set
                if not frontier:
                    frontier = set(range(nphys)) - sel_set
                nxt = min(
                    frontier,
                    key=lambda q: node_cost(q) + min(
                        (edge_cost(q, s) for s in selected if frozenset((q, s)) in edges),
                        default=0.75,
                    ),
                )
                selected.append(nxt)
                sel_set.add(nxt)
            sc = subgraph_cost(selected)
            if sc < best_score:
                best_score = sc
                best_layout = selected
        return best_layout

    def _calibrated_paired_layout(self, n_logical: int, backend) -> list[int] | None:
        """Select a ``2 * n_logical`` physical layout for 2-copy Transversal Virtual Distillation.

        Returns ``[a_0, ..., a_{n-1}, b_0, ..., b_{n-1}]`` maximizing direct
        physical coupling between copies (``a_i - b_i``) and within each copy
        while minimizing readout, 2Q gate, and coherence costs. On heavy-hex
        topologies where a full ladder does not exist, falls back gracefully to
        the best connected ``2*n`` subgraph ordered so cross-copy pairs stay
        graph-adjacent (shortest SWAP distance).
        """
        from qiskit import QuantumCircuit
        n = int(n_logical)
        if n <= 0:
            return None
        base = self._calibrated_initial_layout(QuantumCircuit(2 * n), backend)
        if not base or len(base) != 2 * n:
            return base
        # On 2-qubit systems (4 physical qubits in a path p0-p1-p2-p3), map
        # copy A = [p0, p1] and copy B = [p3, p2] or [p2, p3] so (a1, b1) share
        # a direct physical coupler and intra-copy (a0,a1) & (b0,b1) are direct!
        if n == 2:
            target = getattr(backend, "target", None)
            coupling = getattr(backend, "coupling_map", None)
            if coupling is None and target is not None:
                try:
                    coupling = target.build_coupling_map()
                except Exception:
                    coupling = None
            if coupling is not None:
                edge_set = {frozenset((int(u), int(v))) for u, v in coupling}
                sub = list(base)
                # Try all 24 permutations of the 4 physical qubits to maximize
                # direct couplers for (a0,a1), (b0,b1), (a0,b0), (a1,b1).
                import itertools
                best_perm = sub
                best_hits = -1
                for perm in itertools.permutations(sub):
                    a0, a1, b0, b1 = perm
                    hits = (
                        (2 if frozenset((a0, a1)) in edge_set else 0)
                        + (2 if frozenset((b0, b1)) in edge_set else 0)
                        + (1 if frozenset((a0, b0)) in edge_set else 0)
                        + (1 if frozenset((a1, b1)) in edge_set else 0)
                    )
                    if hits > best_hits:
                        best_hits = hits
                        best_perm = list(perm)
                return best_perm
        return base

    def _layout_quality(self, layout: list[int] | None, backend) -> dict[str, Any] | None:
        """Calibration summary for the chosen physical qubits (post-hoc trust)."""
        if not layout:
            return None
        readouts: dict[str, float] = {}
        t2s: list[float] = []
        try:
            props = backend.properties() if hasattr(backend, "properties") else None
            if props is not None:
                for q in layout:
                    readouts[str(q)] = float(props.readout_error(q))
                    if hasattr(props, "t2"):
                        t2 = props.t2(q)
                        if t2 is not None:
                            t2s.append(round(float(t2) * 1e6, 3))
        except Exception:
            readouts = {}
        if not readouts:
            return None
        return {
            "physical_qubits": list(layout),
            "readout_error": readouts,
            "max_readout_error": max(readouts.values()) if readouts else None,
            "sum_readout_error": round(sum(readouts.values()), 6),
            "min_t2_us": min(t2s) if t2s else None,
            "suspicious": any(v > 0.05 for v in readouts.values()),
            "source": f"backend-properties:{getattr(backend, 'name', 'backend')}",
        }

    def _transpile_for_backend(self, qc, backend, initial_layout=None):
        # ISA is mandatory on IBM hardware; transpile locally before Sampler.
        from qiskit.transpiler import generate_preset_pass_manager
        layout = initial_layout
        if layout is None:
            layout = self._calibrated_initial_layout(qc, backend)
        return generate_preset_pass_manager(
            optimization_level=_transpile_level(2), backend=backend, initial_layout=layout
        ).run(qc)

    def _layout_summary(self, original, isa) -> dict[str, Any] | None:
        """Logical→physical mapping for post-hoc device/layout diagnosis."""
        layout = getattr(isa, "layout", None)
        if layout is None or getattr(layout, "initial_layout", None) is None:
            return None
        out: dict[str, Any] = {"initial": {}, "final": {}, "physical_qubits": []}
        try:
            physical: list[int] = []
            for i, q in enumerate(original.qubits):
                phys = int(layout.initial_layout[q])
                out["initial"][str(i)] = phys
                physical.append(phys)
            out["physical_qubits"] = physical
        except Exception:
            return None
        try:
            if getattr(layout, "final_layout", None) is not None:
                for i, q in enumerate(original.qubits):
                    out["final"][str(i)] = int(layout.final_layout[q])
        except Exception:
            out["final"] = {}
        return out

    def _validate_job_limits(self, qc, shots: int, backend) -> None:
        # IBM validates after transpilation: <=10M executions (circuit x shots)
        # and <=5M two-qubit gates per circuit. Fail locally with a clear
        # message instead of queueing a guaranteed validation failure.
        if shots > 10_000_000:
            raise ProviderError("Sampler executions exceed the 10M per-job IBM limit")
        num_qubits = getattr(backend, "num_qubits", None)
        if isinstance(num_qubits, int) and qc.num_qubits > num_qubits:
            raise ProviderError(
                f"circuit uses {qc.num_qubits} qubits but {getattr(backend, 'name', 'backend')} "
                f"has {num_qubits}")
        twoq = sum(1 for inst in qc.data if len(inst.qubits) == 2)
        if twoq > 5_000_000:
            raise ProviderError("circuit exceeds the 5M two-qubit-gate IBM limit")

    def _detect_plan(self, service) -> None:
        if self._plan is not None or service is None:
            return
        try:
            for inst in service.instances() or []:
                if not isinstance(inst, dict):
                    continue
                self._plan = str(inst.get("plan") or "").strip().lower() or None
                self._pricing_type = str(inst.get("pricing_type") or "").strip().lower() or None
                if self._plan:
                    return
        except Exception:  # noqa: BLE001 — unknown plan keeps list-price estimates
            return

    def _actual_usd(self, usage_seconds: float) -> float | None:
        # Open Plan has no per-second charge. Do not bill the local ledger
        # list-price dollars for free vendor time.
        if self._plan in ("open", "free") or self._pricing_type == "free":
            return 0.0
        pricing = self.capability.pricing
        if pricing.model != "per_second" or pricing.per_second_usd <= 0:
            return None
        return round(max(1.0, float(usage_seconds)) * pricing.per_second_usd, 6)

    def _max_execution_seconds(self) -> int:
        import os
        raw = os.environ.get("KUBONA_IBM_MAX_EXECUTION_SECONDS", "600").strip()
        try:
            n = int(raw)
        except ValueError:
            return 600
        return min(max(n, 1), 10800)

    def _submit_ibm_runtime(self, qc, shots: int, token: str,
                            on_hardware_job=None,
                            wait_for_result: bool = True,
                            service_factory=None) -> dict[str, Any]:
        """Real IBM hardware via Qiskit Runtime (channel: ibm_quantum_platform)."""
        layout = None
        if service_factory is None:
            service = self._service(token)
            backend = self._resolve_backend(service)
            if hasattr(backend, "target"):
                try:
                    from qiskit_ibm_runtime.executor_sampler import Sampler
                    sampler = Sampler(mode=backend)
                    sampler.options.max_execution_time = self._max_execution_seconds()
                except (ImportError, ModuleNotFoundError):
                    from qiskit_ibm_runtime import SamplerV2
                    sampler = SamplerV2(backend=backend)
                original = qc
                chosen = self._calibrated_initial_layout(original, backend)
                qc = self._transpile_for_backend(original, backend, chosen)
                layout = self._layout_summary(original, qc)
                if layout is not None:
                    layout["quality"] = self._layout_quality(chosen, backend)
                self._validate_job_limits(qc, shots, backend)
            else:
                # Test/older stub that does not expose BackendV2.target.
                from qiskit_ibm_runtime import SamplerV2
                sampler = SamplerV2(backend=backend)
                layout = None
        else:
            service = service_factory(token)
            backend = self._resolve_backend(service)
            if hasattr(backend, "target"):
                original = qc
                chosen = self._calibrated_initial_layout(original, backend)
                qc = self._transpile_for_backend(original, backend, chosen)
                layout = self._layout_summary(original, qc)
                if layout is not None:
                    layout["quality"] = self._layout_quality(chosen, backend)
                self._validate_job_limits(qc, shots, backend)
            if hasattr(service, "sampler"):
                sampler = service.sampler(backend)
            else:
                from qiskit_ibm_runtime import SamplerV2
                sampler = SamplerV2(backend=backend)
        job = sampler.run([qc], shots=shots)
        job_id = str(job.job_id())
        # Durable pin before the result wait and before any ack. The
        # callback remains for the in-process job store.
        from kubona.hardware_pin import persist_vendor_job
        persist_vendor_job(job_id)
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
                "layout": layout,
            }
        result = job.result()
        pub = result[0].data.meas
        counts = {
            "".join(str(b) for b in k): int(v)
            for k, v in pub.get_counts().items()
        }
        out: dict[str, Any] = {
            "counts": counts, "shots": shots,
            "backend": getattr(backend, "name", self.backend_name),
            "job_id": job_id, "hardware": True,
            "channel": self._channel(), "layout": layout,
        }
        try:
            cal_snap = _snapshot_from_properties(
                backend.properties(),
                str(getattr(backend, "name", self.backend_name or "")),
            )
            if cal_snap:
                out["calibration"] = cal_snap
        except Exception:
            pass
        try:
            usage = _extract_usage_seconds(job, retry_attempts=0 if service_factory is not None else 4)
            if usage is not None:
                out["usage_seconds"] = float(usage)
                actual = self._actual_usd(float(usage))
                if actual is not None:
                    out["actual_usd"] = actual
        except Exception:
            pass
        return out

    def submit_batch(self, qasm_list: list[str], shots: int = 4096,
                     seed: int = 7, service_factory=None,
                     shots_per_circuit: list[int] | None = None,
                     already_transpiled: bool = False,
                     initial_layout: list[int] | None = None) -> dict[str, Any]:
        """Submit multiple measurement circuits as one Runtime job.

        This is the provider primitive behind one governed hardware experiment:
        one vendor job id, one usage reading, one settlement. It is explicitly
        job/batch mode (Open Plan compatible), never a session.

        When ``shots_per_circuit`` is provided with heterogeneous shot counts,
        realizes per-circuit budgets via bounded GCD uniform-shot replication
        (``<= 64`` expanded PUBs to minimize Runtime compilation overhead) and
        count merging. When ``already_transpiled=True``, submits pre-compiled
        ISA circuits verbatim (preserving 100% identical virtual-RZ schedules).
        """
        self._ensure_ready()
        if not isinstance(qasm_list, list) or not qasm_list:
            raise ProviderError("hardware batch needs at least one circuit")
        if not isinstance(shots, int) or isinstance(shots, bool) or shots < 1:
            raise ProviderError(f"shots must be an integer >= 1, got {shots!r}")
        resolved_shots: list[int] | None = None
        if shots_per_circuit is not None:
            if not isinstance(shots_per_circuit, list) or len(shots_per_circuit) != len(qasm_list):
                raise ProviderError(
                    f"shots_per_circuit length ({len(shots_per_circuit) if isinstance(shots_per_circuit, list) else 'invalid'}) "
                    f"must match qasm_list length ({len(qasm_list)})"
                )
            resolved_shots = []
            for s in shots_per_circuit:
                if not isinstance(s, int) or isinstance(s, bool) or s < 1:
                    raise ProviderError(f"shots_per_circuit entries must be ints >= 1, got {s!r}")
                resolved_shots.append(int(s))
        token = self._credential()
        if not token:
            raise ProviderError("cannot submit hardware batch without credentials: KUBONA_IBM_QISKIT_TOKEN unset")
        try:
            from qiskit import qasm2
        except ImportError as e:
            raise ProviderError(f"qiskit qasm2 unavailable: {e}")
        service = self._service(token, service_factory)
        backend = self._resolve_backend(service)
        try:
            from qiskit_ibm_runtime.executor_sampler import Sampler
            sampler = Sampler(mode=backend)
        except (ImportError, ModuleNotFoundError):
            try:
                from qiskit_ibm_runtime import SamplerV2 as Sampler
                try:
                    sampler = Sampler(mode=backend)
                except TypeError:
                    sampler = Sampler(backend=backend)
            except (ImportError, ModuleNotFoundError) as e:
                raise ProviderError(f"qiskit-ibm-runtime not installed: {e}")
        # G2 + Phase 0: explicitly configure XY4 ALAP dynamical decoupling and
        # active-accum gate + measurement Pauli twirling with deterministic
        # randomization counts so Runtime never falls back to unconfigured defaults.
        error_suppression: dict[str, Any] = {
            "dynamical_decoupling": False,
            "dynamical_decoupling_sequence": None,
            "dynamical_decoupling_scheduling": None,
            "dynamical_decoupling_slack": None,
            "gate_twirling": False,
            "measure_twirling": False,
            "twirling_strategy": None,
            "twirling_num_randomizations": None,
        }
        opts = getattr(sampler, "options", None)
        if opts is not None:
            dd = getattr(opts, "dynamical_decoupling", None)
            if dd is not None and hasattr(dd, "enable"):
                try:
                    dd.enable = True
                    if hasattr(dd, "sequence_type"):
                        dd.sequence_type = "XY4"
                    if hasattr(dd, "scheduling_method"):
                        dd.scheduling_method = "alap"
                        error_suppression["dynamical_decoupling_scheduling"] = "alap"
                    if hasattr(dd, "extra_slack_distribution"):
                        dd.extra_slack_distribution = "middle"
                        error_suppression["dynamical_decoupling_slack"] = "middle"
                    error_suppression["dynamical_decoupling"] = True
                    error_suppression["dynamical_decoupling_sequence"] = "XY4"
                except Exception:
                    pass
            tw = getattr(opts, "twirling", None)
            if tw is not None:
                try:
                    if hasattr(tw, "enable_gates"):
                        tw.enable_gates = True
                        error_suppression["gate_twirling"] = True
                    if hasattr(tw, "enable_measure"):
                        tw.enable_measure = True
                        error_suppression["measure_twirling"] = True
                    if hasattr(tw, "strategy"):
                        tw.strategy = "active-accum"
                        error_suppression["twirling_strategy"] = "active-accum"
                    # Explicitly pin randomization count (32 twirls per PUB keeps
                    # coherent-to-Pauli averaging tight without inflating QPU time).
                    if hasattr(tw, "num_randomizations"):
                        n_rand = 32 if int(shots) >= 1024 else 16
                        tw.num_randomizations = n_rand
                        error_suppression["twirling_num_randomizations"] = n_rand
                except Exception:
                    pass
        circuits = []
        layout = None
        layout_quality = None
        transpile_meta = None
        transpile_samples: list[dict[str, int]] = []
        cached_layouts: dict[int, list[int] | None] = {}
        for idx_c, qasm in enumerate(qasm_list):
            qc = qasm2.loads(str(qasm))
            original = qc
            if hasattr(backend, "target"):
                if already_transpiled:
                    chosen = list(initial_layout) if initial_layout is not None else None
                else:
                    n_q = int(original.num_qubits)
                    if initial_layout is not None and len(initial_layout) == n_q:
                        chosen = list(initial_layout)
                    else:
                        if n_q not in cached_layouts:
                            cached_layouts[n_q] = self._calibrated_initial_layout(original, backend)
                        chosen = cached_layouts[n_q]
                    qc = self._transpile_for_backend(original, backend, chosen)
                if layout is None:
                    layout = self._layout_summary(original, qc)
                    if layout is None and chosen is not None:
                        layout = {
                            "initial": {str(i): int(p) for i, p in enumerate(chosen)},
                            "final": {},
                            "physical_qubits": [int(p) for p in chosen],
                        }
                    if layout_quality is None:
                        phys = layout.get("physical_qubits") if isinstance(layout, dict) else chosen
                        layout_quality = self._layout_quality(phys, backend)
                try:
                    def _twoq(circuit):
                        return sum(
                            1 for inst in circuit.data
                            if len(inst.qubits) >= 2
                            and str(getattr(inst.operation, "name", "")) not in ("barrier", "measure", "reset")
                        )

                    def _oneq_pulses(circuit):
                        return sum(
                            1 for inst in circuit.data
                            if len(inst.qubits) == 1
                            and str(getattr(inst.operation, "name", "")) in ("sx", "x", "rx", "ry", "u", "u2", "u3")
                        )
                    transpile_samples.append({
                        "logical_depth": int(original.depth()),
                        "logical_two_qubit_gates": int(_twoq(original)),
                        "transpiled_depth": int(qc.depth()),
                        "transpiled_two_qubit_gates": int(_twoq(qc)),
                        "transpiled_one_qubit_pulses": int(_oneq_pulses(qc)),
                    })
                except Exception:
                    pass
            circuit_shots = resolved_shots[idx_c] if resolved_shots is not None else shots
            self._validate_job_limits(qc, circuit_shots, backend)
            circuits.append(qc)
        if transpile_samples:
            transpile_meta = {
                "circuits": len(transpile_samples),
                "already_transpiled": bool(already_transpiled),
                "min_transpiled_depth": min(s["transpiled_depth"] for s in transpile_samples),
                "max_transpiled_depth": max(s["transpiled_depth"] for s in transpile_samples),
                "min_transpiled_two_qubit_gates": min(s["transpiled_two_qubit_gates"] for s in transpile_samples),
                "max_transpiled_two_qubit_gates": max(s["transpiled_two_qubit_gates"] for s in transpile_samples),
                "per_circuit_depth": [s["transpiled_depth"] for s in transpile_samples],
                "per_circuit_two_qubit_gates": [s["transpiled_two_qubit_gates"] for s in transpile_samples],
                "per_circuit_one_qubit_pulses": [s["transpiled_one_qubit_pulses"] for s in transpile_samples],
                "first": transpile_samples[0],
            }
        expanded_reps: list[int] | None = None
        pub_base_shots: int | None = None
        if resolved_shots is not None and len(set(resolved_shots)) > 1:
            # IBM qiskit_ibm_runtime executor_sampler enforces uniform shots across
            # PUBs in one job. Cap expanded PUBs at <= 64 (previously 400 -> 152 PUBs
            # wasting ~3.5x Runtime wall-clock framing overhead on Heron).
            max_pubs = max(64, len(circuits))
            base = math.gcd(*resolved_shots)
            if base < 1 or sum(s // base for s in resolved_shots) > max_pubs:
                # Choose clean base shots so sum(reps) <= max_pubs while honoring ratios
                raw_base = max(1, int(math.ceil(sum(resolved_shots) / float(max_pubs))))
                # Round up to nearest multiple of 512 when feasible for clean twirl binning
                base = int(math.ceil(raw_base / 512.0)) * 512 if raw_base >= 512 else raw_base
                expanded_reps = [max(1, int(round(s / float(base)))) for s in resolved_shots]
            else:
                expanded_reps = [s // base for s in resolved_shots]
            pub_base_shots = int(base)
            expanded_circuits: list[Any] = []
            for qc_item, reps in zip(circuits, expanded_reps):
                expanded_circuits.extend([qc_item] * reps)
            job = sampler.run(expanded_circuits, shots=pub_base_shots)
        else:
            uniform_shots = resolved_shots[0] if resolved_shots is not None else shots
            job = sampler.run(circuits, shots=uniform_shots)
        job_id = str(job.job_id())
        from kubona.hardware_pin import persist_vendor_job
        persist_vendor_job(job_id)
        try:
            result = job.result()
            counts_out = []
            if expanded_reps is not None:
                cursor = 0
                for reps in expanded_reps:
                    merged: dict[str, int] = {}
                    for _ in range(reps):
                        pub = result[cursor].data.meas
                        cursor += 1
                        for k, v in pub.get_counts().items():
                            bs = "".join(str(b) for b in k)
                            merged[bs] = merged.get(bs, 0) + int(v)
                    counts_out.append(merged)
            else:
                for i in range(len(circuits)):
                    pub = result[i].data.meas
                    counts_out.append({
                        "".join(str(b) for b in k): int(v)
                        for k, v in pub.get_counts().items()
                    })
        except Exception as e:  # noqa: BLE001 — vendor result is data
            raise ProviderError(f"hardware batch result read failed for job {job_id!r}: {e}")
        out: dict[str, Any] = {
            "success": True,
            "counts": counts_out,
            "shots": shots,
            "seed": seed,
            "backend": getattr(backend, "name", self.backend_name),
            "job_id": job_id,
            "hardware_job_id": job_id,
            "hardware": True,
            "channel": self._channel(),
            "layout": layout,
            "layout_quality": layout_quality,
            "transpile": transpile_meta,
            "error_suppression": error_suppression,
            "plan": self._plan,
        }
        if resolved_shots is not None:
            out["shots_per_circuit"] = (
                [int(r * pub_base_shots) for r in expanded_reps]
                if (expanded_reps is not None and pub_base_shots is not None)
                else resolved_shots
            )
            if expanded_reps is not None and pub_base_shots is not None:
                out["pub_base_shots"] = pub_base_shots
                out["expanded_pubs"] = int(sum(expanded_reps))
        try:
            bname = str(getattr(backend, "name", self.backend_name or ""))
            props = backend.properties() if hasattr(backend, "properties") else None
            cal_snap = _snapshot_from_properties(props, bname) if props is not None else None
            if cal_snap is None or not cal_snap.get("per_qubit"):
                t_snap = _snapshot_from_target(getattr(backend, "target", None), bname)
                if cal_snap is None:
                    cal_snap = t_snap
                elif t_snap and t_snap.get("per_qubit"):
                    cal_snap["per_qubit"] = t_snap["per_qubit"]
            if cal_snap:
                out["calibration"] = cal_snap
        except Exception:
            pass
        try:
            usage = _extract_usage_seconds(job, retry_attempts=0 if service_factory is not None else 4)
            if usage is not None:
                out["usage_seconds"] = float(usage)
                actual = self._actual_usd(float(usage))
                if actual is not None:
                    out["actual_usd"] = actual
        except Exception:
            pass
        return out

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
            self._detect_plan(service)
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
                    "".join(str(b) for b in k): int(v)
                    for k, v in pub.get_counts().items()
                }
            except Exception as e:  # noqa: BLE001
                raise ProviderError(
                    f"ibm job {hardware_job_id!r} reported {status_str} but result read failed: {e}"
                )
            backend_name = self.backend_name
            try:
                backend_obj = getattr(job, "backend", None)
                if callable(backend_obj):
                    backend_obj = backend_obj()
                backend_name = getattr(backend_obj, "name", backend_name)
            except Exception:  # noqa: BLE001 — a display label must not fail settlement
                pass
            result = {
                "status": "completed",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
                "result": {
                    "counts": counts,
                    "backend": backend_name,
                    "job_id": hardware_job_id,
                    "hardware": True,
                    "channel": self._channel(),
                },
            }
            try:
                usage = _extract_usage_seconds(job, retry_attempts=0 if service_factory is not None else 4)
                if usage is not None:
                    result["result"]["usage_seconds"] = float(usage)
                    actual = self._actual_usd(float(usage))
                    if actual is not None:
                        result["result"]["actual_usd"] = actual
            except Exception:
                pass
            return result
        if status_str in ("error", "failed"):
            err_fn = getattr(job, "error_message", None)
            err_msg = err_fn() if callable(err_fn) else err_fn
            out = {
                "status": "failed",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
                "error": str(err_msg or f"ibm runtime job {hardware_job_id} failed ({status_str})"),
            }
            # Failure/cancel usage reporting: the vendor meter ran even
            # though the job produced no counts. Report usage_seconds (and
            # the priced actual_usd) on the failure/cancel envelope so
            # settlement can reconcile measured usage instead of keeping the
            # full estimate blindly. Best-effort: an unreadable meter leaves
            # the envelope without usage (unknown), never a fabricated zero.
            try:
                usage = _extract_usage_seconds(job)
                if usage is not None:
                    out["usage_seconds"] = float(usage)
                    actual = self._actual_usd(float(usage))
                    if actual is not None:
                        out["actual_usd"] = actual
            except Exception:
                pass
            return out
        if status_str in ("cancelled", "canceled"):
            out = {
                "status": "cancelled",
                "vendor_status": status_str,
                "hardware_job_id": hardware_job_id,
            }
            try:
                usage = _extract_usage_seconds(job)
                if usage is not None:
                    out["usage_seconds"] = float(usage)
                    actual = self._actual_usd(float(usage))
                    if actual is not None:
                        out["actual_usd"] = actual
            except Exception:
                pass
            return out
        return {
            "status": "running",
            "vendor_status": status_str,
            "hardware_job_id": hardware_job_id,
        }

    def health(self) -> dict[str, Any]:
        return {"provider": self.capability.provider, "ready": self._ready,
                "hardware_ready": self._credential() is not None and bool(self.backend_name)}

    def vendor_plan(self) -> dict[str, Any] | None:
        """Authenticated vendor plan/pricing posture, or None when unknown.

        Pre-dispatch estimates use this to distinguish an Open Plan/free
        instance from pay-as-you-go list price. Missing credentials or an
        unreadable instance list returns None (never a guessed free plan).
        """
        token = self._credential()
        if not token:
            return None
        try:
            service = self._service(token)
            self._detect_plan(service)
            if not self._plan and not self._pricing_type:
                return None
            return {"plan": self._plan, "pricing_type": self._pricing_type,
                    "free": self._plan in ("open", "free") or self._pricing_type == "free"}
        except Exception:
            return None

    def queue_snapshot(self) -> dict[str, Any] | None:
        """Live vendor queue depth, or None when unmeasurable.

        IBM exposes ``pending_jobs`` but no public job ETA; the bridge returns
        the measured queue depth and an explicit unknown/reason for ETA. It
        never invents seconds from a pending count.
        """
        try:
            token = self._credential()
            if not token:
                return None
            service = self._service(token)
            backend = self._resolve_backend(service)
            status = backend.status()
            return {
                "backend": str(backend.name),
                "pending_jobs": int(status.pending_jobs),
                "operational": bool(status.operational),
                "status": str(getattr(status, "status_msg", "") or ""),
                "eta_seconds": None,
                "eta_reason": "vendor does not expose a public job ETA",
                "source": "live-backend-status",
            }
        except Exception:
            return None

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
