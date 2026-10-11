"""Ecosystem-intelligence entity store: gather -> curate -> store, phase 1.

Implements the "Now" phase of the market-intelligence capability: a governed
entity schema with deterministic-ID upsert, tiered validation, correction
chains, and versioned snapshots — all ledger-recorded. No network, no accounts.

Rules (fail-closed, TQI-model):
  - Machine-extracted input lands at T3 at most; T2 requires a named validator
    + primary-source URL; T1 requires a second independent source.
  - Identity is (kind, normalized name-or-alias); an alias claimed by two
    different names refuses (no silent merge).
  - No silent edits: every mutation bumps the version, chains `supersedes`,
    and appends to the ledger. Retraction preserves the id with a notice.
"""

from __future__ import annotations

import hashlib
import time
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

from kubona.epistemics.claim_grammar import EvidenceTier
from kubona.provenance import Ledger

ENTITY_KINDS = ("company", "investor", "round", "group", "patent",
                "program", "person")

REL_KINDS = ("invested_in", "partnered_with", "published", "affiliated_with",
             "funded_by")


class IntelligenceError(RuntimeError):
    """Raised on identity conflict, tier violation, or correction misuse."""


def _normalize(name: str) -> str:
    return " ".join(name.strip().lower().split())


def deterministic_id(kind: str, name: str) -> str:
    """Stable id from (kind, normalized name) — same input, same id."""
    digest = hashlib.sha1(f"{kind}:{_normalize(name)}".encode()).hexdigest()[:16]
    return f"{kind}-{digest}"


@dataclass
class Entity:
    id: str
    kind: str
    name: str
    aliases: List[str] = field(default_factory=list)
    tier: EvidenceTier = EvidenceTier.T3_SAMPLED
    provenance: Dict[str, Any] = field(default_factory=dict)
    version: int = 1
    supersedes: Optional[str] = None
    retracted: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        return {"id": self.id, "kind": self.kind, "name": self.name,
                "aliases": list(self.aliases), "tier": self.tier.value,
                "provenance": dict(self.provenance), "version": self.version,
                "supersedes": self.supersedes, "retracted": self.retracted}


@dataclass
class Relationship:
    id: str
    kind: str
    source_id: str
    target_id: str
    tier: EvidenceTier = EvidenceTier.T3_SAMPLED
    provenance: Dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> Dict[str, Any]:
        return {"id": self.id, "kind": self.kind, "source_id": self.source_id,
                "target_id": self.target_id, "tier": self.tier.value,
                "provenance": dict(self.provenance)}


