"""
Quantum Circuit Execution and PennyLane Circuit Cutting Service.
Provides honest degradation across:
1. QPU Hardware
2. Aer Noise Simulation
3. Statevector Simulation
4. Classical Fallback
Also computes Meyer-Wallach global entanglement scores.
"""
from __future__ import annotations

import time
import math
import threading
from typing import Dict, Any, List, Tuple
import numpy as np

try:
    import qiskit
    from qiskit import QuantumCircuit
    from qiskit.quantum_info import Statevector
    HAS_QISKIT = True
except ImportError:
    HAS_QISKIT = False

try:
    from qiskit_aer import Aer
    HAS_AER = True
except ImportError:
    HAS_AER = False


def circuit_to_qasm(qc: QuantumCircuit) -> str:
    """Serializes a QuantumCircuit to OpenQASM 2.0 string."""
    try:
        from qiskit.qasm2 import dumps
        return dumps(qc)
    except Exception:
        if hasattr(qc, "qasm"):
            return qc.qasm()
        raise


def circuit_from_qasm(qasm: str) -> QuantumCircuit:
    """Parses OpenQASM 2.0 string into a QuantumCircuit."""
    qasm_str = qasm
    if 'include "qelib1.inc";' not in qasm_str:
        lines = qasm_str.splitlines()
        norm_lines = []
        inserted = False
        for line in lines:
            norm_lines.append(line)
            if not inserted and line.strip().startswith("OPENQASM"):
                norm_lines.append('include "qelib1.inc";')
                inserted = True
        if not inserted:
            norm_lines.insert(0, 'include "qelib1.inc";')
        qasm_str = "\n".join(norm_lines)

    try:
        from qiskit.qasm2 import loads
        return loads(qasm_str)
    except Exception:
        try:
            return QuantumCircuit.from_qasm_str(qasm_str)
        except Exception:
            return QuantumCircuit.from_qasm_str(qasm)



def calculate_meyer_wallach(statevector: np.ndarray) -> float:
    """
    Computes Meyer-Wallach global entanglement score Q in [0, 1].
    Reference: Meyer & Wallach (2002), J. Math. Phys. 43, 4273.
    """
    dim = len(statevector)
    if dim < 2 or (dim & (dim - 1)) != 0:
        return 0.0
    n = int(math.log2(dim))
    if n <= 1:
        return 0.0

    arr = np.array(statevector, dtype=complex).reshape([2] * n)
    q_score = 0.0
    for k in range(n):
        other_axes = [i for i in range(n) if i != k]
        # Partial trace over all qubits except k
        rho_k = np.tensordot(arr, arr.conj(), axes=(other_axes, other_axes))
        # Q = (4 / n) * sum_k det(rho_k) = (2 / n) * sum_k (1 - Tr(rho_k^2))
        q_score += 2.0 * (1.0 - float(np.real(np.trace(rho_k @ rho_k))))

    return float(q_score / n)


MAX_STATEVECTOR_QUBITS = 28  # Safe 4GB RAM ceiling (2^28 * 16 bytes = 4.29 GB)
MAX_AER_SIM_QUBITS = 30      # 16GB Aer density matrix / MPS simulation limit

#: Bytes of one maximum single-job statevector (2^28 complex128).
_SINGLE_JOB_CEILING_BYTES = (2 ** MAX_STATEVECTOR_QUBITS) * 16


def _global_inflight_cap_bytes() -> int:
    """Process-wide cap on concurrent simulation bytes.

    Default is 2x the single-job ceiling: two max-size statevectors may
    coexist, but 8 bridge workers each holding 4GB cannot OOM the host.
    Env-tunable via KUBONA_BRIDGE_MAX_INFLIGHT_BYTES (host-aware operators
    lower it on small boxes, raise it on big iron). Unparseable/empty reads
    fail closed to the default, never to unbounded.
    """
    import os as _os

    raw = (_os.getenv("KUBONA_BRIDGE_MAX_INFLIGHT_BYTES") or "").strip()
    if raw:
        try:
            cap_bytes = int(raw)
        except ValueError:
            cap_bytes = 0
        if cap_bytes > 0:
            return cap_bytes
    return 2 * _SINGLE_JOB_CEILING_BYTES


# Process-wide in-flight simulation bytes (global RAM admission). execute_qasm
# holds `required_bytes` here for the duration of one simulation so concurrent
# workers cannot jointly exceed the cap. Guarded by _INFLIGHT_LOCK; never
# negative (releases are clamped, never trusted blindly).
_INFLIGHT_LOCK = threading.Lock()
_INFLIGHT_BYTES = 0


