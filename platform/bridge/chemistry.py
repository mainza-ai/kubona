"""
Numerical Electronic Structure, Classical Physics Anchors & Molecular Discovery Pipeline.
Computes:
1. Restricted Hartree-Fock (RHF) and exact Full Configuration Interaction (FCI) energies
   using ab-initio quantum chemistry (PySCF).
2. Comprehensive ADMET property profiling, Lipinski Rule of 5, ESOL aqueous solubility,
   Caco-2 permeability, PAINS alerts, toxicophores, hERG risk flag, and SA score (RDKit).
3. Single-loop ADMET ranking over molecular candidates for a target protein.
Zero fake placeholders or empirical curve-fit approximations presented as measurement.
"""
from __future__ import annotations

import math
from typing import Dict, Any, List, Optional
import numpy as np

# ---------------------------------------------------------------------------
# Explicit validation + labeling constants (v1 heuristics say so).
# ---------------------------------------------------------------------------

#: Bond-distance sweep window (Angstrom). Outside this, minimal-basis RHF is
#: unphysical (compressed-core / dissociated-limit artifacts) — refuse.
R_MIN_ANGSTROM_V1 = 0.3
R_MAX_ANGSTROM_V1 = 5.0

#: Basis sets this bridge will run. Anything else refuses instead of letting
#: PySCF fail obscurely halfway through (or silently degrading accuracy).
BASIS_ALLOWLIST_V1 = frozenset({
    "sto-3g", "3-21g", "6-31g", "6-31g*", "6-31+g*", "cc-pvdz", "cc-pvtz",
    "def2-svp",
})

#: Variational slack (Hartree): FCI/CASCI must come in at or below RHF. A
#: positive correlation energy beyond this slack means solver failure — refuse.
VARIATIONAL_SLACK_HARTREE_V1 = 1e-6

#: In-house synthetic-accessibility weights (v1 heuristic — NOT the
#: Ertl–Schuffenhauer SA score, which is a trained fragment model).
SA_SCORE_WEIGHTS_V1_HEURISTIC = {
    "heavy_atom": 0.05,
    "stereo_center": 0.5,
    "ring": 0.4,
    "macrocycle": 2.0,
    "sp3_complement": 0.5,
}
SA_MODEL_LABEL_V1 = "sa_heuristic_v1 (in-house heuristic; NOT the Ertl-SAscores)"

#: Composite ranking weights (v1 heuristic — a ranking convenience, not a
#: measured developability score).
COMPOSITE_HEURISTIC_V1_WEIGHTS = {
    "lipinski_pass": 1.0,
    "lipinski_fail": 0.5,
    "pains_penalty": 0.5,
}
COMPOSITE_MODEL_LABEL_V1 = "composite_heuristic_v1"


