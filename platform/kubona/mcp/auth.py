"""Gateway authentication boundary (re-exports from kubona.core.auth for backwards compatibility)."""

from __future__ import annotations

from kubona.core.auth import AuthError, AuthProvider, DevAuthProvider, Principal

__all__ = [
    "AuthError",
    "AuthProvider",
    "DevAuthProvider",
    "Principal",
]
