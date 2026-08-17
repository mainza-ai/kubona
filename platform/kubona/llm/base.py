"""The LLM abstraction the agent loop talks to."""

from __future__ import annotations

from dataclasses import asdict, dataclass
from typing import Any


class LLMUnavailable(RuntimeError):
    pass


@dataclass(frozen=True)
class ModelInfo:
    provider: str       # "ollama" | "lm-studio" | "mlx" | "llama.cpp" | "openai-compat"
    model_id: str       # e.g. "llama3.2", "qwen2.5-coder", "mlx-community/..." , ...
    base_url: str

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class AbstractLLM:
    """generate + list_models + available; adapters implement these."""

    name: str = "abstract"
    base_url: str = ""

    @property
    def available(self) -> bool:
        return False

    def list_models(self) -> list[ModelInfo]:
        return []

    def complete(self, system: str, prompt: str, temperature: float = 0.2,
                 max_tokens: int = 256, json_mode: bool = False) -> str:
        raise LLMUnavailable(f"{self.name} not available")


class ScriptedLLM(AbstractLLM):
    """Deterministic stand-in for tests / offline development (no server needed)."""

    name = "scripted"

    def __init__(self, responses: list[str] | None = None, default: str = "ok"):
        self._responses = list(responses or [])
        self._default = default
        self._calls: list[tuple[str, str]] = []

    @property
    def available(self) -> bool:
        return True

    def complete(self, system: str, prompt: str, temperature: float = 0.2,
                 max_tokens: int = 256, json_mode: bool = False) -> str:
        self._calls.append((system, prompt))
        if self._responses:
            return self._responses.pop(0)
        return self._default
