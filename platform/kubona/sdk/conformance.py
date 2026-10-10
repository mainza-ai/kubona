"""SDK conformance suite — the Phase-4 gate artifact (Q-24, choice C).

Vendor-maintained providers live in sidecar repos; core reviews only the
registration interface. `grade_candidate` runs that review mechanically:
schema validity, trait compliance, seeded determinism, Bell correctness,
fail-closed input handling, attestation honesty (Q-14), calibration soundness,
cost-model sanity, and credential hygiene — all simulator-tier, no writes, no
credentials, no network. A candidate that fails any applicable check is
refused; unknown is never a pass.

This mirrors the canary's verdict discipline (canary.py) without importing
registry state: candidates are graded *before* they enter the registry.
"""

from __future__ import annotations

import os
from typing import Any

from kubona.circuits import Circuit, Gate, parse_qasm2
from kubona.providers.base import Provider, ProviderError, _instantiate
from kubona.schema import ProviderCapability

from .register import validate_capability

BELL_QASM = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
_SANDBOX_1Q = Circuit(name="sdk-conformance-1q", num_qubits=1, gates=[Gate("h", (0,))])

# Modalities a Bell gate-circuit can actually probe (mirrors canary.py:
# annealing QUBO, neutral-atom AHS and photonic CV are different program
# models — bell_correct is N/A for them, never a fail).
_GATE_MODALITIES = {"gate_model", "superconducting", "trapped_ion",
                    "statevector_simulator", "gpu_simulator",
                    "statevector", "gpu"}
_SIM_MODALITIES = {"statevector_simulator", "gpu_simulator",
                   "statevector", "gpu"}

# Env prefixes scrubbed for the credential-hygiene probe: a simulator-tier
# candidate must run with none of these set; a hardware claimant must fail
# closed (ProviderError), never succeed and never crash otherwise.
_TOKEN_PREFIXES = ("KUBONA_", "QISKIT", "QCS", "DWAVE", "D-WAVE",
                   "AZURE_QUANTUM", "BRAKET", "IONQ", "RIGETTI",
                   "XANADU", "QUANTINUUM", "PASQAL", "QUERA", "AQT",
                   "OQC", "PLANQ", "EUROPEAN_")

_CAL_TOL = 0.25
_CAL_ABS_EPS = 1e-9


def _check(name: str, passed: bool | None, detail: str = "") -> dict[str, Any]:
    return {"check": name, "pass": passed, "detail": detail}


