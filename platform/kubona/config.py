"""Runtime configuration — single source of truth for state/ledger paths and ports.

Production posture: all mutable state (sessions, ledger, governance, identities,
agents, the algorithm library, reports, labs, workflows, settings, meta-proposals)
lives under ONE absolute state root that survives reboots (default: ~/.kubona,
not /tmp). Every module that persists to disk routes through these helpers so a
forwarded/injected path is honored first and the env-configurable default second.

Env overrides (all optional):
  KUBONA_STATE_DIR   base for everything (default: ~/.kubona)
  KUBONA_LEDGER      ledger file path (default: <state>/ledger.jsonl)
  KUBONA_CONSOLE_TOKEN  operator bearer token (default: none -> fail closed unless dev)
"""

from __future__ import annotations

import os
from pathlib import Path


def state_root() -> Path:
    """Absolute, resolved state root; created on first use.

    W0.13: refuses "/" (state scattered onto the filesystem root destroys
    it) and blank overrides (treated as unset, documented). An attacker or
    typo pointing state at "/" used to be honored with mkdir -p.
    """
    d = os.environ.get("KUBONA_STATE_DIR", "").strip()
    root = Path(d).expanduser() if d else Path.home() / ".kubona"
    root = root.resolve()
    if root == Path("/"):
        raise ValueError("KUBONA_STATE_DIR must not be the filesystem root: refusing")
    root.mkdir(parents=True, exist_ok=True)
    return root


def state_path(rel: str) -> Path:
    """A file under the state root (parents created)."""
    p = state_root() / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    return p


def ledger_path() -> Path:
    """The provenance ledger (system of record) — env-overridable.

    W0.13: refuses /dev/null (silent audit erasure) and existing
    directories (append would fail late and confusingly)."""
    d = os.environ.get("KUBONA_LEDGER", "").strip()
    if not d:
        return state_path("ledger.jsonl")
    p = Path(d).expanduser()
    if p == Path("/dev/null"):
        raise ValueError("KUBONA_LEDGER must not be /dev/null: the ledger is the system of record")
    if p.exists() and p.is_dir():
        raise ValueError(f"KUBONA_LEDGER {p} is a directory: refusing")
    return p


def is_production() -> bool:
    """KUBONA_ENV=production flips auth fail-closed (no dev-token fallback)."""
    return os.environ.get("KUBONA_ENV", "").strip().lower() == "production"


def console_token() -> str | None:
    """Operator bearer token for the console/gateway.

    Production posture: in KUBONA_ENV=production a token MUST be configured
    (KUBONA_CONSOLE_TOKEN); otherwise every request is unauthorized (fail closed).
    Outside production, the dev-token is the explicit local/CI default — but
    ONLY for explicit dev/test values or unset (W0.13): any other KUBONA_ENV
    spelling (e.g. a "prod" typo) used to silently enable god-mode. Unknown
    values fail closed.
    """
    t = os.environ.get("KUBONA_CONSOLE_TOKEN")
    if t:
        return t
    env = os.environ.get("KUBONA_ENV", "").strip().lower()
    if env in ("", "dev", "test"):
        return "dev-token"
    return None  # fail closed — refuse every request without a configured token


def signing_key_path() -> Path:
    """Where the OAuth signing key lives (seeded once, reused across restarts)."""
    return state_path("signing-key.pem")
