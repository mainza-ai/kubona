"""Unit tests for Epistemic Continuum, Quantum Krylov (QKSD), Shadows, and 4-Tier Memory."""

import numpy as np
import pytest

from kubona.epistemics.continuum import (
    ComputationalHypothesis,
    EpistemicContinuum,
    EpistemicLevel,
    NegativeCertificate,
)
from kubona.circuits.krylov import QKSDSolver, QKSDResult
from kubona.circuits.shadows import ClassicalShadowEngine, ShadowSnapshot
from kubona.memory.tiered_storage import TieredStorageManager
from kubona.science.memory import KnowledgeEntity, KnowledgeRelation


def test_epistemic_continuum_levels():
    assert EpistemicContinuum.classify_level(1.0) == EpistemicLevel.LEVEL_4_FORMAL_DEDUCTION
    assert EpistemicContinuum.classify_level(0.97) == EpistemicLevel.LEVEL_3_ANALYTICAL_ANCHOR
    assert EpistemicContinuum.classify_level(0.92) == EpistemicLevel.LEVEL_2_CLASSICAL_BASELINE
    assert EpistemicContinuum.classify_level(0.85) == EpistemicLevel.LEVEL_1_QUANTUM_SUBSPACE
    assert EpistemicContinuum.classify_level(0.50) == EpistemicLevel.LEVEL_0_EXPLORATORY_SEARCH


def test_computational_hypothesis_hashing():
    hyp = ComputationalHypothesis(
        hypothesis_id="h_test_01",
        lean4_statement="theorem heisenberg_gap : E_0 <= -1.5",
        anchor_system="Heisenberg_Ferromagnet_3D",
        circuit_qir=None,
        spend_budget_usd=25.0,
        mandate_signature="ed25519_sig_abc123",
        target_entropy_reduction=1.42,
        description="Heisenberg ferromagnet ground state proof",
    )
    h_hash = hyp.content_hash()
    assert len(h_hash) == 64
    d = hyp.to_dict()
    assert d["hypothesis_id"] == "h_test_01"
    assert d["content_hash"] == h_hash
    # Hash-then-sign: rotating the signature must not move the content address.
    import dataclasses
    assert dataclasses.replace(hyp, mandate_signature="different_signature").content_hash() == h_hash

    # Fisher stopping rule
    should_stop = EpistemicContinuum.evaluate_fisher_stopping_rule(
        expected_fisher_info=0.01,
        marginal_cost_per_step=0.05,
        lambda_multiplier=1.0,
    )
    assert should_stop is True

    should_continue = EpistemicContinuum.evaluate_fisher_stopping_rule(
        expected_fisher_info=0.50,
        marginal_cost_per_step=0.05,
        lambda_multiplier=1.0,
    )
    assert should_continue is False

    # Negative certificate
    neg_cert = EpistemicContinuum.create_negative_certificate(
        hypothesis=hyp,
        refutation_type="BARREN_PLATEAU",
        evidence="Gradient variance vanished below 2^-n threshold",
    )
    assert neg_cert.refutation_type == "BARREN_PLATEAU"
    assert neg_cert.hypothesis_hash == h_hash


def test_qksd_solver_two_qubit():
    # 2-qubit transverse field Ising Hamiltonian: H = - Z0 Z1 - 0.5 (X0 + X1)
    Z = np.array([[1, 0], [0, -1]], dtype=complex)
    X = np.array([[0, 1], [1, 0]], dtype=complex)
    I = np.eye(2, dtype=complex)

    ZZ = np.kron(Z, Z)
    X0 = np.kron(X, I)
    X1 = np.kron(I, X)
    H = -ZZ - 0.5 * (X0 + X1)

    exact_eigs = np.linalg.eigvalsh(H)
    exact_ground = float(exact_eigs[0])

    # Initial state |00>
    psi0 = np.zeros(4, dtype=complex)
    psi0[0] = 1.0

    result = QKSDSolver.solve(
        hamiltonian=H,
        initial_state=psi0,
        subspace_dim=4,
        time_step=0.2,
    )

    assert isinstance(result, QKSDResult)
    assert result.is_converged is True
    # Krylov variational ground state energy must be close to exact ground state
    assert abs(result.ground_state_energy - exact_ground) < 1e-3
    assert result.ground_state_energy >= exact_ground - 1e-6  # Variational bound


