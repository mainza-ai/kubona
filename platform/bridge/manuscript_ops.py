"""Server-side manuscript authoring and reproducible LaTeX builds.

This module is the bridge-side half of the publication pipeline. Go owns
identity, tenancy, persistence, evidence resolution, and signoff; this module
owns prose composition, peer review, and the LaTeX build.

Honesty contract:
  * Every number in the prose comes from the campaign projection passed in.
    There is no fallback trajectory and no decorative figure.
  * Verification prose is never emitted from a hash string. The review only
    reaches ACCEPT on a comparator verdict the Go layer resolved against the
    ledger (``live_kernel_proven`` + exactly zero sorryAx).
  * Unresolved receipts are printed as unresolved; they never earn
    reproducibility credit.
  * The build is byte-reproducible: fixed ``SOURCE_DATE_EPOCH``, pdfTeX
    date/trailer suppression, and no wall-clock or hostname in the source.
"""

from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

_MAX_FIGURE_BYTES = 8 * 1024 * 1024
_MAX_TOTAL_ARTIFACT_BYTES = 32 * 1024 * 1024
_MAX_BUILD_LOG = 6000
_FIGURE_NAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*\.(png|pdf|jpg|jpeg)$")
_CITE_KEY_RE = re.compile(r"[^A-Za-z0-9_]")


class ManuscriptError(Exception):
    """Typed refusal: the caller maps ``code`` onto an HTTP status."""

    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code
        self.message = message


