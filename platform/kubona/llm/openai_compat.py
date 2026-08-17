"""Generic OpenAI-compatible adapter — covers LM Studio (:1234), OMLX/MLX (:8080),
llama.cpp (:8080), and Ollama's /v1. Uses httpx; offline => LLMUnavailable. The
optional api_key is for servers that require one (local ones usually do not).
"""

from __future__ import annotations

import json
from typing import Any

import httpx

from .base import AbstractLLM, LLMUnavailable, ModelInfo


class OpenAICompatLLM(AbstractLLM):
    def __init__(self, base_url: str, default_model: str | None = None,
                 api_key: str | None = None, provider: str = "openai-compat",
                 timeout: float = 30.0):
        self.base_url = base_url.rstrip("/")
        self.default_model = default_model
        self.api_key = api_key
        self.name = provider
        self._timeout = timeout
        self._client = httpx.Client(timeout=timeout)

    @property
    def available(self) -> bool:
        try:
            self._client.get(f"{self.base_url}/v1/models", timeout=3)
            return True
        except Exception:  # noqa: BLE001
            return False

    def _headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self.api_key}"} if self.api_key else {}

    def list_models(self) -> list[ModelInfo]:
        try:
            r = self._client.get(f"{self.base_url}/v1/models", headers=self._headers(), timeout=5)
            r.raise_for_status()
            ids = [m.get("id") for m in r.json().get("data", [])]
            return [ModelInfo(provider=self.name, model_id=i, base_url=self.base_url) for i in ids]
        except Exception:  # noqa: BLE001
            return []

    def complete(self, system: str, prompt: str, temperature: float = 0.2,
                 max_tokens: int = 256, json_mode: bool = False) -> str:
        model = self.default_model
        if model is None:
            models = self.list_models()
            if not models:
                raise LLMUnavailable(f"{self.name} has no discoverable models")
            model = models[0].model_id
        payload = {
            "model": model,
            "messages": [{"role": "system", "content": system},
                         {"role": "user", "content": prompt}],
            "temperature": temperature,
            "max_tokens": max_tokens,
        }
        # OpenAI-compatible local servers (OMLX, LM Studio, MLX, llama.cpp) support
        # forced-JSON via response_format; this makes tool-decision output reliable.
        if json_mode:
            payload["response_format"] = {"type": "json_object"}
        try:
            r = self._client.post(f"{self.base_url}/v1/chat/completions",
                                  headers={**self._headers(), "content-type": "application/json"},
                                  json=payload, timeout=self._timeout)
            r.raise_for_status()
            return r.json()["choices"][0]["message"]["content"]
        except (httpx.HTTPError, KeyError, json.JSONDecodeError) as e:
            raise LLMUnavailable(f"{self.name} request failed: {e}") from e
