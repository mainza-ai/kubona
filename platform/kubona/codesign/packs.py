"""kubona-pack/v1 codesign pack registry (scaffold — no logic claimed).

Each pack is anchor-required with evidence requirements; status starts at
`scaffold` (registered intent + acceptance criteria, zero circuit logic).
Promoting a pack to `active` requires a classical anchor + evaluator proof
recorded in the ledger — the registry refuses `active` without both.
This is the anti-theater rule: a pack name is not a result.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field

SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+$")

PACK_ORDER = ("numerics", "inversion", "dqi", "chemistry", "gauge")


@dataclass(frozen=True)
class PackSpec:
    name: str
    version: str
    anchor_required: bool
    evidence_reqs: tuple[str, ...]
    status: str = "scaffold"
    anchor_ref: str | None = None
    proof_ref: str | None = None

    def __post_init__(self) -> None:
        if self.name not in PACK_ORDER:
            raise ValueError(f"pack {self.name!r} not in {list(PACK_ORDER)}: refusing")
        if not SEMVER_RE.match(self.version):
            raise ValueError(f"version {self.version!r} must be semver x.y.z: refusing")
        if self.anchor_required is not True:
            raise ValueError("anchor_required must be true: refusing anchorless packs")
        if not self.evidence_reqs or any(not e.strip() for e in self.evidence_reqs):
            raise ValueError("evidence_reqs must be non-empty strings: refusing")
        if self.status not in ("scaffold", "active"):
            raise ValueError(f"status {self.status!r} must be scaffold|active: refusing")
        if self.status == "active" and not (self.anchor_ref and self.proof_ref):
            raise ValueError(f"pack {self.name!r} cannot be active without anchor_ref + proof_ref: refusing")


_DEFAULT_EVIDENCE: dict[str, tuple[str, ...]] = {
    "numerics": ("dyadic-partition proof", "integer-exact fit report", "Aer accuracy sweep"),
    "inversion": ("Lean history-decode proof", "256-bit walk sims", "Table-S4 union bound"),
    "dqi": ("DQI distribution harness", "BM/Lanczos evaluator proof", "Prange comparator"),
    "chemistry": ("frame-finder report", "alias-scheduler proof", "row-wise fallback proof"),
    "gauge": ("Xo-recoder proof", "loop-DP report", "4^3-only scope attestation"),
}


def default_registry() -> dict[str, PackSpec]:
    return {name: PackSpec(name=name, version="0.1.0", anchor_required=True,
                           evidence_reqs=_DEFAULT_EVIDENCE[name])
            for name in PACK_ORDER}


def promote(pack: PackSpec, anchor_ref: str, proof_ref: str) -> PackSpec:
    """Scaffold -> active only with ledger refs. Empty refs refuse."""
    if not anchor_ref.strip() or not proof_ref.strip():
        raise ValueError("promotion needs non-blank anchor_ref + proof_ref: refusing")
    return PackSpec(name=pack.name, version=pack.version, anchor_required=True,
                    evidence_reqs=pack.evidence_reqs, status="active",
                    anchor_ref=anchor_ref, proof_ref=proof_ref)
