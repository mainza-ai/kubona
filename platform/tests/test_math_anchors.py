"""Tests for Mathematical Physics analytical anchors and domain pack integration."""

import pytest
from pathlib import Path
from kubona.anchors.math_physics import MathPhysicsAnchorEngine, MathPhysicsAnchor
from kubona.science.domain_pack import DomainPackLoader


def test_math_physics_anchors_available():
    benchmarks = MathPhysicsAnchorEngine.list_benchmarks()
    assert "Heisenberg_Ferromagnet_3D" in benchmarks
    assert "SK_Spin_Glass_Ground_State" in benchmarks
    assert "Haldane_Spin_1_Chain" in benchmarks
    assert "MUB_Dimension_6_Overlap" in benchmarks
    assert "Laughlin_Fractional_Hall_Gap" in benchmarks


def test_heisenberg_anchor_comparison():
    anchor = MathPhysicsAnchorEngine.get_anchor("Heisenberg_Ferromagnet_3D")
    assert anchor is not None
    assert anchor.family_id == 271
    assert anchor.analytical_value == -1.5

    # Simulate a VQE run that achieves energy -1.498 J (above the true ground state -1.5)
    comp = MathPhysicsAnchorEngine.compare_vqe_result("Heisenberg_Ferromagnet_3D", -1.498)
    assert comp["variational_principle_satisfied"]
    assert comp["difference"] > 0
    assert comp["relative_error"] < 0.01


def test_sk_spin_glass_anchor_comparison():
    anchor = MathPhysicsAnchorEngine.get_anchor("SK_Spin_Glass_Ground_State")
    assert anchor is not None
    assert anchor.family_id == 281
    assert abs(anchor.analytical_value - (-0.763166)) < 1e-5

    # QAOA approximation above the ground state
    comp = MathPhysicsAnchorEngine.compare_vqe_result("SK_Spin_Glass_Ground_State", -0.750)
    assert comp["variational_principle_satisfied"]
    assert comp["relative_error"] < 0.05


def test_domain_pack_manifest_registration(tmp_path):
    loader = DomainPackLoader(packs_dir=tmp_path / "packs")
    packs = loader.register_default_packs()
    pack_names = [p.name for p in packs]

    assert "quantum-molecular-chemistry" in pack_names
    assert "condensed-matter-hubbard-lattice" in pack_names
    assert "mathematical-physics-frontier" in pack_names

    math_pack = next(p for p in packs if p.name == "mathematical-physics-frontier")
    assert math_pack.domain_category == "mathematical_physics"
    assert "Analytical-Math-Proof" in math_pack.classical_anchor_methods
    assert "Heisenberg_Ferromagnet_3D" in math_pack.benchmark_systems
