"""Information Flow Control (IFC) Provenance Lattice (Ring 1 & 0).

Adopts the buzz/crates/ifc-core ReaderSet security lattice model.
Labels propagate forward through provenance derivations by lattice join:
    join(L1, L2) = L1 ∩ L2 (the least upper bound shrinks readership).

Protects against cross-tenant data leaks and enforces export-control / publication boundaries.
"""

from __future__ import annotations

import dataclasses
from typing import FrozenSet, Iterable, Set


@dataclasses.dataclass(frozen=True)
class ConfidentialityLabel:
    """ReaderSet confidentiality label.
    
    Empty readers set (frozenset()) denotes strictly internal/private unexportable data.
    frozenset({'*'}) denotes fully public open-source data.
    Specific identifiers denote allowed tenant or consortium identities.
    """
    readers: FrozenSet[str]

    @classmethod
    def public(cls) -> ConfidentialityLabel:
        return cls(readers=frozenset({"*"}))

    @classmethod
    def internal(cls) -> ConfidentialityLabel:
        return cls(readers=frozenset())

    @classmethod
    def scoped(cls, readers: Iterable[str]) -> ConfidentialityLabel:
        return cls(readers=frozenset(readers))

    def is_public(self) -> bool:
        return "*" in self.readers

    def join(self, other: ConfidentialityLabel) -> ConfidentialityLabel:
        """Lattice join (least upper bound): intersection of readers.
        
        Deriving data from two sources means only parties allowed to read BOTH
        can read the derived artifact.
        """
        if self.is_public():
            return other
        if other.is_public():
            return self
        return ConfidentialityLabel(readers=self.readers.intersection(other.readers))

    def can_flow_to(self, target: ConfidentialityLabel) -> bool:
        """Information flow check: can data with this label flow to target context?
        
        A flow is permitted if target does not grant readership to unauthorized parties.
        self ⊑ target <=> target.readers ⊆ self.readers (or self is public).
        """
        if self.is_public():
            return True
        if target.is_public():
            # Private data cannot flow to public target
            return False
        # Target must be at least as restrictive as self
        return target.readers.issubset(self.readers)

    def is_readable_by(self, principal: str) -> bool:
        """Checks if a principal (tenant/user) can read this record."""
        if self.is_public():
            return True
        return principal in self.readers

    def to_list(self) -> list[str]:
        return sorted(list(self.readers))

    @classmethod
    def from_list(cls, readers_list: list[str]) -> ConfidentialityLabel:
        return cls(readers=frozenset(readers_list))