def test_classical_shadow_engine():
    # 2-qubit Bell state |Phi+> = 1/sqrt(2) (|00> + |11>)
    state = np.zeros(4, dtype=complex)
    state[0] = 1.0 / np.sqrt(2)
    state[3] = 1.0 / np.sqrt(2)

    # In |Phi+>, <ZZ> = +1.0, <XX> = +1.0, <YY> = -1.0
    snapshots = ClassicalShadowEngine.sample_shadows_from_state(
        state_vector=state,
        num_qubits=2,
        num_snapshots=800,
        rng=np.random.default_rng(123),
    )
    assert len(snapshots) == 800

    est_zz = ClassicalShadowEngine.estimate_observable_from_snapshots(snapshots, "ZZ")
    est_xx = ClassicalShadowEngine.estimate_observable_from_snapshots(snapshots, "XX")

    # Shadow estimation should be close to 1.0 within statistical noise
    assert abs(est_zz - 1.0) < 0.25
    assert abs(est_xx - 1.0) < 0.25


def test_tiered_storage_and_hipporag():
    storage = TieredStorageManager()

    # Tier 0 put/get/evict
    h0 = storage.put_tier0("scratch_tensor_01", {"dimension": 16, "norm": 1.0})
    assert len(h0) == 64
    retrieved = storage.get_tier0("scratch_tensor_01")
    assert retrieved["dimension"] == 16
    storage.evict_tier0("scratch_tensor_01")
    assert storage.get_tier0("scratch_tensor_01") is None

    # Tier 1 Merkle block chaining
    b1_hash = storage.append_tier1_block({"action": "PROVE_LEMMA", "lemma": "L1"})
    b2_hash = storage.append_tier1_block({"action": "VERIFY_ANCHOR", "system": "Family271"})
    assert b1_hash != b2_hash
    b2 = storage.get_tier1_block(b2_hash)
    assert b2["prev_hash"] == b1_hash

    # Tier 2 GraphRAG and Personalized PageRank
    e1 = KnowledgeEntity("ent_haldane", "theorem", "Haldane Spin 1 Gap")
    e2 = KnowledgeEntity("ent_aklt", "hamiltonian", "AKLT Model")
    e3 = KnowledgeEntity("ent_refutation", "negative_certificate", "Barren Plateau Ansatz 4")

    storage.index_entity(e1)
    storage.index_entity(e2)
    storage.index_entity(e3)

    r1 = KnowledgeRelation("ent_haldane", "ent_aklt", "PROVEN_BY", confidence_rho=1.0)
    storage.index_relation(r1)

    ppr = storage.personalized_pagerank(["ent_haldane"], alpha=0.85)
    assert "ent_haldane" in ppr
    assert "ent_aklt" in ppr
    assert ppr["ent_aklt"] > 0.0

    # Negative certificate query
    negs = storage.check_refutation("Barren Plateau")
    assert len(negs) == 1
    assert negs[0].entity_id == "ent_refutation"

    # Tier 3 Strategy Bank
    storage.record_strategy(
        strategy_id="strat_cavity_01",
        domain="quantum_spin_glass",
        heuristic_name="Mezard_Parisi_Diluted_Cavity",
        reasoning_trace_summary="Decoupled single spin cavity before telescope induction",
        success_rate=0.94,
    )
    strats = storage.query_strategies("quantum_spin_glass")
    assert len(strats) == 1
    assert strats[0]["heuristic_name"] == "Mezard_Parisi_Diluted_Cavity"


def test_fisher_rao_credibility_manifold_and_geodesics():
    from kubona.epistemics.curvature import FisherRaoCredibilityManifold

    manifold = FisherRaoCredibilityManifold(dimension=2)
    theta = np.array([1.0, 0.5])
    
    # 1. Metric tensor
    g = manifold.compute_metric_tensor(theta)
    assert g.shape == (2, 2)
    assert np.all(np.linalg.eigvals(g) > 0)  # Positive definite

    # 2. Christoffel symbols
    gamma = manifold.compute_christoffel_symbols(theta)
    assert gamma.shape == (2, 2, 2)
    # Torsion-free (symmetric in lower indices)
    assert np.allclose(gamma[:, 0, 1], gamma[:, 1, 0], atol=1e-5)

    # 3. Ricci curvature
    ricci_tensor, ricci_scalar = manifold.compute_ricci_curvature(theta)
    assert ricci_tensor.shape == (2, 2)
    assert isinstance(ricci_scalar, float)

    # 4. Geodesic step & trajectory
    vel = np.array([0.1, -0.05])
    delta = np.array([0.02, 0.01])
    theta_next, vel_next, r = manifold.geodesic_step(theta, vel, dtau=0.05, delta_perturbation=delta)
    assert theta_next.shape == (2,)
    assert vel_next.shape == (2,)

    traj = manifold.integrate_geodesic(theta, vel, steps=5, dtau=0.02, delta_perturbation=delta)
    assert len(traj.points) == 6
    assert len(traj.curvatures) == 5
    d = traj.to_dict()
    assert d["trajectory_steps"] == 6


