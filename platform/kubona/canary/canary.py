"""Canary: a Bell-state circuit, expected 50/50 on 00/11, nothing else.

Catches: silent API breakage, calibration drift, provider misconfiguration —
the regression class the plan says must be caught in staging before it reaches
an agent.
"""

from __future__ import annotations

import hashlib
import json
import sys
from datetime import datetime, timezone
from typing import Any

from kubona.circuits import parse_qasm2
from kubona.providers import ProviderError, get_provider, list_providers
from kubona.provenance import Ledger

CANARY_QASM = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
CANARY_SHOTS = 2048


def wilson_interval(k: int, n: int, z: float = 1.96) -> tuple[float, float]:
    """Wilson score interval for a binomial proportion (95% by default).

    A canary that reports 50/50 without an interval hides whether it ran 20
    shots or 20,000. The interval travels with every proportion the canary
    reports; the pass/fail verdict logic is untouched (additive fields only).
    Edge cases are exact by construction: k=0 pins the lower bound at 0,
    k=n pins the upper at 1.
    """
    if n <= 0:
        raise ValueError("shots must be positive for an interval")
    if not 0 <= k <= n:
        raise ValueError(f"successes {k} outside [0, {n}]")
    return _wilson(k, n, z)


def _wilson(k: int, n: int, z: float) -> tuple[float, float]:
    p = k / n
    denom = 1 + z * z / n
    center = (p + z * z / (2 * n)) / denom
    half = z * ((p * (1 - p) / n + z * z / (4 * n * n)) ** 0.5) / denom
    lo, hi = max(0.0, center - half), min(1.0, center + half)
    if k == 0:
        lo = 0.0
    if k == n:
        hi = 1.0
    return (round(lo, 6), round(hi, 6))


def canary_check(provider_name: str, shots: int = CANARY_SHOTS,
                 tolerance: float = 0.05) -> dict[str, Any]:
    """Run the canary circuit on one provider; return per-check verdicts."""
    provider = get_provider(provider_name)
    circuit = parse_qasm2(CANARY_QASM)
    result = provider.submit(circuit, shots, seed=2026)
    counts = result["counts"]
    total = sum(counts.values()) or 1
    ok00 = counts.get("00", 0)
    ok11 = counts.get("11", 0)
    checks = {
        "only_00_or_11": set(counts) <= {"00", "11"},
        "total_shots": total == shots,
        "balanced_50_50": abs(ok00 - ok11) / max(total, 1) <= tolerance,
    }
    ok = all(checks.values())
    attestation = attest_capacity(provider_name, ok)
    if attestation.get("calibration", {}).get("calibration_ok") is False:
        ok = False  # Q-14: a lying schema fails the canary, not just the attestation
    intervals = {
        outcome: {"k": counts.get(outcome, 0),
                  "interval_95": wilson_interval(counts.get(outcome, 0), total)}
        for outcome in sorted(counts)
    }
    return {"provider": provider_name, "ok": ok, "counts": counts,
            "fidelity_balance": round(abs(ok00 - ok11) / max(total, 1), 6), "checks": checks,
            "intervals": intervals,
            "attestation": attestation}


def attest_capacity(provider_name: str, canary_ok: bool) -> dict[str, Any]:
    """Measured-vs-declared qubit attestation (Q-14).

    Compares the plugin's live `health()["measured_qubits"]` against the
    capability JSON's declared `physical_qubits`. A mismatch is a failed
    attestation (the schema lied); an unmeasurable plugin yields
    `qubit_count_ok=None` labeled unknown — never a pass. The snapshot hash
    seals the attestation for ledger admission checks.
    """
    from kubona.schema import load_provider
    cap = load_provider(provider_name)
    measured: int | None = None
    try:
        plugin = get_provider(provider_name)
        reported = plugin.health().get("measured_qubits")
        if isinstance(reported, bool):
            measured = None
        elif isinstance(reported, int):
            measured = reported
    except Exception:
        measured = None
    if measured is None:
        verdict: bool | None = None
    else:
        verdict = (measured == cap.physical_qubits)
    checked_at = datetime.now(timezone.utc).isoformat()
    snapshot = {
        "provider": provider_name,
        "declared_qubits": cap.physical_qubits,
        "measured_qubits": measured,
        "qubit_count_ok": verdict,
        "canary_ok": bool(canary_ok),
        "checked_at": checked_at,
    }
    snapshot["snapshot_hash"] = hashlib.sha256(
        json.dumps(snapshot, sort_keys=True).encode()).hexdigest()
    snapshot["calibration"] = attest_calibration(provider_name)
    # A lying schema fails the canary: measured calibration outside tolerance
    # of the declared numbers is a rejected claim (Q-14), not a warning.
    if snapshot["calibration"].get("calibration_ok") is False:
        snapshot["calibration_rejection"] = snapshot["calibration"].get("mismatches")
    return snapshot


