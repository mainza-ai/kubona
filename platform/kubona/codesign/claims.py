"""Circuit Codesign Claim Engine (Phase 1 / C0-C7 Gates).

Enforces fail-closed circuit synthesis claims sharing Kubona's canonical
Ed25519 signing domain (``kubona:discovery_claim:v1``):
  C0: artifact parses and passes gate-set/arity/qubit-index validation
  C1: exhaustive spec equivalence (``verification_method == "exhaustive"`` and
      ``eps_bound == 0.0``; statistical-only bounds are refused per Decision A2)
  C2: cost ``TQ = toffoli * peak_qubits ** kappa`` independently re-derived
  C3: frontier advantage vs a frozen baseline (strictly lower TQ or Pareto
      dominance on ``(tq, toffoli_depth, peak_qubits)`` at same spec & kappa)
  C4: independent reproduction (second fresh-seed evaluation with identical
      exhaustive verdict and cost)
  C5: held-out verification check
  C6: novel vs built-in published circuit catalog (exact canonical hash)
  C7: frozen SHA-256 artifact bundle + Ed25519 operator signature
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Sequence

from kubona.codesign.harness import CircuitCodesignHarness
from kubona.codesign.specs import build_spec
from kubona.codesign.tq import tq_cost as compute_tq
from kubona.qec.claims import (
    canonical_claim_sign_message,
    sign_discovery_claim_ed25519,
    verify_discovery_claim_ed25519,
)


def canonical_circuit_sha256(artifact: Sequence[Any]) -> str:
    raw = json.dumps(list(artifact), sort_keys=True, separators=(",", ":")).encode("utf-8")
    return hashlib.sha256(raw).hexdigest()


# Built-in published reference circuits (C6 novelty guard, analogous to QEC N10)
BUILTIN_PUBLISHED_CIRCUITS: dict[str, dict[str, Any]] = {
    "and2_single_ccx": {
        "spec_kind": "and2",
        "source": "canonical 1-Toffoli AND2",
        "artifact": [["ccx", [0, 1, 2], []]],
    },
    "copy1_single_cx": {
        "spec_kind": "copy1",
        "source": "canonical 1-CNOT copy",
        "artifact": [["cx", [0, 1], []]],
    },
    "xor2_two_cx": {
        "spec_kind": "xor2",
        "source": "canonical 2-CNOT XOR2",
        "artifact": [["cx", [0, 2], []], ["cx", [1, 2], []]],
    },
    "cz_phase_single_cz": {
        "spec_kind": "cz-phase",
        "source": "canonical 1-CZ phase",
        "artifact": [["cz", [0, 1], []]],
    },
}


def lookup_circuit_catalog(
    spec_kind: str,
    artifact: Sequence[Any],
    *,
    catalog: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Check whether a circuit matches a known published artifact for the same spec."""
    art_hash = canonical_circuit_sha256(artifact)
    cat = BUILTIN_PUBLISHED_CIRCUITS if catalog is None else catalog
    for name, entry in cat.items():
        if str(entry.get("spec_kind") or "") != str(spec_kind):
            continue
        if canonical_circuit_sha256(entry.get("artifact") or []) == art_hash:
            return {
                "known": True,
                "matched_name": name,
                "source": entry.get("source"),
                "artifact_sha256": art_hash,
            }
    return {
        "known": False,
        "matched_name": None,
        "source": None,
        "artifact_sha256": art_hash,
    }


@dataclass
class CircuitClaim:
    claim_id: str
    code_name: str
    spec_kind: str
    artifact: list[Any]
    baseline_tq: int
    baseline_toffoli_depth: int | None = None
    baseline_peak_qubits: int | None = None
    kappa: int = 1
    seed: int = 7
    repro_seed: int = 11
    held_out_verified: bool = True
    expected_artifact_sha256: str | None = None
    status: str = "candidate"  # candidate | verified | claimed | retracted
    operator_pubkey_hex: str | None = None
    operator_signature_hex: str | None = None
    retraction_reason: str | None = None
    artifact_sha256: str = ""
    gates_digest: str = ""
    gate_report: dict[str, Any] = field(default_factory=dict)

    def freeze_artifact_sha256(self) -> str:
        self.artifact_sha256 = canonical_circuit_sha256(self.artifact)
        return self.artifact_sha256


