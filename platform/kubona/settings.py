"""Persistent user settings (the Settings menu's backend)."""

from __future__ import annotations

import json
import time
from pathlib import Path
from typing import Any

from kubona.config import atomic_write_text, state_path
from kubona.llm.registry import discovery_defaults

#: Secret-bearing settings keys — the SINGLE shared const (W2.17). Every
#: secret path in this module (blank-guard in update/set, masking in public)
#: reads this set; no second list may exist.
SECRET_KEYS = frozenset({"llm_api_key"})

#: Settings safe to expose to the console (W2.17, allowlist — never a
#: denylist: a newly added secret-bearing key is hidden by default until
#: deliberately listed here). Secret members render masked, never raw.
PUBLIC_SETTINGS = frozenset({"llm_provider", "llm_base_url", "llm_model", "llm_api_key"})

#: Keys the LLM edge may write (W2.17): schema-validated in apply_llm_edge.
LLM_EDGE_KEYS = frozenset({"llm_provider", "llm_base_url", "llm_model", "llm_api_key"})


def _scrub(value: Any, key: str) -> Any:
    """Strip whitespace (W2.17). Whitespace-only secrets are treated as
    blank (never stored); whitespace-only non-secrets refuse at the edge."""
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped and key not in SECRET_KEYS:
            raise ValueError(f"setting {key!r} must not be blank or whitespace-only: refusing")
        return stripped
    return value


class SettingsStore:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else state_path("settings.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._data: dict[str, Any] = {}
        if self.path.exists():
            try:
                loaded = json.loads(self.path.read_text())
            except Exception as e:  # noqa: BLE001 — corrupt file quarantined LOUDLY
                self._quarantine(f"not valid JSON: {e}")
            if not isinstance(loaded, dict):
                self._quarantine(f"top-level JSON is {type(loaded).__name__}, not an object")
            else:
                self._data = loaded

    def _quarantine(self, reason: str) -> None:
        """Move the corrupt file aside and REFUSE (W2.17, fail-closed): a
        silent reset to {} would drop the operator's provider config and
        could flip the LLM path without anyone noticing."""
        backup = self.path.with_name(
            f"{self.path.name}.corrupt-{int(time.time())}")
        try:
            self.path.rename(backup)
        except OSError as e:
            raise ValueError(
                f"settings file {self.path} is corrupt ({reason}) and could "
                f"not be quarantined: {e} — refusing to start clean") from e
        raise ValueError(
            f"settings file {self.path} was corrupt ({reason}): quarantined "
            f"to {backup} — restore or delete it explicitly, refusing silent reset")

    def get(self, key: str, default: Any = None) -> Any:
        return self._data.get(key, default)

    def set(self, key: str, value: Any) -> None:
        value = _scrub(value, key)
        if key in SECRET_KEYS and not value:
            return  # never blank out a stored secret with an empty value
        self._data[key] = value
        self._save()

    def update(self, patch: dict[str, Any]) -> dict[str, Any]:
        for k, v in patch.items():
            v = _scrub(v, k)
            if k in SECRET_KEYS and not v:
                continue  # never blank out a stored secret with an empty value
            self._data[k] = v
        self._save()
        return self.public()

    def public(self) -> dict[str, Any]:
        """Settings safe to send to the console: allowlisted user choices
        (W2.17 — secrets masked, unlisted keys never leave the store) plus a
        SEPARATE live host-model snapshot (never clobbers the configured
        llm_provider/model)."""
        out = {k: v for k, v in self._data.items() if k in PUBLIC_SETTINGS}
        for k in SECRET_KEYS & PUBLIC_SETTINGS:
            if k in out:
                out[k] = "********"
        d = discovery_defaults()  # live host model view (read-only)
        out["host_available"] = d.get("llm_available")
        out["host_provider"] = d.get("llm_provider")
        out["host_model"] = d.get("llm_model")
        out["scan_ports"] = d.get("scan_ports")
        return out

    def apply_llm_edge(self, patch: dict[str, Any]) -> dict[str, Any]:
        """LLM settings edge with schema validation (W2.17).

        CALLER REQUIREMENT (Writer scope): this method cannot verify identity
        itself — the console MUST expose it only to Writer-scope principals
        (see console_api.settings). A Reader- or unauthenticated-reachable
        edge here would let anyone repoint the platform's LLM or blank its
        credentials.

        Refuses: unknown keys, unknown providers (must name MODEL_SOURCES),
        malformed base URLs, blank provider/model values. Blank secrets are
        ignored (never blank the stored credential).
        """
        from kubona.llm.factory import MODEL_SOURCES

        for k in patch:
            if k not in LLM_EDGE_KEYS:
                raise ValueError(
                    f"llm edge refuses unknown key {k!r} "
                    f"(allowed: {sorted(LLM_EDGE_KEYS)})")
        provider = patch.get("llm_provider")
        if "llm_provider" in patch:
            provider = _scrub(provider, "llm_provider")
            if provider not in MODEL_SOURCES:
                raise ValueError(
                    f"llm edge refuses unknown provider {provider!r} "
                    f"(known: {sorted(MODEL_SOURCES)})")
        if "llm_base_url" in patch:
            base = _scrub(patch["llm_base_url"], "llm_base_url")
            if not (base.startswith("http://") or base.startswith("https://")):
                raise ValueError(
                    f"llm edge refuses malformed base_url {base!r} "
                    "(must start with http:// or https://)")
        if "llm_model" in patch:
            _scrub(patch["llm_model"], "llm_model")  # refuses blank
        return self.update(patch)

    def _save(self) -> None:
        atomic_write_text(self.path, json.dumps(self._data, indent=2))
