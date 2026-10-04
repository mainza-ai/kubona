"""Structured experiment report builder — the 'organized, systematic results' contract.

A report keeps a `refs` list of (section, ledger_record_id | citation) so every
claim is one click from its ledger record. Buildable from a discovery campaign;
exportable to JSON / Markdown; credential-safe shareable.
"""

from __future__ import annotations


import uuid
from dataclasses import dataclass, field
from typing import Any

from kubona.config import state_path


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
    claims: list[dict[str, Any]] = field(default_factory=list)  # Epistemic claim assertions
    classical_anchor: dict[str, Any] | None = None

    def add_ref(self, section: str, ref: str) -> None:
        self.refs.append((section, ref))

    def add_claim(self, claim_text: str, epistemic_state: dict[str, Any]) -> None:
        self.claims.append({"claim": claim_text, "epistemic_state": epistemic_state})

    def to_dict(self) -> dict[str, Any]:
        return {"report_id": self.report_id, "title": self.title, "objectives": self.objectives,
                "methods": self.methods, "protocol": self.protocol, "results": self.results,
                "analysis": self.analysis, "conclusions": self.conclusions,
                "citations": self.citations, "how_to_reproduce": self.how_to_reproduce,
                "refs": self.refs, "claims": self.claims, "classical_anchor": self.classical_anchor}

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


class ReportStore:
    """Persists generated experiment notebooks (the results studio's data)."""

    def __init__(self, path: str | os.PathLike | None = None):
        from pathlib import Path as _P

        self.path = _P(path) if path else state_path("reports.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._reports: list[dict[str, Any]] = []
        if self.path.exists():
            import json as _json

            self._reports = _json.loads(self.path.read_text())

    def save(self, report: ExperimentReport) -> ExperimentReport:
        self._reports.append(report.to_dict())
        self._save()
        return report

    def all(self) -> list[dict[str, Any]]:
        return list(self._reports)

    def _save(self) -> None:
        import json as _json

        self.path.write_text(_json.dumps(self._reports, indent=2))