# Relative tolerance per calibration field. Zeros compare exact within an
# absolute epsilon (a noiseless claim must actually be noiseless).
_CALIBRATION_TOL = {
    "t1_us": 0.25, "t2_us": 0.25,
    "single_qubit_error": 0.25, "two_qubit_error": 0.25,
    "readout_error": 0.25,
}
_CALIBRATION_ABS_EPS = 1e-9


def attest_calibration(provider_name: str) -> dict[str, Any]:
    """Q-14: compare measured vs declared calibration.

    Returns {compared: bool, calibration_ok: True|False|None, mismatches: []}.
    None = unmeasurable or undeclared (unknown, never a pass, never a fail).
    False = measured outside tolerance of declared (schema lied → canary fails).
    """
    from kubona.schema import load_provider
    try:
        cap = load_provider(provider_name)
    except KeyError:
        return {"compared": False, "calibration_ok": None,
                "reason": "no capability schema"}
    declared = cap.declared_calibration
    if declared is None:
        return {"compared": False, "calibration_ok": None,
                "reason": "no declared calibration to verify against"}
    try:
        from kubona.providers import get_provider
        measured = get_provider(provider_name).calibration_snapshot()
    except Exception:
        measured = None
    if not measured:
        return {"compared": False, "calibration_ok": None,
                "reason": "unmeasurable without credentials/backend access"}
    mismatches = []
    for field, tol in _CALIBRATION_TOL.items():
        d = getattr(declared, field, None)
        m = measured.get(field)
        if d is None or m is None:
            continue  # only compare what both sides state
        if abs(d) <= _CALIBRATION_ABS_EPS and abs(m) <= _CALIBRATION_ABS_EPS:
            continue
        denom = abs(d) if abs(d) > _CALIBRATION_ABS_EPS else _CALIBRATION_ABS_EPS
        if abs(m - d) / denom > tol:
            mismatches.append({"field": field, "declared": d, "measured": m})
    return {"compared": True,
            "calibration_ok": not mismatches,
            "mismatches": mismatches,
            "measured_source": measured.get("source")}


# Gate-model modalities that a Bell-state canary circuit can actually exercise.
# Annealing (QUBO), neutral-atom AHS, and photonic CV are different program
# models — a Bell gate-circuit is not a valid probe for them, so they are not
# canaried (a provider-registry concern, not a regression signal here).
_GATE_MODALITIES = {"gate_model", "superconducting", "trapped_ion",
                    "statevector_simulator", "gpu_simulator", "statevector",
                    "gpu"}


def run_all(ledger: Ledger | None = None, providers: list[str] | None = None,
           use_correctness_harness: bool = True) -> list[dict[str, Any]]:
    """Canary every gate-model provider with an executable plugin.

    The Bell canary catches provider*execution* regressions. Providers that
    fail closed for config (missing credentials/SDK/unsupported gate) are
    reported as `skip` — NOT a regression — so the gate only blocks release on
    genuine execution health problems. Each run is logged to the ledger.
    """
    ledger = ledger or Ledger()
    if providers is None:
        providers = [
            name for name, cap in list_providers().items()
            if cap.execution_plugin and cap.modality.value in _GATE_MODALITIES
        ]
    out = []
    for name in providers:
        try:
            res = canary_check(name)
            if use_correctness_harness:
                from kubona.autoresearch import build_correctness_harness
                cr = build_correctness_harness(provider=name).run(None)
                res["correctness"] = cr
                res["ok"] = res["ok"] and cr["pass"]
        except ProviderError as e:  # fail-closed config (missing creds/SDK) -> skip, not regression
            res = {"provider": name, "ok": False, "error": str(e),
                   "checks": {}, "skip": True}
        except Exception as e:  # noqa: BLE001 — genuine execution regression
            res = {"provider": name, "ok": False, "error": str(e), "checks": {}, "skip": False}
        out.append(res)
        ledger.append("canary", {"provider": name, "ok": res["ok"], "skip": res.get("skip", False),
                                 "counts": res.get("counts"), "error": res.get("error"),
                                 "correctness": res.get("correctness"),
                                 "intervals": res.get("intervals"),
                                 "attestation": res.get("attestation")})
    return out


