"""Phase 1, 3, 4 & 5 Publication Completeness Synthesizers.

Generates:
  - Appendix B: Experiment Datasheet (compact per-job provenance table covering
    seed, shots, gate lines/2Q count, usage, cost, circuit_sha256, counts_sha256)
  - Raw vs. Mitigated (ZNE) & Uncertainty Summary table (Phase 4)
  - Data and Code Availability section with supplement/ bundle map & replay info
  - Scholarly Declarations (Affiliations, Corresponding Author, License, COI,
    Funding, Acknowledgements, and in-paper CRediT Author Contributions)
  - Canonical method & benchmark BibTeX reference catalog (>= 12 entries)

Verification-v1 invariant: every printed numerical value originates directly from
the settled campaign rows passed in; declarations contain no fabricated numbers.
"""

from __future__ import annotations

from typing import Any, Dict, List

from manuscript_sections import extract_anchor_energy, fmt_num, latex_escape, short_job_id

# Canonical method & benchmark bibliography (Phase 5: expands from 3 to 12 entries)
CANONICAL_METHOD_REFERENCES: Dict[str, str] = {
    "Peruzzo2014": (
        "@article{Peruzzo2014,\n"
        "  author = {Peruzzo, Alberto and McClean, Jarrod and Shadbolt, Peter and Yung, Man-Hong and Zhou, Xiao-Qi and Love, Peter J. and Aspuru-Guzik, Al\\'an and O'Brien, Jeremy L.},\n"
        "  title = {A variational eigenvalue solver on a photonic quantum processor},\n"
        "  journal = {Nature Communications},\n"
        "  year = {2014},\n"
        "  doi = {10.1038/ncomms5213}\n"
        "}"
    ),
    "Kandala2017": (
        "@article{Kandala2017,\n"
        "  author = {Kandala, Abhinav and Mezzacapo, Antonio and Temme, Kristan and Takita, Maika and Brink, Markus and Chow, Jerry M. and Gambetta, Jay M.},\n"
        "  title = {Hardware-efficient variational quantum eigensolver for small molecules and quantum magnets},\n"
        "  journal = {Nature},\n"
        "  year = {2017},\n"
        "  doi = {10.1038/nature23879}\n"
        "}"
    ),
    "Kim2023": (
        "@article{Kim2023,\n"
        "  author = {Kim, Youngseok and Eddins, Andrew and Anand, Sajant and Wei, Ken Xuan and van den Berg, Ewout and Rosenblatt, Sami and Nayfeh, Hasan and Wu, Yantao and Zaletel, Michael and Temme, Kristan and Kandala, Abhinav},\n"
        "  title = {Evidence for the utility of quantum computing before fault tolerance},\n"
        "  journal = {Nature},\n"
        "  year = {2023},\n"
        "  doi = {10.1038/s41586-023-06096-3}\n"
        "}"
    ),
    "Sun2018PySCF": (
        "@article{Sun2018PySCF,\n"
        "  author = {Sun, Qiming and Berkelbach, Timothy C. and Blunt, Nick S. and Booth, George H. and Guo, Sheng and Li, Zhendong and Liu, Junzi and McClain, James D. and Sayfutyarova, Elvira R. and Sharma, Sandeep and Wouters, Sebastian and Chan, Garnet Kin-Lic},\n"
        "  title = {PySCF: the Python-based simulations of chemistry framework},\n"
        "  journal = {WIREs Computational Molecular Science},\n"
        "  year = {2018},\n"
        "  doi = {10.1002/wcms.1340}\n"
        "}"
    ),
    "White1992DMRG": (
        "@article{White1992DMRG,\n"
        "  author = {White, Steven R.},\n"
        "  title = {Density matrix formulation for quantum renormalization groups},\n"
        "  journal = {Physical Review Letters},\n"
        "  year = {1992},\n"
        "  doi = {10.1103/PhysRevLett.69.2863}\n"
        "}"
    ),
    "OMalley2016": (
        "@article{OMalley2016,\n"
        "  author = {O'Malley, P. J. J. and Babbush, R. and Kivlichan, I. D. and Romero, J. and McClean, J. R. and Barends, R. and Kelly, J. and Roushan, P. and Tranter, A. and Ding, N. and others},\n"
        "  title = {Scalable Quantum Simulation of Molecular Energies},\n"
        "  journal = {Physical Review X},\n"
        "  year = {2016},\n"
        "  doi = {10.1103/PhysRevX.6.031007}\n"
        "}"
    ),
    "Bravyi2024": (
        "@article{Bravyi2024,\n"
        "  author = {Bravyi, Sergey and Cross, Andrew W. and Gambetta, Jay M. and Maslov, Dmitri and Rall, Patrick and Yoder, Theodore J.},\n"
        "  title = {High-threshold and low-overhead fault-tolerant quantum memory},\n"
        "  journal = {Nature},\n"
        "  year = {2024},\n"
        "  doi = {10.1038/s41586-024-07107-7}\n"
        "}"
    ),
    "Baell2010PAINS": (
        "@article{Baell2010PAINS,\n"
        "  author = {Baell, Jonathan B. and Holloway, Georgina A.},\n"
        "  title = {New Substructure Filters for Removal of Pan Assay Interference Compounds (PAINS) from Screening Libraries},\n"
        "  journal = {Journal of Medicinal Chemistry},\n"
        "  year = {2010},\n"
        "  doi = {10.1021/jm901137j}\n"
        "}"
    ),
    "Temme2017ZNE": (
        "@article{Temme2017ZNE,\n"
        "  author = {Temme, Kristan and Bravyi, Sergey and Gambetta, Jay M.},\n"
        "  title = {Error Mitigation for Short-Depth Quantum Circuits},\n"
        "  journal = {Physical Review Letters},\n"
        "  year = {2017},\n"
        "  doi = {10.1103/PhysRevLett.119.180509}\n"
        "}"
    ),
    "Li2017ZNE": (
        "@article{Li2017ZNE,\n"
        "  author = {Li, Ying and Benjamin, Simon C.},\n"
        "  title = {Efficient Variational Quantum Simulator Incorporating Active Error Minimization},\n"
        "  journal = {Physical Review X},\n"
        "  year = {2017},\n"
        "  doi = {10.1103/PhysRevX.7.021050}\n"
        "}"
    ),
    "Farhi2014QAOA": (
        "@misc{Farhi2014QAOA,\n"
        "  author = {Farhi, Edward and Goldstone, Jeffrey and Gutmann, Sam},\n"
        "  title = {A Quantum Approximate Optimization Algorithm},\n"
        "  year = {2014},\n"
        "  doi = {10.48550/arXiv.1411.4028}\n"
        "}"
    ),
    "Zurek1985KibbleZurek": (
        "@article{Zurek1985KibbleZurek,\n"
        "  author = {Zurek, Wojciech H.},\n"
        "  title = {Cosmological experiments in superfluid helium?},\n"
        "  journal = {Nature},\n"
        "  year = {1985},\n"
        "  doi = {10.1038/317505a0}\n"
        "}"
    ),
}


