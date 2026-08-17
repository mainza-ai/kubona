"""Persistent user settings (the Settings menu's backend)."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from kubona.llm.registry import discovery_defaults

SETTINGS_PATH = Path("/tmp/kubona-state/settings.json")


class SettingsStore:
    def __init__(self, path: str | Path | None = None):
        self.path = Path(path) if path else SETTINGS_PATH
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._data: dict[str, Any] = {}
        if self.path.exists():
            try:
                self._data = json.loads(self.path.read_text())
            except Exception:  # noqa: BLE001 — corrupt settings -> start clean
                self._data = {}

    def get(self, key: str, default: Any = None) -> Any:
        return self._data.get(key, default)

    def set(self, key: str, value: Any) -> None:
        self._data[key] = value
        self._save()

    def update(self, patch: dict[str, Any]) -> dict[str, Any]:
        secret_keys = {"llm_api_key"}
        for k, v in patch.items():
            if k in secret_keys and not v:
                continue  # never blank out a stored secret with an empty value
            self._data[k] = v
        self._save()
        return self.public()

    def public(self) -> dict[str, Any]:
        """Settings safe to send to the console: user choices plus a SEPARATE live
        host-model snapshot (never clobbers the configured llm_provider/model)."""
        out = {k: v for k, v in self._data.items()}
        if "llm_api_key" in out:
            out["llm_api_key"] = "********"
        d = discovery_defaults()  # live host model view (read-only)
        out["host_available"] = d.get("llm_available")
        out["host_provider"] = d.get("llm_provider")
        out["host_model"] = d.get("llm_model")
        out["scan_ports"] = d.get("scan_ports")
        return out

    def apply_llm_edge(self, patch: dict[str, Any]) -> dict[str, Any]:
        return self.update(patch)

    def _save(self) -> None:
        self.path.write_text(json.dumps(self._data, indent=2))
