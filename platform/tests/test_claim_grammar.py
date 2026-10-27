"""Q-04 claim grammar + Q-05 worked VQE credibility example.

Q-04: the grammar is enforced at construction (fail-closed): banned hype
terms refuse, predicates without a permitted reporting verb refuse, advantage
claims without anchor + justification + credibility refuse.
Q-05: worked example on real numbers — H2 FCI anchor from
MolecularAnchorEngine, a VQE measurement within chemical accuracy, three
evidence signals through ClaimCredibilityWavefunction to a collapsed
corroborated record with rho, stored ledger-ready.
"""

import pytest
from pydantic import ValidationError

from kubona.anchors.chemistry import MolecularAnchorEngine
from kubona.epistemics.claim_grammar import (
    AdvantageJustification,
    ClaimCategory,
    CredibilityField,
    EvidenceTier,
    HardwareProvenance,
    ScientificClaim,
)
from kubona.epistemics.wavefunction import (
    ClaimCredibilityWavefunction,
    EpistemicGateError,
    EvidenceSignal,
    build_claim_record,
)


def _hw() -> HardwareProvenance:
    return HardwareProvenance(
        backend_name="qiskit_aer.AerSimulator",
        backend_tier="T1_LOCAL_EMU",
        shots=4000,
        seed=7,
    )


def _base_claim(**over) -> dict:
    kw = dict(
        category=ClaimCategory.EMPIRICAL_SIMULATION,
        predicate="Measured H2 ground energy converged within chemical accuracy",
        classical_anchor_name="PySCF STO-3G FCI",
        classical_anchor_value=-1.137,
        quantum_measured_value=-1.136,
        delta_hartree=0.001,
        chemical_accuracy_achieved=True,
        uncertainty_sigma=0.0005,
        confidence_interval_95=(-1.137, -1.135),
        falsification_criteria=["independent FCI recomputation differs by >1.6 mHa"],
        hardware_provenance=_hw(),
        evidence_tier=EvidenceTier.T3_SAMPLED,
    )
    kw.update(over)
    return kw


def test_valid_measurement_claim_passes():
    c = ScientificClaim(**_base_claim())
    assert "[NO ADVANTAGE CLAIMED]" in c.formatted_statement()


def test_banned_hype_term_refuses():
    with pytest.raises(ValidationError, match="Forbidden ungrounded term"):
        ScientificClaim(**_base_claim(predicate="Measured quantum supremacy over chemistry"))


def test_predicate_without_reporting_verb_refuses():
    with pytest.raises(ValidationError, match="permitted reporting verb"):
        ScientificClaim(**_base_claim(predicate="The system works well overall today"))


def _just() -> AdvantageJustification:
    return AdvantageJustification(
        method="VQE RY-RY-CX depth2 simulator run",
        anchor_ref="ledger:anchor:h2-fci",
    )


def test_advantage_without_anchor_or_credibility_refuses():
    with pytest.raises(ValidationError):
        ScientificClaim(**_base_claim(claims_advantage=True, chemical_accuracy_achieved=False))
    with pytest.raises(ValidationError, match="credibility field"):
        ScientificClaim(
            **_base_claim(
                claims_advantage=True,
                advantage_justification=_just(),
            )
        )


def test_advantage_with_grounded_credibility_passes():
    c = ScientificClaim(
        **_base_claim(
            claims_advantage=True,
            advantage_justification=_just(),
            credibility=CredibilityField(rho=0.95, tier="L3", evidence_refs=["ledger:anchor:1"]),
        )
    )
    assert "[ADVANTAGE GROUNDED]" in c.formatted_statement()
    assert "ρ=0.950" in c.formatted_statement()


def test_advantage_with_weak_credibility_refuses():
    with pytest.raises(ValidationError, match="rho >= 0.90"):
        ScientificClaim(
            **_base_claim(
                claims_advantage=True,
                advantage_justification=_just(),
                credibility=CredibilityField(rho=0.5, tier="L1", evidence_refs=["x"]),
            )
        )


