"""Canary: a Bell-state circuit, expected 50/50 on 00/11, nothing else.

Catches: silent API breakage, calibration drift, provider misconfiguration —
the regression class the plan says must be caught in staging before it reaches
an agent.
"""

from __future__ import annotations

import sys
from typing import Any

from kubona.circuits import parse_qasm2
from kubona.providers import ProviderError, get_provider, list_providers
from kubona.provenance import Ledger

CANARY_QASM = "OPENQASM 2.0;\nqreg q[2];\nh q[0];\ncx q[0],q[1];"
CANARY_SHOTS = 2048


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
    return {"provider": provider_name, "ok": ok, "counts": counts,
            "fidelity_balance": round(abs(ok00 - ok11) / max(total, 1), 6), "checks": checks}


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
                                 "correctness": res.get("correctness")})
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
