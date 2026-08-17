"""Structured experiment report builder — the 'organized, systematic results' contract.

A report keeps a `refs` list of (section, ledger_record_id | citation) so every
claim is one click from its ledger record. Buildable from a discovery campaign;
exportable to JSON / Markdown; credential-safe shareable.
"""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from typing import Any


@dataclass
class ExperimentReport:
    report_id: str = field(default_factory=lambda: uuid.uuid4().hex[:16])
    title: str = ""
    objectives: str = ""
    methods: str = ""
    protocol: str = ""
    results: str = ""
    analysis: str = ""
    conclusions: str = ""
    citations: list[str] = field(default_factory=list)
    how_to_reproduce: str = ""
    refs: list[tuple[str, str]] = field(default_factory=list)  # (section, ledger_ref|citation)

    def add_ref(self, section: str, ref: str) -> None:
        self.refs.append((section, ref))

    def to_dict(self) -> dict[str, Any]:
        return {"report_id": self.report_id, "title": self.title, "objectives": self.objectives,
                "methods": self.methods, "protocol": self.protocol, "results": self.results,
                "analysis": self.analysis, "conclusions": self.conclusions,
                "citations": self.citations, "how_to_reproduce": self.how_to_reproduce,
                "refs": self.refs}

    def to_markdown(self) -> str:
        return (
            f"# {self.title or 'Experiment report'}\n\n"
            f"## Objectives\n{self.objectives}\n"
            f"## Methods\n{self.methods}\n"
            f"## Protocol\n{self.protocol}\n"
            f"## Results\n{self.results}\n"
            f"## Analysis\n{self.analysis}\n"
            f"## Conclusions\n{self.conclusions}\n"
            f"## Citations\n" + ("; ".join(self.citations) if self.citations else "(none)") + "\n"
            f"## How to reproduce\n{self.how_to_reproduce}\n"
            f"\n_Every claim above links to a ledger record; `refs`: " + ", ".join(
                f"{s}->{r}" for s, r in self.refs) + "_"
        )


def build_report(**kwargs: Any) -> ExperimentReport:
    return ExperimentReport(**{k: v for k, v in kwargs.items() if v is not None})
