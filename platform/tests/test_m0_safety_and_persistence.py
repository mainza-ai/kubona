"""Milestone M0 regression tests for Python QEC noise parity, codesign atomic persistence,
calibration_snapshots lookup, and DiscoveryCampaign best-is-None safety.
"""

from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from unittest.mock import patch

import numpy as np
import pytest


def test_codesign_durable_state_outside_tmp_and_atomic_roundtrip(tmp_path, monkeypatch):
    from bridge.codesign_ops import _durable_state_root, run_op

    monkeypatch.delenv("KUBONA_STATE_DIR", raising=False)
    default_root = _durable_state_root()
    assert not str(default_root).startswith("/tmp"), f"default state dir must not live in /tmp: {default_root}"

    monkeypatch.setenv("KUBONA_STATE_DIR", str(tmp_path))
    and2 = [["ccx", [0, 1, 2], []]]
    res1 = run_op("families", {
        "spec_kind": "and2",
        "families": ["a", "b"],
        "campaign_id": "m0_persist_test",
        "offers": [{"family": "a", "artifact": and2}],
    })
    assert res1["ok"] is True and res1["persisted"] is True
    state_file = tmp_path / "codesign_islands" / "m0_persist_test.json"
    assert state_file.is_file()
    # Second call reloads persisted elites without re-offering
    res2 = run_op("families", {
        "spec_kind": "and2",
        "families": ["a", "b"],
        "campaign_id": "m0_persist_test",
        "offers": [],
    })
    assert res2["ok"] is True
    assert res2["elites"]["a"] is not None


def test_campaign_b_circuit_matches_circuits_noise_physics():
    pytest.importorskip("stim")
    from kubona.qec.campaigns import _build_circuit_from_edge_order
    from kubona.qec.circuits import build_css_memory_circuit, uniform_depolarizing_noise

    H = np.array(
        [[1, 0, 1, 0, 1, 0, 1], [0, 1, 1, 0, 0, 1, 1], [0, 0, 0, 1, 1, 1, 1]],
        dtype=np.uint8,
    )
    nm = uniform_depolarizing_noise(0.002)
    c_ref, meta = build_css_memory_circuit(H, H, rounds=2, noise=nm)
    n = H.shape[1]
    rx_m = H.shape[0]
    rz_m = H.shape[0]
    x_anc = list(range(n, n + rx_m))
    z_anc = list(range(n + rx_m, n + rx_m + rz_m))
    x_edges = [(x_anc[r], int(c)) for r in range(rx_m) for c in np.flatnonzero(H[r]).tolist()]
    z_edges = [(int(c), z_anc[r]) for r in range(rz_m) for c in np.flatnonzero(H[r]).tolist()]
    c_sched, depth = _build_circuit_from_edge_order(H, H, x_edges, z_edges, rounds=2, noise=nm)

    assert depth == meta["cnot_layers_per_round"]
    assert str(c_sched) == str(c_ref), "Campaign B circuit noise must match circuits.py byte-for-byte"


def test_calibration_snapshots_table_direct_lookup(tmp_path):
    from kubona.qec.circuits import load_heron_calibration_from_ledger

    db = tmp_path / "kubona_m0.db"
    conn = sqlite3.connect(db)
    conn.execute(
        """CREATE TABLE calibration_snapshots (
            id TEXT PRIMARY KEY,
            backend TEXT NOT NULL DEFAULT '',
            payload_json TEXT NOT NULL,
            created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
        )"""
    )
    payload = {"backend": "ibm_fez", "median_cz_error": 0.0024, "median_readout_error": 0.011}
    conn.execute(
        "INSERT INTO calibration_snapshots (id, backend, payload_json) VALUES (?, ?, ?)",
        ("cal_m0_snap_1", "ibm_fez", json.dumps(payload)),
    )
    conn.commit()
    conn.close()

    loaded = load_heron_calibration_from_ledger("cal_m0_snap_1", db)
    assert loaded["calibration_snapshot_id"] == "cal_m0_snap_1"
    assert loaded["median_cz_error"] == pytest.approx(0.0024)


def test_discovery_campaign_survives_best_is_none(tmp_path):
    from kubona.autoresearch.base import Orchestrator
    from kubona.discovery.campaign import DiscoveryCampaign
    from kubona.provenance import Ledger

    ledger = Ledger(path=tmp_path / "ledger.jsonl")
    camp = DiscoveryCampaign(
        problem="H2",
        hamiltonian="H2",
        ground_state=-1.13727,
        budget_steps=1,
        tenant="default",
    )
    with patch.object(Orchestrator, "best", return_value=None), patch(
        "kubona.discovery.campaign.ground_literature",
        return_value={"source": "arxiv", "items": [], "citations": [], "offline": True, "ledger_ref": "mock_lit_ref"},
    ):
        out = camp.run(ledger=ledger, shots=64)
    assert out["stages"]["experiment"]["best_metric"] is None
    assert out["stages"]["experiment"]["best_theta"] is None