class EntityStore:
    """In-memory governed store (SQLite migration later; JSON-serializable)."""

    def __init__(self, ledger: Optional[Ledger] = None):
        self.entities: Dict[str, Entity] = {}
        self.relationships: Dict[str, Relationship] = {}
        self._alias_index: Dict[str, str] = {}
        self._change_log: List[Dict[str, Any]] = []
        self._ledger = ledger

    def _log(self, kind: str, payload: Dict[str, Any]) -> None:
        entry = {"ts": time.time(), "event": kind, **payload}
        self._change_log.append(entry)
        if self._ledger is not None:
            self._ledger.append(f"intelligence-{kind}", payload)

    def upsert(self, kind: str, name: str,
               aliases: Optional[List[str]] = None,
               provenance: Optional[Dict[str, Any]] = None,
               tier: EvidenceTier = EvidenceTier.T3_SAMPLED,
               validator: Optional[str] = None) -> Entity:
        """Insert or merge an entity. Machine input must stay T3: anything
        above T3 requires a named validator (refused otherwise)."""
        if kind not in ENTITY_KINDS:
            raise IntelligenceError(f"unknown entity kind: {kind}")
        if tier != EvidenceTier.T3_SAMPLED and not validator:
            raise IntelligenceError(
                f"upsert at {tier.value} requires a named validator "
                "(machine input lands at T3 at most)")
        norm = _normalize(name)
        key = f"{kind}:{norm}"
        if key in self._alias_index:
            ent = self.entities[self._alias_index[key]]
            merged = sorted(set(ent.aliases) | {_normalize(a) for a in (aliases or [])})
            for alias in merged:
                owner = self._alias_index.get(f"{kind}:{alias}")
                if owner is not None and owner != ent.id:
                    raise IntelligenceError(
                        f"alias conflict: {alias!r} already identifies {owner}")
                self._alias_index[f"{kind}:{alias}"] = ent.id
            ent.aliases = merged
            ent.version += 1
            self._log("merge", {"id": ent.id, "version": ent.version})
            return ent
        for alias in [_normalize(a) for a in (aliases or [])]:
            owner = self._alias_index.get(f"{kind}:{alias}")
            if owner is not None:
                raise IntelligenceError(
                    f"alias conflict: {alias!r} already identifies {owner}")
        ent = Entity(id=deterministic_id(kind, name), kind=kind, name=name.strip(),
                     aliases=sorted({_normalize(a) for a in (aliases or [])}),
                     tier=tier, provenance=dict(provenance or {}))
        if validator:
            ent.provenance["validator"] = validator
        self.entities[ent.id] = ent
        self._alias_index[key] = ent.id
        for alias in ent.aliases:
            self._alias_index[f"{kind}:{alias}"] = ent.id
        self._log("upsert", {"id": ent.id, "kind": kind, "tier": tier.value})
        return ent

    def promote(self, entity_id: str, tier: EvidenceTier, validator: str,
                primary_source: Optional[str] = None) -> Entity:
        """T3->T2 needs validator + primary source; T2->T1 needs a second
        independent source recorded in provenance."""
        ent = self.entities[entity_id]
        order = [EvidenceTier.T3_SAMPLED, EvidenceTier.T2_CONSISTENT,
                 EvidenceTier.T1_VERIFIED]
        if order.index(tier) <= order.index(ent.tier):
            raise IntelligenceError("promotion must move up exactly the ladder "
                                    "(use correct() to revise down)")
        if order.index(tier) - order.index(ent.tier) > 1:
            raise IntelligenceError("no skipping tiers: validate T2 before T1")
        if not validator:
            raise IntelligenceError("promotion requires a named validator")
        if tier == EvidenceTier.T2_CONSISTENT and not primary_source:
            raise IntelligenceError("T2 promotion requires a primary-source URL")
        if tier == EvidenceTier.T1_VERIFIED:
            prior = ent.provenance.get("primary_source")
            if not primary_source or prior is None or _normalize(primary_source) == _normalize(prior):
                raise IntelligenceError(
                    "T1 promotion requires a second independent source")
        prev_version = ent.version
        ent.supersedes = f"{ent.id}@v{prev_version}"
        ent.version += 1
        ent.tier = tier
        ent.provenance["validator"] = validator
        if primary_source:
            if tier == EvidenceTier.T2_CONSISTENT:
                ent.provenance["primary_source"] = primary_source
            else:
                ent.provenance.setdefault("corroborating_sources", []).append(primary_source)
        self._log("promote", {"id": ent.id, "tier": tier.value,
                              "validator": validator})
        return ent

    def correct(self, entity_id: str, updates: Dict[str, Any], validator: str,
                reason: str) -> Entity:
        """Revise down or amend: bumps version, chains supersedes, logs reason.
        Never silent, never deletes."""
        ent = self.entities[entity_id]
        if not validator or not reason:
            raise IntelligenceError("correction requires validator + reason")
        ent.supersedes = f"{ent.id}@v{ent.version}"
        for field_name in ("name", "aliases", "provenance"):
            if field_name in updates:
                setattr(ent, field_name, updates[field_name])
        if "tier" in updates:
            ent.tier = updates["tier"]
        ent.version += 1
        self._log("correct", {"id": ent.id, "reason": reason,
                              "validator": validator})
        return ent

    def retract(self, entity_id: str, validator: str, reason: str) -> Entity:
        """Withdraw a record: id preserved with notice (TQI retraction shape)."""
        ent = self.entities[entity_id]
        if not validator or not reason:
            raise IntelligenceError("retraction requires validator + reason")
        ent.retracted = f"{reason} (by {validator})"
        ent.version += 1
        self._log("retract", {"id": ent.id, "reason": reason})
        return ent

    def link(self, kind: str, source_id: str, target_id: str,
             provenance: Optional[Dict[str, Any]] = None) -> Relationship:
        if kind not in REL_KINDS:
            raise IntelligenceError(f"unknown relationship kind: {kind}")
        if source_id not in self.entities or target_id not in self.entities:
            raise IntelligenceError("link endpoints must exist")
        rel_id = deterministic_id(f"rel-{kind}", f"{source_id}>{target_id}")
        rel = Relationship(id=rel_id, kind=kind, source_id=source_id,
                           target_id=target_id,
                           provenance=dict(provenance or {}))
        self.relationships[rel_id] = rel
        self._log("link", {"id": rel_id, "kind": kind})
        return rel

    def snapshot(self, label: str) -> Dict[str, Any]:
        """Versioned, citable export: records + methodology + update log."""
        snap = {"label": label, "taken_at": time.time(),
                "counts": {k: sum(1 for e in self.entities.values() if e.kind == k)
                           for k in ENTITY_KINDS},
                "relationships": len(self.relationships),
                "records": [e.to_dict() for e in self.entities.values()],
                "methodology": ("machine candidates at T3; T2 = named validator + "
                                "primary source; T1 = second independent source; "
                                "corrections chained, retractions preserved"),
                "update_log": list(self._change_log)}
        self._log("snapshot", {"label": label,
                               "entities": len(self.entities)})
        return snap
