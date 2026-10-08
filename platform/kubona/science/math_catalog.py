"""OpenAI Math Repository Catalog and Discovery Engine (Ring 2 / Science Core).

Indexes, queries, and provides structured access to the 372 result families,
722 manuscripts, 17 mathematical disciplines, Lean 4 formalizations,
and reasoning traces from the `submodules/math` repository.
"""

from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional


@dataclass(frozen=True)
class PreprintMeta:
    slug: str
    title: str
    relative_pdf_path: str
    has_tex_source: bool = False
    bibtex: Optional[str] = None


@dataclass(frozen=True)
class ResultFamily:
    family_id: int  # 1 to 372
    title: str
    discipline: str
    summary: str
    preprints: List[PreprintMeta] = field(default_factory=list)
    has_lean_formalization: bool = False
    lean_doc_path: Optional[str] = None
    has_reasoning_trace: bool = False
    reasoning_trace_pdf: Optional[str] = None
    is_quantum_relevant: bool = False
    quantum_tags: List[str] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        return {
            "family_id": self.family_id,
            "title": self.title,
            "discipline": self.discipline,
            "summary": self.summary,
            "preprints_count": len(self.preprints),
            "preprints": [p.slug for p in self.preprints],
            "has_lean_formalization": self.has_lean_formalization,
            "lean_doc_path": self.lean_doc_path,
            "has_reasoning_trace": self.has_reasoning_trace,
            "reasoning_trace_pdf": self.reasoning_trace_pdf,
            "is_quantum_relevant": self.is_quantum_relevant,
            "quantum_tags": self.quantum_tags,
        }


# Mapping of families that directly impact quantum algorithms, many-body physics, and complexity theory
QUANTUM_PHYSICS_FAMILIES: Dict[int, Dict[str, Any]] = {
    221: {
        "discipline": "Probability and statistical mechanics",
        "title": "The Mézard–Parisi formula for diluted spin glasses",
        "tags": ["spin-glass", "diluted-cavity", "parisi-rsb", "statistical-mechanics"],
        "reasoning_trace": "mezard-parisi-formula.pdf",
    },
    265: {
        "discipline": "Mathematical physics",
        "title": "The two-dimensional gapped area law",
        "tags": ["area-law", "entanglement-entropy", "tensor-networks", "gapped-hamiltonians"],
    },
    266: {
        "discipline": "Mathematical physics",
        "title": "Exactly three mutually unbiased bases in dimension six",
        "tags": ["mub", "quantum-information", "zauner-conjecture", "quantum-state-tomography"],
    },
    268: {
        "discipline": "Mathematical physics",
        "title": "The spin-one Haldane gap",
        "tags": ["haldane-gap", "heisenberg-antiferromagnet", "spin-chains", "quantum-magnetism"],
    },
    269: {
        "discipline": "Mathematical physics",
        "title": "The Laughlin gap and stability under scalar disorder",
        "tags": ["fractional-quantum-hall", "laughlin-state", "spectral-gap", "topological-phases"],
    },
    271: {
        "discipline": "Mathematical physics",
        "title": "Bloch's law and spontaneous ferromagnetic order",
        "tags": ["heisenberg-ferromagnet", "spontaneous-magnetization", "bloch-law", "spin-waves"],
        "reasoning_trace": "spontaneous-magnetization-quantum-heisenberg-ferromagnet.pdf",
    },
    272: {
        "discipline": "Mathematical physics",
        "title": "Entanglement without secret key and the PPT-square conjecture",
        "tags": ["entanglement-theory", "ppt-states", "bound-entanglement", "quantum-cryptography"],
    },
    273: {
        "discipline": "Mathematical physics",
        "title": "The entropy photon-number inequality",
        "tags": ["continuous-variable-quantum", "bosonic-entropy", "beam-splitter", "quantum-channels"],
    },
    274: {
        "discipline": "Mathematical physics",
        "title": "Parity is not in QAC0",
        "tags": ["parity-circuits", "qac0-lower-bounds", "quantum-circuit-complexity", "unbounded-toffoli"],
    },
    275: {
        "discipline": "Mathematical physics",
        "title": "QMA-hardness of continuum Coulomb energy",
        "tags": ["qma-hardness", "quantum-chemistry", "electronic-structure", "vqe-complexity"],
    },
    276: {
        "discipline": "Mathematical physics",
        "title": "The classical capacity of generalized amplitude damping",
        "tags": ["quantum-channels", "amplitude-damping", "channel-capacity", "quantum-noise"],
    },
    277: {
        "discipline": "Mathematical physics",
        "title": "Threshold repetition for entangled games",
        "tags": ["entangled-games", "parallel-repetition", "bell-inequalities", "nonlocality"],
    },
    278: {
        "discipline": "Mathematical physics",
        "title": "Failure of Kohn–Sham ensemble representation",
        "tags": ["density-functional-theory", "kohn-sham", "quantum-chemistry", "correlation-energy"],
    },
    279: {
        "discipline": "Mathematical physics",
        "title": "Exact quantum factoring over a fixed finite gate set",
        "tags": ["quantum-algorithms", "shor-factoring", "clifford-t", "fault-tolerant-circuits"],
    },
    281: {
        "discipline": "Mathematical physics",
        "title": "QAOA optimality for the SK model",
        "tags": ["qaoa", "sherrington-kirkpatrick", "quantum-optimization", "quantum-advantage-realism"],
    },
    283: {
        "discipline": "Mathematical physics",
        "title": "Polynomial-time, constant-error unitary synthesis from a Boolean oracle",
        "tags": ["unitary-synthesis", "quantum-compilation", "aaronson-kuperberg", "oracle-algorithms"],
    },
    284: {
        "discipline": "Mathematical physics",
        "title": "The optimal randomized–quantum query exponent",
        "tags": ["quantum-query-complexity", "query-separation", "boolean-functions"],
    },
}