def _breakable_path(path: str) -> str:
    """Render a literal path in ``\\texttt`` with break points after each slash.

    A long ``\\texttt{supplement/counts/<job_id>.json}`` has no natural break
    point, so TeX overruns the page margin and the publication margin gate
    fails. Allowing a break after every ``/`` keeps the path inside the text
    block without changing the characters a reader sees.
    """
    escaped = path.replace("_", r"\_").replace("/", r"/\allowbreak ")
    return rf"\texttt{{{escaped}}}"


def build_experiment_datasheet_section(rows: List[Dict[str, Any]]) -> str:
    """Render Appendix B: Experiment Datasheet from settled campaign rows."""
    if not rows:
        return (
            "No settled experiment rows are present in this campaign projection. "
            f"The machine-readable datasheet ({_breakable_path('supplement/datasheet.json')}) is empty."
        )
    lines = [
        "Every settled experiment row below is bound to its immutable ledger record and "
        f"archived in {_breakable_path('supplement/datasheet.json')} and {_breakable_path('supplement/jobs.csv')}. "
        f"Hardware jobs carry OpenQASM sources in {_breakable_path('supplement/qasm/<job_id>.qasm')} "
        f"and measurement histograms in {_breakable_path('supplement/counts/<job_id>.json')} when "
        "shot-level counts are emitted.",
        "",
        r"\begin{table}[H]",
        r"\centering\scriptsize",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}llrrrrll@{}}",
        r"\toprule",
        r"Job & Backend & Seed & Shots & 2Q Gates & Usage (s) & Circuit SHA-256 & Counts SHA-256 \\",
        r"\midrule",
    ]
    for r in rows[:48]:
        jid = latex_escape(short_job_id(str(r.get("job_id") or "--")))
        be = latex_escape(str(r.get("backend") or r.get("model") or "--")[:16])
        seed_s = str(int(r["seed"])) if r.get("seed") is not None else "--"
        shots_s = str(int(r["shots"])) if r.get("shots") is not None else "--"
        twoq_s = str(int(r["two_qubit_gate_count"])) if r.get("two_qubit_gate_count") is not None else "--"
        usage_s = fmt_num(r.get("usage_seconds"), 2) if r.get("usage_seconds") is not None else "--"
        ch = str(r.get("circuit_sha256") or "")
        ch_s = rf"\texttt{{{latex_escape(ch[:12])}}}" if ch else "--"
        cth = str(r.get("counts_sha256") or "")
        cth_s = rf"\texttt{{{latex_escape(cth[:12])}}}" if cth else "--"
        lines.append(f"{jid} & {be} & {seed_s} & {shots_s} & {twoq_s} & {usage_s} & {ch_s} & {cth_s} \\\\")
    if len(rows) > 48:
        lines.append(
            rf"\multicolumn{{8}}{{l}}{{\textit{{Showing first 48 of {len(rows)} rows; "
            rf"complete record in {_breakable_path('supplement/datasheet.json')}.}}}} \\"
        )
    lines.extend([
        r"\bottomrule",
        r"\end{tabular}%",
        r"}",
        r"\caption{Experiment Datasheet summary (seeds, shots, two-qubit gate counts, QPU usage, and content-addressed circuit/counts checksums).}",
        r"\end{table}",
    ])
    return "\n".join(lines)


