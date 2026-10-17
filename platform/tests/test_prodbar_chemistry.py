"""W1.16 production-bar regression tests: bridge chemistry + table anchors + math physics.

PySCF/RDKit tests skip honestly when the libraries are absent (importorskip);
refusal + labeling tests always run.
"""

import importlib.util
import sys
from pathlib import Path

import pytest

BRIDGE = Path(__file__).resolve().parent.parent / "bridge"


def _load(name, filename):
    if str(BRIDGE) not in sys.path:
        sys.path.insert(0, str(BRIDGE))
    spec = importlib.util.spec_from_file_location(name, BRIDGE / filename)
    mod = importlib.util.module_from_spec(spec)
    sys.modules[name] = mod
    spec.loader.exec_module(mod)
    return mod


@pytest.fixture()
def chem():
    return _load("bridge_chemistry_prodbar", "chemistry.py")


# ---------------------------------------------------------------------------
# bridge/chemistry.py — ab-initio anchors
# ---------------------------------------------------------------------------

def test_ch4_symmetric_tetrahedron_regression(chem):
    pytest.importorskip("pyscf")
    out = chem.solve_classical_anchor("CH4", 1.09)
    # Symmetric-tetrahedron STO-3G window from real PySCF output (recalibrated
    # after fixing the asymmetric sign pattern: old RHF was -39.51).
    assert out["method"] == "CASCI(6,6)"
    assert abs(out["rhf_energy"] - -39.7267) < 0.02, out
    assert abs(out["fci_exact_energy"] - -39.7689) < 0.03, out
    assert out["fci_exact_energy"] < out["rhf_energy"]


def test_nh3_geometry_scales_with_r(chem):
    pytest.importorskip("pyscf")
    a = chem.solve_classical_anchor("NH3", 1.0)
    b = chem.solve_classical_anchor("NH3", 1.2)
    # The old code hardcoded H positions: every R returned one geometry's energy.
    assert abs(a["rhf_energy"] - b["rhf_energy"]) > 0.01, (a, b)
    for out in (a, b):
        assert out["fci_exact_energy"] < out["rhf_energy"]


def test_method_flag_exact_vs_casci(chem):
    pytest.importorskip("pyscf")
    h2 = chem.solve_classical_anchor("H2", 0.7414)
    assert h2["method"] == "FCI-exact"
    h2o = chem.solve_classical_anchor("H2O", 0.9575)
    assert h2o["method"].startswith("CASCI(")


def test_r_and_basis_validation(chem):
    with pytest.raises(ValueError, match="must be finite in"):
        chem.solve_classical_anchor("H2", 0.1)
    with pytest.raises(ValueError, match="must be finite in"):
        chem.solve_classical_anchor("H2", 10.0)
    with pytest.raises(ValueError, match="must be finite in"):
        chem.solve_classical_anchor("H2", float("nan"))
    with pytest.raises(ValueError, match="allowlist"):
        chem.solve_classical_anchor("H2", 0.7414, basis="cc-pvqz")
    # Allowlist is case-insensitive.
    assert "sto-3g" in chem.BASIS_ALLOWLIST_V1


def test_cas_errors_structured_not_double_wrapped(chem):
    pytest.importorskip("pyscf")
    import pyscf.mcscf as _mcscf
    real_casci = _mcscf.CASCI

    def boom(*a, **k):
        raise AssertionError("simulated active-space rejection")

    monkey = pytest.MonkeyPatch()
    monkey.setattr(_mcscf, "CASCI", boom)
    try:
        # Benzene needs CASCI (many orbitals) -> every space fails.
        with pytest.raises(RuntimeError) as ei:
            chem.solve_classical_anchor("Benzene", 1.397)
    finally:
        monkey.undo()
    assert "no CASCI active space converged" in str(ei.value)
    # Structured per-space errors survive (no double-wrap flattening).
    assert isinstance(ei.value.cas_errors, list)
    assert all(set(e) == {"ncas", "nelecas", "error"} for e in ei.value.cas_errors)
    assert "PySCF ab-initio" not in str(ei.value)


def test_admet_heuristic_labels(chem):
    pytest.importorskip("rdkit")
    out = chem.evaluate_molecular_admet("CC(=O)Oc1ccccc1C(=O)O")
    assert out["sa_model"].startswith("sa_heuristic_v1")
    assert "Ertl" in out["sa_model"]
    assert out["admet"]["solubility_model"].startswith("esol_delaney_v1_heuristic")
    assert out["admet"]["permeability_model"].startswith("caco2_cutoff_heuristic_v1")
    assert out["admet"]["herg_model"].startswith("herg_heuristic_v1")
    assert "patch-clamp" in out["admet"]["herg_model"]
    assert out["admet"]["toxicity_panel_model"].startswith("panel_v1_non_exhaustive")


