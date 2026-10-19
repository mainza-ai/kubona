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
    "nvidia": lambda base, model, key: OpenAICompatLLM(
        base or NIM_DEFAULT_BASE_URL,
        default_model=model, provider="nvidia", api_key=key),
    "scripted": lambda base, model, key: ScriptedLLM(),
}


class UnknownLLMProviderError(ValueError):
    """Refusal for unconfigured/unknown LLM providers (W2.17, fail-closed)."""


#: NVIDIA NIM default (parked local engines never receive cloud traffic by
#: accident: an nvidia request without a base_url targets NIM, not localhost).
NIM_DEFAULT_BASE_URL = "https://integrate.api.nvidia.com/v1"


def get_llm(settings: dict[str, Any]) -> AbstractLLM:
    """Build an LLM from settings {llm_provider, llm_base_url, llm_model, llm_api_key}.

    Fail-closed (W2.17): a missing provider (None) REFUSES — no silent
    deterministic stand-in (a silently-scripted "LLM" in a reasoning path is
    fiction). The scripted stand-in exists ONLY via the explicit
    `llm_provider="scripted"` opt-in. Unknown provider names refuse instead
    of degrading to scripted.
    """
    from kubona.settings import SECRET_KEYS

    provider = settings.get("llm_provider")
    if not provider:
        raise UnknownLLMProviderError(
            "no llm_provider configured: refusing silent scripted fallback — "
            "set llm_provider explicitly (or \"scripted\" to opt into the "
            "deterministic stand-in)")
    if not isinstance(provider, str) or not provider.strip():
        raise UnknownLLMProviderError(
            f"llm_provider must be a non-blank string, got {provider!r}")
    factory = MODEL_SOURCES.get(provider)
    if factory is None:
        raise UnknownLLMProviderError(
            f"unknown llm_provider {provider!r} (known: {sorted(MODEL_SOURCES)}): "
            "refusing silent scripted fallback")
    api_key = next((settings.get(k) for k in SECRET_KEYS if settings.get(k)), None)
    base = settings.get("llm_base_url")
    if provider == "nvidia" and not base:
        base = NIM_DEFAULT_BASE_URL
    return factory(base or "http://127.0.0.1:11434",
                   settings.get("llm_model"),
                   api_key)