def _dig(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _latex_escape(text: str) -> str:
    """Escape plain text for LaTeX (titles, captions, prose fields)."""
    replacements = {
        "\\": r"\textbackslash{}",
        "&": r"\&",
        "%": r"\%",
        "$": r"\$",
        "#": r"\#",
        "_": r"\_",
        "{": r"\{",
        "}": r"\}",
        "~": r"\textasciitilde{}",
        "^": r"\textasciicircum{}",
    }
    return "".join(replacements.get(ch, ch) for ch in text)


def _fmt(value: Optional[float], digits: int = 6) -> str:
    if value is None:
        return "--"
    return f"{float(value):.{digits}f}"


def _figure_bytes(entry: Dict[str, Any]) -> Tuple[str, bytes]:
    name = str(entry.get("name") or "")
    if not _FIGURE_NAME_RE.match(name) or ".." in name or "/" in name:
        raise ManuscriptError("invalid_request", f"figure name {name!r} is not a safe PNG/PDF/JPEG name")
    import base64

    try:
        raw = base64.b64decode(str(entry.get("content_b64") or ""), validate=True)
    except Exception as exc:  # noqa: BLE001 - refuse malformed input
        raise ManuscriptError("invalid_request", f"figure {name!r} is not valid base64: {exc}") from exc
    if not raw:
        raise ManuscriptError("invalid_request", f"figure {name!r} is empty")
    if len(raw) > _MAX_FIGURE_BYTES:
        raise ManuscriptError("invalid_request", f"figure {name!r} exceeds {_MAX_FIGURE_BYTES} bytes")
    if raw.startswith(b"\x89PNG\r\n\x1a\n") or raw.startswith(b"%PDF-") or raw.startswith(b"\xff\xd8\xff"):
        return name, raw
    raise ManuscriptError("invalid_request", f"figure {name!r} is not PNG, PDF, or JPEG data")


def _campaign_rows(campaign: Dict[str, Any]) -> List[Dict[str, Any]]:
    """Every measured point, including the primary campaign measurement.

    VQE/PES folds put the primary point in ``measurement``/``notebook`` and
    other science results in ``experiments``. A manuscript that read only
    ``experiments`` would print "no measured result" for a real VQE run, so
    the primary point is promoted into the same row shape exactly once.
    """
    rows = campaign.get("experiments")
    out = [r for r in rows if isinstance(r, dict)] if isinstance(rows, list) else []
    measurement = campaign.get("measurement")
    if isinstance(measurement, dict) and bool(measurement.get("measured")) \
            and measurement.get("energy") is not None:
        job_id = str(measurement.get("job_id") or "primary")
        if not any(str(r.get("job_id") or "") == job_id for r in out):
            exact = measurement.get("fci_exact_energy", measurement.get("classical_anchor_value"))
            delta = None
            if exact is not None:
                delta = abs(float(measurement["energy"]) - float(exact)) * 1000.0
            out.append({
                "job_id": job_id,
                "kind": str(measurement.get("method") or "vqe"),
                "energy": measurement.get("energy"),
                "energy_exact": exact,
                "delta": delta,
                "backend": measurement.get("backend") or measurement.get("measurement_backend"),
                "measured": True,
                "claimable": bool(measurement.get("claimable")),
            })
    return out


def _measured_rows(campaign: Dict[str, Any]) -> List[Dict[str, Any]]:
    return [r for r in _campaign_rows(campaign) if bool(r.get("measured"))]


def _primary_point(campaign: Dict[str, Any]) -> Optional[Tuple[float, float, float]]:
    """Return (measured, exact_anchor, delta_mha) for the primary molecular point.

    Only measured values with an exact anchor qualify. The notebook is the
    canonical fold; a measurement without an anchor has no discrepancy.
    """
    notebook = campaign.get("notebook") or {}
    if isinstance(notebook, dict):
        q = notebook.get("quantum_energy")
        c = notebook.get("classical_energy")
        d = notebook.get("delta_mha")
        if q is not None and c is not None and d is not None:
            return float(q), float(c), abs(float(d))
    measurement = campaign.get("measurement") or {}
    if isinstance(measurement, dict) and bool(measurement.get("measured")):
        energy = measurement.get("energy")
        anchor = measurement.get("fci_exact_energy", measurement.get("classical_anchor_value"))
        if energy is not None and anchor is not None:
            return float(energy), float(anchor), abs(float(energy) - float(anchor)) * 1000.0
    return None


def _abstract(campaign: Dict[str, Any], point: Optional[Tuple[float, float, float]],
              target: str, claimable: int) -> str:
    measured = _measured_rows(campaign)
    if not measured:
        return (
            f"This draft covers {target}. The campaign projection holds no measured result, "
            "so no energy, discrepancy, accuracy, or advantage claim is made. Sections below "
            "record the plan and the provenance requirements the run must satisfy."
        )
    backends = sorted({str(r.get("backend") or r.get("model") or "unrecorded") for r in measured})
    kinds = sorted({str(r.get("kind") or "unrecorded") for r in measured})
    parts = [
        f"This draft reports {len(measured)} measured result(s) for {target} "
        f"({', '.join(kinds)}) recorded through the Kubona ledger."
    ]
    if point is not None:
        energy, anchor, delta = point
        parts.append(
            f"The primary point is {energy:.6f} Ha against the exact anchor {anchor:.6f} Ha "
            f"(discrepancy {delta:.4f} mHa; chemical accuracy is "
            f"{'met at this scale' if delta <= 1.6 else 'not met'})."
        )
    parts.append(f"Measured backends: {', '.join(backends)}.")
    parts.append(
        f"{claimable} result(s) carry a claimable flag; claimability is scoped to the recorded "
        "anchor and backend and is not a novelty or advantage claim."
    )
    return " ".join(parts)


def _results_table(measured: List[Dict[str, Any]]) -> str:
    if not measured:
        return (
            "No measured result is present in the campaign projection. "
            "This section is intentionally empty rather than populated with a template value."
        )
    lines = [
        r"\begin{table}[h]",
        r"\centering",
        r"\begin{tabular}{llrrrr}",
        r"\hline",
        r"Job & Kind & Measured & Exact anchor & $\Delta$ (mHa) & Backend \\",
        r"\hline",
    ]
    for row in measured:
        measured_v = row.get("energy")
        if measured_v is None:
            measured_v = row.get("ground_energy", row.get("reaction_energy", row.get("measured_cut")))
        exact_v = row.get("energy_exact")
        if exact_v is None:
            exact_v = row.get("ground_energy_exact", row.get("reaction_energy_exact", row.get("fci_exact_energy")))
        delta_v = row.get("delta")
        if delta_v is None and measured_v is not None and exact_v is not None:
            delta_v = abs(float(measured_v) - float(exact_v)) * 1000.0
        lines.append(
            f"{_latex_escape(str(row.get('job_id') or '--'))} & "
            f"{_latex_escape(str(row.get('kind') or '--'))} & "
            f"{_fmt(measured_v)} & {_fmt(exact_v)} & {_fmt(delta_v, 4)} & "
            f"{_latex_escape(str(row.get('backend') or row.get('model') or '--'))} \\\\"
        )
    lines += [r"\hline", r"\end{tabular}", r"\caption{Measured results folded from settled jobs.}", r"\end{table}"]
    return "\n".join(lines)


def _figures_section(figures: List[Tuple[str, bytes]]) -> str:
    if not figures:
        return "This draft declares no figure. Figures are caller-supplied measured artifacts."
    out = []
    for name, _ in figures:
        out.append(r"\begin{figure}[h]")
        out.append(r"\centering")
        out.append(rf"\includegraphics[width=0.9\linewidth]{{figures/{name}}}")
        out.append(rf"\caption{{{_latex_escape(name)}}}")
        out.append(r"\end{figure}")
    return "\n".join(out)


_MAX_LISTED_RECEIPTS = 40


def _evidence_section(evidence: Dict[str, Any]) -> str:
    resolved = [str(x) for x in evidence.get("resolved_receipts") or []]
    unresolved = [str(x) for x in evidence.get("unresolved_receipts") or []]
    omitted = 0
    if len(resolved) > _MAX_LISTED_RECEIPTS:
        omitted = len(resolved) - _MAX_LISTED_RECEIPTS
        resolved = resolved[:_MAX_LISTED_RECEIPTS]
    lines = []
    if resolved:
        lines.append(
            "The following provenance receipts were resolved against ledger records: "
            + ", ".join(rf"\texttt{{{_latex_escape(x)}}}" for x in resolved) + "."
        )
    else:
        lines.append("No provenance receipt was resolved against the ledger for this draft.")
    if omitted:
        lines.append(
            f"{omitted} further resolved receipt(s) are omitted here; the signed "
            "experiment export carries the complete list."
        )
    if unresolved:
        lines.append(
            "The following supplied references did NOT resolve and earn no credit: "
            + ", ".join(rf"\texttt{{{_latex_escape(x)}}}" for x in unresolved) + "."
        )
    if evidence.get("lean_proof_verified") and evidence.get("lean_proof_hash"):
        lines.append(
            "A comparator-linked kernel proof was found for hash "
            rf"\texttt{{{_latex_escape(str(evidence.get('lean_proof_hash')))}}} "
            "with exactly zero sorryAx."
        )
    else:
        lines.append(
            "No comparator-linked kernel proof was found. Formal verification is "
            r"\textbf{unverified} in this draft; no proof claim is made."
        )
    return "\n\n".join(lines)


def _limitations(claimable: int) -> str:
    return (
        "This draft does not claim quantum advantage, novelty, or general correctness. "
        f"{claimable} result(s) are claimable only against the recorded classical anchor and "
        "backend. Results measured on a simulator are not hardware evidence; hardware results "
        "carry the provider job id and calibration snapshot recorded with the job. "
        "Any figure is the caller's artifact; the text reports only numbers present in the "
        "campaign projection."
    )


def _new_manuscript(*, manuscript_id: str, title: str, authors: List[str],
                    abstract: str, sections: List[Any],
                    bibtex_entries: Dict[str, str],
                    figures: List[Tuple[str, bytes]]):
    """Build the in-memory manuscript with plain prose escaped exactly once.

    Section bodies are LaTeX by construction and are never escaped here;
    title, authors, and abstract are operator/agent prose and must not be
    able to inject markup (an underscore already breaks pdflatex).
    """
    from kubona.science.manuscript import ScientificManuscript

    return ScientificManuscript(
        manuscript_id=manuscript_id,
        title=_latex_escape(title),
        authors=[_latex_escape(a) for a in authors],
        abstract=_latex_escape(abstract),
        sections=sections,
        bibtex_entries=bibtex_entries,
        figures={name: raw for name, raw in figures},
    )


def compose(req: Dict[str, Any]) -> Dict[str, Any]:
    """Compose a manuscript draft and (optionally) build it.

    ``req`` is the JSON contract sent by the Go API. The response is the
    stored draft plus, when ``build`` is true, the artifact bytes and hashes.
    """
    from kubona.science.manuscript import (
        ManuscriptGenerator,
        ManuscriptSection,
        ReviewEvidence,
    )

    campaign = req.get("campaign") or {}
    if not isinstance(campaign, dict):
        raise ManuscriptError("invalid_request", "campaign must be an object")
    evidence = req.get("evidence") or {}
    if not isinstance(evidence, dict):
        raise ManuscriptError("invalid_request", "evidence must be an object")

    target = str(req.get("target_system") or campaign.get("target_system") or "an unrecorded target")
    title = str(req.get("title") or f"Measured results for {target}").strip()
    if not title:
        raise ManuscriptError("invalid_request", "title is empty")
    authors = [str(a).strip() for a in (req.get("authors") or []) if str(a).strip()]
    if not authors:
        authors = ["Kubona Operator"]

    measured = _measured_rows(campaign)
    claimable = sum(1 for r in measured if bool(r.get("claimable")))
    point = _primary_point(campaign)
    abstract = str(req.get("abstract") or "").strip() or _abstract(campaign, point, target, claimable)

    citations: List[Dict[str, Any]] = []
    used_keys: set[str] = set()
    for entry in req.get("citations") or []:
        if not isinstance(entry, dict):
            continue
        raw_key = str(entry.get("key") or entry.get("doi") or entry.get("title") or "")
        key = _CITE_KEY_RE.sub("_", raw_key).strip("_")[:64]
        if not key:
            continue
        # Two distinct references can sanitize to the same BibTeX key
        # ("10.1/x" and "10.1_x"). Silently reusing it makes the PDF cite the
        # wrong entry; suffix deterministically instead.
        base = key
        n = 2
        while key in used_keys:
            key = f"{base}_{n}"[:64]
            n += 1
        used_keys.add(key)
        citations.append({**entry, "key": key})
    cite_keys = [c["key"] for c in citations]

    figures: List[Tuple[str, bytes]] = []
    seen = set()
    for entry in req.get("figures") or []:
        if not isinstance(entry, dict):
            raise ManuscriptError("invalid_request", "each figure must be an object")
        name, raw = _figure_bytes(entry)
        if name in seen:
            raise ManuscriptError("invalid_request", f"duplicate figure name {name!r}")
        seen.add(name)
        figures.append((name, raw))
    figure_sha = [_dig(raw) for _, raw in figures]

    intro = (
        f"We report the measured state of {_latex_escape(target)}. The campaign objective recorded for this "
        f"session is: {_latex_escape(str(campaign.get('research_goal') or 'not recorded'))}. "
        "Every value in this manuscript comes from a settled job in the Kubona ledger; "
        "the provenance section names the receipts that were resolved."
    )
    if cite_keys:
        intro += " Prior work considered: " + ", ".join(rf"\cite{{{k}}}" for k in cite_keys) + "."

    methods = (
        "Experiments were executed through the Kubona bridge and settled by the Go job rail, "
        "which records a signed result receipt and the cost/usage settlement for each job. "
        "The campaign projection is the single source for the tables below; no number is "
        "re-derived from prose or re-entered by the author."
    )

    results = _results_table(measured)
    if figures:
        results += "\n\n" + _figures_section(figures)

    sections = [
        ManuscriptSection(slug="introduction", title="Introduction", latex_content=intro,
                          bibtex_keys=cite_keys),
        ManuscriptSection(slug="methods", title="Methods and Execution", latex_content=methods),
        ManuscriptSection(slug="results", title="Results", latex_content=results),
        ManuscriptSection(slug="provenance", title="Provenance and Review Evidence",
                          latex_content=_evidence_section(evidence)),
        ManuscriptSection(slug="limitations", title="Limitations and Claims",
                          latex_content=_limitations(claimable)),
    ]

    bibtex_entries: Dict[str, str] = {}
    for entry in citations:
        key = entry["key"]
        authors = _latex_escape(str(entry.get("authors") or "Unknown"))
        year = str(entry.get("year") or "n.d.")
        doi = str(entry.get("doi") or "").strip()
        fields = [
            f"  author = {{{authors}}},",
            f"  title = {{{_latex_escape(str(entry.get('title') or key))}}},",
            f"  year = {{{year}}},",
        ]
        if doi:
            fields.append(f"  doi = {{{_latex_escape(doi)}}},")
        bibtex_entries[key] = "@misc{" + key + ",\n" + "\n".join(fields) + "\n}"

    manuscript = _new_manuscript(
        manuscript_id=str(req.get("manuscript_id") or "ms_draft"),
        title=title,
        authors=authors,
        abstract=abstract,
        sections=sections,
        bibtex_entries=bibtex_entries,
        figures=figures,
    )
    # Attach the proof hash only when the comparator attestation is verified;
    # a supplied-but-unverified hash must not earn reproducibility credit.
    if evidence.get("lean_proof_verified") and evidence.get("lean_proof_hash"):
        manuscript.lean4_proof_hash = str(evidence["lean_proof_hash"])

    review_evidence = ReviewEvidence(
        comparator_verdict=(
            str(evidence.get("comparator_verdict"))
            if evidence.get("lean_proof_verified") and evidence.get("comparator_verdict")
            else None
        ),
        sorry_ax_count=(
            int(evidence["sorry_ax_count"])
            if evidence.get("lean_proof_verified") and evidence.get("sorry_ax_count") is not None
            else None
        ),
        anchor_discrepancy_mha=(point[2] if point is not None else None),
        provenance_receipts=[str(x) for x in evidence.get("resolved_receipts") or []],
        citation_distance=(
            float(evidence["citation_distance"])
            if evidence.get("citation_distance") is not None
            else None
        ),
    )
    review = ManuscriptGenerator.automated_peer_review(manuscript, review_evidence)

    main_tex = manuscript.compile_latex()
    bibtex = manuscript.compile_bibtex()

    review_json = {
        "reviewer": review.reviewer_persona,
        "rigor": review.mathematical_rigor_score,
        "novelty": review.novelty_score,
        "grounding": review.empirical_grounding_score,
        "hygiene": review.axiomatic_hygiene_score,
        "reproducibility": review.reproducibility_score,
        "recommendation": review.overall_recommendation,
        "mean_score": review.mean_score(),
        "acceptable": review.is_acceptable(),
        "summary": review.summary,
        "critiques": list(review.actionable_critiques),
        "resolved_receipts": [str(x) for x in evidence.get("resolved_receipts") or []],
        "unresolved_receipts": [str(x) for x in evidence.get("unresolved_receipts") or []],
    }

    response: Dict[str, Any] = {
        "ok": True,
        "manuscript_id": manuscript.manuscript_id,
        "title": title,
        "abstract": abstract,
        "sections": [
            {"slug": s.slug, "title": s.title, "latex": s.latex_content} for s in sections
        ],
        "main_tex": main_tex,
        "bibtex": bibtex,
        "figure_sha256": dict(zip([n for n, _ in figures], figure_sha)),
        "citations": citations,
        "review": review_json,
        "build": None,
    }

    if req.get("build"):
        response["build"] = build_project(
            title=title,
            authors=authors,
            abstract=abstract,
            sections=sections,
            bibtex_entries=bibtex_entries,
            figures=figures,
            epoch=int(req.get("source_date_epoch") or 0),
            manuscript_id=manuscript.manuscript_id,
        )
    return response


def _pdflatex_available(engine: str) -> bool:
    return shutil.which(engine) is not None


def build_project(
    *,
    title: str,
    authors: List[str],
    abstract: str,
    sections: List[Any],
    bibtex_entries: Dict[str, str],
    figures: List[Tuple[str, bytes]],
    epoch: int,
    manuscript_id: str,
) -> Dict[str, Any]:
    """Compile the project and return artifacts with hashes.

    Refuses (``build_unavailable``) when pdfTeX is absent: a deployment that
    cannot compile must not pretend it produced a paper.
    """
    if not _pdflatex_available("pdflatex"):
        raise ManuscriptError(
            "build_unavailable",
            "pdfTeX is not installed in this compute plane; the manuscript cannot be built",
        )
    manuscript = _new_manuscript(
        manuscript_id=manuscript_id,
        title=title,
        authors=authors,
        abstract=abstract,
        sections=sections,
        bibtex_entries=bibtex_entries,
        figures=figures,
    )
    project = manuscript.export_project(Path(tempfile.mkdtemp(prefix="kubona-ms-")))
    try:
        env = {
            "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
            "HOME": str(project),
            "TEXMFVAR": str(project / ".texmf-var"),
            "SOURCE_DATE_EPOCH": str(epoch or 1700000000),
            "FORCE_SOURCE_DATE": "1",
            "openout_any": "p",
            "openin_any": "p",
        }
        log: List[str] = []
        runs = [["pdflatex", "-interaction=nonstopmode", "-halt-on-error", "main.tex"]]
        if bibtex_entries:
            runs.append(["bibtex", "main"])
            runs.append(["pdflatex", "-interaction=nonstopmode", "-halt-on-error", "main.tex"])
            runs.append(["pdflatex", "-interaction=nonstopmode", "-halt-on-error", "main.tex"])
        else:
            runs.append(["pdflatex", "-interaction=nonstopmode", "-halt-on-error", "main.tex"])
        for cmd in runs:
            proc = subprocess.run(
                cmd, cwd=project, env=env, capture_output=True, text=True, timeout=180,
            )
            log.append(f"$ {' '.join(cmd)} (exit {proc.returncode})")
            if proc.stdout:
                log.append(proc.stdout[-8000:])
            if proc.returncode != 0:
                detail = (proc.stdout or "") + (proc.stderr or "")
                raise ManuscriptError(
                    "build_failed",
                    f"{cmd[0]} failed: {detail[-_MAX_BUILD_LOG:]}",
                )
        pdf_path = project / "main.pdf"
        if not pdf_path.exists():
            raise ManuscriptError("build_failed", "pdflatex produced no main.pdf")
        if pdf_path.stat().st_size > _MAX_FIGURE_BYTES:
            raise ManuscriptError(
                "build_failed",
                f"main.pdf exceeds {_MAX_FIGURE_BYTES} bytes; refusing to read it into memory",
            )
        pdf = pdf_path.read_bytes()
        if not pdf.startswith(b"%PDF-") or len(pdf) < 1024:
            raise ManuscriptError("build_failed", "main.pdf is not a plausible PDF")

        # Keep a scrubbed build log: it is the diagnostic record for a
        # successful build (the error path already returns the tail). The
        # project temp path is replaced so the artifact carries no host paths.
        (project / "build.log").write_text(
            "\n".join(log).replace(str(project), "<project>"), encoding="utf-8")

        artifacts: List[Dict[str, Any]] = []
        total = 0
        for path in sorted(project.rglob("*")):
            if not path.is_file():
                continue
            rel = path.relative_to(project).as_posix()
            if rel.startswith(".") or rel == "main.log" or rel.endswith((".aux", ".out", ".toc", ".blg")):
                continue
            size = path.stat().st_size
            if size > _MAX_FIGURE_BYTES:
                raise ManuscriptError("build_failed", f"artifact {rel} exceeds the size cap")
            total += size
            if total > _MAX_TOTAL_ARTIFACT_BYTES:
                raise ManuscriptError("build_failed", "artifact set exceeds the total size cap")
            data = path.read_bytes()
            artifacts.append({"name": rel, "sha256": _dig(data), "content_b64": _b64(data)})
        return {
            "build_hash": _dig(pdf),
            "pdf_sha256": _dig(pdf),
            "log": "\n".join(log)[-_MAX_BUILD_LOG:],
            "artifacts": artifacts,
        }
    finally:
        shutil.rmtree(project, ignore_errors=True)


def _b64(data: bytes) -> str:
    import base64

    return base64.b64encode(data).decode("ascii")


def run(req: Dict[str, Any]) -> Dict[str, Any]:
    """Entry point used by the gRPC servicer; maps refusals to a response."""
    try:
        return compose(req)
    except ManuscriptError as exc:
        return {"ok": False, "error_code": exc.code, "error": exc.message}
    except Exception as exc:  # noqa: BLE001 - never leak a stack trace over gRPC
        return {"ok": False, "error_code": "internal", "error": f"manuscript composition failed: {exc}"}
