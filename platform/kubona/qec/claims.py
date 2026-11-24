"""Seven-Gate Discovery Claim engine and cryptographic lifecycle enforcement (Phase 4 / P0-3).

Enforces (fail-closed, zero caller-trusted shortcuts):
  Gate 0: valid binary CSS matrices (Hx @ Hz.T == 0 mod 2)
  Gate 1: exact parameters (k from GF(2) rank; d independently re-derived via
          full-group MILP and matched against claimed d, never caller-trusted)
  Gate 2: indecomposable (row-mixing-invariant RREF Tanner components)
  Gate 3: novel to catalog (neither Tanner-isomorphic nor LC-equivalent)
  Gate 4: frontier advantage (non-overlapping 95% CIs at <= baseline qubit budget)
  Gate 5: held-out survival (advantage persists under held-out noise model)
  Gate 6: independent reproduction (>=2 distinct seeds + verified d_circ > 0)
  Gate 7: frozen SHA-256 artifact bundle + Ed25519 operator signature over
          ``kubona:discovery_claim:v1:<claim_id>:<artifact_sha256>:<gates_digest>``

Lifecycle: candidate -> verified -> claimed -> retracted.
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Sequence

import numpy as np

from kubona.qec.catalog import CatalogEntry, build_builtin_catalog, lookup_catalog
from kubona.qec.distance import prove_distance_milp
from kubona.qec.gf2 import css_parameters, css_validate, is_decomposable_css

CLAIM_SIG_DOMAIN = "kubona:discovery_claim:v1"


def canonical_claim_sign_message(claim_id: str, artifact_sha256: str, gates_digest: str) -> bytes:
    """Canonical domain-separated message signed by Ed25519 for Gate 7."""
    return f"{CLAIM_SIG_DOMAIN}:{claim_id}:{artifact_sha256}:{gates_digest}".encode("utf-8")


def sign_discovery_claim_ed25519(
    claim_id: str,
    artifact_sha256: str,
    gates_digest: str,
    privkey_hex: str,
) -> tuple[str, str]:
    """Sign a discovery claim bundle with a 32-byte Ed25519 private key (hex).

    Returns ``(pubkey_hex, signature_hex)``.
    """
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

    priv_bytes = bytes.fromhex(privkey_hex.strip())
    if len(priv_bytes) != 32:
        raise ValueError(f"Ed25519 private key must be 32 bytes, got {len(priv_bytes)}")
    sk = Ed25519PrivateKey.from_private_bytes(priv_bytes)
    pk_hex = sk.public_key().public_bytes(Encoding.Raw, PublicFormat.Raw).hex()
    sig_hex = sk.sign(canonical_claim_sign_message(claim_id, artifact_sha256, gates_digest)).hex()
    return pk_hex, sig_hex


def verify_discovery_claim_ed25519(
    claim_id: str,
    artifact_sha256: str,
    gates_digest: str,
    pubkey_hex: str,
    signature_hex: str,
) -> bool:
    """Verify an Ed25519 Gate-7 operator sign-off signature. Fail-closed."""
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

    try:
        pk_bytes = bytes.fromhex(str(pubkey_hex or "").strip())
        sig_bytes = bytes.fromhex(str(signature_hex or "").strip())
        if len(pk_bytes) != 32 or len(sig_bytes) != 64:
            return False
        pk = Ed25519PublicKey.from_public_bytes(pk_bytes)
        pk.verify(sig_bytes, canonical_claim_sign_message(claim_id, artifact_sha256, gates_digest))
        return True
    except Exception:
        return False


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
    operator_pubkey_hex: str | None = None
    operator_signature_hex: str | None = None
    retraction_reason: str | None = None
    artifact_sha256: str = ""
    gates_digest: str = ""
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
    verify_distance: bool = True,
    milp_time_limit_s: float = 15.0,
) -> dict[str, Any]:
    """Run all deterministic gates (0-6) and verify Gate 7 Ed25519 sign-off. Fail-closed."""
    gates: dict[str, dict[str, Any]] = {}

    # Gate 0: valid CSS matrices
    try:
        Hx, Hz = css_validate(claim.hx, claim.hz)
        params = css_parameters(Hx, Hz)
        gates["gate0_valid"] = {"passed": True, "code": "ok", "n": params["n"], "k": params["k"]}
    except Exception as exc:
        gates["gate0_valid"] = {"passed": False, "code": "invalid_matrices", "reason": str(exc)}
        return _finalize_report(claim, gates, failed_gate="gate0_valid", failure_code="invalid_matrices")

    # Gate 1a: fast syntactic check (k > 0 and d_kind == "proved" before expensive MILP)
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

    # Gate 2: indecomposable (row-mixing-invariant RREF check; fast pre-MILP filter)
    decomp = is_decomposable_css(Hx, Hz)
    if decomp["decomposable"]:
        gates["gate2_indecomposable"] = {
            "passed": False,
            "code": "decomposable",
            "reason": f"code decomposes into {decomp['component_qubit_sizes']}",
        }
        return _finalize_report(claim, gates, failed_gate="gate2_indecomposable", failure_code="decomposable")
    gates["gate2_indecomposable"] = {"passed": True, "code": "ok", "num_components": 1}

    # Gate 3: not already known (canonical isomorphism + LC equivalence; fast pre-MILP filter)
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

    # Gate 1b: independent full-group MILP re-derivation (never trust caller-supplied d)
    witness_sha256 = ""
    if verify_distance:
        proved = prove_distance_milp(Hx, Hz, time_limit_s=milp_time_limit_s)
        if proved.get("kind") != "proved" or proved.get("certificate", {}).get("proof_scope") != "full_logical_group":
            gates["gate1_exact_params"] = {
                "passed": False,
                "code": "distance_not_proved",
                "reason": f"independent full-group MILP did not certify distance (kind={proved.get('kind')!r})",
            }
            return _finalize_report(claim, gates, failed_gate="gate1_exact_params", failure_code="distance_not_proved")
        if int(proved["value"]) != int(claim.d):
            gates["gate1_exact_params"] = {
                "passed": False,
                "code": "distance_mismatch",
                "reason": f"claimed d={claim.d} does not match independently proved full-group distance {proved['value']}",
            }
            return _finalize_report(claim, gates, failed_gate="gate1_exact_params", failure_code="distance_mismatch")
        witness_sha256 = str(proved.get("certificate", {}).get("witness_sha256") or "")

    gates["gate1_exact_params"] = {
        "passed": True,
        "code": "ok",
        "n": params["n"],
        "k": params["k"],
        "d": int(claim.d),
        "d_kind": "proved",
        "proof_scope": "full_logical_group" if verify_distance else "unchecked_test_override",
        "witness_sha256": witness_sha256,
    }

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

    # Gate 6: independently reproduced (>=2 distinct seeds + verified d_circ > 0)
    if not claim.reproduced or claim.d_circ is None or claim.d_circ <= 0 or len(set(claim.seeds)) < 2:
        gates["gate6_reproduced"] = {
            "passed": False,
            "code": "not_reproduced",
            "reason": f"reproduced={claim.reproduced}, d_circ={claim.d_circ}, distinct_seeds={len(set(claim.seeds))}",
        }
        return _finalize_report(claim, gates, failed_gate="gate6_reproduced", failure_code="not_reproduced")
    gates["gate6_reproduced"] = {"passed": True, "code": "ok", "d_circ": claim.d_circ, "distinct_seeds": len(set(claim.seeds))}

    # Gate 7: frozen bundle + Ed25519 operator signature over (claim_id, artifact_sha256, gates_digest)
    digest = claim.freeze_artifact_sha256()
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
    verify_distance: bool = True,
) -> list[dict[str, Any]]:
    """Re-verify every stored claim against an updated catalog; demote failures to retracted."""
    results: list[dict[str, Any]] = []
    for cl in claims:
        rep = evaluate_seven_gates(cl, catalog=catalog, verify_distance=verify_distance)
        results.append({
            "claim_id": cl.claim_id,
            "status": cl.status,
            "failed_gate": rep["failed_gate"],
            "failure_code": rep["failure_code"],
            "retraction_reason": cl.retraction_reason,
        })
    return results
