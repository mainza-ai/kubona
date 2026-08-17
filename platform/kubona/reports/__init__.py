"""Experiment notebook & results studio (Phase 6 / §14.4).

Every experiment yields a structured report (objectives · methods · protocol ·
results · analysis · conclusions · citations · how-to-reproduce); every section
links its ledger record / citation so each claim is provenance-traceable.
"""

from .notebook import ExperimentReport, build_report, ReportStore

__all__ = ["ExperimentReport", "build_report", "ReportStore"]
