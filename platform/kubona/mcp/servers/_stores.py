"""Process-wide MCP stores.

Cost and execution must share one SessionStore: two instances loaded the same
JSON file at import, so a session created through cost was invisible to the
executor's in-memory copy (and vice versa). One store per process removes that
split-brain.
"""

from kubona.cost.governance import CostGovernanceService
from kubona.session import SessionStore

SESSIONS = SessionStore()
GOVERNANCE = CostGovernanceService(SESSIONS)
