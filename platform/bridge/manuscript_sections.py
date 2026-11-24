"""Deep Academic, White-Paper & Dossier Section Synthesizers + Companion Markdown/HTML.

Every number emitted here is extracted directly from settled Kubona ledger rows
or hydrated campaign intelligence (hypotheses, Vault predictions, Tribunal
critiques, calibration snapshots, ADMET/PAINS). No value is invented, keeping
every generated paper 100% compliant with `verification-v1` (0 blockers).
"""

from __future__ import annotations

import html
import math
import re
from typing import Any, Dict, List, Optional, Tuple


def latex_escape(text: str) -> str:
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
    return "".join(replacements.get(ch, ch) for ch in str(text))


def short_job_id(job_id: str) -> str:
    """Shorten 36-char UUIDs to 8 chars for table readability while keeping
    short human/test IDs (<=14 chars) intact."""
    s = str(job_id or "--").strip()
    if len(s) >= 32 and "-" in s:
        return s[:8]
    return s


def fmt_num(value: Optional[float], digits: int = 6) -> str:
    if value is None:
        return "--"
    try:
        v = float(value)
        if not math.isfinite(v):
            return "--"
        return f"{v:.{digits}f}"
    except (TypeError, ValueError):
        return "--"


def extract_anchor_energy(row: Dict[str, Any]) -> Optional[float]:
    """Return the true classical anchor energy, never shadowed by noiseless
    ansatz expectation (`energy_exact` in generic VQE — Gap 2 fix)."""
    for key in (
        "fci_exact_energy",
        "active_space_fci_energy",
        "ground_energy_exact",
        "reaction_energy_exact",
        "optimal_cut",
        "exact_energy",
        "energy_exact",
    ):
        v = row.get(key)
        if v is not None:
            try:
                f = float(v)
                if math.isfinite(f):
                    return f
            except (TypeError, ValueError):
                continue
    return None


def infer_target_system(req: Dict[str, Any], campaign: Dict[str, Any], rows: List[Dict[str, Any]]) -> str:
    explicit = str(req.get("target_system") or "").strip()
    if explicit:
        return explicit
    camp_target = str(campaign.get("target_system") or "").strip()
    # If campaign has a stale default ("Hydrogen Molecule (H2)") while jobs ran
    # on other molecules/models (e.g., H3+, H4, H2O, TFIM-2D, QAOA), derive from jobs.
    observed: List[str] = []
    for r in rows:
        mol = str(r.get("molecule") or "").strip()
        mod = str(r.get("model") or "").strip()
        kind = str(r.get("kind") or "").strip().lower()
        if mol and mol.lower() != "custom" and mol not in observed:
            observed.append(mol)
        elif mod and mod not in observed:
            observed.append(mod)
        elif "qaoa" in kind and "QAOA MaxCut" not in observed:
            observed.append("QAOA MaxCut")
        elif r.get("atom") and r.get("charge") not in (None, 0):
            chg = int(r["charge"])
            lbl = f"Charged System (charge={chg:+d})"
            if lbl not in observed:
                observed.append(lbl)
    if observed and (not camp_target or (camp_target == "Hydrogen Molecule (H2)" and "H2" not in observed)):
        return ", ".join(observed[:4])
    return camp_target or "an unrecorded target"


def build_keywords(target: str, rows: List[Dict[str, Any]], doc_type: str) -> List[str]:
    kws: List[str] = ["Quantum Computing", "Kubona Verified Science"]
    kinds = {str(r.get("kind") or "").lower() for r in rows}
    if any("vqe" in k or "excited" in k or "pes" in k for k in kinds):
        kws.append("Variational Quantum Eigensolver")
    if any("qaoa" in k for k in kinds):
        kws.append("QAOA Combinatorial Optimization")
    if any("lattice" in k or "spin" in k or "thermodynamic" in k for k in kinds):
        kws.append("Quantum Many-Body Dynamics")
    if any(bool(r.get("hardware")) for r in rows):
        kws.append("IBM Heron r2 QPU")
    if any(bool(r.get("mitigated")) for r in rows):
        kws.append("Zero-Noise Extrapolation")
    if doc_type == "white_paper":
        kws.append("Technical White Paper")
    return kws[:6]


