"""Phase 4 Red-Team Planted Fakes CI Suite & Whole-Catalog Re-Verification.

Verifies 100% rejection of all 6 planted fakes with the exact expected gate & code,
plus full lifecycle candidate -> verified -> claimed -> retracted on re-verification.
"""

from __future__ import annotations

import importlib.util

import numpy as np
import pytest

pytest.importorskip("rustworkx")
_REQUIRES_HIGHSPY = pytest.mark.skipif(
    importlib.util.find_spec("highspy") is None,
    reason="highspy not installed (install optional extra: --extra qec)",
)

from kubona.qec import (
    CatalogEntry,
    DiscoveryClaim,
    build_bb_code,
    build_builtin_catalog,
    canonical_fingerprint,
    direct_sum_css,
    evaluate_seven_gates,
    relabel_qubits_css,
    reverify_claims,
    sign_discovery_claim_ed25519,
)


def _steane_H() -> np.ndarray:
    return np.array(
        [
            [1, 0, 1, 0, 1, 0, 1],
            [0, 1, 1, 0, 0, 1, 1],
            [0, 0, 0, 1, 1, 1, 1],
        ],
        dtype=np.uint8,
    )


def _make_claim(hx: np.ndarray, hz: np.ndarray, **overrides) -> DiscoveryClaim:
    defaults = dict(
        claim_id="dc_test",
        code_name="candidate_code",
        hx=hx,
        hz=hz,
        d=3,
        d_kind="proved",
        total_qubits=13,
        baseline_total_qubits=17,
        code_ci_high=0.004,
        baseline_ci_low=0.012,
        held_out_ci_high=0.006,
        held_out_baseline_ci_low=0.015,
        held_out_noise_is_held_out=True,
        reproduced=True,
        d_circ=3,
        reproduction_ler=0.0032,
        reproduction_ci_low=0.0024,
        reproduction_ci_high=0.0041,
        tool_versions={"stim": "1.16.0", "highspy": "1.16.0"},
    )
    defaults.update(overrides)
    return DiscoveryClaim(**defaults)


