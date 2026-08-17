"""Control-plane backend (Phase 2): the loop supervisor the console watches.

The Dart console (cost/approval + circuit visualizer first, per \u00a75.3) talks
to this service over MCP: list running loops, read the current step, pause and
kill — the 'human watches and interrupts an in-flight hybrid loop' exit gate.
"""

from .supervisor import LoopSupervisor, LoopHandle

__all__ = ["LoopSupervisor", "LoopHandle"]
