"""Full discovery loop (Phase 6 / §14.4): ground -> hypothesize -> design ->
experiment -> analyze -> report -> archive -> meta-learn, inside the gates."""

from .campaign import DiscoveryCampaign, run_campaign

__all__ = ["DiscoveryCampaign", "run_campaign"]