def test_orthogonal_qksd_solver():
    from kubona.circuits.krylov import OrthogonalQKSDSolver, QKSDSolver

    # 4x4 Heisenberg dimer Hamiltonian
    # H = J (X1 X2 + Y1 Y2 + Z1 Z2)
    s_x = np.array([[0, 1], [1, 0]], dtype=complex)
    s_y = np.array([[0, -1j], [1j, 0]], dtype=complex)
    s_z = np.array([[1, 0], [0, -1]], dtype=complex)
    H = np.kron(s_x, s_x) + np.kron(s_y, s_y) + np.kron(s_z, s_z)

    # True exact ground energy is -3.0 (singlet state)
    exact_ground = float(np.min(np.linalg.eigvalsh(H)))
    assert abs(exact_ground - (-3.0)) < 1e-6

    # Initial state with non-zero overlap on singlet (|01> - |10>)
    psi_0 = np.array([0.2, 0.8, 0.1, 0.3], dtype=complex)
    psi_0 = psi_0 / np.linalg.norm(psi_0)

    # Test standard QKSD
    res_qksd = QKSDSolver.solve(H, initial_state=psi_0, subspace_dim=4, time_step=0.2)
    assert abs(res_qksd.ground_state_energy - exact_ground) < 0.2

    # Test Orthogonal QKSD (OQKD)
    res_oqkd = OrthogonalQKSDSolver.solve(H, initial_state=psi_0, subspace_dim=4)
    assert abs(res_oqkd.ground_state_energy - exact_ground) < 1e-4
    assert res_oqkd.condition_number == 1.0  # Orthonormal basis
    assert len(res_oqkd.eigenvalues) <= 4

    # Also test via QKSDSolver wrapper
    res_wrap = QKSDSolver.solve_orthogonal_qksd(H, initial_state=psi_0, subspace_dim=4)
    assert abs(res_wrap.ground_state_energy - exact_ground) < 1e-4


def test_classical_shadow_matrix_reconstruction_and_bounds():
    # 2-qubit Bell state |Phi+> = (|00> + |11>) / sqrt(2)
    bell = np.array([1.0, 0, 0, 1.0], dtype=complex) / np.sqrt(2.0)
    snapshots = ClassicalShadowEngine.sample_shadows_from_state(
        bell, num_qubits=2, num_snapshots=800, rng=np.random.default_rng(42)
    )

    # Reconstruct single snapshot matrix
    rho_hat = ClassicalShadowEngine.reconstruct_snapshot_matrix(snapshots[0])
    assert rho_hat.shape == (4, 4)
    # Trace of unbiased inverted channel snapshot is 1
    assert abs(np.trace(rho_hat) - 1.0) < 1e-5

    # Estimate ZZ observable expectation (should be +1 for |Phi+>)
    est_zz = ClassicalShadowEngine.estimate_observable_from_snapshots(snapshots, "ZZ", num_chunks=5)
    assert abs(est_zz - 1.0) < 0.35

    # Matrix observable estimation
    obs_zz = np.kron(
        np.array([[1, 0], [0, -1]], dtype=complex),
        np.array([[1, 0], [0, -1]], dtype=complex),
    )
    est_matrix_zz = ClassicalShadowEngine.estimate_matrix_observable(snapshots, obs_zz, num_chunks=5)
    assert abs(est_matrix_zz - 1.0) < 0.35

    # Shadow norm and sample complexity
    norm_2_local = ClassicalShadowEngine.compute_shadow_norm(pauli_weight=2)
    assert norm_2_local == 9.0  # 3^2
    n_samples = ClassicalShadowEngine.sample_complexity_bound(num_observables_m=100, epsilon=0.1)
    assert n_samples > 0


