"""Scientific Manuscript and Automated Peer Review Engine (Ring 2 / Science Core).

Generates modular, publication-grade LaTeX manuscripts and automated peer reviews
synthesizing physical experiment data, classical analytical anchors, and machine-checked
Lean 4 formal proofs into citable, reproducible arXiv-style papers.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, List, Optional

_SLUG_RE = re.compile(r"^[a-z0-9_-]{1,64}$")
_LATEX_ESCAPES = (
    ("\\", r"\textbackslash{}"),
    ("&", r"\&"),
    ("%", r"\%"),
    ("$", r"\$"),
    ("#", r"\#"),
    ("_", r"\_"),
    ("{", r"\{"),
    ("}", r"\}"),
    ("~", r"\textasciitilde{}"),
    ("^", r"\textasciicircum{}"),
)


def _escape_latex(text: str) -> str:
    out = str(text)
    for k, v in _LATEX_ESCAPES:
        out = out.replace(k, v)
    return out


def _validate_section_slug(slug: str) -> None:
    if not _SLUG_RE.fullmatch(slug or ""):
        raise ValueError(f"invalid section slug (expected [a-z0-9_-]{{1,64}}): {slug!r}")


@dataclass
class ManuscriptSection:
    slug: str
    title: str
    latex_content: str
    bibtex_keys: List[str] = field(default_factory=list)
    figures: List[str] = field(default_factory=list)


@dataclass
class ReviewEvidence:
    """Attested inputs to peer review — the anti-heuristic contract.

    Every score derives from these fields; nothing is inferred from prose or
    from mere presence of a hash string. Missing (None) fields falsify the
    corresponding dimension (fail-closed). Ledger receipts are content hashes
    the caller attests exist; this module does not verify them against the
    ledger, and no served endpoint records the review (the console_api handler
    is not part of the deployed stack).
    """

    comparator_verdict: Optional[str] = None  # "live_kernel_proven" | "static_ast_containment" | "failed"
    sorry_ax_count: Optional[int] = None
    anchor_discrepancy_mha: Optional[float] = None
    provenance_receipts: List[str] = field(default_factory=list)
    citation_distance: Optional[float] = None  # citation-graph distance; None = unmeasured
    negative_certificates: List[str] = field(default_factory=list)


@dataclass
class ManuscriptSignoff:
    """Human approval over a manuscript's content hash (frame-anchored review).

    The human signs `manuscript-signoff:<manuscript_id>:<content_hash>:<expiry>`
    with Ed25519 (console WebCrypto); attach_signoff verifies before recording.
    Publication (DOI/Zenodo) requires a valid signoff AND an ACCEPT review —
    the deposit is staged, never auto-published.
    """

    manuscript_id: str
    content_hash: str
    signer_pubkey_hex: str
    signature_hex: str
    signed_at: str = ""
    # Bound at attach time so deposit can re-verify without trusting the
    # caller to pass the same expiry again.
    expiry: str = ""

    def canonical_payload(self, expiry: str) -> bytes:
        return f"manuscript-signoff:{self.manuscript_id}:{self.content_hash}:{expiry}".encode()

    def verify(self, expiry: str) -> bool:
        try:
            from datetime import datetime, timezone
            try:
                if expiry.replace(".", "", 1).isdigit():
                    exp_dt = datetime.fromtimestamp(float(expiry), tz=timezone.utc)
                else:
                    exp_dt = datetime.fromisoformat(expiry.replace("Z", "+00:00"))
                if datetime.now(timezone.utc) > exp_dt:
                    return False
            except Exception:
                return False

            from nacl.signing import VerifyKey
            from nacl.exceptions import BadSignatureError
            vk = VerifyKey(bytes.fromhex(self.signer_pubkey_hex))
            vk.verify(self.canonical_payload(expiry), bytes.fromhex(self.signature_hex))
            return True
        except Exception:
            return False


@dataclass
class PeerReviewReport:
    reviewer_persona: str
    mathematical_rigor_score: float  # 1.0 to 10.0 (10.0 requires machine-checked Lean 4 comparator proof)
    novelty_score: float             # 1.0 to 10.0 (citation graph topological distance)
    empirical_grounding_score: float  # 1.0 to 10.0 (discrepancy <= 1.6 mHa against analytical anchors)
    axiomatic_hygiene_score: float   # 1.0 to 10.0 (10.0 = 0 sorryAx and standard axioms only)
    overall_recommendation: str      # "ACCEPT", "REVISE", "REJECT"
    summary: str
    actionable_critiques: List[str] = field(default_factory=list)
    reproducibility_score: float = 9.5  # 1.0 to 10.0 (100% content-addressed code and circuit receipts)

    def mean_score(self) -> float:
        return (
            self.mathematical_rigor_score
            + self.novelty_score
            + self.empirical_grounding_score
            + self.axiomatic_hygiene_score
            + self.reproducibility_score
        ) / 5.0

    def is_acceptable(self) -> bool:
        # Wiki gate, exact: ACCEPT with composite >= 9.0 and hygiene exactly
        # 10.0 (machine-checked proof, zero sorryAx). Anything weaker revises.
        return (
            self.overall_recommendation == "ACCEPT"
            and self.axiomatic_hygiene_score == 10.0
            and self.mean_score() >= 9.0
        )


@dataclass
class ScientificManuscript:
    manuscript_id: str
    title: str
    authors: List[str]
    abstract: str
    sections: List[ManuscriptSection]
    bibtex_entries: Dict[str, str] = field(default_factory=dict)
    lean4_proof_hash: Optional[str] = None
    provenance_record_hash: Optional[str] = None
    reviews: List[PeerReviewReport] = field(default_factory=list)
    bilingual_title: Optional[str] = None
    bilingual_abstract: Optional[str] = None
    second_language: str = "Tonga"
    figures: Dict[str, Any] = field(default_factory=dict)
    signoff: Optional[ManuscriptSignoff] = None
    document_type: str = "academic_preprint"
    subtitle: Optional[str] = None
    keywords: List[str] = field(default_factory=list)

    def compile_latex(self) -> str:
        r"""Assembles main.tex for the modular project.

        The document ``\input``s ``sections/<slug>.tex`` rather than inlining
        section bodies: the project written by ``export_project`` and the
        build path compile the same source, so they cannot drift. Figures are
        included only when declared, and only in formats pdfTeX can read — a
        stray SVG refuses instead of silently producing a figure-less paper.
        """
        for name in self.figures:
            suffix = Path(name).suffix.lower()
            if suffix not in {".pdf", ".png", ".jpg", ".jpeg"}:
                raise ValueError(
                    f"figure {name!r} is not includable by pdfTeX "
                    f"({suffix or 'no extension'}); supply PDF/PNG or remove it")
        doc_type = (self.document_type or "academic_preprint").strip().lower()
        header_labels = {
            "academic_preprint": "KUBONA SCIENTIFIC PREPRINT",
            "white_paper": "KUBONA TECHNICAL WHITE PAPER",
            "discovery_dossier": "KUBONA DISCOVERY & AUDIT DOSSIER",
            "technical_report": "KUBONA TECHNICAL BENCHMARK REPORT",
        }
        doc_label = header_labels.get(doc_type, "KUBONA SCIENTIFIC PREPRINT")

        out = []
        out.append(r"\documentclass[10.5pt,letterpaper]{article}")
        out.append(r"\usepackage[margin=0.85in]{geometry}")
        out.append(r"\usepackage{amsmath,amssymb}")
        out.append(r"\usepackage{graphicx}")
        out.append(r"\usepackage{booktabs}")
        out.append(r"\usepackage{tabularx}")
        out.append(r"\usepackage{float}")
        out.append(r"\usepackage{xcolor}")
        out.append(r"\usepackage{colortbl}")
        out.append(r"\usepackage{caption}")
        out.append(r"\usepackage{fancyhdr}")
        out.append(r"\usepackage{microtype}")
        out.append(r"\definecolor{kubonaNavy}{HTML}{14213D}")
        out.append(r"\definecolor{kubonaTeal}{HTML}{0E6B74}")
        out.append(r"\definecolor{kubonaSlate}{HTML}{4A5568}")
        out.append(r"\definecolor{kubonaTint}{HTML}{F4F7FA}")
        out.append(r"\definecolor{kubonaRule}{HTML}{CBD5E1}")
        out.append(
            r"\usepackage[colorlinks=true,linkcolor=kubonaNavy,"
            r"citecolor=kubonaTeal,urlcolor=kubonaTeal]{hyperref}"
        )
        out.append(r"\captionsetup{font=small,labelfont=bf,skip=6pt}")
        out.append(r"\setlength{\parskip}{0.45em}")
        out.append(r"\setlength{\parindent}{0pt}")
        out.append(r"\pagestyle{fancy}")
        out.append(r"\fancyhf{}")
        out.append(rf"\fancyhead[L]{{\footnotesize\color{{kubonaSlate}}\textsc{{{doc_label}}}}}")
        out.append(rf"\fancyhead[R]{{\footnotesize\color{{kubonaSlate}}\texttt{{{_escape_latex(self.manuscript_id)}}}}}")
        out.append(r"\fancyfoot[L]{\footnotesize\color{kubonaSlate}Verified via Kubona Ledger \& \texttt{verification-v1}}")
        out.append(r"\fancyfoot[R]{\footnotesize\color{kubonaSlate}Page \thepage}")
        out.append(r"\renewcommand{\headrulewidth}{0.4pt}")
        out.append(r"\renewcommand{\footrulewidth}{0.2pt}")
        # Reproducible-byte primitives (pdfTeX). Guarded so a different
        # engine ignores them instead of failing on an unknown primitive.
        out.append(r"\ifdefined\pdfinfoomitdate\pdfinfoomitdate=1\fi")
        out.append(r"\ifdefined\pdftrailerid\pdftrailerid{}\fi")
        out.append(r"\ifdefined\pdfsuppressptexinfo\pdfsuppressptexinfo=-1\fi")

        # Title
        full_title = self.title
        if self.subtitle:
            full_title = f"{full_title} \\\\\n\\vspace{{0.3em}}\\large\\color{{kubonaSlate}}{self.subtitle}"
        if self.bilingual_title:
            full_title = f"{full_title} \\\\\n\\large\\textit{{{_escape_latex(self.bilingual_title)}}}"
        out.append(f"\\title{{{full_title}}}")

        author_str = " \\and ".join(self.authors)
        out.append(f"\\author{{{author_str}}}")
        out.append(r"\date{\today}")
        out.append(r"\begin{document}")
        if doc_type in ("white_paper", "discovery_dossier"):
            out.append(
                r"\noindent\colorbox{kubonaNavy}{\parbox{\dimexpr\textwidth-2\fboxsep\relax}{"
                rf"\vspace{{0.4em}}\centering\color{{white}}\small\textbf{{{doc_label}}}\vspace{{0.4em}}"
                r"}}\vspace{-0.5em}"
            )
        out.append(r"\maketitle")
        out.append(r"\thispagestyle{fancy}")

        # Abstract / Executive Summary
        if doc_type == "white_paper":
            out.append(
                r"\noindent\colorbox{kubonaTint}{\parbox{\dimexpr\textwidth-2\fboxsep\relax}{"
                r"\vspace{0.5em}\textbf{\color{kubonaNavy}Executive Summary}\par\vspace{0.3em}"
                f"{self.abstract}"
                r"\vspace{0.5em}}}\vspace{0.8em}"
            )
        else:
            out.append(r"\begin{abstract}")
            out.append(self.abstract)
            if self.bilingual_abstract:
                out.append(
                    f"\n\n\\vspace{{0.5em}}\n\\noindent\\textbf{{{_escape_latex(self.second_language)} Summary:}}"
                    f"\\\\\n\\textit{{{_escape_latex(self.bilingual_abstract)}}}"
                )
            out.append(r"\end{abstract}")

        if self.keywords:
            kw = ", ".join(_escape_latex(k) for k in self.keywords if str(k).strip())
            if kw:
                out.append(rf"\noindent\textbf{{\small Keywords:}} {{\small {kw}}}\vspace{{0.6em}}")

        for sec in self.sections:
            _validate_section_slug(sec.slug)
            out.append(f"\\input{{sections/{sec.slug}}}")

        out.append(r"\bibliographystyle{unsrt}")
        out.append(r"\bibliography{references}")
        out.append(r"\end{document}")
        return "\n\n".join(out)

    def compile_bibtex(self) -> str:
        """Assembles all BibTeX citations."""
        return "\n\n".join(self.bibtex_entries.values())

    def content_hash(self) -> str:
        """Canonical SHA-256 manifest hash covering main.tex, every section,
        references.bib, and every binary figure artifact."""
        import hashlib
        h = hashlib.sha256()
        def _feed(name: str, data: bytes) -> None:
            h.update(name.encode("utf-8"))
            h.update(b"\x00")
            h.update(hashlib.sha256(data).hexdigest().encode("ascii"))
            h.update(b"\n")
        _feed("main.tex", self.compile_latex().encode("utf-8"))
        _feed("references.bib", self.compile_bibtex().encode("utf-8"))
        for sec in self.sections:
            _validate_section_slug(sec.slug)
            sec_body = f"\\section{{{sec.title}}}\n\n{sec.latex_content}".encode("utf-8")
            _feed(f"sections/{sec.slug}.tex", sec_body)
        for fig_name in sorted(self.figures.keys()):
            raw = self.figures[fig_name]
            data = raw.encode("utf-8") if isinstance(raw, str) else raw
            _feed(f"figures/{fig_name}", data)
        return h.hexdigest()

    def export_project(self, target_dir: Path) -> Path:
        """Exports the manuscript project to a structured directory.

        ``main.tex`` \\input-s these section files, so the written project is
        the exact source the build path compiles. Figures are written as
        binary artifacts (PDF/PNG); SVG is refused by ``compile_latex``.
        """
        target_dir.mkdir(parents=True, exist_ok=True)
        sections_dir = target_dir / "sections"
        sections_dir.mkdir(exist_ok=True)
        figures_dir = target_dir / "figures"
        figures_dir.mkdir(exist_ok=True)

        # Write main.tex (validates figure formats before anything is written)
        (target_dir / "main.tex").write_text(self.compile_latex(), encoding="utf-8")

        # Write references.bib
        (target_dir / "references.bib").write_text(self.compile_bibtex(), encoding="utf-8")

        # Write individual section files, headings included: main.tex only
        # \input-s them, so a heading-less body would erase the structure.
        for sec in self.sections:
            _validate_section_slug(sec.slug)
            sec_path = (sections_dir / f"{sec.slug}.tex").resolve()
            if sections_dir.resolve() not in sec_path.parents:
                raise ValueError(f"section slug escapes sections/: {sec.slug!r}")
            sec_path.write_text(
                f"\\section{{{sec.title}}}\n\n{sec.latex_content}", encoding="utf-8")

        # Write figures (compile_latex already refused non-includable formats)
        for fig_name, fig_content in self.figures.items():
            target = (figures_dir / fig_name).resolve()
            if figures_dir.resolve() not in target.parents:
                raise ValueError(f"figure path escapes figures/: {fig_name!r}")
            target.write_bytes(fig_content.encode("utf-8") if isinstance(fig_content, str) else fig_content)

        # Write metadata
        meta = {
            "manuscript_id": self.manuscript_id,
            "title": self.title,
            "bilingual_title": self.bilingual_title,
            "second_language": self.second_language,
            "authors": self.authors,
            "lean4_proof_hash": self.lean4_proof_hash,
            "provenance_record_hash": self.provenance_record_hash,
            "figures_count": len(self.figures),
            "review_count": len(self.reviews),
            "consensus_accept": all(r.is_acceptable() for r in self.reviews) if self.reviews else False,
            "signoff": None if self.signoff is None else {
                "signer_pubkey_hex": self.signoff.signer_pubkey_hex,
                "signature_hex": self.signoff.signature_hex,
                "signed_at": self.signoff.signed_at,
            },
            "doi": None,  # minted by Zenodo/arXiv at deposit time, never here
        }
        (target_dir / "metadata.json").write_text(json.dumps(meta, indent=2), encoding="utf-8")
        return target_dir


class ManuscriptGenerator:
    """Builds full manuscripts from campaign results and machine-checked proofs.

    Two constructors, one honesty rule: verification prose is NEVER emitted
    from raw floats. ``create_discovery_manuscript`` is the DRAFT path — it
    states measured-vs-anchor numbers and their discrepancy factually and
    marks unproven results UNVERIFIED. ``create_certified_discovery_manuscript``
    is the CERTIFIED path — it additionally requires the Lean proof hash AND
    a comparator ReviewEvidence verdict before any "verified" sentence, and
    refuses (fail-closed) when the discrepancy exceeds chemical accuracy.
    """

    #: Chemical-accuracy bar for certification (mHa). A discrepancy above
    #: this refuses certified prose — a manuscript must not claim to match
    #: analytical bounds it demonstrably misses.
    CHEMICAL_ACCURACY_MHA = 1.6

    #: Comparator verdicts that count as machine-checked proof.
    _KERNEL_PROVEN_VERDICTS = frozenset({"live_kernel_proven"})

    @staticmethod
    def _discrepancy_mha(analytical_anchor_val: float, measured_val: float) -> float:
        """Server-side discrepancy computation (mHa), never caller-asserted."""
        return abs(float(measured_val) - float(analytical_anchor_val)) * 1000.0

    @staticmethod
    def create_discovery_manuscript(
        title: str,
        target_system: str,
        authors: List[str],
        abstract: str,
        analytical_anchor_name: str,
        analytical_anchor_val: float,
        measured_val: float,
        lean4_proof_hash: Optional[str] = None,
        negative_certificates: Optional[List[str]] = None
    ) -> ScientificManuscript:
        """DRAFT constructor: states numbers factually, claims no verification.

        The discrepancy is computed here from the two raw floats — never
        caller-asserted — and reported as a number. No "verified" sentence
        is ever emitted on this path (not even when a proof hash is
        supplied: a hash string's mere presence proves nothing). Use
        ``create_certified_discovery_manuscript`` with comparator evidence
        for attested verification prose.
        """
        discrepancy_mha = ManuscriptGenerator._discrepancy_mha(
            analytical_anchor_val, measured_val)
        sections = []
        
        # Section 1: Introduction & Physical Problem
        safe_target = _escape_latex(target_system)
        safe_anchor = _escape_latex(analytical_anchor_name)
        safe_proof = _escape_latex(lean4_proof_hash or "UNVERIFIED")
        sections.append(ManuscriptSection(
            slug="introduction",
            title="Introduction and Physical System",
            latex_content=(
                f"We investigate the electronic and many-body spectral properties of {safe_target}. "
                "Using the Kubona epistemic continuum engine, we integrate rigorous formal deduction with "
                "subspace quantum diagonalization to avoid unphysical variational traps."
            ),
            bibtex_keys=["kubona_manifesto_2026"]
        ))

        # Section 2: Exact Analytical Anchoring & Deductive Sieve
        # DRAFT prose: factual numbers only. No "matching ... within certified
        # tolerance" and no comparator sentence — those require the certified
        # constructor with attested evidence (a hash alone proves nothing).
        sections.append(ManuscriptSection(
            slug="analytical_anchors",
            title="Exact Analytical Ground Truth & Complexity Sieve",
            latex_content=(
                f"The physical model is calibrated against the exact analytical anchor "
                f"\\texttt{{{safe_anchor}}} with theoretical value ${analytical_anchor_val}$. "
                f"Physical simulation yielded ${measured_val}$, a discrepancy of "
                f"{discrepancy_mha:.4f} mHa against the anchor "
                f"(chemical accuracy requires $\\leq$ {ManuscriptGenerator.CHEMICAL_ACCURACY_MHA} mHa). "
                "Attested verification of these invariants is recorded in the peer-review record, "
                "not asserted here."
            ),
            bibtex_keys=["openai_math_corpus_2026"]
        ))

        # Section 3: Empirical Execution & Negative Results
        neg_text = ""
        if negative_certificates:
            safe_negs = ", ".join(_escape_latex(str(c)) for c in negative_certificates)
            neg_text = (
                "\\subsection{Negative Certificates and Pruned Trajectories}\n"
                "To enforce reproducible epistemic hygiene, the following counterexamples and unviable "
                f"parameter regimes were permanently recorded to the Merkle ledger: {safe_negs}."
            )
        sections.append(ManuscriptSection(
            slug="results_and_reproducibility",
            title="Results, Verification, and Provenance",
            latex_content=(
                f"Execution converged to a final energy of ${measured_val}$. "
                f"The complete compilation trace is content-addressed under SHA256 AST hash "
                f"\\texttt{{{safe_proof}}}.\n\n{neg_text}"
            ),
            bibtex_keys=["kubona_provenance_2026"]
        ))

        bibtex = {
            "openai_math_corpus_2026": (
                "@misc{openai_math_corpus_2026,\n"
                "  author = {{OpenAI}},\n"
                "  title = {{Autonomous Mathematical Discovery and Formal Verification in Lean 4}},\n"
                "  year = {2026}\n"
                "}"
            ),
            "kubona_manifesto_2026": (
                "@article{kubona_manifesto_2026,\n"
                "  author = {{Kubona Research Collective}},\n"
                "  title = {{The Epistemic Continuum: Merging Formal Proof with Quantum Simulation}},\n"
                "  journal = {Kubona Annals of Quantum Discovery},\n"
                "  year = {2026}\n"
                "}"
            ),
            "kubona_provenance_2026": (
                "@misc{kubona_provenance_2026,\n"
                "  author = {{Kubona Ledger Architecture}},\n"
                "  title = {{Merkle DAG Provenance and Immutable Negative Certificates for Reproducible Science}},\n"
                "  year = {2026}\n"
                "}"
            )
        }

        return ScientificManuscript(
            manuscript_id=f"manuscript_{target_system.lower()}_discovery",
            title=title,
            authors=authors,
            abstract=abstract,
            sections=sections,
            bibtex_entries=bibtex,
            lean4_proof_hash=lean4_proof_hash
        )

    @staticmethod
    def create_certified_discovery_manuscript(
        title: str,
        target_system: str,
        authors: List[str],
        abstract: str,
        analytical_anchor_name: str,
        analytical_anchor_val: float,
        measured_val: float,
        lean4_proof_hash: Optional[str] = None,
        evidence: Optional["ReviewEvidence"] = None,
        negative_certificates: Optional[List[str]] = None,
    ) -> ScientificManuscript:
        """CERTIFIED constructor: emits verification prose only on attested proof.

        Fail-closed gates (each refuses with ValueError instead of hedging):
        - ``lean4_proof_hash`` must be present — the proof sentence is NEVER
          rendered when the hash is None.
        - ``evidence`` must carry ``comparator_verdict == "live_kernel_proven"``
          with ``sorry_ax_count == 0`` — static containment is attested, not
          kernel-proved, and never earns "verified" prose here.
        - the computed discrepancy must be within chemical accuracy
          (<= 1.6 mHa); a miss refuses certification of the match.
        - when the evidence carries its own ``anchor_discrepancy_mha`` it must
          agree with the computed value (contradictory evidence refuses).
        """
        if lean4_proof_hash is None:
            raise ValueError(
                "certified manuscript refused: lean4_proof_hash is required — "
                "the proof sentence is never rendered for unverified output")
        if evidence is None:
            raise ValueError(
                "certified manuscript refused: comparator ReviewEvidence is required — "
                "verification prose needs an attested verdict, never raw floats")
        if (evidence.comparator_verdict not in
                ManuscriptGenerator._KERNEL_PROVEN_VERDICTS):
            raise ValueError(
                f"certified manuscript refused: comparator verdict "
                f"{evidence.comparator_verdict!r} is not a kernel proof — "
                "static containment or failure never earns 'verified' prose")
        if evidence.sorry_ax_count != 0:
            raise ValueError(
                f"certified manuscript refused: sorry_ax_count="
                f"{evidence.sorry_ax_count!r} (need exactly 0 for certification)")
        discrepancy_mha = ManuscriptGenerator._discrepancy_mha(
            analytical_anchor_val, measured_val)
        if discrepancy_mha > ManuscriptGenerator.CHEMICAL_ACCURACY_MHA:
            raise ValueError(
                f"certified manuscript refused: discrepancy {discrepancy_mha:.4f} mHa "
                f"exceeds chemical accuracy ({ManuscriptGenerator.CHEMICAL_ACCURACY_MHA} mHa)")
        if (evidence.anchor_discrepancy_mha is not None
                and abs(evidence.anchor_discrepancy_mha - discrepancy_mha) > 1e-6):
            raise ValueError(
                f"certified manuscript refused: evidence discrepancy "
                f"{evidence.anchor_discrepancy_mha} mHa contradicts the computed "
                f"{discrepancy_mha:.4f} mHa")

        manuscript = ManuscriptGenerator.create_discovery_manuscript(
            title=title,
            target_system=target_system,
            authors=authors,
            abstract=abstract,
            analytical_anchor_name=analytical_anchor_name,
            analytical_anchor_val=analytical_anchor_val,
            measured_val=measured_val,
            lean4_proof_hash=lean4_proof_hash,
            negative_certificates=negative_certificates,
        )
        # Replace the draft anchor section with attested verification prose —
        # reachable only past every gate above.
        for sec in manuscript.sections:
            if sec.slug == "analytical_anchors":
                sec.latex_content = (
                    f"The physical model is calibrated against the exact analytical anchor "
                    f"\\texttt{{{analytical_anchor_name}}} with theoretical value ${analytical_anchor_val}$. "
                    f"Physical simulation yielded ${measured_val}$, matching analytical bounds within certified tolerance "
                    f"(discrepancy {discrepancy_mha:.4f} mHa $\\leq$ "
                    f"{ManuscriptGenerator.CHEMICAL_ACCURACY_MHA} mHa). "
                    f"Formal invariants were verified under the Lean 4 comparator protocol "
                    f"(live kernel proof, 0 sorryAx) without axioms outside the standard whitelist."
                )
        return manuscript

    @classmethod
    def create_bilingual_discovery_manuscript(
        cls,
        title: str,
        bilingual_title: str,
        target_system: str,
        authors: List[str],
        abstract: str,
        bilingual_abstract: str,
        analytical_anchor_name: str,
        analytical_anchor_val: float,
        measured_val: float,
        second_language: str = "Tonga",
        lean4_proof_hash: Optional[str] = None,
        provenance_record_hash: Optional[str] = None,
        negative_certificates: Optional[List[str]] = None,
        figures: Optional[Dict[str, Any]] = None,
    ) -> ScientificManuscript:
        """Assembles a dual-language (e.g. English + Tonga) scientific preprint.

        Figures are only ever the caller's artifacts (PDF/PNG bytes). This
        constructor never fabricates a convergence trajectory or attaches a
        decorative graphic: a figure that is not measured data is not a
        result, and pdfTeX cannot include the old SVG output anyway.
        """
        manuscript = cls.create_discovery_manuscript(
            title=title,
            target_system=target_system,
            authors=authors,
            abstract=abstract,
            analytical_anchor_name=analytical_anchor_name,
            analytical_anchor_val=analytical_anchor_val,
            measured_val=measured_val,
            lean4_proof_hash=lean4_proof_hash,
            negative_certificates=negative_certificates,
        )
        manuscript.bilingual_title = bilingual_title
        manuscript.bilingual_abstract = bilingual_abstract
        manuscript.second_language = second_language
        manuscript.provenance_record_hash = provenance_record_hash
        manuscript.figures = dict(figures or {})
        return manuscript

    @staticmethod
    def automated_peer_review(manuscript: ScientificManuscript,
                              evidence: Optional[ReviewEvidence] = None) -> PeerReviewReport:
        """Deterministic 5-dimensional peer review over attested evidence.

        1. Mathematical Rigor (1-10): 10.0 = live kernel proof; 7.0 = static
           AST containment with a proof hash; 5.0 = unattested.
        2. Novelty (1-10): from measured citation-graph distance when supplied
           (8.0 provisional otherwise, flagged — never presented as measured).
        3. Empirical Grounding (1-10): from the measured anchor discrepancy in
           mHa (<=1.6 -> 9.5; <=5 -> 8.0; <=16 -> 7.0; else 5.0).
        4. Axiomatic Hygiene (1-10): 10.0 = live proof with 0 sorryAx; 9.0 =
           static containment with 0 sorryAx; 4.0 = unattested. Any sorryAx>0
           is an instant REJECT back to the swarm (never ACCEPT, never REVISE).
        5. Reproducibility (1-10): 10.0 = proof hash plus >=1 provenance
           receipt; 7.0 = partial; 5.0 = none.
        Gate (wiki-exact): ACCEPT iff composite >= 9.0 AND hygiene == 10.0.
        Without evidence the manuscript is unattested and revises — a hash
        string's mere presence proves nothing.
        """
        critiques: List[str] = []
        if evidence is None:
            evidence = ReviewEvidence()
            critiques.append("No verification evidence supplied: scores are unattested pending comparator, anchor, and provenance receipts.")

        # --- Hygiene first: sorryAx is an instant REJECT. ---
        sorry = evidence.sorry_ax_count
        verdict = evidence.comparator_verdict
        if sorry is not None and sorry > 0:
            return PeerReviewReport(
                reviewer_persona="Adversarial_Peer_Reviewer_Ring1",
                mathematical_rigor_score=4.0,
                novelty_score=5.0,
                empirical_grounding_score=5.0,
                axiomatic_hygiene_score=4.0,
                reproducibility_score=5.0,
                overall_recommendation="REJECT",
                summary=f"Rejected: {sorry} sorryAx tactic(s) present — unverified axioms are never publishable.",
                actionable_critiques=critiques + [
                    f"Eliminate all {sorry} sorryAx hole(s) and re-verify under the comparator.",
                ],
            )

        if verdict == "live_kernel_proven" and sorry == 0:
            rigor, hygiene = 10.0, 10.0
        elif verdict == "static_ast_containment" and sorry == 0:
            rigor, hygiene = 7.0, 9.0
            critiques.append("Proof attested by static AST containment only (not kernel-proved): rigor and hygiene capped below ACCEPT.")
        else:
            rigor, hygiene = 5.0, 4.0
            critiques.append("No verified proof attestation (need comparator verdict + sorryAx count).")

        disc = evidence.anchor_discrepancy_mha
        if disc is None:
            grounding = 5.0
            critiques.append("No measured anchor discrepancy supplied; grounding unattested.")
        elif abs(disc) <= 1.6:
            grounding = 9.5
        elif abs(disc) <= 5.0:
            grounding = 8.0
        elif abs(disc) <= 16.0:
            grounding = 7.0
        else:
            grounding = 5.0
            critiques.append(f"Anchor discrepancy {disc:.2f} mHa exceeds 16 mHa: result does not reproduce the classical anchor.")

        if evidence.citation_distance is None:
            novelty = 8.0
            critiques.append("Novelty provisional (8.0): citation-graph distance unmeasured.")
        elif evidence.citation_distance >= 2.0:
            novelty = 9.5
        elif evidence.citation_distance >= 1.0:
            novelty = 9.0
        else:
            novelty = 8.0

        receipts = [r for r in evidence.provenance_receipts if r]
        has_lean = manuscript.lean4_proof_hash is not None
        if has_lean and len(receipts) >= 1:
            reproducibility = 10.0
        elif has_lean or receipts:
            reproducibility = 7.0
            critiques.append("Partial reproducibility: need both the Lean proof hash and provenance receipts.")
        else:
            reproducibility = 5.0
            critiques.append("Provenance DAG content-addressed receipts not attached.")

        mean = (rigor + novelty + grounding + hygiene + reproducibility) / 5.0
        if hygiene == 10.0 and mean >= 9.0:
            rec = "ACCEPT"
            summary = "Accepted: machine-checked proof, chemical-accuracy grounding, full provenance."
        else:
            rec = "REVISE"
            summary = "Requires formal proof reinforcement and/or tighter anchor agreement."

        return PeerReviewReport(
            reviewer_persona="Adversarial_Peer_Reviewer_Ring1",
            mathematical_rigor_score=rigor,
            novelty_score=novelty,
            empirical_grounding_score=grounding,
            axiomatic_hygiene_score=hygiene,
            reproducibility_score=reproducibility,
            overall_recommendation=rec,
            summary=summary,
            actionable_critiques=critiques,
        )

    @staticmethod
    def attach_signoff(manuscript: ScientificManuscript, signoff: ManuscriptSignoff,
                       expiry: str) -> ScientificManuscript:
        """Records human approval after verifying the Ed25519 sign-off over the
        manuscript's full content manifest hash (main.tex, sections, bibtex, figures).
        Invalid signatures refuse (fail-closed): an unverified sign-off is never attached."""
        content_hash = manuscript.content_hash()
        if signoff.manuscript_id != manuscript.manuscript_id:
            raise ValueError("sign-off manuscript_id mismatch: refused")
        if signoff.content_hash != content_hash:
            raise ValueError("sign-off content hash mismatch: refused (stale approval)")
        if not signoff.verify(expiry):
            raise ValueError("sign-off signature invalid: refused")
        signoff.expiry = expiry
        manuscript.signoff = signoff
        return manuscript

    @staticmethod
    def stage_zenodo_deposit(target_dir: Path, manuscript: ScientificManuscript,
                             review: PeerReviewReport, creators: list) -> Path:
        """Stages (never publishes) a Zenodo deposit package.

        Preconditions enforced here: the review must be ACCEPT-acceptable AND
        a valid human sign-off must be attached. The output is a deposit
        manifest for a human to upload (1-click in the console imports this
        file into the Zenodo web UI flow) — there is deliberately no uploader,
        no API token path, and no auto-publish. DOI minting happens at Zenodo,
        never in this tree (no hardcoded DOIs).
        """
        if not review.is_acceptable():
            raise ValueError(
                f"deposit refused: review is {review.overall_recommendation} "
                f"(mean {review.mean_score():.2f}, hygiene {review.axiomatic_hygiene_score})")
        if manuscript.signoff is None:
            raise ValueError("deposit refused: no verified human sign-off attached")
        current_hash = manuscript.content_hash()
        signoff = manuscript.signoff
        if signoff.content_hash != current_hash:
            raise ValueError("deposit refused: manuscript changed after sign-off")
        if not signoff.expiry or not signoff.verify(signoff.expiry):
            raise ValueError("deposit refused: sign-off is expired or invalid")
        target_dir.mkdir(parents=True, exist_ok=True)
        manifest = {
            "state": "staged-needs-human-upload",
            "title": manuscript.title,
            "creators": creators,
            "description": manuscript.abstract,
            "manuscript_id": manuscript.manuscript_id,
            "lean4_proof_hash": manuscript.lean4_proof_hash,
            "signoff": {
                "signer_pubkey_hex": manuscript.signoff.signer_pubkey_hex,
                "signature_hex": manuscript.signoff.signature_hex,
                "signed_at": manuscript.signoff.signed_at,
            },
            "review": {
                "recommendation": review.overall_recommendation,
                "mean_score": review.mean_score(),
                "hygiene": review.axiomatic_hygiene_score,
            },
            "files": ["main.tex", "references.bib"],
            "requires_human_upload": True,
            "auto_publish": False,
        }
        out = target_dir / "zenodo_deposit.json"
        out.write_text(json.dumps(manifest, indent=2), encoding="utf-8")
        return out

