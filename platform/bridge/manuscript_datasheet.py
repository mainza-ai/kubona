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
    "Kandala2019": (
        "@article{Kandala2019,\n"
        "  author = {Kandala, Abhinav and Temme, Kristan and C\\'orcoles, Antonio D. and Mezzacapo, Antonio and Chow, Jerry M. and Gambetta, Jay M.},\n"
        "  title = {Error mitigation extends the computational reach of a noisy quantum processor},\n"
        "  journal = {Nature},\n"
        "  year = {2019},\n"
        "  doi = {10.1038/s41586-019-1040-7}\n"
        "}"
    ),
    "Harrigan2021": (
        "@article{Harrigan2021,\n"
        "  author = {Harrigan, Matthew P. and Sung, Kevin J. and Neeley, Matthew and Satzinger, Kevin J. and Arute, Frank and Arya, Kunal and Atalaya, Juan and Bardin, Joseph C. and Barends, Rami and Boixo, Sergio and others},\n"
        "  title = {Quantum approximate optimization of non-planar graph problems on a planar superconducting processor},\n"
        "  journal = {Nature Physics},\n"
        "  year = {2021},\n"
        "  doi = {10.1038/s41567-020-01105-y}\n"
        "}"
    ),
    "Pfeuty1970": (
        "@article{Pfeuty1970,\n"
        "  author = {Pfeuty, Pierre},\n"
        "  title = {The one-dimensional Ising model with a transverse field},\n"
        "  journal = {Annals of Physics},\n"
        "  year = {1970},\n"
        "  doi = {10.1016/0003-4916(70)90270-8}\n"
        "}"
    ),
    "Steane1996": (
        "@article{Steane1996,\n"
        "  author = {Steane, Andrew M.},\n"
        "  title = {Error Correcting Codes in Quantum Theory},\n"
        "  journal = {Physical Review Letters},\n"
        "  year = {1996},\n"
        "  doi = {10.1103/PhysRevLett.77.793}\n"
        "}"
    ),
    "BravyiKitaev2005": (
        "@article{BravyiKitaev2005,\n"
        "  author = {Bravyi, Sergey and Kitaev, Alexei},\n"
        "  title = {Universal quantum computation with ideal Clifford gates and noisy ancillas},\n"
        "  journal = {Physical Review A},\n"
        "  year = {2005},\n"
        "  doi = {10.1103/PhysRevA.71.022316}\n"
        "}"
    ),
}

# Structured metadata for canonical campaign-matched citation auto-seeding (G5)
CANONICAL_CITATION_METADATA: Dict[str, Dict[str, str]] = {
    "Peruzzo2014": {"key": "Peruzzo2014", "title": "A variational eigenvalue solver on a photonic quantum processor", "authors": "Peruzzo et al.", "year": "2014", "doi": "10.1038/ncomms5213"},
    "OMalley2016": {"key": "OMalley2016", "title": "Scalable Quantum Simulation of Molecular Energies", "authors": "O'Malley et al.", "year": "2016", "doi": "10.1103/PhysRevX.6.031007"},
    "Kandala2017": {"key": "Kandala2017", "title": "Hardware-efficient variational quantum eigensolver for small molecules and quantum magnets", "authors": "Kandala et al.", "year": "2017", "doi": "10.1038/nature23879"},
    "Kandala2019": {"key": "Kandala2019", "title": "Error mitigation extends the computational reach of a noisy quantum processor", "authors": "Kandala et al.", "year": "2019", "doi": "10.1038/s41586-019-1040-7"},
    "Sun2018PySCF": {"key": "Sun2018PySCF", "title": "PySCF: the Python-based simulations of chemistry framework", "authors": "Sun et al.", "year": "2018", "doi": "10.1002/wcms.1340"},
    "Farhi2014QAOA": {"key": "Farhi2014QAOA", "title": "A Quantum Approximate Optimization Algorithm", "authors": "Farhi, Goldstone, Gutmann", "year": "2014", "doi": "10.48550/arXiv.1411.4028"},
    "Harrigan2021": {"key": "Harrigan2021", "title": "Quantum approximate optimization of non-planar graph problems on a planar superconducting processor", "authors": "Harrigan et al.", "year": "2021", "doi": "10.1038/s41567-020-01105-y"},
    "Pfeuty1970": {"key": "Pfeuty1970", "title": "The one-dimensional Ising model with a transverse field", "authors": "Pfeuty", "year": "1970", "doi": "10.1016/0003-4916(70)90270-8"},
    "Kim2023": {"key": "Kim2023", "title": "Evidence for the utility of quantum computing before fault tolerance", "authors": "Kim et al.", "year": "2023", "doi": "10.1038/s41586-023-06096-3"},
    "Steane1996": {"key": "Steane1996", "title": "Error Correcting Codes in Quantum Theory", "authors": "Steane", "year": "1996", "doi": "10.1103/PhysRevLett.77.793"},
    "BravyiKitaev2005": {"key": "BravyiKitaev2005", "title": "Universal quantum computation with ideal Clifford gates and noisy ancillas", "authors": "Bravyi, Kitaev", "year": "2005", "doi": "10.1103/PhysRevA.71.022316"},
}


