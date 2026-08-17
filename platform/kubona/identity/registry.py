"""Ed25519 identities + delegation attestations (Phase 2 groundwork).

The OAuth/PQC layer (Phase 3) authenticates at the transport; this registry is
the *identity* layer beneath it: who is who, who delegated what to which agent,
and whether a delegation is still live.
"""

from __future__ import annotations

import json
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from cryptography.hazmat.primitives.asymmetric.ed25519 import (
    Ed25519PrivateKey, Ed25519PublicKey,
)
from cryptography.hazmat.primitives import serialization
from cryptography.exceptions import InvalidSignature


@dataclass(frozen=True)
class Principal:
    sub: str          # identity id (person or agent)
    kind: str         # "user" | "agent"
    pubkey_hex: str
    owner: str | None = None   # for agents: the delegating user

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


@dataclass(frozen=True)
class Delegation:
    delegation_id: str
    owner: str
    agent: str
    scopes: tuple[str, ...]
    issued_at: float
    expires_at: float | None
    revoked: bool = False

    def is_live(self, now: float | None = None) -> bool:
        now = now or time.time()
        return not self.revoked and (self.expires_at is None or now < self.expires_at)


class IdentityRegistry:
    """Persistent registry of principals and delegations (JSON file)."""

    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else Path("/tmp/kubona-state/identity.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._principals: dict[str, Principal] = {}
        self._delegations: dict[str, Delegation] = {}
        self._load()

    # -- persistence ------------------------------------------------------
    def _load(self) -> None:
        if self.path.exists():
            data = json.loads(self.path.read_text())
            self._principals = {k: Principal(**v) for k, v in data.get("principals", {}).items()}
            self._delegations = {k: Delegation(**v) for k, v in data.get("delegations", {}).items()}

    def _save(self) -> None:
        data = {
            "principals": {k: p.to_dict() for k, p in self._principals.items()},
            "delegations": {k: asdict(d) for k, d in self._delegations.items()},
        }
        self.path.write_text(json.dumps(data, indent=2, sort_keys=True))

    # -- principals -------------------------------------------------------
    def register(self, sub: str, kind: str, pubkey_hex: str, owner: str | None = None) -> Principal:
        p = Principal(sub=sub, kind=kind, pubkey_hex=pubkey_hex, owner=owner)
        self._principals[sub] = p
        self._save()
        return p

    def get(self, sub: str) -> Principal | None:
        return self._principals.get(sub)

    def agents_of(self, owner: str) -> list[Principal]:
        return [p for p in self._principals.values() if p.owner == owner]

    # -- delegations ------------------------------------------------------
    def delegate(self, owner: str, agent: str, scopes: tuple[str, ...],
                 expires_at: float | None = None) -> Delegation:
        if self.get(owner) is None or self.get(agent) is None:
            raise KeyError("owner and agent must both be registered")
        d = Delegation(delegation_id=f"{owner}:{agent}", owner=owner, agent=agent,
                       scopes=scopes, issued_at=time.time(), expires_at=expires_at)
        self._delegations[d.delegation_id] = d
        self._save()
        return d

    def revoke(self, delegation_id: str) -> Delegation:
        d = self._delegations[delegation_id]
        d = Delegation(**{**asdict(d), "revoked": True})
        self._delegations[delegation_id] = d
        self._save()
        return d

    def revoke_all_for(self, owner: str) -> list[str]:
        """Revoke every delegation owned by `owner` — instant cascade."""
        ids = [d.delegation_id for d in self._delegations.values()
               if d.owner == owner and not d.revoked]
        for i in ids:
            self.revoke(i)
        return ids

    def access(self, agent: str) -> tuple[str, ...] | None:
        """Effective scopes for an agent via a live owner delegation."""
        p = self.get(agent)
        if p is None or p.kind != "agent" or p.owner is None:
            return None
        d = self._delegations.get(f"{p.owner}:{agent}")
        return d.scopes if d is not None and d.is_live() else None

    def verify_signature(self, sub: str, message: bytes, signature: bytes) -> bool:
        p = self.get(sub)
        if p is None:
            return False
        try:
            pub = Ed25519PublicKey.from_public_bytes(bytes.fromhex(p.pubkey_hex))
            pub.verify(signature, message)
            return True
        except (InvalidSignature, ValueError):
            return False


def create_identity() -> tuple[str, str]:
    """Create a fresh Ed25519 keypair; returns (pubkey_hex, private_key_pem)."""
    priv = Ed25519PrivateKey.generate()
    pub_hex = priv.public_key().public_bytes(
        serialization.Encoding.Raw, serialization.PublicFormat.Raw).hex()
    pem = priv.private_bytes(
        serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption()).decode()
    return pub_hex, pem
