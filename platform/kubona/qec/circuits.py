"""Circuit-level syndrome-extraction synthesis and noise models (Phase 2).

Supports three noise models:
1. UniformDepolarizingNoise(p)
2. SI1000Noise(p) (standard superconducting circuit-level benchmark)
3. HeronCalibrationNoiseModel (Kubona differentiator vs. OmniQEC: parameterized
   directly from frozen, ledger-signed IBM Heron r2 calibration snapshots).

Every synthesized circuit can be checked for effective circuit-level distance
d_circ via Stim so hook errors cannot silently invalidate LER rankings (G6).
"""

from __future__ import annotations

import json
import math
import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Tuple

import numpy as np
import stim

from kubona.qec.gf2 import css_logicals, css_validate


@dataclass(frozen=True)
class CircuitNoiseModel:
    name: str
    p_1q: float
    p_2q: float
    p_meas: float
    p_reset: float
    p_idle: float
    calibration_snapshot_id: str | None = None
    is_held_out: bool = False

    def to_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "p_1q": self.p_1q,
            "p_2q": self.p_2q,
            "p_meas": self.p_meas,
            "p_reset": self.p_reset,
            "p_idle": self.p_idle,
            "calibration_snapshot_id": self.calibration_snapshot_id,
            "is_held_out": self.is_held_out,
        }


def uniform_depolarizing_noise(p: float, *, is_held_out: bool = False) -> CircuitNoiseModel:
    if not (0.0 <= p < 0.75):
        raise ValueError(f"physical error rate p must be in [0, 0.75), got {p}")
    return CircuitNoiseModel(
        name=f"uniform_depolarizing_{p:g}",
        p_1q=p,
        p_2q=p,
        p_meas=p,
        p_reset=p,
        p_idle=p,
        is_held_out=is_held_out,
    )


def si1000_noise(p: float, *, is_held_out: bool = False) -> CircuitNoiseModel:
    """Superconducting-inspired SI1000 circuit-level noise model."""
    if not (0.0 <= p < 0.15):
        raise ValueError(f"SI1000 p must be in [0, 0.15), got {p}")
    return CircuitNoiseModel(
        name=f"si1000_{p:g}",
        p_1q=p / 10.0,
        p_2q=p,
        p_meas=min(0.49, 5.0 * p),
        p_reset=min(0.49, 2.0 * p),
        p_idle=p / 10.0,
        is_held_out=is_held_out,
    )


def _extract_snapshot_dict(obj: Any, snapshot_id: str) -> dict[str, Any] | None:
    """Recursively locate the calibration block tagged with ``snapshot_id``."""
    if isinstance(obj, dict):
        sid = obj.get("calibration_snapshot_id") or obj.get("snapshot_id") or obj.get("id")
        if str(sid or "") == snapshot_id and any(
            k in obj for k in ("two_qubit_error", "median_2q_error", "t1_us", "median_t1_us")
        ):
            return dict(obj)
        for v in obj.values():
            found = _extract_snapshot_dict(v, snapshot_id)
            if found is not None:
                return found
    elif isinstance(obj, list):
        for item in obj:
            found = _extract_snapshot_dict(item, snapshot_id)
            if found is not None:
                return found
    return None


