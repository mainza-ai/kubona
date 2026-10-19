"""Slice 28: tower inverter circuits (plain + MBU-packed).

Production bar: both modes prove exhaustive (plain, 16 trajectories) or
sampled with bound (MBU, records force sampling); the family table pins
all six measured numbers with no winner declared beyond the counts.
"""

import pytest

from kubona.codesign import (
    KickmixEvaluator,
    build_tower_inverter,
    family_comparison,
    to_tower,
    tower_change_matrices,
    tower_inv_spec,
)


def test_tower_change_matrices_match_to_tower():
    fwd, inv = tower_change_matrices()
    for x in range(16):
        bits = [(x >> i) & 1 for i in range(4)]
        c = [sum(r[i] * bits[i] for i in range(4)) & 1 for r in fwd]
        assert (c[0] | (c[1] << 1), c[2] | (c[3] << 1)) == to_tower(x)
    with pytest.raises(ValueError):
        build_tower_inverter("magic")  # mode refuses


def test_tower_circuits_prove():
    (b, regs) = build_tower_inverter("plain")
    assert regs["ccx"] == 60
    res = KickmixEvaluator().evaluate(b, tower_inv_spec(), num_tests=32, seed=7)
    assert res["valid"] is True and res["tests"] == 16  # exhaustive, no hmr
    (bm, regm) = build_tower_inverter("mbu")
    assert regm["ccx"] == 30  # packed: exactly half the plain count
    res = KickmixEvaluator().evaluate(bm, tower_inv_spec(), num_tests=256, seed=7)
    assert res["valid"] is True and res["failures"] == 0
    assert res["eps_bound_97_5"] < 0.02  # sampled: bound reported, not hidden


def test_family_comparison_pins_six_numbers():
    fam = family_comparison()
    assert fam == {"fermat_naive": 88, "fermat_mbu": 48, "fermat_kara": 27,
                   "qrom_full_lookup": 204, "tower_plain": 60, "tower_mbu": 30}