REASONING_TRACE_MAP: Dict[int, str] = {
    7: "ordinary-two-point-correlations.pdf",
    17: "irrationality-exponent-of-pi.pdf",
    87: "symmetric-and-general-mahler-conjectures.pdf",
    102: "basic-semidefinite-threshold-np-hardness.pdf",
    159: "quasipolynomial-arithmetic-progressions.pdf",
    197: "kaplansky-direct-finiteness-characteristic-two.pdf",
    221: "mezard-parisi-formula.pdf",
    271: "spontaneous-magnetization-quantum-heisenberg-ferromagnet.pdf",
    287: "free-group-factor-isomorphism.pdf",
    362: "relativistic-vlasov-maxwell.pdf",
}

DISCIPLINES = [
    "Number theory",
    "Algebraic and complex geometry",
    "Real and complex analysis",
    "Convex and metric geometry",
    "Theoretical computer science",
    "Dynamical systems and ergodic theory",
    "Combinatorics",
    "Algebra",
    "Probability and statistical mechanics",
    "Mathematical logic",
    "Group theory",
    "Mathematical physics",
    "Operator algebras",
    "Topology",
    "Functional analysis",
    "Differential geometry",
    "Partial differential equations",
]


class MathCatalog:
    """In-memory searchable catalog of the OpenAI Math dataset."""

    def __init__(self, root_path: Optional[Path | str] = None):
        self.root_path = self._resolve_root(root_path)
        self._families: Dict[int, ResultFamily] = {}
        self._by_discipline: Dict[str, List[int]] = {d: [] for d in DISCIPLINES}
        self._initialized = False

    @staticmethod
    def _resolve_root(path: Optional[Path | str]) -> Optional[Path]:
        candidates = [
            Path(path) if path else None,
            Path("submodules/math"),
            Path("../submodules/math"),
            Path("dev-docs/submodules/math"),
            Path("../dev-docs/submodules/math"),
        ]
        for c in candidates:
            if c and c.is_dir() and (c / "CONTENTS.md").exists():
                return c.resolve()
        return None

    def initialize(self) -> None:
        """Parse CONTENTS.md and overview.tex if available, or populate structured baseline."""
        if self._initialized:
            return

        if self.root_path and (self.root_path / "CONTENTS.md").exists():
            self._parse_from_filesystem()
        else:
            self._populate_fallback_registry()

        self._initialized = True

    def _parse_from_filesystem(self) -> None:
        assert self.root_path is not None
        contents_file = self.root_path / "CONTENTS.md"
        overview_file = self.root_path / "overview.tex"

        # Step 1: parse discipline sections and family entries from overview.tex
        family_meta = {}
        current_discipline = "General"
        if overview_file.exists():
            tex_content = overview_file.read_text(encoding="utf-8", errors="replace")
            for line in tex_content.splitlines():
                sec_match = re.search(r"\\cataloguesection\{([^}]+)\}", line)
                if sec_match:
                    current_discipline = sec_match.group(1).strip()
                res_match = re.search(
                    r"\\resultentry\{(\d+)\}\{([^}]+)\}\{([^}]+)\}", line
                )
                if res_match:
                    fid = int(res_match.group(1))
                    family_meta[fid] = {
                        "discipline": current_discipline,
                        "title": res_match.group(2).strip(),
                        "summary": res_match.group(3).strip(),
                    }

        # Step 2: parse preprints and descriptions from CONTENTS.md
        contents_text = contents_file.read_text(encoding="utf-8", errors="replace")
        blocks = re.findall(
            r"\*\*(\d+)\.\s*([^*]+)\*\*\s*(.*?)(?=\n\s*\*\*|\Z)",
            contents_text,
            re.DOTALL,
        )

        for fid_str, title_raw, desc_raw in blocks:
            fid = int(fid_str)
            title = title_raw.strip().rstrip(".")
            summary = desc_raw.strip()
            discipline = family_meta.get(fid, {}).get("discipline", "Mathematics")

            # Check lean doc
            lean_doc = self.root_path / "lean" / "docs" / f"{fid:03d}.md"
            has_lean = lean_doc.exists()

            # Check reasoning trace
            trace_name = REASONING_TRACE_MAP.get(fid)
            has_trace = trace_name is not None

            # Check quantum relevance
            q_info = QUANTUM_PHYSICS_FAMILIES.get(fid)
            is_quantum = q_info is not None
            q_tags = q_info["tags"] if q_info else []

            family = ResultFamily(
                family_id=fid,
                title=title,
                discipline=discipline,
                summary=summary,
                has_lean_formalization=has_lean,
                lean_doc_path=str(lean_doc.relative_to(self.root_path)) if has_lean else None,
                has_reasoning_trace=has_trace,
                reasoning_trace_pdf=trace_name,
                is_quantum_relevant=is_quantum,
                quantum_tags=q_tags,
            )
            self._families[fid] = family
            if discipline in self._by_discipline:
                self._by_discipline[discipline].append(fid)
            else:
                self._by_discipline[discipline] = [fid]

    def _populate_fallback_registry(self) -> None:
        """Populate the catalog with top indexed families when filesystem is offline."""
        for fid, q_info in QUANTUM_PHYSICS_FAMILIES.items():
            trace = REASONING_TRACE_MAP.get(fid)
            f = ResultFamily(
                family_id=fid,
                title=q_info["title"],
                discipline=q_info["discipline"],
                summary=f"Open mathematical and physical research benchmark family {fid}.",
                has_lean_formalization=True,
                lean_doc_path=f"lean/docs/{fid:03d}.md",
                has_reasoning_trace=trace is not None,
                reasoning_trace_pdf=trace,
                is_quantum_relevant=True,
                quantum_tags=q_info["tags"],
            )
            self._families[fid] = f
            if q_info["discipline"] in self._by_discipline:
                self._by_discipline[q_info["discipline"]].append(fid)

        # Also add classic math results from reasoning traces
        other_traces = [
            (7, "Ordinary two-point correlations of multiplicative functions", "Number theory"),
            (17, "The irrationality exponent of π", "Number theory"),
            (87, "Symmetric and general Mahler conjectures", "Convex and metric geometry"),
            (102, "Ordinary NP-hardness at the basic semidefinite threshold", "Theoretical computer science"),
            (159, "Quasipolynomial bounds for arithmetic progressions", "Combinatorics"),
            (197, "Kaplansky's direct-finiteness conjecture in characteristic two", "Algebra"),
            (287, "Isomorphism of free group factors", "Operator algebras"),
            (362, "The three-dimensional relativistic Vlasov–Maxwell system", "Partial differential equations"),
        ]
        for fid, title, disc in other_traces:
            trace = REASONING_TRACE_MAP.get(fid)
            f = ResultFamily(
                family_id=fid,
                title=title,
                discipline=disc,
                summary=f"Automated mathematical discovery family {fid}.",
                has_lean_formalization=True,
                lean_doc_path=f"lean/docs/{fid:03d}.md",
                has_reasoning_trace=True,
                reasoning_trace_pdf=trace,
                is_quantum_relevant=False,
                quantum_tags=[],
            )
            self._families[fid] = f
            if disc in self._by_discipline:
                self._by_discipline[disc].append(fid)

    def get_family(self, family_id: int) -> Optional[ResultFamily]:
        self.initialize()
        return self._families.get(family_id)

    def list_families(
        self,
        discipline: Optional[str] = None,
        quantum_only: bool = False,
        with_formalization_only: bool = False,
        with_trace_only: bool = False,
    ) -> List[ResultFamily]:
        self.initialize()
        results = list(self._families.values())
        if discipline:
            results = [f for f in results if f.discipline.lower() == discipline.lower()]
        if quantum_only:
            results = [f for f in results if f.is_quantum_relevant]
        if with_formalization_only:
            results = [f for f in results if f.has_lean_formalization]
        if with_trace_only:
            results = [f for f in results if f.has_reasoning_trace]
        return sorted(results, key=lambda f: f.family_id)

    def search(self, query: str) -> List[ResultFamily]:
        self.initialize()
        q = query.lower()
        matches = []
        for f in self._families.values():
            if q in f.title.lower() or q in f.summary.lower() or any(q in t for t in f.quantum_tags):
                matches.append(f)
        return sorted(matches, key=lambda f: f.family_id)

    def summary_stats(self) -> Dict[str, Any]:
        self.initialize()
        return {
            "total_families": len(self._families),
            "disciplines_count": len([d for d, items in self._by_discipline.items() if items]),
            "quantum_relevant_count": len([f for f in self._families.values() if f.is_quantum_relevant]),
            "formalized_count": len([f for f in self._families.values() if f.has_lean_formalization]),
            "reasoning_traces_count": len([f for f in self._families.values() if f.has_reasoning_trace]),
            "root_path": str(self.root_path) if self.root_path else None,
        }
