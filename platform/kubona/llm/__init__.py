"""Local LLM integration (Phase-6 closure): Ollama / LM Studio / OMLX / llama.cpp.

An AbstractLLM interface + OpenAI-compatible and Ollama-native adapters + a
LocalModelRegistry that discovers the models present on the host machine. This is
the missing "brain" for the autoresearch agent and the discovery loop — steering
ansatz proposals and workflow steps with a local model (offline-tolerant).
"""

from .base import AbstractLLM, ModelInfo, ScriptedLLM
from .registry import LocalModelRegistry, discovery_defaults
from .openai_compat import OpenAICompatLLM
from .ollama import OllamaLLM
from .factory import get_llm, MODEL_SOURCES, UnknownLLMProviderError

__all__ = ["AbstractLLM", "ModelInfo", "ScriptedLLM", "LocalModelRegistry", "discovery_defaults",
           "OpenAICompatLLM", "OllamaLLM", "get_llm", "MODEL_SOURCES", "UnknownLLMProviderError"]
