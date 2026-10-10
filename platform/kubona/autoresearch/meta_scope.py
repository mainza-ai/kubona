"""Meta-loop write scope registry (Q-09).

The meta-loop may only mutate what this registry allows — and the registry
itself may only be widened by a trusted human admin signature, never by the
loop. Some scopes are unwidenable by design (compiler, MCP servers, security
layer, ledger schema): those require a code change with human review, never a
runtime grant, no matter who signs.

Mutable set (v1): mutation_radius, resample_prob, search_md_patch,
new_harness_draft — exactly the §13 write scope (strategy, instruction files,
harness definitions). Everything else refuses at propose time (fail-closed).
"""

from __future__ import annotations

import hashlib
import json
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

from kubona.config import state_path

SCOPE_VERSION = "meta-scope-v1"

# Default mutable set: the §13 write scope, nothing else.
DEFAULT_MUTABLE = frozenset({
    "mutation_radius",
    "resample_prob",
    "search_md_patch",
    "new_harness_draft",
})

# Unwidenable by design: no signature, however trusted, can admit these.
# Changing them is a code change with human review, full stop.
ALWAYS_IMMUTABLE = frozenset({
    "compiler",
    "mcp_servers",
    "security_layer",
    "ledger_schema",
    "cost_rail",
    "admission_policy",
})


class ScopeError(ValueError):
    """Raised when a kind is outside the meta-loop write scope."""


@dataclass
class MetaScope:
    """Versioned write-scope registry with a signed widening rule."""

    version: int = 1
    mutable: frozenset = field(default_factory=lambda: frozenset(DEFAULT_MUTABLE))
    trust_roots: tuple[str, ...] = ()  # admin pubkey hex allowed to widen
    history: list[dict[str, Any]] = field(default_factory=list)

    def allows(self, kind: str) -> bool:
        return kind in self.mutable

    def check(self, kind: str) -> None:
        """Fail closed on anything outside the mutable set."""
        if kind in ALWAYS_IMMUTABLE:
            raise ScopeError(
                f"scope {kind!r} is unwidenable by design (compiler/MCP/security/ledger "
                "change only via code review, never a runtime grant)")
        if kind not in self.mutable:
            raise ScopeError(
                f"scope {kind!r} outside meta-loop write scope v{self.version}; "
                f"mutable: {sorted(self.mutable)}")

    @staticmethod
    def canonical_payload(version: int, kind: str, expires_at: float) -> bytes:
        return f"meta-scope-widen:{version}:{kind}:{int(expires_at)}".encode()

    def widen(self, kind: str, authorizer_pubkey_hex: str, signature_hex: str,
              expires_at: float | None = None) -> "MetaScope":
        """Admit a new mutable kind. Requires a trusted admin signature over
        the canonical widening payload; unwidenable scopes refuse regardless
        of signer. Returns the new version (registries are immutable values)."""
        if kind in ALWAYS_IMMUTABLE:
            raise ScopeError(
                f"scope {kind!r} is unwidenable by design: refused for any signer")
        if kind in self.mutable:
            return self
        if authorizer_pubkey_hex not in self.trust_roots:
            raise ScopeError("widening requires a trusted admin key: refused")
        exp = expires_at if expires_at is not None else time.time() + 300.0
        if time.time() > exp:
            raise ScopeError("widening envelope expired: refused")
        try:
            from nacl.signing import VerifyKey
            vk = VerifyKey(bytes.fromhex(authorizer_pubkey_hex))
            vk.verify(self.canonical_payload(self.version, kind, exp),
                      bytes.fromhex(signature_hex))
        except Exception as e:
            raise ScopeError(f"widening signature invalid: {e}")
        entry = {"version": self.version + 1, "kind": kind,
                 "authorizer": authorizer_pubkey_hex[:16] + "…",
                 "at": time.time()}
        return MetaScope(version=self.version + 1,
                         mutable=self.mutable | {kind},
                         trust_roots=self.trust_roots,
                         history=[*self.history, entry])

    def digest(self) -> str:
        s = f"{SCOPE_VERSION}:{self.version}:{sorted(self.mutable)}:{sorted(self.trust_roots)}"
        return hashlib.sha256(s.encode()).hexdigest()[:16]

    def to_dict(self) -> dict[str, Any]:
        d = asdict(self)
        d["mutable"] = sorted(self.mutable)
        d["digest"] = self.digest()
        return d

    @classmethod
    def default(cls, trust_roots: tuple[str, ...] = ()) -> "MetaScope":
        return cls(trust_roots=trust_roots)

    def save(self, path: str | Path) -> Path:
        p = Path(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(json.dumps(self.to_dict(), indent=2))
        return p

    @classmethod
    def load(cls, path: str | Path) -> "MetaScope":
        d = json.loads(Path(path).read_text())
        return cls(version=d.get("version", 1),
                   mutable=frozenset(d.get("mutable", ())),
                   trust_roots=tuple(d.get("trust_roots", ())),
                   history=d.get("history", []))


def default_scope_path() -> Path:
    return state_path("meta-scope.json")