def canonical_citations_for_campaign(rows: List[Dict[str, Any]]) -> List[Dict[str, str]]:
    """Select DOI-backed canonical peer-reviewed citations matching the campaign kinds (G5)."""
    keys: list[str] = []
    kinds = {str(r.get("kind") or "").lower() for r in rows if isinstance(r, dict)}
    if any(k in kind for kind in kinds for k in ("vqe", "pes", "excited", "geometry", "reaction")):
        keys.extend(["Peruzzo2014", "OMalley2016", "Kandala2017", "Kandala2019", "Sun2018PySCF"])
    if any("qaoa" in kind for kind in kinds):
        keys.extend(["Farhi2014QAOA", "Harrigan2021"])
    if any(k in kind for kind in kinds for k in ("spin_chain", "tfim", "lattice", "thermodynamic")):
        keys.extend(["Pfeuty1970", "Kim2023"])
    if any(k in kind for kind in kinds for k in ("qec", "codesign")):
        keys.extend(["Steane1996", "BravyiKitaev2005"])
    seen: set[str] = set()
    out: List[Dict[str, str]] = []
    for k in keys:
        if k not in seen and k in CANONICAL_CITATION_METADATA:
            seen.add(k)
            out.append(dict(CANONICAL_CITATION_METADATA[k]))
    return out


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

    # R4: Hardware layout & per-qubit calibration summary when physical layout or calibration exists
    hw_cal_rows = [
        r for r in rows
        if bool(r.get("hardware")) and (r.get("layout") or r.get("used_qubits") or isinstance(r.get("calibration"), dict))
    ]
    if hw_cal_rows:
        lines.extend([
            "",
            r"\subsection*{Hardware Layout and Calibration Provenance}",
            r"\begin{table}[H]",
            r"\centering\scriptsize",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}llllrrr@{}}",
            r"\toprule",
            r"Job & Backend & Qubits & T1/T2 (us) & RO Err & 2Q Err & Snapshot \\",
            r"\midrule",
        ])
        for r in hw_cal_rows[:24]:
            jid = latex_escape(short_job_id(str(r.get("job_id") or "--")))
            be = latex_escape(str(r.get("backend") or "--")[:16])
            used = r.get("used_qubits") or r.get("layout")
            if isinstance(used, list) and used:
                q_s = latex_escape(",".join(str(x) for x in used[:10]) + ("..." if len(used) > 10 else ""))
            else:
                q_s = "device-default"
            cal = r.get("calibration") if isinstance(r.get("calibration"), dict) else {}
            snap = latex_escape(str(cal.get("snapshot_id") or r.get("calibration_snapshot_id") or "--")[:16])
            e2q = cal.get("median_two_qubit_error", cal.get("two_qubit_error"))
            ero = cal.get("median_readout_error", cal.get("readout_error"))
            # R-B: per-qubit values for the physical qubits that actually ran;
            # historical snapshots fall back to the device medians and say so.
            uqc = r.get("used_qubit_calibration") if isinstance(r.get("used_qubit_calibration"), dict) else {}
            t1s, t2s, ros = [], [], []
            for v in uqc.values():
                if not isinstance(v, dict):
                    continue
                if v.get("t1_us") is not None:
                    t1s.append(float(v["t1_us"]))
                if v.get("t2_us") is not None:
                    t2s.append(float(v["t2_us"]))
                if v.get("readout_error") is not None:
                    ros.append(float(v["readout_error"]))
            if t1s or t2s:
                t12_s = (f"{min(t1s):.0f}-{max(t1s):.0f}/{min(t2s):.0f}-{max(t2s):.0f}"
                         if t1s and t2s else "--")
                ro_s = f"$\\le${max(ros):.4f}" if ros else fmt_num(ero, 5)
            else:
                t1d, t2d = cal.get("t1_us"), cal.get("t2_us")
                t12_s = (f"{float(t1d):.0f}/{float(t2d):.0f} (device)"
                         if t1d is not None and t2d is not None else "--")
                ro_s = fmt_num(ero, 5)
            lines.append(f"{jid} & {be} & {q_s} & {t12_s} & {ro_s} & {fmt_num(e2q, 5)} & {snap} \\\\")
        lines.extend([
            r"\bottomrule",
            r"\end{tabular}%",
            r"}",
            r"\caption{Hardware layout, used physical qubits, and calibration snapshot mapping. T1/T2 and readout are per used qubit when the snapshot captured them; otherwise device medians are marked (device).}",
            r"\end{table}",
        ])

    # R5: Classical reference & solver settings table (per-molecule / DMRG / ED parameters)
    classical_rows = [
        r for r in rows
        if r.get("molecule") or r.get("basis") or r.get("active_space") or r.get("max_bond_dim") or r.get("bond_dim")
    ]
    if classical_rows:
        lines.extend([
            "",
            r"\subsection*{Classical Reference and Solver Settings}",
            r"\begin{table}[H]",
            r"\centering\scriptsize",
            r"\resizebox{\textwidth}{!}{%",
            r"\begin{tabular}{@{}lllll@{}}",
            r"\toprule",
            r"Job & Target / Model & Basis / Solver & Active Space / Bond Dim & Layers / Shots \\",
            r"\midrule",
        ])
        for r in classical_rows[:24]:
            jid = latex_escape(short_job_id(str(r.get("job_id") or "--")))
            tgt_s = str(r.get("molecule") or r.get("model") or r.get("kind") or "--")
            if r.get("bond_distance") is not None:
                tgt_s += f" R={float(r['bond_distance']):.3f}"
            elif r.get("n_sites") is not None:
                tgt_s += f" n={int(r['n_sites'])}"
            tgt = latex_escape(tgt_s[:26])
            basis = latex_escape(str(r.get("basis") or r.get("optimizer") or "--")[:16])
            asp = r.get("active_space")
            bdim = r.get("max_bond_dim", r.get("bond_dim"))
            if asp is not None:
                space_s = latex_escape(str(asp)[:18])
            elif bdim is not None:
                space_s = f"bond={int(bdim)}"
            else:
                space_s = "full-space"
            ls = f"L={r['layers']}" if r.get("layers") is not None else "--"
            sh = f"{int(r['shots'])} sh" if r.get("shots") is not None else "--"
            lines.append(f"{jid} & {tgt} & {basis} & {space_s} & {ls} / {sh} \\\\")
        lines.extend([
            r"\bottomrule",
            r"\end{tabular}%",
            r"}",
            r"\caption{Classical anchor and solver configuration per settled job (software lock hash archived in supplement/environment.json).}",
            r"\end{table}",
        ])
    return "\n".join(lines)


