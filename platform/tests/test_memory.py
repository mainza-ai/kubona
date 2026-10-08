"""Tests for Epistemic Knowledge Graph and Self-Evolution Engine."""

import pytest
from kubona.science.memory import (
    EpistemicKnowledgeGraph,
    KnowledgeEntity,
    KnowledgeRelation,
    SelfEvolutionEngine
)


def test_knowledge_graph_multi_hop_query():
    kg = EpistemicKnowledgeGraph()

    # Add entities
    femoco = KnowledgeEntity(
        entity_id="mol_femoco",
        entity_type="molecule",
        name="FeMoco Nitrogenase Cluster",
        properties={"electrons": 112, "active_orbitals": 74}
    )
    fam278 = KnowledgeEntity(
        entity_id="theorem_fam278",
        entity_type="theorem",
        name="Failure of Kohn-Sham ensemble representation",
        properties={"family_id": 278, "lean4_verified": True}
    )
    qksd = KnowledgeEntity(
        entity_id="alg_qksd",
        entity_type="ansatz",
        name="Quantum Krylov Subspace Diagonalization",
        properties={"type": "subspace_diagonalization", "avoids_barren_plateaus": True}
    )
    qpu_quera = KnowledgeEntity(
        entity_id="dev_quera_aquila",
        entity_type="hardware_backend",
        name="QuEra Neutral Atom Processor",
        properties={"modality": "neutral_atom", "logical_qubits": 48}
    )

    kg.add_entity(femoco)
    kg.add_entity(fam278)
    kg.add_entity(qksd)
    kg.add_entity(qpu_quera)

    # Add relations
    kg.add_relation(KnowledgeRelation(
        source_id="mol_femoco",
        target_id="theorem_fam278",
        relation_type="PROVEN_BY",
        confidence_rho=1.0
    ))
    kg.add_relation(KnowledgeRelation(
        source_id="theorem_fam278",
        target_id="alg_qksd",
        relation_type="ROUTES_TO",
        confidence_rho=0.98
    ))
    kg.add_relation(KnowledgeRelation(
        source_id="alg_qksd",
        target_id="dev_quera_aquila",
        relation_type="DISPATCHED_ON",
        confidence_rho=0.95
    ))

    # Multi-hop query: from FeMoco, traverse 3 hops
    hops = kg.query_multi_hop("mol_femoco", max_hops=3)
    assert len(hops) == 3

    hop1 = next(h for h in hops if h["hop_depth"] == 1)
    assert hop1["target_entity"].entity_id == "theorem_fam278"

    hop2 = next(h for h in hops if h["hop_depth"] == 2)
    assert hop2["target_entity"].entity_id == "alg_qksd"

    hop3 = next(h for h in hops if h["hop_depth"] == 3)
    assert hop3["target_entity"].entity_id == "dev_quera_aquila"


def test_negative_certificate_retrieval_and_self_evolution():
    kg = EpistemicKnowledgeGraph()

    # Register failed experiments as negative certificates
    nc1 = KnowledgeEntity(
        entity_id="nc_001",
        entity_type="negative_certificate",
        name="Barren Plateau on Hardware Efficient Ansatz",
        properties={
            "harness": "ansatz_vqe_harness",
            "violating_ansatz_pattern": "HardwareEfficient_CZ_Alternating",
            "reason": "Gradient variance < 1e-7 for n >= 14"
        }
    )
    nc2 = KnowledgeEntity(
        entity_id="nc_002",
        entity_type="negative_certificate",
        name="Barren Plateau on Hardware Efficient Deep Circuit",
        properties={
            "harness": "ansatz_vqe_harness",
            "violating_ansatz_pattern": "HardwareEfficient_CZ_Alternating",
            "reason": "Entanglement-induced barren plateau"
        }
    )
    kg.add_entity(nc1)
    kg.add_entity(nc2)

    matches = kg.find_negative_certificates("Hardware Efficient")
    assert len(matches) == 2

    # Self-evolution engine proposes heuristic update
    engine = SelfEvolutionEngine(kg)
    proposal = engine.analyze_campaign_refutations("ansatz_vqe_harness")
    assert proposal is not None
    assert proposal.target_harness == "ansatz_vqe_harness"
    assert "HardwareEfficient_CZ_Alternating" in proposal.banned_ansatz_patterns
    assert proposal.evidence_campaign_count == 2