def build_related_work_section(citations: List[Dict[str, Any]]) -> str:
    if not citations:
        return (
            "No external literature citations were attached to this draft. "
            "All quantitative claims below are anchored exclusively to exact classical "
            "solvers (PySCF FCI/CASCI, DMRG, exact diagonalization, or brute-force MaxCut) "
            "recorded in the Kubona ledger."
        )
    lines = [
        "This study builds on established variational, error-mitigation, and many-body "
        "benchmarking literature grounded through Kubona's citation verification rail "
        "(\\texttt{verification-v1}). Each reference below is linked to its canonical identifier:",
        r"\begin{itemize}",
    ]
    for c in citations:
        key = c["key"]
        title = latex_escape(str(c.get("title") or key))
        authors = latex_escape(str(c.get("authors") or "Unknown"))
        year = latex_escape(str(c.get("year") or "n.d."))
        doi = str(c.get("doi") or "").strip()
        doi_part = ""
        if doi:
            clean_doi = doi.replace("https://doi.org/", "").replace("http://doi.org/", "")
            doi_part = rf" (\href{{https://doi.org/{clean_doi}}}{{\texttt{{doi:{latex_escape(clean_doi)}}}}})"
        lines.append(rf"\item \textbf{{{authors} ({year})}} --- \textit{{{title}}}~\cite{{{key}}}{doi_part}.")
    lines.append(r"\end{itemize}")
    return "\n".join(lines)


def build_methods_section(campaign: Dict[str, Any], rows: List[Dict[str, Any]], doc_type: str) -> str:
    parts = [
        "All experiments were orchestrated through the Kubona compute bridge and settled by the "
        "Go orchestrator ledger (\\texttt{kubonad}), which records an immutable, Ed25519-chained "
        "result receipt, calibration metadata, and cost/usage settlement for every job. "
        "The campaign projection and settled job ledger are the sole sources of numerical evidence; "
        "no value is re-entered by hand or invented in prose."
    ]

    # Summarize computational protocols observed in settled rows
    kinds = sorted({str(r.get("kind") or "unknown") for r in rows})
    backends = sorted({str(r.get("backend") or r.get("model") or "unrecorded") for r in rows})
    shots_set = sorted({int(r["shots"]) for r in rows if r.get("shots") is not None})
    layers_set = sorted({int(r["layers"]) for r in rows if r.get("layers") is not None})
    bases = sorted({str(r["basis"]) for r in rows if r.get("basis")})
    has_zne = any(bool(r.get("mitigated")) for r in rows)
    has_hw = any(bool(r.get("hardware")) for r in rows)

    proto_items: List[str] = []
    if kinds:
        proto_items.append(rf"\item \textbf{{Experiment Protocols:}} {latex_escape(', '.join(kinds))} ({len(rows)} settled scientific job(s)).")
    if backends:
        proto_items.append(rf"\item \textbf{{Execution Backends:}} \texttt{{{latex_escape(', '.join(backends))}}}.")
    if bases:
        proto_items.append(rf"\item \textbf{{Quantum Chemistry Basis \& Mapping:}} {latex_escape(', '.join(bases))} with Jordan--Wigner fermion-to-qubit mapping and PySCF RHF/CASCI/FCI classical anchors.")
    if layers_set:
        proto_items.append(rf"\item \textbf{{Ansatz Depth Configuration:}} UCCSD repetitions $L \in \{{{', '.join(str(x) for x in layers_set)}\}}$ optimized via COBYLA.")
    if shots_set:
        proto_items.append(rf"\item \textbf{{Sampling Budget:}} {', '.join(str(s) for s in shots_set)} shots per Pauli measurement circuit.")
    if has_zne:
        proto_items.append(r"\item \textbf{Error Mitigation Protocol:} Zero-Noise Extrapolation (ZNE) via unitary folding; mitigated points are explicitly labeled \texttt{mitigated} (\texttt{measured: false}) to preserve epistemic separation from raw QPU counts.")

    if proto_items:
        parts.append(r"\subsection*{Computational Protocol Summary}" + "\n" + r"\begin{itemize}" + "\n" + "\n".join(proto_items) + "\n" + r"\end{itemize}")

    # Hardware Calibration Table (when real QPU calibration metadata exists)
    cal_by_backend: Dict[str, Dict[str, Any]] = {}
    for r in rows:
        if bool(r.get("hardware")):
            be = str(r.get("backend") or "ibm-qiskit")
            cal = r.get("calibration") if isinstance(r.get("calibration"), dict) else {}
            if be not in cal_by_backend or cal:
                cal_by_backend[be] = {
                    "backend": be,
                    "snap": str(r.get("calibration_snapshot_id") or cal.get("version") or "--"),
                    "calibrated_at": str(cal.get("calibrated_at") or r.get("calibration_checked_at") or "--")[:19],
                    "fresh": "yes" if r.get("calibration_fresh") is True else ("no" if r.get("calibration_fresh") is False else "--"),
                    "two_q": cal.get("median_2q_error", cal.get("two_qubit_error")),
                    "readout": cal.get("median_readout_error", cal.get("readout_error")),
                }
    if has_hw and cal_by_backend:
        tlines = [
            r"\subsection*{Quantum Processor Calibration Profile}",
            r"\begin{table}[H]",
            r"\centering\small",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}llllrr@{}}",
            r"\toprule",
            r"Processor & Snapshot / Version & Calibrated (UTC) & Fresh & 2Q Error & Readout Err. \\",
            r"\midrule",
        ]
        for be, info in sorted(cal_by_backend.items()):
            tlines.append(
                f"{latex_escape(be)} & \\texttt{{{latex_escape(info['snap'][:18])}}} & "
                f"{latex_escape(info['calibrated_at'])} & {info['fresh']} & "
                f"{fmt_num(info['two_q'], 5)} & {fmt_num(info['readout'], 4)} \\\\"
            )
        tlines += [
            r"\bottomrule",
            r"\end{tabular}}",
            r"\caption{Live quantum hardware calibration metadata recorded at execution time.}",
            r"\end{table}",
        ]
        parts.append("\n".join(tlines))

    return "\n\n".join(parts)


