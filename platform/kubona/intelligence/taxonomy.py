"""Ecosystem taxonomy v0: controlled vocabularies for intelligence records.

Reuses the provider-capability `Modality` enum as the technology axis so
intelligence tags and routing vocabulary can never drift apart. All tags are
validated against controlled vocabularies — unknown values refuse (no
free-text tags, no silent "other").

Axes:
  - technology: provider Modality values (superconducting, trapped_ion, ...)
  - domain: computing / networking / sensing (TQI's three coverage domains)
  - segment: hardware / software / services / enabling / investment / research
  - stage: pre-seed / seed / series-a / series-b / growth / public (funding)

`suggest()` proposes technology tags from description text by deterministic
keyword rules; below threshold it returns UNCLASSIFIED (analyst decides) —
the engine proposes, the validator disposes.
"""

from __future__ import annotations

from typing import Dict, List

from kubona.intelligence.entities import EntityStore, IntelligenceError
from kubona.schema.provider import Modality

UNCLASSIFIED = "unclassified"

TECHNOLOGIES = sorted(m.value for m in Modality)

DOMAINS = ("computing", "networking", "sensing")

SEGMENTS = ("hardware", "software", "services", "enabling", "investment",
            "research")

STAGES = ("pre-seed", "seed", "series-a", "series-b", "growth", "public")

_KEYWORDS: Dict[str, List[str]] = {
    "superconducting": ["superconducting", "transmon", "fluxonium"],
    "trapped_ion": ["trapped ion", "trapped-ion", "ion trap", "paul trap"],
    "neutral_atom": ["neutral atom", "rydberg", "atom array", "tweezer"],
    "photonic": ["photonic", "photonics", "continuous-variable", "boson sampling"],
    "annealing": ["annealing", "annealer", "qubo", "ising machine"],
    "gate_model": ["gate-model", "gate based", "universal quantum"],
    "statevector_simulator": ["statevector", "state-vector simulator"],
    "gpu_simulator": ["gpu simulator", "gpu-accelerated simulator"],
}


def validate_tags(tags: Dict[str, str]) -> Dict[str, str]:
    """Refuse any tag outside the controlled vocabularies."""
    out = dict(tags)
    if "technology" in out and out["technology"] not in TECHNOLOGIES:
        raise IntelligenceError(f"unknown technology tag: {out['technology']}")
    if "domain" in out and out["domain"] not in DOMAINS:
        raise IntelligenceError(f"unknown domain tag: {out['domain']}")
    if "segment" in out and out["segment"] not in SEGMENTS:
        raise IntelligenceError(f"unknown segment tag: {out['segment']}")
    if "stage" in out and out["stage"] not in STAGES:
        raise IntelligenceError(f"unknown stage tag: {out['stage']}")
    return out


def suggest(description: str, threshold: int = 2) -> Dict[str, object]:
    """Deterministic keyword suggestion for the technology axis.

    Returns {"technology": value | UNCLASSIFIED, "hits": {...}}. A technology
    needs `threshold` distinct keyword hits; ties or misses yield UNCLASSIFIED
    rather than a guess.
    """
    text = (description or "").lower()
    hits = {tech: sorted({kw for kw in kws if kw in text})
            for tech, kws in _KEYWORDS.items()}
    scored = {tech: len(kws) for tech, kws in hits.items() if len(kws) >= threshold}
    if len(scored) != 1:
        return {"technology": UNCLASSIFIED, "hits": hits}
    tech = next(iter(scored))
    return {"technology": tech, "hits": {tech: hits[tech]}}


def tag_entity(store: EntityStore, entity_id: str, tags: Dict[str, str],
               validator: str) -> Dict[str, str]:
    """Attach validated tags to an entity (validator named, ledger-recorded)."""
    if not validator:
        raise IntelligenceError("tagging requires a named validator")
    validated = validate_tags(tags)
    ent = store.entities[entity_id]
    ent.provenance["taxonomy_v0"] = validated
    ent.provenance["taxonomy_validator"] = validator
    ent.version += 1
    store._log("tag", {"id": entity_id, "tags": validated,
                       "validator": validator})
    return validated