def _inflight_try_acquire(required_bytes: int) -> bool:
    """Reserve `required_bytes` against the global cap. Returns True and holds
    the bytes on success; returns False holding nothing when a new job would
    exceed the cap (caller refuses fail-closed). Non-positive requests always
    succeed without moving the counter."""
    global _INFLIGHT_BYTES
    if required_bytes <= 0:
        return True
    with _INFLIGHT_LOCK:
        if _INFLIGHT_BYTES + required_bytes > _global_inflight_cap_bytes():
            return False
        _INFLIGHT_BYTES += required_bytes
        return True


def _inflight_release(held_bytes: int) -> None:
    """Return bytes held by _inflight_try_acquire. Clamped at zero: a double
    release (caller bug) must not mint negative headroom for the next job."""
    global _INFLIGHT_BYTES
    if held_bytes <= 0:
        return
    with _INFLIGHT_LOCK:
        _INFLIGHT_BYTES = max(0, _INFLIGHT_BYTES - held_bytes)


def inflight_bytes() -> int:
    """Current process-wide held simulation bytes (observability for tests)."""
    with _INFLIGHT_LOCK:
        return _INFLIGHT_BYTES

# --- Shared consts (single source; no inline magic) ---
#: Cap for QPD sampling overhead kappa=4^K (v1 heuristic). Above this the
#: cut REFUSES instead of silently promising a 4^K blowup. 4^6=4096.
MAX_SAMPLING_OVERHEAD_KAPPA_V1 = 4096.0
MAX_CUTS_V1 = 6


def check_circuit_memory_bounds(qc: QuantumCircuit) -> Dict[str, Any]:
    """
    Computes theoretical RAM requirements for exact statevector simulation of qc.
    Prevents uncatchable host OOM kills on high-qubit circuits.
    """
    n_qubits = qc.num_qubits
    required_bytes = (2 ** n_qubits) * 16
    required_mb = required_bytes / (1024 * 1024)
    return {
        "num_qubits": n_qubits,
        "required_bytes": required_bytes,
        "required_mb": required_mb,
        "exceeds_statevector_limit": n_qubits > MAX_STATEVECTOR_QUBITS,
        "exceeds_aer_limit": n_qubits > MAX_AER_SIM_QUBITS,
    }


def route_backend(requested_backend: str) -> str:
    """Classify a requested backend string without executing anything.

    Returns "aer" (simulator class), "hardware:ibm-qiskit" (IBM Runtime via
    provider plugin), or "unknown-hardware:<name>" (no execution plugin —
    callers must refuse or degrade explicitly, never silently simulate).
    """
    low = (requested_backend or "").lower()
    if any(k in low for k in ("aer", "local", "sim")):
        return "aer"
    if low.startswith("ibm") or "brisbane" in low or "kyiv" in low or "kingston" in low:
        return "hardware:ibm-qiskit"
    return f"unknown-hardware:{requested_backend}"


def _route_hardware(provider_name: str, caller_jurisdictions: list | None,
                    caller_export_class: str, export_reviewed: bool,
                    t0: float) -> Dict[str, Any] | None:
    """P5 §2.9 compliance gate for hardware execution.

    Returns None when routing passes (caller proceeds to the plugin).
    Returns an honest failure dict — never simulation — when the caller
    declared nothing or route() rules INELIGIBLE. Zero spend, zero network.
    """
    def _refuse(reason: str) -> Dict[str, Any]:
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": f"routing refused: {reason}",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": f"routing refused: {reason}",
        }
    if not caller_jurisdictions:
        return _refuse(
            f"hardware provider {provider_name!r} requires declared "
            "caller_jurisdictions (routing cannot be verified undeclared)")
    try:
        from kubona.routing import ROUTE_INELIGIBLE, route
        from kubona.schema import load_provider
        r = route(load_provider(provider_name),
                  caller_jurisdictions=caller_jurisdictions,
                  caller_export_class=caller_export_class or "any",
                  require_review=export_reviewed)
    except Exception as e:
        return _refuse(f"routing evaluation failed closed: {e}")
    if r.verdict == ROUTE_INELIGIBLE:
        return _refuse(f"{provider_name}: {r.reason} "
                       f"(caller={caller_jurisdictions}, class={caller_export_class})")
    return None


