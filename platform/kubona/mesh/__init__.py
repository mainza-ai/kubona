"""Mesh subsystem for Kubona.

Includes multimodal model routing, local HuggingFace hub client/downloader,
and user-defined agent registry integration.
"""

from __future__ import annotations

from kubona.mesh.hf_hub import (
    HFDownloadManager,
    HFModelQuantization,
    HFModelSummary,
    HuggingFaceHubClient,
    _infer_quant_type,
)
from kubona.mesh.model_router import (
    HybridModelRouter,
    Modality,
    MultimodalItem,
    MultimodalPayload,
    RoutingDecision,
)
from kubona.agents.user_defined import (
    STANDARD_SYSTEM_AGENTS,
    CustomAgentRegistry,
    UserDefinedAgentManifest,
)

__all__ = [
    "HFDownloadManager",
    "HFModelQuantization",
    "HFModelSummary",
    "HuggingFaceHubClient",
    "_infer_quant_type",
    "HybridModelRouter",
    "Modality",
    "MultimodalItem",
    "MultimodalPayload",
    "RoutingDecision",
    "UserDefinedAgentManifest",
    "CustomAgentRegistry",
    "STANDARD_SYSTEM_AGENTS",
]
