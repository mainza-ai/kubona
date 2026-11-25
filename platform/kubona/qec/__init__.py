"""Kubona QEC scientific core (Phases 1-6 — zero-GPL Apache-2.0/MIT stack).

Exports are resolved lazily via PEP 562 ``__getattr__`` so importing lightweight
submodules such as ``kubona.qec.claims`` (used by ``kubona.codesign`` in CI jobs
that do not install ``--extra qec``) does not eagerly pull optional dependencies
like ``sinter``, ``stim``, ``pymatching``, or ``rustworkx``.
"""

from __future__ import annotations

import importlib
from typing import Any

_LAZY_EXPORTS: dict[str, tuple[str, str]] = {
    "build_surface_sinter_tasks": ("kubona.qec.batch", "build_surface_sinter_tasks"),
    "run_sinter_batch": ("kubona.qec.batch", "run_sinter_batch"),
    "DEFAULT_HERON_R2_CALIBRATION": ("kubona.qec.campaigns", "DEFAULT_HERON_R2_CALIBRATION"),
    "run_campaign_a_catalog_benchmark": ("kubona.qec.campaigns", "run_campaign_a_catalog_benchmark"),
    "run_campaign_b_schedule_search": ("kubona.qec.campaigns", "run_campaign_b_schedule_search"),
    "are_tanner_isomorphic": ("kubona.qec.canon", "are_tanner_isomorphic"),
    "canonical_fingerprint": ("kubona.qec.canon", "canonical_fingerprint"),
    "is_lc_equivalent_css": ("kubona.qec.canon", "is_lc_equivalent_css"),
    "tanner_graph_rx": ("kubona.qec.canon", "tanner_graph_rx"),
    "CatalogEntry": ("kubona.qec.catalog", "CatalogEntry"),
    "build_builtin_catalog": ("kubona.qec.catalog", "build_builtin_catalog"),
    "load_qcode_discovery_json": ("kubona.qec.catalog", "load_qcode_discovery_json"),
    "lookup_catalog": ("kubona.qec.catalog", "lookup_catalog"),
    "CircuitNoiseModel": ("kubona.qec.circuits", "CircuitNoiseModel"),
    "build_css_memory_circuit": ("kubona.qec.circuits", "build_css_memory_circuit"),
    "build_surface_code_memory_circuit": ("kubona.qec.circuits", "build_surface_code_memory_circuit"),
    "heron_calibration_noise": ("kubona.qec.circuits", "heron_calibration_noise"),
    "load_heron_calibration_from_ledger": ("kubona.qec.circuits", "load_heron_calibration_from_ledger"),
    "si1000_noise": ("kubona.qec.circuits", "si1000_noise"),
    "uniform_depolarizing_noise": ("kubona.qec.circuits", "uniform_depolarizing_noise"),
    "verify_circuit_distance": ("kubona.qec.circuits", "verify_circuit_distance"),
    "DiscoveryClaim": ("kubona.qec.claims", "DiscoveryClaim"),
    "canonical_claim_sign_message": ("kubona.qec.claims", "canonical_claim_sign_message"),
    "evaluate_seven_gates": ("kubona.qec.claims", "evaluate_seven_gates"),
    "reverify_claims": ("kubona.qec.claims", "reverify_claims"),
    "sign_discovery_claim_ed25519": ("kubona.qec.claims", "sign_discovery_claim_ed25519"),
    "verify_discovery_claim_ed25519": ("kubona.qec.claims", "verify_discovery_claim_ed25519"),
    "BBCodeSpec": ("kubona.qec.codes", "BBCodeSpec"),
    "KNOWN_BB_SPECS": ("kubona.qec.codes", "KNOWN_BB_SPECS"),
    "build_bb_code": ("kubona.qec.codes", "build_bb_code"),
    "direct_sum_css": ("kubona.qec.codes", "direct_sum_css"),
    "relabel_qubits_css": ("kubona.qec.codes", "relabel_qubits_css"),
    "decode_bposd": ("kubona.qec.decoders", "decode_bposd"),
    "decode_pymatching": ("kubona.qec.decoders", "decode_pymatching"),
    "dem_to_matrices": ("kubona.qec.decoders", "dem_to_matrices"),
    "tune_ebposd_weights": ("kubona.qec.decoders", "tune_ebposd_weights"),
    "estimate_distance_bposd": ("kubona.qec.distance", "estimate_distance_bposd"),
    "prove_distance_milp": ("kubona.qec.distance", "prove_distance_milp"),
    "evaluate_bb_code": ("kubona.qec.evaluator", "evaluate_bb_code"),
    "evaluate_css_code": ("kubona.qec.evaluator", "evaluate_css_code"),
    "PERMITTED_LEAN_AXIOMS": ("kubona.qec.formal", "PERMITTED_LEAN_AXIOMS"),
    "build_qec_formal_attestation": ("kubona.qec.formal", "build_qec_formal_attestation"),
    "css_logicals": ("kubona.qec.gf2", "css_logicals"),
    "css_parameters": ("kubona.qec.gf2", "css_parameters"),
    "css_validate": ("kubona.qec.gf2", "css_validate"),
    "gf2_nullspace": ("kubona.qec.gf2", "gf2_nullspace"),
    "gf2_rank": ("kubona.qec.gf2", "gf2_rank"),
    "gf2_rref": ("kubona.qec.gf2", "gf2_rref"),
    "is_decomposable_css": ("kubona.qec.gf2", "is_decomposable_css"),
    "S5_MAGIC_STATE_TARGETS": ("kubona.qec.magic_state", "S5_MAGIC_STATE_TARGETS"),
    "compute_gamma": ("kubona.qec.magic_state", "compute_gamma"),
    "hamming_15_1_3_parity_matrix": ("kubona.qec.magic_state", "hamming_15_1_3_parity_matrix"),
    "verify_magic_state_syndromes": ("kubona.qec.magic_state", "verify_magic_state_syndromes"),
    "DiscoveryController": ("kubona.qec.search", "DiscoveryController"),
    "Island": ("kubona.qec.search", "Island"),
    "IslandSearchPool": ("kubona.qec.search", "IslandSearchPool"),
    "MapElitesQecArchive": ("kubona.qec.search", "MapElitesQecArchive"),
    "QecCandidate": ("kubona.qec.search", "QecCandidate"),
    "evaluate_bb_candidate_fast": ("kubona.qec.search", "evaluate_bb_candidate_fast"),
    "mutate_bb_terms": ("kubona.qec.search", "mutate_bb_terms"),
    "verifier_in_loop_refine": ("kubona.qec.search", "verifier_in_loop_refine"),
    "benchmark_css_vs_surface": ("kubona.qec.sim", "benchmark_css_vs_surface"),
    "simulate_memory_circuit": ("kubona.qec.sim", "simulate_memory_circuit"),
    "wilson_ci_95": ("kubona.qec.sim", "wilson_ci_95"),
}

__all__ = sorted(_LAZY_EXPORTS.keys())


def __getattr__(name: str) -> Any:
    target = _LAZY_EXPORTS.get(name)
    if target is None:
        raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
    mod_name, attr_name = target
    value = getattr(importlib.import_module(mod_name), attr_name)
    globals()[name] = value
    return value


def __dir__() -> list[str]:
    return sorted(set(globals().keys()) | set(__all__))
