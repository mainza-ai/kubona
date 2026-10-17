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

#: Bound on the Tier-3 reflective strategy bank (W2.8). The bank is keyed by
#: strategy_id but re-recorded content dedups (see record_strategy), and past
#: this bound the lowest-success oldest entry is evicted. One const, so the
#: bound cannot rot apart from the code that enforces it.
MAX_STRATEGIES = 256


@dataclass
class StorageTierRecord:
    tier: int  # 0, 1, 2, or 3
    key: str
    data: Any
    created_at: float
    access_count: int = 0
    content_hash: str = ""
    last_accessed_at: float = 0.0


class TieredStorageManager:
    """Manages the 4-tier epistemic memory and knowledge lifecycle.

    Retention policy, per tier (W2.8 — documented, no silent immortality):
    - Tier 0 (transient cache): entries live until `evict_tier0` is called.
      No time-decay runs anywhere in this file.
    - Tier 1 (Merkle ledger): append-only and immutable — blocks are never
      decayed, compacted, or rewritten.
    - Tier 2 (knowledge graph): monotonic growth; entities/relations are
      never decayed by this manager.
    - Tier 3 (strategy bank): bounded at MAX_STRATEGIES with content dedup;
      overflow evicts the lowest-success oldest entry (see record_strategy).
    """

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
        """Read-only fetch (W2.8): returns the data WITHOUT touching access
        metadata. A read that mutates LastAccessedAt lies to every
        recency-based policy downstream. Call `touch_tier0` to mark access
        explicitly."""
        rec = self._tier0_cache.get(key)
        if rec:
            return rec.data
        return None

    def touch_tier0(self, key: str) -> bool:
        """Explicit access-marking (W2.8): bump access_count and stamp
        last_accessed_at. Returns False when the key is absent."""
        rec = self._tier0_cache.get(key)
        if rec is None:
            return False
        rec.access_count += 1
        rec.last_accessed_at = time.time()
        return True

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
    @staticmethod
    def _strategy_dedup_key(domain: str, heuristic_name: str,
                            reasoning_trace_summary: str) -> str:
        """Content identity for dedup (W2.8): same domain + heuristic + summary
        hash is the same strategy, regardless of strategy_id."""
        h = hashlib.sha256(reasoning_trace_summary.encode("utf-8")).hexdigest()[:16]
        return f"{domain.lower()}|{heuristic_name}|{h}"

    def record_strategy(
        self,
        strategy_id: str,
        domain: str,
        heuristic_name: str,
        reasoning_trace_summary: str,
        success_rate: float,
        properties: Optional[Dict[str, Any]] = None,
    ) -> None:
        """Records meta-learning reasoning strategies, bounded with dedup (W2.8).

        - Dedup: an entry with the same (domain, heuristic, summary) content
          updates in place (latest success_rate/properties win, original
          recorded_at kept, update_count bumped) instead of duplicating.
        - Bound: past MAX_STRATEGIES the lowest-success oldest entry is
          evicted — deterministic, documented, never silent unbounded growth.
        """
        if not isinstance(success_rate, (int, float)) or not (0.0 <= success_rate <= 1.0):
            raise ValueError(f"success_rate must be in [0, 1], got {success_rate!r}")
        dedup = self._strategy_dedup_key(domain, heuristic_name, reasoning_trace_summary)
        for entry in self._tier3_strategy_bank.values():
            if entry.get("_dedup") == dedup:
                entry["success_rate"] = float(success_rate)
                entry["properties"] = properties or {}
                entry["updated_at"] = time.time()
                entry["update_count"] = int(entry.get("update_count", 0)) + 1
                return
        self._tier3_strategy_bank[strategy_id] = {
            "strategy_id": strategy_id,
            "domain": domain,
            "heuristic_name": heuristic_name,
            "reasoning_trace_summary": reasoning_trace_summary,
            "success_rate": float(success_rate),
            "recorded_at": time.time(),
            "updated_at": None,
            "update_count": 0,
            "properties": properties or {},
            "_dedup": dedup,
        }
        while len(self._tier3_strategy_bank) > MAX_STRATEGIES:
            victim = min(self._tier3_strategy_bank.values(),
                         key=lambda e: (e["success_rate"], e["recorded_at"]))
            del self._tier3_strategy_bank[victim["strategy_id"]]

    def query_strategies(self, domain: str) -> List[Dict[str, Any]]:
        return [
            strat for strat in self._tier3_strategy_bank.values()
            if strat["domain"].lower() == domain.lower() or domain == "*"
        ]

    # --- REFUTATION INDEX ---
    def search_refutations_substring(self, keyword: str) -> List[KnowledgeEntity]:
        """Lexical substring search over negative certificates (W2.8, honest
        name): matches `keyword` against certificate names/properties
        case-insensitively. This is NOT semantic or vector recall — no vector
        store exists in this file — and the name says so."""
        return self.graph.search_substring(keyword)

    def check_refutation(self, keyword: str) -> List[KnowledgeEntity]:
        """Backward-compatible alias for `search_refutations_substring`
        (substring, not semantic — see above)."""
        return self.search_refutations_substring(keyword)