def build_zne_and_uncertainty_section(rows: List[Dict[str, Any]]) -> str:
    """Render Phase 4 + R2/R3 uncertainty, count-derived QAOA ratio CI, and ZNE summary table."""
    relevant = [
        r for r in rows
        if bool(r.get("mitigated"))
        or r.get("ci95_mha") is not None
        or r.get("raw_energy") is not None
        or r.get("delta_mha") is not None
        or r.get("approximation_ratio") is not None
    ]
    if not relevant:
        return ""
    # Hardware rows with a recorded interval are the claims a reader is most
    # likely to scrutinise; show them first so the 28-row cap cannot hide them.
    relevant.sort(key=lambda r: (
        0 if bool(r.get("hardware")) else 1,
        0 if (r.get("ci95_mha") is not None or r.get("ratio_ci95") is not None) else 1,
    ))
    lines = [
        "All discrepancies $\\Delta$ and approximation ratios below are reported alongside their "
        "recorded 95\\% confidence interval (\\texttt{CI95}) or a count-derived ratio interval when "
        "the sampled cut distribution is retained; entries without a sampled interval are explicitly tagged "
        "\\texttt{point estimate} (with explicit histogram retention status) and must not be "
        "read as statistically separated claims.",
        "",
        r"\begin{table}[H]",
        r"\centering\scriptsize",
        r"\resizebox{\textwidth}{!}{%",
        r"\begin{tabular}{@{}lllrrrr@{}}",
        r"\toprule",
        r"Job & Kind & Mode & Value & Anchor & $\Delta$ / Ratio & CI95 / Status \\",
        r"\midrule",
    ]
    shown = relevant[:28]
    for r in shown:
        jid = latex_escape(short_job_id(str(r.get("job_id") or "--")))
        kind = latex_escape(str(r.get("kind") or "--")[:16])
        mode = "ZNE mitigated" if bool(r.get("mitigated")) else ("hardware" if bool(r.get("hardware")) else "simulator")
        cstatus = str(r.get("counts_status") or "")
        if r.get("approximation_ratio") is not None:
            ratio = float(r["approximation_ratio"])
            interval = r.get("ratio_ci95")
            if isinstance(interval, (list, tuple)) and len(interval) == 2:
                # Server-derived from the sampled cut distribution; recorded in
                # the campaign so verification-v1 can trace these endpoints.
                ci_s = f"[{float(interval[0]):.3f}, {float(interval[1]):.3f}] (95\\% CI)"
            else:
                ci_s = r"\texttt{point estimate}"
            mc = r.get("measured_cut")
            oc = r.get("optimal_cut")
            lines.append(
                f"{jid} & {kind} & {mode} & {fmt_num(mc, 3)} & {fmt_num(oc, 3)} & {ratio:.4f} & {ci_s} \\\\"
            )
            continue
        e = r.get("energy", r.get("mitigated_energy", r.get("ground_energy")))
        anc = extract_anchor_energy(r)
        delta = r.get("delta_mha", r.get("delta"))
        if delta is None and e is not None and anc is not None:
            delta = abs(float(e) - float(anc)) * 1000.0
        ci = r.get("ci95_mha")
        if ci is not None:
            ci_s = f"$\\pm${fmt_num(ci, 3)} mHa"
        elif cstatus == "not_retained":
            ci_s = r"\texttt{point est. (no counts)}"
        else:
            ci_s = r"\texttt{point estimate}"
        lines.append(
            f"{jid} & {kind} & {mode} & {fmt_num(e, 6)} & {fmt_num(anc, 6)} & {fmt_num(delta, 3)} & {ci_s} \\\\"
        )
    if len(relevant) > len(shown):
        lines.append(
            rf"\multicolumn{{7}}{{l}}{{\textit{{Showing first {len(shown)} of {len(relevant)} rows; "
            rf"complete record in {_breakable_path('supplement/datasheet.json')}.}}}} \\\\"
        )
    lines.extend([
        r"\bottomrule",
        r"\end{tabular}%",
        r"}",
        r"\caption{Uncertainty, QAOA count-derived 95\% ratio CI, and Zero-Noise Extrapolation (ZNE) audit table. Mitigated points remain explicitly separated from raw hardware measurements.}",
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