def release_gate(results: list[dict[str, Any]]) -> dict[str, Any]:
    """Turn raw canary results into a release-gate verdict.

    This is the ONLY place the gate rule lives: CI (hosted and local) calls it
    through `python -m kubona.canary --gate` instead of re-deriving an assertion,
    because two copies of the rule are how the gate went red for a month while
    being green locally.

    A result marked `skip` failed *closed* on configuration (missing
    credentials/SDK) or runs a program model the Bell circuit cannot probe
    (annealing QUBO, neutral-atom AHS, photonic CV). That is a provider-registry
    fact, not an execution regression, so it does not block release. A provider
    that actually ran and came back unhealthy always blocks release.

    An all-skip run is deliberately NOT a pass: a gate that executed nothing has
    proven nothing, and would go green if the last executable plugin were removed.
    """
    ran = [r for r in results if not r.get("skip")]
    skipped = [r for r in results if r.get("skip")]
    failures = [r for r in ran if not r["ok"]]
    return {
        "ok": bool(ran) and not failures,
        "ran": len(ran),
        "skipped": len(skipped),
        "failures": [{"provider": r["provider"], "error": r.get("error"),
                      "checks": r.get("checks")} for r in failures],
        "skip_reasons": {r["provider"]: r.get("error", "fail-closed") for r in skipped},
    }


def main(argv: list[str] | None = None) -> int:
    import argparse

    ap = argparse.ArgumentParser(description="Kubona hardware-canary CI")
    ap.add_argument("--provider", default=None, help="canary a single provider")
    ap.add_argument("--shots", type=int, default=CANARY_SHOTS)
    ap.add_argument("--tolerance", type=float, default=0.05)
    ap.add_argument("--gate", action="store_true",
                    help="evaluate the release gate over every provider with an execution "
                         "plugin (modality-blind): fail-closed config/program-model gaps are "
                         "skips, genuine execution regressions fail the build")
    args = ap.parse_args(argv)

    if args.provider:
        results = [canary_check(args.provider, args.shots, args.tolerance)]
    elif args.gate:
        # Canonical gate selection: every plugin-bearing provider, modality-blind.
        # Non-gate modalities report skip=True rather than being hidden from the run,
        # so the gate's coverage is visible in the log instead of filtered away.
        results = run_all(providers=[
            name for name, cap in list_providers().items() if cap.execution_plugin
        ])
    else:
        results = run_all()  # gate-model executable providers; credential/SDK gaps are skips

    failures = 0
    skipped = 0
    for r in results:
        if r.get("skip"):
            print(f"[SKIP] {r['provider']}: {r.get('error', 'fail-closed (no credentials/SDK)')}")
            skipped += 1
            continue
        status = "PASS" if r["ok"] else "FAIL"
        print(f"[{status}] {r['provider']}: {r.get('checks', {})}")
        failures += 0 if r["ok"] else 1
    ran = len(results) - skipped
    print(f"canary summary: {ran - failures}/{ran} passed ({skipped} skipped, no credentials)")

    if args.gate:
        verdict = release_gate(results)
        for f in verdict["failures"]:
            print(f"[GATE-FAIL] {f['provider']}: {f.get('error') or f.get('checks')}")
        if verdict["ran"] == 0:
            print("[GATE-FAIL] no executable provider ran — the gate proved nothing")
        print(f"release gate: {'OK' if verdict['ok'] else 'FAIL'} "
              f"({verdict['ran']} executable providers; {verdict['skipped']} skipped)")
        return 0 if verdict["ok"] else 1
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