def solve_classical_anchor(molecule: str, bond_distance_angstrom: float, basis: str = "sto-3g") -> Dict[str, Any]:
    """
    Computes genuine RHF and FCI energies along with nuclear repulsion and orbital energies
    using ab-initio PySCF numerical solvers.

    Returns a ``method`` flag: ``'FCI-exact'`` when the full orbital space fits
    in FCI, else ``'CASCI(ncas,nelecas)'`` naming the winning active space.
    CALLERS MUST CHECK ``method`` before citing ``fci_exact_energy`` as exact —
    a CASCI value is variational within its active space, not full FCI.
    """
    mol_upper = molecule.strip().upper()
    r = float(bond_distance_angstrom)
    if not math.isfinite(r) or not (R_MIN_ANGSTROM_V1 <= r <= R_MAX_ANGSTROM_V1):
        raise ValueError(
            f"bond distance {bond_distance_angstrom!r} refused: must be finite in "
            f"[{R_MIN_ANGSTROM_V1}, {R_MAX_ANGSTROM_V1}] Angstrom"
        )
    basis_norm = basis.strip().lower().replace(" ", "")
    if basis_norm not in BASIS_ALLOWLIST_V1:
        raise ValueError(
            f"basis {basis!r} refused: allowlist is {sorted(BASIS_ALLOWLIST_V1)}"
        )

    # Build atom specification
    if mol_upper in ("H2", "H_2"):
        atom_str = f"H 0 0 0; H 0 0 {r}"
    elif mol_upper in ("LIH", "LI_H"):
        atom_str = f"Li 0 0 0; H 0 0 {r}"
    elif mol_upper in ("BEH2", "BE_H2"):
        atom_str = f"Be 0 0 0; H 0 0 -{r}; H 0 0 {r}"
    elif mol_upper in ("H2O", "H_2O"):
        # Non-linear water geometry with angle ~104.5 deg
        theta_rad = np.radians(104.5 / 2.0)
        x = r * np.sin(theta_rad)
        y = r * np.cos(theta_rad)
        atom_str = f"O 0 0 0; H {x} {y} 0; H -{x} {y} 0"
    elif mol_upper in ("N2", "N_2"):
        atom_str = f"N 0 0 0; N 0 0 {r}"
    elif mol_upper in ("CO", "C_O"):
        atom_str = f"C 0 0 0; O 0 0 {r}"
    elif mol_upper in ("HF", "H_F"):
        atom_str = f"F 0 0 0; H 0 0 {r}"
    elif mol_upper in ("CO2", "CO_2"):
        atom_str = f"C 0 0 0; O 0 0 -{r}; O 0 0 {r}"
    elif mol_upper in ("NH3", "NH_3"):
        # Pyramidal NH3: fixed HNH-angle directions, N-H bond length = r.
        # The old code IGNORED r (hardcoded H positions ~1.014 A), so R-sweeps
        # silently returned one geometry's energy at every distance. Directions
        # below preserve the pyramidal angle; scaling sets N-H = r exactly.
        # (Angle relaxation along the sweep is NOT modeled — fixed-angle stretch.)
        dirs = [
            np.array([0.0, 0.94, 0.38]),
            np.array([0.81, -0.47, 0.38]),
            np.array([-0.81, -0.47, 0.38]),
        ]
        pts = [r * d / float(np.linalg.norm(d)) for d in dirs]
        h_str = "; ".join(f"H {p[0]} {p[1]} {p[2]}" for p in pts)
        atom_str = f"N 0 0 0; {h_str}"
    elif mol_upper in ("CH4", "CH_4"):
        # Symmetric tetrahedron: vertices (1,1,1), (1,-1,-1), (-1,1,-1),
        # (-1,-1,1) scaled by r/sqrt(3) — all six H-H distances equal
        # (2*sqrt(2/3)*r) and every C-H distance is r. The old code used sign
        # pattern (+,+,+),(-,+,+),(-,+,-),(+,-,-), which is NOT a tetrahedron
        # (three distinct H-H distances, +0.22 Ha above the symmetric energy
        # at r=1.09: -39.510 vs -39.727 Ha RHF/STO-3G) — regression-pinned below.
        s = r / math.sqrt(3.0)
        atom_str = (
            f"C 0 0 0; H {s} {s} {s}; H {s} {-s} {-s}; "
            f"H {-s} {s} {-s}; H {-s} {-s} {s}"
        )
    else:
        # Named larger molecules resolve to SMILES, then RDKit conformers.
        # Anything unresolvable REFUSES — an earlier revision silently
        # substituted H2 geometry and returned H2 energies labeled as the
        # requested molecule (success:true). That corrupted every large-
        # molecule anchor whenever the bridge was up. Never again.
        NAME_TO_SMILES = {
            "BENZENE": "c1ccccc1",
            "CAFFEINE": "CN1C=NC2=C1C(=O)N(C(=O)N2C)C",
            "ASPIRIN": "CC(=O)Oc1ccccc1C(=O)O",
            "PARACETAMOL": "CC(=O)Nc1ccc(O)cc1",
        }
        candidate = NAME_TO_SMILES.get(mol_upper, molecule.strip())
        try:
            from rdkit import Chem
            from rdkit.Chem import AllChem
            rd_mol = Chem.MolFromSmiles(candidate)
            if rd_mol is None:
                raise ValueError(f"not valid SMILES: {candidate!r}")
            rd_mol = Chem.AddHs(rd_mol)
            # NOTE: ETKDG conformer only (EmbedMolecule, randomSeed=42) — NO
            # force-field minimization (UFF/MMFF). Coordinates are an
            # unoptimized ETKDG geometry; energies from it are single-point
            # values at that geometry, not relaxed minima.
            if AllChem.EmbedMolecule(rd_mol, randomSeed=42) != 0:
                raise ValueError(f"conformer embedding failed for {candidate!r}")
            conf = rd_mol.GetConformer()
            coords = [f"{a.GetSymbol()} {conf.GetAtomPosition(i).x:.4f} {conf.GetAtomPosition(i).y:.4f} {conf.GetAtomPosition(i).z:.4f}" for i, a in enumerate(rd_mol.GetAtoms())]
            atom_str = "; ".join(coords)
        except Exception as e:
            raise RuntimeError(
                f"unsupported molecule {molecule.strip()!r}: no explicit geometry and not resolvable "
                f"to a conformer ({e}) — refusing instead of substituting another molecule"
            )
        # NOTE: no fallback geometry here. An earlier revision silently
        # substituted H2 and returned H2 energies as the requested molecule;
        # the except above now refuses, and dead fallback code was removed
        # (2026-10-12) so no reader can mistake it for a live path.

    method = "FCI-exact"
    cas_errors: List[Dict[str, Any]] = []
    try:
        from pyscf import gto, scf, fci, mcscf

        mol = gto.M(atom=atom_str, basis=basis_norm, verbose=0)
        e_nuc = float(mol.energy_nuc())

        # Restricted Hartree-Fock
        mf = scf.RHF(mol).run()
        e_rhf = float(mf.e_tot)
        mo_energies = [float(x) for x in mf.mo_energy]

        # Full Configuration Interaction (FCI) or Active Space CASCI.
        # PySCF rejects some (ncas, nelecas) combos outright (bare
        # AssertionError, e.g. CASCI(6,6) on 7-orbital H2O) — so candidates
        # run in fixed descending order with the first success winning.
        # Fixed order keeps energies deterministic; previously-working
        # molecules still hit (6,6) first and keep identical numbers, while
        # previously-failing geometries fall through instead of dying.
        # Total failure raises (honest outage) — never a substitute molecule.
        num_orbitals = int(mol.nao_nr())
        num_electrons = int(mol.nelectron)

        if num_orbitals <= 6:
            cisolver = fci.FCI(mf)
            e_fci, _ = cisolver.kernel()
            e_fci = float(e_fci)
            method = "FCI-exact"
        else:
            e_fci = None
            for ncas, nelecas in [(6, 6), (6, 4), (4, 4), (4, 2), (2, 2)]:
                if ncas > num_orbitals or nelecas > min(num_electrons, 2 * ncas):
                    continue
                try:
                    cas = mcscf.CASCI(mf, ncas, nelecas)
                    e_fci = float(cas.kernel()[0])
                    method = f"CASCI({ncas},{nelecas})"
                    break
                except Exception as e:
                    # Structured per-space errors (no string munging): callers
                    # and logs can attribute WHICH active space failed and why.
                    cas_errors.append({"ncas": ncas, "nelecas": nelecas,
                                       "error": str(e)})
            if e_fci is None:
                err = RuntimeError(
                    f"no CASCI active space converged for {mol_upper} "
                    f"({num_electrons}e/{num_orbitals}o)"
                )
                err.cas_errors = list(cas_errors)  # type: ignore[attr-defined]
                raise err

        # Variational guard: correlated energy must not sit above RHF beyond
        # solver-noise slack. A violation means the solve failed — refuse
        # rather than return an unphysical (positive-correlation) anchor.
        if not math.isfinite(e_fci) or e_fci > e_rhf + VARIATIONAL_SLACK_HARTREE_V1:
            raise RuntimeError(
                f"variational violation for {mol_upper}: correlated energy "
                f"{e_fci} above RHF {e_rhf} (slack {VARIATIONAL_SLACK_HARTREE_V1}): "
                "refusing instead of returning unphysical correlation"
            )

    except RuntimeError:
        # Already an honest refusal (unsupported molecule, CASCI outage,
        # variational violation) — re-raise WITHOUT double-wrapping, so the
        # original reason (and structured cas_errors) survives intact.
        raise
    except Exception as e:
        raise RuntimeError(f"PySCF ab-initio electronic structure calculation failed for {mol_upper} (basis={basis_norm}, R={r}): {e}")

    correlation_energy = float(e_fci - e_rhf)

    return {
        "molecule": mol_upper,
        "bond_distance": r,
        "basis": basis_norm,
        "rhf_energy": e_rhf,
        "fci_exact_energy": e_fci,
        "method": method,
        "correlation_energy": correlation_energy,
        "nuclear_repulsion": e_nuc,
        "orbital_energies": mo_energies,
    }


