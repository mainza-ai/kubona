"""Identity groundwork (rev 3, \u00a714.2) — Phase 2.

Keypair identities for users and agents, with owner-delegated authority
(the Buzz NIP-OA pattern): an agent inherits its owner's access; revoking the
owner revokes every agent they authorized — instantly, no per-agent surgery.
"""

from .registry import IdentityRegistry, Identity, create_identity, Delegation

__all__ = ["IdentityRegistry", "Identity", "create_identity", "Delegation"]
