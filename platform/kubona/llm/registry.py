"""Local model discovery : AUTO-DETECT the port a local LLM serves on.

Probes a range of common local-LLM ports (not just defaults) for an
OpenAI-compatible (`/v1/models`) or Ollama-native (`/api/tags`) endpoint, classifies
the provider, reads the models, and caches the result. Overridable via
`KUBONA_LLM_PORTS` (comma list) and `KUBONA_LLM_BASE_URL` (explicit endpoint).
Also honors the `lms` CLI (LM Studio) when present for the exact endpoint.
"""

from __future__ import annotations

import os
import shutil
import subprocess
from typing import Any

import httpx

from .base import ModelInfo

DEFAULT_PORTS = [11434, 8787, 1234, 8080, 8000, 8899, 8081, 5000, 3000, 9000, 9997]
# provider heuristics by default port (only a hint; the endpoint response is authoritative)
_PORT_PROVIDER = {11434: "ollama", 8787: "omlx", 1234: "lm-studio", 8080: "mlx",
                  8081: "llama.cpp", 8000: "local", 8899: "omlx"}


def _probe(base_url: str, timeout: float = 2.0) -> dict[str, Any] | None:
    """Probe one endpoint; returns {provider, models} or None if not an LLM server."""
    try:
        with httpx.Client(timeout=timeout) as client:
            # Ollama-family: /api/tags
            try:
                r = client.get(f"{base_url}/api/tags")
                if r.status_code == 200:
                    models = [m.get("name") for m in r.json().get("models", [])]
                    return {"provider": "ollama", "models": models}
            except Exception:  # noqa: BLE001
                pass
            # OpenAI-compatible: /v1/models
            try:
                r = client.get(f"{base_url}/v1/models")
                if r.status_code == 200:
                    models = [m.get("id") for m in r.json().get("data", [])]
                    return {"provider": "openai-compat", "models": models}
            except Exception:  # noqa: BLE001
                pass
    except Exception:  # noqa: BLE001
        pass
    return None


def _lms_endpoint() -> str | None:
    """LM Studio CLI endpoint, if the `lms` binary is available."""
    if not shutil.which("lms"):
        return None
    try:
        r = subprocess.run(["lms", "server", "status"], capture_output=True, text=True, timeout=5)
        for line in (r.stdout + r.stderr).splitlines():
            if "http" in line.lower() and ("127.0.0.1" in line or "localhost" in line):
                return line.split("http")[-1].strip().strip(":`").split()[0]
    except Exception:  # noqa: BLE001
        pass
    return None


class LocalModelRegistry:
    """Auto-detect which local LLM servers exist and on which port."""

    def __init__(self, ports: list[int] | None = None):
        env_p = os.environ.get("KUBONA_LLM_PORTS")
        if env_p:
            self.ports = [int(p) for p in env_p.split(",") if p.strip().isdigit()]
        else:
            self.ports = ports or DEFAULT_PORTS
        self._cache: dict[str, Any] | None = None

    def discover(self) -> dict[str, Any]:
        body = os.environ.get("KUBONA_LLM_BASE_URL", "").rstrip("/")
        if not body:
            body = _lms_endpoint()
        out: dict[str, Any] = {}
        if body:
            hit = _probe(body)
            if hit:
                port = body.rsplit(":", 1)[-1]
                out[f"manual:{body}"] = {"base_url": body, "port": port, **hit,
                                         "manual": True}
        for port in self.ports:
            base = f"http://127.0.0.1:{port}"
            hit = _probe(base)
            if hit:
                out[str(port)] = {"base_url": base, "port": port,
                                  "provider": hit["provider"], "models": hit["models"]}
        out["summary"] = {k: v["provider"] for k, v in out.items() if k != "summary"}
        self._cache = out
        return out

    def available_models(self) -> list[ModelInfo]:
        d = self.discover()
        models: list[ModelInfo] = []
        provider_hint = {11434: "ollama", 8787: "omlx", 1234: "lm-studio", 8080: "mlx", 8899: "omlx"}
        for key, info in d.items():
            if key == "summary":
                continue
            provider = info.get("provider")
            if provider == "openai-compat":
                provider = provider_hint.get(int(info.get("port")), "local")
            for mid in info.get("models", []):
                models.append(ModelInfo(provider=provider, model_id=mid,
                                        base_url=info["base_url"]))
        return models

    def pick_default(self) -> dict[str, Any] | None:
        d = self.discover()
        for key, info in d.items():
            if key == "summary":
                continue
            if info.get("models"):
                provider = info["provider"]
                if provider == "openai-compat":
                    provider = {11434: "ollama", 8787: "omlx", 1234: "lm-studio", 8080: "mlx"}.get(
                        int(info.get("port")), "local")
                return {"provider": provider, "base_url": info["base_url"],
                        "model": info["models"][0]}
        return None


def discovery_defaults() -> dict[str, Any]:
    chosen = LocalModelRegistry().pick_default()
    return {
        "llm_provider": chosen["provider"] if chosen else None,
        "llm_base_url": chosen["base_url"] if chosen else "",
        "llm_model": chosen["model"] if chosen else "",
        "llm_available": chosen is not None,
        "scan_ports": DEFAULT_PORTS,
    }
