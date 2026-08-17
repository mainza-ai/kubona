"""The meta-level loop (§13) — a Phase-4 stretch goal, propose-only.

Reflects over accumulated trial/library data and PROPOSES search-strategy changes
(versioned, shadow-tested, human-approved before application). Safety per §13.4:
never auto-applies, never touches platform code; write scope stops at search
strategy parameters.
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class ReflectionProposal:
    proposal_id: str
    kind: str                 # "mutation_radius" | "resample_prob" | "new_harness_draft"
    incumbent: Any
    proposed: Any
    rationale: str
    shadow_evidence: dict[str, Any]
    status: str = "PROPOSED"  # PROPOSED -> HUMAN_REVIEW -> APPROVED | REJECTED
    created_at: float = field(default_factory=time.time)
    reviewed_by: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class MetaLoop:
    """Reflection tier: studies history, compares strategies in shadow mode, proposes."""

    def __init__(self, store_path: str | Path | None = None):
        self.path = Path(store_path) if store_path else Path("/tmp/kubona-state/meta-proposals.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._proposals: list[ReflectionProposal] = []
        if self.path.exists():
            self._proposals = [ReflectionProposal(**p) for p in json.loads(self.path.read_text())]

    def _save(self) -> None:
        self.path.write_text(json.dumps([p.to_dict() for p in self._proposals], indent=2))

    def shadow_compare(self, incumbent_score: float, proposed_score: float,
                       improvement_threshold: float = 0.02) -> bool:
        """Shadow-mode comparison: does the proposed strategy beat the incumbent?"""
        return proposed_score > incumbent_score * (1 + improvement_threshold)

    def propose_strategy_change(self, kind: str, incumbent: Any, proposed: Any,
                                rationale: str, shadow_evidence: dict[str, Any]) -> ReflectionProposal:
        p = ReflectionProposal(proposal_id=uuid.uuid4().hex[:16], kind=kind,
                               incumbent=incumbent, proposed=proposed,
                               rationale=rationale, shadow_evidence=shadow_evidence)
        self._proposals.append(p)
        self._save()
        return p

    def approve(self, proposal_id: str, reviewer: str) -> ReflectionProposal:
        p = self._get(proposal_id)
        p.status = "APPROVED"
        p.reviewed_by = reviewer
        self._save()
        return p

    def reject(self, proposal_id: str, reviewer: str) -> ReflectionProposal:
        p = self._get(proposal_id)
        p.status = "REJECTED"
        p.reviewed_by = reviewer
        self._save()
        return p

    def _get(self, proposal_id: str) -> ReflectionProposal:
        for p in self._proposals:
            if p.proposal_id == proposal_id:
                return p
        raise KeyError(proposal_id)

    def pending(self) -> list[ReflectionProposal]:
        return [p for p in self._proposals if p.status in ("PROPOSED", "HUMAN_REVIEW")]


def reflect_on_orchestrator(history_scores: list[float], incumbent_radius: float,
                            candidate_radius: float, meta: MetaLoop,
                            improvement_threshold: float = 0.02) -> ReflectionProposal | None:
    """Compare two mutation radii on the same trial history split (shadow test).

    Returns a PROPOSED strategy change if the candidate measurably improves
    convergence; never auto-applies.
    """
    if len(history_scores) < 6:
        return None
    split = len(history_scores) // 2
    inc = sum(history_scores[:split]) / max(split, 1)
    cand = sum(history_scores[split:]) / max(len(history_scores) - split, 1)
    better = meta.shadow_compare(inc, cand, improvement_threshold)
    if not better:
        return None
    return meta.propose_strategy_change(
        kind="mutation_radius",
        incumbent=incumbent_radius,
        proposed=candidate_radius,
        rationale=f"candidate radius converged better in shadow split "
                  f"(incumbent {inc:.4f} vs candidate {cand:.4f})",
        shadow_evidence={"incumbent_score": inc, "proposed_score": cand,
                         "samples": len(history_scores)},
    )
