"""Build the configured LLM from a SettingsStore (or discovery defaults)."""

from __future__ import annotations

from typing import Any

from .base import AbstractLLM, ScriptedLLM
from .ollama import OllamaLLM
from .openai_compat import OpenAICompatLLM

MODEL_SOURCES = {
    "ollama": lambda base, model, key: OllamaLLM(base_url=base, default_model=model),
    "lm-studio": lambda base, model, key: OpenAICompatLLM(base, default_model=model, provider="lm-studio", api_key=key),
    "mlx": lambda base, model, key: OpenAICompatLLM(base, default_model=model, provider="mlx", api_key=key),
    "omlx": lambda base, model, key: OpenAICompatLLM(base, default_model=model, provider="omlx", api_key=key),
    "openai-compat": lambda base, model, key: OpenAICompatLLM(base, default_model=model, provider="openai-compat", api_key=key),
    "llama.cpp": lambda base, model, key: OpenAICompatLLM(base, default_model=model, provider="llama.cpp", api_key=key),
    "scripted": lambda base, model, key: ScriptedLLM(),
}


def get_llm(settings: dict[str, Any]) -> AbstractLLM:
    """Build an LLM from settings {llm_provider, llm_base_url, llm_model, llm_api_key}."""
    provider = settings.get("llm_provider")
    if not provider:
        return ScriptedLLM()  # no local LLM configured -> deterministic stand-in
    factory = MODEL_SOURCES.get(provider)
    if factory is None:
        return ScriptedLLM()
    return factory(settings.get("llm_base_url") or "http://127.0.0.1:11434",
                   settings.get("llm_model"),
                   settings.get("llm_api_key") or None)
