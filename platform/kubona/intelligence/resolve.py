"""Entity resolution: block -> match -> cluster with verified merges.

Pure-python, dependency-free, no network (static-gate safe). Implements the
practitioner lessons (block/match/cluster architecture; verified merges
against transitive chaining; sparsity-aware floors):
  - block: by (kind, first significant token) — pairs only form inside blocks.
  - match: token-set Jaccard blended with alias overlap; sparse records
    (<=1 token) need an exact alias hit or they never match (sparsity floor).
  - cluster: union-find, but every union is *verified* — the candidate must
    score >= threshold against ALL current members, not just one linker.
    This refuses the classic chain-merge failure (A~B, B~C, but A!~C).
  - Output is merge *proposals* (T3 candidates). `apply_merges` executes them
    only with a named validator; losers are retracted with supersedes chains
    (id-preserving, never deleted).
"""

from __future__ import annotations

from typing import Dict, List, Sequence

from kubona.intelligence.entities import EntityStore, IntelligenceError

TOKEN_STOPWORDS = frozenset({"the", "inc", "incorporated", "ltd", "limited",
                             "corp", "corporation", "co", "gmbh", "sas", "pty",
                             "quantum", "labs", "lab", "technologies",
                             "technology", "systems", "computing"})


def tokens(name: str) -> frozenset:
    import re
    words = re.findall(r"[a-z0-9]+", name.lower())
    return frozenset(w for w in words if w not in TOKEN_STOPWORDS)


def significant_token(name: str) -> str:
    toks = sorted(tokens(name))
    return toks[0] if toks else ""


def pair_score(a_names: Sequence[str], b_names: Sequence[str]) -> float:
    """Max over name/alias pairs of token-set Jaccard; exact alias hit = 1.0."""
    a_sets = [(n.lower().strip(), tokens(n)) for n in a_names]
    b_sets = [(n.lower().strip(), tokens(n)) for n in b_names]
    best = 0.0
    for an, at in a_sets:
        for bn, bt in b_sets:
            if an == bn and an:
                return 1.0
            if not at or not bt:
                continue
            inter = len(at & bt)
            union = len(at | bt)
            best = max(best, inter / union if union else 0.0)
    return best


def block_index(store: EntityStore) -> Dict[str, List[str]]:
    """(kind, significant token) -> entity ids. Sparse/empty tokens each get
    their own singleton block (they never match without an alias hit)."""
    blocks: Dict[str, List[str]] = {}
    for ent in store.entities.values():
        sig = significant_token(ent.name)
        key = f"{ent.kind}:{sig or ent.id}"
        blocks.setdefault(key, []).append(ent.id)
    return blocks


def propose_merges(store: EntityStore, threshold: float = 0.67,
                   floor: float = 0.5) -> List[Dict[str, object]]:
    """Return merge proposals: {members, confidence, reason}.

    Union-find with verified merges: a candidate joins a cluster only if it
    scores >= `threshold` against the linker AND >= `floor` against every
    existing member. Sparse records (<=1 significant token on either side)
    additionally require an exact alias/normalized-name hit.
    """
    parent: Dict[str, str] = {}

    def find(x: str) -> str:
        while parent[x] != x:
            parent[x] = parent[parent[x]]
            x = parent[x]
        return x

    def names(eid: str) -> List[str]:
        e = store.entities[eid]
        return [e.name] + list(e.aliases)

    def sparse(eid: str) -> bool:
        return len(tokens(store.entities[eid].name)) <= 1

    for eid in store.entities:
        parent[eid] = eid

    for block in block_index(store).values():
        if len(block) < 2:
            continue
        for i in range(len(block)):
            for j in range(i + 1, len(block)):
                a, b = block[i], block[j]
                ra, rb = find(a), find(b)
                if ra == rb:
                    continue
                score = pair_score(names(a), names(b))
                if score < threshold:
                    continue
                if (sparse(a) or sparse(b)) and score < 1.0:
                    continue  # sparsity floor: exact hit or nothing
                members_a = [m for m in parent if find(m) == ra]
                members_b = [m for m in parent if find(m) == rb]
                ok = True
                for m in members_a:
                    for n in members_b:
                        if m == a and n == b:
                            continue
                        if pair_score(names(m), names(n)) < floor:
                            ok = False
                            break
                    if not ok:
                        break
                if ok:
                    parent[ra] = rb

    clusters: Dict[str, List[str]] = {}
    for eid in store.entities:
        clusters.setdefault(find(eid), []).append(eid)
    proposals = []
    for members in clusters.values():
        if len(members) < 2:
            continue
        conf = min(pair_score(names(m), names(n))
                   for i, m in enumerate(members)
                   for n in members[i + 1:])
        proposals.append({"members": sorted(members), "confidence": round(conf, 3),
                          "reason": "verified-merge cluster (all-pairs floor held)"})
    return sorted(proposals, key=lambda p: str(p["members"]))


def apply_merges(store: EntityStore, proposals: List[Dict[str, object]],
                 validator: str, reason: str) -> List[str]:
    """Execute proposals with a named validator. Canonical = lowest id;
    losers become aliases of the canonical and are retracted with chains."""
    if not validator or not reason:
        raise IntelligenceError("applying merges requires validator + reason")
    canonical_ids = []
    for prop in proposals:
        members = sorted(str(m) for m in prop["members"])  # type: ignore[union-attr]
        canon = store.entities[members[0]]
        for loser_id in members[1:]:
            loser = store.entities[loser_id]
            canon.aliases = sorted(set(canon.aliases) | {loser.name.lower().strip()}
                                   | set(loser.aliases))
            store._log("merge-entities", {"canonical": canon.id, "absorbed": loser_id,
                                          "validator": validator, "reason": reason})
            store.retract(loser_id, validator=validator,
                          reason=f"duplicate of {canon.id} ({reason})")
            loser.supersedes = f"{loser.id}@v{loser.version - 1}>:{canon.id}"
            canon.version += 1
        canonical_ids.append(canon.id)
    return canonical_ids