def evaluate_molecular_admet(smiles: str) -> Dict[str, Any]:
    """
    Computes genuine molecular descriptors, Lipinski compliance, aqueous solubility (ESOL),
    permeability-bin flags, PAINS filter alerts, toxicophore flags, a rule-based hERG-risk
    flag, and an in-house Synthetic Accessibility heuristic score using RDKit
    cheminformatics routines.

    Labels: every model below is a HEURISTIC reimplementation/binning, never a
    measurement — see the ``*_model`` fields (esol_delaney_v1_heuristic,
    caco2_cutoff_heuristic_v1, herg_heuristic_v1, sa_heuristic_v1,
    panel_v1_non_exhaustive).
    """
    from rdkit import Chem
    from rdkit.Chem import Descriptors, Crippen, Lipinski, QED, rdMolDescriptors
    from rdkit.Chem.FilterCatalog import FilterCatalog, FilterCatalogParams

    mol = Chem.MolFromSmiles(smiles.strip())
    if mol is None:
        raise ValueError(f"Invalid SMILES string provided: '{smiles}'")

    mw = float(Descriptors.MolWt(mol))
    logp = float(Crippen.MolLogP(mol))
    hbd = int(Lipinski.NumHDonors(mol))
    hba = int(Lipinski.NumHAcceptors(mol))
    tpsa = float(Descriptors.TPSA(mol))
    rotb = int(Lipinski.NumRotatableBonds(mol))
    qed_score = float(QED.qed(mol))
    heavy_atoms = int(mol.GetNumHeavyAtoms())

    # Lipinski Rule of 5
    violations = 0
    violation_reasons = []
    if mw > 500:
        violations += 1
        violation_reasons.append(f"MW {mw:.1f} > 500 Da")
    if logp > 5.0:
        violations += 1
        violation_reasons.append(f"cLogP {logp:.2f} > 5.0")
    if hbd > 5:
        violations += 1
        violation_reasons.append(f"H-Bond Donors {hbd} > 5")
    if hba > 10:
        violations += 1
        violation_reasons.append(f"H-Bond Acceptors {hba} > 10")

    lipinski_pass = violations <= 1

    # ESOL (Estimated Aqueous Solubility) - Delaney model, heuristic
    # reimplementation (Delaney, J Chem Inf Comput Sci 2004 — original is a
    # fitted MLR model; this is a formula reuse, NOT the fitted model).
    aromatic_atoms = sum(1 for a in mol.GetAtoms() if a.GetIsAromatic())
    ap = aromatic_atoms / heavy_atoms if heavy_atoms > 0 else 0.0
    esol_logs = 0.16 - (0.63 * logp) - (0.0062 * mw) + (0.066 * rotb) - (0.74 * ap)
    if esol_logs > -1.0:
        solubility_class = "Very Soluble"
    elif esol_logs > -3.0:
        solubility_class = "Soluble"
    elif esol_logs > -5.0:
        solubility_class = "Moderately Soluble"
    else:
        solubility_class = "Poorly Soluble"

    # Caco-2 permeability BINNING from TPSA/LogP/MW cutoffs — a heuristic bin,
    # not a Caco-2 assay. Cutoffs labeled as such; bins are not measurements.
    if tpsa < 90 and -0.5 <= logp <= 4.0 and mw < 450:
        permeability = "High"
    elif tpsa < 140 and mw < 500:
        permeability = "Moderate"
    else:
        permeability = "Low"

    # PAINS filter (Pan Assay Interference Compounds)
    params = FilterCatalogParams()
    params.AddCatalog(FilterCatalogParams.FilterCatalogs.PAINS)
    pains_catalog = FilterCatalog(params)
    pains_match = pains_catalog.GetFirstMatch(mol)
    pains_alerts = []
    if pains_match:
        pains_alerts.append(pains_match.GetDescription())

    # Toxicophore SMARTS panel — 6 patterns, NON-EXHAUSTIVE: absence of a hit
    # is not safety evidence, only "no hit in this 6-pattern panel".
    toxicophore_patterns = {
        "Michael Acceptor": "[C;H2,H1]=[C;H1,H0]-[C,S,P]=[O,S]",
        "Quinone": "O=C1C=CC(=O)C=C1",
        "Epoxide": "C1OC1",
        "Nitroaromatic": "c[N+](=O)[O-]",
        "Alkyl Halide": "[CX4][Cl,Br,I]",
        "Aliphatic Ester": "CC(=O)OC",
    }
    toxicity_alerts = []
    for alert_name, smarts in toxicophore_patterns.items():
        pat = Chem.MolFromSmarts(smarts)
        if pat and mol.HasSubstructMatch(pat):
            toxicity_alerts.append(alert_name)

    # hERG cardiotoxicity RISK FLAG (basic amine + high lipophilicity) — a
    # rule-based heuristic, NOT patch-clamp data: no IC50 is measured here,
    # and a "Low" flag is not a cardiac-safety claim.
    has_basic_amine = False
    amine_pat = Chem.MolFromSmarts("[#6]-[NH1,NH0](-[#6])-[#6]")
    if amine_pat and mol.HasSubstructMatch(amine_pat):
        has_basic_amine = True
    herg_risk = "Low"
    if has_basic_amine and logp > 3.0:
        herg_risk = "High (Lipophilic basic amine alert)"
    elif logp > 4.5:
        herg_risk = "Moderate (High lipophilicity)"

    # Synthetic Accessibility heuristic (v1 weights — in-house, NOT Ertl).
    num_stereo = len(Chem.FindMolChiralCenters(mol, includeUnassigned=True))
    num_rings = mol.GetRingInfo().NumRings()
    macrocycles = sum(1 for r in mol.GetRingInfo().AtomRings() if len(r) > 8)
    sp3_frac = float(rdMolDescriptors.CalcFractionCSP3(mol))
    w = SA_SCORE_WEIGHTS_V1_HEURISTIC
    sa_score = (1.0 + (heavy_atoms * w["heavy_atom"]) + (num_stereo * w["stereo_center"])
                + (num_rings * w["ring"]) + (macrocycles * w["macrocycle"])
                + ((1.0 - sp3_frac) * w["sp3_complement"]))
    sa_score = min(10.0, max(1.0, float(sa_score)))

    return {
        "smiles": smiles,
        "molecular_weight": mw,
        "clogp": logp,
        "hbd": hbd,
        "hba": hba,
        "tpsa": tpsa,
        "rotatable_bonds": rotb,
        "qed": qed_score,
        "lipinski": {
            "pass": lipinski_pass,
            "violations": violations,
            "reasons": violation_reasons,
        },
        "admet": {
            "solubility_logs": float(esol_logs),
            "solubility_class": solubility_class,
            "solubility_model": "esol_delaney_v1_heuristic (Delaney 2004 heuristic reuse; not the fitted model)",
            "caco2_permeability": permeability,
            "permeability_model": "caco2_cutoff_heuristic_v1 (TPSA/LogP/MW cutoff bins; heuristic, not Caco-2 data)",
            "herg_risk": herg_risk,
            "herg_model": "herg_heuristic_v1 (rule-based flag; NOT patch-clamp data, no IC50 measured)",
            "toxicity_alerts": toxicity_alerts,
            "toxicity_panel_model": "panel_v1_non_exhaustive (6 SMARTS; absence of hits is not safety evidence)",
        },
        "pains_alerts": pains_alerts,
        "sa_score": sa_score,
        "sa_model": SA_MODEL_LABEL_V1,
        "synthesizability": "High (Synthesizable)" if sa_score < 4.0 else ("Moderate" if sa_score < 6.5 else "Challenging"),
    }


