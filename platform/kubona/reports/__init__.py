"""Experiment notebook & results studio (Phase 6 / §14.4).

Every experiment yields a structured report (objectives · methods · protocol ·
results · analysis · conclusions · citations · how-to-reproduce); every section
links its ledger record / citation so each claim is provenance-traceable.
"""

from .notebook import ExperimentReport, ReportStore, build_report
from kubona.science.manuscript import (
    ManuscriptGenerator,
    ManuscriptSection,
    PeerReviewReport,
    ScientificManuscript,
)

__all__ = [
    "ExperimentReport",
    "ManuscriptGenerator",
    "ManuscriptSection",
    "PeerReviewReport",
    "ReportStore",
    "ScientificManuscript",
    "build_report",
]

