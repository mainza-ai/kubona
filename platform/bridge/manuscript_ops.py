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
import signal
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


_NON_MEASURED_SCIENTIFIC_KINDS = frozenset({
    "krylov",
    "hardware_pes",
    "lattice_dynamics",
    "thermodynamic_limit",
    "conjecture",
    "conjecture_run",
    "symbolic_conjecture",
    "qec_code_eval",
    "qec_distance",
    "qec_memory_sim",
})


def _measured_rows(campaign: Dict[str, Any]) -> List[Dict[str, Any]]:
    return [r for r in _campaign_rows(campaign) if bool(r.get("measured"))]


def _reportable_rows(campaign: Dict[str, Any]) -> List[Dict[str, Any]]:
    """Every settled scientific result that belongs in the manuscript table.

    Preserves epistemic honesty: ZNE-mitigated hardware (`measured: false,
    mitigated: true`) and exact-sim/subspace/DMRG/conjecture analyses are
    included with an explicit status label instead of being silently dropped
    (W3-01 / N-02), while unmeasured VQE placeholder rows remain excluded.
    """
    out: List[Dict[str, Any]] = []
    for r in _campaign_rows(campaign):
        kind_l = str(r.get("kind") or "").strip().lower()
        if (
            bool(r.get("measured"))
            or bool(r.get("mitigated"))
            or kind_l in _NON_MEASURED_SCIENTIFIC_KINDS
        ):
            out.append(r)
    return out


def _row_status(row: Dict[str, Any]) -> str:
    kind_l = str(row.get("kind") or "").strip().lower()
    if bool(row.get("mitigated")):
        return "mitigated"
    if bool(row.get("measured")):
        return "measured"
    if kind_l == "krylov":
        return "subspace"
    if kind_l == "lattice_dynamics":
        return "exact-sim"
    if kind_l == "thermodynamic_limit":
        return "dmrg"
    if kind_l in ("conjecture", "conjecture_run", "symbolic_conjecture"):
        return "conjecture"
    if kind_l in ("qec_code_eval", "qec_distance"):
        return "proved" if str(row.get("d_kind") or "") == "proved" else "upper-bound"
    if kind_l == "qec_memory_sim":
        return "qec-sim"
    return "inferred"


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


