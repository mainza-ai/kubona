"""Zero-GPL Tanner-graph canonicalization and LC-equivalence checks (Gate 3 / G1).

Uses Apache-2.0 ``rustworkx`` (Rust-backed VF2 isomorphism) + permutation-
invariant Weisfeiler-Lehman fingerprints — strictly avoiding GPL ``python-igraph``
and GPLv3 ``pynauty``.
"""

from __future__ import annotations

import hashlib
import json
from typing import Tuple

import numpy as np
import rustworkx as rx

from kubona.qec.gf2 import css_parameters, css_validate


def tanner_graph_rx(hx: np.ndarray, hz: np.ndarray) -> rx.PyGraph:
    """Build a node-colored Tanner graph (0=data, 1=X-check, 2=Z-check)."""
    Hx, Hz = css_validate(hx, hz)
    rx_m, n = Hx.shape
    rz_m, _ = Hz.shape
    g = rx.PyGraph()
    data_nodes = g.add_nodes_from([0] * n)
    x_nodes = g.add_nodes_from([1] * rx_m)
    z_nodes = g.add_nodes_from([2] * rz_m)

    edges = []
    xs_r, xs_c = np.nonzero(Hx)
    for r, c in zip(xs_r.tolist(), xs_c.tolist()):
        edges.append((x_nodes[r], data_nodes[c], None))
    zs_r, zs_c = np.nonzero(Hz)
    for r, c in zip(zs_r.tolist(), zs_c.tolist()):
        edges.append((z_nodes[r], data_nodes[c], None))
    if edges:
        g.add_edges_from(edges)
    return g


def canonical_fingerprint(hx: np.ndarray, hz: np.ndarray) -> str:
    """Permutation-invariant SHA-256 structural fingerprint over (Hx, Hz).

    Combines exact (n, k), check weight multisets, data-qubit (deg_x, deg_z)
    multisets, and 2-round Weisfeiler-Lehman color refinement on the Tanner graph.
    """
    Hx, Hz = css_validate(hx, hz)
    params = css_parameters(Hx, Hz)
    g = tanner_graph_rx(Hx, Hz)

    # 2-round Weisfeiler-Lehman node color refinement (strictly permutation-invariant)
    colors = {idx: str(g[idx]) for idx in g.node_indices()}
    for _ in range(2):
        new_colors = {}
        for idx in g.node_indices():
            nbrs = sorted(colors[nb] for nb in g.neighbors(idx))
            sig = f"{colors[idx]}:{','.join(nbrs)}"
            new_colors[idx] = hashlib.sha256(sig.encode()).hexdigest()[:12]
        colors = new_colors

    wl_hist = sorted(colors.values())
    x_weights = sorted(int(w) for w in Hx.sum(axis=1))
    z_weights = sorted(int(w) for w in Hz.sum(axis=1))
    data_degs = sorted((int(dx), int(dz)) for dx, dz in zip(Hx.sum(axis=0), Hz.sum(axis=0)))

    payload = {
        "n": params["n"],
        "k": params["k"],
        "x_weights": x_weights,
        "z_weights": z_weights,
        "data_degs": data_degs,
        "wl2": wl_hist,
    }
    raw = json.dumps(payload, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(raw).hexdigest()


def are_tanner_isomorphic(
    hx1: np.ndarray,
    hz1: np.ndarray,
    hx2: np.ndarray,
    hz2: np.ndarray,
) -> bool:
    """Exact Tanner-graph isomorphism check via Apache-2.0 rustworkx VF2."""
    if hx1.shape[1] != hx2.shape[1]:
        return False
    if canonical_fingerprint(hx1, hz1) != canonical_fingerprint(hx2, hz2):
        return False
    g1 = tanner_graph_rx(hx1, hz1)
    g2 = tanner_graph_rx(hx2, hz2)
    return bool(rx.is_isomorphic(g1, g2, node_matcher=lambda a, b: a == b))


def is_lc_equivalent_css(
    hx1: np.ndarray,
    hz1: np.ndarray,
    hx2: np.ndarray,
    hz2: np.ndarray,
) -> Tuple[bool, str]:
    """Check CSS local-Clifford equivalence (identity or global Hadamard swap X <-> Z)."""
    if are_tanner_isomorphic(hx1, hz1, hx2, hz2):
        return True, "tanner_isomorphic"
    if are_tanner_isomorphic(hz1, hx1, hx2, hz2):
        return True, "lc_hadamard_swap"
    return False, "distinct"
