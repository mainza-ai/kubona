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

from kubona.config import state_path
from typing import Any, TYPE_CHECKING

if TYPE_CHECKING:
    from kubona.autoresearch.meta_scope import MetaScope


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

    def __init__(self, store_path: str | Path | None = None, scope: MetaScope | None = None):
        from kubona.autoresearch.meta_scope import MetaScope as _DefaultScope
        self.scope = scope or _DefaultScope.default()
        self.path = Path(store_path) if store_path else state_path("meta-proposals.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._proposals: list[ReflectionProposal] = []
        if self.path.exists():
            self._proposals = [ReflectionProposal(**p) for p in json.loads(self.path.read_text())]

    def _save(self) -> None:
        self.path.write_text(json.dumps([p.to_dict() for p in self._proposals], indent=2))

    def shadow_compare(self, incumbent_score: float, proposed_score: float,
                       improvement_threshold: float = 0.02) -> bool:
        """Shadow-mode comparison: does the proposed strategy beat the incumbent?

        Relative improvement is measured on the true score axis: for a
        negative incumbent, multiplying by (1+threshold) makes the bar MORE
        negative, silently demanding a larger gain. Require
        proposed - incumbent > |incumbent| * threshold instead.
        """
        gain = proposed_score - incumbent_score
        required = abs(incumbent_score) * improvement_threshold if incumbent_score != 0 else improvement_threshold
        return gain > required

    def propose_strategy_change(self, kind: str, incumbent: Any, proposed: Any,
                                 rationale: str, shadow_evidence: dict[str, Any]) -> ReflectionProposal:
        self.scope.check(kind)  # Q-09: fail closed outside the write scope
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

    # -- Phase-5: staged auto-application (§13.5) --------------------------
    def auto_apply_candidate(self, proposal_id: str, *, shadow_scores_incumbent: list[float],
                             shadow_scores_proposed: list[float],
                             safety_period_elapsed: bool,
                             min_shadow_runs: int = 4,
                             improvement_threshold: float = 0.02) -> ReflectionProposal:
        """Staged rollout: auto-apply an APPROVED proposal ONLY after (a) a safety
        period has elapsed AND (b) it won in shadow mode across >= min_shadow_runs.
        Otherwise the proposal stays APPROVED-but-not-applied (rolled back to review)."""
        p = self._get(proposal_id)
        if p.status != "APPROVED":
            raise ValueError(f"proposal {proposal_id} is {p.status}, not APPROVED")
        if not safety_period_elapsed:
            raise ValueError("safety period not elapsed — staging window open")
        if len(shadow_scores_incumbent) < min_shadow_runs or len(shadow_scores_proposed) < min_shadow_runs:
            raise ValueError("too few shadow runs for auto-application")
        inc = sum(shadow_scores_incumbent) / len(shadow_scores_incumbent)
        prop = sum(shadow_scores_proposed) / len(shadow_scores_proposed)
        if not self.shadow_compare(inc, prop, improvement_threshold):
            raise ValueError(
                f"shadow mode shows no improvement (incumbent {inc:.4f} vs proposed {prop:.4f}) — "
                "NOT auto-applying; staged rollout aborts")
        p = ReflectionProposal(**{**asdict(p), "status": "APPLIED",
                                  "shadow_evidence": {**p.shadow_evidence,
                                                      "auto_apply": {"incumbent": inc, "proposed": prop,
                                                                     "shadow_runs": len(shadow_scores_proposed)}}})
        self._replace(p)
        return p

    def _replace(self, proposal: ReflectionProposal) -> None:
        for i, existing in enumerate(self._proposals):
            if existing.proposal_id == proposal.proposal_id:
                self._proposals[i] = proposal
                break
        self._save()


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