def load_heron_calibration_from_ledger(
    snapshot_id: str,
    db_path: str | Path,
) -> dict[str, Any]:
    """Load an exact frozen calibration snapshot from the Kubona SQLite ledger.

    Fail-closed: raises ``ValueError`` if ``snapshot_id`` is empty, the ledger
    does not exist, or the snapshot id is unknown / lacks required metrics.
    """
    sid = str(snapshot_id or "").strip()
    if not sid:
        raise ValueError("calibration snapshot_id must be non-empty")
    path = Path(db_path)
    if not path.is_file():
        raise ValueError(f"calibration ledger not found: {path}")

    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        cur = conn.cursor()
        # Check calibration_snapshots table if present
        tables = {row[0] for row in cur.execute("SELECT name FROM sqlite_master WHERE type='table'")}
        if "calibration_snapshots" in tables:
            cols = [c[1] for c in cur.execute("PRAGMA table_info(calibration_snapshots)")]
            if "id" in cols and "payload_json" in cols:
                row = cur.execute("SELECT payload_json FROM calibration_snapshots WHERE id = ? LIMIT 1", (sid,)).fetchone()
                if row and row[0]:
                    parsed = json.loads(row[0])
                    if isinstance(parsed, dict):
                        parsed.setdefault("calibration_snapshot_id", sid)
                        return parsed
        if "jobs" in tables:
            for (raw_json,) in cur.execute(
                "SELECT result_json FROM jobs WHERE result_json LIKE ? ORDER BY created_at DESC LIMIT 20",
                (f"%{sid}%",),
            ):
                if not raw_json:
                    continue
                try:
                    parsed = json.loads(raw_json)
                except Exception:
                    continue
                found = _extract_snapshot_dict(parsed, sid)
                if found is not None:
                    found["calibration_snapshot_id"] = sid
                    return found
    finally:
        conn.close()
    raise ValueError(f"unknown calibration snapshot {sid!r} in ledger {path}")


def _require_pos_float(cal: dict[str, Any], keys: tuple[str, ...], label: str) -> float:
    for k in keys:
        v = cal.get(k)
        if v is not None:
            fv = float(v)
            if not math.isfinite(fv) or fv <= 0.0:
                raise ValueError(f"calibration field {label} ({k}) must be finite and > 0, got {v!r}")
            return fv
    raise ValueError(f"calibration snapshot missing required field {label} (tried {keys})")


def heron_calibration_noise(
    cal_snapshot: dict[str, Any],
    *,
    cycle_us: float = 1.0,
    is_held_out: bool = True,
) -> CircuitNoiseModel:
    """Build a realistic circuit-level noise model from a Kubona IBM Heron r2
    calibration snapshot (``cal_<sha256>``, G7 differentiator vs. OmniQEC).

    Fail-closed: refuses snapshots missing ``calibration_snapshot_id`` or any
    required physical metric (2Q error, readout error, T1, T2) rather than
    substituting hardcoded defaults.
    """
    if not isinstance(cal_snapshot, dict):
        raise ValueError("cal_snapshot must be a dict")
    snap_id = str(cal_snapshot.get("calibration_snapshot_id") or cal_snapshot.get("snapshot_id") or "").strip()
    if not snap_id or snap_id == "cal_unknown":
        raise ValueError("calibration snapshot requires a valid calibration_snapshot_id")

    p_2q = _require_pos_float(cal_snapshot, ("two_qubit_error", "median_2q_error"), "two_qubit_error")
    p_meas = _require_pos_float(cal_snapshot, ("readout_error", "median_readout_error"), "readout_error")
    t1_us = _require_pos_float(cal_snapshot, ("t1_us", "median_t1_us"), "t1_us")
    t2_us = _require_pos_float(cal_snapshot, ("t2_us", "median_t2_us"), "t2_us")
    raw_1q = cal_snapshot.get("single_qubit_error")
    if raw_1q is None:
        raw_1q = cal_snapshot.get("median_1q_error")
    p_1q = float(raw_1q) if raw_1q is not None else (p_2q / 10.0)
    if not math.isfinite(p_1q) or p_1q < 0.0:
        raise ValueError(f"invalid single_qubit_error: {raw_1q!r}")

    t_eff = max(1.0, min(t1_us, t2_us))
    p_idle = min(0.25, max(1e-6, 1.0 - math.exp(-float(cycle_us) / t_eff)))
    backend = str(cal_snapshot.get("backend") or "heron_r2")
    return CircuitNoiseModel(
        name=f"heron_cal_{backend}_{snap_id[:16]}",
        p_1q=min(0.25, max(0.0, p_1q)),
        p_2q=min(0.25, max(0.0, p_2q)),
        p_meas=min(0.49, max(0.0, p_meas)),
        p_reset=min(0.49, max(0.0, p_meas)),
        p_idle=p_idle,
        calibration_snapshot_id=snap_id,
        is_held_out=is_held_out,
    )


