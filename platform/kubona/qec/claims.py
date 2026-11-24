"""Seven-Gate Discovery Claim engine and lifecycle enforcement (Phase 4).

Enforces:
  Gate 0: valid binary CSS matrices (Hx @ Hz.T == 0 mod 2)
  Gate 1: exact parameters (k from GF(2) rank; d PROVED, never upper_bound)
  Gate 2: indecomposable (row-mixing-invariant RREF Tanner components)
  Gate 3: novel to catalog (neither Tanner-isomorphic nor LC-equivalent)
  Gate 4: frontier advantage (non-overlapping 95% CIs at <= baseline qubit budget)
  Gate 5: held-out survival (advantage persists under held-out noise model)
  Gate 6: independent reproduction (second path / seed + verified d_circ > 0)
  Gate 7: frozen SHA-256 artifact bundle + explicit human operator sign-off

Lifecycle: candidate -> verified -> claimed -> retracted.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Sequence

import numpy as np

from kubona.qec.catalog import CatalogEntry, build_builtin_catalog, lookup_catalog
from kubona.qec.gf2 import css_parameters, css_validate, is_decomposable_css


@dataclass
class DiscoveryClaim:
    claim_id: str
    code_name: str
    hx: np.ndarray
    hz: np.ndarray
    d: int | None
    d_kind: str
    total_qubits: int
    baseline_total_qubits: int
    code_ci_high: float
    baseline_ci_low: float
    held_out_ci_high: float
    held_out_baseline_ci_low: float
    held_out_noise_is_held_out: bool
    reproduced: bool
    d_circ: int | None
    seeds: list[int] = field(default_factory=lambda: [7, 11])
    tool_versions: dict[str, str] = field(default_factory=dict)
    status: str = "candidate"  # candidate | verified | claimed | retracted
    operator_signoff: str | None = None
    retraction_reason: str | None = None
    artifact_sha256: str = ""
    gate_report: dict[str, Any] = field(default_factory=dict)

    def freeze_artifact_sha256(self) -> str:
        payload = {
            "claim_id": self.claim_id,
            "code_name": self.code_name,
            "hx": np.asarray(self.hx, dtype=np.uint8).tolist(),
            "hz": np.asarray(self.hz, dtype=np.uint8).tolist(),
            "d": self.d,
            "d_kind": self.d_kind,
            "d_circ": self.d_circ,
            "total_qubits": self.total_qubits,
            "baseline_total_qubits": self.baseline_total_qubits,
            "seeds": list(self.seeds),
            "tool_versions": dict(sorted(self.tool_versions.items())),
        }
        raw = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
        self.artifact_sha256 = hashlib.sha256(raw).hexdigest()
        return self.artifact_sha256


def evaluate_seven_gates(
    claim: DiscoveryClaim,
    *,
    catalog: Sequence[CatalogEntry] | None = None,
) -> dict[str, Any]:
    """Run all deterministic gates (0-6) and check Gate 7 readiness. Fail-closed."""
    gates: dict[str, dict[str, Any]] = {}

    # Gate 0: valid CSS matrices
    try:
        Hx, Hz = css_validate(claim.hx, claim.hz)
        params = css_parameters(Hx, Hz)
        gates["gate0_valid"] = {"passed": True, "code": "ok", "n": params["n"], "k": params["k"]}
    except Exception as exc:
        gates["gate0_valid"] = {"passed": False, "code": "invalid_matrices", "reason": str(exc)}
        return _finalize_report(claim, gates, failed_gate="gate0_valid", failure_code="invalid_matrices")

    # Gate 1: exact parameters (proved distance mandatory)
    if params["k"] <= 0:
        gates["gate1_exact_params"] = {"passed": False, "code": "k_zero", "reason": "k must be > 0"}
        return _finalize_report(claim, gates, failed_gate="gate1_exact_params", failure_code="k_zero")
    if claim.d is None or claim.d <= 0 or claim.d_kind != "proved":
        gates["gate1_exact_params"] = {
            "passed": False,
            "code": "upper_bound_not_claimable",
            "reason": f"distance must be proved (got d={claim.d}, d_kind={claim.d_kind!r})",
        }
        return _finalize_report(claim, gates, failed_gate="gate1_exact_params", failure_code="upper_bound_not_claimable")
    gates["gate1_exact_params"] = {"passed": True, "code": "ok", "n": params["n"], "k": params["k"], "d": claim.d, "d_kind": "proved"}

    # Gate 2: indecomposable (row-mixing-invariant RREF check)
    decomp = is_decomposable_css(Hx, Hz)
    if decomp["decomposable"]:
        gates["gate2_indecomposable"] = {
            "passed": False,
            "code": "decomposable",
            "reason": f"code decomposes into {decomp['component_qubit_sizes']}",
        }
        return _finalize_report(claim, gates, failed_gate="gate2_indecomposable", failure_code="decomposable")
    gates["gate2_indecomposable"] = {"passed": True, "code": "ok", "num_components": 1}

    # Gate 3: not already known (canonical isomorphism + LC equivalence)
    entries = list(catalog) if catalog is not None else build_builtin_catalog()
    cat = lookup_catalog(Hx, Hz, catalog=entries)
    if cat["known"]:
        fail_code = "lc_equivalent" if cat["relation"] == "lc_hadamard_swap" else "known"
        gates["gate3_novel"] = {
            "passed": False,
            "code": fail_code,
            "matched_name": cat["matched_name"],
            "relation": cat["relation"],
        }
        return _finalize_report(claim, gates, failed_gate="gate3_novel", failure_code=fail_code)
    gates["gate3_novel"] = {"passed": True, "code": "ok", "canonical_hash": cat["canonical_hash"]}

    # Gate 4: better on declared metric at fixed qubit budget (non-overlapping 95% CIs)
    if claim.total_qubits > claim.baseline_total_qubits:
        gates["gate4_frontier"] = {
            "passed": False,
            "code": "budget_exceeded",
            "reason": f"total_qubits {claim.total_qubits} > baseline {claim.baseline_total_qubits}",
        }
        return _finalize_report(claim, gates, failed_gate="gate4_frontier", failure_code="budget_exceeded")
    if claim.code_ci_high >= claim.baseline_ci_low:
        gates["gate4_frontier"] = {
            "passed": False,
            "code": "ci_overlap",
            "reason": f"code_ci_high {claim.code_ci_high} >= baseline_ci_low {claim.baseline_ci_low}",
        }
        return _finalize_report(claim, gates, failed_gate="gate4_frontier", failure_code="ci_overlap")
    gates["gate4_frontier"] = {"passed": True, "code": "ok"}

    # Gate 5: survives held-out conditions
    if not claim.held_out_noise_is_held_out or claim.held_out_ci_high >= claim.held_out_baseline_ci_low:
        gates["gate5_held_out"] = {
            "passed": False,
            "code": "held_out_failed",
            "reason": f"held_out_ci_high {claim.held_out_ci_high} >= baseline {claim.held_out_baseline_ci_low}",
        }
        return _finalize_report(claim, gates, failed_gate="gate5_held_out", failure_code="held_out_failed")
    gates["gate5_held_out"] = {"passed": True, "code": "ok"}

    # Gate 6: independently reproduced + circuit-level distance verified
    if not claim.reproduced or claim.d_circ is None or claim.d_circ <= 0:
        gates["gate6_reproduced"] = {
            "passed": False,
            "code": "not_reproduced",
            "reason": f"reproduced={claim.reproduced}, d_circ={claim.d_circ}",
        }
        return _finalize_report(claim, gates, failed_gate="gate6_reproduced", failure_code="not_reproduced")
    gates["gate6_reproduced"] = {"passed": True, "code": "ok", "d_circ": claim.d_circ}

    # Gate 7: frozen bundle + human signoff
    digest = claim.freeze_artifact_sha256()
    signed = bool(claim.operator_signoff and str(claim.operator_signoff).strip())
    gates["gate7_frozen_signed"] = {
        "passed": signed,
        "code": "ok" if signed else "awaiting_signoff",
        "artifact_sha256": digest,
    }
    claim.status = "claimed" if signed else "verified"
    claim.gate_report = {
        "all_deterministic_passed": True,
        "claimed": signed,
        "status": claim.status,
        "failed_gate": None,
        "failure_code": None,
        "artifact_sha256": digest,
        "gates": gates,
    }
    return claim.gate_report


def _finalize_report(
    claim: DiscoveryClaim,
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


def reverify_claims(
    claims: Sequence[DiscoveryClaim],
    *,
    catalog: Sequence[CatalogEntry],
) -> list[dict[str, Any]]:
    """Re-verify every stored claim against an updated catalog; demote failures to retracted."""
    results: list[dict[str, Any]] = []
    for cl in claims:
        rep = evaluate_seven_gates(cl, catalog=catalog)
        results.append({
            "claim_id": cl.claim_id,
            "status": cl.status,
            "failed_gate": rep["failed_gate"],
            "failure_code": rep["failure_code"],
            "retraction_reason": cl.retraction_reason,
        })
    return results