def build_zne_and_uncertainty_section(rows: List[Dict[str, Any]]) -> str:
    """Render Phase 4 uncertainty & raw-vs-mitigated ZNE summary table."""
    relevant = [
        r for r in rows
        if bool(r.get("mitigated"))
        or r.get("ci95_mha") is not None
        or r.get("raw_energy") is not None
        or r.get("delta_mha") is not None
    ]
    if not relevant:
        return ""
    lines = [
        "All discrepancies $\\Delta$ below are reported alongside their recorded 95\\% "
        "confidence interval (\\texttt{CI95}) when available; entries without a sampled "
        "interval are explicitly tagged \\texttt{point estimate} and must not be read as "
        "statistically separated claims.",
        "",
        r"\begin{table}[H]",
        r"\centering\scriptsize",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}lllrrrr@{}}",
        r"\toprule",
        r"Job & Kind & Mode & Energy (Ha) & Anchor (Ha) & $\Delta$ (mHa) & CI95 / Status \\",
        r"\midrule",
    ]
    for r in relevant[:24]:
        jid = latex_escape(short_job_id(str(r.get("job_id") or "--")))
        kind = latex_escape(str(r.get("kind") or "--")[:16])
        mode = "ZNE mitigated" if bool(r.get("mitigated")) else ("hardware" if bool(r.get("hardware")) else "simulator")
        e = r.get("energy", r.get("mitigated_energy", r.get("ground_energy")))
        anc = extract_anchor_energy(r)
        delta = r.get("delta_mha", r.get("delta"))
        if delta is None and e is not None and anc is not None:
            delta = abs(float(e) - float(anc)) * 1000.0
        ci = r.get("ci95_mha")
        ci_s = f"$\\pm${fmt_num(ci, 3)} mHa" if ci is not None else r"\texttt{point estimate}"
        lines.append(
            f"{jid} & {kind} & {mode} & {fmt_num(e, 6)} & {fmt_num(anc, 6)} & {fmt_num(delta, 3)} & {ci_s} \\\\"
        )
    lines.extend([
        r"\bottomrule",
        r"\end{tabular}%",
        r"}",
        r"\caption{Uncertainty and Zero-Noise Extrapolation (ZNE) audit table. Mitigated points remain explicitly separated from raw hardware measurements.}",
        r"\end{table}",
    ])
    return "\n".join(lines)