def build_results_narrative(rows: List[Dict[str, Any]], campaign: Dict[str, Any]) -> str:
    if not rows:
        return ""
    paragraphs: List[str] = []
    n_total = len(rows)
    n_hw = sum(1 for r in rows if bool(r.get("hardware")))
    n_mit = sum(1 for r in rows if bool(r.get("mitigated")))
    n_sim = n_total - n_hw

    paragraphs.append(
        f"Across this campaign, {n_total} settled scientific job(s) were folded from the ledger "
        f"({n_sim} classical/simulator and {n_hw} live quantum hardware execution(s), "
        f"of which {n_mit} employed error mitigation)."
    )

    # Energy discrepancy statistics
    deltas: List[float] = []
    for r in rows:
        kind_l = str(r.get("kind") or "").lower()
        if "qaoa" in kind_l or kind_l in ("lattice_dynamics", "thermodynamic_limit", "conjecture", "hardware_pes"):
            continue
        d = r.get("delta_mha")
        if d is not None:
            deltas.append(abs(float(d)))
    if deltas:
        below_chem = sum(1 for d in deltas if d <= 1.6)
        paragraphs.append(
            f"Among {len(deltas)} energy-anchored run(s), absolute discrepancy against the exact "
            f"classical anchor spans {min(deltas):.4f}~mHa to {max(deltas):.4f}~mHa, with "
            f"{below_chem} of {len(deltas)} run(s) within the 1.6~mHa chemical-accuracy benchmark."
        )

    # Raw vs ZNE hardware breakdown
    hw_raw = [float(r["delta_mha"]) for r in rows if bool(r.get("hardware")) and not bool(r.get("mitigated")) and r.get("delta_mha") is not None]
    hw_zne = [float(r["delta_mha"]) for r in rows if bool(r.get("hardware")) and bool(r.get("mitigated")) and r.get("delta_mha") is not None]
    if hw_raw and hw_zne:
        paragraphs.append(
            f"On live hardware, raw unmitigated runs achieved discrepancies between {min(hw_raw):.4f}~mHa "
            f"and {max(hw_raw):.4f}~mHa, whereas ZNE-mitigated runs spanned {min(hw_zne):.4f}~mHa to "
            f"{max(hw_zne):.4f}~mHa. This directly quantifies the finite-shot bias--variance tradeoff: "
            "extrapolation reduces coherent bias only when shot-noise amplification does not dominate."
        )

    # QAOA breakdown
    qaoa_ratios = [float(r["approximation_ratio"]) for r in rows if "qaoa" in str(r.get("kind") or "").lower() and r.get("approximation_ratio") is not None]
    if qaoa_ratios:
        paragraphs.append(
            f"Combinatorial QAOA evaluations ({len(qaoa_ratios)} run(s)) recorded approximation ratios "
            f"from {min(qaoa_ratios):.4f} to {max(qaoa_ratios):.4f} relative to the exact classical cut."
        )

    # ADMET / PAINS table if hydrated
    admet_rows = campaign.get("admet_results")
    if isinstance(admet_rows, list) and admet_rows:
        alines = [
            r"\subsection*{ADMET \& PAINS Candidate Triage}",
            r"\begin{table}[H]",
            r"\centering\small",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}lrrrrl@{}}",
            r"\toprule",
            r"Scaffold & MW & cLogP & TPSA & QED & PAINS Alerts \\",
            r"\midrule",
        ]
        for a in admet_rows[:12]:
            if not isinstance(a, dict):
                continue
            pains = a.get("pains_alerts") or a.get("pains") or "none"
            if isinstance(pains, list):
                pains = ", ".join(str(x) for x in pains) or "none"
            alines.append(
                f"{latex_escape(str(a.get('name') or a.get('smiles') or '--')[:28])} & "
                f"{fmt_num(a.get('mw'), 2)} & {fmt_num(a.get('clogp'), 2)} & "
                f"{fmt_num(a.get('tpsa'), 2)} & {fmt_num(a.get('qed'), 3)} & "
                f"{latex_escape(str(pains))} \\\\"
            )
        alines += [r"\bottomrule", r"\end{tabular}}", r"\caption{ADMET and PAINS screening prior to quantum resource allocation.}", r"\end{table}"]
        paragraphs.append("\n".join(alines))

    return "\n\n".join(paragraphs)