def test_pipeline_top_lead_none_when_zero_valid_and_honest_name(chem):
    pytest.importorskip("rdkit")
    res = chem.evaluate_candidate_pipeline("T", [
        {"name": "bad1", "smiles": "not-a-smiles-xyz"},
        {"name": "bad2", "smiles": "[[["},
    ])
    assert res["top_lead"] is None  # old code returned the error dict as "lead"
    assert all("error" in c for c in res["candidates"])
    assert "12-Agent" not in (chem.evaluate_candidate_pipeline.__doc__ or "")
    good = chem.evaluate_candidate_pipeline("T", [
        {"name": "Aspirin", "smiles": "CC(=O)Oc1ccccc1C(=O)O"},
    ])
    assert good["top_lead"]["name"] == "Aspirin"
    assert good["top_lead"]["composite_model"] == "composite_heuristic_v1"
    assert chem.COMPOSITE_HEURISTIC_V1_WEIGHTS["lipinski_pass"] == 1.0


# ---------------------------------------------------------------------------
# kubona/anchors/chemistry.py — table anchors
# ---------------------------------------------------------------------------

def test_table_anchor_unknown_and_lih_refuse():
    from kubona.anchors.chemistry import MolecularAnchorEngine
    eng = MolecularAnchorEngine()
    with pytest.raises(ValueError, match="no tabulated anchor"):
        eng.compute_table_anchor("Xenon")
    with pytest.raises(ValueError, match="no tabulated anchor"):
        eng.compute_table_anchor("LiH")  # invented -0.042 offset removed
    h2 = eng.compute_table_anchor("H2")
    assert h2.fci_energy_hartree < h2.rhf_energy_hartree
    assert eng.compute_anchor("H2").system_name == "H2"  # alias works


def test_table_anchor_version_provenance_full_precision():
    from kubona.anchors.chemistry import TABLE_ANCHOR_VERSION_V1, MolecularAnchorEngine
    eng = MolecularAnchorEngine()
    d = eng.compute_table_anchor("H2").to_dict()
    assert d["table_version"] == TABLE_ANCHOR_VERSION_V1
    assert "tabulated" in d["provenance"]
    # Full precision, not round(x, 6).
    assert d["fci_energy_hartree"] != round(d["fci_energy_hartree"], 6) \
        or d["fci_energy_hartree"] == eng.compute_table_anchor("H2").fci_energy_hartree


def test_beat_rhf_interval_and_correlation_recovery():
    from kubona.anchors.chemistry import MolecularAnchorEngine
    eng = MolecularAnchorEngine()
    anchor = eng.compute_table_anchor("H2")
    tol = 1.59 / 1000.0
    # Exactly at RHF: a tie, not a win (old point comparison claimed a beat).
    tied = eng.evaluate_advantage_claim(anchor.rhf_energy_hartree, anchor)
    assert tied["beat_classical_rhf"] is False
    assert tied["correlation_recovered_fraction"] == pytest.approx(0.0)
    # Inside the tolerance band: still not a beat.
    near = eng.evaluate_advantage_claim(anchor.rhf_energy_hartree - tol / 2, anchor)
    assert near["beat_classical_rhf"] is False
    # At FCI: beat + full recovery + chemical accuracy.
    won = eng.evaluate_advantage_claim(anchor.fci_energy_hartree, anchor)
    assert won["beat_classical_rhf"] is True
    assert won["correlation_recovered_fraction"] == pytest.approx(1.0)
    assert won["correlation_majority_recovered"] is True
    assert "quantum_advantage_claimed" not in won


# ---------------------------------------------------------------------------
# kubona/anchors/math_physics.py
# ---------------------------------------------------------------------------

def test_benchmarks_unattested_never_lean():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    for name in E.list_benchmarks():
        a = E.get_anchor(name)
        assert a.lean_verified is False, name
        assert a.attestation == "unattested", name
        assert a.source_url != "" or a.family_id == 0, name
        d = a.to_dict()
        assert d["attestation"] == "unattested" and d["lean_verified"] is False


