"""Kubona QEC scientific core (Phase 1 — zero-GPL Apache-2.0/MIT stack)."""

from kubona.qec.canon import are_tanner_isomorphic, canonical_fingerprint, is_lc_equivalent_css, tanner_graph_rx
from kubona.qec.catalog import CatalogEntry, build_builtin_catalog, load_qcode_discovery_json, lookup_catalog
from kubona.qec.codes import BBCodeSpec, KNOWN_BB_SPECS, build_bb_code, direct_sum_css, relabel_qubits_css
from kubona.qec.distance import estimate_distance_bposd, prove_distance_milp
from kubona.qec.evaluator import evaluate_bb_code, evaluate_css_code
from kubona.qec.gf2 import css_logicals, css_parameters, css_validate, gf2_nullspace, gf2_rank, gf2_rref, is_decomposable_css

__all__ = [
    "BBCodeSpec",
    "CatalogEntry",
    "KNOWN_BB_SPECS",
    "are_tanner_isomorphic",
    "build_bb_code",
    "build_builtin_catalog",
    "canonical_fingerprint",
    "css_logicals",
    "css_parameters",
    "css_validate",
    "direct_sum_css",
    "estimate_distance_bposd",
    "evaluate_bb_code",
    "evaluate_css_code",
    "gf2_nullspace",
    "gf2_rank",
    "gf2_rref",
    "is_decomposable_css",
    "is_lc_equivalent_css",
    "load_qcode_discovery_json",
    "lookup_catalog",
    "prove_distance_milp",
    "relabel_qubits_css",
    "tanner_graph_rx",
]