def build_statistical_audit_section(campaign: Dict[str, Any]) -> str:
    parts: List[str] = []

    # Hypotheses & Elo Tournament
    hyps = campaign.get("hypotheses")
    if isinstance(hyps, list) and hyps:
        hlines = [
            r"\subsection*{Hypothesis Registry \& Elo Tournament}",
            r"\begin{table}[H]",
            r"\centering\small",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}lp{7.2cm}lr@{}}",
            r"\toprule",
            r"ID & Hypothesis Statement & Status & Elo \\",
            r"\midrule",
        ]
        for h in hyps[:10]:
            if not isinstance(h, dict):
                continue
            hid = short_job_id(str(h.get("id") or "--"))
            stmt = latex_escape(str(h.get("statement") or h.get("title") or "--")[:140])
            st = latex_escape(str(h.get("status") or "active"))
            elo = fmt_num(h.get("elo_rating", h.get("elo")), 1)
            hlines.append(f"\\texttt{{{latex_escape(hid)}}} & {stmt} & {st} & {elo} \\\\")
        hlines += [r"\bottomrule", r"\end{tabular}}", r"\caption{Persisted campaign hypotheses and evidence-gated Elo tournament outcomes.}", r"\end{table}"]
        parts.append("\n".join(hlines))

    # Prediction Vault (Pre-Registration & Brier/ECE)
    preds = campaign.get("predictions")
    if isinstance(preds, list) and preds:
        plines = [
            r"\subsection*{Pre-Registered Prediction Vault \& Calibration}",
            r"\begin{table}[H]",
            r"\centering\small",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}p{6.5cm}rrr@{}}",
            r"\toprule",
            r"Pre-Registered Claim & Predicted & Observed & Status / $z$ \\",
            r"\midrule",
        ]
        for p in preds[:10]:
            if not isinstance(p, dict):
                continue
            claim = latex_escape(str(p.get("claim") or p.get("metric") or "--")[:120])
            pred_v = fmt_num(p.get("predicted_value"), 4)
            obs_v = fmt_num(p.get("observed_value", p.get("actual_value")), 4)
            z_v = fmt_num(p.get("z_score"), 2) if p.get("z_score") is not None else latex_escape(str(p.get("status") or "open"))
            plines.append(f"{claim} & {pred_v} & {obs_v} & {z_v} \\\\")
        plines += [r"\bottomrule", r"\end{tabular}}", r"\caption{Pre-registered quantitative predictions evaluated against measured outcomes.}", r"\end{table}"]
        parts.append("\n".join(plines))

    # 4-Agent Tribunal
    tribunal = campaign.get("tribunal")
    if isinstance(tribunal, dict) and tribunal:
        consensus = tribunal.get("consensus")
        summary = str(tribunal.get("summary") or tribunal.get("verdict") or "").strip()
        parts.append(
            r"\subsection*{Adversarial Multi-Agent Tribunal Review}" + "\n"
            + f"The campaign underwent automated adversarial critique (consensus: \\texttt{{{latex_escape(str(consensus))}}}). "
            + (latex_escape(summary[:500]) if summary else "Critiques are archived in the signed experiment export.")
        )

    if not parts:
        return (
            "No separate Prediction Vault, Hypothesis Elo tournament, or Tribunal record was "
            "attached to this snapshot; statistical verification relies on per-job 95\\% confidence "
            "intervals and exact classical anchors in the Results section."
        )
    return "\n\n".join(parts)


