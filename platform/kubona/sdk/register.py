"""Provider registration SDK — the documented plugin API (Phase-4 gate).

register_provider(capability_json, plugin_path, sandbox_test=True):
  1. validates the capability schema (pydantic)
  2. verifies the plugin imports and exposes the Provider trait
  3. runs a sandboxed test submission (local simulator only — no real hardware)
  4. writes the capability entry to schema/providers/ (a data change, not a code change)

Contribution governance (docs/contribution-governance.md): a merged third-party
provider validates the extensibility model (Phase-4 gate).
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from kubona.circuits import Circuit, Gate
from kubona.providers.base import Provider, ProviderError, _instantiate
from kubona.schema import ProviderCapability, SCHEMA_DIR

_SANDBOX_CIRCUIT = Circuit(name="sdk-sandbox", num_qubits=1, gates=[Gate("h", (0,))])


def validate_capability(capability_json: dict[str, Any]) -> ProviderCapability:
    """Strict schema validation (pydantic) — fail fast on bad data."""
    return ProviderCapability.model_validate(capability_json)


def register_provider(capability_json: dict[str, Any], plugin_path: str,
                      sandbox_test: bool = True, write: bool = True) -> dict[str, Any]:
    """Validate + smoke-test + (optionally) write a new provider entry."""
    cap = validate_capability(capability_json)
    cap.execution_plugin = plugin_path

    # 1. plugin must exist and satisfy the trait
    try:
        provider = _instantiate(plugin_path, cap)
    except Exception as e:  # noqa: BLE001
        raise ProviderError(f"plugin {plugin_path!r} failed to load: {e}") from e
    if not isinstance(provider, Provider):
        raise ProviderError(f"plugin {plugin_path!r} does not implement the Provider trait")

    # 2. sandboxed smoke test (simulator tier only; no credentials, no real HW)
    smoke: dict[str, Any] | None = None
    if sandbox_test:
        try:
            result = provider.submit(_SANDBOX_CIRCUIT, shots=256, seed=1)
            smoke = {"ok": True, "counts_keys": sorted(result.get("counts", {})),
                     "shots": result.get("shots")}
        except ProviderError as e:
            smoke = {"ok": False, "error": str(e)}  # optional-deps plugins may fail closed

    # 3. write the capability entry (data change)
    written = False
    if write:
        path = SCHEMA_DIR / f"{cap.provider}.json"
        path.write_text(json.dumps(cap.model_dump(), indent=2))
        written = True

    return {"provider": cap.provider, "status": cap.status, "plugin": plugin_path,
            "smoke": smoke, "written": written, "schema_version": "1"}
