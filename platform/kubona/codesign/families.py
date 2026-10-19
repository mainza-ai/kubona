"""Ideation families + elite sharing + widen-nudge (paper's anti-caution fix).

N qualitatively-distinct families mature separately, then share elites
(cross-pollination). Stagnation is measured, never vibes: offers since
last cell-improvement per family. LLM proposals are an explicit callable;
the deterministic mutator (no network) is the test path. The NVIDIA
proposer refuses fail-closed without a key — oMLX/OpenCode are parked
during development and are never called here.
"""

from __future__ import annotations

import os
from typing import Any, Callable

from .frontier import MapElitesFrontier

PARKED_PROVIDERS = frozenset({"omlx", "opencode", "ollama", "lm-studio", "mlx", "llama.cpp"})


class FamilySearch:
    """N frontiers + lineage + stagnation. Deterministic, ledger-friendly."""

    def __init__(self, families: list[str], kappa: int = 1):
        if not families or len(set(families)) != len(families) or any(
                not isinstance(f, str) or not f.strip() for f in families):
            raise ValueError("families must be distinct non-blank names: refusing")
        self.families = list(families)
        self.frontiers = {f: MapElitesFrontier(kappa=kappa) for f in families}
        self.lineage: list[dict[str, Any]] = []
        self._stagnant: dict[str, int] = {f: 0 for f in families}

    def offer(self, family: str, artifact: Any, toffoli: int, qubits: int,
              parent: str | None = None) -> bool:
        if family not in self.frontiers:
            raise ValueError(f"unknown family {family!r}: refusing")
        improved = self.frontiers[family].offer(artifact, toffoli, qubits,
                                                {"family": family, "parent": parent})
        entry = {"family": family, "toffoli": toffoli, "qubits": qubits,
                 "improved": improved, "parent": parent}
        self.lineage.append(entry)
        self._stagnant[family] = 0 if improved else self._stagnant[family] + 1
        return improved

    def elites(self) -> dict[str, Any]:
        """Best entry per family (cross-pollination pool)."""
        out: dict[str, Any] = {}
        for f, fr in self.frontiers.items():
            b = fr.best()
            if b is not None:
                out[f] = {"toffoli": b.toffoli, "qubits": b.qubits,
                          "tq": b.tq, "artifact": b.artifact}
        return out

    def share_elites(self) -> dict[str, Any]:
        """One cross-pollination round (paper Fig-overview(a)): offer every
        family's best artifact into every OTHER family's frontier.

        Deterministic (sorted family order); each cross-offer records lineage
        with parent="x-poll:<src-family>" so the idea's origin is auditable.
        Returns {offers, improvements, details}. Fail-closed on empty elites
        (nothing to share -> zero offers, never a fake improvement).
        """
        elites = self.elites()
        if not elites:
            return {"offers": 0, "improvements": 0, "details": []}
        details: list[dict[str, Any]] = []
        improvements = 0
        offers = 0
        for dst in sorted(self.frontiers):
            for src in sorted(elites):
                if src == dst:
                    continue
                e = elites[src]
                offers += 1
                improved = self.offer(dst, e["artifact"], e["toffoli"],
                                      e["qubits"], parent=f"x-poll:{src}")
                details.append({"from": src, "to": dst, "improved": improved,
                                "toffoli": e["toffoli"], "qubits": e["qubits"]})
                if improved:
                    improvements += 1
        return {"offers": offers, "improvements": improvements, "details": details}

    def widen_nudge(self) -> dict[str, Any]:
        """Which family most needs a widen nudge (max offers-since-improvement)."""
        fam = max(self._stagnant, key=lambda f: self._stagnant[f])
        return {"family": fam, "offers_since_improvement": self._stagnant[fam],
                "nudge": f"widen {fam}: propose a qualitatively-distinct rewrite, not a parameter tweak"}

    def to_dict(self) -> dict[str, Any]:
        return {"families": self.families, "stagnation": dict(self._stagnant),
                "elites": self.elites(), "lineage_len": len(self.lineage)}


def deterministic_mutator(artifact: list, step: int) -> list:
    """No-network proposer for tests: toggles a trailing marker op deterministically."""
    if not isinstance(artifact, list):
        raise ValueError("artifact must be a list: refusing")
    out = [tuple(t) for t in artifact]
    # deterministic, kickmix-legal tweak: append x(0) on even steps, drop it on odd
    if step % 2 == 0:
        out.append(("x", (0,), ()))
    elif out and out[-1][0] == "x":
        out.pop()
    return out


class NvidiaProposer:
    """Production proposer hook: NVIDIA NIM only, fail-closed without key.

    oMLX/OpenCode (and other local shims) are parked: requesting them here
    refuses instead of silently routing to a parked engine.
    """

    def __init__(self, provider: str = "nvidia", model: str | None = None,
                 base_url: str | None = None):
        if provider in PARKED_PROVIDERS:
            raise ValueError(
                f"provider {provider!r} is parked during development: refusing "
                "(use nvidia for LLM inference)")
        if provider != "nvidia":
            raise ValueError(f"proposer supports nvidia only, got {provider!r}: refusing")
        self.provider = provider
        self.model = model or os.environ.get("NVIDIA_MODEL", "")
        self.base_url = base_url or "https://integrate.api.nvidia.com/v1"
        if not os.environ.get("NVIDIA_API_KEY"):
            raise ValueError("NVIDIA_API_KEY absent: refusing LLM proposal (deterministic path only)")

    def propose(self, prompt: str) -> str:
        raise NotImplementedError(
            "live NVIDIA proposal is a network call: wire via get_llm() at the "
            "search-loop edge with ledger sealing, never inside unit tests")
