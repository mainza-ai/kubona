"""Unified QEC code evaluator emitting Kubona experiment-result-v1 envelopes."""

from __future__ import annotations

from typing import Any, Sequence, Tuple

import numpy as np

from kubona.qec.catalog import CatalogEntry, lookup_catalog
from kubona.qec.codes import build_bb_code
from kubona.qec.distance import estimate_distance_bposd, prove_distance_milp
from kubona.qec.gf2 import css_parameters, css_validate, is_decomposable_css


def evaluate_css_code(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    name: str = "custom_css",
    prove_distance: bool = True,
    time_limit_s: float = 20.0,
    bposd_trials: int = 32,
    check_catalog: bool = True,
    catalog: Sequence[CatalogEntry] | None = None,
) -> dict[str, Any]:
    """Evaluate a CSS code through exact GF(2) rank, decomposability, distance, and catalog lookup."""
    Hx, Hz = css_validate(hx, hz)
    params = css_parameters(Hx, Hz)
    n, k = params["n"], params["k"]
    decomp = is_decomposable_css(Hx, Hz)

    if k == 0:
        dist_res: dict[str, Any] = {
            "value": None,
            "dx": None,
            "dz": None,
            "kind": "upper_bound",
            "method": "none_k_zero",
            "certificate": {"k": 0},
        }
    elif prove_distance:
        dist_res = prove_distance_milp(Hx, Hz, time_limit_s=time_limit_s)
    else:
        dist_res = estimate_distance_bposd(Hx, Hz, num_trials=bposd_trials)

    d_val = dist_res.get("value")
    d_kind = str(dist_res.get("kind") or "upper_bound")
    fom = round((k * (d_val ** 2)) / n, 6) if (d_val is not None and n > 0) else 0.0

    cat_res = (
        lookup_catalog(Hx, Hz, catalog=catalog)
        if check_catalog
        else {"known": False, "matched_name": None, "relation": "unchecked", "canonical_hash": ""}
    )

    return {
        "result_version": "experiment-result-v1",
        "success": True,
        "kind": "qec_code_eval",
        "code_name": name,
        "n": n,
        "k": k,
        "d": d_val,
        "dx": dist_res.get("dx"),
        "dz": dist_res.get("dz"),
        "d_kind": d_kind,
        "distance_method": dist_res.get("method"),
        "fom": fom,
        "rank_x": params["rank_x"],
        "rank_z": params["rank_z"],
        "decomposable": bool(decomp["decomposable"]),
        "num_components": int(decomp["num_components"]),
        "component_qubit_sizes": decomp["component_qubit_sizes"],
        "canonical_hash": cat_res["canonical_hash"],
        "catalog_known": bool(cat_res["known"]),
        "catalog_match": cat_res["matched_name"],
        "catalog_relation": cat_res["relation"],
        "certificate": dist_res.get("certificate", {}),
        "measured": True,
        "hardware": False,
        "claimable": bool(
            d_kind == "proved"
            and not decomp["decomposable"]
            and not cat_res["known"]
            and cat_res["relation"] == "novel_to_catalog"
            and bool(cat_res["canonical_hash"])
        ),
    }


def evaluate_bb_code(
    ell: int,
    m: int,
    a_terms: Sequence[Tuple[int, int]],
    b_terms: Sequence[Tuple[int, int]],
    *,
    name: str | None = None,
    prove_distance: bool = True,
    time_limit_s: float = 20.0,
    bposd_trials: int = 32,
    check_catalog: bool = True,
    catalog: Sequence[CatalogEntry] | None = None,
) -> dict[str, Any]:
    """Construct and evaluate a Bivariate Bicycle code."""
    hx, hz = build_bb_code(ell, m, a_terms, b_terms)
    code_name = name or f"BB({ell},{m})"
    res = evaluate_css_code(
        hx,
        hz,
        name=code_name,
        prove_distance=prove_distance,
        time_limit_s=time_limit_s,
        bposd_trials=bposd_trials,
        check_catalog=check_catalog,
        catalog=catalog,
    )
    res["bb_params"] = {
        "ell": int(ell),
        "m": int(m),
        "a_terms": [[int(a), int(b)] for a, b in a_terms],
        "b_terms": [[int(a), int(b)] for a, b in b_terms],
    }
    return res
