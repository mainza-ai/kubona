"""Slice 3: DQI bound math + pack registry scaffold."""

import random

import pytest

from kubona.codesign import (
    PACK_ORDER,
    default_registry,
    dqi_loss_bound,
    eps_from_gate,
    prange_advantage_report,
    promote,
    sample_sparse_error,
)


def test_sampler_toy_shape_and_bounds():
    rng = random.Random(7)
    errs = sample_sparse_error(4095, 35, rng)
    assert all(0 <= p < 4095 for p in errs) and len(errs) <= 35
    with pytest.raises(ValueError):
        sample_sparse_error(0, 0, rng)
    with pytest.raises(ValueError):
        sample_sparse_error(10, 11, rng)


def test_gate_conversion_shape():
    # paper: 100/100k -> eps <= 2.434e-3 @97.5% (exact Clopper-Pearson).
    # Ours is a Hoeffding tail — conservative (larger), never tighter than
    # the paper's number, so assert validity shape, not equality.
    eps100 = eps_from_gate(100_000, allowed_failures=100)
    eps0 = eps_from_gate(100_000, allowed_failures=0)
    assert 0 < eps0 < eps100 < 0.02
    assert eps100 > 2.434e-3  # conservative direction pinned (not exact)
    with pytest.raises(ValueError):
        eps_from_gate(0)


def test_eq25_and_prange_report():
    assert dqi_loss_bound(2.434e-3, 0.5763) == pytest.approx(4 * 2.434e-3 * 0.5763)
    rep = prange_advantage_report(0.5763, 0.5009, 2.434e-3)
    assert rep["advantage"] == pytest.approx(0.0754)
    assert 0 < rep["loss_share_of_advantage"] < 0.2  # ~7.4% per paper
    with pytest.raises(ValueError):
        prange_advantage_report(0.4, 0.5, 1e-3)  # no advantage -> refuse
    with pytest.raises(ValueError):
        dqi_loss_bound(float("nan"), 0.5)


def test_pack_registry_scaffold_and_promotion_gate():
    reg = default_registry()
    assert tuple(reg) == PACK_ORDER
    for name, pack in reg.items():
        assert pack.status == "scaffold" and pack.anchor_required is True
        assert len(pack.evidence_reqs) >= 3
    with pytest.raises(ValueError):
        promote(reg["dqi"], "", "proof")  # blank anchor refuses
    active = promote(reg["dqi"], "ledger:anchor", "ledger:proof")
    assert active.status == "active"


def test_divstep_s4_union_matches_paper():
    # Table S4 transcribed verbatim (see divstep_s4_rows docstring for the
    # rate/uses folding caveat): union must equal 1.599e-4 to 3 decimals.
    from kubona.codesign import divstep_s4_rows, ledger_rows, union_bound
    rows = divstep_s4_rows()
    assert len(rows) == 5
    assert abs(union_bound(rows) - 1.599e-4) < 1e-7
    assert ledger_rows(rows)[-1]["source"] == "union_bound"