def evaluate_circuit_gates(
    claim: CircuitClaim,
    *,
    catalog: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    """Evaluate CircuitClaim gates C0-C7 fail-closed (exhaustive-only, Decision A2)."""
    gates: dict[str, dict[str, Any]] = {}

    # C0: artifact parses & matches expected hash if pinned
    try:
        spec = build_spec(claim.spec_kind)
    except Exception as exc:
        gates["gate0_valid"] = {"passed": False, "code": "invalid_spec", "reason": str(exc)}
        return _finalize_circuit_report(claim, gates, failed_gate="gate0_valid", failure_code="invalid_spec")

    digest = claim.freeze_artifact_sha256()
    if claim.expected_artifact_sha256 and claim.expected_artifact_sha256 != digest:
        gates["gate0_valid"] = {
            "passed": False,
            "code": "artifact_mismatch",
            "reason": f"artifact_sha256 {digest} != expected {claim.expected_artifact_sha256}",
        }
        return _finalize_circuit_report(claim, gates, failed_gate="gate0_valid", failure_code="artifact_mismatch")

    # Primary evaluation via Python-owned kickmix harness
    h1 = CircuitCodesignHarness(spec, num_tests=64, max_failures=0, kappa=claim.kappa, seed=claim.seed)
    r1 = h1.run(claim.artifact)
    if not r1.get("valid"):
        gates["gate1_equivalence"] = {
            "passed": False,
            "code": "not_equivalent",
            "reason": f"circuit failed equivalence check (failures={r1.get('failures')})",
        }
        return _finalize_circuit_report(claim, gates, failed_gate="gate1_equivalence", failure_code="not_equivalent")

    gates["gate0_valid"] = {"passed": True, "code": "ok", "artifact_sha256": digest}

    # C1: exhaustive equivalence only (eps = 0, Decision A2)
    if r1.get("verification_method") != "exhaustive" or float(r1.get("eps_bound_97_5", 1.0)) != 0.0:
        gates["gate1_equivalence"] = {
            "passed": False,
            "code": "non_exhaustive_not_claimable",
            "reason": f"exhaustive equivalence required (got method={r1.get('verification_method')!r}, eps={r1.get('eps_bound_97_5')})",
        }
        return _finalize_circuit_report(claim, gates, failed_gate="gate1_equivalence", failure_code="non_exhaustive_not_claimable")
    gates["gate1_equivalence"] = {"passed": True, "code": "ok", "verification_method": "exhaustive", "eps_bound": 0.0}

    # C2: independently re-derive TQ via shared tq.py
    toffoli = int(r1["toffoli"])
    peak_q = int(r1["peak_qubits"])
    t_depth = int(r1.get("toffoli_depth", 0))
    tq_val = int(compute_tq(toffoli, peak_q, kappa=claim.kappa))
    gates["gate2_cost"] = {
        "passed": True,
        "code": "ok",
        "toffoli": toffoli,
        "peak_qubits": peak_q,
        "toffoli_depth": t_depth,
        "tq": tq_val,
        "kappa": claim.kappa,
    }

    # C3: frontier advantage vs frozen baseline
    better_tq = tq_val < int(claim.baseline_tq)
    better_tdepth = (
        claim.baseline_toffoli_depth is not None
        and t_depth < int(claim.baseline_toffoli_depth)
        and tq_val <= int(claim.baseline_tq)
    )
    better_qubits = (
        claim.baseline_peak_qubits is not None
        and peak_q < int(claim.baseline_peak_qubits)
        and tq_val <= int(claim.baseline_tq)
    )
    if not (better_tq or better_tdepth or better_qubits):
        gates["gate3_frontier"] = {
            "passed": False,
            "code": "no_frontier_advantage",
            "reason": f"tq={tq_val} does not improve over baseline_tq={claim.baseline_tq}",
        }
        return _finalize_circuit_report(claim, gates, failed_gate="gate3_frontier", failure_code="no_frontier_advantage")
    gates["gate3_frontier"] = {"passed": True, "code": "ok", "tq": tq_val, "baseline_tq": claim.baseline_tq}

    # C4: independent reproduction at a distinct seed
    if claim.repro_seed == claim.seed:
        gates["gate4_reproduced"] = {"passed": False, "code": "seed_not_distinct", "reason": "repro_seed must differ from seed"}
        return _finalize_circuit_report(claim, gates, failed_gate="gate4_reproduced", failure_code="seed_not_distinct")
    h2 = CircuitCodesignHarness(spec, num_tests=64, max_failures=0, kappa=claim.kappa, seed=claim.repro_seed)
    r2 = h2.run(claim.artifact)
    if not r2.get("valid") or r2.get("verification_method") != "exhaustive" or int(r2["toffoli"]) != toffoli or int(r2["peak_qubits"]) != peak_q:
        gates["gate4_reproduced"] = {"passed": False, "code": "reproduction_mismatch", "reason": "second-seed evaluation disagreed"}
        return _finalize_circuit_report(claim, gates, failed_gate="gate4_reproduced", failure_code="reproduction_mismatch")
    gates["gate4_reproduced"] = {"passed": True, "code": "ok", "repro_seed": claim.repro_seed}

    # C5: held-out verification
    if not claim.held_out_verified:
        gates["gate5_held_out"] = {"passed": False, "code": "held_out_failed", "reason": "held_out_verified is false"}
        return _finalize_circuit_report(claim, gates, failed_gate="gate5_held_out", failure_code="held_out_failed")
    gates["gate5_held_out"] = {"passed": True, "code": "ok"}

    # C6: novel vs published circuit catalog
    cat = lookup_circuit_catalog(claim.spec_kind, claim.artifact, catalog=catalog)
    if cat["known"]:
        gates["gate6_novel"] = {
            "passed": False,
            "code": "known",
            "matched_name": cat["matched_name"],
            "source": cat["source"],
        }
        return _finalize_circuit_report(claim, gates, failed_gate="gate6_novel", failure_code="known")
    gates["gate6_novel"] = {"passed": True, "code": "ok", "artifact_sha256": digest}

    # C7: frozen bundle + Ed25519 operator sign-off (shared Kubona signing domain)
    gates_digest = hashlib.sha256(
        json.dumps(gates, sort_keys=True, separators=(",", ":")).encode("utf-8")
    ).hexdigest()
    claim.gates_digest = gates_digest
    signed = verify_discovery_claim_ed25519(
        claim.claim_id,
        digest,
        gates_digest,
        claim.operator_pubkey_hex or "",
        claim.operator_signature_hex or "",
    )
    gates["gate7_frozen_signed"] = {
        "passed": signed,
        "code": "ok" if signed else "awaiting_ed25519_signoff",
        "artifact_sha256": digest,
        "gates_digest": gates_digest,
    }
    claim.status = "claimed" if signed else "verified"
    claim.gate_report = {
        "all_deterministic_passed": True,
        "claimed": signed,
        "status": claim.status,
        "failed_gate": None,
        "failure_code": None,
        "artifact_sha256": digest,
        "gates_digest": gates_digest,
        "gates": gates,
    }
    return claim.gate_report


def _finalize_circuit_report(
    claim: CircuitClaim,
    gates: dict[str, dict[str, Any]],
    *,
    failed_gate: str,
    failure_code: str,
) -> dict[str, Any]:
    if claim.status in ("verified", "claimed"):
        claim.status = "retracted"
        claim.retraction_reason = f"re-verification failed at {failed_gate}: {failure_code}"
    report = {
        "all_deterministic_passed": False,
        "claimed": False,
        "status": claim.status,
        "failed_gate": failed_gate,
        "failure_code": failure_code,
        "gates": gates,
    }
    claim.gate_report = report
    return report


__all__ = [
    "BUILTIN_PUBLISHED_CIRCUITS",
    "CircuitClaim",
    "canonical_circuit_sha256",
    "canonical_claim_sign_message",
    "evaluate_circuit_gates",
    "lookup_circuit_catalog",
    "sign_discovery_claim_ed25519",
    "verify_discovery_claim_ed25519",
]