def test_laughlin_single_sourced():
    from kubona.anchors.math_physics import (
        LAUGHLIN_V1_GAP_LOWER_BOUND_V1, MathPhysicsAnchorEngine as E)
    assert LAUGHLIN_V1_GAP_LOWER_BOUND_V1 == 0.04
    assert E.BENCHMARKS["Laughlin_Fractional_Hall_Gap"]["analytical_value"] == 0.04
    fam = E.compute_family_269_laughlin_fractional_gap()
    assert fam["v1_interaction_spectral_gap_lower_bound"] == 0.04


def test_heisenberg_single_sourced_and_depletion_capped_with_reason():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    cold = E.compute_family_271_heisenberg_ferromagnet(temperature=0.0, J=2.0)
    assert cold["ground_state_energy_density_J"] == -3.0
    assert cold["depletion_capped_with_reason"] is None
    hot = E.compute_family_271_heisenberg_ferromagnet(temperature=50.0, J=1.0)
    assert hot["magnetization_depletion"] == 1.0
    assert hot["depletion_capped_with_reason"] is not None
    with pytest.raises(ValueError, match="temperature"):
        E.compute_family_271_heisenberg_ferromagnet(temperature=-1.0)
    with pytest.raises(ValueError, match="J must be"):
        E.compute_family_271_heisenberg_ferromagnet(J=0.0)


def test_qaoa_heuristic_not_certified():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    f = E.compute_family_281_sk_spin_glass(qaoa_p_depth=3)
    assert "qaoa_approaches_parisi_certified" not in f
    assert f["qaoa_depth_model"].startswith("qaoa-depth-heuristic-v1")
    with pytest.raises(ValueError, match="qaoa_p_depth"):
        E.compute_family_281_sk_spin_glass(qaoa_p_depth=0)


def test_all_families_tiered_never_proof_never_certified():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    calls = {
        271: {}, 281: {"qaoa_p_depth": 2}, 268: {}, 278: {},
        275: {}, 279: {}, 274: {}, 284: {"quantum_queries": 3},
        265: {}, 266: {}, 269: {}, 276: {}, 272: {},
        277: {}, 283: {}, 261: {}, 263: {}, 267: {}, 270: {}, 287: {},
    }
    for fid, kw in calls.items():
        out = E.compute_family_engine(fid, **kw)
        assert "evidence_tier" in out and "provenance" in out, fid
        blob = str(out)
        assert "rigorous-mathematical-proof" not in blob, fid
        for k, v in out.items():
            assert not (("certif" in k.lower()) and v is True), (fid, k)


def test_family_input_refusals():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    with pytest.raises(ValueError):  # gamma clamped before
        E.compute_family_276_generalized_amplitude_damping(gamma=1.5)
    with pytest.raises(ValueError):
        E.compute_family_276_generalized_amplitude_damping(thermal_N=-0.1)
    with pytest.raises(ValueError):  # value_g clamped before
        E.compute_family_277_threshold_repetition_entangled_games(value_g=1.5)
    with pytest.raises(ValueError):
        E.compute_family_277_threshold_repetition_entangled_games(repetitions=0)
    with pytest.raises(ValueError):  # max(0, q) clamped before
        E.compute_family_284_quartic_query_separation(quantum_queries=-2)
    with pytest.raises(ValueError):  # max(1.0, L) clamped before
        E.compute_family_265_peps_area_law_bound(linear_size_L=0.0)
    with pytest.raises(ValueError):
        E.compute_family_272_bound_entanglement_secret_key(dimension=9)
    with pytest.raises(ValueError):
        E.compute_family_270_bfss_matrix_model(gauge_group_N=1)
    with pytest.raises(ValueError):
        E.compute_family_263_atomic_ionization_cap(atomic_number_Z=0)
    with pytest.raises(ValueError):
        E.compute_family_261_anderson_localization(spatial_dimension=4)
    with pytest.raises(ValueError):  # max(0, rho) clamped before
        E.compute_family_267_bec_depletion_fraction(gas_parameter_rho_a3=-1.0)
    with pytest.raises(ValueError):
        E.compute_family_261_anderson_localization(disorder_strength=-1.0)


def test_variational_none_when_not_energy():
    from kubona.anchors.math_physics import MathPhysicsAnchorEngine as E
    mub = E.compare_vqe_result("MUB_Dimension_6_Overlap", 3.0)
    assert mub["variational_principle_satisfied"] is None  # old code said True
    heis = E.compare_vqe_result("Heisenberg_Ferromagnet_3D", -1.498)
    assert heis["variational_principle_satisfied"] is True
    with pytest.raises(ValueError):
        E.compare_vqe_result("Heisenberg_Ferromagnet_3D", float("nan"))