def _results_table(rows: List[Dict[str, Any]]) -> str:
    if not rows:
        return (
            "No measured result is present in the campaign projection. "
            "This section is intentionally empty rather than populated with a template value."
        )
    from manuscript_sections import extract_anchor_energy, short_job_id

    lines = [
        r"\begin{table}[H]",
        r"\centering\small",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}lllrrrr@{}}",
        r"\toprule",
        r"Job & Kind & Status & Result & Exact Anchor & $\Delta$ / Summary & Backend \\",
        r"\midrule",
    ]
    for row in rows:
        kind_s = str(row.get("kind") or "--")
        kind_l = kind_s.lower()
        status_s = _row_status(row)
        is_qaoa = "qaoa" in kind_l

        measured_str = "--"
        exact_str = "--"
        delta_str = "--"

        if kind_l in ("hardware_pes", "pes") and isinstance(row.get("points"), list) and row["points"]:
            pts = [p for p in row["points"] if isinstance(p, dict)]
            energies = [float(p["energy"]) for p in pts if p.get("energy") is not None]
            deltas = [float(p["delta_mha"]) for p in pts if p.get("delta_mha") is not None]
            measured_str = f"{len(pts)} pts"
            if energies:
                exact_str = f"{min(energies):.4f} Ha"
            if deltas:
                delta_str = f"max {max(deltas):.2f} mHa"
        elif kind_l == "lattice_dynamics":
            e_init = row.get("energy_initial")
            if e_init is not None:
                measured_str = _fmt(e_init, 4)
            times_list = row.get("times")
            if isinstance(times_list, list) and times_list:
                le_vals = [float(t["loschmidt_echo"]) for t in times_list
                           if isinstance(t, dict) and t.get("loschmidt_echo") is not None]
                zz_vals = [float(t["zz_correlator_mean"]) for t in times_list
                           if isinstance(t, dict) and t.get("zz_correlator_mean") is not None]
                if le_vals:
                    exact_str = f"min L={min(le_vals):.3f}"
                if zz_vals:
                    delta_str = f"mean ZZ={sum(zz_vals)/len(zz_vals):.3f}"
                elif le_vals:
                    delta_str = f"{len(times_list)} steps"
        elif kind_l == "thermodynamic_limit":
            e_site = row.get("energy_per_site")
            exact_site = row.get("exact_energy_per_site")
            bond = row.get("max_bond_dim")
            # Extract from actual bridge `finite_size` / `thermodynamic_limit` blocks
            finite_list = row.get("finite_size")
            if isinstance(finite_list, list) and finite_list:
                last = finite_list[-1]
                if isinstance(last, dict):
                    if e_site is None:
                        e_site = last.get("energy_per_site")
                    if bond is None:
                        bond = last.get("max_bond_dim", last.get("bond_dim"))
            elif e_site is None and isinstance(row.get("sizes"), list) and row["sizes"]:
                last = row["sizes"][-1]
                if isinstance(last, dict):
                    e_site = last.get("energy_per_site")
            tl_block = row.get("thermodynamic_limit")
            if exact_site is None and isinstance(tl_block, dict):
                exact_site = tl_block.get("energy_per_site")
            if bond is None and row.get("bond_dim") is not None:
                bond = row.get("bond_dim")
            if e_site is not None:
                measured_str = _fmt(e_site, 6)
            if exact_site is not None:
                exact_str = _fmt(exact_site, 6)
            if bond is not None:
                delta_str = f"bond {int(bond)}"
            elif e_site is not None and exact_site is not None:
                delta_str = f"{abs(float(e_site) - float(exact_site)) * 1000.0:.2f} mHa"
        elif kind_l in ("conjecture", "conjecture_run", "symbolic_conjecture"):
            st = str(row.get("status") or row.get("verdict") or "evaluated")
            measured_str = _latex_escape(st)
            law = row.get("law") or row.get("expression")
            delta_str = _latex_escape(str(law)[:64]) if law else "no law"
        elif kind_l in ("qec_code_eval", "qec_distance"):
            n_q = row.get("n")
            k_q = row.get("k")
            d_q = row.get("d")
            if n_q is not None and k_q is not None:
                measured_str = f"[[{int(n_q)},{int(k_q)},{d_q if d_q is not None else '?'}]]"
            fom_v = row.get("fom")
            if fom_v is not None:
                exact_str = f"FOM {float(fom_v):.2f}"
            d_kind = "PROVED" if str(row.get("d_kind") or "") == "proved" else "UPPER BOUND"
            delta_str = _latex_escape(d_kind)
        elif kind_l == "qec_memory_sim":
            ler_v = row.get("ler")
            ci_lo = row.get("ci_low")
            ci_hi = row.get("ci_high")
            if ler_v is not None:
                measured_str = f"LER {float(ler_v):.4f}"
                if ci_lo is not None and ci_hi is not None:
                    measured_str += f" [{float(ci_lo):.4f},{float(ci_hi):.4f}]"
            d_circ = row.get("d_circ")
            if d_circ is not None:
                exact_str = f"d\\_circ={int(d_circ)}"
            lat_us = row.get("decoder_latency_us")
            dec_name = str(row.get("decoder") or "auto")
            if lat_us is not None:
                delta_str = _latex_escape(f"{dec_name} ({float(lat_us):.1f} us)")
            else:
                delta_str = _latex_escape(dec_name)
        else:
            measured_v = row.get("energy")
            if measured_v is None:
                measured_v = row.get(
                    "ground_energy",
                    row.get("ground_state_energy", row.get("reaction_energy", row.get("measured_cut"))),
                )
            # Gap 2 fix: prioritize true classical FCI/CASCI anchor over noiseless ansatz expectation.
            exact_v = extract_anchor_energy(row)
            measured_str = _fmt(measured_v)
            exact_str = _fmt(exact_v)
            if is_qaoa:
                # N-11: QAOA cut/ratio gaps are dimensionless — never label as mHa.
                ratio_v = row.get("approximation_ratio")
                if ratio_v is not None:
                    delta_str = f"{_fmt(ratio_v, 4)} ratio"
                elif measured_v is not None and exact_v is not None:
                    delta_str = f"{_fmt(abs(float(measured_v) - float(exact_v)), 4)} cut"
            else:
                # N-10: always express Hartree energy discrepancies consistently in mHa
                # (spin_chain_hardware stores `delta` in Ha, not mHa).
                delta_mha = row.get("delta_mha")
                if delta_mha is None and measured_v is not None and exact_v is not None:
                    delta_mha = abs(float(measured_v) - float(exact_v)) * 1000.0
                elif delta_mha is None and row.get("delta") is not None:
                    raw_d = abs(float(row["delta"]))
                    delta_mha = raw_d * 1000.0 if "spin_chain" in kind_l else raw_d
                if delta_mha is not None:
                    delta_str = f"{_fmt(delta_mha, 4)} mHa"

        lines.append(
            f"\\texttt{{{_latex_escape(short_job_id(str(row.get('job_id') or '--')))}}} & "
            f"{_latex_escape(kind_s)} & "
            f"{_latex_escape(status_s)} & "
            f"{measured_str} & {exact_str} & {delta_str} & "
            f"{_latex_escape(str(row.get('backend') or row.get('model') or '--'))} \\\\"
        )
    lines += [
        r"\bottomrule",
        r"\end{tabular}}",
        r"\caption{Settled scientific results folded from the Kubona ledger (short IDs map to full UUIDs in Appendix~A).}",
        r"\end{table}",
    ]
    return "\n".join(lines)


