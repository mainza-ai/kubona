"""Ollama-native adapter — lists models via /api/tags, generates via /api/generate."""

from __future__ import annotations

import json

import httpx

from typing import Any

from .base import AbstractLLM, LLMUnavailable, ModelInfo
from .openai_compat import OpenAICompatLLM  # fallback for compatibility


class OllamaLLM(AbstractLLM):
    name = "ollama"

    def __init__(self, base_url: str = "http://127.0.0.1:11434", default_model: str | None = None,
                 timeout: float = 60.0):
        self.base_url = base_url.rstrip("/")
        self.default_model = default_model
        self._client = httpx.Client(timeout=timeout)

    @property
    def available(self) -> bool:
        try:
            self._client.get(f"{self.base_url}/api/tags", timeout=3)
            return True
        except Exception:  # noqa: BLE001
            return False

    def list_models(self) -> list[ModelInfo]:
        try:
            r = self._client.get(f"{self.base_url}/api/tags", timeout=5)
            r.raise_for_status()
            return [ModelInfo(provider="ollama", model_id=m.get("name"), base_url=self.base_url)
                    for m in r.json().get("models", [])]
        except Exception:  # noqa: BLE001
            return []

    def complete(self, system: str, prompt: str, temperature: float = 0.2,
                 max_tokens: int = 256, json_mode: bool = False) -> str:
        model = self.default_model
        if model is None:
            models = self.list_models()
            if not models:
                raise LLMUnavailable("ollama has no discoverable models")
            model = models[0].model_id
        payload: dict[str, Any] = {"model": model, "system": system, "prompt": prompt,
                   "temperature": temperature, "stream": False, "num_predict": max_tokens}
        # W0.15: the loop always passes json_mode — the missing param used to
        # crash with TypeError instead of failing closed.
        if json_mode:
            payload["format"] = "json"
        try:
            r = self._client.post(f"{self.base_url}/api/generate", json=payload, timeout=60)
            r.raise_for_status()
            return r.json().get("response", "")
        except Exception as e:
            raise LLMUnavailable(f"ollama generate failed: {e}") from e
