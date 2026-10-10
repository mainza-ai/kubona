"""Plugin/provider-registration SDK (Phase 4): onboarding a provider is data + plugin."""

from .conformance import conformance_gate, grade_candidate
from .register import register_provider, validate_capability

__all__ = ["register_provider", "validate_capability",
           "grade_candidate", "conformance_gate"]
