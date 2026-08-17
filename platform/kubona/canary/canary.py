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


def main(argv: list[str] | None = None) -> int:
    import argparse

    ap = argparse.ArgumentParser(description="Kubona hardware-canary CI")
    ap.add_argument("--provider", default=None, help="canary a single provider")
    ap.add_argument("--shots", type=int, default=CANARY_SHOTS)
    ap.add_argument("--tolerance", type=float, default=0.05)
    args = ap.parse_args(argv)

    if args.provider:
        results = [canary_check(args.provider, args.shots, args.tolerance)]
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
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
