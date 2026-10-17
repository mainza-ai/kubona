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


def test_all_twenty_analytical_anchor_families():
    # 1. Family 271: Heisenberg Ferromagnet 3D
    f271 = MathPhysicsAnchorEngine.compute_family_271_heisenberg_ferromagnet(temperature=0.5, J=1.0)
    assert f271["family_id"] == 271
    assert f271["ground_state_energy_density_J"] == -1.5
    # Arithmetic evaluations carry tier + provenance, never a certified flag.
    assert "spontaneous_ferromagnetic_order_certified" not in f271
    assert "evidence_tier" in f271 and "provenance" in f271

    # 2. Family 281: SK Spin Glass
    f281 = MathPhysicsAnchorEngine.compute_family_281_sk_spin_glass(qaoa_p_depth=3)
    assert f281["family_id"] == 281
    assert abs(f281["parisi_ground_state_energy"] - (-0.7631668)) < 1e-4

    # 3. Family 268: Haldane Spin-1 Chain
    f268 = MathPhysicsAnchorEngine.compute_family_268_haldane_gap(J=2.0)
    assert f268["family_id"] == 268
    assert f268["topological_string_order_index"] == -1
    assert abs(f268["haldane_spectral_gap_J"] - 0.820958) < 1e-4

    # 4. Family 278: Kohn-Sham Failure
    f278 = MathPhysicsAnchorEngine.compute_family_278_kohn_sham_failure(num_electrons=3, nuclear_charges=[1, 1, 1])
    assert f278["fails_noninteracting_representability"] is True

    # 5. Family 275: Continuum Coulomb QMA
    f275 = MathPhysicsAnchorEngine.compute_family_275_continuum_coulomb_qma_gap(promise_gap_hartree=1.2)
    assert f275["is_qma_hard"] is True

    # 6. Family 279: Clifford+T Factoring
    f279 = MathPhysicsAnchorEngine.compute_family_279_clifford_t_factoring(bit_length=128)
    assert f279["ancilla_qubits"] == 0
    assert f279["exact_success_probability"] == 1.0

    # 7. Family 274: QAC0 Parity Bound
    f274 = MathPhysicsAnchorEngine.compute_family_274_qac0_parity_bound(circuit_depth=3, num_qubits=64)
    assert f274["in_qac0"] is False
    assert f274["error_prob_lower_bound"] > 0.3

    # 8. Family 284: Quartic Query Separation
    f284 = MathPhysicsAnchorEngine.compute_family_284_quartic_query_separation(quantum_queries=5)
    assert f284["randomized_query_upper_bound"] == (1 + 5) ** 4
    assert f284["refutes_cubic_relation"] is True

    # 9. Family 265: 2D PEPS Area Law
    f265 = MathPhysicsAnchorEngine.compute_family_265_peps_area_law_bound(linear_size_L=20.0)
    assert f265["peps_vector_error_bound"] == 0.05

    # 10. Family 266: MUB Dimension 6
    f266 = MathPhysicsAnchorEngine.compute_family_266_mub_dimension_6()
    assert f266["max_mutually_unbiased_bases"] == 3
    assert f266["fourth_mub_exists"] is False

    # 11. Family 269: Laughlin Fractional Gap
    f269 = MathPhysicsAnchorEngine.compute_family_269_laughlin_fractional_gap()
    assert f269["v1_interaction_spectral_gap_lower_bound"] == 0.04

    # 12. Family 276: Generalized Amplitude Damping
    f276 = MathPhysicsAnchorEngine.compute_family_276_generalized_amplitude_damping(gamma=0.2, thermal_N=0.1)
    assert 0.0 <= f276["classical_capacity_bits"] <= 1.0
    assert abs(f276["dephasing_time_t2"] - 2.0 * f276["relaxation_time_t1"]) < 1e-3

    # 13. Family 272: Bound Entanglement No Key
    f272 = MathPhysicsAnchorEngine.compute_family_272_bound_entanglement_secret_key(dimension=10)
    assert f272["distillable_secret_key_rate"] == 0.0
    assert f272["disproves_christandl_ppt_square"] is True

    # 14. Family 277: Threshold Repetition Entangled Games
    f277 = MathPhysicsAnchorEngine.compute_family_277_threshold_repetition_entangled_games(value_g=0.75, repetitions=12)
    assert f277["entangled_value_upper_bound"] < 0.75

    # 15. Family 283: Unitary Synthesis Oracle
    f283 = MathPhysicsAnchorEngine.compute_family_283_unitary_synthesis_oracle(dim=16)
    assert f283["diamond_norm_approximation_bound"] <= 0.5

    # 16. Family 261: Anderson Localization
    f261_2d = MathPhysicsAnchorEngine.compute_family_261_anderson_localization(spatial_dimension=2, disorder_strength=0.5)
    assert f261_2d["spectral_type"] == "pure_point_localized"
    f261_3d = MathPhysicsAnchorEngine.compute_family_261_anderson_localization(spatial_dimension=3, disorder_strength=2.0)
    assert f261_3d["spectral_type"] == "absolutely_continuous_metallic"

    # 17. Family 263: Atomic Ionization Cap
    f263 = MathPhysicsAnchorEngine.compute_family_263_atomic_ionization_cap(atomic_number_Z=11)
    assert f263["max_bound_electrons"] == 12
    assert f263["max_excess_electrons"] == 1

    # 18. Family 267: Positive Temperature BEC Depletion
    f267 = MathPhysicsAnchorEngine.compute_family_267_bec_depletion_fraction(gas_parameter_rho_a3=4e-4)
    assert f267["bogoliubov_quantum_depletion_fraction"] > 0.0

    # 19. Family 270: BFSS Matrix Model
    f270 = MathPhysicsAnchorEngine.compute_family_270_bfss_matrix_model(gauge_group_N=3)
    assert f270["ground_state_energy"] == 0.0
    assert f270["unique_zero_energy_state"] is True

    # 20. Family 287: Free Group Factors Isomorphism
    f287 = MathPhysicsAnchorEngine.compute_family_287_free_group_factor_isomorphism()
    assert "L(F_2)" in f287["isomorphic_factors"]
    assert f287["fundamental_group"] == "R_{>0}"

    # Test unified dispatcher
    assert MathPhysicsAnchorEngine.compute_family_engine(271)["family_id"] == 271
    assert MathPhysicsAnchorEngine.compute_family_engine(287)["fundamental_group"] == "R_{>0}"

