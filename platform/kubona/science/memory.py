"""Epistemic Knowledge Graph, Memory Architecture, and Self-Evolution Engine (Ring 2 / Science Core).

Implements temporally aware GraphRAG memory, multi-hop scientific reasoning,
negative certificate retrieval, and self-improving meta-search evolution.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Set


@dataclass(frozen=True)
class KnowledgeEntity:
    entity_id: str
    entity_type: str  # "molecule", "hamiltonian", "ansatz", "theorem", "lemma", "anchor", "negative_certificate"
    name: str
    properties: Dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class KnowledgeRelation:
    source_id: str
    target_id: str
    relation_type: str  # "PROVEN_BY", "BOUNDED_BY", "FAILS_ON", "REPRESENTED_AS", "REFUTES", "PRECEDES"
    confidence_rho: float = 1.0  # 0.0 to 1.0
    ledger_hash: Optional[str] = None
    properties: Dict[str, Any] = field(default_factory=dict)


class EpistemicKnowledgeGraph:
    """Temporally aware scientific knowledge graph supporting multi-hop associative recall."""

    def __init__(self):
        self._entities: Dict[str, KnowledgeEntity] = {}
        self._adjacency: Dict[str, List[KnowledgeRelation]] = {}
        self._reverse_adjacency: Dict[str, List[KnowledgeRelation]] = {}

    def add_entity(self, entity: KnowledgeEntity) -> None:
        self._entities[entity.entity_id] = entity
        if entity.entity_id not in self._adjacency:
            self._adjacency[entity.entity_id] = []
        if entity.entity_id not in self._reverse_adjacency:
            self._reverse_adjacency[entity.entity_id] = []

    def add_relation(self, relation: KnowledgeRelation) -> None:
        if relation.source_id not in self._entities or relation.target_id not in self._entities:
            raise ValueError(f"Both entities ({relation.source_id}, {relation.target_id}) must exist in graph.")
        self._adjacency[relation.source_id].append(relation)
        self._reverse_adjacency[relation.target_id].append(relation)

    def get_entity(self, entity_id: str) -> Optional[KnowledgeEntity]:
        return self._entities.get(entity_id)

    def query_multi_hop(self, start_id: str, max_hops: int = 2) -> List[Dict[str, Any]]:
        """Traverses knowledge paths up to max_hops away from start_id."""
        if start_id not in self._entities:
            return []

        results = []
        visited: Set[str] = {start_id}
        queue = [(start_id, 0, [])]

        while queue:
            current_id, depth, path = queue.pop(0)
            if depth >= max_hops:
                continue

            for rel in self._adjacency.get(current_id, []):
                next_id = rel.target_id
                target_entity = self._entities[next_id]
                new_path = path + [{
                    "from": current_id,
                    "relation": rel.relation_type,
                    "to": next_id,
                    "confidence_rho": rel.confidence_rho,
                    "target_type": target_entity.entity_type,
                    "target_name": target_entity.name
                }]
                results.append({
                    "hop_depth": depth + 1,
                    "target_entity": target_entity,
                    "path": new_path
                })
                if next_id not in visited:
                    visited.add(next_id)
                    queue.append((next_id, depth + 1, new_path))

        return results

    def find_negative_certificates(self, topic_keyword: str) -> List[KnowledgeEntity]:
        """Retrieves verified negative certificates and refutations matching keyword."""
        matches = []
        for entity in self._entities.values():
            if entity.entity_type == "negative_certificate":
                if topic_keyword.lower() in entity.name.lower() or topic_keyword.lower() in str(entity.properties).lower():
                    matches.append(entity)
        return matches


@dataclass
class HeuristicMutationProposal:
    target_harness: str
    suggested_search_radius_adjustment: float
    banned_ansatz_patterns: List[str]
    rationale: str
    evidence_campaign_count: int


class SelfEvolutionEngine:
    """Analyzes campaign trajectories and knowledge graphs to evolve search heuristics."""

    def __init__(self, knowledge_graph: EpistemicKnowledgeGraph):
        self.kg = knowledge_graph
        self._learned_heuristics: List[HeuristicMutationProposal] = []

    def analyze_campaign_refutations(self, target_harness: str) -> Optional[HeuristicMutationProposal]:
        """Examines negative certificates to propose anti-patterns and prevent repeated dead ends."""
        negative_certs = [
            e for e in self.kg._entities.values()
            if e.entity_type == "negative_certificate" and e.properties.get("harness") == target_harness
        ]

        if not negative_certs:
            return None

        banned_patterns = []
        for cert in negative_certs:
            pattern = cert.properties.get("violating_ansatz_pattern")
            if pattern and pattern not in banned_patterns:
                banned_patterns.append(pattern)

        proposal = HeuristicMutationProposal(
            target_harness=target_harness,
            suggested_search_radius_adjustment=1.25 if len(negative_certs) > 3 else 1.0,
            banned_ansatz_patterns=banned_patterns,
            rationale=(
                f"Identified {len(negative_certs)} negative certificates for {target_harness}. "
                "Recommending exclusion of non-convergent ansatz structures and widening exploration radius."
            ),
            evidence_campaign_count=len(negative_certs)
        )
        self._learned_heuristics.append(proposal)
        return proposal
