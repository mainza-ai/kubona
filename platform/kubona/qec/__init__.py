"""Kubona QEC scientific core (Phases 1-6 — zero-GPL Apache-2.0/MIT stack)."""

from kubona.qec.batch import build_surface_sinter_tasks, run_sinter_batch
from kubona.qec.campaigns import (
    DEFAULT_HERON_R2_CALIBRATION,
    run_campaign_a_catalog_benchmark,
    run_campaign_b_schedule_search,
)
from kubona.qec.canon import are_tanner_isomorphic, canonical_fingerprint, is_lc_equivalent_css, tanner_graph_rx
from kubona.qec.catalog import CatalogEntry, build_builtin_catalog, load_qcode_discovery_json, lookup_catalog
from kubona.qec.circuits import (
    CircuitNoiseModel,
    build_css_memory_circuit,
    build_surface_code_memory_circuit,
    heron_calibration_noise,
    si1000_noise,
    uniform_depolarizing_noise,
    verify_circuit_distance,
)
from kubona.qec.claims import DiscoveryClaim, evaluate_seven_gates, reverify_claims
from kubona.qec.codes import BBCodeSpec, KNOWN_BB_SPECS, build_bb_code, direct_sum_css, relabel_qubits_css
from kubona.qec.decoders import decode_bposd, decode_pymatching, dem_to_matrices, tune_ebposd_weights
from kubona.qec.distance import estimate_distance_bposd, prove_distance_milp
from kubona.qec.evaluator import evaluate_bb_code, evaluate_css_code
from kubona.qec.formal import PERMITTED_LEAN_AXIOMS, build_qec_formal_attestation
from kubona.qec.gf2 import css_logicals, css_parameters, css_validate, gf2_nullspace, gf2_rank, gf2_rref, is_decomposable_css
from kubona.qec.magic_state import (
    S5_MAGIC_STATE_TARGETS,
    compute_gamma,
    hamming_15_1_3_parity_matrix,
    verify_magic_state_syndromes,
)
from kubona.qec.search import (
    DiscoveryController,
    Island,
    IslandSearchPool,
    MapElitesQecArchive,
    QecCandidate,
    evaluate_bb_candidate_fast,
    mutate_bb_terms,
    verifier_in_loop_refine,
)
from kubona.qec.sim import benchmark_css_vs_surface, simulate_memory_circuit, wilson_ci_95

__all__ = [
    "BBCodeSpec",
    "CatalogEntry",
    "CircuitNoiseModel",
    "DEFAULT_HERON_R2_CALIBRATION",
    "DiscoveryClaim",
    "DiscoveryController",
    "Island",
    "IslandSearchPool",
    "KNOWN_BB_SPECS",
    "MapElitesQecArchive",
    "PERMITTED_LEAN_AXIOMS",
    "QecCandidate",
    "S5_MAGIC_STATE_TARGETS",
    "are_tanner_isomorphic",
    "benchmark_css_vs_surface",
    "build_bb_code",
    "build_builtin_catalog",
    "build_css_memory_circuit",
    "build_qec_formal_attestation",
    "build_surface_code_memory_circuit",
    "build_surface_sinter_tasks",
    "canonical_fingerprint",
    "compute_gamma",
    "css_logicals",
    "css_parameters",
    "css_validate",
    "decode_bposd",
    "decode_pymatching",
    "dem_to_matrices",
    "direct_sum_css",
    "estimate_distance_bposd",
    "evaluate_bb_candidate_fast",
    "evaluate_bb_code",
    "evaluate_css_code",
    "evaluate_seven_gates",
    "gf2_nullspace",
    "gf2_rank",
    "gf2_rref",
    "hamming_15_1_3_parity_matrix",
    "heron_calibration_noise",
    "is_decomposable_css",
    "is_lc_equivalent_css",
    "load_qcode_discovery_json",
    "lookup_catalog",
    "mutate_bb_terms",
    "prove_distance_milp",
    "relabel_qubits_css",
    "reverify_claims",
    "run_campaign_a_catalog_benchmark",
    "run_campaign_b_schedule_search",
    "run_sinter_batch",
    "si1000_noise",
    "simulate_memory_circuit",
    "tanner_graph_rx",
    "tune_ebposd_weights",
    "uniform_depolarizing_noise",
    "verifier_in_loop_refine",
    "verify_circuit_distance",
    "verify_magic_state_syndromes",
    "wilson_ci_95",
]