def _greedy_edge_color(edges: list[Tuple[int, int]]) -> list[list[Tuple[int, int]]]:
    """Partition two-qubit edges into collision-free parallel layers."""
    layers: list[list[Tuple[int, int]]] = []
    used_per_layer: list[set[int]] = []
    for u, v in edges:
        placed = False
        for layer, used in zip(layers, used_per_layer):
            if u not in used and v not in used:
                layer.append((u, v))
                used.add(u)
                used.add(v)
                placed = True
                break
        if not placed:
            layers.append([(u, v)])
            used_per_layer.append({u, v})
    return layers


def build_css_memory_circuit(
    hx: np.ndarray,
    hz: np.ndarray,
    *,
    rounds: int = 3,
    noise: CircuitNoiseModel | None = None,
    x_edges: list[tuple[int, int]] | None = None,
    z_edges: list[tuple[int, int]] | None = None,
) -> Tuple[stim.Circuit, dict[str, Any]]:
    """Build a noisy Stim Z-memory circuit for any CSS code (Hx, Hz).

    Qubit layout:
      - Data qubits: 0 .. n-1
      - X-check ancillas: n .. n + rx - 1
      - Z-check ancillas: n + rx .. n + rx + rz - 1

    Optional ``x_edges`` / ``z_edges`` override the default CNOT edge orderings
    while keeping identical gate/idle/measurement/reset noise physics.
    Returns (stim.Circuit, schedule_metadata).
    """
    Hx, Hz = css_validate(hx, hz)
    if rounds < 1:
        raise ValueError(f"rounds must be >= 1, got {rounds}")
    nm = noise or uniform_depolarizing_noise(0.001)
    _, lz = css_logicals(Hx, Hz)
    k = int(lz.shape[0])
    rx_m, n = Hx.shape
    rz_m, _ = Hz.shape
    total_qubits = n + rx_m + rz_m
    x_anc = list(range(n, n + rx_m))
    z_anc = list(range(n + rx_m, total_qubits))
    all_qubits = list(range(total_qubits))
    data_qubits = list(range(n))

    # Build collision-free parallel CNOT layers for X checks (anc -> data) and Z checks (data -> anc)
    xe = list(x_edges) if x_edges is not None else [(x_anc[r], int(c)) for r in range(rx_m) for c in np.flatnonzero(Hx[r]).tolist()]
    ze = list(z_edges) if z_edges is not None else [(int(c), z_anc[r]) for r in range(rz_m) for c in np.flatnonzero(Hz[r]).tolist()]
    x_layers = _greedy_edge_color(xe)
    z_layers = _greedy_edge_color(ze)

    c = stim.Circuit()
    c.append("R", all_qubits)
    if nm.p_reset > 0:
        c.append("X_ERROR", all_qubits, nm.p_reset)

    meas_per_round = rx_m + rz_m
    for r_idx in range(rounds):
        c.append("TICK")
        if x_anc:
            c.append("H", x_anc)
            if nm.p_1q > 0:
                c.append("DEPOLARIZE1", x_anc, nm.p_1q)
        for layer in x_layers:
            flat = [q for pair in layer for q in pair]
            c.append("CX", flat)
            if nm.p_2q > 0:
                c.append("DEPOLARIZE2", flat, nm.p_2q)
            c.append("TICK")
        for layer in z_layers:
            flat = [q for pair in layer for q in pair]
            c.append("CX", flat)
            if nm.p_2q > 0:
                c.append("DEPOLARIZE2", flat, nm.p_2q)
            c.append("TICK")
        if x_anc:
            c.append("H", x_anc)
            if nm.p_1q > 0:
                c.append("DEPOLARIZE1", x_anc, nm.p_1q)
        if nm.p_idle > 0:
            c.append("DEPOLARIZE1", data_qubits, nm.p_idle)
        anc_all = x_anc + z_anc
        if nm.p_meas > 0:
            c.append("X_ERROR", anc_all, nm.p_meas)
        c.append("MR", anc_all)
        if nm.p_reset > 0:
            c.append("X_ERROR", anc_all, nm.p_reset)

        # Detectors: Z-checks deterministic on round 0 (data initialized in |0>);
        # all checks compare against previous round for r_idx > 0.
        if r_idx == 0:
            for zi in range(rz_m):
                rec_idx = -(rz_m - zi)
                c.append("DETECTOR", [stim.target_rec(rec_idx)], [zi, 0])
        else:
            for xi in range(rx_m):
                cur = -(meas_per_round - xi)
                prev = cur - meas_per_round
                c.append("DETECTOR", [stim.target_rec(cur), stim.target_rec(prev)], [xi, r_idx])
            for zi in range(rz_m):
                cur = -(rz_m - zi)
                prev = cur - meas_per_round
                c.append("DETECTOR", [stim.target_rec(cur), stim.target_rec(prev)], [rx_m + zi, r_idx])

    # Final Z-basis data measurement
    if nm.p_meas > 0:
        c.append("X_ERROR", data_qubits, nm.p_meas)
    c.append("M", data_qubits)

    # Final Z-stabilizer detectors tying data measurements to last Z-ancilla round
    for zi in range(rz_m):
        supp = np.flatnonzero(Hz[zi]).tolist()
        targets = [stim.target_rec(-(n - q)) for q in supp]
        targets.append(stim.target_rec(-(n + (rz_m - zi))))
        c.append("DETECTOR", targets, [rx_m + zi, rounds])

    # Logical Z observables
    for li in range(k):
        supp = np.flatnonzero(lz[li]).tolist()
        targets = [stim.target_rec(-(n - q)) for q in supp]
        c.append("OBSERVABLE_INCLUDE", targets, li)

    meta = {
        "n_data": n,
        "n_x_ancilla": rx_m,
        "n_z_ancilla": rz_m,
        "total_qubits": total_qubits,
        "k": k,
        "rounds": rounds,
        "cnot_layers_per_round": len(x_layers) + len(z_layers),
        "noise": nm.to_dict(),
    }
    return c, meta