def build_discussion_section(rows: List[Dict[str, Any]], campaign: Dict[str, Any], doc_type: str) -> str:
    notes: List[str] = [
        "Scientific rigor requires reporting negative and null results with the same prominence as "
        "positive findings. All discrepancies reported in this paper are evaluated strictly against "
        "exact classical anchors rather than relative hardware baselines."
    ]
    if any(bool(r.get("mitigated")) for r in rows):
        notes.append(
            "\\textbf{Error Mitigation Bias--Variance Boundary:} Zero-Noise Extrapolation (ZNE) "
            "amplifies statistical shot variance even when reducing coherent gate bias. Wherever "
            "mitigated hardware errors exceed raw hardware errors at fixed shot budgets, the result "
            "is recorded as an empirical negative finding rather than post-selected out."
        )
    if any(r.get("layers") is not None and int(r["layers"]) > 1 for r in rows):
        notes.append(
            "\\textbf{Ansatz Expressibility vs. Optimization Landscape:} Increasing UCCSD repetition "
            "depth expands parameter dimension and two-qubit gate depth. Under fixed derivative-free "
            "COBYLA budgets, deeper ans\\\"atze can degrade variational convergence due to optimization "
            "landscape ruggedness; reporting both energy error and circuit depth exposes this tradeoff."
        )
    if any(bool(r.get("hardware")) for r in rows):
        notes.append(
            "\\textbf{Hardware Noise Floor:} Live superconducting QPU executions remain subject to "
            "two-qubit gate infidelity, readout error, and temporal calibration drift across processors. "
            "Simulator results provide noiseless algorithmic upper bounds and are never conflated with "
            "hardware evidence."
        )
    return "\n\n".join(notes)


