"""Kubona QEC scientific core (Phase 1 — zero-GPL Apache-2.0/MIT stack)."""

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
from kubona.qec.codes import BBCodeSpec, KNOWN_BB_SPECS, build_bb_code, direct_sum_css, relabel_qubits_css
from kubona.qec.decoders import decode_bposd, decode_pymatching, dem_to_matrices, tune_ebposd_weights
from kubona.qec.distance import estimate_distance_bposd, prove_distance_milp
from kubona.qec.evaluator import evaluate_bb_code, evaluate_css_code
from kubona.qec.gf2 import css_logicals, css_parameters, css_validate, gf2_nullspace, gf2_rank, gf2_rref, is_decomposable_css
from kubona.qec.sim import benchmark_css_vs_surface, simulate_memory_circuit, wilson_ci_95

__all__ = [
    "BBCodeSpec",
    "CatalogEntry",
    "CircuitNoiseModel",
    "KNOWN_BB_SPECS",
    "are_tanner_isomorphic",
    "benchmark_css_vs_surface",
    "build_bb_code",
    "build_builtin_catalog",
    "build_css_memory_circuit",
    "build_surface_code_memory_circuit",
    "canonical_fingerprint",
    "css_logicals",
    "css_parameters",
    "css_validate",
    "decode_bposd",
    "decode_pymatching",
    "dem_to_matrices",
    "direct_sum_css",
    "estimate_distance_bposd",
    "evaluate_bb_code",
    "evaluate_css_code",
    "gf2_nullspace",
    "gf2_rank",
    "gf2_rref",
    "heron_calibration_noise",
    "is_decomposable_css",
    "is_lc_equivalent_css",
    "load_qcode_discovery_json",
    "lookup_catalog",
    "prove_distance_milp",
    "relabel_qubits_css",
    "si1000_noise",
    "simulate_memory_circuit",
    "tanner_graph_rx",
    "tune_ebposd_weights",
    "uniform_depolarizing_noise",
    "verify_circuit_distance",
    "wilson_ci_95",
]
