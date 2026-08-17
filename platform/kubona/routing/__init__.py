"""Routing policy (\u00a72.9 / \u00a714.2): export-control + jurisdiction enforcement.
A provider is ELIGIBLE only if its jurisdiction/export-control flags satisfy the
caller's allowed jurisdictions and export-control class.
"""

from .policy import ROUTE_ELIGIBLE, ROUTE_INELIGIBLE, route, Route

__all__ = ["route", "Route", "ROUTE_ELIGIBLE", "ROUTE_INELIGIBLE"]
