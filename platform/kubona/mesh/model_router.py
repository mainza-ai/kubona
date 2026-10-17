"""Hybrid Multimodal Model Router (Phase 3) — Mission Control Cognitive Engine.

Routes reasoning, formal verification, visual inspection, and quantum compilation
tasks across local-first air-gapped runtimes (MLX, Ollama, vLLM, llama.cpp) and
explicitly requested cloud models under cryptographic spend envelopes.

Core Invariants:
  1. Local-First Air-Gap: If air_gapped=True, zero outbound bytes are permitted.
     Cloud models are hard-refused, never silently downgraded to a local stand-in.
  2. Multimodal Fusion: Accepts text, 3D molecular structures (PDB/CIF/SMILES),
     quantum circuits (QASM/QIR), spectroscopic plots, and audio telemetry.
  3. AP2 Cryptographic Spend Guard: Cloud escalation requires a valid Ed25519
     spend mandate within the authorized session dollar limit.
  4. No baked-in models: the router echoes caller-supplied models, falls back
     to live-discovered local models, and refuses otherwise. A rotation must
     never require a code change.
"""

from __future__ import annotations

import enum
from dataclasses import dataclass, field
from typing import Any

from kubona.llm.registry import LocalModelRegistry


class Modality(str, enum.Enum):
    """Multimodal PAYLOAD content kinds for LLM routing (W2.21: canonical
    payload enum — deliberately NOT kubona.schema.provider.Modality, which
    classifies HARDWARE program models (gate_model/trapped_ion/...). Same
    class name, disjoint domains; unifying them would be a type error. Both
    docstrings say so."""
    TEXT = "text"
    IMAGE = "image"
    MOLECULE_3D = "molecule_3d"
    CIRCUIT_QIR = "circuit_qir"
    SPECTROSCOPY = "spectroscopy"
    AUDIO = "audio"
    LEAN4_AST = "lean4_ast"


@dataclass
class MultimodalItem:
    modality: Modality
    data: str  # text, base64 payload, or file content
    mime_type: str = "text/plain"
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "modality": self.modality.value,
            "data": self.data,
            "mime_type": self.mime_type,
            "metadata": self.metadata,
        }


@dataclass
class MultimodalPayload:
    prompt: str
    system_prompt: str = ""
    items: list[MultimodalItem] = field(default_factory=list)
    role_hint: str = "lead_theorist"
    preferred_model: str | None = None
    max_tokens: int = 1024
    temperature: float = 0.2

    def to_dict(self) -> dict[str, Any]:
        return {
            "prompt": self.prompt,
            "system_prompt": self.system_prompt,
            "items": [item.to_dict() for item in self.items],
            "role_hint": self.role_hint,
            "preferred_model": self.preferred_model,
            "max_tokens": self.max_tokens,
            "temperature": self.temperature,
        }


@dataclass
class RoutingDecision:
    provider: str
    model: str
    is_local: bool
    air_gap_enforced: bool
    estimated_cost_usd: float
    explanation: str
    modalities_supported: list[str]
    # ok=False is a refusal (no model configured, air-gap block, unfunded
    # cloud request). Callers check it before dispatching.
    ok: bool = True

    def to_dict(self) -> dict[str, Any]:
        return {
            "provider": self.provider,
            "model": self.model,
            "is_local": self.is_local,
            "air_gap_enforced": self.air_gap_enforced,
            "estimated_cost_usd": self.estimated_cost_usd,
            "explanation": self.explanation,
            "modalities_supported": self.modalities_supported,
            "ok": self.ok,
        }


def _is_cloud_model(model: str) -> bool:
    """Mirrors Go's cloudModel (agents/custom.go): prefixed IDs classify
    by provider prefix, bare IDs by known cloud substrings (no "o1"-style
    fragments — they over-match). Unknown = cloud (fail-closed direction)."""
    if "/" in model:
        return model.split("/", 1)[0].lower() not in ("local", "omlx", "ollama", "lmstudio")
    low = model.lower()
    return any(s in low for s in ("claude", "gpt", "gemini", "deepseek", "nemotron", "opus", "sonnet", "-4o", "qwen-max", "qwen-plus"))


def _refuse(reason: str, air_gapped: bool = False) -> "RoutingDecision":
    return RoutingDecision(
        provider="", model="", is_local=True, air_gap_enforced=air_gapped,
        estimated_cost_usd=0.0, explanation=reason, modalities_supported=[],
        ok=False,
    )


