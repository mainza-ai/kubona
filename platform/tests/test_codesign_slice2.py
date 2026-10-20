"""Slice 2: specs registry, budget union-bound, kappa sweep, families."""

import pytest

from kubona.codesign import (
    BudgetEntry,
    FamilySearch,
    KickmixEvaluator,
    NvidiaProposer,
    build_spec,
    build_truth_table_spec,
    deterministic_mutator,
    kappa_sweep,
    ledger_rows,
    union_bound,
)


def test_named_registry_covers_four_kinds():
    # NOTE: input-swapping tasks are ill-posed under kickmix (inputs must be
    # preserved); the registry covers copy/AND/XOR/phase only by construction.
    for kind in ("and2", "cz-phase", "copy1", "xor2"):
        assert build_spec(kind).num_qubits >= 2
    with pytest.raises(ValueError):
        build_spec("shor")


def test_truth_table_and2_matches_named():
    table = {
        "00": {"out": {"2": 0}},
        "01": {"out": {"2": 0}},
        "10": {"out": {"2": 0}},
        "11": {"out": {"2": 1}},
    }
    spec = build_truth_table_spec(3, [0, 1], [2], table)
    from kubona.codesign import Builder
    b = Builder(3)
    b.ccx(0, 1, 2)
    assert KickmixEvaluator().evaluate(b, spec, num_tests=4, seed=7)["valid"] is True
    # partial table refuses (cannot prove)
    with pytest.raises(ValueError):
        build_truth_table_spec(3, [0, 1], [2], {"00": {"out": {"2": 0}}})
    # n > 8 refuses
    with pytest.raises(ValueError):
        build_truth_table_spec(9, list(range(9)), [8],
                               {format(m, "09b"): {"out": {"8": 0}} for m in range(512)})


def test_budget_union_bound_table_s4_shape():
    entries = [
        BudgetEntry("multiplier-carry", 0.5e-6, 40),
        BudgetEntry("history-stack", 2.4e-6, 2),
    ]
    total = union_bound(entries)
    assert abs(total - (0.5e-6 * 40 + 2.4e-6 * 2)) < 1e-12
    rows = ledger_rows(entries)
    assert rows[-1]["source"] == "union_bound"
    assert rows[-1]["contribution"] == total
    with pytest.raises(ValueError):
        BudgetEntry("bad", float("nan"), 1)
    with pytest.raises(ValueError):
        BudgetEntry("bad", 0.1, -1)
    with pytest.raises(ValueError):
        union_bound([{"not": "entry"}])  # type: ignore[list-item]


def test_kappa_sweep_pins_triple():
    assert kappa_sweep(5, 10) == {1: 50, 2: 500, 3: 5000}
    assert kappa_sweep(1348, 115)[1] == 155020


def test_families_elites_and_nudge():
    fs = FamilySearch(["fermat", "qrom", "tower"])
    assert fs.offer("fermat", "a", 100, 10) is True
    assert fs.offer("fermat", "b", 120, 10) is False  # same log-cell, worse TQ tracked
    fs.offer("qrom", "c", 50, 10)
    elites = fs.elites()
    assert set(elites) >= {"fermat", "qrom"}
    nudge = fs.widen_nudge()
    assert nudge["family"] in ("fermat", "qrom", "tower")
    assert "widen" in nudge["nudge"]
    with pytest.raises(ValueError):
        FamilySearch(["a", "a"])
    with pytest.raises(ValueError):
        fs.offer("nope", "x", 1, 1)


def test_deterministic_mutator_no_network():
    assert deterministic_mutator([], 0) == [("x", (0,), ())]
    assert deterministic_mutator([("x", (0,), ())], 1) == []
    with pytest.raises(ValueError):
        deterministic_mutator("nope", 0)  # type: ignore[arg-type]


def test_nvidia_proposer_parks_others_and_refuses_keyless(monkeypatch):
    with pytest.raises(ValueError, match="parked"):
        NvidiaProposer(provider="omlx")
    with pytest.raises(ValueError, match="got 'anthropic'"):
        NvidiaProposer(provider="anthropic")
    # nvidia stays explicitly usable (parked, not removed) but still
    # requires its key when selected.
    monkeypatch.delenv("NVIDIA_API_KEY", raising=False)
    with pytest.raises(ValueError, match="NVIDIA_API_KEY"):
        NvidiaProposer(provider="nvidia")
    # strata is the dev default: no key required (localhost, optional auth),
    # but an explicit model is still fail-closed.
    monkeypatch.delenv("STRATA_MODEL", raising=False)
    monkeypatch.delenv("STRATA_BASE_URL", raising=False)
    with pytest.raises(ValueError, match="STRATA_MODEL"):
        NvidiaProposer()
    monkeypatch.setenv("STRATA_MODEL", "iq3_s")
    p = NvidiaProposer()
    assert p.provider == "strata" and p.model == "iq3_s"
    assert p.base_url == "http://127.0.0.1:47831/v1"
    monkeypatch.setenv("STRATA_BASE_URL", "http://127.0.0.1:9999/v1")
    assert NvidiaProposer().base_url == "http://127.0.0.1:9999/v1"
    # empty string counts as unset (never silently route nowhere)
    monkeypatch.setenv("STRATA_BASE_URL", "")
    assert NvidiaProposer().base_url == "http://127.0.0.1:47831/v1"