def evaluate_candidate_pipeline(target_name: str, candidates: List[Dict[str, str]]) -> Dict[str, Any]:
    """
    Single-loop ADMET ranking over candidate molecules for a target protein.

    Honest name for what this is: one loop over candidates, scoring each with
    evaluate_molecular_admet and ranking by a composite heuristic. There is a
    single loop and no agents; the historical docstring name overstated the
    machinery and has been replaced with this description.
    """
    evaluated = []
    for cand in candidates:
        name = cand.get("name", "Unknown Candidate")
        smiles = cand.get("smiles", "")
        try:
            profile = evaluate_molecular_admet(smiles)
            profile["name"] = name
            evaluated.append(profile)
        except Exception as e:
            evaluated.append({
                "name": name,
                "smiles": smiles,
                "error": str(e),
            })

    # Rank candidates by composite drug-likeness heuristic v1:
    # QED * (1 - SA/10) * LipinskiFactor * PAINSFactor (weights const above).
    for cand in evaluated:
        if "qed" in cand:
            w = COMPOSITE_HEURISTIC_V1_WEIGHTS
            lip_mult = w["lipinski_pass"] if cand["lipinski"]["pass"] else w["lipinski_fail"]
            sa_norm = max(0.1, 1.0 - (cand["sa_score"] / 10.0))
            pains_pen = w["pains_penalty"] if len(cand["pains_alerts"]) > 0 else 1.0
            cand["composite_score"] = float(cand["qed"] * sa_norm * lip_mult * pains_pen)
            cand["composite_model"] = COMPOSITE_MODEL_LABEL_V1
        else:
            cand["composite_score"] = 0.0
            cand["composite_model"] = COMPOSITE_MODEL_LABEL_V1

    evaluated.sort(key=lambda x: x.get("composite_score", 0), reverse=True)

    valid = [c for c in evaluated if "error" not in c]
    return {
        "target": target_name,
        "total_evaluated": len(evaluated),
        "candidates": evaluated,
        # None when NOTHING valid ranked (the old code returned evaluated[0]
        # even when every candidate errored — an error dict as "top lead").
        "top_lead": valid[0] if valid else None,
    }