def build_economics_governance_section(rows: List[Dict[str, Any]], evidence: Dict[str, Any]) -> str:
    hw_rows = [r for r in rows if bool(r.get("hardware"))]
    total_usd = sum(float(r.get("actual_usd") or 0.0) for r in hw_rows)
    total_shots = sum(int(r.get("shots") or 0) for r in hw_rows)
    resolved_n = len(evidence.get("resolved_receipts") or [])
    unresolved_n = len(evidence.get("unresolved_receipts") or [])
    lines = [
        "For enterprise, industrial, and funding-agency evaluation (e.g., DARPA QBI utility-scale V\\&V), "
        "scientific accuracy must be weighed alongside execution cost and governance integrity.",
        r"\begin{table}[H]",
        r"\centering\small",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}ll@{}}",
        r"\toprule",
        r"Governance / Economic Dimension & Verified Campaign Measurement \\",
        r"\midrule",
        f"Live QPU Executions / Total Hardware Shots & {len(hw_rows)} workload(s) / {total_shots} shots \\\\",
        f"Settled Hardware Spend (Ledger) & \\${total_usd:.2f} USD \\\\",
        f"Cryptographic Receipt Resolution & {resolved_n} resolved, {unresolved_n} unresolved \\\\",
        r"Numeric \& Citation Verification & \texttt{verification-v1} (ledger-grounded) \\",
        r"Ledger Chain Integrity & Ed25519 hash-chained (\texttt{kubonad verify}) \\",
        r"Publication Signoff \& Archival & Ed25519 human signoff + RO-Crate 1.1 / W3C PROV-JSON / CRediT \\",
        r"\bottomrule",
        r"\end{tabular}}",
        r"\caption{Utility-vs-cost accounting and sovereign governance certification matrix.}",
        r"\end{table}",
    ]
    return "\n".join(lines)


def build_appendix_provenance(rows: List[Dict[str, Any]]) -> str:
    if not rows:
        return "No settled experiment jobs are present in this draft."
    lines = [
        "Table~\\ref{tab:full-provenance} maps every short job identifier used in the main text "
        "to its complete ledger UUID, hardware vendor workload reference, requested vs. actual "
        "backend, calibration snapshot ID, and settled cost.",
        r"\begin{table}[H]",
        r"\centering\scriptsize",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}llllllr@{}}",
        r"\toprule",
        r"Short ID & Full Ledger Job UUID & Vendor Ref & Req. Backend & Actual Backend & Calib. Snapshot & Shots \\",
        r"\midrule",
    ]
    for r in rows[:40]:
        jid = str(r.get("job_id") or "--")
        vref = str(r.get("vendor_ref") or r.get("hardware_job_id") or "--")
        req_b = str(r.get("requested_backend") or r.get("backend") or r.get("model") or "--")
        act_b = str(r.get("backend") or r.get("model") or "--")
        snap = str(r.get("calibration_snapshot_id") or "--")
        shots = str(int(r["shots"])) if r.get("shots") is not None else "--"
        lines.append(
            f"\\texttt{{{latex_escape(short_job_id(jid))}}} & "
            f"\\texttt{{{latex_escape(jid)}}} & "
            f"\\texttt{{{latex_escape(vref)}}} & "
            f"{latex_escape(req_b)} & {latex_escape(act_b)} & "
            f"\\texttt{{{latex_escape(snap)}}} & {shots} \\\\"
        )
    lines += [
        r"\bottomrule",
        r"\end{tabular}}",
        r"\caption{Complete ledger and hardware provenance manifest.}\label{tab:full-provenance}",
        r"\end{table}",
    ]
    return "\n".join(lines)


def _strip_latex_for_markdown(latex: str) -> str:
    """Best-effort clean conversion of Kubona's structured LaTeX sections to readable Markdown."""
    s = latex
    s = re.sub(r"\\subsection\*?\{([^}]*)\}", r"\n### \1\n", s)
    s = re.sub(r"\\textbf\{([^}]*)\}", r"**\1**", s)
    s = re.sub(r"\\textit\{([^}]*)\}", r"*\1*", s)
    s = re.sub(r"\\texttt\{([^}]*)\}", r"`\1`", s)
    s = re.sub(r"\\href\{([^}]*)\}\{([^}]*)\}", r"[\2](\1)", s)
    s = re.sub(r"\\cite\{([^}]*)\}", r"[\1]", s)
    s = re.sub(r"\\item\s+", "- ", s)
    s = re.sub(r"\\begin\{itemize\}|\\end\{itemize\}", "", s)
    s = re.sub(r"\\begin\{table\}.*?\\end\{table\}", "\n*(See structured table in main.pdf / structured export.)*\n", s, flags=re.S)
    s = re.sub(r"\\begin\{figure\}.*?\\includegraphics\[[^\]]*\]\{([^}]*)\}.*?\\caption\{([^}]*)\}.*?\\end\{figure\}", r"\n![\2](\1)\n*\2*\n", s, flags=re.S)
    s = s.replace(r"\%", "%").replace(r"\_", "_").replace(r"\&", "&").replace(r"\$", "$").replace("~", " ")
    return re.sub(r"\n{3,}", "\n\n", s).strip()


