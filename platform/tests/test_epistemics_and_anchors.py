"""Unit tests for Ring 1 Epistemic Wavefunctions and Ring 2 Classical Anchors."""

import pytest
from kubona.anchors.chemistry import MolecularAnchorEngine
from kubona.discovery.campaign import DiscoveryCampaign
from kubona.epistemics.wavefunction import (
    ClaimCredibilityWavefunction,
    CredibilityPolicy,
    EpistemicGateError,
    EvidenceSignal,
)


def test_wavefunction_empty_prior():
    wf = ClaimCredibilityWavefunction()
    state = wf.evaluate([])
    assert not state.collapsed
    assert state.verdict == "uncollapsed"
    assert state.probabilities["unverified"] >= 0.85
    with pytest.raises(EpistemicGateError):
        wf.assert_claim("Energy reached -1.137 Ha", state)


def test_wavefunction_single_source_cannot_collapse():
    wf = ClaimCredibilityWavefunction()
    # High confidence but single source (diversity = 0.2 < 0.3, n_ev = 1 < 3)
    ev = [EvidenceSignal(source_id="paper1", source_type="arxiv", supports=True, confidence=1.0, engagement=0.1)]
    state = wf.evaluate(ev)
    # Peak may be high, but strength gate must reject
    assert not state.collapsed
    assert state.verdict == "uncollapsed"


def test_wavefunction_multi_source_collapse():
    wf = ClaimCredibilityWavefunction()
    # 3 distinct sources with high confidence
    ev = [
        EvidenceSignal(source_id="exp1", source_type="experiment", supports=True, confidence=1.0),
        EvidenceSignal(source_id="rep1", source_type="reproduction", supports=True, confidence=0.9),
        EvidenceSignal(source_id="qpu1", source_type="qpu_canary", supports=True, confidence=0.9),
    ]
    state = wf.evaluate(ev)
    assert state.collapsed
    assert state.verdict == "corroborated"
    assert state.probabilities["corroborated"] > 0.75
    assertion = wf.assert_claim("Ground state matches FCI", state)
    assert assertion.startswith("[CORROBORATED]")


def test_wavefunction_contested_state():
    wf = ClaimCredibilityWavefunction()
    # High contradictory evidence
    ev = [
        EvidenceSignal(source_id="exp1", source_type="experiment", supports=False, confidence=1.0),
        EvidenceSignal(source_id="rep1", source_type="reproduction", supports=False, confidence=1.0),
        EvidenceSignal(source_id="rep2", source_type="reproduction", supports=False, confidence=0.9),
    ]
    state = wf.evaluate(ev)
    assert state.collapsed
    assert state.verdict == "contested"
    assert state.probabilities["contested"] > 0.75
    assertion = wf.assert_claim("Energy diverges", state)
    assert assertion.startswith("[CONTESTED]")


def test_molecular_anchor_engine():
    engine = MolecularAnchorEngine()
    anchor = engine.compute_anchor("H2")
    assert anchor.system_name == "H2"
    assert anchor.active_space_qubits == 2
    assert anchor.fci_energy_hartree < anchor.rhf_energy_hartree
    assert anchor.correlation_energy() < 0.0

    # Advantage check: standing policy never claims advantage even if accuracy is achieved
    eval_res = engine.evaluate_advantage_claim(anchor.fci_energy_hartree, anchor)
    assert eval_res["chemical_accuracy_achieved"]
    assert not eval_res["quantum_advantage_claimed"]
    assert "chemical accuracy" in eval_res["honest_verdict"].lower()


def test_discovery_campaign_with_epistemics_and_anchors(tmp_path):
    campaign = DiscoveryCampaign(problem="H2-dissociation", hamiltonian="H2", ground_state=-1.137, budget_steps=10)
    result = campaign.run(source="mock")
    assert result["stages"]["report"]["collapsed"]
    assert "epistemics" in result["stages"]
    assert "anchor" in result["stages"]["epistemics"]
    assert result["report"]["classical_anchor"] is not None
    assert len(result["report"]["claims"]) > 0
    # The claim was evaluated by the gate and collapsed into a rigorous verdict
    assert any(tag in result["report"]["conclusions"] for tag in ("[CORROBORATED]", "[CONTESTED]"))
    assert "Does not surpass classical benchmark" in result["report"]["conclusions"]


def test_fisher_rao_credibility_manifold():
    import numpy as np
    from kubona.epistemics.curvature import FisherRaoCredibilityManifold

    manifold = FisherRaoCredibilityManifold(dimension=2)
    theta = np.array([1.5, 2.0])

    # 1. Metric tensor: symmetric, positive definite
    g = manifold.compute_metric_tensor(theta)
    assert g.shape == (2, 2)
    assert np.allclose(g, g.T)
    eigenvalues = np.linalg.eigvalsh(g)
    assert np.all(eigenvalues > 0)

    # 2. Christoffel symbols
    gamma = manifold.compute_christoffel_symbols(theta)
    assert gamma.shape == (2, 2, 2)
    # Torsion-free connection: Γ^μ_αβ = Γ^μ_βα
    for mu in range(2):
        assert np.allclose(gamma[mu], gamma[mu].T, atol=1e-5)

    # 3. Ricci curvature tensor and scalar
    ricci_tensor, ricci_scalar = manifold.compute_ricci_curvature(theta)
    assert ricci_tensor.shape == (2, 2)
    assert isinstance(ricci_scalar, float)

    # 4. Geodesic integration
    v0 = np.array([0.1, -0.05])
    trajectory = manifold.integrate_geodesic(theta, v0, steps=5, dtau=0.02)
    assert len(trajectory.points) == 6
    assert len(trajectory.velocities) == 6
    assert len(trajectory.curvatures) == 5
    summary = trajectory.to_dict()
    assert summary["trajectory_steps"] == 6
    assert "endpoints" in summary

