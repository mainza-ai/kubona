"""Circuit cutting (Phase 2) — cut wide/deep circuits into smaller subcircuits.

Reference implementation: LOCC wire cutting of a CNOT for Z-diagonal target
expectation values, numerically verified against the uncut simulator. The
production path (Qiskit Circuit Knitting Toolbox) is Phase-2 hardening; this
module fixes the semantics and the fragment shapes.
"""

from .wire import cut_expectation, cut_histogram, plan_cut

__all__ = ["cut_expectation", "cut_histogram", "plan_cut"]