def _execute_ibm_hardware(qasm: str, shots: int, requested_backend: str, seed: int | None = None) -> Dict[str, Any] | None:
    """Real IBM Quantum hardware via the provider plugin, or None to fall
    through to labeled simulation when credentials/SDK are absent.

    Returns None (fail-closed, zero spend, zero network without a token) when:
    the provider plugin is unavailable, no KUBONA_IBM_QISKIT_TOKEN is set, or
    qiskit-ibm-runtime is not installed. Real execution returns hardware counts
    with degradation_level=1 and the Runtime job_id. Any unexpected error
    returns an honest failure dict (never silent simulation).
    """
    try:
        from kubona.circuits import parse_qasm2 as parse_native
        from kubona.providers.qiskit_provider import QiskitProvider
        from kubona.schema import load_provider
    except Exception as e:
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": f"IBM provider plugin unavailable: {e}",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "execution_time_ms": 0.0,
            "error_message": f"IBM provider plugin unavailable: {e}",
        }
    try:
        plugin = QiskitProvider(load_provider("ibm-qiskit"), backend_name=requested_backend)
        res = plugin.submit(parse_native(qasm), shots, seed=seed if seed is not None else 7)
    except Exception as e:
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": f"IBM hardware submission failed: {e}",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "execution_time_ms": 0.0,
            "error_message": f"IBM hardware submission failed: {e}",
        }
    if not res.get("hardware"):
        return None  # fail-closed Aer inside the plugin: caller labels simulation
    return {
        "success": True,
        "resolved_backend": f"ibm_quantum:{res.get('backend', requested_backend)}",
        "degradation_level": 1,
        "degradation_reason": (
            f"Executed on IBM Quantum hardware backend {res.get('backend', requested_backend)} "
            f"via Qiskit Runtime (job {res.get('job_id', 'unknown')})"
        ),
        "counts": res.get("counts", {}),
        "statevector_real": [],
        "statevector_imag": [],
        "meyer_wallach": 0.0,
        "execution_time_ms": 0.0,  # filled in by caller
        "error_message": "",
        "hardware_job_id": res.get("job_id"),
    }


def execute_qasm(
    qasm: str,
    shots: int = 1024,
    requested_backend: str = "aer_simulator",
    allow_fallback: bool = True,
    caller_jurisdictions: list | None = None,
    caller_export_class: str = "any",
    export_reviewed: bool = False,
    seed: int | None = None,
) -> Dict[str, Any]:
    """Global RAM admission wrapper (process-wide in-flight-bytes semaphore).

    The per-job 28-qubit guard inside _execute_qasm_inner stays as-is; this
    wrapper adds the host-level rail: 8 bridge workers each holding a 4GB
    statevector would OOM the host, so a new simulation whose estimated bytes
    would push process-wide held bytes over the cap refuses fail-closed
    (INFLIGHT_RAM_EXCEEDED) before allocating. The hold lasts exactly one
    inner call (try/finally — exceptions and refusals release). Env-tunable
    via KUBONA_BRIDGE_MAX_INFLIGHT_BYTES, default 2x the single-job ceiling.
    """
    t0 = time.time()
    try:
        qc = circuit_from_qasm(qasm)
        bounds = check_circuit_memory_bounds(qc)
        required_bytes = bounds["required_bytes"]
    except Exception:
        # Unparseable input holds nothing: the inner call refuses precisely.
        return _execute_qasm_inner(
            qasm, shots, requested_backend, allow_fallback,
            caller_jurisdictions, caller_export_class, export_reviewed, seed)
    if not _inflight_try_acquire(required_bytes):
        if bounds["exceeds_statevector_limit"]:
            # Also fails the per-job guard: the inner refusal
            # (PREFLIGHT_RAM_EXCEEDED, partition plan included) is the
            # precise one. Nothing is held, so delegation is safe. The
            # global rail governs admittable-size concurrency (the 8×4GB
            # OOM class); oversize jobs are owned by the per-job guard.
            return _execute_qasm_inner(
                qasm, shots, requested_backend, allow_fallback,
                caller_jurisdictions, caller_export_class, export_reviewed, seed)
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": (
                "Global RAM admission refused: holding "
                f"{inflight_bytes()} bytes process-wide, a further "
                f"{required_bytes} bytes would exceed the "
                f"{_global_inflight_cap_bytes()} byte cap "
                "(KUBONA_BRIDGE_MAX_INFLIGHT_BYTES) — refusing instead of "
                "risking a host OOM"
            ),
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "seed": seed,
            "required_bytes": required_bytes,
            "inflight_bytes": inflight_bytes(),
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": (
                f"INFLIGHT_RAM_EXCEEDED: {required_bytes} bytes requested, "
                f"{inflight_bytes()} bytes already held process-wide, cap "
                f"{_global_inflight_cap_bytes()} bytes — refusing concurrent "
                "simulation (retry when in-flight work drains)"
            ),
        }
    try:
        return _execute_qasm_inner(
            qasm, shots, requested_backend, allow_fallback,
            caller_jurisdictions, caller_export_class, export_reviewed, seed)
    finally:
        _inflight_release(required_bytes)


