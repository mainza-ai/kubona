"""4-Tier Epistemic Storage Hierarchy and Long-Term Memory Architecture.

Implements the 4-tier storage hierarchy:
- Tier 0: Transient NVMe tmpfs memory for intermediate quantum tensor contractions.
- Tier 1: Immutable Content-Addressed Merkle DAG Ledger.
- Tier 2: HippoRAG Associative Knowledge Graph (GraphRAG with Personalized PageRank).
- Tier 3: Long-term reflective strategy bank (RLVR and meta-learning reasoning traces).
"""

from __future__ import annotations

import hashlib
import json
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional, Set
import numpy as np

from kubona.science.memory import EpistemicKnowledgeGraph, KnowledgeEntity, KnowledgeRelation


@dataclass
class StorageTierRecord:
    tier: int  # 0, 1, 2, or 3
    key: str
    data: Any
    created_at: float
    access_count: int = 0
    content_hash: str = ""


class TieredStorageManager:
    """Manages the 4-tier epistemic memory and knowledge lifecycle."""

    def __init__(self, tmp_dir: Optional[Path | str] = None):
        self.tmp_dir = Path(tmp_dir) if tmp_dir else Path("/tmp/kubona_tier0")
        self.tmp_dir.mkdir(parents=True, exist_ok=True)

        # Tier 0: Transient In-Memory / Tmpfs cache
        self._tier0_cache: Dict[str, StorageTierRecord] = {}

        # Tier 1: Merkle DAG block chain index
        self._tier1_ledger: List[Dict[str, Any]] = []
        self._tier1_head_hash: str = "0" * 64

        # Tier 2: Associative Knowledge Graph (HippoRAG)
        self.graph = EpistemicKnowledgeGraph()

        # Tier 3: Reflective Strategy Bank
        self._tier3_strategy_bank: Dict[str, Dict[str, Any]] = {}

    # --- TIER 0: TRANSIENT TENSOR & SCRATCH BUFFER ---
    def put_tier0(self, key: str, data: Any) -> str:
        """Stores high-throughput intermediate state in Tier 0."""
        serialized = json.dumps(data, default=str, sort_keys=True).encode("utf-8")
        h = hashlib.sha256(serialized).hexdigest()
        record = StorageTierRecord(
            tier=0,
            key=key,
            data=data,
            created_at=time.time(),
            content_hash=h,
        )
        self._tier0_cache[key] = record
        return h

    def get_tier0(self, key: str) -> Optional[Any]:
        rec = self._tier0_cache.get(key)
        if rec:
            rec.access_count += 1
            return rec.data
        return None

    def evict_tier0(self, key: str) -> None:
        self._tier0_cache.pop(key, None)

    # --- TIER 1: MERKLE DAG LEDGER ---
    def append_tier1_block(self, payload: Dict[str, Any]) -> str:
        """Appends an immutable block to Tier 1 content-addressed Merkle DAG."""
        prev_hash = self._tier1_head_hash
        block = {
            "index": len(self._tier1_ledger),
            "timestamp": time.time(),
            "prev_hash": prev_hash,
            "payload": payload,
        }
        encoded = json.dumps(block, sort_keys=True).encode("utf-8")
        block_hash = hashlib.sha256(encoded).hexdigest()
        block["block_hash"] = block_hash

        self._tier1_ledger.append(block)
        self._tier1_head_hash = block_hash
        return block_hash

    def get_tier1_block(self, block_hash: str) -> Optional[Dict[str, Any]]:
        for blk in self._tier1_ledger:
            if blk.get("block_hash") == block_hash:
                return blk
        return None

    # --- TIER 2: ASSOCIATIVE HIPPORAG (PERSONALIZED PAGERANK) ---
    def index_entity(self, entity: KnowledgeEntity) -> None:
        self.graph.add_entity(entity)

    def index_relation(self, relation: KnowledgeRelation) -> None:
        self.graph.add_relation(relation)

    def associative_recall(self, start_entity_id: str, max_hops: int = 2) -> List[Dict[str, Any]]:
        """Multi-hop associative recall across scientific knowledge graph."""
        return self.graph.query_multi_hop(start_entity_id, max_hops=max_hops)

    def personalized_pagerank(
        self,
        seed_entities: List[str],
        alpha: float = 0.85,
        max_iter: int = 30,
        tol: float = 1e-6,
    ) -> Dict[str, float]:
        """Approximates Personalized PageRank (PPR) for associative subgraph discovery."""
        entities = list(self.graph._entities.keys())
        n = len(entities)
        if n == 0:
            return {}

        entity_to_idx = {e: i for i, e in enumerate(entities)}
        
        # Build adjacency matrix
        A = np.zeros((n, n), dtype=float)
        for src, rels in self.graph._adjacency.items():
            if src in entity_to_idx:
                for rel in rels:
                    dst = rel.target_id
                    if dst in entity_to_idx:
                        A[entity_to_idx[dst], entity_to_idx[src]] = rel.confidence_rho

        # Column normalize A
        col_sums = A.sum(axis=0)
        col_sums[col_sums == 0] = 1.0
        M = A / col_sums

        # Personalization vector p
        p = np.zeros(n, dtype=float)
        for seed in seed_entities:
            if seed in entity_to_idx:
                p[entity_to_idx[seed]] = 1.0
        if p.sum() == 0:
            p = np.ones(n, dtype=float) / n
        else:
            p = p / p.sum()

        # Power iteration
        v = p.copy()
        for _ in range(max_iter):
            v_next = alpha * (M @ v) + (1.0 - alpha) * p
            if np.linalg.norm(v_next - v, 1) < tol:
                break
            v = v_next

        return {entities[i]: float(v[i]) for i in range(n)}

    # --- TIER 3: REFLECTIVE STRATEGY BANK ---
    def record_strategy(
        self,
        strategy_id: str,
        domain: str,
        heuristic_name: str,
        reasoning_trace_summary: str,
        success_rate: float,
        properties: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Records meta-learning reasoning strategies and cavity expansion maneuvers."""
        self._tier3_strategy_bank[strategy_id] = {
            "strategy_id": strategy_id,
            "domain": domain,
            "heuristic_name": heuristic_name,
            "reasoning_trace_summary": reasoning_trace_summary,
            "success_rate": success_rate,
            "recorded_at": time.time(),
            "properties": properties or {},
        }

    def query_strategies(self, domain: str) -> List[Dict[str, Any]]:
        return [
            strat for strat in self._tier3_strategy_bank.values()
            if strat["domain"].lower() == domain.lower() or domain == "*"
        ]

    # --- REFUTATION INDEX ---
    def check_refutation(self, keyword: str) -> List[KnowledgeEntity]:
        """Queries the negative certificate refutation index to prune unphysical search paths."""
        return self.graph.find_negative_certificates(keyword)