def test_worked_vqe_credibility_example():
    """Q-05 worked example, real numbers end to end.

    Axes: {corroborated, contested, unverified}. Computed by:
    ClaimCredibilityWavefunction.evaluate over three evidence signals.
    Stored: ClaimRecord.to_ledger_payload (same provenance ledger family).
    Consumed: rho gates assertion; refs — never the score — get cited.
    """
    engine = MolecularAnchorEngine()
    anchor = engine.compute_anchor("H2")
    # Real engine numbers (STO-3G, R=0.7414A): FCI below RHF, correlation
    # energy negative. Derived from the engine, never hardcoded.
    assert anchor.fci_energy_hartree < anchor.rhf_energy_hartree
    assert anchor.fci_energy_hartree < 0

    # Simulated VQE measurement 1.0 mHa above FCI: inside chemical accuracy.
    vqe_energy = anchor.fci_energy_hartree + 0.001
    ev = engine.evaluate_advantage_claim(vqe_energy, anchor)
    assert ev["chemical_accuracy_achieved"] is True

    signals = [
        EvidenceSignal(source_id="pyscf-fci-anchor", source_type="reproduction",
                       supports=True, confidence=1.0, engagement=0.9,
                       ledger_ref="ledger:anchor:h2-fci"),
        EvidenceSignal(source_id="vqe-simulator-run", source_type="experiment",
                       supports=True, confidence=0.95, engagement=0.7,
                       ledger_ref="ledger:job:vqe-h2-001"),
        EvidenceSignal(source_id="zne-canary", source_type="qpu_canary",
                       supports=True, confidence=0.85, engagement=0.5,
                       ledger_ref="ledger:canary:zne-014"),
    ]
    wf = ClaimCredibilityWavefunction()
    state = wf.evaluate(signals)
    assert state.collapsed is True
    assert state.verdict == "corroborated"

    record = build_claim_record(
        "Measured H2 ground energy converged within chemical accuracy",
        state, signals, computed_at="2026-10-09T00:00:00Z",
    )
    assert record.rho > 0.75
    payload = record.to_ledger_payload()
    assert payload["verdict"] == "corroborated"
    assert payload["evidence_refs"] == ["ledger:anchor:h2-fci", "ledger:job:vqe-h2-001", "ledger:canary:zne-014"]
    assert "policy_digest" in payload and "quantum_provenance" in payload

    # The record's rho flows into a report claim's credibility field.
    # 2026-10-26 audit P0-8: the H2 anchor is now a live PySCF RHF+FCI
    # solve (-1.13727 Ha at R=0.7414 A). The claim derives the anchor value
    # from the engine itself (delta recomputed, never caller-supplied), so
    # the earlier -1.0718 table value can never come back unnoticed.
    anchor_fci = anchor.fci_energy_hartree
    claim = ScientificClaim(
        **_base_claim(
            classical_anchor_value=anchor_fci,
            quantum_measured_value=vqe_energy,
            delta_hartree=abs(vqe_energy - anchor_fci),
            confidence_interval_95=(vqe_energy - 0.001, vqe_energy + 0.001),
            credibility=CredibilityField(rho=round(record.rho, 3), tier="L2",
                                         evidence_refs=payload["evidence_refs"]),
        )
    )
    assert f"ρ={record.rho:.3f}" in claim.formatted_statement()


def test_uncollapsed_record_has_zero_rho():
    wf = ClaimCredibilityWavefunction()
    state = wf.evaluate([
        EvidenceSignal(source_id="lone-blog", source_type="arxiv",
                       supports=True, confidence=0.9, engagement=0.0),
    ])
    assert state.collapsed is False
    record = build_claim_record("Measured something somewhere", state, [])
    assert record.rho == 0.0
    with pytest.raises(EpistemicGateError, match="uncollapsed"):
        wf.assert_claim("Measured something somewhere", state)


def test_missing_evidence_tier_refuses():
    kw = _base_claim()
    del kw["evidence_tier"]
    with pytest.raises(ValidationError):
        ScientificClaim(**kw)


def test_invalid_evidence_tier_refuses():
    with pytest.raises(ValidationError):
        ScientificClaim(**_base_claim(evidence_tier="T9_MAYBE"))


def test_evidence_tier_renders_in_statement():
    c = ScientificClaim(**_base_claim())
    assert "[T3_SAMPLED]" in c.formatted_statement()
    c1 = ScientificClaim(**_base_claim(evidence_tier=EvidenceTier.T1_VERIFIED))
    assert "[T1_VERIFIED]" in c1.formatted_statement()