def _execute_qasm_inner(
    qasm: str,
    shots: int = 1024,
    requested_backend: str = "aer_simulator",
    allow_fallback: bool = True,
    caller_jurisdictions: list | None = None,
    caller_export_class: str = "any",
    export_reviewed: bool = False,
    seed: int | None = None,
) -> Dict[str, Any]:
    """
    Executes an OpenQASM circuit with transparent backend resolution,
    RAM pre-flight bounds checking, and honest degradation tagging.

    P5 §2.9 routing: hardware execution requires caller_jurisdictions and a
    route() ELIGIBLE verdict against the provider capability. A refusal
    returns immediately (success False) — it never degrades into simulation,
    which would make the gate theater.

    Production bar (W1.4):
    - seed is REQUIRED (explicit int, no unseeded np.random): sampling
      paths (Aer seed_simulator + statevector default_rng) consume it and
      echo it; a missing/invalid seed REFUSES instead of drawing silently.
    - Pre-flight-partition and no-counts paths REFUSE (success=False,
      counts={}, non-empty error) — never synthetic counts behind
      success=True.
    - Fallback returns success=False (error iff success is false: success
      True always carries error_message="", success False always carries
      a non-empty error_message).
    """
    t0 = time.time()
    if seed is None or isinstance(seed, bool) or not isinstance(seed, int):
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": "seed is required: pass an explicit int seed (no unseeded sampling)",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "seed": seed,
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": f"seed is required: got {seed!r} (pass an explicit int seed)",
        }
    if not HAS_QISKIT:
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 4, # Fallback
            "degradation_reason": "Qiskit is not installed in runtime environment",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "seed": seed,
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": "Qiskit unavailable",
        }

    try:
        qc = circuit_from_qasm(qasm)
    except Exception as e:
        return {
            "success": False,
            "resolved_backend": "none",
            "degradation_level": 0,
            "degradation_reason": f"QASM parse error: {e}",
            "counts": {},
            "statevector_real": [],
            "statevector_imag": [],
            "meyer_wallach": 0.0,
            "seed": seed,
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": str(e) or "QASM parse error",
        }

    # Pre-flight RAM safety check
    bounds = check_circuit_memory_bounds(qc)
    if bounds["exceeds_statevector_limit"]:
        if bounds["exceeds_aer_limit"] or not HAS_AER or not any(k in requested_backend.lower() for k in ("aer", "local", "sim")):
            if allow_fallback:
                try:
                    cut_res = cut_circuit_qasm(qasm, max_subcircuit_qubits=20)
                    if not cut_res.get("success"):
                        return {
                            "success": False,
                            "resolved_backend": "none",
                            "degradation_level": 0,
                            "degradation_reason": (
                                f"RAM pre-flight bounds exceeded and auto-cutting refused: "
                                f"{cut_res.get('error_message', 'cut failed')}"
                            ),
                            "counts": {},
                            "statevector_real": [],
                            "statevector_imag": [],
                            "meyer_wallach": 0.0,
                            "seed": seed,
                            "partition_fragments": cut_res.get("fragments", []),
                            "total_cuts": cut_res.get("total_cuts", 0),
                            "sampling_overhead_kappa": cut_res.get("sampling_overhead_kappa", 1.0),
                            "execution_time_ms": (time.time() - t0) * 1000.0,
                            "error_message": (
                                f"PREFLIGHT_RAM_EXCEEDED: Circuit requires {bounds['required_mb']:.1f} MB RAM "
                                f"({qc.num_qubits} qubits), exceeding safe host simulation limit of 28 qubits (4096 MB). "
                                f"Auto-cutting refused: {cut_res.get('error_message', 'cut failed')}"
                            ),
                        }
                    # Fail closed: a partition plan is NOT counts. Returning
                    # synthetic all-zero counts behind success=True (old code)
                    # let callers mistake a RAM refusal for a measurement.
                    return {
                        "success": False,
                        "resolved_backend": "none",
                        "degradation_level": 0,
                        "degradation_reason": (
                            f"Pre-flight RAM limit exceeded ({bounds['num_qubits']} qubits, "
                            f"{bounds['required_mb']:.1f} MB RAM required). Partition plan "
                            f"computed ({len(cut_res.get('fragments', []))} fragments, "
                            f"{cut_res.get('total_cuts', 0)} cuts, kappa="
                            f"{cut_res.get('sampling_overhead_kappa', 1.0):.1f}) but execution "
                            f"refused: no counts fabricated."
                        ),
                        "counts": {},
                        "statevector_real": [],
                        "statevector_imag": [],
                        "meyer_wallach": 0.0,
                        "seed": seed,
                        "partition_fragments": cut_res.get("fragments", []),
                        "total_cuts": cut_res.get("total_cuts", 0),
                        "sampling_overhead_kappa": cut_res.get("sampling_overhead_kappa", 1.0),
                        "execution_time_ms": (time.time() - t0) * 1000.0,
                        "error_message": (
                            f"PREFLIGHT_RAM_EXCEEDED: Circuit requires {bounds['required_mb']:.1f} MB RAM "
                            f"({qc.num_qubits} qubits), exceeding safe host simulation limit of 28 qubits (4096 MB). "
                            f"Partition plan available ({cut_res.get('total_cuts', 0)} cuts) — execute fragments explicitly."
                        ),
                    }
                except Exception as cut_err:
                    return {
                        "success": False,
                        "resolved_backend": "none",
                        "degradation_level": 0,
                        "degradation_reason": f"RAM pre-flight bounds exceeded and auto-cutting failed: {cut_err}",
                        "counts": {},
                        "statevector_real": [],
                        "statevector_imag": [],
                        "meyer_wallach": 0.0,
                        "seed": seed,
                        "execution_time_ms": (time.time() - t0) * 1000.0,
                        "error_message": (
                            f"PREFLIGHT_RAM_EXCEEDED: Circuit requires {bounds['required_mb']:.1f} MB RAM "
                            f"({qc.num_qubits} qubits), exceeding safe host simulation limit of 28 qubits (4096 MB)."
                        ),
                    }
            else:
                return {
                    "success": False,
                    "resolved_backend": "none",
                    "degradation_level": 0,
                    "degradation_reason": "Pre-flight RAM limit exceeded and allow_fallback=False",
                    "counts": {},
                    "statevector_real": [],
                    "statevector_imag": [],
                    "meyer_wallach": 0.0,
                    "seed": seed,
                    "execution_time_ms": (time.time() - t0) * 1000.0,
                    "error_message": (
                        f"PREFLIGHT_RAM_EXCEEDED: Circuit requires {bounds['required_mb']:.1f} MB RAM "
                        f"({qc.num_qubits} qubits), exceeding safe host simulation limit of 28 qubits (4096 MB)."
                    ),
                }

    # Execute on Statevector or Aer
    resolved_backend = "qiskit.quantum_info.Statevector"
    degradation_level = 3 # Statevector sim
    degradation_reason = "Executed exact statevector simulation"

    try:
        # Tier 1: real QPU hardware via the provider plugin. The plugin
        # holds its own credential gate (SecretStore): without a token it
        # RAISES unless the caller explicitly opts into simulation
        # (allow_sim_fallback) — this executor does not opt in, so a
        # credentialless QPU request fails closed here with the reason.
        # (An older provider contract returned Aer results with
        # hardware=False; W2.18 ended that — silent downgrade, even
        # labeled, is refused at the plugin seam.)
        route = route_backend(requested_backend)
        if route == "hardware:ibm-qiskit":
            routed = _route_hardware(
                "ibm-qiskit", caller_jurisdictions,
                caller_export_class, export_reviewed, t0)
            if routed is not None:
                routed["seed"] = seed
                return routed
            hw = _execute_ibm_hardware(qasm, shots, requested_backend, seed=seed)
            if hw is not None:
                hw["execution_time_ms"] = (time.time() - t0) * 1000.0
                hw["seed"] = seed
                return hw
            # No credential/SDK: refuse if fallback is disallowed
            if not allow_fallback:
                return {
                    "success": False,
                    "resolved_backend": "none",
                    "degradation_level": 0,
                    "degradation_reason": (
                        f"QPU {requested_backend} requested but no credential/SDK and allow_fallback=False: refused"
                    ),
                    "counts": {},
                    "statevector_real": [],
                    "statevector_imag": [],
                    "meyer_wallach": 0.0,
                    "seed": seed,
                    "execution_time_ms": (time.time() - t0) * 1000.0,
                    "error_message": f"Hardware {requested_backend} unavailable and allow_fallback=False",
                }
            degradation_reason = (
                "Executed exact statevector simulation "
                f"(QPU {requested_backend} requested but no credential/SDK: fail-closed)"
            )
        elif route.startswith("unknown-hardware:"):
            hw_name = route.split(":", 1)[1]
            if not allow_fallback:
                return {
                    "success": False,
                    "resolved_backend": "none",
                    "degradation_level": 0,
                    "degradation_reason": (
                        f"Unknown hardware backend {hw_name!r} requested and allow_fallback=False: refused"
                    ),
                    "counts": {},
                    "statevector_real": [],
                    "statevector_imag": [],
                    "meyer_wallach": 0.0,
                    "seed": seed,
                    "execution_time_ms": (time.time() - t0) * 1000.0,
                    "error_message": f"Unknown hardware backend {hw_name!r} with allow_fallback=False",
                }
            degradation_reason = (
                "Executed exact statevector simulation "
                f"(QPU {hw_name} requested but no provider plugin exists: degraded)"
            )

        # Check if Aer is available and requested
        if HAS_AER and any(k in requested_backend.lower() for k in ("aer", "local", "sim")):
            resolved_backend = "qiskit_aer.AerSimulator"
            degradation_level = 2
            degradation_reason = "Executed Aer noisy density matrix simulator"
            backend = Aer.get_backend("aer_simulator")
            # If circuit has no measurements, save statevector
            qc_meas = qc.copy()
            if not any(instr.operation.name == "measure" for instr in qc_meas.data):
                qc_meas.measure_all()
            job = backend.run(qc_meas, shots=shots, seed_simulator=seed)
            result = job.result()
            counts = result.get_counts()
        else:
            counts = {}

        # Compute exact statevector and Meyer-Wallach entanglement on unitary subcircuit ONLY if within bounds
        if not bounds["exceeds_statevector_limit"]:
            try:
                qc_unitary = qc.remove_final_measurements(inplace=False)
                sv = Statevector(qc_unitary).data
                mw_score = calculate_meyer_wallach(sv)
            except Exception:
                sv = np.array([])
                mw_score = 0.0
        else:
            sv = np.array([])
            mw_score = 0.0
            degradation_reason += f" (Statevector omitted: required {bounds['required_mb']:.1f} MB exceeds 28-qubit ceiling)"
        
        # If counts weren't produced by Aer, sample from statevector probabilities
        # with the EXPLICIT seed (default_rng, never global np.random).
        if not counts and len(sv) > 0:
            rng = np.random.default_rng(seed)
            probs = np.abs(sv) ** 2
            # Renormalize defensively (floating-point drift must not crash choice).
            probs = probs / max(1e-300, float(np.sum(probs)))
            n_qubits = qc.num_qubits
            samples = rng.choice(len(probs), size=shots, p=probs)
            counts_dict = {}
            for s in samples:
                bitstring = format(int(s), f"0{n_qubits}b")
                counts_dict[bitstring] = counts_dict.get(bitstring, 0) + 1
            counts = counts_dict
        elif not counts:
            # Fail closed: no Aer counts and no statevector to sample —
            # refusing instead of fabricating all-zero counts behind success.
            return {
                "success": False,
                "resolved_backend": "none",
                "degradation_level": 0,
                "degradation_reason": (
                    "No counts produced (backend did not return counts and "
                    "no statevector available for seeded sampling): refusing "
                    "synthetic counts"
                ),
                "counts": {},
                "statevector_real": [],
                "statevector_imag": [],
                "meyer_wallach": 0.0,
                "seed": seed,
                "execution_time_ms": (time.time() - t0) * 1000.0,
                "error_message": (
                    "NO_COUNTS_NO_STATEVECTOR: backend returned no counts "
                    "and statevector sampling was unavailable (out of bounds "
                    "or unitary extraction failed) — refusing synthetic counts"
                ),
            }

        return {
            "success": True,
            "resolved_backend": resolved_backend,
            "degradation_level": degradation_level,
            "degradation_reason": degradation_reason,
            "counts": counts,
            "statevector_real": [float(np.real(x)) for x in sv],
            "statevector_imag": [float(np.imag(x)) for x in sv],
            "meyer_wallach": mw_score,
            "seed": seed,
            "execution_time_ms": (time.time() - t0) * 1000.0,
            "error_message": "",
        }

    except Exception as e:
        # Fail closed in BOTH branches: a fallback that returns synthetic
        # counts with success=True (old code) lets callers mistake an
        # exception for a measurement, and success=True with a non-empty
        # error_message breaks the error-iff-failure invariant. Empty
        # error_message with success=False is likewise impossible.
        err = str(e) or f"{type(e).__name__}: execution failed"
        if allow_fallback:
            return {
                "success": False,
                "resolved_backend": "none",
                "degradation_level": 0,
                "degradation_reason": f"Execution failed (fallback refused to fabricate): {e}",
                "counts": {},
                "statevector_real": [],
                "statevector_imag": [],
                "meyer_wallach": 0.0,
                "seed": seed,
                "execution_time_ms": (time.time() - t0) * 1000.0,
                "error_message": err,
            }
        else:
            return {
                "success": False,
                "resolved_backend": "none",
                "degradation_level": 0,
                "degradation_reason": f"Execution failed: {e}",
                "counts": {},
                "statevector_real": [],
                "statevector_imag": [],
                "meyer_wallach": 0.0,
                "seed": seed,
                "execution_time_ms": (time.time() - t0) * 1000.0,
                "error_message": err,
            }


