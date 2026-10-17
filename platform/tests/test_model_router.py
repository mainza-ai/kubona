"""Unit tests for HybridModelRouter (Phase 3)."""

from kubona.mesh.model_router import (
    HybridModelRouter,
    Modality,
    MultimodalItem,
    MultimodalPayload,
    RoutingDecision,
)


def test_air_gap_blocks_cloud_with_refusal_not_fallback():
    router = HybridModelRouter(default_air_gapped=True)
    payload = MultimodalPayload(
        prompt="Synthesize research plan for family 278",
        preferred_model="anthropic/claude-3-7-sonnet",
        role_hint="lead_theorist",
    )
    decision = router.route(payload, spend_budget_usd=10.0, has_ed25519_mandate=True)

    assert decision.ok is False
    assert "air-gap" in decision.explanation


def test_cloud_escalation_when_air_gap_disabled():
    router = HybridModelRouter(default_air_gapped=False)
    payload = MultimodalPayload(
        prompt="Deep cavity formal proof search for Laughlin gap",
        preferred_model="anthropic/claude-3-7-sonnet",
        role_hint="lean4_proof_engineer",
        max_tokens=4096,
    )
    decision = router.route(payload, spend_budget_usd=1.0, has_ed25519_mandate=True)

    assert decision.is_local is False
    assert decision.air_gap_enforced is False
    assert decision.provider == 'anthropic'
    assert decision.model == 'anthropic/claude-3-7-sonnet'
    assert decision.estimated_cost_usd > 0.0


def test_air_gap_enforces_explicit_local_model():
    router = HybridModelRouter(default_air_gapped=True)
    payload = MultimodalPayload(
        prompt="Compile this circuit",
        preferred_model="local/test-model",
    )
    decision = router.route(payload)

    assert decision.ok is True
    assert decision.is_local is True
    assert decision.air_gap_enforced is True
    assert decision.model == "local/test-model"


class _EmptyRegistry:
    def discover(self):
        return {}


def test_no_model_anywhere_refuses():
    router = HybridModelRouter(local_registry=_EmptyRegistry(), default_air_gapped=False)
    payload = MultimodalPayload(prompt="anything")
    decision = router.route(payload)

    assert decision.ok is False
    assert "no model configured" in decision.explanation


def test_unfunded_cloud_refuses_instead_of_downgrading():
    router = HybridModelRouter(default_air_gapped=False)
    payload = MultimodalPayload(
        prompt="anything",
        preferred_model="anthropic/claude-x",
    )
    decision = router.route(payload, spend_budget_usd=0.0, has_ed25519_mandate=False)

    assert decision.ok is False
    assert "spend backing" in decision.explanation


def test_multimodal_context_formatting():
    router = HybridModelRouter()
    payload = MultimodalPayload(
        prompt="Analyze this active space and spectroscopic plot",
        system_prompt="You are the Lead Theorist",
        items=[
            MultimodalItem(
                modality=Modality.IMAGE,
                data="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
                mime_type="image/png",
            ),
            MultimodalItem(
                modality=Modality.MOLECULE_3D,
                data="H 0.0 0.0 0.0\nH 0.0 0.0 0.7414",
                mime_type="chemical/x-xyz",
            ),
            MultimodalItem(
                modality=Modality.CIRCUIT_QIR,
                data="OPENQASM 3.0;\nqubit[2] q;\nh q[0];\ncx q[0], q[1];",
                mime_type="text/x-qasm",
            ),
        ],
    )
    ctx = router.prepare_multimodal_context(payload)
    assert ctx["role"] == "user"
    assert len(ctx["content"]) == 4
    assert ctx["content"][0]["type"] == "text"
    assert ctx["content"][1]["type"] == "image_url"
    assert "[3D Molecular Structure" in ctx["content"][2]["text"]
    assert "[Quantum Circuit" in ctx["content"][3]["text"]