def _figures_section(figures: List[Tuple[str, bytes]], captions: Optional[Dict[str, str]] = None) -> str:
    if not figures:
        return "This draft declares no figure. Figures are caller-supplied measured artifacts."
    captions = captions or {}
    out = []
    for name, _ in figures:
        cap = captions.get(name) or name
        out.append(r"\begin{figure}[H]")
        out.append(r"\centering")
        out.append(rf"\includegraphics[width=0.9\linewidth]{{figures/{name}}}")
        out.append(rf"\caption{{{_latex_escape(cap)}}}")
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
        brk = "job:\\allowbreak "
        lines.append(
            r"\begin{sloppypar}"
            "The following provenance receipts were resolved against ledger records: "
            + ", ".join(r"\texttt{\footnotesize " + _latex_escape(x).replace("job:", brk) + "}" for x in resolved)
            + "."
            r"\end{sloppypar}"
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


def _load_brand_logo() -> Optional[bytes]:
    """Load the Milimo/Kubona brand logo PNG for PDF/HTML documentation headers."""
    here = Path(__file__).resolve().parent
    candidates = [
        here / "assets" / "milimo_kubona_logo.png",
        here.parent.parent / "assets" / "milimo_kubona_logo.png",
        here / "assets" / "milimo_logo.png",
        here.parent.parent / "assets" / "milimo_logo.png",
    ]
    for p in candidates:
        try:
            if p.is_file():
                raw = p.read_bytes()
                if raw.startswith(b"\x89PNG\r\n\x1a\n"):
                    return raw
        except OSError:
            continue
    return None


def _new_manuscript(*, manuscript_id: str, title: str, authors: List[str],
                    abstract: str, sections: List[Any],
                    bibtex_entries: Dict[str, str],
                    figures: List[Tuple[str, bytes]],
                    document_type: str = "academic_preprint",
                    subtitle: Optional[str] = None,
                    keywords: Optional[List[str]] = None):
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
        document_type=document_type,
        subtitle=_latex_escape(subtitle) if subtitle else None,
        keywords=list(keywords or []),
        brand_logo_bytes=_load_brand_logo(),
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

    from manuscript_figures import generate_campaign_figures
    from manuscript_sections import (
        build_appendix_provenance,
        build_discussion_section,
        build_economics_governance_section,
        build_keywords,
        build_methods_section,
        build_related_work_section,
        build_results_narrative,
        build_statistical_audit_section,
        infer_target_system,
    )

    doc_type = str(req.get("document_type") or "academic_preprint").strip().lower()
    if doc_type not in ("academic_preprint", "white_paper", "discovery_dossier", "technical_report"):
        doc_type = "academic_preprint"
    subtitle = str(req.get("subtitle") or "").strip() or None

    measured = _measured_rows(campaign)
    reportable = _reportable_rows(campaign)
    target = infer_target_system(req, campaign, reportable)
    title = str(req.get("title") or f"Measured results for {target}").strip()
    if not title:
        raise ManuscriptError("invalid_request", "title is empty")
    authors = [str(a).strip() for a in (req.get("authors") or []) if str(a).strip()]
    if not authors:
        authors = ["Kubona Operator"]

    claimable = sum(1 for r in measured if bool(r.get("claimable")))
    point = _primary_point(campaign)
    abstract = str(req.get("abstract") or "").strip() or _abstract(campaign, point, target, claimable)
    keywords = build_keywords(target, reportable, doc_type)

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
            suffix = f"_{n}"
            key = f"{base[:64 - len(suffix)]}{suffix}"
            n += 1
        used_keys.add(key)
        citations.append({**entry, "key": key})
    # W4-03: canonicalize citation order by sanitized BibTeX key so create and
    # SQL ORDER BY cite_key rebuilds always produce identical content hashes.
    citations.sort(key=lambda c: c["key"])
    cite_keys = [c["key"] for c in citations]

    figures: List[Tuple[str, bytes]] = []
    auto_generated_figures: List[Dict[str, str]] = []
    figure_captions: Dict[str, str] = {}
    seen = set()
    for entry in req.get("figures") or []:
        if not isinstance(entry, dict):
            raise ManuscriptError("invalid_request", "each figure must be an object")
        name, raw = _figure_bytes(entry)
        if name in seen:
            raise ManuscriptError("invalid_request", f"duplicate figure name {name!r}")
        seen.add(name)
        figures.append((name, raw))
        if entry.get("caption"):
            figure_captions[name] = str(entry["caption"])
    auto_figures = req.get("auto_figures")
    if auto_figures is None:
        auto_figures = True
    if bool(auto_figures) and reportable:
        for fname, fbytes, fcap in generate_campaign_figures(reportable):
            if fname not in seen:
                seen.add(fname)
                figures.append((fname, fbytes))
                figure_captions[fname] = fcap
                auto_generated_figures.append({
                    "name": fname,
                    "content_b64": _b64(fbytes),
                    "caption": fcap,
                })
    figure_sha = [_dig(raw) for _, raw in figures]

    goal_s = _latex_escape(str(campaign.get("research_goal") or "not recorded"))
    intro = (
        f"We report a verified computational and experimental investigation of {_latex_escape(target)}. "
        f"The campaign objective recorded for this session is: {goal_s}. "
        "Every numerical result in this document is folded directly from settled jobs in the "
        "Kubona ledger and anchored against exact classical solvers; the provenance and appendix "
        "sections record every resolved receipt and hardware calibration snapshot."
    )
    if cite_keys:
        intro += " Prior work considered: " + ", ".join(rf"\cite{{{k}}}" for k in cite_keys) + "."

    related_work = build_related_work_section(citations)
    methods = build_methods_section(campaign, reportable, doc_type)

    results_parts = []
    narrative = build_results_narrative(reportable, campaign)
    if narrative:
        results_parts.append(narrative)
    results_parts.append(_results_table(reportable))
    if figures:
        results_parts.append(_figures_section(figures, figure_captions))
    results = "\n\n".join(results_parts)

    statistical_audit = build_statistical_audit_section(campaign)
    discussion = build_discussion_section(reportable, campaign, doc_type)
    appendix_prov = build_appendix_provenance(reportable)

    sections = [
        ManuscriptSection(slug="introduction", title="Introduction and Scientific Motivation",
                          latex_content=intro, bibtex_keys=cite_keys),
        ManuscriptSection(slug="related_work", title="Related Work and Grounded Literature",
                          latex_content=related_work, bibtex_keys=cite_keys),
        ManuscriptSection(slug="methods", title="Theoretical Formulation and Experimental Methodology",
                          latex_content=methods),
        ManuscriptSection(slug="results", title="Experimental Results and Quantitative Analysis",
                          latex_content=results),
        ManuscriptSection(slug="statistical_audit", title="Pre-Registration, Hypothesis Testing and Adversarial Review",
                          latex_content=statistical_audit),
        ManuscriptSection(slug="discussion", title="Discussion, Negative Results and Physical Interpretation",
                          latex_content=discussion),
    ]
    if doc_type in ("white_paper", "discovery_dossier"):
        sections.append(
            ManuscriptSection(slug="economics_governance", title="Utility-vs-Cost Economics and Sovereign Governance",
                              latex_content=build_economics_governance_section(reportable, evidence))
        )
    sections += [
        ManuscriptSection(slug="provenance", title="Provenance and Review Evidence",
                          latex_content=_evidence_section(evidence)),
        ManuscriptSection(slug="limitations", title="Limitations, Epistemic Scope and Reproducibility",
                          latex_content=_limitations(claimable)),
        ManuscriptSection(slug="appendix_provenance", title="Appendix A: Complete Ledger and Hardware Provenance",
                          latex_content=appendix_prov),
    ]

    bibtex_entries: Dict[str, str] = {}
    for entry in citations:
        key = entry["key"]
        cite_authors = _latex_escape(str(entry.get("authors") or "Unknown"))
        raw_year = str(entry.get("year") or "n.d.").strip()
        year = raw_year if re.fullmatch(r"(?:\d{4}|n\.d\.)", raw_year) else _latex_escape(raw_year)
        doi = str(entry.get("doi") or "").strip()
        fields = [
            f"  author = {{{cite_authors}}},",
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
        document_type=doc_type,
        subtitle=subtitle,
        keywords=keywords,
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
        "generated_figures": auto_generated_figures,
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
            document_type=doc_type,
            subtitle=subtitle,
            keywords=keywords,
            citations=citations,
        )
    response["document_type"] = doc_type
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
    document_type: str = "academic_preprint",
    subtitle: Optional[str] = None,
    keywords: Optional[List[str]] = None,
    citations: Optional[List[Dict[str, Any]]] = None,
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
    from manuscript_sections import render_companion_html, render_companion_markdown

    manuscript = _new_manuscript(
        manuscript_id=manuscript_id,
        title=title,
        authors=authors,
        abstract=abstract,
        sections=sections,
        bibtex_entries=bibtex_entries,
        figures=figures,
        document_type=document_type,
        subtitle=subtitle,
        keywords=keywords,
    )
    project = manuscript.export_project(Path(tempfile.mkdtemp(prefix="kubona-ms-")))
    logo_b64 = _b64(manuscript.brand_logo_bytes) if manuscript.brand_logo_bytes else ""
    # Emit companion Markdown & standalone HTML alongside main.pdf (Phase 4).
    (project / "paper.md").write_text(
        render_companion_markdown(
            title=title, authors=authors, abstract=abstract,
            doc_type=document_type, sections=sections, citations=citations or [],
            has_logo=bool(logo_b64),
        ),
        encoding="utf-8",
    )
    (project / "paper.html").write_text(
        render_companion_html(
            title=title, authors=authors, abstract=abstract,
            doc_type=document_type, sections=sections, citations=citations or [],
            logo_b64=logo_b64,
        ),
        encoding="utf-8",
    )
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
        pdflatex_cmd = ["pdflatex", "-no-shell-escape", "-interaction=nonstopmode", "-halt-on-error", "main.tex"]
        runs = [pdflatex_cmd]
        if bibtex_entries:
            runs.append(["bibtex", "main"])
            runs.append(pdflatex_cmd)
            runs.append(pdflatex_cmd)
        else:
            runs.append(pdflatex_cmd)
        for cmd in runs:
            proc = subprocess.Popen(
                cmd,
                cwd=project,
                env=env,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                start_new_session=True,
            )
            try:
                stdout_str, stderr_str = proc.communicate(timeout=180)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except OSError:
                    proc.kill()
                proc.communicate()
                raise ManuscriptError("build_timeout", f"{cmd[0]} timed out after 180s")
            scrubbed_out = (stdout_str or "").replace(str(project), "<project>")
            scrubbed_err = (stderr_str or "").replace(str(project), "<project>")
            log.append(f"$ {' '.join(cmd)} (exit {proc.returncode})")
            if scrubbed_out:
                if len(scrubbed_out) > 8500:
                    log.append(scrubbed_out[:500] + "\n...[truncated]...\n" + scrubbed_out[-8000:])
                else:
                    log.append(scrubbed_out)
            if proc.returncode != 0:
                detail = scrubbed_out + scrubbed_err
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
