"""AG-1 benchmark runner: scoring and task-contract tests (no network)."""

from __future__ import annotations

import pytest

from bench_agentic.runner import (
    BenchError,
    extract_path,
    load_tasks,
    resolve_target,
    score_attempt,
    summarize,
)


def test_task_catalog_is_well_formed():
    tasks = load_tasks()
    assert 10 <= len(tasks) <= 20
    ids = {t["id"] for t in tasks}
    for wanted in (
        "h2_vqe_ground_state",
        "h2_classical_anchor",
        "h2_vqe_layers3",
        "h2_vqe_repeats_consistency",
        "h2_excited_state_gap",
        "h2_reaction_energy",
        "tfim_spin_chain_exact",
        "tfim_spin_chain_6",
        "tfim_spin_chain_4",
        "qaoa_maxcut_ratio",
    ):
        assert wanted in ids, wanted
    # Every task carries a hidden target or an explicit internal anchor.
    for task in tasks:
        assert ("target" in task) or ("target_from" in task), task["id"]


def test_extract_path_and_target_resolution():
    result = {"energy": -1.1, "anchor": {"value": -1.137}}
    assert extract_path(result, "energy") == -1.1
    assert extract_path(result, "anchor.value") == -1.137
    assert extract_path(result, "missing") is None
    task = {"id": "t", "target_from": "anchor.value"}
    assert resolve_target(task, result) == -1.137
    assert resolve_target({"id": "t", "target": 2.0}, result) == 2.0


def test_score_pass_fail_missing_and_chemical_accuracy():
    task = {"id": "t", "extract": "energy", "target": -1.137, "tolerance": 0.02,
            "chemical_accuracy": 0.0016}
    ok = score_attempt(task, {"energy": -1.1369})
    assert ok["pass"] and ok["failure"] is None and ok["chemical_accuracy"]
    loose = score_attempt(task, {"energy": -1.12})
    assert loose["pass"] and not loose["chemical_accuracy"]
    fail = score_attempt(task, {"energy": -1.0})
    assert not fail["pass"] and fail["failure"] == "tolerance"
    missing = score_attempt(task, {})
    assert not missing["pass"] and missing["failure"] == "missing_metric"
    no_target = score_attempt({"id": "t", "extract": "energy", "tolerance": 1,
                               "target_from": "exact"}, {"energy": -1})
    assert no_target["failure"] == "missing_target"


def test_summarize_pass_at_k_and_labels():
    attempts = [
        {"pass": False, "failure": "execution", "cost_usd": 0.0, "wall_seconds": 2},
        {"pass": True, "failure": None, "cost_usd": 0.1, "wall_seconds": 4},
    ]
    s = summarize(attempts)
    assert s["runs"] == 2 and s["passes"] == 1
    assert s["pass_at_k"] == 1.0
    assert s["pass_rate"] == 0.5
    assert s["mean_cost_usd"] == pytest.approx(0.05)
    assert s["failure_labels"] == {"execution": 1, "pass": 1}


def test_duplicate_or_incomplete_tasks_refuse(tmp_path):
    import json

    bad = tmp_path / "bad.json"
    bad.write_text(json.dumps({"tasks": [{"id": "a", "kind": "vqe", "spec": {},
                                          "extract": "energy", "tolerance": 1}]}))
    with pytest.raises(BenchError):
        load_tasks(bad)


def test_main_dry_run_and_allow_failures_contract():
    from bench_agentic.runner import main

    assert main(["--dry-run"]) == 0
    assert main(["--dry-run", "--allow-failures"]) == 0
    with pytest.raises(SystemExit):
        main(["--unknown-flag"])
