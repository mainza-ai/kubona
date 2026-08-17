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
    """Absolute, resolved state root; created on first use."""
    d = os.environ.get("KUBONA_STATE_DIR")
    root = Path(d).expanduser() if d else Path.home() / ".kubona"
    root = root.resolve()
    root.mkdir(parents=True, exist_ok=True)
    return root


def state_path(rel: str) -> Path:
    """A file under the state root (parents created)."""
    p = state_root() / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    return p


def ledger_path() -> Path:
    """The provenance ledger (system of record) — env-overridable."""
    d = os.environ.get("KUBONA_LEDGER")
    return Path(d).expanduser() if d else state_path("ledger.jsonl")


def is_production() -> bool:
    """KUBONA_ENV=production flips auth fail-closed (no dev-token fallback)."""
    return os.environ.get("KUBONA_ENV", "").strip().lower() == "production"


def console_token() -> str | None:
    """Operator bearer token for the console/gateway.

    Production posture: in KUBONA_ENV=production a token MUST be configured
    (KUBONA_CONSOLE_TOKEN); otherwise every request is unauthorized (fail closed).
    Outside production, the dev-token is the explicit local/CI default.
    """
    t = os.environ.get("KUBONA_CONSOLE_TOKEN")
    if t:
        return t
    if is_production():
        return None  # fail closed — refuse every request without a configured token
    return "dev-token"


def signing_key_path() -> Path:
    """Where the OAuth signing key lives (seeded once, reused across restarts)."""
    return state_path("signing-key.pem")