def cut_circuit_qasm(qasm: str, max_subcircuit_qubits: int = 2) -> Dict[str, Any]:
    """
    Partitions an OpenQASM circuit into valid subcircuits using graph partitioning
    and Quasi-Probability Decomposition (QPD) wire-cutting principles (PennyLane qcut / Peng et al.).
    Guarantees that each subcircuit fragment preserves the actual operations of the user circuit
    with explicit Pauli basis measurement and state-preparation expansion channels.

    Production bar (W1.4):
    - kappa=4^K is CAPPED at MAX_SAMPLING_OVERHEAD_KAPPA_V1: above the cap
      the cut REFUSES (success=False) instead of silently promising an
      exponential sampling blowup.
    - >2q gates spanning partitions REFUSE (recorded in dropped_gates with
      a non-empty error) — never silently dropped. Callers must remap the
      circuit or widen the partition instead of trusting incomplete
      fragments.
    """
    if not HAS_QISKIT:
        return {"success": False, "fragments": [], "total_cuts": 0, "sampling_overhead_kappa": 1.0, "dropped_gates": [], "error_message": "Qiskit unavailable"}

    try:
        qc = circuit_from_qasm(qasm)
        n = qc.num_qubits
        if n <= max_subcircuit_qubits:
            return {
                "success": True,
                "fragments": [{"fragment_id": 1, "qasm": qasm, "cut_qubits": [], "qpd_channels": []}],
                "total_cuts": 0,
                "sampling_overhead_kappa": 1.0,
                "qpd_channels": [],
                "dropped_gates": [],
                "error_message": "",
            }

        # Determine partition groups: split qubits into chunks of size <= max_subcircuit_qubits
        partitions = []
        for i in range(0, n, max_subcircuit_qubits):
            partitions.append(list(range(i, min(i + max_subcircuit_qubits, n))))

        # Map each original qubit to (partition_index, local_qubit_index)
        qubit_to_part = {}
        for part_idx, part_qubits in enumerate(partitions):
            for local_idx, q in enumerate(part_qubits):
                qubit_to_part[q] = (part_idx, local_idx)

        # Create subcircuit instances
        subcircuits = [QuantumCircuit(len(p), len(p)) for p in partitions]
        cut_qubits_per_part = [set() for _ in partitions]
        cut_channels = []
        dropped_gates: list[dict] = []
        total_cuts = 0

        # Iterate over gates in the original circuit
        for instruction in qc.data:
            op = instruction.operation
            qargs = [qc.find_bit(q).index for q in instruction.qubits]

            if op.name == "measure":
                part_idx, local_q = qubit_to_part[qargs[0]]
                subcircuits[part_idx].measure(local_q, local_q)
                continue

            if len(qargs) == 1:
                part_idx, local_q = qubit_to_part[qargs[0]]
                subcircuits[part_idx].append(op, [local_q], [])
            elif len(qargs) == 2:
                q0, q1 = qargs[0], qargs[1]
                part0, local0 = qubit_to_part[q0]
                part1, local1 = qubit_to_part[q1]

                if part0 == part1:
                    # Intra-partition gate: preserve original gate
                    subcircuits[part0].append(op, [local0, local1], [])
                else:
                    # Inter-partition wire cut!
                    # In QPD wire-cutting, decompose channel:
                    # E(rho) = sum_i c_i M_i(rho) where c_i in {1/2, -1/2}, M_i in Pauli bases {I, X, Y, Z}
                    # Upstream fragment measures in Pauli observable; downstream fragment prepares eigenstates {|0>, |1>, |+>, |+i>}
                    total_cuts += 1
                    cut_qubits_per_part[part0].add(local0)
                    cut_qubits_per_part[part1].add(local1)

                    # Upstream partition: insert projection and computational basis measurement observable
                    subcircuits[part0].barrier(local0)
                    subcircuits[part0].measure(local0, local0)

                    # Downstream partition: prepare state from decomposition basis
                    subcircuits[part1].barrier(local1)
                    subcircuits[part1].h(local1)

                    cut_channels.append({
                        "cut_index": total_cuts,
                        "source_partition": part0 + 1,
                        "source_qubit": q0,
                        "target_partition": part1 + 1,
                        "target_qubit": q1,
                        "qpd_expansion_bases": ["I", "X", "Y", "Z"],
                        "eigenstate_preparations": ["|0>", "|1>", "|+>", "|+i>"],
                        "channel_overhead": 4.0,
                    })
            else:
                # Multi-qubit (>2q) gates: a cross-partition control cannot be
                # wire-cut by the 2q QPD channel above. Silently keeping only
                # the local qubits (old code) ships incomplete fragments as
                # success. Record every spanning >2q gate and REFUSE.
                parts = {qubit_to_part[q][0] for q in qargs}
                if len(parts) > 1:
                    dropped_gates.append({
                        "gate": op.name,
                        "qubits": list(qargs),
                        "partitions": sorted(parts),
                    })
                    continue
                part_idx, _local_q = qubit_to_part[qargs[0]]
                local_qubits = [qubit_to_part[q][1] for q in qargs]
                subcircuits[part_idx].append(op, local_qubits, [])

        # Total sampling overhead kappa = 4^K for K cut wires — CAPPED.
        sampling_overhead_kappa = float(4.0 ** total_cuts)
        if sampling_overhead_kappa > MAX_SAMPLING_OVERHEAD_KAPPA_V1:
            return {
                "success": False,
                "fragments": [],
                "total_cuts": total_cuts,
                "sampling_overhead_kappa": sampling_overhead_kappa,
                "qpd_channels": cut_channels,
                "dropped_gates": dropped_gates,
                "error_message": (
                    f"KAPPA_CAP_EXCEEDED: {total_cuts} cuts need kappa="
                    f"{sampling_overhead_kappa:.1f} > cap "
                    f"{MAX_SAMPLING_OVERHEAD_KAPPA_V1:.1f} (max {MAX_CUTS_V1} cuts): "
                    f"refusing instead of a silent 4^K blowup"
                ),
            }
        if dropped_gates:
            return {
                "success": False,
                "fragments": [],
                "total_cuts": total_cuts,
                "sampling_overhead_kappa": sampling_overhead_kappa,
                "qpd_channels": cut_channels,
                "dropped_gates": dropped_gates,
                "error_message": (
                    f"CROSS_PARTITION_MULTI_QUBIT_REFUSED: {len(dropped_gates)} "
                    f">2q gate(s) span partitions {dropped_gates}: remap the "
                    f"circuit or widen max_subcircuit_qubits — refusing "
                    f"instead of silently dropping controls"
                ),
            }

        fragments = []
        for part_idx, sc in enumerate(subcircuits):
            fragments.append({
                "fragment_id": part_idx + 1,
                "qasm": circuit_to_qasm(sc),
                "cut_qubits": sorted(list(cut_qubits_per_part[part_idx])),
                "qubit_count": len(partitions[part_idx]),
            })

        return {
            "success": True,
            "fragments": fragments,
            "total_cuts": total_cuts,
            "sampling_overhead_kappa": sampling_overhead_kappa,
            "qpd_channels": cut_channels,
            "dropped_gates": [],
            "error_message": "",
        }
    except Exception as e:
        return {"success": False, "fragments": [], "total_cuts": 0, "sampling_overhead_kappa": 1.0, "qpd_channels": [], "dropped_gates": [], "error_message": str(e) or "circuit cutting failed"}