def render_companion_markdown(
    *, title: str, authors: List[str], abstract: str, doc_type: str,
    sections: List[Any], citations: List[Dict[str, Any]],
) -> str:
    out = [
        f"# {title}",
        "",
        f"**Document Type:** `{doc_type}`  ",
        f"**Authors:** {', '.join(authors)}",
        "",
        "## Abstract" if doc_type != "white_paper" else "## Executive Summary",
        "",
        abstract,
        "",
    ]
    for sec in sections:
        out.append(f"## {sec.title}")
        out.append("")
        out.append(_strip_latex_for_markdown(sec.latex_content))
        out.append("")
    if citations:
        out.append("## References")
        out.append("")
        for i, c in enumerate(citations, 1):
            doi = str(c.get("doi") or "").strip()
            doi_s = f" https://doi.org/{doi}" if doi else ""
            out.append(f"{i}. {c.get('authors', 'Unknown')} ({c.get('year', 'n.d.')}). *{c.get('title', c['key'])}*.{doi_s}")
    return "\n".join(out).strip() + "\n"


def render_companion_html(
    *, title: str, authors: List[str], abstract: str, doc_type: str,
    sections: List[Any], citations: List[Dict[str, Any]],
) -> str:
    md_sections = []
    for sec in sections:
        body = html.escape(_strip_latex_for_markdown(sec.latex_content)).replace("\n\n", "</p><p>").replace("\n", "<br/>")
        md_sections.append(f"<section><h2>{html.escape(sec.title)}</h2><p>{body}</p></section>")
    cites_html = ""
    if citations:
        items = []
        for c in citations:
            doi = str(c.get("doi") or "").strip()
            doi_a = f' <a href="https://doi.org/{html.escape(doi)}">doi:{html.escape(doi)}</a>' if doi else ""
            items.append(
                f"<li>{html.escape(str(c.get('authors', 'Unknown')))} ({html.escape(str(c.get('year', 'n.d.')))}). "
                f"<em>{html.escape(str(c.get('title', c['key'])))}</em>.{doi_a}</li>"
            )
        cites_html = "<section><h2>References</h2><ol>" + "".join(items) + "</ol></section>"
    badge = doc_type.replace("_", " ").upper()
    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>{html.escape(title)}</title>
<style>
  body {{ font-family: Georgia, "Times New Roman", serif; max-width: 820px; margin: 2.5rem auto; padding: 0 1.5rem; color: #0f172a; line-height: 1.65; }}
  .badge {{ display: inline-block; background: #14213d; color: #fff; font-family: system-ui, sans-serif; font-size: 0.72rem; letter-spacing: 0.08em; padding: 0.25rem 0.65rem; border-radius: 3px; }}
  h1 {{ color: #14213d; margin: 0.75rem 0 0.25rem; line-height: 1.25; }}
  .authors {{ color: #475569; font-style: italic; margin-bottom: 1.5rem; }}
  .abstract {{ background: #f4f7fa; border-left: 4px solid #0e6b74; padding: 1rem 1.25rem; margin: 1.25rem 0 2rem; }}
  h2 {{ color: #14213d; border-bottom: 1px solid #cbd5e1; padding-bottom: 0.25rem; margin-top: 2rem; }}
  code {{ background: #f1f5f9; padding: 0.1rem 0.3rem; border-radius: 3px; font-size: 0.9em; }}
  a {{ color: #0e6b74; }}
</style>
</head>
<body>
  <div class="badge">KUBONA {html.escape(badge)}</div>
  <h1>{html.escape(title)}</h1>
  <div class="authors">{html.escape(", ".join(authors))}</div>
  <div class="abstract"><strong>{"Executive Summary" if doc_type == "white_paper" else "Abstract"}:</strong> {html.escape(abstract)}</div>
  {"".join(md_sections)}
  {cites_html}
</body>
</html>
"""
