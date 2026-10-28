"""Security hardening (Phase 3): hybrid PQC transport, per-call identity-aware
authorization, immutable audit.

Matches wiki/concepts/security-and-pqc: OAuth 2.1 + scoped short-lived tokens,
per-call identity-aware auth (not coarse roles), hybrid post-quantum transport
(ML-KEM alongside classical TLS), cryptographic agility (swappable suites), and
an immutable audit log that doubles as the cost ledger.
"""

from .hybrid_transport import (HYBRID_SUITE, HybridKeyPair, HybridSession,
                                 handshake_initiator, handshake_responder)
from .policy import PolicyEngine, TOOL_SCOPES
from .tenant import (GRANT_DECISION, CrossTenantRefusal, caller_from_principal,
                     principal_tenant, resolve_tenant)

__all__ = ["HybridKeyPair", "HybridSession", "HYBRID_SUITE", "PolicyEngine", "TOOL_SCOPES",
           "CrossTenantRefusal", "GRANT_DECISION", "caller_from_principal",
           "principal_tenant", "resolve_tenant"]