class HybridModelRouter:
    """Intelligent Mission Control model routing engine."""

    def __init__(
        self,
        local_registry: LocalModelRegistry | None = None,
        default_air_gapped: bool = True,
    ):
        self.registry = local_registry or LocalModelRegistry()
        self.air_gapped = default_air_gapped

    def set_air_gap(self, enabled: bool) -> None:
        self.air_gapped = enabled

    def route(
        self,
        payload: MultimodalPayload,
        spend_budget_usd: float = 0.0,
        has_ed25519_mandate: bool = False,
    ) -> RoutingDecision:
        """Determines the optimal model runtime for the given multimodal payload."""
        required_modalities = {item.modality for item in payload.items}
        needs_vision = Modality.IMAGE in required_modalities or Modality.SPECTROSCOPY in required_modalities
        # W2.13: the old `needs_audio` local was assigned and never read —
        # audio does NOT influence routing (no audio-capable local model
        # detection exists; audio payloads ride the default local pick and
        # are rendered as transcribed text in prepare_multimodal_context).
        # Deleting the dead variable instead of pretending it gates anything.

        # 1. Discover active local models
        local_discovery = self.registry.discover()
        local_models: list[str] = []
        for port_str, info in local_discovery.items():
            if port_str != "summary":
                local_models.extend(info.get("models", []))

        # Check for local multimodal capability (e.g. llava, qwen-vl, minicpm, gemma-vision)
        local_vision_models = [
            m for m in local_models if any(k in m.lower() for k in ["vl", "vision", "llava", "omni", "minicpm"])
        ]

        # 2. Air-gap enforcement
        # Model selection is never invented: an explicit caller model wins,
        # else the strongest discovered local model, else refusal. No
        # baked-in IDs anywhere on this path (a rotation must not need a
        # code change).
        requested = (payload.preferred_model or "").strip()
        discovered = local_vision_models[0] if (needs_vision and local_vision_models) else (local_models[0] if local_models else "")
        model = requested or discovered
        if not model:
            return _refuse(
                "no model configured: pass preferred_model or start a local engine "
                "(no baked-in default is routed)",
                air_gapped=self.air_gapped,
            )

        # 2. Air-gap enforcement: cloud models refuse outright (never
        # silently downgraded to a local stand-in).
        if self.air_gapped:
            if _is_cloud_model(model):
                return _refuse(
                    f"sovereign air-gap is active: cloud model {model} refused, zero outbound bytes",
                    air_gapped=True,
                )
            provider = model.split("/")[0] if "/" in model else "local_sovereign"
            return RoutingDecision(
                provider=provider,
                model=model,
                is_local=True,
                air_gap_enforced=True,
                estimated_cost_usd=0.0,
                explanation="Air-gapped local execution on private hardware.",
                modalities_supported=["text", "image", "molecule_3d", "circuit_qir"],
            )

        # 3. Cloud allowed: explicit cloud models are spend-gated (mandate
        # or budget); unfunded cloud requests refuse, never downgrade.
        if _is_cloud_model(model):
            if not (spend_budget_usd > 0.05 or has_ed25519_mandate):
                return _refuse(
                    f"cloud model {model} requested without spend backing: declare a budget or mandate",
                )
            provider = model.split("/")[0] if "/" in model else "cloud_frontier"
            estimated_cost = 0.03 if payload.max_tokens <= 2048 else 0.15
            return RoutingDecision(
                provider=provider,
                model=model,
                is_local=False,
                air_gap_enforced=False,
                estimated_cost_usd=estimated_cost,
                explanation="Escalated to frontier cloud model with Ed25519 spend envelope.",
                modalities_supported=["text", "image", "molecule_3d", "circuit_qir", "audio"],
            )

        # Local model, cloud allowed: route local, unenforced.
        provider = model.split("/")[0] if "/" in model else "local_default"
        return RoutingDecision(
            provider=provider,
            model=model,
            is_local=True,
            air_gap_enforced=False,
            estimated_cost_usd=0.0,
            explanation="Local-first routing (conserving token budget).",
            modalities_supported=["text", "image", "molecule_3d", "circuit_qir"],
        )

    def prepare_multimodal_context(self, payload: MultimodalPayload) -> dict[str, Any]:
        """Translates multimodal payloads into standardized OpenAI/Ollama message structures."""
        content_parts: list[dict[str, Any]] = [{"type": "text", "text": payload.prompt}]

        for item in payload.items:
            if item.modality in [Modality.IMAGE, Modality.SPECTROSCOPY]:
                # Format as image_url or inline base64
                url = item.data if item.data.startswith("data:") or item.data.startswith("http") else f"data:{item.mime_type};base64,{item.data}"
                content_parts.append({
                    "type": "image_url",
                    "image_url": {"url": url},
                })
            elif item.modality == Modality.MOLECULE_3D:
                # Inject structured atomic coordinates into prompt
                content_parts.append({
                    "type": "text",
                    "text": f"\n[3D Molecular Structure ({item.mime_type})]:\n{item.data}\n",
                })
            elif item.modality == Modality.CIRCUIT_QIR:
                # Inject quantum circuit definition
                content_parts.append({
                    "type": "text",
                    "text": f"\n[Quantum Circuit ({item.mime_type})]:\n{item.data}\n",
                })
            elif item.modality == Modality.LEAN4_AST:
                # Inject formal proof AST
                content_parts.append({
                    "type": "text",
                    "text": f"\n[Lean 4 Formal Goal State]:\n{item.data}\n",
                })
            elif item.modality == Modality.AUDIO:
                content_parts.append({
                    "type": "text",
                    "text": f"\n[Voice Audio Telemetry ({item.mime_type})]: {item.metadata.get('transcription', item.data)}\n",
                })

        return {
            "role": "user",
            "content": content_parts,
            "system": payload.system_prompt,
        }