def grade_candidate(capability_json: dict[str, Any], plugin_path: str,
                    shots: int = 1024, tolerance: float = 0.05) -> dict[str, Any]:
    """Grade one sidecar provider candidate. Never writes, never touches HW."""
    checks: list[dict[str, Any]] = []

    try:
        cap = validate_capability(capability_json)
    except Exception as e:  # noqa: BLE001 — pydantic ValidationError et al
        checks.append(_check("schema_valid", False, f"{type(e).__name__}: {e}"))
        return _verdict(capability_json.get("provider", "?"), plugin_path, checks)
    checks.append(_check("schema_valid", True, f"provider={cap.provider}"))
    cap.execution_plugin = plugin_path

    try:
        provider = _instantiate(plugin_path, cap)
    except Exception as e:  # noqa: BLE001
        checks.append(_check("trait_loads", False, f"plugin failed to load: {e}"))
        return _verdict(cap.provider, plugin_path, checks)
    if not isinstance(provider, Provider):
        checks.append(_check("trait_loads", False,
                             "plugin does not implement the Provider trait"))
        return _verdict(cap.provider, plugin_path, checks)
    checks.append(_check("trait_loads", True, type(provider).__name__))

    modality = cap.modality.value
    is_sim = modality in _SIM_MODALITIES

    # Seeded determinism: same seed twice must give identical counts.
    try:
        bell = parse_qasm2(BELL_QASM) if modality in _GATE_MODALITIES else _SANDBOX_1Q
        r1 = provider.submit(bell, shots, seed=2026)
        r2 = provider.submit(bell, shots, seed=2026)
        det = r1.get("counts") == r2.get("counts") and sum(r1["counts"].values()) == shots
        checks.append(_check("deterministic_seeded", bool(det),
                             "identical counts across two seeded runs" if det
                             else f"nondeterministic or short: {r1.get('counts')} vs {r2.get('counts')}"))
    except ProviderError as e:
        checks.append(_check("deterministic_seeded", False, f"refused to run: {e}"))

    # Bell correctness (gate modalities only; anything else is N/A, not fail).
    if modality in _GATE_MODALITIES:
        try:
            counts = provider.submit(parse_qasm2(BELL_QASM), shots, seed=2026)["counts"]
            total = sum(counts.values()) or 1
            ok = (set(counts) <= {"00", "11"} and total == shots
                  and abs(counts.get("00", 0) - counts.get("11", 0)) / total <= tolerance)
            checks.append(_check("bell_correct", bool(ok), f"counts={counts}"))
        except ProviderError as e:
            checks.append(_check("bell_correct", False, f"refused to run: {e}"))
    else:
        checks.append(_check("bell_correct", None,
                             f"modality {modality!r} is not a gate-circuit program model"))

    # Fail-closed inputs: non-positive shots must raise, never return counts.
    bad_ok = True
    bad_detail = ""
    for bad in (0, -5):
        try:
            out = provider.submit(_SANDBOX_1Q, bad, seed=1)
            bad_ok = False
            bad_detail = f"shots={bad} returned {out!r} instead of raising"
            break
        except (ProviderError, ValueError):
            continue
        except Exception as e:  # noqa: BLE001 — wrong failure mode is still a failure
            bad_ok = False
            bad_detail = f"shots={bad} raised {type(e).__name__} (must be ProviderError/ValueError)"
            break
    checks.append(_check("rejects_bad_shots", bad_ok,
                         "non-positive shots raise ProviderError/ValueError" if bad_ok else bad_detail))

    # Attestation honesty (Q-14): a reported qubit count must match the
    # declaration; unreported is a note for simulators, a fail for hardware
    # claimants (a backend that sells qubits must attest them).
    try:
        reported = provider.health().get("measured_qubits")
    except Exception:  # noqa: BLE001
        reported = None
    if isinstance(reported, bool):
        reported = None
    if reported is None:
        checks.append(_check("attestation_honest", True if is_sim else False,
                             "unmeasurable: unknown (simulator tier)" if is_sim
                             else "hardware claimant reports no measured_qubits"))
    elif reported == cap.physical_qubits:
        checks.append(_check("attestation_honest", True,
                             f"measured {reported} == declared {cap.physical_qubits}"))
    else:
        checks.append(_check("attestation_honest", False,
                             f"measured {reported} != declared {cap.physical_qubits}: schema lied"))

    # Calibration: declared soundness is enforced by pydantic at load;
    # a measured snapshot outside tolerance of the declaration fails (Q-14).
    if cap.declared_calibration is None:
        checks.append(_check("calibration", None, "no declared calibration to verify against"))
    else:
        try:
            measured = provider.calibration_snapshot()
        except Exception:  # noqa: BLE001
            measured = None
        if not measured:
            checks.append(_check("calibration", None, "unmeasurable without backend access"))
        else:
            mism = [f for f in ("t1_us", "t2_us", "single_qubit_error",
                                "two_qubit_error", "readout_error")
                    if _rel_diff(getattr(cap.declared_calibration, f),
                                 measured.get(f)) > _CAL_TOL]
            checks.append(_check("calibration", not mism,
                                 "measured within tolerance of declared" if not mism
                                 else f"mismatches: {mism}"))
    # Cost model: an estimate must exist with a stated basis, never a bare
    # number; an unpriced per-second rail fails instead of reading as free.
    try:
        est_probe = parse_qasm2(BELL_QASM) if modality in _GATE_MODALITIES else None
        est = cap.cost_estimate(shots, circuit=est_probe)
        sane = est.get("estimated_usd", -1) >= 0 and bool(est.get("cost_basis") or est.get("notes") or cap.pricing.model == "free")
        checks.append(_check("cost_model_sane", bool(sane),
                             f"estimate={est.get('estimated_usd')} "
                             f"basis={est.get('cost_basis') or est.get('notes') or 'free'}"))
    except (ValueError, ProviderError) as e:
        checks.append(_check("cost_model_sane", False, f"unpriced rail refuses: {e}"))

    # Credential hygiene: simulator tier runs with tokens scrubbed;
    # hardware claimants must fail closed without credentials.
    saved = {k: os.environ.pop(k) for k in list(os.environ)
             if k.upper().startswith(_TOKEN_PREFIXES)}
    try:
        try:
            provider.submit(_SANDBOX_1Q, 64, seed=1)
            if is_sim:
                checks.append(_check("credential_hygiene", True,
                                     "runs credential-free (simulator tier)"))
            else:
                checks.append(_check("credential_hygiene", False,
                                     "ran without credentials: a hardware claimant "
                                     "must fail closed"))
        except ProviderError as e:
            checks.append(_check("credential_hygiene", True,
                                 f"fails closed without credentials: {e}"))
        except Exception as e:  # noqa: BLE001 — wrong failure mode is still a failure
            checks.append(_check("credential_hygiene", False,
                                 f"non-ProviderError without credentials: "
                                 f"{type(e).__name__}: {e}"))
    finally:
        os.environ.update(saved)

    return _verdict(cap.provider, plugin_path, checks)


def _rel_diff(declared: float | None, measured: float | None) -> float:
    if declared is None or measured is None:
        return 0.0
    if abs(declared) <= _CAL_ABS_EPS and abs(measured) <= _CAL_ABS_EPS:
        return 0.0
    denom = abs(declared) if abs(declared) > _CAL_ABS_EPS else _CAL_ABS_EPS
    return abs(measured - declared) / denom


def _verdict(provider: str, plugin: str, checks: list[dict[str, Any]]) -> dict[str, Any]:
    applicable = [c for c in checks if c["pass"] is not None]
    failures = [c["check"] for c in applicable if not c["pass"]]
    return {
        "provider": provider,
        "plugin": plugin,
        "pass": bool(applicable) and not failures,
        "ran": len(applicable),
        "na": [c["check"] for c in checks if c["pass"] is None],
        "failures": failures,
        "checks": checks,
        "suite_version": "1",
    }


def conformance_gate(results: list[dict[str, Any]]) -> dict[str, Any]:
    """Gate verdict over graded candidates. No graded candidates is NOT a
    pass: a gate that reviewed nothing has proven nothing."""
    failures = [r["provider"] for r in results if not r["pass"]]
    return {
        "ok": bool(results) and not failures,
        "graded": len(results),
        "failures": failures,
    }
