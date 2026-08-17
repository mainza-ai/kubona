"""Provider trait + registry.

The plugin trait from day one: a provider is a capability schema (data) + an
execution plugin (code) that implements the trait. Onboarding = new JSON + plugin.
"""

from .base import Provider, ProviderError, get_provider, list_providers, registry_health

__all__ = ["Provider", "ProviderError", "get_provider", "list_providers", "registry_health"]