def build_surface_code_memory_circuit(
    distance: int,
    *,
    rounds: int | None = None,
    noise: CircuitNoiseModel | None = None,
) -> Tuple[stim.Circuit, dict[str, Any]]:
    """Build a rotated surface code Z-memory circuit via Stim as the fixed-budget baseline."""
    if distance < 3 or distance % 2 == 0:
        raise ValueError(f"surface code distance must be an odd integer >= 3, got {distance}")
    r = rounds if rounds is not None else distance
    nm = noise or uniform_depolarizing_noise(0.001)
    c = stim.Circuit.generated(
        "surface_code:rotated_memory_z",
        distance=distance,
        rounds=r,
        after_clifford_depolarization=nm.p_2q,
        before_round_data_depolarization=nm.p_idle,
        before_measure_flip_probability=nm.p_meas,
        after_reset_flip_probability=nm.p_reset,
    )
    total_qubits = c.num_qubits
    meta = {
        "family": "rotated_surface_code",
        "distance": distance,
        "k": 1,
        "total_qubits": total_qubits,
        "rounds": r,
        "noise": nm.to_dict(),
    }
    return c, meta


def verify_circuit_distance(circuit: stim.Circuit) -> dict[str, Any]:
    """Verify effective circuit-level distance d_circ via Stim DEM search (G6)."""
    try:
        err = circuit.shortest_graphlike_error()
        return {"d_circ": len(err), "method": "stim_shortest_graphlike_error", "verified": True}
    except Exception:
        pass
    try:
        errs = circuit.search_for_undetectable_logical_errors(
            dont_explore_detection_event_sets_with_size_above=4,
            dont_explore_edges_with_degree_above=999,
            dont_explore_edges_increasing_symptom_degree=False,
            canonicalize_circuit_errors=True,
        )
        return {"d_circ": len(errs), "method": "stim_undetectable_logical_search", "verified": True}
    except Exception as exc:
        return {"d_circ": None, "method": "unavailable", "verified": False, "reason": str(exc)}