def build_data_availability_section(rows: List[Dict[str, Any]], manuscript_id: str) -> str:
    """Render Phase 3 Data and Code Availability statement."""
    hw_with_qasm = sum(1 for r in rows if r.get("circuit_sha256"))
    with_counts = sum(1 for r in rows if r.get("counts_sha256"))
    return (
        "All experiment inputs, OpenQASM circuits, measurement histograms, and classical "
        f"anchor specifications are archived alongside this manuscript in the {_breakable_path('supplement/')} "
        f"bundle of both the Zenodo deposit manifest ({_breakable_path('zenodo_deposit.json')}) and the "
        f"RO-Crate/PROV-JSON export ({_breakable_path('ro_crate.json')}). Specifically, this bundle "
        f"indexes {len(rows)} settled job record(s) in {_breakable_path('supplement/datasheet.json')} and "
        f"{_breakable_path('supplement/jobs.csv')}, {hw_with_qasm} circuit checksum(s) in "
        f"{_breakable_path('supplement/qasm/')}, and {with_counts} histogram checksum(s) in "
        f"{_breakable_path('supplement/counts/')}, together with software environment metadata in "
        f"{_breakable_path('supplement/environment.json')} and the automated completeness audit in "
        f"{_breakable_path('supplement/completeness_report.json')}.\n\n"
        "Source code is publicly available under the Apache-2.0 license at "
        "\\href{https://github.com/mainza-ai/kubona}{\\texttt{https://github.com/mainza-ai/kubona}}; "
        "manuscript prose and figures are released under CC-BY-4.0. "
        f"To inspect or replay manuscript \\texttt{{{latex_escape(manuscript_id)}}}, fetch the "
        "archived \\texttt{supplement/} artifacts via the Kubona API or RO-Crate export and "
        "verify every file against its recorded SHA-256 digest."
    )


def build_scholarly_declarations_section(evidence: Dict[str, Any], authors: List[str]) -> str:
    """Render Phase 5 Affiliations, COI, Funding, License, and CRediT statements."""
    meta = evidence.get("_scholarly") if isinstance(evidence.get("_scholarly"), dict) else {}
    affiliation = latex_escape(str(meta.get("affiliation") or "Kubona Research Lab (Open-Source Scientific Discovery Platform)"))
    corr_author = latex_escape(str(meta.get("corresponding_author") or (authors[0] if authors else "Kubona Maintainers")))
    coi = latex_escape(str(meta.get("competing_interests") or "The authors declare no competing financial or non-financial interests."))
    funding = latex_escape(str(meta.get("funding") or "This work was conducted using open-source research infrastructure with no external commercial sponsorship."))
    license_str = latex_escape(str(meta.get("license") or "Text and figures: CC-BY-4.0; software and circuits: Apache-2.0."))
    credit = latex_escape(
        str(
            meta.get("credit_statement")
            or "Conceptualization, Methodology, Software, Validation, Formal Analysis, Data Curation, and Writing: recorded in the accompanying RO-Crate CRediT manifest."
        )
    )
    return (
        r"\begin{itemize}" + "\n"
        + rf"\item \textbf{{Affiliations \& Corresponding Author:}} {affiliation}. Correspondence: {corr_author}." + "\n"
        + rf"\item \textbf{{License:}} {license_str}" + "\n"
        + rf"\item \textbf{{Competing Interests:}} {coi}" + "\n"
        + rf"\item \textbf{{Funding:}} {funding}" + "\n"
        + rf"\item \textbf{{Author Contributions (CRediT):}} {credit}" + "\n"
        + r"\end{itemize}"
    )