def test_redteam_fake1_relabelled_gross_code_rejected_gate3():
    """Fake 1: qubit-relabelled Gross code -> Gate 3 (known)."""
    catalog = build_builtin_catalog()
    hx, hz = build_bb_code(12, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    perm = np.random.default_rng(99).permutation(144).tolist()
    hx_r, hz_r = relabel_qubits_css(hx, hz, perm)

    cl = _make_claim(hx_r, hz_r, d=12, total_qubits=288, baseline_total_qubits=288)
    rep = evaluate_seven_gates(cl, catalog=catalog)
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate3_novel"
    assert rep["failure_code"] == "known"


def test_redteam_fake2_lc_hadamard_swapped_code_rejected_gate3():
    """Fake 2: Hadamard-swapped connected asymmetric CSS code -> Gate 3 (lc_equivalent)."""
    hx_orig = np.array([[1, 1, 1, 1]], dtype=np.uint8)
    hz_orig = np.array([[1, 1, 0, 0], [0, 0, 1, 1]], dtype=np.uint8)
    cat = [
        CatalogEntry(
            name="known_asymmetric_css",
            n=4,
            k=1,
            d=2,
            d_kind="proved",
            source="unit-test",
            hx=hx_orig,
            hz=hz_orig,
            fingerprint=canonical_fingerprint(hx_orig, hz_orig),
            swapped_fingerprint=canonical_fingerprint(hz_orig, hx_orig),
        )
    ]
    cl = _make_claim(hz_orig, hx_orig, d=2)
    rep = evaluate_seven_gates(cl, catalog=cat)
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate3_novel"
    assert rep["failure_code"] == "lc_equivalent"


def test_redteam_fake3_row_scrambled_direct_sum_rejected_gate2():
    """Fake 3: row-scrambled Gross (+) Gross ([[288,24,12]]) -> Gate 2 (decomposable)."""
    hx, hz = build_bb_code(12, 6, [(3, 0), (0, 1), (0, 2)], [(0, 3), (1, 0), (2, 0)])
    hx2, hz2 = direct_sum_css(hx, hz, hx, hz)
    # Invertible row mixing across blocks
    for i in range(hx.shape[0]):
        hx2[i] ^= hx2[hx.shape[0] + i]
        hz2[i] ^= hz2[hz.shape[0] + i]
    cl = _make_claim(hx2, hz2, d=12, total_qubits=576, baseline_total_qubits=600)
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate2_indecomposable"
    assert rep["failure_code"] == "decomposable"


def test_redteam_fake4_upper_bound_distance_rejected_gate1():
    """Fake 4: heuristic BP-OSD upper_bound distance -> Gate 1 (upper_bound_not_claimable)."""
    H = _steane_H()
    cl = _make_claim(H, H, d=24, d_kind="upper_bound")
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate1_exact_params"
    assert rep["failure_code"] == "upper_bound_not_claimable"


@_REQUIRES_HIGHSPY
def test_redteam_fake5_held_out_failure_rejected_gate5():
    """Fake 5: wins training noise but loses held-out Heron-r2 noise -> Gate 5 (held_out_failed)."""
    H = _steane_H()
    cl = _make_claim(H, H, held_out_ci_high=0.025, held_out_baseline_ci_low=0.015)
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate5_held_out"
    assert rep["failure_code"] == "held_out_failed"


def test_redteam_fake6_malformed_non_commuting_matrices_rejected_gate0():
    """Fake 6: non-commuting check matrices -> Gate 0 (invalid_matrices)."""
    hx = np.array([[1, 1, 0]], dtype=np.uint8)
    hz = np.array([[1, 0, 1]], dtype=np.uint8)  # Hx @ Hz.T = 1 != 0 mod 2
    cl = _make_claim(hx, hz)
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate0_valid"
    assert rep["failure_code"] == "invalid_matrices"


@_REQUIRES_HIGHSPY
def test_redteam_forged_distance_rejected_gate1():
    """P0-3: caller-forged d=999 with d_kind='proved' is caught by independent MILP re-derivation."""
    H = _steane_H()
    cl = _make_claim(H, H, d=999, d_kind="proved")
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate1_exact_params"
    assert rep["failure_code"] == "distance_mismatch"


@_REQUIRES_HIGHSPY
def test_redteam_n10_steane_and_surface_are_known_in_builtin_catalog():
    """N10: Steane [[7,1,3]] must be recognized as known in build_builtin_catalog(), never novel."""
    from kubona.qec import evaluate_css_code, lookup_catalog

    H = _steane_H()
    cat = lookup_catalog(H, H)
    assert cat["known"] is True
    assert cat["matched_name"] == "steane_7_1_3"

    # check_catalog=False must NOT mark claimable=True
    ev = evaluate_css_code(H, H, name="steane_unchecked", prove_distance=True, check_catalog=False)
    assert ev["catalog_relation"] == "unchecked"
    assert ev["claimable"] is False


@_REQUIRES_HIGHSPY
def test_redteam_gate6_refuses_bare_reproduced_flag_without_measured_ci():
    """N2: reproduced=True without measured second-seed LER/CI is refused at Gate 6."""
    H = _steane_H()
    cl = _make_claim(H, H, reproduced=True, reproduction_ler=None, reproduction_ci_low=None, reproduction_ci_high=None)
    rep = evaluate_seven_gates(cl, catalog=[])
    assert rep["all_deterministic_passed"] is False
    assert rep["failed_gate"] == "gate6_reproduced"
    assert rep["failure_code"] == "not_reproduced"


@_REQUIRES_HIGHSPY
def test_valid_claim_lifecycle_and_automatic_retraction_on_reverify():
    """Valid novel candidate -> verified -> Ed25519-signed claimed -> retracted when added to catalog."""
    H = _steane_H()
    cl = _make_claim(H, H, code_name="steane_7_1_3")

    # Without signoff: reaches verified (never claimed)
    rep1 = evaluate_seven_gates(cl, catalog=[])
    assert rep1["all_deterministic_passed"] is True
    assert cl.status == "verified"
    assert len(cl.artifact_sha256) == 64
    assert len(cl.gates_digest) == 64

    # Bare string signoff without Ed25519 signature MUST be refused (P0-3)
    cl.operator_signoff = "operator-mainza"
    rep_unsigned = evaluate_seven_gates(cl, catalog=[])
    assert rep_unsigned["claimed"] is False
    assert cl.status == "verified"

    # Real Ed25519 signature over (claim_id, artifact_sha256, gates_digest) promotes to claimed
    priv_hex = "11" * 32
    pub_hex, sig_hex = sign_discovery_claim_ed25519(cl.claim_id, cl.artifact_sha256, cl.gates_digest, priv_hex)
    cl.operator_pubkey_hex = pub_hex
    cl.operator_signature_hex = sig_hex
    rep2 = evaluate_seven_gates(cl, catalog=[])
    assert rep2["claimed"] is True
    assert cl.status == "claimed"

    # Whole-catalog re-verification once code enters catalog -> automatically retracted!
    updated_catalog = [
        CatalogEntry(
            name="steane_published",
            n=7,
            k=1,
            d=3,
            d_kind="proved",
            source="Steane 1996",
            hx=H,
            hz=H,
            fingerprint=canonical_fingerprint(H, H),
            swapped_fingerprint=canonical_fingerprint(H, H),
        )
    ]
    rev = reverify_claims([cl], catalog=updated_catalog)
    assert rev[0]["status"] == "retracted"
    assert cl.status == "retracted"
    assert "gate3_novel" in (cl.retraction_reason or "")
